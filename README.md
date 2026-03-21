# Asset Service

Go asset search service — layered architecture with OpenAPI-first code generation.

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

| Variable | Required | Default | Description |
|---|---|---|---|
| `OIDC_ISSUER_URL` | no | — | Pocket-ID base URL (e.g. `https://id.example.com`). When empty, auth is disabled. |
| `OIDC_CLIENT_ID` | no | — | OIDC client ID. Auto-filled when provisioning is enabled. |
| `OIDC_CLIENT_SECRET` | no | — | OIDC client secret. Auto-filled when provisioning is enabled. |
| `OIDC_REDIRECT_URL` | no | — | OAuth2 callback URL (e.g. `https://assets.example.com/auth/callback`) |
| `OIDC_POST_LOGOUT_REDIRECT_URL` | no | — | Post-logout redirect URL (e.g. `https://assets.example.com`) |

### Pocket-ID Auto-Provisioning

When `POCKET_ID_STATIC_API_KEY` and `OIDC_ISSUER_URL` are both set, the service automatically provisions Pocket-ID at startup: creates the OIDC client, admin group, admin user, and prints a one-time login URL. Credentials are AES-256-GCM encrypted and persisted in the database, making the process idempotent across restarts.

| Variable | Required | Default | Description |
|---|---|---|---|
| `POCKET_ID_STATIC_API_KEY` | no | — | Pocket-ID admin API key. Enables auto-provisioning when set. |
| `POCKET_ID_ENCRYPTION_KEY` | when provisioning | — | Passphrase for encrypting stored OIDC credentials. **Required** when `POCKET_ID_STATIC_API_KEY` is set. |
| `POCKET_ID_ADMIN_EMAIL` | no | `admin@example.com` | Email for the initial admin user in Pocket-ID |
| `POCKET_ID_CLIENT_NAME` | no | `Asset Service` | Display name for the OIDC client in Pocket-ID |

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

# OIDC (Pocket-ID)
OIDC_ISSUER_URL=https://<your-pocket-id-domain>
OIDC_REDIRECT_URL=https://<your-asset-service-domain>/auth/callback
OIDC_POST_LOGOUT_REDIRECT_URL=https://<your-asset-service-domain>

# Pocket-ID auto-provisioning
POCKET_ID_STATIC_API_KEY=<your-pocket-id-static-api-key>
POCKET_ID_ENCRYPTION_KEY=<random-32-char-string>
POCKET_ID_ADMIN_EMAIL=<your-admin-email>
POCKET_ID_CLIENT_NAME=Asset Service
```

Generate a secure encryption key with:

```sh
openssl rand -base64 32
```
