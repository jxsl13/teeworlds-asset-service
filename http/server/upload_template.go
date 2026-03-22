package server

import (
	"context"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
)

// uploadTemplate handles template image uploads.
// Templates can accept any resolution from any image-based type.
// Grouped by name and resolution.
func (s *Server) uploadTemplate(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	if resp, err := s.validateAndSetResolution(uc); resp != nil || err != nil {
		return resp, err
	}
	return s.persistUpload(ctx, uc)
}
