package seedutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ── rate limiting ────────────────────────────────────────────────────────────

// IsLocalhost reports whether the given base URL points to a loopback address.
func IsLocalhost(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// Throttle is a context-aware rate limiter that allows at most N operations per
// second.  Multiple goroutines may call Wait concurrently; the ticker channel
// serialises them.
type Throttle struct {
	ticker *time.Ticker
}

// NewThrottle creates a rate limiter allowing rps requests per second.
// If rps <= 0 it returns a no-op throttle that never blocks.
func NewThrottle(rps int) *Throttle {
	if rps <= 0 {
		return &Throttle{}
	}
	return &Throttle{
		ticker: time.NewTicker(time.Second / time.Duration(rps)),
	}
}

// Wait blocks until the next request slot is available or the context is
// cancelled.  It returns the context error when cancelled.
func (t *Throttle) Wait(ctx context.Context) error {
	if t.ticker == nil {
		return ctx.Err()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.ticker.C:
		return nil
	}
}

// Stop releases resources held by the throttle.
func (t *Throttle) Stop() {
	if t.ticker != nil {
		t.ticker.Stop()
	}
}

const userAgent = "teeworlds-asset-db-seeder/1.0"

// ── HTTP helpers ─────────────────────────────────────────────────────────────

// HTTPGet performs an HTTP GET with a reasonable timeout and user-agent.
func HTTPGet(rawURL string) (*http.Response, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return client.Do(req)
}

// FetchBytes downloads a URL and returns the response body, limited to maxBytes.
func FetchBytes(rawURL string, maxBytes int64) ([]byte, error) {
	resp, err := HTTPGet(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return data, nil
}

// FetchText downloads a URL and returns the response body as a string (max 1 MiB).
func FetchText(rawURL string) (string, error) {
	data, err := FetchBytes(rawURL, 1<<20)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ── upload client ────────────────────────────────────────────────────────────

// NewUploadClient creates an HTTP client with a cookie jar for
// communicating with the target asset server (CSRF cookie handling).
func NewUploadClient() *http.Client {
	jar, _ := cookiejar.New(nil) //nolint:errcheck
	return &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second,
	}
}

// FetchCSRFToken performs a GET against the server to obtain the __csrf
// cookie.  The cookie is stored in the client's jar for subsequent
// requests; the token value is returned for the X-CSRF-Token header.
func FetchCSRFToken(client *http.Client, baseURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/", nil)
	if err != nil {
		return "", fmt.Errorf("create CSRF request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("CSRF GET %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}

	for _, c := range client.Jar.Cookies(parsed) {
		if c.Name == "__csrf" {
			return c.Value, nil
		}
	}
	return "", fmt.Errorf("no __csrf cookie received from %s", baseURL)
}

// ── multipart upload ─────────────────────────────────────────────────────────

// UploadAsset uploads a single asset to the server via multipart POST.
// assetType is the URL path segment (e.g. "skin", "map").
// license is the SPDX-like license identifier (e.g. "cc0", "unknown").
func UploadAsset(client *http.Client, csrfToken, baseURL, assetType, name, license string, creators []string, filename string, data []byte) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Part 1: metadata JSON (must be first).
	metaHeader := make(map[string][]string)
	metaHeader["Content-Disposition"] = []string{`form-data; name="metadata"; filename="metadata.json"`}
	metaHeader["Content-Type"] = []string{"application/json"}
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return fmt.Errorf("create metadata part: %w", err)
	}
	meta := map[string]any{
		"name":     name,
		"license":  license,
		"creators": creators,
	}
	if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	// Part 2: file (must be second).
	fileHeader := make(map[string][]string)
	fileHeader["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)}
	fileHeader["Content-Type"] = []string{"application/octet-stream"}
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return fmt.Errorf("create file part: %w", err)
	}
	if _, err := filePart.Write(data); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	writer.Close()

	uploadURL := fmt.Sprintf("%s/api/upload/%s", baseURL, assetType)
	req, err := http.NewRequest(http.MethodPost, uploadURL, &body)
	if err != nil {
		return fmt.Errorf("create POST request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", uploadURL, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusConflict:
		// Already exists — not an error for seeding.
		return nil
	default:
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
}

// ── creator parsing ──────────────────────────────────────────────────────────

// reParenAttrib matches parenthetical attributions like
// "(toast skin by DianChi)" or "(source from Tater)".
var reParenAttrib = regexp.MustCompile(`\((?:[^)]*?\b(?:by|from)\s+)([^)]+)\)`)

// ParseCreators splits a creator string into a list.
// DDNet uses various separators and attribution patterns:
//   - comma: "A, B"
//   - ampersand: "A & B", "A&B"
//   - plus: "A + B"
//   - and: "A and B"
//   - feat: "A .feat B"
//   - "Hat by" / "Skin by" attribution: "A Hat by B"
//   - parenthetical: "A (skin by B)", "A (source from B)"
func ParseCreators(creator string) []string {
	if strings.TrimSpace(creator) == "" {
		return []string{"Unknown"}
	}

	s := creator

	// Extract names from parenthetical attributions and flatten.
	s = reParenAttrib.ReplaceAllString(s, ", $1")

	// Handle "Hat by" / "Skin by" mid-string attributions.
	for _, sep := range []string{" Hat by ", " hat by ", " Skin by ", " skin by "} {
		s = strings.ReplaceAll(s, sep, ",")
	}

	// Handle ".feat" separator.
	s = strings.ReplaceAll(s, ".feat ", ",")

	// Normalize separators.
	s = strings.ReplaceAll(s, " & ", ",")
	s = strings.ReplaceAll(s, "&", ",")
	s = strings.ReplaceAll(s, " + ", ",")
	s = strings.ReplaceAll(s, " and ", ",")

	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		trimmed = strings.TrimRight(trimmed, ".,;:")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{"Unknown"}
	}
	return result
}

// ── license mapping ──────────────────────────────────────────────────────────

// MapLicense converts a DDNet license string to an asset-service license enum value.
func MapLicense(ddnetLicense string) string {
	normalized := strings.TrimSpace(strings.ToLower(ddnetLicense))
	switch {
	case normalized == "cc0":
		return "cc0"
	case normalized == "cc by" || normalized == "cc-by":
		return "cc-by"
	case normalized == "cc by-sa" || normalized == "cc-by-sa":
		return "cc-by-sa"
	case normalized == "cc by-nd" || normalized == "cc-by-nd":
		return "cc-by-nd"
	case normalized == "cc by-nc" || normalized == "cc-by-nc":
		return "cc-by-nc"
	case normalized == "cc by-nc-sa" || normalized == "cc-by-nc-sa":
		return "cc-by-nc-sa"
	case normalized == "cc by-nc-nd" || normalized == "cc-by-nc-nd":
		return "cc-by-nc-nd"
	case normalized == "gpl-2" || normalized == "gpl2":
		return "gpl-2"
	case normalized == "gpl-3" || normalized == "gpl3":
		return "gpl-3"
	case normalized == "mit":
		return "mit"
	case normalized == "apache-2" || normalized == "apache 2.0" || normalized == "apache-2.0":
		return "apache-2"
	case normalized == "zlib":
		return "zlib"
	case normalized == "unknown" || normalized == "":
		return "unknown"
	default:
		return "custom"
	}
}
