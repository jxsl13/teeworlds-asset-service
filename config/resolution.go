package config

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Resolution is an allowed width×height pair for an image.
type Resolution struct {
	Width  int
	Height int
}

func (r Resolution) String() string {
	return fmt.Sprintf("%dx%d", r.Width, r.Height)
}

// DefaultResolutions defines the default permitted resolutions per item type.
// "map" is intentionally absent because .map files are not images.
//
// Resolutions are derived from the DDNet sprite-grid definitions in content.py.
// The engine requires width % grid_x == 0 && height % grid_y == 0, and the
// image must be RGBA PNG. Only power-of-two multiples of the base grid are
// accepted here, matching what the community actually uses.
var DefaultResolutions = map[string][]Resolution{
	// Gameskin: grid 32×16 → aspect 2:1
	"gameskin": {
		{Width: 1024, Height: 512},  // original 0.6
		{Width: 2048, Height: 1024}, // HD (DDNet default)
		{Width: 4096, Height: 2048}, // 4K / UHD
		{Width: 8192, Height: 4096}, // 8K
	},
	// Emoticons: grid 4×4 → aspect 1:1
	"emoticon": {
		{Width: 256, Height: 256},   // original 0.6 (64px per emoticon)
		{Width: 512, Height: 512},   // HD
		{Width: 1024, Height: 1024}, // UHD
		{Width: 2048, Height: 2048}, // 4K
		{Width: 4096, Height: 4096}, // 8K
	},
	// HUD: grid 16×16 → aspect 1:1
	"hud": {
		{Width: 256, Height: 256},   // original 0.6
		{Width: 512, Height: 512},   // HD
		{Width: 1024, Height: 1024}, // UHD
		{Width: 2048, Height: 2048}, // 4K
		{Width: 4096, Height: 4096}, // 8K
	},
	// Tee skins: grid 8×4 → aspect 2:1
	"skin": {
		{Width: 256, Height: 128},   // original 0.6
		{Width: 512, Height: 256},   // mini-HD
		{Width: 1024, Height: 512},  // HD
		{Width: 2048, Height: 1024}, // UHD
		{Width: 4096, Height: 2048}, // 4K
		{Width: 8192, Height: 4096}, // 8K
	},
	// Particles: grid 8×8 → aspect 1:1
	"particle": {
		{Width: 256, Height: 256},   // original 0.6
		{Width: 512, Height: 512},   // HD
		{Width: 1024, Height: 1024}, // UHD
		{Width: 2048, Height: 2048}, // 4K
		{Width: 4096, Height: 4096}, // 8K
	},
	// Extras: grid 16×16 → aspect 1:1
	"extra": {
		{Width: 256, Height: 256},
		{Width: 512, Height: 512},
		{Width: 1024, Height: 1024},
		{Width: 2048, Height: 2048},
		{Width: 4096, Height: 4096}, // 8K
	},
	"entity": {
		{Width: 256, Height: 256},
		{Width: 512, Height: 512},
		{Width: 1024, Height: 1024}, // default
		{Width: 2048, Height: 2048}, // 4K / UDH
		{Width: 4096, Height: 4096}, // 8K
	},
	"theme": {
		// 4:3
		{Width: 640, Height: 480},
		{Width: 800, Height: 600},
		{Width: 1024, Height: 768},
		{Width: 1152, Height: 864},
		{Width: 1280, Height: 960},
		{Width: 1400, Height: 1050},
		{Width: 1600, Height: 1200},
		// 16:10
		{Width: 1280, Height: 800},
		{Width: 1440, Height: 900},
		{Width: 1680, Height: 1050},
		{Width: 1920, Height: 1200},
		{Width: 2560, Height: 1600},
		{Width: 3840, Height: 2400},
		// 16:9
		{Width: 1280, Height: 720},
		{Width: 1366, Height: 768},
		{Width: 1600, Height: 900},
		{Width: 1920, Height: 1080},
		{Width: 2560, Height: 1440},
		{Width: 3840, Height: 2160},
		{Width: 5120, Height: 2880},
		{Width: 7680, Height: 4320},
	},
	// "template" is populated by init() below from all image-type resolutions.
}

