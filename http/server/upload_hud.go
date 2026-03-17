package server

import (
	"context"

	"github.com/jxsl13/asset-service/http/api"
)

// uploadHud handles HUD sprite sheet uploads.
// Grouped by name and resolution.
func (s *Server) uploadHud(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	if resp, err := s.validateAndSetResolution(uc); resp != nil || err != nil {
		return resp, err
	}
	return s.persistUpload(ctx, uc)
}
