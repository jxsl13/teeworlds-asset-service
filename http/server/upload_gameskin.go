package server

import (
	"context"

	"github.com/jxsl13/asset-service/http/api"
)

// uploadGameskin handles gameskin (game skin) sprite sheet uploads.
// Grouped by name and resolution like skins.
func (s *Server) uploadGameskin(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	if resp, err := s.validateAndSetResolution(uc); resp != nil || err != nil {
		return resp, err
	}
	return s.persistUpload(ctx, uc)
}
