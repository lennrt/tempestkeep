package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDefaultTransportTLS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version uint16
		suite   uint16
		trust   bool
		wantOK  bool
	}{
		{"TLS 1.3", tls.VersionTLS13, 0, true, true},
		{"TLS 1.2 AEAD", tls.VersionTLS12, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256, true, true},
		{"TLS 1.2 CBC", tls.VersionTLS12, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, true, false},
		{"TLS 1.1", tls.VersionTLS11, tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA, true, false},
		{"untrusted certificate", tls.VersionTLS13, 0, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeTestJSON(w, stationsJSON)
			}))
			srv.Config.ErrorLog = log.New(io.Discard, "", 0)
			srv.TLS = &tls.Config{MinVersion: tc.version, MaxVersion: tc.version}
			if tc.suite != 0 {
				srv.TLS.CipherSuites = []uint16{tc.suite}
			}
			srv.StartTLS()
			t.Cleanup(srv.Close)

			client := newTestClient(t, srv.URL)
			transport := apiTransport.Clone()
			t.Cleanup(transport.CloseIdleConnections)
			if tc.trust {
				roots := x509.NewCertPool()
				roots.AddCert(srv.Certificate())
				transport.TLSClientConfig.RootCAs = roots
			}
			client.http.Transport = transport
			client.retry.MaxAttempts = 1
			_, err := client.Stations(t.Context())
			if tc.wantOK && err != nil {
				t.Fatal(err)
			}
			if !tc.wantOK && !errors.Is(err, ErrTransport) {
				t.Fatalf("insecure TLS connection returned %v, want ErrTransport", err)
			}
		})
	}
}

func TestDefaultClientDoesNotFollowRedirects(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		writeTestJSON(w, stationsJSON)
	}))
	t.Cleanup(target.Close)
	for _, status := range []int{301, 302, 303, 307, 308} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"?token=test-token", status)
		}))
		t.Cleanup(srv.Close)
		_, err := newTestClient(t, srv.URL).Stations(t.Context())
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != status || httpErr.Retryable {
			t.Errorf("redirect %d returned %v, want a non-retryable HTTPError", status, err)
		}
	}
	if got := targetCalls.Load(); got != 0 {
		t.Fatalf("redirect destination received %d requests", got)
	}
}

func TestCertificateKeySizes(t *testing.T) {
	strongRSA := &x509.Certificate{PublicKey: &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 2047)}}
	weakRSA := &x509.Certificate{PublicKey: &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 1023)}}
	strongEC := &x509.Certificate{PublicKey: &ecdsa.PublicKey{Curve: elliptic.P256()}}
	for _, tc := range []struct {
		name   string
		chains [][]*x509.Certificate
		wantOK bool
	}{
		{"unverified", nil, false},
		{"empty chain", [][]*x509.Certificate{{}}, false},
		{"RSA 2048", [][]*x509.Certificate{{strongRSA}}, true},
		{"RSA 1024", [][]*x509.Certificate{{weakRSA}}, false},
		{"weak issuer", [][]*x509.Certificate{{strongEC, weakRSA}}, false},
		{"alternate strong chain", [][]*x509.Certificate{{strongEC, weakRSA}, {strongEC, strongRSA}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyCertificateKeySizes(tls.ConnectionState{VerifiedChains: tc.chains})
			if (err == nil) != tc.wantOK {
				t.Fatalf("key size verification returned %v, want success=%t", err, tc.wantOK)
			}
		})
	}
}
