package server

import (
	"context"
	"fmt"

	"github.com/jxsl13/asset-service/http/api"
	"github.com/jxsl13/asset-service/model"
)

// ListItems implements api.StrictServerInterface.
func (s *Server) ListItems(ctx context.Context, request api.ListItemsRequestObject) (api.ListItemsResponseObject, error) {
	limit := 20
	if request.Params.Limit != nil {
		limit = *request.Params.Limit
	}
	offset := 0
	if request.Params.Offset != nil {
		offset = *request.Params.Offset
	}

	sortField := "name"
	if request.Params.Sort != nil {
		sortField = string(*request.Params.Sort)
	}
	sortDesc := false
	if request.Params.Order != nil && *request.Params.Order == api.Desc {
		sortDesc = true
	}

	var license *string
	if request.Params.License != nil {
		s := string(*request.Params.License)
		license = &s
	}

	query, err := model.NewListQuery(
		string(request.AssetType),
		limit,
		offset,
		request.Params.Name,
		request.Params.Creator,
		license,
		[]model.SortDirective{{Field: sortField, Desc: sortDesc}},
	)
	if err != nil {
		return api.ListItems400JSONResponse{Error: err.Error()}, nil
	}

	result, err := s.newService().ListItems(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	resp := result.ToAPI()
	return api.ListItems200JSONResponse(resp), nil
}
