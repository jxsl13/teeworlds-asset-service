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
			ItemID:    row.ItemID,
			ItemType:  row.ItemType,
			Score:     row.Sml,
			ItemValue: row.ItemValue,
		})
	}
	return SearchResult{Items: items, Total: total}
}

// ToAPI converts the domain SearchResult into an api.SearchResponse DTO.
// Returns the first decode error encountered, if any.
func (result SearchResult) ToAPI() (api.SearchResponse, error) {
	apiResults := make([]api.SearchResult, 0, len(result.Items))
	for _, item := range result.Items {
		r, err := item.ToAPI()
		if err != nil {
			return api.SearchResponse{}, err
		}
		apiResults = append(apiResults, r)
	}
	return api.SearchResponse{
		Results: apiResults,
		Total:   result.Total,
	}, nil
}
