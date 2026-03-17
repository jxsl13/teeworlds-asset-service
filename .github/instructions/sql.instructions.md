---
description: "Use when editing SQL queries, adding new database queries, modifying search/list/insert queries, or working with sqlc code generation. Covers sqlc conventions, query patterns, and the pg_trgm search algorithm."
applyTo: "sql/**"
---
# SQL & sqlc Conventions

## sqlc Query Annotations

Every query file in `sql/queries/` uses sqlc annotations:
```sql
-- name: QueryName :many    -- returns []Row
-- name: QueryName :one     -- returns single Row
-- name: QueryName :exec    -- no return (INSERT/UPDATE/DELETE)
```

## Generated Output

- `sql/db.gen.go` — DAO struct with typed methods per query
- `sql/models.gen.go` — Go structs for DB enums and table models
- Config: `sql/sqlc.yaml` (engine: postgresql, references queries/ and migrations/)

After editing any `.sql` file, run `make generate`.

## Type Mapping Gotchas

- `COALESCE(SUM(...), 0)` → `interface{}` unless you cast: `::BIGINT`
- `COALESCE(timestamptz_subquery, literal)` → `interface{}` unless you cast: `::timestamptz`
- Always add explicit casts for COALESCE results used in SELECT

## Search Algorithm (pg_trgm)

- `strict_word_similarity(key_value, $1)` scores matches [0,1]
- Optional weights per `search_value.key_name` via `search_value_weight` table
- Scores aggregated per group, sorted DESC
- `COUNT(*) OVER ()` window function provides `total_count` for pagination

## Sort Pattern (ORDER BY with dynamic field)

Uses text-based CASE expressions for multi-field sort:
- Strings: return directly (`ag.group_name`)
- Timestamps: `to_char(ts, 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`
- Integers: `lpad(cast(int as text), 20, '0')` for text-sortable numeric values
- Two sort levels: `sort_field`/`sort_desc` + `sort_field_2`/`sort_desc_2`

## Schema: Core Tables

- `asset_group` — logical asset (group_id, asset_type, group_name, group_key)
- `asset_item` — per-file variant (item_id, group_id, group_value, size, checksum)
- `asset_item_metadata` — per-item metadata (created_at, etc.)
- `search_value` — searchable key-value pairs (group_id, key_name, key_value)
- `search_value_weight` — per-key_name score boost for search ranking
