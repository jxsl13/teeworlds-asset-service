package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jxsl13/teeworlds-asset-service/http/api"
	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
)

// resolveThumbnail looks up the thumbnail path and checksum for an item,
// falling back to the group's smallest item if no direct match is found.
// Returns ("", "", nil) when no thumbnail exists.
func (s *Server) resolveThumbnail(ctx context.Context, itemID uuid.UUID, assetType sqlc.AssetTypeEnum) (thumbPath, checksum string, err error) {
	thumb, err := s.dao.GetItemThumbnailPath(ctx, sqlc.GetItemThumbnailPathParams{
		ItemID:    uuidToPgtype(itemID),
		AssetType: assetType,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("get thumbnail path: %w", err)
	}

	if err == nil && thumb.ItemThumbnailPath != nil && *thumb.ItemThumbnailPath != "" {
		return *thumb.ItemThumbnailPath, thumb.ThumbnailChecksum, nil
	}

	// Fallback: treat the ID as a group_id.
	group, err := s.dao.GetGroupThumbnailPath(ctx, uuidToPgtype(itemID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("get group thumbnail path: %w", err)
	}

	if group.ItemThumbnailPath == nil || *group.ItemThumbnailPath == "" {
		return "", "", nil
	}
	return *group.ItemThumbnailPath, group.ThumbnailChecksum, nil
}

// DownloadThumbnail implements api.StrictServerInterface.
// It first tries to find a thumbnail by item_id. If the item_id doesn't match
// a search_item row, it falls back to treating the ID as a group_id and serves
// the smallest item's thumbnail from that group.
func (s *Server) DownloadThumbnail(ctx context.Context, request api.DownloadThumbnailRequestObject) (api.DownloadThumbnailResponseObject, error) {
	thumbPath, checksum, err := s.resolveThumbnail(ctx, request.ItemId, sqlc.AssetTypeEnum(request.AssetType))
	if err != nil {
		return nil, err
	}
	if thumbPath == "" {
		return api.DownloadThumbnail404JSONResponse{Error: "thumbnail not found"}, nil
	}

	// Use the file's checksum from the database as the ETag.
	etag := `"` + checksum + `"`

	// If the client already has this version, return 304.
	if request.Params.IfNoneMatch != nil && etagMatch(*request.Params.IfNoneMatch, etag) {
		return notModifiedResponse{etag: etag}, nil
	}

	f, err := s.fsys.Open(filepath.FromSlash(thumbPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.DownloadThumbnail404JSONResponse{Error: "thumbnail file not found"}, nil
		}
		return nil, fmt.Errorf("open thumbnail file: %w", err)
	}

	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat thumbnail file: %w", err)
	}

	return api.DownloadThumbnail200ImagewebpResponse{
		Body:          f,
		ContentLength: stat.Size(),
		Headers: api.DownloadThumbnail200ResponseHeaders{
			CacheControl: "public, max-age=31536000, immutable",
			ETag:         etag,
		},
	}, nil
}

// HeadThumbnail implements api.StrictServerInterface.
// Returns the same headers as DownloadThumbnail (ETag, Cache-Control,
// Content-Type, Content-Length) without streaming the body.
func (s *Server) HeadThumbnail(ctx context.Context, request api.HeadThumbnailRequestObject) (api.HeadThumbnailResponseObject, error) {
	thumbPath, checksum, err := s.resolveThumbnail(ctx, request.ItemId, sqlc.AssetTypeEnum(request.AssetType))
	if err != nil {
		return nil, err
	}
	if thumbPath == "" {
		return api.HeadThumbnail404JSONResponse{Error: "thumbnail not found"}, nil
	}

	// Use the file's checksum from the database as the ETag.
	etag := `"` + checksum + `"`

	// If the client already has this version, return 304.
	if request.Params.IfNoneMatch != nil && etagMatch(*request.Params.IfNoneMatch, etag) {
		return notModifiedResponse{etag: etag}, nil
	}

	f, err := s.fsys.Open(filepath.FromSlash(thumbPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return api.HeadThumbnail404JSONResponse{Error: "thumbnail file not found"}, nil
		}
		return nil, fmt.Errorf("open thumbnail file: %w", err)
	}

	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat thumbnail file: %w", err)
	}
	_ = f.Close()

	return headThumbnailResponse{
		contentLength: int(stat.Size()),
		etag:          etag,
	}, nil
}

// headThumbnailResponse writes caching headers for HEAD requests.
type headThumbnailResponse struct {
	contentLength int
	etag          string
}

func (r headThumbnailResponse) VisitHeadThumbnailResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", r.etag)
	w.Header().Set("Content-Type", "image/webp")
	w.Header().Set("Content-Length", fmt.Sprint(r.contentLength))
	w.WriteHeader(http.StatusOK)
	return nil
}
