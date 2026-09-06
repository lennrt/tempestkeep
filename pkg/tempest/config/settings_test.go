package config

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lennrt/tempestkeep/pkg/tempest/api"
)

func TestParseBool(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"1", true}, {"true", true}, {"TRUE", true}, {" yes ", true}, {"On", true},
		{"", false}, {"0", false}, {"false", false}, {"no", false}, {"OFF", false},
	} {
		got, err := ParseBool(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("ParseBool(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
		}
	}
	for _, in := range []string{"ture", "2", "y", "enabled", "true false"} {
		if _, err := ParseBool(in); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("ParseBool(%q) error = %v; want ErrInvalidConfig", in, err)
		}
	}
	const secret = "synthetic-sensitive-value"
	if _, err := ParseBool(secret); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("invalid boolean was accepted or echoed: %v", err)
	}
}

func TestClientOptions(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		ttl          time.Duration
		wantRequests int32
	}{
		{"cache enabled", time.Minute, 1},
		{"cache disabled", 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/custom/stations" {
					t.Errorf("unexpected configured endpoint: %q", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"stations":[]}`))
			}))
			t.Cleanup(srv.Close)
			settings := APISettings{BaseURL: srv.URL + "/custom", CacheTTL: tc.ttl}
			client, err := api.New("synthetic-test-token", settings.ClientOptions()...)
			if err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 0 {
				t.Fatal("client construction performed I/O")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			for range 2 {
				if _, err := client.Stations(ctx); err != nil {
					t.Fatal(err)
				}
			}
			if got := requests.Load(); got != tc.wantRequests {
				t.Fatalf("requests = %d, want %d", got, tc.wantRequests)
			}
		})
	}
	for _, settings := range []APISettings{{BaseURL: "file:///private"}, {CacheTTL: -time.Second}} {
		if _, err := api.New("synthetic-test-token", settings.ClientOptions()...); !errors.Is(err, api.ErrInvalidArgument) {
			t.Fatalf("invalid settings should be rejected by api.New: %v", err)
		}
	}
}

func TestAPISettingsFromEnvDefaults(t *testing.T) {
	t.Setenv("TEMPEST_API_BASE", "")
	t.Setenv("TEMPEST_CACHE_TTL", "")
	settings, err := APISettingsFromEnv()
	if err != nil {
		t.Fatalf("APISettingsFromEnv: %v", err)
	}
	if settings.CacheTTL != DefaultCacheTTL || settings.BaseURL != "" {
		t.Fatalf("defaults = %+v, want empty base URL and %v cache", settings, DefaultCacheTTL)
	}
}
