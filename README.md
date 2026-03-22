# Teeworlds Asset Database

Community database for Teeworlds assets — skins, maps, gameskins & more. Go service with layered architecture and OpenAPI-first code generation.

## Configuration

All configuration is via environment variables. See [`docker/dev.env`](docker/dev.env) for a working example.

### Database

| Variable | Required | Default | Description |
|---|---|---|---|
| `DB_HOST` | **yes** | — | PostgreSQL host |
| `DB_PORT` | no | `5432` | PostgreSQL port |
| `DB_USER` | **yes** | — | PostgreSQL user |
| `DB_PASSWORD` | **yes** | — | PostgreSQL password |
| `DB_NAME` | **yes** | — | PostgreSQL database name |
| `DB_SSLMODE` | no | `disable` | SSL mode (`disable`, `require`, `verify-full`, …) |

### Server

| Variable | Required | Default | Description |
|---|---|---|---|
| `ADDR` | no | `:8080` | TCP address the HTTP server listens on |
| `INSECURE` | no | `false` | Set to `true` to allow non-HTTPS cookies (local dev) |
| `ADMIN_ONLY_UPLOAD` | no | `false` | Restrict uploads to admin users only |
| `ITEMS_PER_PAGE` | no | `100` | Default number of items per page (1–1000) |

### Branding

| Variable | Required | Default | Description |
|---|---|---|---|
| `BRANDING_TITLE` | no | `Teeworlds Asset Database` | Page title and header heading |
| `BRANDING_SUBTITLE` | no | `Community database for skins, maps, gameskins & more` | Tagline below the header heading |
| `BRANDING_HEADER_IMAGE_PATH` | no | — | Local file path for a logo/image displayed in the header (served at `/branding/header-image`) |
| `BRANDING_FAVICON_PATH` | no | — | Local file path for the browser tab icon (served at `/branding/favicon`) |

### Storage

| Variable | Required | Default | Description |
|---|---|---|---|
| `STORAGE_PATH` | **yes** | — | Base directory for all stored asset files |
| `TEMP_UPLOAD_PATH` | no | OS temp dir | Directory for temporary upload files |
| `MAX_STORAGE_SIZE` | no | `1GiB` | Maximum total size for all stored items (human-readable, e.g. `10GiB`, `500MB`) |

### Per-Asset-Type Overrides

Each variable below can be suffixed with the asset type in uppercase:
`MAP`, `GAMESKIN`, `HUD`, `SKIN`, `ENTITY`, `THEME`, `TEMPLATE`, `EMOTICON`.

| Variable Pattern | Default | Description |
|---|---|---|
| `ALLOWED_RESOLUTIONS_{TYPE}` | built-in per type | Comma-separated `WxH` pairs (e.g. `256x128,512x256`) |
| `MAX_UPLOAD_SIZE_{TYPE}` | derived from largest resolution | Maximum upload size per item (e.g. `10MiB`) |
| `THUMBNAIL_SIZE_{TYPE}` | smallest resolution (map: `1920x1080`, skin: `64x64`) | Thumbnail bounding box as `WxH` |

### OIDC / Pocket-ID

When all four variables below are set, OIDC authentication and admin functionality are enabled. When omitted, the service runs in anonymous-only mode with no login link, no admin controls, and no admin API routes.

| Variable | Required | Default | Description |
|---|---|---|---|
| `EXTERNAL_URL` | when OIDC is set | — | Publicly reachable base URL of this service (e.g. `https://assets.example.com`); OIDC callback URLs (`/auth/callback`, `/auth/post-logout`) are derived from this |
| `OIDC_ISSUER_URL` | no | — | Pocket-ID base URL (e.g. `https://id.example.com`) |
| `OIDC_CLIENT_ID` | no | — | OIDC client ID (from `cmd/provision-pocketid`) |
| `OIDC_CLIENT_SECRET` | no | — | OIDC client secret (from `cmd/provision-pocketid`) |

> **Note:** Setting only some of the three `OIDC_*` variables is a configuration error — set all three or none.

### Rate Limiting

