package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/jxsl13/asset-service/http/api"
	"github.com/jxsl13/asset-service/model"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// RenderUI implements api.StrictServerInterface.
// It returns the full HTML page with tabs and the HTMX search bar.
func (s *Server) RenderUI(ctx context.Context, _ api.RenderUIRequestObject) (api.RenderUIResponseObject, error) {
	allEnums := sqlc.AllItemTypeEnumValues()
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

	itemType := string(request.ItemType)

	var (
		views []itemView
		total int
	)

	svc := s.newService()

	if q != "" {
		query, err := model.NewSearchQueryByType(q, itemType, limit, offset)
		if err != nil {
			return api.RenderItemList400JSONResponse{Error: err.Error()}, nil
		}
		result, err := svc.Search(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("search items: %w", err)
		}
		total = result.Total
		for _, item := range result.Items {
			views = append(views, toItemView(item.ItemID.String(), string(item.ItemType), item.ItemValue))
		}
	} else {
		query, err := model.NewListQuery(itemType, limit, offset, nil, nil, nil, "name", false)
		if err != nil {
			return api.RenderItemList400JSONResponse{Error: err.Error()}, nil
		}
		result, err := svc.ListItems(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("list items: %w", err)
		}
		total = result.Total
		for _, item := range result.Items {
			views = append(views, toItemView(item.ItemID.String(), string(item.ItemType), item.ItemValue))
		}
	}

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

	data := itemsPageData{
		Items:     views,
		Total:     total,
		Limit:     limit,
		Offset:    offset,
		PageStart: offset + 1,
		PageEnd:   pageEnd,
		PrevURL:   fmt.Sprintf("%s?limit=%d&offset=%d%s", baseURL, limit, prevOffset, qParam),
		NextURL:   fmt.Sprintf("%s?limit=%d&offset=%d%s", baseURL, limit, offset+limit, qParam),
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
		if q != "" || offset > 0 {
			pushURL = fmt.Sprintf("%s?limit=%d&offset=%d%s", baseURL, limit, offset, qParam)
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
// pre-selected. Used when /{item_type} is accessed via direct browser navigation.
func (s *Server) renderFullPage(activeType, query string) (api.RenderItemListResponseObject, error) {
	allEnums := sqlc.AllItemTypeEnumValues()
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

type itemView struct {
	ItemID       string
	ItemType     string
	Name         string
	Creators     string
	HasThumbnail bool
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
}

func toItemView(id, itemType string, raw json.RawMessage) itemView {
	var val struct {
		Name     string   `json:"name"`
		Creators []string `json:"creators"`
	}
	json.Unmarshal(raw, &val)
	creators := strings.Join(val.Creators, ", ")
	if creators == "" {
		creators = "unknown"
	}
	return itemView{
		ItemID:       id,
		ItemType:     itemType,
		Name:         val.Name,
		Creators:     creators,
		HasThumbnail: true,
	}
}
