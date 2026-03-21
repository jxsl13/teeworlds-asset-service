package oidcauth

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// LoginHandler initiates the OIDC Authorization Code + PKCE flow.
func (p *Provider) LoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := generateRandom(32)
		nonce := generateRandom(32)
		codeVerifier := generateRandom(64)

		// Create a server-side session to store flow state
		sessionID, sess := p.store.create()
		sess.State = state
		sess.Nonce = nonce
		sess.CodeVerifier = codeVerifier

		// Allow callers to specify where to redirect after login.
		// Only accept relative paths to prevent open redirect.
		if returnTo := r.URL.Query().Get("return_to"); returnTo != "" {
			if strings.HasPrefix(returnTo, "/") && !strings.HasPrefix(returnTo, "//") {
				sess.ReturnTo = returnTo
			}
		}

		p.store.set(sessionID, sess)
		setSessionCookie(w, p.config.SessionCookieName, sessionID, 5*time.Minute, p.config.CookieSecure, p.config.CookieDomain)

		authURL := p.authCodeURL(state, nonce, codeVerifier)
		http.Redirect(w, r, authURL, http.StatusFound)
	}
}

// CallbackHandler handles the redirect from Pocket ID after user authentication.
func (p *Provider) CallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Retrieve the session
		cookie, err := r.Cookie(p.config.SessionCookieName)
		if err != nil {
			http.Error(w, "Missing session", http.StatusBadRequest)
			return
		}
		sess, ok := p.store.get(cookie.Value)
		if !ok {
			http.Error(w, "Invalid session", http.StatusBadRequest)
			return
		}

		// Verify state (CSRF protection)
		if r.URL.Query().Get("state") != sess.State {
			http.Error(w, "Invalid state parameter", http.StatusBadRequest)
			return
		}

		// Check for errors from the provider
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			desc := r.URL.Query().Get("error_description")
			http.Error(w, fmt.Sprintf("Authentication error: %s — %s", errParam, desc), http.StatusBadRequest)
			return
		}

		// Exchange authorization code for tokens (with PKCE verifier)
		code := r.URL.Query().Get("code")
		token, err := p.exchange(ctx, code, sess.CodeVerifier)
		if err != nil {
			slog.Error("Token exchange failed", "error", err)
			http.Error(w, "Token exchange failed", http.StatusInternalServerError)
			return
		}

		// Extract and verify the ID token
		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok {
			http.Error(w, "No id_token in response", http.StatusInternalServerError)
			return
		}

		idToken, err := p.verifyIDToken(ctx, rawIDToken)
		if err != nil {
			slog.Error("ID token verification failed", "error", err)
			http.Error(w, "ID token verification failed", http.StatusInternalServerError)
			return
		}

		// Verify nonce
		if idToken.Nonce != sess.Nonce {
			http.Error(w, "Invalid nonce", http.StatusBadRequest)
			return
		}

		// Extract claims
		var claims Claims
		if err := idToken.Claims(&claims); err != nil {
			slog.Error("Failed to parse claims", "error", err)
			http.Error(w, "Failed to parse user claims", http.StatusInternalServerError)
			return
		}

		// Promote to authenticated session
		returnTo := sess.ReturnTo
		sess.State = ""
		sess.Nonce = ""
		sess.CodeVerifier = ""
		sess.ReturnTo = ""
		sess.Claims = &claims
		sess.AccessToken = token.AccessToken
		sess.RefreshToken = token.RefreshToken
		sess.IDToken = rawIDToken
		if token.Expiry.After(time.Now()) {
			sess.ExpiresAt = token.Expiry.Unix()
		}

		p.store.set(cookie.Value, sess)
		setSessionCookie(w, p.config.SessionCookieName, cookie.Value, p.config.SessionMaxAge, p.config.CookieSecure, p.config.CookieDomain)

		// Redirect to the original page or home.
		// Validate to prevent open redirects (defence-in-depth).
		if returnTo == "" || !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
			returnTo = "/"
		}
		http.Redirect(w, r, returnTo, http.StatusFound)
	}
}

// LogoutHandler clears the local session and redirects back to the page
// the user was on. This is a local-only logout — it does not redirect to
// the identity provider. The user keeps their Pocket-ID session but is
// logged out of this application.
func (p *Provider) LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Destroy local session.
		if cookie, err := r.Cookie(p.config.SessionCookieName); err == nil {
			p.store.delete(cookie.Value)
		}
		clearSessionCookie(w, p.config.SessionCookieName, p.config.CookieDomain)

		// Redirect back to the page the user came from (default: /).
		target := "/"
		if returnTo := r.URL.Query().Get("return_to"); returnTo != "" {
			// Only accept relative paths to prevent open redirect.
			if strings.HasPrefix(returnTo, "/") && !strings.HasPrefix(returnTo, "//") {
				target = returnTo
			}
		}
		http.Redirect(w, r, target, http.StatusFound)
	}
}

// RegisterHandlers registers the login, callback, and logout handlers on a given mux.
func (p *Provider) RegisterHandlers(mux *http.ServeMux) {
	mux.Handle(p.config.LoginPath, p.LoginHandler())
	mux.Handle(p.config.CallbackPath, p.CallbackHandler())
	mux.Handle(p.config.LogoutPath, p.LogoutHandler())
}
