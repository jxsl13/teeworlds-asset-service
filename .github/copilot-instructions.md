# Teeworlds Asset Database

Community database for Teeworlds assets — Go service with layered architecture and OpenAPI-first code generation.

## Architecture

```
http/server/  (handlers implement api.StrictServerInterface)
  → http/service/  (business logic: SearchService)
    → model/  (domain: Item, SearchQuery, ListQuery, results)
      → sql/  (DAO via sqlc-generated code)
        → PostgreSQL (pg_trgm fuzzy search)
```

## Code Generation — run `make generate`

Two code generators, **both triggered by `make generate`** (`go generate ./...`):

1. **oapi-codegen**: `http/api/openapi.yaml` → `http/api/server.gen.go` + `types.gen.go`
   - Never edit `*.gen.go` — edit `openapi.yaml` then regenerate
   - Handlers must satisfy `api.StrictServerInterface`

2. **sqlc**: `sql/queries/*.sql` + `sql/migrations/*.sql` → `sql/db.gen.go` + `models.gen.go`
   - Never edit `sql/*.gen.go` — edit `.sql` files then regenerate
   - Config: `sql/sqlc.yaml`

## Key Build Commands

- `make generate` — regenerate all code (oapi-codegen + sqlc)
- `make build` — compile binary
- `make syntax` — go vet + go build
- `make test` — source dev.env + run tests
- `make db-up` / `make db-down` / `make db-reset` — Docker PostgreSQL lifecycle

## Handler Pattern

Every handler follows this flow:
```
Request (api.*RequestObject)
  → Parse + Validate (model.New*Query)
  → Service call (svc.Search / svc.ListItems)
  → Domain model (model.*Result)
  → Convert (.ToAPI() for JSON, toItemView() for HTML)
  → Response (api.*ResponseObject)
```

## File Organization Conventions

| Area | Location | Notes |
|------|----------|-------|
| OpenAPI spec | `http/api/openapi.yaml` | Single source of truth for HTTP API |
| Generated server/types | `http/api/*.gen.go` | Never edit manually |
| Handler impls | `http/server/*.go` | One file per concern (search, upload, download, ui) |
| Upload handlers | `http/server/upload_{type}.go` | One per asset type (8 types) |
| Templates (HTMX) | `http/server/templates/*.html` | Embedded via `//go:embed` |
| Static assets | `http/server/static/` | CSS, JS — embedded via `//go:embed` |
| Business logic | `http/service/service.go` | Thin: calls DAO, returns domain models |
| Domain models | `model/*.go` | Query/Result value objects, DB↔API converters |
| SQL queries | `sql/queries/*.sql` | sqlc annotated (`-- name: X :many`) |
| Migrations | `sql/migrations/*.sql` | golang-migrate format (NNN_name.up/down.sql) |
| Generated DB code | `sql/*.gen.go` | Never edit manually |
| Config | `config/config.go` | Env var loading (DB_*, STORAGE_*, MAX_*, etc.) |
| Docker | `docker/docker-compose.yaml` | PostgreSQL 17 + PgAdmin |

## Asset Types (8)

map, gameskin, hud, skin, entity, theme, template, emoticon — defined as PostgreSQL enum in `sql/migrations/001_schema.up.sql` and Go config in `config/resolution.go`.

## Configuration Changes

Whenever any file in `config/` is modified (e.g. `config/config.go`, `config/resolution.go`), always update `README.md` to reflect the changes:

- Add, remove, or update rows in the relevant configuration table (Database, Server, Storage, OIDC, Rate Limiting, Branding, Per-Asset-Type Overrides).
- Update the **Example Production Config** snippet if required/optional variables changed.
- Update the **Pocket-ID Provisioning** table if `cmd/provision-pocketid` reads new or renamed variables.