| Variable | Required | Default | Description |
|---|---|---|---|
| `RATE_LIMIT_MAX_GROUPS` | no | `10` | Max new asset groups per IP within the window (0 = disabled) |
| `RATE_LIMIT_WINDOW` | no | `24h` | Sliding window for per-IP group creation rate limit |
| `HTTP_RATE_LIMIT_RATE` | no | `20` | Requests per second per IP (0 = disabled) |
| `HTTP_RATE_LIMIT_BURST` | no | `40` | Max burst size for per-IP token bucket |
| `HTTP_RATE_LIMIT_CLEANUP` | no | `10m` | How long idle IP entries are kept before eviction |

### Pocket-ID Provisioning (one-time setup)

Run `cmd/provision-pocketid` once to create the OIDC client, admin group, admin user, and obtain the client credentials. Add the output `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET` to your environment before starting the service.

| Variable | Required | Default | Description |
|---|---|---|---|
| `POCKET_ID_STATIC_API_KEY` | **yes** | — | Pocket-ID admin API key |
| `POCKET_ID_ADMIN_EMAIL` | no | `admin@example.com` | Email for the initial admin user in Pocket-ID |
| `POCKET_ID_CLIENT_NAME` | no | `Teeworlds Asset Database` | Display name for the OIDC client in Pocket-ID |

> **Note:** `cmd/provision-pocketid` also reads `OIDC_ISSUER_URL` and `EXTERNAL_URL` from the server configuration above.

## CLI Commands

### Server

Start the server (all configuration via environment variables):

```bash
# Source dev config and run
set -a && . docker/dev.env && set +a && go run .

# Or build and run the binary
go build -o teeworlds-asset-db .
./teeworlds-asset-db
```

### Pocket-ID Provisioning (`cmd/provision-pocketid`)

One-time setup: creates the OIDC client, admin group, and admin user on a Pocket-ID instance.
Outputs `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET`.

```bash
# Provision and print credentials to stdout
set -a && . docker/dev.env && set +a && go run ./cmd/provision-pocketid

# Provision and auto-update docker/dev.env with the credentials
set -a && . docker/dev.env && set +a && go run ./cmd/provision-pocketid -env-file docker/dev.env

# Shorthand via Make
make pocketid-provision
```

| Flag | Default | Description |
|---|---|---|
| `-env-file` | — | Path to `.env` file to update with OIDC credentials (omit to print to stdout) |

### DDNet Skin Seeder (`cmd/seed-ddnet-skins`)

Fetches the DDNet skin database and uploads all skins (including UHD variants) to a running server.

```bash
# Upload all skins to localhost:8080
go run ./cmd/seed-ddnet-skins

# Target a different server
go run ./cmd/seed-ddnet-skins -addr http://localhost:9090

# Only community skins
go run ./cmd/seed-ddnet-skins -type community

# Only normal (non-community) skins
go run ./cmd/seed-ddnet-skins -type normal

# Limit parallel downloads to 4
go run ./cmd/seed-ddnet-skins -concurrency 4

# Shorthand via Make
make seed-ddnet-skins
```

| Flag | Default | Description |
|---|---|---|
| `-addr` | `http://localhost:8080` | Base URL of the running server |
| `-type` | _(all)_ | Filter by skin type: `normal`, `community`, or empty for all |
| `-concurrency` | `8` | Number of parallel download/upload workers |

### Make Targets

```bash
make build               # Compile the binary
make syntax              # Run go vet + go build
make generate            # Regenerate code (oapi-codegen + sqlc)
make test                # Run tests (sources docker/dev.env)
make db-up               # Start PostgreSQL container
make db-down             # Stop PostgreSQL and remove volumes
make db-reset            # Full DB reset: stop, wipe data, restart
make pocketid-provision  # Provision OIDC client (one-time setup)
make seed-ddnet-skins    # Import DDNet skins into running service
```

### Example Production Config

```env
# Database
DB_HOST=<your-postgres-host>
DB_PORT=5432
DB_USER=<your-db-user>
DB_PASSWORD=<your-db-password>
DB_NAME=<your-db-name>
DB_SSLMODE=require

# Server
ADDR=:8080

# Storage
STORAGE_PATH=/var/lib/asset-service/data
TEMP_UPLOAD_PATH=/var/lib/asset-service/tmp
MAX_STORAGE_SIZE=50GiB

# OIDC (Pocket-ID) — credentials from cmd/provision-pocketid
OIDC_ISSUER_URL=https://<your-pocket-id-domain>
OIDC_CLIENT_ID=<from provision output>
OIDC_CLIENT_SECRET=<from provision output>
EXTERNAL_URL=https://<your-asset-service-domain>
```
