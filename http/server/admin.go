package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	sqlc "github.com/jxsl13/teeworlds-asset-service/sql"
)

// AdminDeleteGroup deletes an entire asset group and its files.
func (s *Server) AdminDeleteGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	assetType := chi.URLParam(r, "asset_type")
	groupID, err := uuid.Parse(chi.URLParam(r, "group_id"))
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	at := sqlc.AssetTypeEnum(assetType)
	if !at.Valid() {
		http.Error(w, "invalid asset_type", http.StatusBadRequest)
		return
	}

	// Collect file paths before deleting DB rows (CASCADE will remove items).
	paths, err := s.dao.GetGroupItemPaths(ctx, uuidToPgtype(groupID))
	if err != nil {
		slog.Error("admin: get group item paths", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := s.dao.DeleteGroup(ctx, sqlc.DeleteGroupParams{
		GroupID:   uuidToPgtype(groupID),
		AssetType: at,
	}); err != nil {
		slog.Error("admin: delete group", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Best-effort file cleanup.
	s.removeFiles(paths)

	w.WriteHeader(http.StatusNoContent)
}

// AdminDeleteVariant deletes a single variant from a group.
func (s *Server) AdminDeleteVariant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	assetType := chi.URLParam(r, "asset_type")
	groupID, err := uuid.Parse(chi.URLParam(r, "group_id"))
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}
	itemID, err := uuid.Parse(chi.URLParam(r, "item_id"))
	if err != nil {
		http.Error(w, "invalid item_id", http.StatusBadRequest)
		return
	}

	at := sqlc.AssetTypeEnum(assetType)
	if !at.Valid() {
		http.Error(w, "invalid asset_type", http.StatusBadRequest)
		return
	}

	// Get the item info to find the file paths.
	info, err := s.dao.GetItemInfo(ctx, sqlc.GetItemInfoParams{ItemID: uuidToPgtype(itemID), GroupID: uuidToPgtype(groupID)})
	if err != nil {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}

	if err := s.dao.DeleteItem(ctx, sqlc.DeleteItemParams{ItemID: uuidToPgtype(itemID), GroupID: uuidToPgtype(groupID)}); err != nil {
		slog.Error("admin: delete item", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// If the group is now empty, delete it too.
	count, err := s.dao.CountGroupItems(ctx, uuidToPgtype(groupID))
	if err == nil && count == 0 {
		_ = s.dao.DeleteGroup(ctx, sqlc.DeleteGroupParams{GroupID: uuidToPgtype(groupID), AssetType: at})
	}

	// Best-effort file cleanup.
	s.removeFiles([]sqlc.GetGroupItemPathsRow{
		{ItemFilePath: info.ItemFilePath, ItemThumbnailPath: info.ItemThumbnailPath},
	})

	w.WriteHeader(http.StatusNoContent)
}

// adminUpdateGroupRequest is the JSON body for PATCH /admin/{asset_type}/{group_id}.
type adminUpdateGroupRequest struct {
	Name     *string  `json:"name,omitempty"`
	Creators []string `json:"creators,omitempty"`
	License  *string  `json:"license,omitempty"`
}

// AdminUpdateGroup updates editable properties of a group.
func (s *Server) AdminUpdateGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	assetType := chi.URLParam(r, "asset_type")
	groupID, err := uuid.Parse(chi.URLParam(r, "group_id"))
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	at := sqlc.AssetTypeEnum(assetType)
	if !at.Valid() {
		http.Error(w, "invalid asset_type", http.StatusBadRequest)
		return
	}

	var body adminUpdateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	// Verify the group exists.
	if _, err := s.dao.GetGroupInfo(ctx, sqlc.GetGroupInfoParams{GroupID: uuidToPgtype(groupID), AssetType: at}); err != nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}

	if err := s.dao.Tx(ctx, func(tx sqlc.DAO) error {
		if body.Name != nil {
			name := strings.TrimSpace(*body.Name)
			if name == "" {
				return errBadRequest("name must not be empty")
			}
			if err := tx.UpdateGroupName(ctx, sqlc.UpdateGroupNameParams{
				GroupName: name,
				GroupID:   uuidToPgtype(groupID),
				AssetType: at,
			}); err != nil {
				return err
			}
			// Update the search_value for "name".
			if err := tx.DeleteSearchValues(ctx, sqlc.DeleteSearchValuesParams{GroupID: uuidToPgtype(groupID), KeyName: "name"}); err != nil {
				return err
			}
			if err := tx.InsertSearchValue(ctx, sqlc.InsertSearchValueParams{GroupID: uuidToPgtype(groupID), KeyName: "name", KeyValue: name}); err != nil {
				return err
			}
		}

		if body.Creators != nil {
			// Replace creators search values.
			if err := tx.DeleteSearchValues(ctx, sqlc.DeleteSearchValuesParams{GroupID: uuidToPgtype(groupID), KeyName: "creators"}); err != nil {
				return err
			}
			for _, c := range body.Creators {
				c = strings.TrimSpace(c)
				if c == "" {
					continue
				}
				if err := tx.InsertSearchValue(ctx, sqlc.InsertSearchValueParams{GroupID: uuidToPgtype(groupID), KeyName: "creators", KeyValue: c}); err != nil {
					return err
				}
			}
		}

		if body.License != nil {
			license := strings.TrimSpace(*body.License)
			if err := tx.DeleteSearchValues(ctx, sqlc.DeleteSearchValuesParams{GroupID: uuidToPgtype(groupID), KeyName: "license"}); err != nil {
				return err
			}
			if license != "" {
				if err := tx.InsertSearchValue(ctx, sqlc.InsertSearchValueParams{GroupID: uuidToPgtype(groupID), KeyName: "license", KeyValue: license}); err != nil {
					return err
				}
			}
		}

		return nil
	}); err != nil {
		if isBadRequest(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		slog.Error("admin: update group", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// AdminReplaceVariant replaces the file for an existing variant.
func (s *Server) AdminReplaceVariant(w http.ResponseWriter, r *http.Request) {
	// TODO: implement variant file replacement (multipart upload similar to upload handler)
	http.Error(w, "not implemented yet", http.StatusNotImplemented)
}

// adminGroupItemResponse is a single item in the JSON response from AdminGetGroupItems.
type adminGroupItemResponse struct {
	ItemID           string `json:"item_id"`
	GroupValue       string `json:"group_value"`
	Size             int64  `json:"size"`
	OriginalFilename string `json:"original_filename"`
}

// adminItemMetadataResponse is a single item in the JSON response from AdminGetGroupItemsMetadata.
type adminItemMetadataResponse struct {
	ItemID           string `json:"item_id"`
	GroupValue       string `json:"group_value"`
	Size             int64  `json:"size"`
	OriginalFilename string `json:"original_filename"`
	CreatedAt        string `json:"created_at"`
	CreatorIP        string `json:"creator_ip"`
	CreatorAgent     string `json:"creator_agent"`
	AcceptLanguage   string `json:"accept_language"`
	Referer          string `json:"referer"`
	ContentType      string `json:"content_type"`
	RequestID        string `json:"request_id"`
}

// AdminGetGroupItems returns the list of items (variants) inside a group.
func (s *Server) AdminGetGroupItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	groupID, err := uuid.Parse(chi.URLParam(r, "group_id"))
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	rows, err := s.dao.GetGroupItems(ctx, uuidToPgtype(groupID))
	if err != nil {
		slog.Error("admin: get group items", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]adminGroupItemResponse, 0, len(rows))
	for _, r := range rows {
		items = append(items, adminGroupItemResponse{
			ItemID:           uuid.UUID(r.ItemID.Bytes).String(),
			GroupValue:       r.GroupValue,
			Size:             r.Size,
			OriginalFilename: r.OriginalFilename,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// AdminGetGroupItemsMetadata returns items with their upload metadata.
func (s *Server) AdminGetGroupItemsMetadata(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	groupID, err := uuid.Parse(chi.URLParam(r, "group_id"))
	if err != nil {
		http.Error(w, "invalid group_id", http.StatusBadRequest)
		return
	}

	rows, err := s.dao.GetGroupItemsWithMetadata(ctx, uuidToPgtype(groupID))
	if err != nil {
		slog.Error("admin: get group items metadata", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	items := make([]adminItemMetadataResponse, 0, len(rows))
	for _, r := range rows {
		items = append(items, adminItemMetadataResponse{
			ItemID:           uuid.UUID(r.ItemID.Bytes).String(),
			GroupValue:       r.GroupValue,
			Size:             r.Size,
			OriginalFilename: r.OriginalFilename,
			CreatedAt:        r.CreatedAt.Time.Format("2006-01-02 15:04:05"),
			CreatorIP:        r.CreatorIp,
			CreatorAgent:     r.CreatorAgent,
			AcceptLanguage:   r.AcceptLanguage,
			Referer:          r.Referer,
			ContentType:      r.ContentType,
			RequestID:        r.RequestID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}

// removeFiles deletes item files from the filesystem (best-effort).
func (s *Server) removeFiles(paths []sqlc.GetGroupItemPathsRow) {
	for _, p := range paths {
		fp := filepath.Join(string(s.fsys), p.ItemFilePath)
		if err := os.Remove(fp); err != nil && !os.IsNotExist(err) {
			slog.Warn("admin: remove file", "path", fp, "err", err)
		}
		if p.ItemThumbnailPath != nil && *p.ItemThumbnailPath != "" {
			tp := filepath.Join(string(s.fsys), *p.ItemThumbnailPath)
			if err := os.Remove(tp); err != nil && !os.IsNotExist(err) {
				slog.Warn("admin: remove thumbnail", "path", tp, "err", err)
			}
		}
		// Try to remove the parent dir (empty after file removal).
		dir := filepath.Dir(fp)
		_ = os.Remove(dir)               // item dir
		_ = os.Remove(filepath.Dir(dir)) // group dir
	}
}

// errBadRequest wraps a validation error for admin handlers.
type badRequestError struct{ msg string }

func (e *badRequestError) Error() string { return e.msg }

func errBadRequest(msg string) error { return &badRequestError{msg: msg} }

func isBadRequest(err error) bool {
	_, ok := err.(*badRequestError)
	return ok
}
