---
description: "Use when editing HTTP handlers, implementing new API endpoints, working with the server struct, upload handlers, download logic, HTMX responses, or middleware. Covers the handler pattern, UI rendering, and request flow."
applyTo: "http/server/**"
---
# HTTP Server Conventions

## Server Struct

`http/server/server.go` — implements `api.StrictServerInterface` (generated from OpenAPI).
Holds: DAO, filesystem (http.Dir), validator, cache, parsed templates (layout.html + items.html).

## Handler Implementation Pattern

```go
func (s *Server) OperationName(ctx context.Context, request api.OperationNameRequestObject) (api.OperationNameResponseObject, error) {
    // 1. Parse params from request.Params.*
    // 2. Validate via model.New*Query(...)
    // 3. Call service: svc.Method(ctx, query)
    // 4. Convert result: result.ToAPI() or toItemView(...)
    // 5. Return: api.OperationName200JSONResponse{...} or 400/404
}
```

## UI (HTMX) Rendering

- Full-page: execute `layout.html` template (detected by `!isHxRequest(ctx)`)
- Fragment: execute `items.html` template (HTMX partial swap, set `hx-push-url`)
- Template data struct `itemsPageData` contains Items, pagination, sort columns
- `toItemView()` converts domain model → template view model

## Upload Handlers

- Dispatcher: `upload.go` routes to `upload_{type}.go` based on asset type
- Each type handler validates resolution/size → generates thumbnail → writes file → inserts DB rows
- 8 asset types: map, gameskin, hud, skin, entity, theme, template, emoticon

## Templates (go:embed)

- `templates/layout.html` — full page structure, nav tabs, search box
- `templates/items.html` — result table (thead sort header + tbody data rows)
- `static/` — CSS (style.css), JS (htmx.min.js, upload.js)

## Middleware Stack (main.go)

RequestLogger → ClientIP → HTMX header parsing → Routes
