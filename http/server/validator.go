package server

import (
	"fmt"
	"image/png"
	"os"

	"github.com/jxsl13/search-service/config"
	"github.com/jxsl13/search-service/http/api"
	"github.com/jxsl13/search-service/internal/twmap"
)

// ItemTypeConfig holds the validation constraints for a single item type.
type ItemTypeConfig struct {
	// Resolutions lists the allowed width×height pairs for PNG-based types.
	// Empty for non-image types (e.g. "map").
	Resolutions []config.Resolution

	// MaxUploadSize is the maximum file size in bytes for this item type.
	MaxUploadSize int64
}

// Validator centralises all per-item-type validation configuration.
type Validator struct {
	types map[string]ItemTypeConfig
}

// NewValidator constructs a Validator from the resolved configuration maps.
func NewValidator(allowedResolutions map[string][]config.Resolution, maxUploadSizes map[string]int64) *Validator {
	types := make(map[string]ItemTypeConfig)

	// Collect every item type mentioned in either map.
	seen := make(map[string]struct{})
	for k := range allowedResolutions {
		seen[k] = struct{}{}
	}
	for k := range maxUploadSizes {
		seen[k] = struct{}{}
	}

	for itemType := range seen {
		types[itemType] = ItemTypeConfig{
			Resolutions:   allowedResolutions[itemType],
			MaxUploadSize: maxUploadSizes[itemType],
		}
	}
	return &Validator{types: types}
}

// MaxUploadSize returns the maximum upload size for the given item type.
// If the item type is unknown, ok is false.
func (v *Validator) MaxUploadSize(itemType string) (int64, bool) {
	tc, ok := v.types[itemType]
	if !ok {
		return 0, false
	}
	return tc.MaxUploadSize, true
}

// IsAllowedResolution checks whether the given width and height are permitted
// for the specified item type. Returns true if the item type has no resolution
// constraints (e.g. "map") or if the type is unknown.
func (v *Validator) IsAllowedResolution(itemType string, width, height int) bool {
	tc, ok := v.types[itemType]
	if !ok {
		return true
	}
	if len(tc.Resolutions) == 0 {
		return true
	}
	for _, r := range tc.Resolutions {
		if r.Width == width && r.Height == height {
			return true
		}
	}
	return false
}

// ValidateFile dispatches to the type-specific validator.
// Returns a non-nil error response if validation fails, or nil if the file is valid.
func (v *Validator) ValidateFile(itemType api.ItemType, tmpDir *os.Root, tmpName string) *api.ErrorResponse {
	switch itemType {
	case api.Map:
		return v.validateMap(tmpDir, tmpName)
	case api.Gameskin, api.Hud, api.Skin, api.Entity, api.Theme, api.Template, api.Emoticon:
		return v.validatePNG(tmpDir, tmpName, string(itemType))
	default:
		return &api.ErrorResponse{Error: fmt.Sprintf("unknown item type %q", itemType)}
	}
}

// validateMap validates a .map file using the Teeworlds/DDNet datafile parser.
func (v *Validator) validateMap(tmpDir *os.Root, tmpName string) *api.ErrorResponse {
	f, err := tmpDir.Open(tmpName)
	if err != nil {
		return &api.ErrorResponse{Error: "internal server error"}
	}
	defer f.Close()

	if err := twmap.Validate(f); err != nil {
		return &api.ErrorResponse{Error: fmt.Sprintf("invalid map file: %s", err)}
	}
	return nil
}

// validatePNG validates that the temp file is a valid PNG with an allowed resolution.
func (v *Validator) validatePNG(tmpDir *os.Root, tmpName string, itemType string) *api.ErrorResponse {
	f, err := tmpDir.Open(tmpName)
	if err != nil {
		return &api.ErrorResponse{Error: "internal server error"}
	}
	defer f.Close()

	pngCfg, err := png.DecodeConfig(f)
	if err != nil {
		return &api.ErrorResponse{Error: "file is not a valid PNG image"}
	}

	if !v.IsAllowedResolution(itemType, pngCfg.Width, pngCfg.Height) {
		return &api.ErrorResponse{
			Error: fmt.Sprintf("resolution %dx%d is not allowed for item type %q", pngCfg.Width, pngCfg.Height, itemType),
		}
	}
	return nil
}
