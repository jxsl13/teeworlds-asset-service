package oidcauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

// testProvider returns a minimal Provider suitable for handler tests that
// don't require a real OIDC identity provider (e.g. login/logout redirect tests).
func testProvider(t *testing.T) *Provider {
	t.Helper()
	cfg := DefaultConfig()
	cfg.CookieSecure = false
	return &Provider{
		config: cfg,
		store:  newSessionStore(),
		oauth2: &oauth2.Config{
			ClientID: "test",
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://idp.example.com/authorize",
				TokenURL: "https://idp.example.com/token",
			},
		},
	}
}

func TestLoginHandler_ReturnTo(t *testing.T) {
	tests := []struct {
		name         string
		returnTo     string
		wantStored   bool   // whether returnTo should be stored in session
		wantReturnTo string // expected value if stored
	}{
		{"relative path", "/skin", true, "/skin"},
		{"relative path with query", "/skin?limit=20&offset=0&sort=name:asc", true, "/skin?limit=20&offset=0&sort=name:asc"},
		{"root path", "/", true, "/"},
		{"no return_to", "", false, ""},
		{"absolute URL rejected", "https://evil.com", false, ""},
		{"protocol-relative rejected", "//evil.com", false, ""},
		{"protocol-relative with path rejected", "//evil.com/foo", false, ""},
		{"bare domain rejected", "evil.com", false, ""},
		{"backslash bypass rejected", "/\\evil.com", false, ""},
		{"path with fragment", "/skin#section", true, "/skin#section"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProvider(t)
			handler := p.LoginHandler()

			target := "/auth/login"
			if tt.returnTo != "" {
				target += "?return_to=" + url.QueryEscape(tt.returnTo)
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			// LoginHandler should redirect to the IdP.
			if rec.Code != http.StatusFound {
				t.Fatalf("expected 302, got %d", rec.Code)
			}

			// Find the session that was just created.
			var sess *sessionData
			for _, s := range p.store.sessions {
				sess = s
				break
			}
			if sess == nil {
				t.Fatal("no session created")
			}

			if tt.wantStored {
				if sess.ReturnTo != tt.wantReturnTo {
					t.Errorf("ReturnTo = %q, want %q", sess.ReturnTo, tt.wantReturnTo)
				}
			} else {
				if sess.ReturnTo != "" {
					t.Errorf("ReturnTo = %q, want empty (should be rejected)", sess.ReturnTo)
				}
			}
		})
	}
}

func TestIsSafeReturnURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/skin", true},
		{"/skin?limit=20&sort=name:asc", true},
		{"/", true},
		{"/skin#section", true},
		{"", false},
		{"https://evil.com", false},
		{"http://evil.com/path", false},
		{"//evil.com", false},
		{"//evil.com/path", false},
		{"/\\evil.com", false},
		{"evil.com", false},
		{"javascript:alert(1)", false},
		{"data:text/html,<h1>hi</h1>", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isSafeReturnURL(tt.input); got != tt.want {
				t.Errorf("isSafeReturnURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLogoutHandler_ReturnTo(t *testing.T) {
	tests := []struct {
		name     string
		returnTo string
		wantURL  string
	}{
		{"relative path", "/skin", "/skin"},
		{"relative path with query", "/skin?sort=name:asc", "/skin?sort=name:asc"},
		{"no return_to", "", "/"},
		{"absolute URL rejected", "https://evil.com", "/"},
		{"protocol-relative rejected", "//evil.com", "/"},
		{"protocol-relative with path rejected", "//evil.com/foo", "/"},
		{"bare domain rejected", "evil.com", "/"},
		{"backslash bypass rejected", "/\\evil.com", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testProvider(t)
			handler := p.LogoutHandler()

			target := "/auth/logout"
			if tt.returnTo != "" {
				target += "?return_to=" + url.QueryEscape(tt.returnTo)
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusFound {
				t.Fatalf("expected 302, got %d", rec.Code)
			}

			got := rec.Header().Get("Location")
			if got != tt.wantURL {
				t.Errorf("Location = %q, want %q", got, tt.wantURL)
			}
		})
	}
}
