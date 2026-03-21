package oidcauth

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Provider wraps the OIDC provider, OAuth2 config, and ID token verifier.
type Provider struct {
	config       Config
	oidc         *oidc.Provider
	oauth2       *oauth2.Config
	verifier     *oidc.IDTokenVerifier
	store        *sessionStore
	insecureHTTP *http.Client // non-nil when Insecure=true
}

// NewProvider initializes the OIDC provider using Pocket ID's discovery endpoint.
// It fetches /.well-known/openid-configuration to auto-configure all endpoints.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	cfg.applyDefaults()

	// When running against a self-signed cert (local dev), inject an
	// HTTP client that skips TLS verification into the context.
	// go-oidc and oauth2 both honour oauth2.HTTPClient.
	var insecureClient *http.Client
	if cfg.Insecure {
		insecureClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev-only
			},
		}
		ctx = context.WithValue(ctx, oauth2.HTTPClient, insecureClient)
	}

	// Discover OIDC endpoints from Pocket ID
	oidcProvider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidcauth: failed to discover provider at %s: %w", cfg.IssuerURL, err)
	}

	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       cfg.Scopes,
	}

	verifier := oidcProvider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	return &Provider{
		config:       cfg,
		oidc:         oidcProvider,
		oauth2:       oauth2Config,
		verifier:     verifier,
		store:        newSessionStore(),
		insecureHTTP: insecureClient,
	}, nil
}

// authCodeURL generates the authorization URL with state, nonce, and optional PKCE.
func (p *Provider) authCodeURL(state, nonce, codeVerifier string) string {
	opts := []oauth2.AuthCodeOption{
		oidc.Nonce(nonce),
	}
	if p.config.EnablePKCE {
		opts = append(opts, oauth2.S256ChallengeOption(codeVerifier))
	}
	return p.oauth2.AuthCodeURL(state, opts...)
}

// exchange trades the authorization code for tokens, with optional PKCE verifier.
func (p *Provider) exchange(ctx context.Context, code, codeVerifier string) (*oauth2.Token, error) {
	opts := []oauth2.AuthCodeOption{}
	if p.config.EnablePKCE {
		opts = append(opts, oauth2.VerifierOption(codeVerifier))
	}
	return p.oauth2.Exchange(p.oidcCtx(ctx), code, opts...)
}

// verifyIDToken verifies and parses the ID token.
func (p *Provider) verifyIDToken(ctx context.Context, rawIDToken string) (*oidc.IDToken, error) {
	return p.verifier.Verify(p.oidcCtx(ctx), rawIDToken)
}

// userInfo fetches claims from the Pocket ID userinfo endpoint.
func (p *Provider) userInfo(ctx context.Context, accessToken string) (*oidc.UserInfo, error) {
	return p.oidc.UserInfo(p.oidcCtx(ctx), oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	}))
}

// oidcCtx injects the TLS-insecure HTTP client into ctx when Insecure mode
// is enabled, so that oauth2 token exchanges use the same transport.
func (p *Provider) oidcCtx(ctx context.Context) context.Context {
	if p.insecureHTTP != nil {
		return context.WithValue(ctx, oauth2.HTTPClient, p.insecureHTTP)
	}
	return ctx
}
