package server

import (
	"context"
	"slices"

	"github.com/jxsl13/search-service/http/api"
	sqlc "github.com/jxsl13/search-service/sql"
)

// ListItemTypes implements api.StrictServerInterface.
func (s *Server) ListItemTypes(_ context.Context, _ api.ListItemTypesRequestObject) (api.ListItemTypesResponseObject, error) {
	allEnums := sqlc.AllItemTypeEnumValues()
	types := make([]api.ItemType, 0, len(allEnums))
	for _, e := range allEnums {
		types = append(types, api.ItemType(e))
	}
	slices.Sort(types)
	return api.ListItemTypes200JSONResponse{ItemTypes: types}, nil
}
