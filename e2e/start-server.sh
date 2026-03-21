#!/usr/bin/env bash
# e2e/start-server.sh — boots the full E2E environment.
#
# 1. Starts PostgreSQL + Pocket-ID + Caddy via docker compose.
# 2. Provisions the OIDC client & admin user (idempotent).
# 3. Seeds the database with test fixtures.
# 4. Starts the asset-service in foreground (Playwright manages lifecycle).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="$ROOT/docker/dev.env"
COMPOSE="docker compose -f docker/docker-compose.yaml --env-file $ENV_FILE"

# ── 1. Docker containers ────────────────────────────────────────────────────
echo "▸ Starting docker containers…"
mkdir -p "$ROOT/volumes/postgresql/data"
$COMPOSE up -d db pocket-id caddy --wait 2>&1

# ── 2. Provision Pocket-ID (idempotent) ─────────────────────────────────────
echo "▸ Provisioning Pocket-ID…"
set -a && . "$ENV_FILE" && set +a
go run ./cmd/provision-pocketid -env-file "$ENV_FILE" 2>&1

# Reload env (provision may have updated OIDC_CLIENT_ID / OIDC_CLIENT_SECRET).
set -a && . "$ENV_FILE" && set +a

# ── 3. Ensure storage directory exists ──────────────────────────────────────
mkdir -p "${STORAGE_PATH:-./testdata}"

# ── 4. Start the asset-service (foreground) ─────────────────────────────────
echo "▸ Starting asset-service on ${ADDR:-:8080}…"
exec go run .
