package twmap

import (
	"embed"
	"fmt"
	"image"
	"image/png"
	"strings"
)

//go:embed mapres/*.png
var mapresFS embed.FS

// externalImages holds all embedded tileset PNGs, keyed by lowercase name
// without extension (e.g. "grass_main"). Populated once at startup by Init.
var externalImages map[string]*image.NRGBA

// Init decodes all embedded tileset PNGs so they are ready for use.
// Must be called once at application startup before any map thumbnails
// are generated.
func Init() error {
	entries, err := mapresFS.ReadDir("mapres")
	if err != nil {
		return fmt.Errorf("read embedded mapres: %w", err)
	}

	m := make(map[string]*image.NRGBA, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}

		key := strings.TrimSuffix(e.Name(), ".png")

		f, err := mapresFS.Open("mapres/" + e.Name())
		if err != nil {
			return fmt.Errorf("open embedded %s: %w", e.Name(), err)
		}

		decoded, err := png.Decode(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("decode embedded %s: %w", e.Name(), err)
		}

		nrgba, ok := decoded.(*image.NRGBA)
		if !ok {
			bounds := decoded.Bounds()
			nrgba = image.NewNRGBA(bounds)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					nrgba.Set(x, y, decoded.At(x, y))
				}
			}
		}

		m[key] = nrgba
	}

	externalImages = m
	return nil
}

// resolveExternalImage looks up an external image by name in the pre-loaded
// tileset cache. Returns nil if the name does not match any shipped tileset.
func resolveExternalImage(name string) *image.NRGBA {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil
	}
	return externalImages[key]
}