func init() {
	DefaultResolutions["template"] = collectImageResolutions()
}

// collectImageResolutions gathers all unique resolutions from every image-based
// item type (everything except "map" and "template" itself) and returns them
// sorted by total pixel count ascending.
func collectImageResolutions() []Resolution {
	seen := make(map[Resolution]struct{})
	for itemType, resolutions := range DefaultResolutions {
		if itemType == "map" || itemType == "template" {
			continue
		}
		for _, r := range resolutions {
			seen[r] = struct{}{}
		}
	}
	out := make([]Resolution, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	// Sort by pixel count ascending, then width as tiebreaker.
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			pi := int64(out[i].Width) * int64(out[i].Height)
			pj := int64(out[j].Width) * int64(out[j].Height)
			if pj < pi || (pj == pi && out[j].Width < out[i].Width) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// DefaultThumbnailSizes returns the default thumbnail bounding box per item type.
//
//   - map:  1920×1080 (rendered preview)
//   - skin: 64×64     (composited idle tee)
//   - others: smallest allowed resolution from DefaultResolutions
//
// Types without a default resolution entry get no thumbnail size (absent from map).
func DefaultThumbnailSizes() map[string]Resolution {
	m := map[string]Resolution{
		"map":  {Width: 1920, Height: 1080},
		"skin": {Width: 64, Height: 64},
	}
	for itemType, resolutions := range DefaultResolutions {
		if _, ok := m[itemType]; ok {
			continue // already set with an explicit override
		}
		if len(resolutions) == 0 {
			continue
		}
		smallest := resolutions[0]
		for _, r := range resolutions[1:] {
			if int64(r.Width)*int64(r.Height) < int64(smallest.Width)*int64(smallest.Height) {
				smallest = r
			}
		}
		m[itemType] = smallest
	}
	return m
}

// ParseResolutions parses a comma-separated list of WxH pairs (e.g. "256x128,512x512").
// If H > W for any entry, a warning is logged.
func ParseResolutions(raw string) ([]Resolution, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	res := make([]Resolution, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		w, h, err := parseWxH(p)
		if err != nil {
			return nil, fmt.Errorf("invalid resolution %q: %w", p, err)
		}
		if h > w {
			slog.Warn("resolution has height > width", "resolution", fmt.Sprintf("%dx%d", w, h))
		}
		res = append(res, Resolution{Width: w, Height: h})
	}
	return res, nil
}

func parseWxH(s string) (int, int, error) {
	idx := strings.IndexByte(s, 'x')
	if idx < 0 {
		return 0, 0, fmt.Errorf("expected WxH format")
	}
	w, err := strconv.Atoi(s[:idx])
	if err != nil {
		return 0, 0, fmt.Errorf("width: %w", err)
	}
	h, err := strconv.Atoi(s[idx+1:])
	if err != nil {
		return 0, 0, fmt.Errorf("height: %w", err)
	}
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("width and height must be positive")
	}
	return w, h, nil
}

// DefaultMaxUploadSize returns a sensible default maximum upload size in bytes
// for the given item type. For PNG types it is derived from the largest allowed
// resolution (W×H×4 bytes/pixel for uncompressed RGBA). For maps it is 16 MiB.
func DefaultMaxUploadSize(itemType string, resolutions []Resolution) int64 {
	if itemType == "map" {
		return 16 * 1024 * 1024 // 16 MiB
	}
	var maxPixels int64
	for _, r := range resolutions {
		px := int64(r.Width) * int64(r.Height)
		if px > maxPixels {
			maxPixels = px
		}
	}
	if maxPixels == 0 {
		return 10 * 1024 * 1024 // fallback 10 MiB
	}
	return maxPixels * 4 // RGBA
}
