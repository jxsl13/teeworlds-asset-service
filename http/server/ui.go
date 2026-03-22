package server

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	"github.com/jxsl13/teeworlds-asset-service/http/server/middleware/htmx"
	"github.com/jxsl13/teeworlds-asset-service/http/server/middleware/oidcauth"
	"github.com/jxsl13/teeworlds-asset-service/model"
	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
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
		"ItemTypes":      types,
		"ActiveType":     types[0],
		"Query":          "",
		"ContentURL":     "/" + types[0],
		"SortParam":      "name:asc",
		"IsAdmin":        isAdmin(ctx),
		"UserName":       userName(ctx),
		"CanUpload":      !s.adminOnlyUpload || isAdmin(ctx),
		"SiteTitle":      s.branding.SiteTitle,
		"SiteSubtitle":   s.branding.SiteSubtitle,
		"HasHeaderImage": s.branding.HeaderImagePath != "",
		"HasFavicon":     s.branding.FaviconPath != "",
		"SourceURL":      s.branding.SourceURL,
	}); err != nil {
		return nil, fmt.Errorf("render layout: %w", err)
	}

	resp := api.RenderUI200TexthtmlResponse{
		Body:          &buf,
		ContentLength: int64(buf.Len()),
	}

	// When navigated via HTMX (e.g. history restore), push the canonical URL.
	if htmx.IsHTMXRequest(ctx) {
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
	limit := s.itemsPerPage
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

	// Filter params (only meaningful in list mode, ignored when q is set).
	var filterCreator, filterLicense, filterDate *string
	if request.Params.Creator != nil && *request.Params.Creator != "" {
		filterCreator = request.Params.Creator
	}
	if request.Params.License != nil && *request.Params.License != "" {
		filterLicense = request.Params.License
	}
	if request.Params.Date != nil && *request.Params.Date != "" {
		filterDate = request.Params.Date
	}

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
			views = append(views, toItemView(item.GroupID.String(), string(item.AssetType), item.GroupName, item.GroupKey, item.Creators, item.License, item.Variants, item.TotalSize, item.CreatedAt))
		}
	} else {
		query, err := model.NewListQuery(itemType, limit, offset, nil, filterCreator, filterLicense, filterDate, sortDirs)
		if err != nil {
			return api.RenderItemList400JSONResponse{Error: err.Error()}, nil
		}
		result, err := svc.ListItems(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("list items: %w", err)
		}
		total = result.Total
		for _, item := range result.Items {
			views = append(views, toItemView(item.GroupID.String(), string(item.AssetType), item.GroupName, item.GroupKey, item.Creators, item.License, item.Variants, item.TotalSize, item.CreatedAt))
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
	filterQParam := ""
	if filterCreator != nil {
		filterQParam += "&creator=" + template.URLQueryEscaper(*filterCreator)
	}
	if filterLicense != nil {
		filterQParam += "&license=" + template.URLQueryEscaper(*filterLicense)
	}
	if filterDate != nil {
		filterQParam += "&date=" + template.URLQueryEscaper(*filterDate)
	}

	// Build active filter list for the filter bar.
	var activeFilters []activeFilterView
	if filterCreator != nil {
		activeFilters = append(activeFilters, activeFilterView{Field: "creator", Label: "Creator", Value: *filterCreator})
	}
	if filterLicense != nil {
		activeFilters = append(activeFilters, activeFilterView{Field: "license", Label: "License", Value: *filterLicense})
	}
	if filterDate != nil {
		activeFilters = append(activeFilters, activeFilterView{Field: "date", Label: "Date", Value: *filterDate})
	}

	data := itemsPageData{
		Items:         views,
		Total:         total,
		Limit:         limit,
		Offset:        offset,
		PageStart:     offset + 1,
		PageEnd:       pageEnd,
		PrevURL:       fmt.Sprintf("%s?limit=%d&offset=%d%s%s%s", baseURL, limit, prevOffset, qParam, sortQParam, filterQParam),
		NextURL:       fmt.Sprintf("%s?limit=%d&offset=%d%s%s%s", baseURL, limit, offset+limit, qParam, sortQParam, filterQParam),
		SortParam:     sortParam,
		Columns:       columns,
		ActiveFilters: activeFilters,
		IsAdmin:       isAdmin(ctx),
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
	if htmx.IsHTMXRequest(ctx) {
		pushURL := baseURL
		if q != "" || offset > 0 || sortParam != "" || filterQParam != "" {
			pushURL = fmt.Sprintf("%s?limit=%d&offset=%d%s%s%s", baseURL, limit, offset, qParam, sortQParam, filterQParam)
		}
		return renderItemListHtmxResponse{
			inner: resp,
			hx:    htmxHeaders{pushURL: pushURL},
		}, nil
	}

	// Direct browser navigation (not HTMX) — render the full page layout
	// with the correct tab pre-selected so the content loads on page init.
	contentParams := url.Values{}
	contentParams.Set("limit", strconv.Itoa(limit))
	contentParams.Set("offset", strconv.Itoa(offset))
	if q != "" {
		contentParams.Set("q", q)
	}
	if sortParam != "" {
		contentParams.Set("sort", sortParam)
	}
	if filterCreator != nil {
		contentParams.Set("creator", *filterCreator)
	}
	if filterLicense != nil {
		contentParams.Set("license", *filterLicense)
	}
	if filterDate != nil {
		contentParams.Set("date", *filterDate)
	}
	contentURL := "/" + itemType + "?" + contentParams.Encode()
	return s.renderFullPage(ctx, itemType, q, contentURL, sortParam)
}

// renderFullPage renders the complete layout HTML with the given item type
// pre-selected. Used when /{asset_type} is accessed via direct browser navigation.
func (s *Server) renderFullPage(ctx context.Context, activeType, query, contentURL, sortParam string) (api.RenderItemListResponseObject, error) {
	allEnums := sqlc.AllAssetTypeEnumValues()
	types := make([]string, 0, len(allEnums))
	for _, e := range allEnums {
		types = append(types, string(e))
	}
	slices.Sort(types)

	var buf bytes.Buffer
	if err := s.layoutTpl.Execute(&buf, map[string]any{
		"ItemTypes":      types,
		"ActiveType":     activeType,
		"Query":          query,
		"ContentURL":     contentURL,
		"SortParam":      sortParam,
		"IsAdmin":        isAdmin(ctx),
		"UserName":       userName(ctx),
		"CanUpload":      !s.adminOnlyUpload || isAdmin(ctx),
		"SiteTitle":      s.branding.SiteTitle,
		"SiteSubtitle":   s.branding.SiteSubtitle,
		"HasHeaderImage": s.branding.HeaderImagePath != "",
		"HasFavicon":     s.branding.FaviconPath != "",
		"SourceURL":      s.branding.SourceURL,
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
	Creators     string   // comma-separated, used by the admin edit modal call
	CreatorList  []string // individual creator names for rendering chips
	License      string
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

// activeFilterView describes a single active column filter shown in the filter bar.
type activeFilterView struct {
	Field string // "creator", "license", or "date"
	Label string // human-readable field name
	Value string // the filter value
}

type itemsPageData struct {
	Items         []itemView
	Total         int
	Limit         int
	Offset        int
	PageStart     int
	PageEnd       int
	PrevURL       string
	NextURL       string
	SortParam     string             // raw sort param to preserve in pagination links
	Columns       []sortColumn       // columns for header rendering
	ActiveFilters []activeFilterView // active column filters shown in the filter bar
	IsAdmin       bool               // true when the current user has the "admin" group
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

func toItemView(groupID, assetType, groupName, groupKey, creators, license, variants string, totalSize int64, createdAt time.Time) itemView {
	if creators == "" {
		creators = "unknown"
	}
	// Split the aggregated comma-separated creators string into individual names.
	parts := strings.Split(creators, ", ")
	creatorList := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			creatorList = append(creatorList, p)
		}
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
		CreatorList:  creatorList,
		License:      license,
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
		{"license", "License"},
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

// isAdmin returns true if the request context contains an authenticated user
// who belongs to the "admin" Pocket-ID group.
func isAdmin(ctx context.Context) bool {
	claims := oidcauth.ClaimsFromContext(ctx)
	return claims != nil && claims.HasGroup("admin")
}

// userName returns the display name of the authenticated user, or "".
func userName(ctx context.Context) string {
	claims := oidcauth.ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	if claims.Name != "" {
		return claims.Name
	}
	return claims.PreferredUsername
}
