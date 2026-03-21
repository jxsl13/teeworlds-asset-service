package csrf

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/jxsl13/asset-service/http/server/middleware/clientip"
)

const (
	cookieName = "__csrf"
	headerName = "X-CSRF-Token"
	tokenBytes = 32
)

// Middleware implements the Double Submit Cookie pattern.
//
// On every response it ensures a __csrf cookie exists (JS-readable).
// For state-changing methods (POST, PUT, PATCH, DELETE) it requires an
// X-CSRF-Token header whose value matches the cookie.
//
// Requests authenticated via Bearer token or originating from loopback
// addresses are exempt.
func Middleware(secure bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ensureCookie(w, r, secure)

			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			// Bearer-token requests are not vulnerable to CSRF.
			if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			// Loopback addresses are exempt — enables CLI tools.
			if addr := clientip.FromContext(r.Context()); addr.IsValid() && addr.IsLoopback() {
				next.ServeHTTP(w, r)
				return
			}

			headerToken := r.Header.Get(headerName)
			if headerToken == "" || headerToken != token {
				http.Error(w, "CSRF token missing or invalid", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func ensureCookie(w http.ResponseWriter, r *http.Request, secure bool) string {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return c.Value
	}

	token := generateToken()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
	})
	return token
}

func generateToken() string {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		panic("csrf: crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
