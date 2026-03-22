package server

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
)

// DownloadBundle implements api.StrictServerInterface.
// It streams a ZIP archive containing all variant files of the given group.
func (s *Server) DownloadBundle(ctx context.Context, request api.DownloadBundleRequestObject) (api.DownloadBundleResponseObject, error) {
	itemType := sqlc.AssetTypeEnum(request.AssetType)

	rows, err := s.dao.GetGroupFiles(ctx, sqlc.GetGroupFilesParams{
		GroupID:   uuidToPgtype(request.ItemId),
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

// DownloadMultiBundle implements api.StrictServerInterface.
// It streams a ZIP archive containing all files of the requested asset groups.
func (s *Server) DownloadMultiBundle(ctx context.Context, request api.DownloadMultiBundleRequestObject) (api.DownloadMultiBundleResponseObject, error) {
	if request.Body == nil || len(request.Body.GroupIds) == 0 {
		return api.DownloadMultiBundle400JSONResponse{Error: "group_ids must not be empty"}, nil
	}
	if len(request.Body.GroupIds) > 100 {
		return api.DownloadMultiBundle400JSONResponse{Error: "selected group_ids must not exceed 100 entries"}, nil
	}

	groupIDs := make([]pgtype.UUID, len(request.Body.GroupIds))
	for i, id := range request.Body.GroupIds {
		groupIDs[i] = pgtype.UUID{Bytes: id, Valid: true}
	}

	rows, err := s.dao.GetMultiGroupFiles(ctx, groupIDs)
	if err != nil {
		return nil, fmt.Errorf("get multi group files: %w", err)
	}
	if len(rows) == 0 {
		return api.DownloadMultiBundle404JSONResponse{Error: "none of the requested groups were found"}, nil
	}

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

			entryPath := multiBundleEntryPath(string(row.AssetType), row.GroupName, row.GroupValue, row.OriginalFilename, row.ItemFilePath)

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

	return multiBundleDownload{
		inner: api.DownloadMultiBundle200ApplicationzipResponse{Body: pr},
	}, nil
}

// multiBundleEntryPath builds the ZIP entry path for a multi-group bundle.
// Resolution-based types: {asset_type}/{group_name}/{group_value}/{filename}
// Map type:               {asset_type}/{group_name}/{filename}
func multiBundleEntryPath(assetType, groupName, groupValue, originalFilename, itemFilePath string) string {
	name := originalFilename
	if name == "" {
		name = filepath.Base(itemFilePath)
	}

	parts := []string{sanitizePathComponent(assetType), sanitizePathComponent(groupName)}

	if assetType != string(sqlc.AssetTypeEnumMap) {
		folder := groupValue
		if folder == "" {
			folder = "default"
		}
		parts = append(parts, sanitizePathComponent(folder))
	}

	parts = append(parts, sanitizePathComponent(name))
	return strings.Join(parts, "/")
}

// sanitizePathComponent replaces characters unsafe for ZIP/filesystem paths.
func sanitizePathComponent(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\x00':
			return '_'
		}
		return r
	}, s)
}

type multiBundleDownload struct {
	inner api.DownloadMultiBundle200ApplicationzipResponse
}

func (r multiBundleDownload) VisitDownloadMultiBundleResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Disposition", `attachment; filename="assets.zip"`)
	return r.inner.VisitDownloadMultiBundleResponse(w)
}
