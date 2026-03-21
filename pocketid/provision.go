// Package pocketid provisions a Pocket-ID instance via its admin REST API.
//
// It creates the OIDC client, client secret, admin user group, admin user,
// assigns the user to the group, and generates a one-time access token so
// the admin can register a passkey on first login.
//
// Credentials (client_id + client_secret) are encrypted with AES-256-GCM
// and persisted in the kv_store table. On subsequent starts the stored
// credentials are reused — a new secret is only generated when no valid
// credentials exist in the database.
package pocketid

import (
	"bytes"
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	kvKeyOIDCCredentials = "oidc_credentials"
)

// storedCredentials is the JSON structure persisted (encrypted) in kv_store.
type storedCredentials struct {
	ClientID string `json:"client_id"`
	Secret   string `json:"secret"`
}

// KVStore is the subset of the sqlc Queries interface needed by this package.
type KVStore interface {
	GetKV(ctx context.Context, key string) (string, error)
	UpsertKV(ctx context.Context, arg interface {
		GetKey() string
		GetValue() string
	}) error
}

// Config holds the parameters needed to provision a Pocket-ID instance.
type Config struct {
	// BaseURL is the Pocket-ID application URL (e.g. "http://localhost:1411").
	BaseURL string

	// StaticAPIKey is the STATIC_API_KEY configured on the Pocket-ID instance.
	StaticAPIKey string

	// ClientName is the display name for the OIDC client.
	ClientName string

	// CallbackURLs are the allowed OIDC redirect URIs.
	CallbackURLs []string

	// LogoutCallbackURLs are the allowed post-logout redirect URIs.
	LogoutCallbackURLs []string

	// AdminEmail is the email for the initial admin user.
	AdminEmail string

	// AdminGroupName is the name of the admin user group (e.g. "admin").
	AdminGroupName string

	// EncryptionKey is the passphrase used to encrypt/decrypt stored credentials.
	EncryptionKey string

	// DB is the database connection used to persist encrypted credentials.
	DB *sql.DB

	// Insecure skips TLS certificate verification (for self-signed dev certs).
	Insecure bool
}

// Result contains the provisioned OIDC credentials and one-time login token.
type Result struct {
	ClientID    string
	Secret      string
	LoginURL    string // Full URL to log in and register a passkey (empty on reuse).
	AdminUserID string
}

// Provision provisions Pocket-ID for first use. It is idempotent:
//
//  1. Try to load encrypted credentials from the database.
//  2. If found, decrypt and verify them against Pocket-ID, then return.
//  3. If not found, create the OIDC client + secret, encrypt, and store them.
//  4. In both cases, ensure the admin group, admin user, and group assignment exist.
func Provision(ctx context.Context, cfg Config) (*Result, error) {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	if cfg.Insecure {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // dev-only
		}
	}

	c := &apiClient{
		baseURL: cfg.BaseURL,
		apiKey:  cfg.StaticAPIKey,
		http:    httpClient,
	}

	// Wait for Pocket-ID to become ready.
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}

	// ── Try stored credentials first ────────────────────────────────────────
	creds, err := loadCredentials(ctx, cfg.DB, cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	var loginURL string

	if creds != nil {
		// Verify the client still exists on Pocket-ID.
		if c.clientExists(ctx, creds.ClientID) {
			slog.Info("pocket-id: reusing stored credentials", "client_id", creds.ClientID)
		} else {
			// Client was deleted in Pocket-ID — reprovision.
			slog.Warn("pocket-id: stored client no longer exists, reprovisioning")
			creds = nil
		}
	}

	if creds == nil {
		// Full provisioning path.
		clientID, err := c.ensureOIDCClient(ctx, cfg.ClientName, cfg.CallbackURLs, cfg.LogoutCallbackURLs)
		if err != nil {
			return nil, fmt.Errorf("ensure oidc client: %w", err)
		}
		slog.Info("pocket-id: OIDC client ready", "id", clientID)

		secret, err := c.createClientSecret(ctx, clientID)
		if err != nil {
			return nil, fmt.Errorf("create client secret: %w", err)
		}
		slog.Info("pocket-id: client secret generated")

		creds = &storedCredentials{ClientID: clientID, Secret: secret}
		if err := saveCredentials(ctx, cfg.DB, cfg.EncryptionKey, creds); err != nil {
			return nil, fmt.Errorf("save credentials: %w", err)
		}
	}

	// ── Ensure supporting resources (always, for idempotency) ───────────────

	groupID, err := c.ensureUserGroup(ctx, cfg.AdminGroupName)
	if err != nil {
		return nil, fmt.Errorf("ensure admin group: %w", err)
	}
	slog.Info("pocket-id: admin group ready", "id", groupID)

	userID, err := c.ensureAdminUser(ctx, cfg.AdminEmail)
	if err != nil {
		return nil, fmt.Errorf("ensure admin user: %w", err)
	}
	slog.Info("pocket-id: admin user ready", "id", userID)

	if err := c.assignUserToGroup(ctx, groupID, userID); err != nil {
		return nil, fmt.Errorf("assign user to group: %w", err)
	}

	// Ensure the OIDC client has the admin group as an allowed user group,
	// otherwise Pocket-ID won't include "groups" in the ID token.
	if err := c.assignGroupToClient(ctx, creds.ClientID, groupID); err != nil {
		return nil, fmt.Errorf("assign group to client: %w", err)
	}

	// Generate a one-time login token (cheap, always regenerated).
	token, err := c.createOneTimeAccessToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("create one-time access token: %w", err)
	}
	loginURL = fmt.Sprintf("%s/lc/%s", cfg.BaseURL, token)
	slog.Info("pocket-id: provisioning complete", "login_url", loginURL)

	return &Result{
		ClientID:    creds.ClientID,
		Secret:      creds.Secret,
		LoginURL:    loginURL,
		AdminUserID: userID,
	}, nil
}

