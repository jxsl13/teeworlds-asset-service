package oidcauth

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is an unexported type to avoid collisions in context values.
type contextKey string

const (
	claimsContextKey contextKey = "oidcauth.claims"
	tokenContextKey  contextKey = "oidcauth.access_token"
)

// ClaimsFromContext retrieves the authenticated user's claims from the request context.
// Returns nil if the user is not authenticated.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsContextKey).(*Claims)
	return claims
}

// AccessTokenFromContext retrieves the OAuth2 access token from the request context.
func AccessTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(tokenContextKey).(string)
	return token
}

// RequireAuth is middleware that rejects unauthenticated requests with 401.
// Authenticated user claims are stored in the request context.
//
// It supports two modes:
//  1. Session-based: looks up the server-side session from the session cookie.
//  2. Bearer token: validates a "Bearer <token>" Authorization header against the
//     Pocket ID userinfo endpoint (useful for API clients).
func (p *Provider) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try Bearer token first (for API clients)
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			accessToken := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := p.authenticateBearer(r.Context(), accessToken)
			if err != nil {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			ctx = context.WithValue(ctx, tokenContextKey, accessToken)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Fall back to session cookie
		cookie, err := r.Cookie(p.config.SessionCookieName)
		if err != nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		sess, ok := p.store.get(cookie.Value)
		if !ok || sess.Claims == nil {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), claimsContextKey, sess.Claims)
		ctx = context.WithValue(ctx, tokenContextKey, sess.AccessToken)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAuthFunc is a convenience wrapper that accepts an http.HandlerFunc.
func (p *Provider) RequireAuthFunc(next http.HandlerFunc) http.Handler {
	return p.RequireAuth(next)
}

// OptionalAuth is middleware that populates the context with user claims if present,
// but does NOT reject unauthenticated requests. Useful for endpoints that behave
// differently for logged-in vs. anonymous users.
func (p *Provider) OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try Bearer token first
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			accessToken := strings.TrimPrefix(authHeader, "Bearer ")
			if claims, err := p.authenticateBearer(r.Context(), accessToken); err == nil {
				ctx := context.WithValue(r.Context(), claimsContextKey, claims)
				ctx = context.WithValue(ctx, tokenContextKey, accessToken)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Try session cookie
		if cookie, err := r.Cookie(p.config.SessionCookieName); err == nil {
			if sess, ok := p.store.get(cookie.Value); ok && sess.Claims != nil {
				ctx := context.WithValue(r.Context(), claimsContextKey, sess.Claims)
				ctx = context.WithValue(ctx, tokenContextKey, sess.AccessToken)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Proceed without authentication context
		next.ServeHTTP(w, r)
	})
}

// RequireGroup is middleware that checks the user belongs to at least one of the
// specified Pocket ID groups. Must be used after RequireAuth.
func (p *Provider) RequireGroup(next http.Handler, groups ...string) http.Handler {
	return p.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil || !claims.HasAnyGroup(groups...) {
			http.Error(w, "Forbidden: insufficient group membership", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	}))
}

// RequireGroupFunc is a convenience wrapper for RequireGroup with an http.HandlerFunc.
func (p *Provider) RequireGroupFunc(next http.HandlerFunc, groups ...string) http.Handler {
	return p.RequireGroup(next, groups...)
}

// authenticateBearer validates a Bearer access token by calling the Pocket ID userinfo endpoint.
func (p *Provider) authenticateBearer(ctx context.Context, accessToken string) (*Claims, error) {
	userInfo, err := p.userInfo(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	var claims Claims
	if err := userInfo.Claims(&claims); err != nil {
		return nil, err
	}

	return &claims, nil
}
