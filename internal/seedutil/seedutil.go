package seedutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ── HTTP error type ──────────────────────────────────────────────────────────

// HTTPStatusError is returned by HTTP helpers when the server responds with an
// unexpected status code.  Using a typed error allows callers to inspect the
// code without parsing the error string.
type HTTPStatusError struct {
	StatusCode int
	Body       string // optional response body snippet
}

func (e *HTTPStatusError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

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

// Retry calls fn in a loop, retrying with exponential backoff whenever fn
// returns a context.DeadlineExceeded error (HTTP client timeout) or an
// HTTPStatusError whose status code is in the given retryable codes list.
// The delay starts at 5 s, doubles on each attempt, and is capped at 60 s;
// a random jitter of up to half the current delay is added each time.
// The loop stops immediately on any other error, on success, or when the
// parent ctx is cancelled.
func Retry(ctx context.Context, fn func() error, codes ...int) error {
	const (
		minDelay = 5 * time.Second
		maxDelay = 60 * time.Second
	)
	delay := minDelay
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := fn()
		if err == nil {
			return nil
		}
		retryable := false
		if errors.Is(err, context.DeadlineExceeded) {
			retryable = true
		} else {
			var httpErr *HTTPStatusError
			if errors.As(err, &httpErr) {
				if slices.Contains(codes, httpErr.StatusCode) {
					retryable = true
				}
			}
		}
		if !retryable {
			return err
		}
		// Exponential backoff with jitter: sleep in [delay/2, delay].
		jitter := time.Duration(rand.Int64N(int64(delay)/2 + 1))
		sleep := delay/2 + jitter
		if sleep > maxDelay {
			sleep = maxDelay
		}
		log.Printf("retryable error (%v) — retrying in %s …", err, sleep.Round(time.Millisecond))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
		delay = min(delay*2, maxDelay)
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
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode}
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

	if resp.StatusCode == http.StatusCreated {
		return nil
	}
	return &HTTPStatusError{StatusCode: resp.StatusCode, Body: string(respBody)}
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
	s = strings.ReplaceAll(s, "! ", ",")
	s = strings.ReplaceAll(s, "; ", ",")

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

// licensePattern pairs a regex with the canonical license enum value.
type licensePattern struct {
	re    *regexp.Regexp
	value string
}

// licensePatterns is evaluated in order; more specific patterns come first.
var licensePatterns = []licensePattern{
	{regexp.MustCompile(`\bcc\b.*\bby\b.*\bnc\b.*\bsa\b`), "cc-by-nc-sa"},
	{regexp.MustCompile(`\bcc\b.*\bby\b.*\bnc\b.*\bnd\b`), "cc-by-nc-nd"},
	{regexp.MustCompile(`\bcc\b.*\bby\b.*\bnc\b`), "cc-by-nc"},
	{regexp.MustCompile(`\bcc\b.*\bby\b.*\bsa\b`), "cc-by-sa"},
	{regexp.MustCompile(`\bcc\b.*\bby\b.*\bnd\b`), "cc-by-nd"},
	{regexp.MustCompile(`\bcc\b.*\bby\b`), "cc-by"},
	{regexp.MustCompile(`\bcc\s*0\b|\bcc\b.*\bzero\b|\bpublic\s*domain\b`), "cc0"},
	{regexp.MustCompile(`\bgpl\b.*\b3\b`), "gpl-3"},
	{regexp.MustCompile(`\bgpl\b.*\b2\b`), "gpl-2"},
	{regexp.MustCompile(`\bgpl\b`), "gpl-3"},
	{regexp.MustCompile(`\bmit\b`), "mit"},
	{regexp.MustCompile(`\bapache\b`), "apache-2"},
	{regexp.MustCompile(`\bzlib\b`), "zlib"},
}

// MapLicense maps a free-form license string to the closest known license
// enum value.  It first tries exact matches against canonical values, then
// falls back to regex-based fuzzy matching.  Unrecognised strings map to
// "unknown".
func MapLicense(raw string) string {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	if normalized == "" {
		return "unknown"
	}

	// Exact match against known enum values.
	switch normalized {
	case "cc0", "cc-by", "cc-by-sa", "cc-by-nd", "cc-by-nc",
		"cc-by-nc-sa", "cc-by-nc-nd", "gpl-2", "gpl-3",
		"mit", "apache-2", "zlib", "unknown":
		return normalized
	}

	// Fuzzy match: strip non-alphanumeric chars and match patterns.
	scrubbed := regexp.MustCompile(`[^a-z0-9 ]`).ReplaceAllString(normalized, " ")
	scrubbed = regexp.MustCompile(`\s+`).ReplaceAllString(scrubbed, " ")
	scrubbed = strings.TrimSpace(scrubbed)

	for _, p := range licensePatterns {
		if p.re.MatchString(scrubbed) {
			return p.value
		}
	}

	return "unknown"
}

// ── existing-asset lookup ────────────────────────────────────────────────────

// configResponse mirrors the JSON returned by GET /api/config.
type configResponse struct {
	ItemsPerPage int `json:"items_per_page"`
}

// FetchItemsPerPage fetches the configured maximum page size from the
// asset-service's /api/config endpoint.
func FetchItemsPerPage(baseURL string) (int, error) {
	resp, err := HTTPGet(baseURL + "/api/config")
	if err != nil {
		return 0, fmt.Errorf("fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("fetch config: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var cfg configResponse
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return 0, fmt.Errorf("decode config: %w", err)
	}
	if cfg.ItemsPerPage <= 0 {
		return 0, fmt.Errorf("server returned invalid items_per_page: %d", cfg.ItemsPerPage)
	}
	return cfg.ItemsPerPage, nil
}

// listResponse mirrors the JSON returned by GET /api/{asset_type}.
type listResponse struct {
	Results []struct {
		ItemValue map[string]interface{} `json:"item_value"`
	} `json:"results"`
	Total int `json:"total"`
}

// FetchExistingNames paginates through the list endpoint of the asset-service
// and returns a set of all existing item names for the given asset type.
// It queries /api/config to determine the server's page size limit, falling
// back to 100 if the endpoint is unavailable.
func FetchExistingNames(baseURL, assetType string) (map[string]struct{}, error) {
	pageSize, err := FetchItemsPerPage(baseURL)
	if err != nil {
		log.Printf("WARN  config endpoint unavailable, using default page size 100: %v", err)
		pageSize = 100
	}

	existing := make(map[string]struct{})
	offset := 0

	for {
		listURL := fmt.Sprintf("%s/api/%s?limit=%d&offset=%d", baseURL, url.PathEscape(assetType), pageSize, offset)
		resp, err := HTTPGet(listURL)
		if err != nil {
			return nil, fmt.Errorf("fetch existing %s names: %w", assetType, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return nil, fmt.Errorf("fetch existing %s names: HTTP %d: %s", assetType, resp.StatusCode, string(body))
		}

		var page listResponse
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return nil, fmt.Errorf("decode %s list response: %w", assetType, err)
		}

		for _, item := range page.Results {
			if name, ok := item.ItemValue["name"].(string); ok {
				existing[name] = struct{}{}
			}
		}

		offset += len(page.Results)
		if offset >= page.Total || len(page.Results) == 0 {
			break
		}
	}

	return existing, nil
}
