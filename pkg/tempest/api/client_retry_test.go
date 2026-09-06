package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestRedactTokenKeepsTimeoutClassification checks that sanitizing a transport
// error drops every detail except whether it was a timeout.
func TestRedactTokenKeepsTimeoutClassification(t *testing.T) {
	t.Parallel()
	c := &Client{}
	if got := c.redactToken(nil); got != nil {
		t.Fatalf("redactToken(nil) = %v, want nil", got)
	}

	leaky := &url.Error{Op: "Get", URL: "https://example.invalid/stations?token=secret-value", Err: errors.New("dial tcp 10.0.0.1:443: refused")}
	got := c.redactToken(leaky)
	if !errors.Is(got, ErrTransport) {
		t.Fatalf("errors.Is(got, ErrTransport) = false for %v", got)
	}
	if errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("connection refused must not classify as a timeout: %v", got)
	}
	for _, secret := range []string{"secret-value", "example.invalid", "10.0.0.1"} {
		if strings.Contains(got.Error(), secret) {
			t.Fatalf("sanitized error %q leaks %q", got.Error(), secret)
		}
	}

	timedOut := c.redactToken(&url.Error{Op: "Get", URL: "https://example.invalid/stations", Err: context.DeadlineExceeded})
	if !errors.Is(timedOut, ErrTransport) || !errors.Is(timedOut, context.DeadlineExceeded) {
		t.Fatalf("timeout must match both ErrTransport and context.DeadlineExceeded: %v", timedOut)
	}
	var timeouter interface{ Timeout() bool }
	if !errors.As(timedOut, &timeouter) || !timeouter.Timeout() {
		t.Fatalf("timeout must report Timeout() == true: %v", timedOut)
	}
	var original *url.Error
	if errors.As(timedOut, &original) || errors.As(got, &original) {
		t.Fatal("sanitized errors retain the sensitive original cause")
	}
	dnsTimeout := c.redactToken(&net.DNSError{Name: "private.invalid", Err: "secret-value", IsTimeout: true})
	if !errors.Is(dnsTimeout, context.DeadlineExceeded) || strings.Contains(dnsTimeout.Error(), "private.invalid") || strings.Contains(dnsTimeout.Error(), "secret-value") {
		t.Fatalf("network timeout was misclassified or leaked details: %v", dnsTimeout)
	}
}

func TestHTTP408Retries(t *testing.T) {
	t.Parallel()
	for _, recoverAfterTwo := range []bool{true, false} {
		name := "exhausted"
		if recoverAfterTwo {
			name = "recovered"
		}
		t.Run(name, func(t *testing.T) {
			var attempts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempt := attempts.Add(1)
				if recoverAfterTwo && attempt == 3 {
					writeTestJSON(w, stationsJSON)
					return
				}
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusRequestTimeout)
			}))
			t.Cleanup(srv.Close)
			client := mustClient(t, "synthetic-test-token", WithBaseURL(srv.URL), WithCacheTTL(0),
				WithRetryPolicy(RetryPolicy{MaxAttempts: 3, BaseWait: time.Millisecond, MaxWait: 2 * time.Millisecond}))
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			stations, err := client.Stations(ctx)
			if recoverAfterTwo {
				if err != nil || len(stations) != 1 {
					t.Fatalf("recovery returned %v, %v", stations, err)
				}
			} else {
				var failure *HTTPError
				if !errors.As(err, &failure) || failure.StatusCode != http.StatusRequestTimeout || !failure.Retryable {
					t.Fatalf("exhaustion returned %v, want retryable HTTP 408", err)
				}
			}
			if got := attempts.Load(); got != 3 {
				t.Fatalf("attempts = %d, want 3", got)
			}
		})
	}
}

// TestRequestTimeoutIsClassified drives a real per-attempt timeout through the
// client and checks that the caller can still tell it was a timeout.
func TestRequestTimeoutIsClassified(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	client, err := New("synthetic-test-token",
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
		WithCacheTTL(0),
		WithRequestTimeout(50*time.Millisecond),
		WithRetryPolicy(RetryPolicy{MaxAttempts: 1, BaseWait: time.Millisecond, MaxWait: time.Millisecond}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.Stations(ctx)
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("want ErrTransport, got %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want a timeout classification, got %v", err)
	}
	if strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("sanitized error leaks the endpoint: %q", err.Error())
	}
}

// TestBackoffWaitBounds checks the jittered delay stays inside its contract for
// every attempt a policy allows.
func TestBackoffWaitBounds(t *testing.T) {
	t.Parallel()
	policy := RetryPolicy{MaxAttempts: 10, BaseWait: 100 * time.Millisecond, MaxWait: 2 * time.Second}
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		ceiling := min(policy.BaseWait<<(attempt-1), policy.MaxWait)
		for range 200 {
			wait := backoffWait(attempt, 0, policy)
			if wait < policy.BaseWait || wait > ceiling {
				t.Fatalf("attempt %d: wait %v outside [%v, %v]", attempt, wait, policy.BaseWait, ceiling)
			}
		}
	}

	// Retry-After is a floor, MaxWait is a hard ceiling.
	if wait := backoffWait(1, 1500*time.Millisecond, policy); wait != 1500*time.Millisecond {
		t.Fatalf("Retry-After floor not honored: %v", wait)
	}
	if wait := backoffWait(1, time.Hour, policy); wait != policy.MaxWait {
		t.Fatalf("MaxWait ceiling not honored: %v", wait)
	}
}
