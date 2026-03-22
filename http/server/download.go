package server

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
)

// DownloadItem implements api.StrictServerInterface.
// It first tries to find the file by item_id. If not found, it falls back
// to treating the ID as a group_id and serves the smallest item's file.
func (s *Server) DownloadItem(ctx context.Context, request api.DownloadItemRequestObject) (api.DownloadItemResponseObject, error) {
	itemType := sqlc.AssetTypeEnum(request.AssetType)

	row, err := s.dao.GetItemFilePath(ctx, sqlc.GetItemFilePathParams{
		ItemID:    request.ItemId,
		AssetType: itemType,
	})
	if err != nil && !errors.Is(err, stdsql.ErrNoRows) {
		return nil, fmt.Errorf("get item file path: %w", err)
	}

	// Fallback: treat the ID as a group_id.
	if errors.Is(err, stdsql.ErrNoRows) {
		groupRow, groupErr := s.dao.GetGroupFilePath(ctx, sqlc.GetGroupFilePathParams{
			GroupID:   request.ItemId,
			AssetType: itemType,
		})
		if groupErr != nil {
			if errors.Is(groupErr, stdsql.ErrNoRows) {
				return api.DownloadItem404JSONResponse{Error: "item not found"}, nil
			}
			return nil, fmt.Errorf("get group file path: %w", groupErr)
		}
		row.ItemFilePath = groupRow.ItemFilePath
		row.OriginalFilename = groupRow.OriginalFilename
	}

	f, err := s.fsys.Open(filepath.FromSlash(row.ItemFilePath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.DownloadItem404JSONResponse{Error: "file not found"}, nil
		}
		return nil, fmt.Errorf("open item file: %w", err)
	}

	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat item file: %w", err)
	}

	resp := api.DownloadItem200ApplicationoctetStreamResponse{
		Body:          f,
		ContentLength: stat.Size(),
	}

	if row.OriginalFilename != "" {
		return downloadWithFilename{inner: resp, filename: row.OriginalFilename}, nil
	}
	return resp, nil
}

// downloadWithFilename wraps a download response and adds a Content-Disposition header.
type downloadWithFilename struct {
	inner    api.DownloadItem200ApplicationoctetStreamResponse
	filename string
}

func (r downloadWithFilename) VisitDownloadItemResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Disposition", "attachment; filename=\""+r.filename+"\"")
	return r.inner.VisitDownloadItemResponse(w)
}
