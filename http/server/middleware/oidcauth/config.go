package oidcauth

import "time"

// Config holds all settings needed to connect to a Pocket ID OIDC provider.
type Config struct {
	// IssuerURL is the base URL of your Pocket ID instance (e.g., "https://id.example.com").
	// The OIDC discovery document is fetched from {IssuerURL}/.well-known/openid-configuration.
	IssuerURL string

	// ClientID is the OIDC client identifier created in the Pocket ID admin panel.
	ClientID string

	// ClientSecret is the OIDC client secret. Leave empty for public clients (PKCE-only).
	ClientSecret string

	// RedirectURL is the callback URL registered in Pocket ID (e.g., "https://myapp.com/auth/callback").
	RedirectURL string

	// PostLogoutRedirectURL is where users land after logging out. Optional.
	PostLogoutRedirectURL string

	// Scopes to request from Pocket ID. Defaults to ["openid", "profile", "email", "groups"].
	Scopes []string

	// SessionCookieName is the name of the cookie that stores the encrypted session.
	// Defaults to "oidc_session".
	SessionCookieName string

	// SessionMaxAge controls how long the session cookie lives. Defaults to 8 hours.
	SessionMaxAge time.Duration

	// CookieSecure sets the Secure flag on cookies. Should be true in production (HTTPS).
	CookieSecure bool

	// CookieDomain optionally restricts cookies to a specific domain.
	CookieDomain string

	// EnablePKCE enables Proof Key for Code Exchange (S256). Defaults to true.
	EnablePKCE bool

	// Insecure skips TLS certificate verification (for self-signed dev certs).
	Insecure bool

	// LoginPath is the path that initiates the OIDC flow. Defaults to "/auth/login".
	LoginPath string

	// CallbackPath is the path that handles the OIDC callback. Defaults to "/auth/callback".
	CallbackPath string

	// LogoutPath is the path that handles logout. Defaults to "/auth/logout".
	LogoutPath string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Scopes:            []string{"openid", "profile", "email", "groups"},
		SessionCookieName: "oidc_session",
		SessionMaxAge:     8 * time.Hour,
		CookieSecure:      true,
		EnablePKCE:        true,
		LoginPath:         "/auth/login",
		CallbackPath:      "/auth/callback",
		LogoutPath:        "/auth/logout",
	}
}

func (c *Config) applyDefaults() {
	defaults := DefaultConfig()
	if len(c.Scopes) == 0 {
		c.Scopes = defaults.Scopes
	}
	if c.SessionCookieName == "" {
		c.SessionCookieName = defaults.SessionCookieName
	}
	if c.SessionMaxAge == 0 {
		c.SessionMaxAge = defaults.SessionMaxAge
	}
	if c.LoginPath == "" {
		c.LoginPath = defaults.LoginPath
	}
	if c.CallbackPath == "" {
		c.CallbackPath = defaults.CallbackPath
	}
	if c.LogoutPath == "" {
		c.LogoutPath = defaults.LogoutPath
	}
}
