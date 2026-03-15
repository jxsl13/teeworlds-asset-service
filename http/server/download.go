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

// DownloadItem implements api.StrictServerInterface.
func (s *Server) DownloadItem(ctx context.Context, request api.DownloadItemRequestObject) (api.DownloadItemResponseObject, error) {
	itemType := sqlc.ItemTypeEnum(request.ItemType)

	relPath, err := s.dao.GetItemFilePath(ctx, sqlc.GetItemFilePathParams{
		ItemID:   request.ItemId,
		ItemType: itemType,
	})
	if err != nil {
		if errors.Is(err, stdsql.ErrNoRows) {
			return api.DownloadItem404JSONResponse{Error: "item not found"}, nil
		}
		return api.DownloadItem500JSONResponse{Error: "internal server error"}, nil
	}

	f, err := s.fsys.Open(filepath.FromSlash(relPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.DownloadItem404JSONResponse{Error: "file not found"}, nil
		}
		return api.DownloadItem500JSONResponse{Error: "internal server error"}, nil
	}

	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return api.DownloadItem500JSONResponse{Error: "internal server error"}, nil
	}

	return api.DownloadItem200ApplicationoctetStreamResponse{
		Body:          f,
		ContentLength: stat.Size(),
	}, nil
}
