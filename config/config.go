package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// Config holds all environment-driven configuration for the service.
type Config struct {
	// Database connection parts. Each maps to a required environment variable.
	// DB_HOST     – PostgreSQL host              (e.g. localhost)
	// DB_PORT     – PostgreSQL port              (default: 5432)
	// DB_USER     – PostgreSQL user
	// DB_PASSWORD – PostgreSQL password
	// DB_NAME     – PostgreSQL database name
	// DB_SSLMODE  – SSL mode                     (default: disable)
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	// DatabaseURL is assembled from the individual DB_* parts after validation.
	DatabaseURL string

	// Addr is the TCP address the HTTP server listens on.
	// Env: ADDR  (default: :8080)
	Addr string

	// StoragePath is the absolute base directory for all stored item files.
	// Env: STORAGE_PATH (required)
	StoragePath string

	// TempUploadPath is the directory used for temporary file storage during uploads.
	// Env: TEMP_UPLOAD_PATH (default: os.TempDir())
	TempUploadPath string

	// MaxStorageSize is the maximum total size in bytes for all stored items.
	// Env: MAX_STORAGE_SIZE (default: 1 GiB)
	MaxStorageSize int64

	// AllowedResolutions maps each item type to its permitted PNG resolutions.
	// Env per type: ALLOWED_RESOLUTIONS_GAMESKIN, ALLOWED_RESOLUTIONS_SKIN, etc.
	// Format: comma-separated WxH (e.g. "256x128,512x256").
	// "map" has no resolution constraint (not a PNG).
	AllowedResolutions map[string][]Resolution

	// MaxUploadSizes maps each item type to the maximum allowed upload size in bytes.
	// Env per type: MAX_UPLOAD_SIZE_MAP, MAX_UPLOAD_SIZE_SKIN, etc.
	// Accepts human-readable values like "10MiB", "2MB", "512KB".
	// Defaults are derived from the largest allowed resolution per type.
	MaxUploadSizes map[string]int64

	// ThumbnailSizes maps each item type to the bounding box (width × height)
	// used when generating thumbnails. Images exceeding the box are scaled down;
	// smaller images keep their original file as the thumbnail.
	//
	// Env per type: THUMBNAIL_SIZE_MAP, THUMBNAIL_SIZE_SKIN, etc. (format: WxH)
	//
	// Defaults:
	//   map  → 1920×1080
	//   skin → 64×64
	//   others → smallest allowed resolution
	ThumbnailSizes map[string]Resolution

	// OIDC / Pocket-ID configuration (required).
	// Provision credentials via cmd/provision-pocketid before first start.
	OIDCIssuerURL             string // Env: OIDC_ISSUER_URL
	OIDCClientID              string // Env: OIDC_CLIENT_ID
	OIDCClientSecret          string // Env: OIDC_CLIENT_SECRET
	OIDCRedirectURL           string // Env: OIDC_REDIRECT_URL
	OIDCPostLogoutRedirectURL string // Env: OIDC_POST_LOGOUT_REDIRECT_URL

	// Insecure disables secure cookies (HTTPS requirement) for OIDC sessions.
	// Env: INSECURE (default: false — set to "true" for local HTTP dev)
	Insecure bool

	// RateLimitMaxGroups is the maximum number of new asset groups a single IP
	// may create within RateLimitWindow. Set to 0 to disable rate limiting.
	// Env: RATE_LIMIT_MAX_GROUPS (default: 10)
	RateLimitMaxGroups int

	// RateLimitWindow is the sliding time window for the per-IP group creation
	// rate limit. Accepts Go duration strings (e.g. 24h, 1h30m10s).
	// Env: RATE_LIMIT_WINDOW (default: 24h)
	RateLimitWindow time.Duration

	// HTTPRateLimitRate is the steady-state request rate allowed per IP, in
	// requests per second. Set to 0 to disable HTTP-level rate limiting.
	// Env: HTTP_RATE_LIMIT_RATE (default: 20)
	HTTPRateLimitRate float64

	// HTTPRateLimitBurst is the maximum burst size for the per-IP token bucket.
	// Env: HTTP_RATE_LIMIT_BURST (default: 40)
	HTTPRateLimitBurst int

	// HTTPRateLimitCleanup is how long an idle IP entry is kept before eviction.
	// Env: HTTP_RATE_LIMIT_CLEANUP (default: 10m)
	HTTPRateLimitCleanup time.Duration

	// AdminOnlyUpload restricts asset uploads to authenticated admin users.
	// When true, only users in the "admin" group can upload; the upload
	// button is hidden for everyone else.
	// Env: ADMIN_ONLY_UPLOAD (default: false)
	AdminOnlyUpload bool

	// ItemsPerPage is the default number of items returned per page when the
	// client does not specify a limit. Also used as the default in UI views.
	// Env: ITEMS_PER_PAGE (default: 100, max: 1000)
	ItemsPerPage int

	// Branding controls the visual identity shown in the UI header.
	Branding Branding
}

