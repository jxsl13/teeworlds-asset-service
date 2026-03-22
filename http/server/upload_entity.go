package server

import (
	"context"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
)

// uploadEntity handles entity layer image uploads.
// Grouped by name and resolution.
func (s *Server) uploadEntity(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	if resp, err := s.validateAndSetResolution(uc); resp != nil || err != nil {
		return resp, err
	}
	return s.persistUpload(ctx, uc)
}
