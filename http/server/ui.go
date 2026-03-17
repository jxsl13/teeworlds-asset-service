package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jxsl13/asset-service/http/api"
	"github.com/jxsl13/asset-service/model"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// RenderUI implements api.StrictServerInterface.
// It returns the full HTML page with tabs and the HTMX search bar.
func (s *Server) RenderUI(ctx context.Context, _ api.RenderUIRequestObject) (api.RenderUIResponseObject, error) {
	allEnums := sqlc.AllAssetTypeEnumValues()
	types := make([]string, 0, len(allEnums))
	for _, e := range allEnums {
		types = append(types, string(e))
	}
	slices.Sort(types)

	var buf bytes.Buffer
	if err := s.layoutTpl.Execute(&buf, map[string]any{
		"ItemTypes":  types,
		"ActiveType": types[0],
		"Query":      "",
	}); err != nil {
		return nil, fmt.Errorf("render layout: %w", err)
	}

	resp := api.RenderUI200TexthtmlResponse{
		Body:          &buf,
		ContentLength: int64(buf.Len()),
	}

	// When navigated via HTMX (e.g. history restore), push the canonical URL.
	if isHxRequest(ctx) {
		return renderUIHtmxResponse{
			inner: resp,
			hx:    htmxHeaders{pushURL: "/"},
		}, nil
	}
	return resp, nil
}

// RenderItemList implements api.StrictServerInterface.
// It returns an HTML fragment with the items grid and pagination controls.
func (s *Server) RenderItemList(ctx context.Context, request api.RenderItemListRequestObject) (api.RenderItemListResponseObject, error) {
	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	q := ""
	if request.Params.Q != nil {
		q = strings.TrimSpace(*request.Params.Q)
	}

	sortRaw := ""
	if request.Params.Sort != nil {
		sortRaw = *request.Params.Sort
	}
	sortDirs := model.ParseSortDirectives(sortRaw)
	if len(sortDirs) == 0 {
		sortDirs = []model.SortDirective{{Field: "name", Desc: false}}
	}

	itemType := string(request.AssetType)

	var (
		views []itemView
		total int
	)

	svc := s.newService()

	if q != "" {
		query, err := model.NewSearchQueryByType(q, itemType, limit, offset, sortDirs)
		if err != nil {
			return api.RenderItemList400JSONResponse{Error: err.Error()}, nil
		}
		result, err := svc.Search(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("search items: %w", err)
		}
		total = result.Total
		for _, item := range result.Items {
			views = append(views, toItemView(item.GroupID.String(), string(item.AssetType), item.GroupName, item.GroupKey, item.Creators, item.Variants, item.TotalSize, item.CreatedAt))
		}
	} else {
		query, err := model.NewListQuery(itemType, limit, offset, nil, nil, nil, sortDirs)
		if err != nil {
			return api.RenderItemList400JSONResponse{Error: err.Error()}, nil
		}
		result, err := svc.ListItems(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("list items: %w", err)
		}
		total = result.Total
		for _, item := range result.Items {
			views = append(views, toItemView(item.GroupID.String(), string(item.AssetType), item.GroupName, item.GroupKey, item.Creators, item.Variants, item.TotalSize, item.CreatedAt))
		}
	}

	// Build sort state for template columns.
	columns := buildSortColumns(sortDirs)

	// Build the sort param string for pagination links.
	sortParam := buildSortParam(sortDirs)

	pageEnd := offset + len(views)
	prevOffset := offset - limit
	if prevOffset < 0 {
		prevOffset = 0
	}

	baseURL := "/" + itemType
	qParam := ""
	if q != "" {
		qParam = "&q=" + template.URLQueryEscaper(q)
	}
	sortQParam := ""
	if sortParam != "" {
		sortQParam = "&sort=" + template.URLQueryEscaper(sortParam)
	}

	data := itemsPageData{
		Items:     views,
		Total:     total,
		Limit:     limit,
		Offset:    offset,
		PageStart: offset + 1,
		PageEnd:   pageEnd,
		PrevURL:   fmt.Sprintf("%s?limit=%d&offset=%d%s%s", baseURL, limit, prevOffset, qParam, sortQParam),
		NextURL:   fmt.Sprintf("%s?limit=%d&offset=%d%s%s", baseURL, limit, offset+limit, qParam, sortQParam),
		SortParam: sortParam,
		Columns:   columns,
	}

	var buf bytes.Buffer
	if err := s.itemsTpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render items: %w", err)
	}

	resp := api.RenderItemList200TexthtmlResponse{
		Body:          &buf,
		ContentLength: int64(buf.Len()),
	}

	// For HTMX requests, push the current URL so the browser address bar
	// stays in sync with the visible content (enables refresh & bookmarks).
	if isHxRequest(ctx) {
		pushURL := baseURL
		if q != "" || offset > 0 || sortParam != "" {
			pushURL = fmt.Sprintf("%s?limit=%d&offset=%d%s%s", baseURL, limit, offset, qParam, sortQParam)
		}
		return renderItemListHtmxResponse{
			inner: resp,
			hx:    htmxHeaders{pushURL: pushURL},
		}, nil
	}

	// Direct browser navigation (not HTMX) — render the full page layout
	// with the correct tab pre-selected so the content loads on page init.
	return s.renderFullPage(itemType, q)
}