// Branding holds the configurable UI header text and images.
type Branding struct {
	// SiteTitle is the page <title> and header heading.
	// Env: BRANDING_TITLE (default: "Teeworlds Asset Database")
	SiteTitle string

	// SiteSubtitle is the tagline shown below the header heading.
	// Env: BRANDING_SUBTITLE (default: "Community database for skins, maps, gameskins & more")
	SiteSubtitle string

	// HeaderImagePath is an optional local file path for a logo/image displayed in the header.
	// The file is served statically at /branding/header-image.
	// Env: BRANDING_HEADER_IMAGE_PATH (default: empty — no image shown)
	HeaderImagePath string

	// FaviconPath is an optional local file path for the browser tab icon.
	// The file is served statically at /branding/favicon.
	// Env: BRANDING_FAVICON_PATH (default: empty — browser default)
	FaviconPath string
}

// Load reads configuration from environment variables, validates required
// fields, and constructs the final DatabaseURL.
func Load() (Config, error) {
	cfg := Config{
		DBHost:         os.Getenv("DB_HOST"),
		DBPort:         os.Getenv("DB_PORT"),
		DBUser:         os.Getenv("DB_USER"),
		DBPassword:     os.Getenv("DB_PASSWORD"),
		DBName:         os.Getenv("DB_NAME"),
		DBSSLMode:      os.Getenv("DB_SSLMODE"),
		Addr:           os.Getenv("ADDR"),
		StoragePath:    os.Getenv("STORAGE_PATH"),
		TempUploadPath: os.Getenv("TEMP_UPLOAD_PATH"),

		OIDCIssuerURL:             os.Getenv("OIDC_ISSUER_URL"),
		OIDCClientID:              os.Getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret:          os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:           os.Getenv("OIDC_REDIRECT_URL"),
		OIDCPostLogoutRedirectURL: os.Getenv("OIDC_POST_LOGOUT_REDIRECT_URL"),
		Insecure:                  os.Getenv("INSECURE") == "true",
		AdminOnlyUpload:           os.Getenv("ADMIN_ONLY_UPLOAD") == "true",
	}

	var missing []string
	for _, kv := range []struct{ key, val string }{
		{"DB_HOST", cfg.DBHost},
		{"DB_USER", cfg.DBUser},
		{"DB_PASSWORD", cfg.DBPassword},
		{"DB_NAME", cfg.DBName},
		{"STORAGE_PATH", cfg.StoragePath},
		{"OIDC_ISSUER_URL", cfg.OIDCIssuerURL},
		{"OIDC_CLIENT_ID", cfg.OIDCClientID},
		{"OIDC_CLIENT_SECRET", cfg.OIDCClientSecret},
		{"OIDC_REDIRECT_URL", cfg.OIDCRedirectURL},
	} {
		if kv.val == "" {
			missing = append(missing, kv.key)
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("required environment variables not set: %s", strings.Join(missing, ", "))
	}

	if cfg.DBPort == "" {
		cfg.DBPort = "5432"
	}
	if cfg.DBSSLMode == "" {
		cfg.DBSSLMode = "disable"
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}
	if cfg.TempUploadPath == "" {
		cfg.TempUploadPath = os.TempDir()
	}

	// ── Per-IP group creation rate limit ──────────────────────────────────────
	cfg.RateLimitMaxGroups = 10
	if raw := os.Getenv("RATE_LIMIT_MAX_GROUPS"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("RATE_LIMIT_MAX_GROUPS: must be a non-negative integer, got %q", raw)
		}
		cfg.RateLimitMaxGroups = n
	}

	cfg.RateLimitWindow = 24 * time.Hour
	if raw := os.Getenv("RATE_LIMIT_WINDOW"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("RATE_LIMIT_WINDOW: must be a positive duration (e.g. 24h, 1h30m10s): %w", err)
		}
		cfg.RateLimitWindow = d
	}

	// ── HTTP-level per-IP request rate limit ─────────────────────────────
	cfg.HTTPRateLimitRate = 20
	if raw := os.Getenv("HTTP_RATE_LIMIT_RATE"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v < 0 {
			return Config{}, fmt.Errorf("HTTP_RATE_LIMIT_RATE: must be a non-negative number, got %q", raw)
		}
		cfg.HTTPRateLimitRate = v
	}

	cfg.HTTPRateLimitBurst = 40
	if raw := os.Getenv("HTTP_RATE_LIMIT_BURST"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return Config{}, fmt.Errorf("HTTP_RATE_LIMIT_BURST: must be a non-negative integer, got %q", raw)
		}
		cfg.HTTPRateLimitBurst = n
	}

	cfg.HTTPRateLimitCleanup = 10 * time.Minute
	if raw := os.Getenv("HTTP_RATE_LIMIT_CLEANUP"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("HTTP_RATE_LIMIT_CLEANUP: must be a positive duration, got %q", raw)
		}
		cfg.HTTPRateLimitCleanup = d
	}

	// ── Items per page ────────────────────────────────────────────────────
	cfg.ItemsPerPage = 100
	if raw := os.Getenv("ITEMS_PER_PAGE"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			return Config{}, fmt.Errorf("ITEMS_PER_PAGE: must be an integer between 1 and 1000, got %q", raw)
		}
		cfg.ItemsPerPage = n
	}

	// ── Branding ─────────────────────────────────────────────────────────
	cfg.Branding = Branding{
		SiteTitle:    "Teeworlds Asset Database",
		SiteSubtitle: "Community database for skins, maps, gameskins \u0026 more",
	}
	if v := os.Getenv("BRANDING_TITLE"); v != "" {
		cfg.Branding.SiteTitle = v
	}
	if v := os.Getenv("BRANDING_SUBTITLE"); v != "" {
		cfg.Branding.SiteSubtitle = v
	}
	if v := os.Getenv("BRANDING_HEADER_IMAGE_PATH"); v != "" {
		if _, err := os.Stat(v); err != nil {
			return Config{}, fmt.Errorf("BRANDING_HEADER_IMAGE_PATH: %w", err)
		}
		cfg.Branding.HeaderImagePath = v
	}
	if v := os.Getenv("BRANDING_FAVICON_PATH"); v != "" {
		if _, err := os.Stat(v); err != nil {
			return Config{}, fmt.Errorf("BRANDING_FAVICON_PATH: %w", err)
		}
		cfg.Branding.FaviconPath = v
	}

	const defaultMaxStorageSize = 1 * 1024 * 1024 * 1024 // 1 GiB
	if raw := os.Getenv("MAX_STORAGE_SIZE"); raw != "" {
		n, err := humanize.ParseBytes(raw)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_STORAGE_SIZE: %w", err)
		}
		cfg.MaxStorageSize = int64(n)
	} else {
		cfg.MaxStorageSize = defaultMaxStorageSize
	}

	cfg.DatabaseURL = fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.DBUser, cfg.DBPassword,
		cfg.DBHost, cfg.DBPort,
		cfg.DBName, cfg.DBSSLMode,
	)

	// ── Allowed resolutions per item type ─────────────────────────────────────
	cfg.AllowedResolutions = make(map[string][]Resolution, len(DefaultResolutions))
	for k, v := range DefaultResolutions {
		cfg.AllowedResolutions[k] = v
	}
	// Image types whose resolutions can be overridden via env.
	for _, itemType := range []string{"gameskin", "hud", "skin", "entity", "theme", "template", "emoticon"} {
		envKey := "ALLOWED_RESOLUTIONS_" + strings.ToUpper(itemType)
		if raw := os.Getenv(envKey); raw != "" {
			parsed, err := ParseResolutions(raw)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", envKey, err)
			}
			cfg.AllowedResolutions[itemType] = parsed
		}
	}

	// ── Max upload sizes per item type ────────────────────────────────────────
	cfg.MaxUploadSizes = make(map[string]int64)
	for _, itemType := range []string{"map", "gameskin", "hud", "skin", "entity", "theme", "template", "emoticon"} {
		cfg.MaxUploadSizes[itemType] = DefaultMaxUploadSize(itemType, cfg.AllowedResolutions[itemType])
	}
	for _, itemType := range []string{"map", "gameskin", "hud", "skin", "entity", "theme", "template", "emoticon"} {
		envKey := "MAX_UPLOAD_SIZE_" + strings.ToUpper(itemType)
		if raw := os.Getenv(envKey); raw != "" {
			bytes, err := humanize.ParseBytes(raw)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", envKey, err)
			}
			cfg.MaxUploadSizes[itemType] = int64(bytes)
		}
	}

	// ── Thumbnail bounding box per item type ──────────────────────────────────
	cfg.ThumbnailSizes = DefaultThumbnailSizes()
	for _, itemType := range []string{"map", "gameskin", "hud", "skin", "entity", "theme", "template", "emoticon"} {
		envKey := "THUMBNAIL_SIZE_" + strings.ToUpper(itemType)
		if raw := os.Getenv(envKey); raw != "" {
			w, h, err := parseWxH(raw)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", envKey, err)
			}
			cfg.ThumbnailSizes[itemType] = Resolution{Width: w, Height: h}
		}
	}

	return cfg, nil
}
