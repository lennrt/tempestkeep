package api

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"
)

// Clients share idle connections, as they did with http.DefaultTransport.
// This private transport cannot be weakened by replacing that global variable.
// Construction performs no I/O; TLS and randomness remain in Go's standard library.
var apiTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: time.Second,
	TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		// TLS 1.3 always uses AEAD. For TLS 1.2, require ephemeral key
		// agreement and AEAD; exclude RSA key exchange, CBC, RC4, and 3DES.
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
		// Normal chain and hostname verification happens first. This adds
		// minimum RSA and EC certificate key sizes without replacing it.
		VerifyConnection: verifyCertificateKeySizes,
	},
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Transport: apiTransport,
		// A redirect can copy the token query into an attacker-controlled
		// Location or send a sensitive Referer. Surface the 3xx as HTTPError.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func verifyCertificateKeySizes(state tls.ConnectionState) error {
	if len(state.VerifiedChains) == 0 {
		return errors.New("TLS certificate chain is not verified")
	}
	// Accept if at least one normally verified chain meets the size policy.
	for _, chain := range state.VerifiedChains {
		strong := len(chain) > 0
		for _, cert := range chain {
			switch key := cert.PublicKey.(type) {
			case *rsa.PublicKey:
				strong = strong && key.N.BitLen() >= 2048
			case *ecdsa.PublicKey:
				strong = strong && key.Curve.Params().BitSize >= 224
			}
		}
		if strong {
			return nil
		}
	}
	return errors.New("TLS certificate key is too small")
}
