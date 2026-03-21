package server

import (
	"context"
	"crypto/sha256"
	stdsql "database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jxsl13/asset-service/http/api"
	"github.com/jxsl13/asset-service/http/server/middleware/clientip"
	"github.com/jxsl13/asset-service/internal/twmap"
	"github.com/jxsl13/asset-service/internal/twskin"
	sqlc "github.com/jxsl13/asset-service/sql"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/image/draw"
)

// uploadContext bundles every piece of state accumulated during a single upload.
type uploadContext struct {
	itemID   uuid.UUID
	groupID  uuid.UUID
	itemType api.ItemType
	meta     api.ItemMetadata

	groupKey   string // e.g. "resolution"
	groupValue string // e.g. "256x128"

	ext              string // file extension including dot
	originalFilename string // original filename from the upload
	size             int64
	checksum         string

	relPath       string            // permanent storage path for the item file
	absPath       string            // absolute OS path for the item file
	thumbnailPath stdsql.NullString // DB value for item_thumbnail_path
	hasTempThumb  bool              // true when a separate _thumb.png was created in tmpDir
}

// tmpThumbName returns the temp file name used for a generated thumbnail.
func (u *uploadContext) tmpThumbName() string {
	return u.itemID.String() + "_thumb.png"
}

func (u *uploadContext) tmpName() string {
	return u.itemID.String() + u.ext
}

