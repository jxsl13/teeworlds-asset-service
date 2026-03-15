package server

import (
	"context"
	stdsql "database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/jxsl13/search-service/http/api"
	sqlc "github.com/jxsl13/search-service/sql"
)

// DownloadThumbnail implements api.StrictServerInterface.
func (s *Server) DownloadThumbnail(ctx context.Context, request api.DownloadThumbnailRequestObject) (api.DownloadThumbnailResponseObject, error) {
	itemType := sqlc.ItemTypeEnum(request.ItemType)

	thumbPath, err := s.dao.GetItemThumbnailPath(ctx, sqlc.GetItemThumbnailPathParams{
		ItemID:   request.ItemId,
		ItemType: itemType,
	})
	if err != nil {
		if errors.Is(err, stdsql.ErrNoRows) {
			return api.DownloadThumbnail404JSONResponse{Error: "thumbnail not found"}, nil
		}
		return api.DownloadThumbnail500JSONResponse{Error: "internal server error"}, nil
	}
	if !thumbPath.Valid || thumbPath.String == "" {
		return api.DownloadThumbnail404JSONResponse{Error: "thumbnail not found"}, nil
	}

	f, err := s.fsys.Open(filepath.FromSlash(thumbPath.String))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.DownloadThumbnail404JSONResponse{Error: "thumbnail file not found"}, nil
		}
		return api.DownloadThumbnail500JSONResponse{Error: "internal server error"}, nil
	}

	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return api.DownloadThumbnail500JSONResponse{Error: "internal server error"}, nil
	}

	return api.DownloadThumbnail200ImagepngResponse{
		Body:          f,
		ContentLength: stat.Size(),
	}, nil
}
