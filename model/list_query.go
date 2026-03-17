package model

import (
	"fmt"
	"strings"
)

// SortDirective represents a single column sort instruction.
type SortDirective struct {
	Field string // "name" or "created_at"
	Desc  bool
}

// ListQuery is the validated input value object for the list items use case.
type ListQuery struct {
	ItemType      string
	Limit         int
	Offset        int
	FilterName    *string
	FilterCreator *string
	FilterLicense *string
	Sort          []SortDirective // max 2 directives
}

var validSortFields = map[string]bool{
	"name":       true,
	"creators":   true,
	"size":       true,
	"created_at": true,
}

// ParseSortDirectives parses a comma-separated sort string
// like "name:asc,created_at:desc" into validated SortDirective slices.
func ParseSortDirectives(raw string) []SortDirective {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]SortDirective, 0, len(parts))
	seen := make(map[string]bool)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		field, dir, _ := strings.Cut(p, ":")
		field = strings.TrimSpace(field)
		dir = strings.TrimSpace(strings.ToLower(dir))
		if !validSortFields[field] || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, SortDirective{
			Field: field,
			Desc:  dir == "desc",
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

// NewListQuery validates and constructs a ListQuery.
func NewListQuery(itemType string, limit, offset int, name, creator, license *string, sort []SortDirective) (ListQuery, error) {
	if itemType == "" {
		return ListQuery{}, fmt.Errorf("asset_type must not be empty")
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
	if len(sort) == 0 {
		sort = []SortDirective{{Field: "name", Desc: false}}
	}
	return ListQuery{
		ItemType:      itemType,
		Limit:         limit,
		Offset:        offset,
		FilterName:    name,
		FilterCreator: creator,
		FilterLicense: license,
		Sort:          sort,
	}, nil
}

// PrimarySort returns the first sort directive.
func (q ListQuery) PrimarySort() SortDirective {
	if len(q.Sort) > 0 {
		return q.Sort[0]
	}
	return SortDirective{Field: "name"}
}

// SecondarySort returns the second sort directive, if any.
func (q ListQuery) SecondarySort() SortDirective {
	if len(q.Sort) > 1 {
		return q.Sort[1]
	}
	return SortDirective{}
}
