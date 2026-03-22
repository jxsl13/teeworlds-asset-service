package twmap

import (
	"embed"
	"image"
	"image/png"
	"strings"
	"sync"
)

//go:embed mapres/*.png
var mapresFS embed.FS

// externalCache lazily decodes embedded PNGs the first time they are needed.
var externalCache sync.Map // map[string]*image.NRGBA

// resolveExternalImage looks up an external image by name in the embedded
// DDNet mapres directory.  Returns nil if the name does not match any
// shipped tileset.
func resolveExternalImage(name string) *image.NRGBA {
	// Normalize: map files store the name without extension and without
	// path prefix, e.g. "grass_main".  The embedded files live at
	// "mapres/<name>.png".
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil
	}

	if cached, ok := externalCache.Load(key); ok {
		img, _ := cached.(*image.NRGBA)
		return img
	}

	path := "mapres/" + key + ".png"
	f, err := mapresFS.Open(path)
	if err != nil {
		// Not a shipped tileset — cache the miss.
		externalCache.Store(key, (*image.NRGBA)(nil))
		return nil
	}
	defer f.Close()

	decoded, err := png.Decode(f)
	if err != nil {
		externalCache.Store(key, (*image.NRGBA)(nil))
		return nil
	}

	// Convert to *image.NRGBA if needed.
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

	externalCache.Store(key, nrgba)
	return nrgba
}
