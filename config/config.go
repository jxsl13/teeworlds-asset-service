package config

import (
	"fmt"
	"os"
	"strings"

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

	// OIDC / Pocket-ID configuration.
	// All fields are optional — when OIDCIssuerURL is empty, auth is disabled.
	OIDCIssuerURL             string // Env: OIDC_ISSUER_URL
	OIDCClientID              string // Env: OIDC_CLIENT_ID
	OIDCClientSecret          string // Env: OIDC_CLIENT_SECRET
	OIDCRedirectURL           string // Env: OIDC_REDIRECT_URL
	OIDCPostLogoutRedirectURL string // Env: OIDC_POST_LOGOUT_REDIRECT_URL

	// Insecure disables secure cookies (HTTPS requirement) for OIDC sessions.
	// Env: INSECURE (default: false — set to "true" for local HTTP dev)
	Insecure bool

	// PocketIDStaticAPIKey is the admin API key for auto-provisioning Pocket-ID.
	// When set (together with OIDCIssuerURL), the service provisions the OIDC
	// client, admin group, admin user and prints a one-time login URL at startup.
	// Env: POCKET_ID_STATIC_API_KEY
	PocketIDStaticAPIKey string

	// PocketIDAdminEmail is the email for the initial Pocket-ID admin user.
	// Env: POCKET_ID_ADMIN_EMAIL (default: admin@example.com)
	PocketIDAdminEmail string

	// PocketIDClientName is the display name for the OIDC client in Pocket-ID.
	// Env: POCKET_ID_CLIENT_NAME (default: "Asset Service")
	PocketIDClientName string

	// PocketIDEncryptionKey is the passphrase for encrypting OIDC credentials
	// stored in the database. Required when PocketIDStaticAPIKey is set.
	// Env: POCKET_ID_ENCRYPTION_KEY
	PocketIDEncryptionKey string
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

		PocketIDStaticAPIKey:  os.Getenv("POCKET_ID_STATIC_API_KEY"),
		PocketIDAdminEmail:    os.Getenv("POCKET_ID_ADMIN_EMAIL"),
		PocketIDClientName:    os.Getenv("POCKET_ID_CLIENT_NAME"),
		PocketIDEncryptionKey: os.Getenv("POCKET_ID_ENCRYPTION_KEY"),
	}

	var missing []string
	for _, kv := range []struct{ key, val string }{
		{"DB_HOST", cfg.DBHost},
		{"DB_USER", cfg.DBUser},
		{"DB_PASSWORD", cfg.DBPassword},
		{"DB_NAME", cfg.DBName},
		{"STORAGE_PATH", cfg.StoragePath},
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
	if cfg.PocketIDAdminEmail == "" {
		cfg.PocketIDAdminEmail = "admin@example.com"
	}
	if cfg.PocketIDClientName == "" {
		cfg.PocketIDClientName = "Asset Service"
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
