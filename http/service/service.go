package service

import (
	"context"

	"github.com/jxsl13/teeworlds-asset-service/model"
	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
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
	primary := query.PrimarySort()
	rows, err := s.dao.SearchByType(ctx, sqlc.SearchByTypeParams{
		StrictWordSimilarity: query.Q,
		Limit:                int32(query.Limit),
		Offset:               int32(query.Offset),
		AssetType:            sqlc.AssetTypeEnum(query.ItemType),
		SortField:            primary.Field,
		SortDesc:             primary.Desc,
	})
	if err != nil {
		return model.SearchResult{}, err
	}
	return model.SearchResultFromByTypeRows(rows), nil
}

// ListItems executes the list items use case.
func (s *SearchService) ListItems(ctx context.Context, query model.ListQuery) (model.ListResult, error) {
	primary := query.PrimarySort()
	secondary := query.SecondarySort()
	rows, err := s.dao.ListItems(ctx, sqlc.ListItemsParams{
		AssetType:     sqlc.AssetTypeEnum(query.ItemType),
		Limit:         int32(query.Limit),
		Offset:        int32(query.Offset),
		FilterName:    query.FilterName,
		FilterCreator: query.FilterCreator,
		FilterLicense: query.FilterLicense,
		FilterDate:    query.FilterDate,
		SortField:     primary.Field,
		SortDesc:      primary.Desc,
		SortField2:    secondary.Field,
		SortDesc2:     secondary.Desc,
	})
	if err != nil {
		return model.ListResult{}, err
	}
	return model.ListResultFromRows(rows), nil
}
