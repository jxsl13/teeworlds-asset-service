package server

import (
	"context"
	stdsql "database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jxsl13/asset-service/http/api"
	sqlc "github.com/jxsl13/asset-service/sql"
)

// DownloadThumbnail implements api.StrictServerInterface.
// It first tries to find a thumbnail by item_id. If the item_id doesn't match
// a search_item row, it falls back to treating the ID as a group_id and serves
// the smallest item's thumbnail from that group.
func (s *Server) DownloadThumbnail(ctx context.Context, request api.DownloadThumbnailRequestObject) (api.DownloadThumbnailResponseObject, error) {
	itemType := sqlc.AssetTypeEnum(request.AssetType)

	// Try item-level lookup first.
	thumbPath, err := s.dao.GetItemThumbnailPath(ctx, sqlc.GetItemThumbnailPathParams{
		ItemID:    request.ItemId,
		AssetType: itemType,
	})
	if err != nil && !errors.Is(err, stdsql.ErrNoRows) {
		return nil, fmt.Errorf("get thumbnail path: %w", err)
	}

	// Fallback: treat the ID as a group_id.
	if errors.Is(err, stdsql.ErrNoRows) || !thumbPath.Valid || thumbPath.String == "" {
		thumbPath, err = s.dao.GetGroupThumbnailPath(ctx, request.ItemId)
		if err != nil {
			if errors.Is(err, stdsql.ErrNoRows) {
				return api.DownloadThumbnail404JSONResponse{Error: "thumbnail not found"}, nil
			}
			return nil, fmt.Errorf("get group thumbnail path: %w", err)
		}
	}

	if !thumbPath.Valid || thumbPath.String == "" {
		return api.DownloadThumbnail404JSONResponse{Error: "thumbnail not found"}, nil
	}

	// Derive a stable ETag from the path (contains a UUID, so it's unique and immutable).
	etag := `"` + strings.TrimSuffix(path.Base(thumbPath.String), path.Ext(thumbPath.String)) + `"`

	// If the client already has this version, return 304.
	if request.Params.IfNoneMatch != nil && etagMatch(*request.Params.IfNoneMatch, etag) {
		return notModifiedResponse{etag: etag}, nil
	}

	f, err := s.fsys.Open(filepath.FromSlash(thumbPath.String))
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

	return cachedThumbnailResponse{
		inner: api.DownloadThumbnail200ImagepngResponse{
			Body:          f,
			ContentLength: stat.Size(),
		},
		etag: etag,
	}, nil
}

// cachedThumbnailResponse wraps a thumbnail response with caching headers.
type cachedThumbnailResponse struct {
	inner api.DownloadThumbnail200ImagepngResponse
	etag  string
}

func (r cachedThumbnailResponse) VisitDownloadThumbnailResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", r.etag)
	return r.inner.VisitDownloadThumbnailResponse(w)
}
