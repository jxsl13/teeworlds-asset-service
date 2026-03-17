package server

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	"github.com/jxsl13/asset-service/http/api"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// DownloadBundle implements api.StrictServerInterface.
// It streams a ZIP archive containing all variant files of the given group.
func (s *Server) DownloadBundle(ctx context.Context, request api.DownloadBundleRequestObject) (api.DownloadBundleResponseObject, error) {
	itemType := sqlc.AssetTypeEnum(request.AssetType)

	rows, err := s.dao.GetGroupFiles(ctx, sqlc.GetGroupFilesParams{
		GroupID:   request.ItemId,
		AssetType: itemType,
	})
	if err != nil {
		return nil, fmt.Errorf("get group files: %w", err)
	}
	if len(rows) == 0 {
		return api.DownloadBundle404JSONResponse{Error: "group not found or has no items"}, nil
	}

	groupName := rows[0].GroupName

	pr, pw := io.Pipe()
	go func() {
		zw := zip.NewWriter(pw)
		var writeErr error
		for _, row := range rows {
			f, err := s.fsys.Open(filepath.FromSlash(row.ItemFilePath))
			if err != nil {
				writeErr = fmt.Errorf("open %s: %w", row.ItemFilePath, err)
				break
			}

			folder := row.GroupValue
			if folder == "" {
				folder = "default"
			}
			name := row.OriginalFilename
			if name == "" {
				name = filepath.Base(row.ItemFilePath)
			}
			entryPath := folder + "/" + name

			w, err := zw.Create(entryPath)
			if err != nil {
				_ = f.Close()
				writeErr = err
				break
			}
			if _, err := io.Copy(w, f); err != nil {
				_ = f.Close()
				writeErr = err
				break
			}
			_ = f.Close()
		}
		if closeErr := zw.Close(); closeErr != nil && writeErr == nil {
			writeErr = closeErr
		}
		pw.CloseWithError(writeErr)
	}()

	return bundleDownload{
		inner:    api.DownloadBundle200ApplicationzipResponse{Body: pr},
		filename: groupName + ".zip",
	}, nil
}

type bundleDownload struct {
	inner    api.DownloadBundle200ApplicationzipResponse
	filename string
}

func (r bundleDownload) VisitDownloadBundleResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", r.filename))
	return r.inner.VisitDownloadBundleResponse(w)
}