// renderFullPage renders the complete layout HTML with the given item type
// pre-selected. Used when /{asset_type} is accessed via direct browser navigation.
func (s *Server) renderFullPage(activeType, query string) (api.RenderItemListResponseObject, error) {
	allEnums := sqlc.AllAssetTypeEnumValues()
	types := make([]string, 0, len(allEnums))
	for _, e := range allEnums {
		types = append(types, string(e))
	}
	slices.Sort(types)

	var buf bytes.Buffer
	if err := s.layoutTpl.Execute(&buf, map[string]any{
		"ItemTypes":  types,
		"ActiveType": activeType,
		"Query":      query,
	}); err != nil {
		return nil, fmt.Errorf("render layout: %w", err)
	}

	return api.RenderItemList200TexthtmlResponse{
		Body:          &buf,
		ContentLength: int64(buf.Len()),
	}, nil
}

// ── template view models ──────────────────────────────────────────────────────

// variantView represents a single downloadable variant (e.g. one resolution).
type variantView struct {
	ItemID string
	Label  string // e.g. "256x128"
}

type itemView struct {
	GroupID      string
	ItemType     string
	Name         string
	Creators     string
	Size         string        // human-readable total size
	Date         string        // formatted creation date
	GroupKey     string        // e.g. "resolution" — used to label the variants column
	Variants     []variantView // individual downloadable variants within this group
	HasThumbnail bool
}

// sortColumn describes a column's current sort state for the template.
type sortColumn struct {
	Field string // "name" or "created_at"
	Label string // display label
	Dir   string // "asc", "desc", or "" (unsorted)
	Rank  int    // 1-based position in multi-sort, 0 if not active
}

type itemsPageData struct {
	Items     []itemView
	Total     int
	Limit     int
	Offset    int
	PageStart int
	PageEnd   int
	PrevURL   string
	NextURL   string
	SortParam string       // raw sort param to preserve in pagination links
	Columns   []sortColumn // columns for header rendering
}

// parseVariants splits the DB-aggregated "uuid:value,uuid:value,…" string
// into a slice of variantView sorted by resolution (smallest to largest).
// Resolution labels like "256x128" are sorted by w*h, then w, then h.
func parseVariants(raw string) []variantView {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]variantView, 0, len(parts))
	for _, p := range parts {
		idx := strings.IndexByte(p, ':')
		if idx < 0 {
			continue
		}
		out = append(out, variantView{
			ItemID: p[:idx],
			Label:  p[idx+1:],
		})
	}
	slices.SortFunc(out, func(a, b variantView) int {
		aw, ah := parseResolution(a.Label)
		bw, bh := parseResolution(b.Label)
		if d := (aw * ah) - (bw * bh); d != 0 {
			return d
		}
		if d := aw - bw; d != 0 {
			return d
		}
		return ah - bh
	})
	return out
}

// parseResolution extracts width and height from a "WxH" string.
// Returns (0, 0) for non-resolution labels.
func parseResolution(s string) (int, int) {
	idx := strings.IndexByte(s, 'x')
	if idx < 0 {
		return 0, 0
	}
	w, err1 := strconv.Atoi(s[:idx])
	h, err2 := strconv.Atoi(s[idx+1:])
	if err1 != nil || err2 != nil {
		return 0, 0
	}
	return w, h
}

func toItemView(groupID, assetType, groupName, groupKey, creators, variants string, totalSize int64, createdAt time.Time) itemView {
	if creators == "" {
		creators = "unknown"
	}
	date := createdAt.Format("2006-01-02")
	if createdAt.Year() <= 1970 {
		date = ""
	}
	return itemView{
		GroupID:      groupID,
		ItemType:     assetType,
		Name:         groupName,
		Creators:     creators,
		Size:         formatSize(totalSize),
		Date:         date,
		GroupKey:     groupKey,
		Variants:     parseVariants(variants),
		HasThumbnail: true,
	}
}

// formatSize converts a byte count to a human-readable string.
func formatSize(b int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// buildSortColumns builds the sortColumn slice used by the template header.
func buildSortColumns(dirs []model.SortDirective) []sortColumn {
	allCols := []struct{ field, label string }{
		{"name", "Name"},
		{"creators", "Creators"},
		{"size", "Size"},
		{"created_at", "Date"},
	}
	// Build a lookup from active directives.
	active := make(map[string]struct {
		dir  string
		rank int
	})
	for i, d := range dirs {
		dir := "asc"
		if d.Desc {
			dir = "desc"
		}
		active[d.Field] = struct {
			dir  string
			rank int
		}{dir, i + 1}
	}
	cols := make([]sortColumn, 0, len(allCols))
	for _, c := range allCols {
		sc := sortColumn{Field: c.field, Label: c.label}
		if a, ok := active[c.field]; ok {
			sc.Dir = a.dir
			sc.Rank = a.rank
		}
		cols = append(cols, sc)
	}
	return cols
}

// buildSortParam serialises sort directives back into "field:dir,field:dir".
func buildSortParam(dirs []model.SortDirective) string {
	if len(dirs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(dirs))
	for _, d := range dirs {
		dir := "asc"
		if d.Desc {
			dir = "desc"
		}
		parts = append(parts, d.Field+":"+dir)
	}
	return strings.Join(parts, ",")
}
