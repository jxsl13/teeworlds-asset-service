package server

import (
	"context"
	"fmt"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	"github.com/jxsl13/teeworlds-asset-service/model"
)

// SearchItemsByType implements api.StrictServerInterface.
func (s *Server) SearchItemsByType(ctx context.Context, request api.SearchItemsByTypeRequestObject) (api.SearchItemsByTypeResponseObject, error) {
	limit := s.itemsPerPage
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	query, err := model.NewSearchQueryByType(request.Params.Q, string(request.AssetType), limit, offset, nil)
	if err != nil {
		return api.SearchItemsByType400JSONResponse{Error: err.Error()}, nil
	}

	result, err := s.newService().Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search by type: %w", err)
	}

	resp := result.ToAPI()
	return api.SearchItemsByType200JSONResponse(resp), nil
}
