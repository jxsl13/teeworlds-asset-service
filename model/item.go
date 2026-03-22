package model

import (
	"time"

	"github.com/google/uuid"
	oapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
)

// Item is the domain entity returned from a search.
type Item struct {
	GroupID   uuid.UUID
	AssetType sqlc.AssetTypeEnum
	GroupName string
	GroupKey  string
	Creators  string
	License   string
	Variants  string // comma-separated "uuid:value" pairs
	TotalSize int64  // sum of all variant file sizes in bytes
	CreatedAt time.Time
	Score     float64
}

// ItemFromRow converts a sqlc.SearchRow (DB layer) into a domain Item.
func ItemFromRow(row sqlc.SearchRow) Item {
	return Item{
		GroupID:   uuid.UUID(row.GroupID.Bytes),
		AssetType: row.AssetType,
		GroupName: row.GroupName,
		GroupKey:  row.GroupKey,
		Creators:  row.Creators,
		License:   row.License,
		Variants:  row.Variants,
		Score:     row.Sml,
	}
}

// ToAPI converts the domain Item into an api.SearchResult DTO.
func (item Item) ToAPI() api.SearchResult {
	return api.SearchResult{
		ItemId:    oapitypes.UUID(item.GroupID),
		AssetType: api.ItemType(item.AssetType),
		Score:     item.Score,
		ItemValue: map[string]interface{}{
			"name":     item.GroupName,
			"creators": item.Creators,
			"license":  item.License,
		},
	}
}
