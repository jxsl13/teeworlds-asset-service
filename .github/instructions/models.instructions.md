---
description: "Use when editing domain models, query/result value objects, adding new fields to items, modifying search or list results, or working with DB-to-API conversions. Covers model structure and conversion patterns."
applyTo: "model/**"
---
# Domain Model Conventions

## Core Types

- `Item` — search result entity (GroupID, AssetType, GroupName, GroupKey, Creators, Variants, TotalSize, CreatedAt, Score)
- `ListItem` — list result entity (same fields minus Score)
- `SearchQuery` — validated search input (Q, ItemType, Limit, Offset, Sort)
- `ListQuery` — validated list input (ItemType, Limit, Offset, Filters, Sort up to 2 directives)
- `SearchResult` / `ListResult` — aggregate: Items slice + Total count

## Constructors Validate

- `model.NewSearchQuery(q, limit, offset)` — validates q non-empty, clamps limit [1,100]
- `model.NewSearchQueryByType(q, itemType, limit, offset, sort)` — adds type constraint
- `model.NewListQuery(itemType, limit, offset, filterName, filterCreator, filterLicense, sortDirs)` — validates sort fields

## Conversion Chain

```
sqlc.Row → model.Item/ListItem      (ItemFromRow, ListResultFromRows)
model.Item → api.SearchResult       (.ToAPI())
model.ListItem → api.ListItem       (via ListResult.ToAPI())
model.Item → itemView               (toItemView() in http/server/ui.go)
```

## Sort Directives

- `ParseSortDirectives("name:asc,created_at:desc")` → `[]SortDirective`
- Valid fields: `name`, `creators`, `size`, `created_at` (in `validSortFields` map)
- Max 2 sort directives (primary + secondary)

## Adding a New Field

1. Add column to SQL SELECT in `sql/queries/list.sql` and `sql/queries/search.sql`
2. Run `make generate` → new field appears in sqlc Row struct
3. Add field to `model.Item` and `model.ListItem`
4. Update conversion functions: `ItemFromRow`, `ListResultFromRows`, `SearchResultFromByTypeRows`
5. Add to `itemView` and `toItemView()` in `http/server/ui.go`
6. Add to template `items.html`
