package server

import (
	"context"

	"github.com/jxsl13/asset-service/http/api"
)

// uploadSkin handles Tee skin uploads.
// Skins are grouped by name and resolution (group_key = "resolution",
// group_value = "WxH"). Multiple resolutions of the same skin share a group
// name but each resolution is a separate item in the group.
// The DB constraint (group_id, group_value) UNIQUE prevents duplicate
// resolutions within the same group.
func (s *Server) uploadSkin(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	if resp, err := s.validateAndSetResolution(uc); resp != nil || err != nil {
		return resp, err
	}
	return s.persistUpload(ctx, uc)
}
