---
description: "Use when editing HTML templates, CSS styles, JavaScript, or HTMX interactions. Covers template structure, CSS conventions, sort header, and upload.js patterns."
applyTo: "http/server/templates/**,http/server/static/**"
---
# Frontend (Templates + Static) Conventions

## Templates

- `layout.html` — full page: nav tabs (asset types), search input with hidden sort field, upload button
- `items.html` — unified `<table>` with `<thead>` (sort header) + `<tbody>` (data rows)
- Template data: `.Items`, `.Columns` (sort headers), `.Total`, `.Limit`, `.Offset`, pagination URLs

## Table Structure (items.html)

Columns in order: Thumb | Name | Creators | Size | Date | Actions (Downloads)
- `<thead>` sort header uses `<th>` with `onclick="onSortClick(event, 'field')"`
- `<tbody>` data rows use matching `<td class="col-{field}">` classes
- Both share the same `<table>` so columns auto-align

## CSS Conventions (style.css)

- CSS variables: `--bg`, `--bg-card`, `--accent`, `--text`, `--text-dim`, `--border`, `--link`, `--radius`, `--shadow`
- Column widths: `.col-thumb` (60px), `.col-size` (5rem), `.col-created_at` (6.5rem), `.col-actions` (auto)
- `.col-name` and `.col-creators` flex naturally
- Sort: `.active` class + `.sort-indicator` (↑/↓) + `.sort-rank` (numbered badge)

## JavaScript (upload.js)

- `currentSort` — array of `{field, dir}` objects (max 2)
- `onSortClick(event, field)` — click = replace sort, shift+click = add secondary
- `applySortRequest()` — builds URL with sort param, includes search query, fires htmx.ajax
- `switchTab(btn)` — changes asset type tab, resets sort + search
- Hidden `<input id="searchSort">` keeps sort param synced for search requests

## HTMX Integration

- Search input: `hx-get="/{type}" hx-trigger="input changed delay:100ms"` with `hx-include="#search, #searchSort"`
- Pagination: `hx-get` on prev/next buttons, `hx-target="#content"`
- Server detects HTMX via `isHxRequest(ctx)` → returns fragment (items.html only) instead of full page
