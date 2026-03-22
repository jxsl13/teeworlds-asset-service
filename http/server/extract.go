package server

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"github.com/jackc/pgx/v5"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	"github.com/jxsl13/teeworlds-asset-service/internal/twmap"
	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
)

// ExtractMapImages implements api.StrictServerInterface.
func (s *Server) ExtractMapImages(ctx context.Context, request api.ExtractMapImagesRequestObject) (api.ExtractMapImagesResponseObject, error) {
	row, err := s.dao.GetItemFilePath(ctx, sqlc.GetItemFilePathParams{
		ItemID:    uuidToPgtype(request.ItemId),
		AssetType: sqlc.AssetTypeEnumMap,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get map file path: %w", err)
	}

	// Fallback: treat the ID as a group_id.
	if errors.Is(err, pgx.ErrNoRows) {
		groupRow, groupErr := s.dao.GetGroupFilePath(ctx, sqlc.GetGroupFilePathParams{
			GroupID:   uuidToPgtype(request.ItemId),
			AssetType: sqlc.AssetTypeEnumMap,
		})
		if groupErr != nil {
			if errors.Is(groupErr, pgx.ErrNoRows) {
				return api.ExtractMapImages404JSONResponse{Error: "map not found"}, nil
			}
			return nil, fmt.Errorf("get group file path: %w", groupErr)
		}
		row.ItemFilePath = groupRow.ItemFilePath
	}

	f, err := s.fsys.Open(filepath.FromSlash(row.ItemFilePath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.ExtractMapImages404JSONResponse{Error: "map file not found"}, nil
		}
		return nil, fmt.Errorf("open map file: %w", err)
	}

	m, err := twmap.Parse(f)
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("parse map: %w", err)
	}

	// Filter to embedded images only.
	var images []twmap.Image
	for _, img := range m.Images {
		if !img.External && img.RGBA != nil {
			images = append(images, img)
		}
	}
	if len(images) == 0 {
		return api.ExtractMapImages404JSONResponse{Error: "map contains no embedded images"}, nil
	}

	pr, pw := io.Pipe()

	go func() {
		zw := zip.NewWriter(pw)
		var writeErr error
		for i, img := range images {
			name := img.Name
			if name == "" {
				name = fmt.Sprintf("image_%d", i)
			}
			name += ".png"

			w, err := zw.Create(name)
			if err != nil {
				writeErr = err
				break
			}
			if err := png.Encode(w, img.RGBA); err != nil {
				writeErr = err
				break
			}
		}
		if closeErr := zw.Close(); closeErr != nil && writeErr == nil {
			writeErr = closeErr
		}
		pw.CloseWithError(writeErr)
	}()

	return api.ExtractMapImages200ApplicationzipResponse{
		Body: pr,
	}, nil
}
