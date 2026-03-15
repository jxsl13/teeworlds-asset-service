package model

import "fmt"

// ListQuery is the validated input value object for the list items use case.
type ListQuery struct {
	ItemType      string
	Limit         int
	Offset        int
	FilterName    *string
	FilterCreator *string
	FilterLicense *string
	SortField     string // "name" or "created_at"
	SortDesc      bool
}

// NewListQuery validates and constructs a ListQuery.
func NewListQuery(itemType string, limit, offset int, name, creator, license *string, sortField string, sortDesc bool) (ListQuery, error) {
	if itemType == "" {
		return ListQuery{}, fmt.Errorf("item_type must not be empty")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	switch sortField {
	case "name", "created_at":
		// valid
	default:
		sortField = "name"
	}
	return ListQuery{
		ItemType:      itemType,
		Limit:         limit,
		Offset:        offset,
		FilterName:    name,
		FilterCreator: creator,
		FilterLicense: license,
		SortField:     sortField,
		SortDesc:      sortDesc,
	}, nil
}
