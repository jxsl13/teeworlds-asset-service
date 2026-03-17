package server

import (
	"context"

	"github.com/jxsl13/asset-service/http/api"
)

// uploadMap handles map file uploads.
// Maps are binary .map files — no PNG resolution grouping applies.
// Each unique map name is its own group (group_key = "", group_value = "").
func (s *Server) uploadMap(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	if resp, err := s.validateMap(uc); resp != nil || err != nil {
		return resp, err
	}

	// Maps have no resolution grouping.
	uc.groupKey = ""
	uc.groupValue = ""

	return s.persistUpload(ctx, uc)
}