// ── Credential persistence ──────────────────────────────────────────────────

func loadCredentials(ctx context.Context, db *sql.DB, encKey string) (*storedCredentials, error) {
	var encrypted string
	err := db.QueryRowContext(ctx, "SELECT value FROM kv_store WHERE key = $1", kvKeyOIDCCredentials).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	plain, err := decrypt(encrypted, encKey)
	if err != nil {
		// Key mismatch or corrupted data — treat as missing so we reprovision.
		slog.Warn("pocket-id: failed to decrypt stored credentials, will reprovision", "err", err)
		return nil, nil
	}

	var creds storedCredentials
	if err := json.Unmarshal([]byte(plain), &creds); err != nil {
		slog.Warn("pocket-id: failed to parse stored credentials, will reprovision", "err", err)
		return nil, nil
	}
	return &creds, nil
}

func saveCredentials(ctx context.Context, db *sql.DB, encKey string, creds *storedCredentials) error {
	plain, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	encrypted, err := encrypt(string(plain), encKey)
	if err != nil {
		return err
	}

	_, err = db.ExecContext(ctx,
		"INSERT INTO kv_store (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value",
		kvKeyOIDCCredentials, encrypted)
	return err
}

// ── HTTP API client ─────────────────────────────────────────────────────────

type apiClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func (c *apiClient) waitReady(ctx context.Context) error {
	slog.Info("pocket-id: waiting for readiness", "url", c.baseURL)
	deadline := time.After(60 * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("pocket-id not reachable at %s after 60s", c.baseURL)
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/.well-known/openid-configuration", nil)
			resp, err := c.http.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					slog.Info("pocket-id: ready")
					return nil
				}
			}
		}
	}
}

// ── OIDC clients ────────────────────────────────────────────────────────────

type oidcClient struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type paginatedClients struct {
	Data []oidcClient `json:"data"`
}

func (c *apiClient) clientExists(ctx context.Context, clientID string) bool {
	var result oidcClient
	err := c.get(ctx, "/api/oidc/clients/"+clientID, &result)
	return err == nil && result.ID == clientID
}

