package server

import (
	"context"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
)

// GetConfig implements api.StrictServerInterface.
func (s *Server) GetConfig(_ context.Context, _ api.GetConfigRequestObject) (api.GetConfigResponseObject, error) {
	return api.GetConfig200JSONResponse{
		ItemsPerPage: s.itemsPerPage,
	}, nil
}
