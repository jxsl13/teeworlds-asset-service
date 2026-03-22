package server

import (
	"context"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
)

// uploadTheme handles background theme image uploads.
// Grouped by name and resolution.
func (s *Server) uploadTheme(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	if resp, err := s.validateAndSetResolution(uc); resp != nil || err != nil {
		return resp, err
	}
	return s.persistUpload(ctx, uc)
}
