package model

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	oapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/jxsl13/asset-service/http/api"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// Item is the domain entity returned from a search.
type Item struct {
	ItemID    uuid.UUID
	ItemType  sqlc.ItemTypeEnum
	Score     float64
	ItemValue json.RawMessage
}

// ItemFromRow converts a sqlc.SearchRow (DB layer) into a domain Item.
func ItemFromRow(row sqlc.SearchRow) Item {
	return Item{
		ItemID:    row.ItemID,
		ItemType:  row.ItemType,
		Score:     row.Sml,
		ItemValue: row.ItemValue,
	}
}

// ToAPI converts the domain Item into an api.SearchResult DTO.
// Returns an error when the stored JSON payload cannot be decoded.
func (item Item) ToAPI() (api.SearchResult, error) {
	var itemValue map[string]interface{}
	if err := json.Unmarshal(item.ItemValue, &itemValue); err != nil {
		return api.SearchResult{}, fmt.Errorf("decode item %s: %w", item.ItemID, err)
	}
	return api.SearchResult{
		ItemId:    oapitypes.UUID(item.ItemID),
		ItemType:  api.ItemType(item.ItemType),
		Score:     item.Score,
		ItemValue: itemValue,
	}, nil
}
