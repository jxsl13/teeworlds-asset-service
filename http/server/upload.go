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
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgerrcode"
	"github.com/jxsl13/asset-service/http/api"
	"github.com/jxsl13/asset-service/internal/twmap"
	"github.com/jxsl13/asset-service/internal/twskin"
	sqlc "github.com/jxsl13/asset-service/sql"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/image/draw"
)

var errStorageLimitExceeded = errors.New("storage limit exceeded")

// uploadContext bundles every piece of state accumulated during a single upload.
type uploadContext struct {
	itemID   uuid.UUID
	itemType api.ItemType
	meta     api.ItemMetadata

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
func (s *Server) UploadItem(ctx context.Context, request api.UploadItemRequestObject) (api.UploadItemResponseObject, error) {
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

	uc := &uploadContext{
		itemID:           itemID,
		itemType:         request.ItemType,
		meta:             meta,
		ext:              fileExtension(request.ItemType),
		originalFilename: filename,
	}

	// ── Stream, validate, generate thumbnail ─────────────────────────────────
	if resp, err := s.receiveFile(uc, filePart); resp != nil || err != nil {
		return resp, err
	}

	// From this point on, temp files exist. Ensure they are cleaned up on any
	// failure path; disarm after the final move to permanent storage succeeds.
	committed := false
	defer func() {
		if !committed {
			s.cleanupTemp(uc)
		}
	}()

	if resp, err := s.validateUpload(uc); resp != nil || err != nil {
		return resp, err
	}
	s.buildStoragePaths(uc)
	if resp, err := s.prepareThumbnail(uc); resp != nil || err != nil {
		return resp, err
	}

	// ── Persist & finalise ───────────────────────────────────────────────────
	if resp, err := s.persistUpload(ctx, uc); resp != nil || err != nil {
		return resp, err
	}
	if err := s.moveToStorage(uc); err != nil {
		return nil, err
	}
	committed = true

	return api.UploadItem201JSONResponse{ItemId: itemID}, nil
}

// ── upload pipeline steps ─────────────────────────────────────────────────────

// parseMetadata reads and decodes the metadata part of the multipart request.
func (s *Server) parseMetadata(request api.UploadItemRequestObject) (api.ItemMetadata, error) {
	metaPart, err := request.Body.NextPart()
	if err != nil || metaPart.FormName() != "metadata" {
		return api.ItemMetadata{}, fmt.Errorf("first multipart part must be named \"metadata\"")
	}

	var meta api.ItemMetadata
	if err := json.NewDecoder(metaPart).Decode(&meta); err != nil {
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

// validateUpload checks the file format and resolution.
func (s *Server) validateUpload(uc *uploadContext) (api.UploadItemResponseObject, error) {
	if errResp := s.validator.ValidateFile(uc.itemType, s.tmpDir, uc.tmpName()); errResp != nil {
		return api.UploadItem400JSONResponse{Error: errResp.Error}, nil
	}
	return nil, nil
}

// buildStoragePaths computes the permanent relative and absolute file paths.
func (s *Server) buildStoragePaths(uc *uploadContext) {
	uc.relPath = fmt.Sprintf("/%s/%s%s", uc.itemType, uc.itemID, uc.ext)
	uc.absPath = filepath.Join(string(s.fsys), filepath.FromSlash(uc.relPath))
}

// prepareThumbnail generates a thumbnail or decides to reuse the source file.
//
// Thumbnail path semantics:
//
//   - Maps are not images themselves, so a thumbnail is always rendered into a
//     separate file under /<item_type>/thumbnails/<uuid>.png. The DB column
//     item_thumbnail_path points to that dedicated file.
//
//   - Image-based types (skins, gameskins, …) that exceed the configured
//     bounding box also get a scaled-down copy in /thumbnails/<uuid>.png.
//
//   - Image-based types that already fit within the bounding box need no extra
//     thumbnail file. Their item_thumbnail_path points back to the original
//     item file (relPath), so the thumbnail endpoint serves the source directly.
func (s *Server) prepareThumbnail(uc *uploadContext) (api.UploadItemResponseObject, error) {
	thumbPath, err := s.generateThumbnail(uc.itemType, uc.itemID, s.tmpDir, uc.tmpName())
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

// persistUpload inserts the item, metadata & search values inside a transaction.
func (s *Server) persistUpload(ctx context.Context, uc *uploadContext) (api.UploadItemResponseObject, error) {
	itemValue, err := json.Marshal(uc.meta)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	txErr := s.dao.Tx(ctx, func(tx sqlc.DAO) error {
		inserted, err := tx.InsertItem(ctx, sqlc.InsertItemParams{
			ItemID:            uc.itemID,
			ItemType:          sqlc.ItemTypeEnum(uc.itemType),
			Size:              uc.size,
			Checksum:          uc.checksum,
			ItemFilePath:      uc.relPath,
			ItemThumbnailPath: uc.thumbnailPath,
			ItemValue:         itemValue,
			OriginalFilename:  uc.originalFilename,
			MaxTotalSize:      s.maxStorageSize,
		})
		if err != nil {
			return err
		}
		if inserted == 0 {
			return errStorageLimitExceeded
		}

		if err := tx.InsertItemMetadata(ctx, buildMetadataParams(ctx, uc.itemID)); err != nil {
			return err
		}

		for _, sv := range metaToSearchValues(uc.itemID, uc.meta) {
			if err := tx.InsertSearchValue(ctx, sv); err != nil {
				return err
			}
		}
		return nil
	})

	if txErr != nil {
		return classifyTxError(txErr)
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

// ── thumbnail generation ──────────────────────────────────────────────────────

// generateThumbnail creates a PNG thumbnail for the uploaded item.
// Returns a valid NullString with the thumbnail's relative storage path when a
// separate file was written, or an invalid NullString when no file was created.
func (s *Server) generateThumbnail(itemType api.ItemType, itemID uuid.UUID, tmpDir *os.Root, tmpName string) (stdsql.NullString, error) {
	thumbRelPath := fmt.Sprintf("/%s/thumbnails/%s.png", itemType, itemID)
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
func moveFile(tmpDir *os.Root, tmpName, absPath string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o750); err != nil {
		return err
	}
	return os.Rename(filepath.Join(tmpDir.Name(), tmpName), absPath)
}

// classifyTxError maps known DB errors to the appropriate HTTP response.
// Returns (response, nil) for expected DB errors (409, 507) and (nil, error)
// for truly unexpected failures.
func classifyTxError(txErr error) (api.UploadItemResponseObject, error) {
	var pgErr *pgconn.PgError
	if errors.As(txErr, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
		return api.UploadItem409JSONResponse{Error: "item or checksum already exists"}, nil
	}
	if errors.Is(txErr, errStorageLimitExceeded) {
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
func metaToSearchValues(id uuid.UUID, meta api.ItemMetadata) []sqlc.InsertSearchValueParams {
	rows := []sqlc.InsertSearchValueParams{
		{ItemID: id, KeyName: "name", KeyValue: meta.Name},
		{ItemID: id, KeyName: "license", KeyValue: string(meta.License)},
	}
	for _, c := range meta.Creators {
		rows = append(rows, sqlc.InsertSearchValueParams{
			ItemID:   id,
			KeyName:  "creators",
			KeyValue: c,
		})
	}

	return rows
}

// buildMetadataParams populates the DB audit row.
func buildMetadataParams(ctx context.Context, itemID uuid.UUID) sqlc.InsertItemMetadataParams {
	ip := clientIPFromContext(ctx)
	return sqlc.InsertItemMetadataParams{
		ItemID: itemID,
		CreatorIp: pqtype.Inet{
			IPNet: net.IPNet{
				IP:   net.ParseIP(ip),
				Mask: net.CIDRMask(128, 128),
			},
			Valid: ip != "" && net.ParseIP(ip) != nil,
		},
	}
}

// fileExtension returns the storage file extension for the given item type.
func fileExtension(itemType api.ItemType) string {
	if itemType == api.Map {
		return ".map"
	}
	return ".png"
}
