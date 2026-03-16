package service

import (
	"context"
	stdsql "database/sql"

	"github.com/jxsl13/asset-service/model"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// SearchService encapsulates the search business logic.
type SearchService struct {
	dao sqlc.DAO
}

// New creates a SearchService with the provided DAO.
func New(dao sqlc.DAO) *SearchService {
	return &SearchService{dao: dao}
}

// Search executes the search use case.
func (s *SearchService) Search(ctx context.Context, query model.SearchQuery) (model.SearchResult, error) {
	if query.ItemType != "" {
		return s.searchByType(ctx, query)
	}
	rows, err := s.dao.Search(ctx, sqlc.SearchParams{
		StrictWordSimilarity: query.Q,
		Limit:                int32(query.Limit),
		Offset:               int32(query.Offset),
	})
	if err != nil {
		return model.SearchResult{}, err
	}
	return model.SearchResultFromRows(rows), nil
}

func (s *SearchService) searchByType(ctx context.Context, query model.SearchQuery) (model.SearchResult, error) {
	rows, err := s.dao.SearchByType(ctx, sqlc.SearchByTypeParams{
		StrictWordSimilarity: query.Q,
		Limit:                int32(query.Limit),
		Offset:               int32(query.Offset),
		ItemType:             sqlc.ItemTypeEnum(query.ItemType),
	})
	if err != nil {
		return model.SearchResult{}, err
	}
	return model.SearchResultFromByTypeRows(rows), nil
}

// ListItems executes the list items use case.
func (s *SearchService) ListItems(ctx context.Context, query model.ListQuery) (model.ListResult, error) {
	rows, err := s.dao.ListItems(ctx, sqlc.ListItemsParams{
		ItemType:      sqlc.ItemTypeEnum(query.ItemType),
		Limit:         int32(query.Limit),
		Offset:        int32(query.Offset),
		FilterName:    toNullString(query.FilterName),
		FilterCreator: toNullString(query.FilterCreator),
		FilterLicense: toNullString(query.FilterLicense),
		SortField:     query.SortField,
		SortDesc:      query.SortDesc,
	})
	if err != nil {
		return model.ListResult{}, err
	}
	return model.ListResultFromRows(rows), nil
}

func toNullString(s *string) stdsql.NullString {
	if s == nil {
		return stdsql.NullString{}
	}
	return stdsql.NullString{String: *s, Valid: true}
}
