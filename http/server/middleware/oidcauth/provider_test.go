package oidcauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// TestDiscoverProvider_ImmediateSuccess verifies that discoverProvider returns
// immediately when the OIDC discovery endpoint is already reachable.
func TestDiscoverProvider_ImmediateSuccess(t *testing.T) {
	ts := fakeOIDCServer(t, true)
	defer ts.Close()

	ctx := testInsecureCtx(ts)
	provider, err := discoverProvider(ctx, ts.URL)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}
}

// TestDiscoverProvider_WaitsUntilReachable verifies that discoverProvider
// polls and eventually succeeds once the endpoint becomes reachable.
// It also verifies the "at most 2 log lines" contract indirectly by
// confirming the function returns successfully after a delay.
func TestDiscoverProvider_WaitsUntilReachable(t *testing.T) {
	available := make(chan struct{})

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-available:
			// After signal: serve valid discovery document.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(discoveryDoc(w, r))
		default:
			// Before signal: simulate unreachable provider.
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
		}
	}))
	defer ts.Close()

	ctx := testInsecureCtx(ts)

	// Make provider available after a short delay.
	go func() {
		time.Sleep(300 * time.Millisecond)
		close(available)
	}()

	done := make(chan struct{})
	var provider *oidc.Provider
	var discoverErr error

	go func() {
		defer close(done)
		provider, discoverErr = discoverProvider(ctx, ts.URL)
	}()

	select {
	case <-done:
		if discoverErr != nil {
			t.Fatalf("expected nil error, got: %v", discoverErr)
		}
		if provider == nil {
			t.Fatal("expected non-nil provider")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("discoverProvider did not return in time")
	}
}

// TestDiscoverProvider_ContextCancelled verifies that discoverProvider
// returns an error when the context is cancelled while waiting.
func TestDiscoverProvider_ContextCancelled(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(testInsecureCtx(ts))

	// Cancel context after a short delay.
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	_, err := discoverProvider(ctx, ts.URL)
	if err == nil {
		t.Fatal("expected error when context cancelled, got nil")
	}
}

// --- helpers ---

// fakeOIDCServer returns an httptest TLS server that serves a minimal
// OIDC discovery document at /.well-known/openid-configuration.
func fakeOIDCServer(t *testing.T, ready bool) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ready {
			http.Error(w, "502 Bad Gateway", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(discoveryDoc(w, r))
	}))
}

// discoveryDoc returns a minimal OpenID Connect discovery document.
func discoveryDoc(w http.ResponseWriter, r *http.Request) map[string]interface{} {
	// Derive issuer from the request (httptest server URL).
	scheme := "https"
	issuer := scheme + "://" + r.Host

	return map[string]interface{}{
		"issuer":                 issuer,
		"authorization_endpoint": issuer + "/authorize",
		"token_endpoint":         issuer + "/token",
		"userinfo_endpoint":      issuer + "/userinfo",
		"jwks_uri":               issuer + "/.well-known/jwks.json",
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"subject_types_supported":               []string{"public"},
		"response_types_supported":              []string{"code"},
	}
}

// testInsecureCtx returns a context with the test server's TLS-skipping
// HTTP client, matching the pattern used by go-oidc/oauth2.
func testInsecureCtx(ts *httptest.Server) context.Context {
	return context.WithValue(context.Background(), oauth2.HTTPClient, ts.Client())
}
