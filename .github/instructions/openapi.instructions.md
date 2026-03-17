---
description: "Use when editing the OpenAPI spec, adding or modifying API endpoints, changing request/response schemas, or running oapi-codegen. Covers the oapi-codegen workflow and spec conventions."
applyTo: "http/api/**"
---
# OpenAPI / oapi-codegen Conventions

## Files

- `http/api/openapi.yaml` — single source of truth for all HTTP endpoints
- `http/api/server.gen.go` — generated Chi router + `StrictServerInterface`
- `http/api/types.gen.go` — generated request/response DTOs
- Never edit `*.gen.go` files

## Workflow: Adding/Changing an Endpoint

1. Edit `openapi.yaml` (paths, schemas, parameters)
2. Run `make generate`
3. Implement or update handler in `http/server/` to satisfy the updated `StrictServerInterface`
4. The compiler will show missing methods if the interface changed

## Config

- `http/api/oapi-codegen.yaml` — generates types (package: api)
- `http/api/oapi-codegen-server.yaml` — generates server (strict-server, chi)
- `//go:generate` directives in `http/api/api.go` invoke both

## Conventions

- Operation IDs: PascalCase (SearchItems, UploadItem, RenderUI, RenderItemList)
- Params: query params use snake_case (limit, offset, asset_type, q, sort)
- Asset types: path param `{asset_type}` validated as enum