// UploadItem implements api.StrictServerInterface.
// It dispatches to a per-asset-type upload function so that each type's
// grouping, validation and thumbnail logic is explicit and self-contained.
func (s *Server) UploadItem(ctx context.Context, request api.UploadItemRequestObject) (api.UploadItemResponseObject, error) {
	if s.adminOnlyUpload && !isAdmin(ctx) {
		return api.UploadItem403JSONResponse{Error: "uploads are restricted to administrators"}, nil
	}

	meta, err := s.parseMetadata(request)
	if err != nil {
		return api.UploadItem400JSONResponse{Error: err.Error()}, nil
	}

	filePart, filename, err := s.parseFilePart(request)
	if err != nil {
		return api.UploadItem400JSONResponse{Error: err.Error()}, nil
	}

	itemID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate item ID: %w", err)
	}

	groupID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("generate group ID: %w", err)
	}

	uc := &uploadContext{
		itemID:           itemID,
		groupID:          groupID,
		itemType:         request.AssetType,
		meta:             meta,
		ext:              fileExtension(request.AssetType),
		originalFilename: filename,
	}

	// ── Stream file into temp storage ────────────────────────────────────────
	if resp, err := s.receiveFile(uc, filePart); resp != nil || err != nil {
		return resp, err
	}

	committed := false
	defer func() {
		if !committed {
			s.cleanupTemp(uc)
		}
	}()

	// ── Dispatch to per-asset-type handler ───────────────────────────────────
	switch request.AssetType {
	case api.Map:
		if resp, err := s.uploadMap(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	case api.Skin:
		if resp, err := s.uploadSkin(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	case api.Gameskin:
		if resp, err := s.uploadGameskin(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	case api.Hud:
		if resp, err := s.uploadHud(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	case api.Entity:
		if resp, err := s.uploadEntity(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	case api.Theme:
		if resp, err := s.uploadTheme(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	case api.Template:
		if resp, err := s.uploadTemplate(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	case api.Emoticon:
		if resp, err := s.uploadEmoticon(ctx, uc); resp != nil || err != nil {
			return resp, err
		}
	default:
		return api.UploadItem400JSONResponse{Error: fmt.Sprintf("unknown asset type %q", request.AssetType)}, nil
	}

	committed = true

	return api.UploadItem201JSONResponse{ItemId: itemID}, nil
}

// ── upload pipeline steps ─────────────────────────────────────────────────────

// maxMetadataSize is the maximum allowed size for the JSON metadata part (32 KiB).
const maxMetadataSize = 32 << 10

// parseMetadata reads and decodes the metadata part of the multipart request.
// The JSON body is limited to maxMetadataSize bytes.
func (s *Server) parseMetadata(request api.UploadItemRequestObject) (api.ItemMetadata, error) {
	metaPart, err := request.Body.NextPart()
	if err != nil || metaPart.FormName() != "metadata" {
		return api.ItemMetadata{}, fmt.Errorf("first multipart part must be named \"metadata\"")
	}

	limited := io.LimitReader(metaPart, maxMetadataSize+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return api.ItemMetadata{}, fmt.Errorf("reading metadata: %s", err)
	}
	if len(raw) > maxMetadataSize {
		return api.ItemMetadata{}, fmt.Errorf("metadata too large (max %d bytes)", maxMetadataSize)
	}

	var meta api.ItemMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return api.ItemMetadata{}, fmt.Errorf("invalid metadata JSON: %s", err)
	}
	return meta, nil
}

// parseFilePart reads the file part of the multipart request.
// Returns the part reader and the original filename.
func (s *Server) parseFilePart(request api.UploadItemRequestObject) (*multipart.Part, string, error) {
	filePart, err := request.Body.NextPart()
	if err != nil || filePart.FormName() != "file" {
		return nil, "", fmt.Errorf("second multipart part must be named \"file\"")
	}
	return filePart, filePart.FileName(), nil
}

// receiveFile streams the upload into a temp file, enforcing the size limit.
func (s *Server) receiveFile(uc *uploadContext, filePart io.Reader) (api.UploadItemResponseObject, error) {
	tmpFile, err := s.tmpDir.OpenFile(uc.tmpName(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()

	var fileReader io.Reader = filePart
	maxSize, hasLimit := s.validator.MaxUploadSize(string(uc.itemType))
	if hasLimit {
		fileReader = io.LimitReader(filePart, maxSize+1)
	}

	size, checksum, err := hashAndWrite(fileReader, tmpFile)
	if err != nil {
		_ = s.tmpDir.Remove(uc.tmpName())
		return nil, fmt.Errorf("receive upload: %w", err)
	}

	if hasLimit && size > maxSize {
		_ = s.tmpDir.Remove(uc.tmpName())
		return api.UploadItem400JSONResponse{
			Error: fmt.Sprintf("file exceeds maximum %d bytes for item type %q", maxSize, uc.itemType),
		}, nil
	}

	uc.size = size
	uc.checksum = checksum
	return nil, nil
}

// validateAndSetResolution validates a PNG file, checks the resolution is allowed,
// and sets the group key/value on uc. Returns a 400 response if validation fails.
func (s *Server) validateAndSetResolution(uc *uploadContext) (api.UploadItemResponseObject, error) {
	errResp, resolution := s.validator.ValidateFile(uc.itemType, s.tmpDir, uc.tmpName())
	if errResp != nil {
		return api.UploadItem400JSONResponse{Error: errResp.Error}, nil
	}
	if resolution == "" {
		return api.UploadItem400JSONResponse{Error: "could not detect image resolution"}, nil
	}
	uc.groupKey = "resolution"
	uc.groupValue = resolution
	return nil, nil
}

// validateMap validates a .map file. Maps have no resolution grouping.
func (s *Server) validateMap(uc *uploadContext) (api.UploadItemResponseObject, error) {
	errResp, _ := s.validator.ValidateFile(uc.itemType, s.tmpDir, uc.tmpName())
	if errResp != nil {
		return api.UploadItem400JSONResponse{Error: errResp.Error}, nil
	}
	return nil, nil
}

// buildStoragePaths computes the permanent relative and absolute file paths.
// Files are stored under /<asset_type>/<group_id>/<item_id>.<ext>.
func (s *Server) buildStoragePaths(uc *uploadContext) {
	uc.relPath = fmt.Sprintf("/%s/%s/%s%s", uc.itemType, uc.groupID, uc.itemID, uc.ext)
	uc.absPath = filepath.Join(string(s.fsys), filepath.FromSlash(uc.relPath))
}

// prepareThumbnail generates a thumbnail or decides to reuse the source file.
//
// Thumbnail path semantics:
//
//   - Maps are not images themselves, so a thumbnail is always rendered into a
//     separate file under /<asset_type>/thumbnails/<uuid>.png. The DB column
//     item_thumbnail_path points to that dedicated file.
//
//   - Image-based types (skins, gameskins, …) that exceed the configured
//     bounding box also get a scaled-down copy in /thumbnails/<uuid>.png.
//
//   - Image-based types that already fit within the bounding box need no extra
//     thumbnail file. Their item_thumbnail_path points back to the original
//     item file (relPath), so the thumbnail endpoint serves the source directly.
func (s *Server) prepareThumbnail(uc *uploadContext) (api.UploadItemResponseObject, error) {
	thumbPath, err := s.generateThumbnail(uc.itemType, uc.itemID, uc.groupID, s.tmpDir, uc.tmpName())
	if err != nil {
		return nil, fmt.Errorf("generate thumbnail: %w", err)
	}

	if thumbPath.Valid {
		// A separate thumbnail file was created in tmpDir.
		uc.thumbnailPath = thumbPath
		uc.hasTempThumb = true
	} else if uc.itemType != api.Map {
		// Source image already fits — reuse its own path as the thumbnail.
		uc.thumbnailPath = stdsql.NullString{String: uc.relPath, Valid: true}
	}
	// Maps that fail to render get thumbnailPath = NULL (no thumbnail).
	return nil, nil
}

// persistUpload upserts the group, inserts the item variant, metadata & search values inside a transaction.
// It resolves the canonical groupID, builds storage paths and generates thumbnails before the DB insert.
func (s *Server) persistUpload(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	// ── Per-IP rate limit: only checked when a NEW group would be created ─────
	addr := clientip.FromContext(ctx)
	if s.rateLimitMaxGroups > 0 && !addr.IsLoopback() {
		_, err := s.dao.GetGroupID(ctx, sqlc.GetGroupIDParams{
			AssetType: sqlc.AssetTypeEnum(uc.itemType),
			GroupName: uc.meta.Name,
			GroupKey:  uc.groupKey,
		})
		if errors.Is(err, stdsql.ErrNoRows) {
			// Group does not exist yet — this upload would create a new group.
			since := time.Now().Add(-s.rateLimitWindow)
			count, err := s.dao.CountGroupsCreatedByIP(ctx, addr, since)
			if err != nil {
				return nil, fmt.Errorf("rate limit check: %w", err)
			}
			if count >= int64(s.rateLimitMaxGroups) {
				return api.UploadItem429JSONResponse{
					Error: fmt.Sprintf("rate limit exceeded: at most %d new asset groups may be created per IP within %s",
						s.rateLimitMaxGroups, s.rateLimitWindow),
				}, nil
			}
		}
	}
	txErr := s.dao.Tx(ctx, func(tx sqlc.DAO) error {
		// Upsert the group (creates if new, no-ops if name+key already exists).
		if err := tx.UpsertGroup(ctx, sqlc.UpsertGroupParams{
			GroupID:   uc.groupID,
			AssetType: sqlc.AssetTypeEnum(uc.itemType),
			GroupName: uc.meta.Name,
			GroupKey:  uc.groupKey,
		}); err != nil {
			return err
		}

		// If the group already existed, fetch the canonical group_id.
		existingGroupID, err := tx.GetGroupID(ctx, sqlc.GetGroupIDParams{
			AssetType: sqlc.AssetTypeEnum(uc.itemType),
			GroupName: uc.meta.Name,
			GroupKey:  uc.groupKey,
		})
		if err != nil {
			return fmt.Errorf("get group ID: %w", err)
		}
		uc.groupID = existingGroupID

		// Now that we know the real groupID, compute storage paths and thumbnail.
		s.buildStoragePaths(uc)
		if resp, err := s.prepareThumbnail(uc); resp != nil || err != nil {
			if resp != nil {
				// This shouldn't happen here, but handle gracefully.
				return fmt.Errorf("thumbnail: %s", resp)
			}
			return err
		}

		if err := tx.InsertItemChecked(ctx, sqlc.InsertItemParams{
			ItemID:            uc.itemID,
			GroupID:           uc.groupID,
			GroupValue:        uc.groupValue,
			Size:              uc.size,
			Checksum:          uc.checksum,
			ItemFilePath:      uc.relPath,
			ItemThumbnailPath: uc.thumbnailPath,
			OriginalFilename:  uc.originalFilename,
			MaxTotalSize:      s.maxStorageSize,
		}); err != nil {
			return err
		}

		if err := tx.InsertItemMetadata(ctx, buildMetadataParams(ctx, uc.itemID)); err != nil {
			return err
		}

		for _, sv := range metaToSearchValues(uc.groupID, uc.meta) {
			if err := tx.InsertSearchValue(ctx, sv); err != nil {
				return err
			}
		}

		// Move files to permanent storage inside the transaction so that
		// the DB changes are rolled back if the move fails.
		if err := s.moveToStorage(uc); err != nil {
			return err
		}
		return nil
	})

	if txErr != nil {
		// If the transaction failed after files were moved (e.g. commit
		// error), clean up any already-moved files so nothing is orphaned.
		s.cleanupStorage(uc)
		return s.classifyUploadError(ctx, txErr, uc.checksum)
	}
	return nil, nil
}

// moveToStorage moves the temp item file (and optional thumbnail) to permanent storage.
func (s *Server) moveToStorage(uc *uploadContext) error {
	if err := moveFile(s.tmpDir, uc.tmpName(), uc.absPath); err != nil {
		return fmt.Errorf("move item file: %w", err)
	}

	if uc.hasTempThumb {
		thumbAbsPath := filepath.Join(string(s.fsys), filepath.FromSlash(uc.thumbnailPath.String))
		if err := moveFile(s.tmpDir, uc.tmpThumbName(), thumbAbsPath); err != nil {
			return fmt.Errorf("move thumbnail file: %w", err)
		}
	}
	return nil
}

// cleanupStorage removes any files that moveToStorage may have already placed
// in permanent storage. This is a best-effort operation used when the DB
// transaction fails after files were moved (e.g. commit error).
func (s *Server) cleanupStorage(uc *uploadContext) {
	_ = os.Remove(uc.absPath)
	if uc.hasTempThumb {
		thumbAbsPath := filepath.Join(string(s.fsys), filepath.FromSlash(uc.thumbnailPath.String))
		_ = os.Remove(thumbAbsPath)
	}
}

// ── thumbnail generation ──────────────────────────────────────────────────────

// generateThumbnail creates a PNG thumbnail for the uploaded item.
// Returns a valid NullString with the thumbnail's relative storage path when a
// separate file was written, or an invalid NullString when no file was created.
func (s *Server) generateThumbnail(itemType api.ItemType, itemID, groupID uuid.UUID, tmpDir *os.Root, tmpName string) (stdsql.NullString, error) {
	thumbRelPath := fmt.Sprintf("/%s/%s/thumbnails/%s.png", itemType, groupID, itemID)
	size, ok := s.thumbnailSizes[string(itemType)]
	if !ok {
		return stdsql.NullString{}, nil // no thumbnail config for this type
	}
	maxW, maxH := size.Width, size.Height

	switch itemType {
	case api.Map:
		return s.generateMapThumbnail(itemID, tmpDir, tmpName, thumbRelPath, maxW, maxH)
	case api.Skin:
		return s.generateSkinThumbnail(itemID, tmpDir, tmpName, thumbRelPath)
	case api.Gameskin, api.Hud, api.Entity, api.Theme, api.Template, api.Emoticon:
		return s.generateImageThumbnail(itemID, tmpDir, tmpName, thumbRelPath, maxW, maxH)
	default:
		return stdsql.NullString{}, nil
	}
}

// generateMapThumbnail renders the map at exactly the bounding box dimensions.
func (s *Server) generateMapThumbnail(itemID uuid.UUID, tmpDir *os.Root, tmpName, thumbRelPath string, maxW, maxH int) (stdsql.NullString, error) {
	f, err := tmpDir.Open(tmpName)
	if err != nil {
		return stdsql.NullString{}, fmt.Errorf("open map for thumbnail: %w", err)
	}
	defer f.Close()

	img, err := twmap.Render(f, maxW, maxH)
	if err != nil {
		return stdsql.NullString{}, fmt.Errorf("render map thumbnail: %w", err)
	}

	return s.writeThumbnailPNG(itemID, tmpDir, img, thumbRelPath)
}

// generateSkinThumbnail composites the Tee character from the skin sprite sheet
// in an idle front-facing pose with default eyes, scaled to the base TW 0.6
// resolution (256x128 skin, 32px grid cells).
func (s *Server) generateSkinThumbnail(itemID uuid.UUID, tmpDir *os.Root, tmpName, thumbRelPath string) (stdsql.NullString, error) {
	f, err := tmpDir.Open(tmpName)
	if err != nil {
		return stdsql.NullString{}, fmt.Errorf("open skin for thumbnail: %w", err)
	}
	defer f.Close()

	img, err := twskin.RenderIdleTee(f)
	if err != nil {
		return stdsql.NullString{}, fmt.Errorf("render skin thumbnail: %w", err)
	}

	return s.writeThumbnailPNG(itemID, tmpDir, img, thumbRelPath)
}

// generateImageThumbnail scales down a PNG that exceeds the bounding box.
// Returns an invalid NullString when the source already fits (no file created).
func (s *Server) generateImageThumbnail(itemID uuid.UUID, tmpDir *os.Root, tmpName, thumbRelPath string, maxW, maxH int) (stdsql.NullString, error) {
	f, err := tmpDir.Open(tmpName)
	if err != nil {
		return stdsql.NullString{}, fmt.Errorf("open image for thumbnail: %w", err)
	}

	src, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		return stdsql.NullString{}, fmt.Errorf("decode png for thumbnail: %w", err)
	}

	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()

	if srcW <= maxW && srcH <= maxH {
		return stdsql.NullString{}, nil // source fits — caller will reuse relPath
	}

	dstW, dstH := fitInBox(srcW, srcH, maxW, maxH)
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	return s.writeThumbnailPNG(itemID, tmpDir, dst, thumbRelPath)
}

// writeThumbnailPNG encodes img as PNG into a temp file.
func (s *Server) writeThumbnailPNG(itemID uuid.UUID, tmpDir *os.Root, img image.Image, thumbRelPath string) (stdsql.NullString, error) {
	tmpThumbName := itemID.String() + "_thumb.png"
	tf, err := tmpDir.OpenFile(tmpThumbName, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return stdsql.NullString{}, fmt.Errorf("create temp thumbnail: %w", err)
	}

	if err := png.Encode(tf, img); err != nil {
		_ = tf.Close()
		_ = tmpDir.Remove(tmpThumbName)
		return stdsql.NullString{}, fmt.Errorf("encode thumbnail png: %w", err)
	}
	if err := tf.Close(); err != nil {
		_ = tmpDir.Remove(tmpThumbName)
		return stdsql.NullString{}, fmt.Errorf("close thumbnail: %w", err)
	}

	return stdsql.NullString{String: thumbRelPath, Valid: true}, nil
}

// ── utility helpers ───────────────────────────────────────────────────────────

// cleanupTemp removes the uploaded temp file and optional thumbnail temp file.
func (s *Server) cleanupTemp(uc *uploadContext) {
	_ = s.tmpDir.Remove(uc.tmpName())
	if uc.hasTempThumb {
		_ = s.tmpDir.Remove(uc.tmpThumbName())
	}
}

// moveFile moves a file from the sandboxed tmpDir into absPath, creating parent dirs.
// If the source and destination are on different devices (cross-device link),
// it falls back to copying the file and removing the source.
func moveFile(tmpDir *os.Root, tmpName, absPath string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return err
	}
	srcPath := filepath.Join(tmpDir.Name(), tmpName)
	err := os.Rename(srcPath, absPath)
	if err == nil {
		return nil
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	// Cross-device link: fall back to copy + delete.
	return copyAndRemove(srcPath, absPath)
}

// copyAndRemove copies src to dst and removes src. If the copy fails the
// partially written dst is removed so no garbage is left behind.
func copyAndRemove(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}

	if _, err := io.Copy(df, sf); err != nil {
		_ = df.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := df.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return os.Remove(src)
}

// classifyUploadError maps known DB/domain errors to the appropriate HTTP response.
// On duplicate checksum, it looks up the existing item to provide a helpful message.
func (s *Server) classifyUploadError(ctx context.Context, txErr error, checksum string) (api.UploadItemResponseObject, error) {
	if errors.Is(txErr, sqlc.ErrDuplicateChecksum) {
		existing, err := s.dao.GetItemByChecksum(ctx, checksum)
		if err != nil {
			return api.UploadItem409JSONResponse{
				Error: "an identical file already exists (could not look up details)",
			}, nil
		}
		msg := fmt.Sprintf(
			"this file already exists as %s \"%s\"",
			existing.AssetType, existing.GroupName,
		)
		if existing.GroupValue != "" {
			msg = fmt.Sprintf(
				"this file already exists as %s \"%s\" (%s)",
				existing.AssetType, existing.GroupName, existing.GroupValue,
			)
		}
		return api.UploadItem409JSONResponse{Error: msg}, nil
	}
	if errors.Is(txErr, sqlc.ErrDuplicateVariant) {
		return api.UploadItem409JSONResponse{Error: "this resolution variant already exists in the group"}, nil
	}
	if errors.Is(txErr, sqlc.ErrItemAlreadyExists) {
		return api.UploadItem409JSONResponse{Error: "item already exists"}, nil
	}
	if errors.Is(txErr, sqlc.ErrStorageLimitExceeded) {
		return api.UploadItem507JSONResponse{Error: "storage limit exceeded"}, nil
	}
	return nil, fmt.Errorf("persist upload: %w", txErr)
}

// fitInBox calculates the largest dimensions that fit within maxW×maxH
// while preserving the aspect ratio of srcW×srcH.
func fitInBox(srcW, srcH, maxW, maxH int) (int, int) {
	ratio := float64(srcW) / float64(srcH)
	dstW := maxW
	dstH := int(float64(dstW) / ratio)
	if dstH > maxH {
		dstH = maxH
		dstW = int(float64(dstH) * ratio)
	}
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}
	return dstW, dstH
}

// hashAndWrite streams src into dst, returning bytes written and hex SHA-256.
func hashAndWrite(src io.Reader, dst io.Writer) (int64, string, error) {
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), src)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// metaToSearchValues converts ItemMetadata into individual search_value rows.
func metaToSearchValues(groupID uuid.UUID, meta api.ItemMetadata) []sqlc.InsertSearchValueParams {
	rows := []sqlc.InsertSearchValueParams{
		{GroupID: groupID, KeyName: "name", KeyValue: meta.Name},
		{GroupID: groupID, KeyName: "license", KeyValue: string(meta.License)},
	}
	for _, c := range meta.Creators {
		rows = append(rows, sqlc.InsertSearchValueParams{
			GroupID:  groupID,
			KeyName:  "creators",
			KeyValue: c,
		})
	}

	return rows
}

// buildMetadataParams populates the DB audit row.
func buildMetadataParams(ctx context.Context, itemID uuid.UUID) sqlc.InsertItemMetadataParams {
	addr := clientip.FromContext(ctx)
	return sqlc.InsertItemMetadataParams{
		ItemID:    itemID,
		CreatorIp: addrToInet(addr),
	}
}

func addrToInet(addr netip.Addr) pqtype.Inet {
	if !addr.IsValid() {
		return pqtype.Inet{}
	}
	ip := addr.As16()
	return pqtype.Inet{
		IPNet: net.IPNet{
			IP:   net.IP(ip[:]),
			Mask: net.CIDRMask(128, 128),
		},
		Valid: true,
	}
}

// fileExtension returns the storage file extension for the given item type.
func fileExtension(itemType api.ItemType) string {
	if itemType == api.Map {
		return ".map"
	}
	return ".png"
}
