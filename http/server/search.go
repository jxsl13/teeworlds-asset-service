package server

import (
	"context"
	"fmt"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	"github.com/jxsl13/teeworlds-asset-service/model"
)

// SearchItems implements api.StrictServerInterface.
func (s *Server) SearchItems(ctx context.Context, request api.SearchItemsRequestObject) (api.SearchItemsResponseObject, error) {
	limit := s.itemsPerPage
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

	resp := result.ToAPI()
	return api.SearchItems200JSONResponse(resp), nil
}
