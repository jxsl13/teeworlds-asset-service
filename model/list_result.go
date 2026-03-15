package model

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	oapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/jxsl13/search-service/http/api"
	sqlc "github.com/jxsl13/search-service/sql"
)

// ListResult is the output value object of the list items use case.
type ListResult struct {
	Items []ListItem
	Total int
}

// ListItem is a single item in a list result (no score).
type ListItem struct {
	ItemID    uuid.UUID
	ItemType  sqlc.ItemTypeEnum
	ItemValue json.RawMessage
}

// ListResultFromRows converts sqlc rows into a domain ListResult.
func ListResultFromRows(rows []sqlc.ListItemsRow) ListResult {
	total := 0
	if len(rows) > 0 {
		total = int(rows[0].TotalCount)
	}
	items := make([]ListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, ListItem{
			ItemID:    row.ItemID,
			ItemType:  row.ItemType,
			ItemValue: row.ItemValue,
		})
	}
	return ListResult{Items: items, Total: total}
}

// ToAPI converts the domain ListResult into an api.ListItemsResponse DTO.
func (result ListResult) ToAPI() (api.ListItemsResponse, error) {
	apiResults := make([]api.ListItem, 0, len(result.Items))
	for _, item := range result.Items {
		var itemValue map[string]interface{}
		if err := json.Unmarshal(item.ItemValue, &itemValue); err != nil {
			return api.ListItemsResponse{}, fmt.Errorf("decode item %s: %w", item.ItemID, err)
		}
		apiResults = append(apiResults, api.ListItem{
			ItemId:    oapitypes.UUID(item.ItemID),
			ItemType:  api.ItemType(item.ItemType),
			ItemValue: itemValue,
		})
	}
	return api.ListItemsResponse{
		Results: apiResults,
		Total:   result.Total,
	}, nil
}