func (c *apiClient) ensureOIDCClient(ctx context.Context, name string, callbackURLs, logoutURLs []string) (string, error) {
	var list paginatedClients
	if err := c.get(ctx, "/api/oidc/clients", &list); err != nil {
		return "", err
	}

	for _, cl := range list.Data {
		if cl.Name == name {
			return cl.ID, nil
		}
	}

	body := map[string]any{
		"name":               name,
		"callbackURLs":       callbackURLs,
		"logoutCallbackURLs": logoutURLs,
		"isPublic":           false,
		"pkceEnabled":        true,
	}

	var created oidcClient
	if err := c.post(ctx, "/api/oidc/clients", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (c *apiClient) createClientSecret(ctx context.Context, clientID string) (string, error) {
	var result struct {
		Secret string `json:"secret"`
	}
	if err := c.post(ctx, "/api/oidc/clients/"+clientID+"/secret", nil, &result); err != nil {
		return "", err
	}
	return result.Secret, nil
}

// ── User groups ─────────────────────────────────────────────────────────────

type userGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type paginatedGroups struct {
	Data []userGroup `json:"data"`
}

func (c *apiClient) ensureUserGroup(ctx context.Context, name string) (string, error) {
	var list paginatedGroups
	if err := c.get(ctx, "/api/user-groups", &list); err != nil {
		return "", err
	}

	for _, g := range list.Data {
		if g.Name == name {
			return g.ID, nil
		}
	}

	body := map[string]any{
		"name":         name,
		"friendlyName": name,
	}

	var created userGroup
	if err := c.post(ctx, "/api/user-groups", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (c *apiClient) assignUserToGroup(ctx context.Context, groupID, userID string) error {
	body := map[string]any{
		"userIds": []string{userID},
	}
	return c.put(ctx, "/api/user-groups/"+groupID+"/users", body)
}

func (c *apiClient) assignGroupToClient(ctx context.Context, clientID, groupID string) error {
	body := map[string]any{
		"userGroupIds": []string{groupID},
	}
	return c.put(ctx, "/api/oidc/clients/"+clientID+"/allowed-user-groups", body)
}

// ── Users ───────────────────────────────────────────────────────────────────

const staticAPIUserID = "00000000-0000-0000-0000-000000000000"

type user struct {
	ID string `json:"id"`
}

type paginatedUsers struct {
	Data []user `json:"data"`
}

func (c *apiClient) ensureAdminUser(ctx context.Context, email string) (string, error) {
	var list paginatedUsers
	if err := c.get(ctx, "/api/users", &list); err != nil {
		return "", err
	}

	for _, u := range list.Data {
		if u.ID != staticAPIUserID {
			return u.ID, nil
		}
	}

	body := map[string]any{
		"email":     email,
		"username":  "admin",
		"firstName": "Admin",
		"lastName":  "User",
		"isAdmin":   true,
	}

	var created user
	if err := c.post(ctx, "/api/users", body, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}

func (c *apiClient) createOneTimeAccessToken(ctx context.Context, userID string) (string, error) {
	var result struct {
		Token string `json:"token"`
	}
	if err := c.post(ctx, "/api/users/"+userID+"/one-time-access-token", map[string]any{}, &result); err != nil {
		return "", err
	}
	return result.Token, nil
}

// ── HTTP helpers ────────────────────────────────────────────────────────────

type apiError struct {
	Error string `json:"error"`
}

func (c *apiClient) do(ctx context.Context, method, path string, reqBody any, respBody any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		buf, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var ae apiError
		if json.Unmarshal(data, &ae) == nil && ae.Error != "" {
			return fmt.Errorf("pocket-id %s %s: %s (HTTP %d)", method, path, ae.Error, resp.StatusCode)
		}
		return fmt.Errorf("pocket-id %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(data))
	}

	if respBody != nil && len(data) > 0 {
		if err := json.Unmarshal(data, respBody); err != nil {
			return fmt.Errorf("unmarshal response: %w", err)
		}
	}
	return nil
}

func (c *apiClient) get(ctx context.Context, path string, result any) error {
	return c.do(ctx, http.MethodGet, path, nil, result)
}

func (c *apiClient) post(ctx context.Context, path string, body any, result any) error {
	return c.do(ctx, http.MethodPost, path, body, result)
}

func (c *apiClient) put(ctx context.Context, path string, body any) error {
	return c.do(ctx, http.MethodPut, path, body, nil)
}
