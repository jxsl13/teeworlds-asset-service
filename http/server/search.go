package server

import (
	"context"
	"fmt"

	"github.com/jxsl13/search-service/http/api"
	"github.com/jxsl13/search-service/model"
)

// SearchItems implements api.StrictServerInterface.
func (s *Server) SearchItems(ctx context.Context, request api.SearchItemsRequestObject) (api.SearchItemsResponseObject, error) {
	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	query, err := model.NewSearchQuery(request.Params.Q, limit, offset)
	if err != nil {
		return api.SearchItems400JSONResponse{Error: err.Error()}, nil
	}

	result, err := s.newService().Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	resp, err := result.ToAPI()
	if err != nil {
		return nil, fmt.Errorf("decode search results: %w", err)
	}
	return api.SearchItems200JSONResponse(resp), nil
}
