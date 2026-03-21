package model

import (
	"github.com/jxsl13/asset-service/http/api"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// SearchResult is the output value object of the search use case.
type SearchResult struct {
	Items []Item
	Total int
}

// SearchResultFromRows converts a slice of sqlc.SearchRows into a domain
// SearchResult. The total is taken from the window-function column on the
// first row (0 when the slice is empty).
func SearchResultFromRows(rows []sqlc.SearchRow) SearchResult {
	total := 0
	if len(rows) > 0 {
		total = int(rows[0].TotalCount)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, ItemFromRow(row))
	}
	return SearchResult{Items: items, Total: total}
}

// SearchResultFromByTypeRows converts a slice of sqlc.SearchByTypeRow into
// a domain SearchResult.
func SearchResultFromByTypeRows(rows []sqlc.SearchByTypeRow) SearchResult {
	total := 0
	if len(rows) > 0 {
		total = int(rows[0].TotalCount)
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, Item{
			GroupID:   row.GroupID,
			AssetType: row.AssetType,
			GroupName: row.GroupName,
			GroupKey:  row.GroupKey,
			Creators:  row.Creators,
			License:   row.License,
			Variants:  row.Variants,
			TotalSize: row.TotalSize,
			CreatedAt: row.CreatedAt,
			Score:     row.Sml,
		})
	}
	return SearchResult{Items: items, Total: total}
}

// ToAPI converts the domain SearchResult into an api.SearchResponse DTO.
func (result SearchResult) ToAPI() api.SearchResponse {
	apiResults := make([]api.SearchResult, 0, len(result.Items))
	for _, item := range result.Items {
		apiResults = append(apiResults, item.ToAPI())
	}
	return api.SearchResponse{
		Results: apiResults,
		Total:   result.Total,
	}
}
