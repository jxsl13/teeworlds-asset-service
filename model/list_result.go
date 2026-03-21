package model

import (
	"time"

	"github.com/google/uuid"
	oapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/jxsl13/asset-service/http/api"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// ListResult is the output value object of the list items use case.
type ListResult struct {
	Items []ListItem
	Total int
}

// ListItem is a single item in a list result (no score).
type ListItem struct {
	GroupID   uuid.UUID
	AssetType sqlc.AssetTypeEnum
	GroupName string
	GroupKey  string
	Creators  string
	License   string
	Variants  string // comma-separated "uuid:value" pairs
	TotalSize int64  // sum of all variant file sizes in bytes
	CreatedAt time.Time
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
			GroupID:   row.GroupID,
			AssetType: row.AssetType,
			GroupName: row.GroupName,
			GroupKey:  row.GroupKey,
			Creators:  row.Creators,
			License:   row.License,
			Variants:  row.Variants,
			TotalSize: row.TotalSize,
			CreatedAt: row.CreatedAt,
		})
	}
	return ListResult{Items: items, Total: total}
}

// ToAPI converts the domain ListResult into an api.ListItemsResponse DTO.
func (result ListResult) ToAPI() api.ListItemsResponse {
	apiResults := make([]api.ListItem, 0, len(result.Items))
	for _, item := range result.Items {
		apiResults = append(apiResults, api.ListItem{
			ItemId:    oapitypes.UUID(item.GroupID),
			AssetType: api.ItemType(item.AssetType),
			ItemValue: map[string]interface{}{
				"name":     item.GroupName,
				"creators": item.Creators,
				"license":  item.License,
			},
		})
	}
	return api.ListItemsResponse{
		Results: apiResults,
		Total:   result.Total,
	}
}
