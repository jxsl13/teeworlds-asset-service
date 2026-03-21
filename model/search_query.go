package model

import "fmt"

// SearchQuery is the validated input value object for the search use case.
type SearchQuery struct {
	Q        string
	ItemType string // empty = all types
	Limit    int
	Offset   int
	Sort     []SortDirective
}

// PrimarySort returns the first sort directive, if any.
func (q SearchQuery) PrimarySort() SortDirective {
	if len(q.Sort) > 0 {
		return q.Sort[0]
	}
	return SortDirective{}
}

// NewSearchQuery validates and constructs a SearchQuery.
func NewSearchQuery(q string, limit, offset int) (SearchQuery, error) {
	if q == "" {
		return SearchQuery{}, fmt.Errorf("query must not be empty")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return SearchQuery{Q: q, Limit: limit, Offset: offset}, nil
}

// NewSearchQueryByType validates and constructs a SearchQuery scoped to a specific item type.
func NewSearchQueryByType(q, itemType string, limit, offset int, sort []SortDirective) (SearchQuery, error) {
	if q == "" {
		return SearchQuery{}, fmt.Errorf("query must not be empty")
	}
	if itemType == "" {
		return SearchQuery{}, fmt.Errorf("asset_type must not be empty")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return SearchQuery{Q: q, ItemType: itemType, Limit: limit, Offset: offset, Sort: sort}, nil
}
