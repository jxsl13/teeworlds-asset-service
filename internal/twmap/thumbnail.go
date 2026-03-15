package twmap

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"

	"golang.org/x/image/draw"
)

// DDNet editor checkerboard background colors.
const (
	checkerLight = 186 // RGB(186,186,186)
	checkerDark  = 153 // RGB(153,153,153)
)

// tilesetGridSize is the number of tiles per row/column in a tileset image (16×16 grid).
const tilesetGridSize = 16

// Thumbnail generates a thumbnail of the map with the given maximum bounding box.
// The output image is scaled to fit within maxWidth × maxHeight while
// preserving the aspect ratio.
//
// The rendering approach:
//   - Tile and quad layers from groups with parallax 100/100 and offset 0/0
//   - The image is cropped to the bounding box of non-air tiles
//   - Tile flags (vflip, hflip, rotate) are handled
//   - Layer colors modulate the tileset/texture pixels
//   - Quads are rasterized with barycentric interpolation (vertex colors + textures)
//   - Physics/special layers and detail layers are excluded
func Render(r io.Reader, maxWidth, maxHeight int) (*image.NRGBA, error) {
	m, err := Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return RenderMap(m, maxWidth, maxHeight)
}

// renderLayer is a collected tile layer ready for rendering.
type renderLayer struct {
	color   color.NRGBA
	imageID int // -1 = no image (use white)
	tiles   []Tile
	width   int
	height  int
}

// renderQuadLayer is a collected quad layer ready for rendering.
type renderQuadLayer struct {
	quads   []Quad
	imageID int // -1 = no image (vertex colors only)
}

// renderStep represents an ordered rendering operation (either tile or quad layer).
// Layers are rendered back-to-front in the order they appear in the map.
type renderStep struct {
	isTile bool
	tile   renderLayer
	quad   renderQuadLayer
}

// RenderThumbnail generates a thumbnail from an already-parsed Map.
func RenderMap(m *Map, maxWidth, maxHeight int) (*image.NRGBA, error) {
	if maxWidth <= 0 || maxHeight <= 0 {
		return nil, fmt.Errorf("invalid bounding box %dx%d", maxWidth, maxHeight)
	}

	// ── 1. Collect all renderable layers (tiles + quads) in order ────────
	steps := collectRenderSteps(m)
	if len(steps) == 0 {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil
	}

	// ── 2. Crop to non-air tile bounding box ─────────────────────────────
	// Tiles define the map extent; quads are clipped to this area.
	tileLayers := extractTileLayers(steps)
	crop := cropToNonAir(tileLayers)
	if crop.maxX <= crop.minX || crop.maxY <= crop.minY {
		return image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil
	}
	cropW := crop.maxX - crop.minX
	cropH := crop.maxY - crop.minY

	// ── 3. Determine tile resolution ─────────────────────────────────────
	targetSize := uint32(math.Max(float64(maxWidth), float64(maxHeight)))
	tileLen := scaleTileLen(cropW, cropH, targetSize)

	// ── 4. Prepare tilesets and quad images ───────────────────────────────
	tilesets := prepareTilesets(m, tileLayers, tileLen)
	quadImages := prepareQuadImages(m, steps)

	// ── 5. Render all layers onto intermediate image ─────────────────────
	imgW := cropW * tileLen
	imgH := cropH * tileLen
	canvas := image.NewNRGBA(image.Rect(0, 0, int(imgW), int(imgH)))
	fillCheckerboard(canvas, tileLen)
	renderAllSteps(canvas, steps, tilesets, quadImages, &crop, tileLen)

	// ── 6. Scale to output bounding box ──────────────────────────────────
	outW, outH := fitInBoundingBox(int(cropW), int(cropH), maxWidth, maxHeight)
	out := scaleNRGBA(canvas, outW, outH)
	return out, nil
}

// collectRenderSteps collects all renderable layers (tiles and quads) in
// back-to-front order from groups with parallax 100/100 and offset 0/0.
func collectRenderSteps(m *Map) []renderStep {
	var steps []renderStep
	for i := range m.Groups {
		g := &m.Groups[i]

		if g.ParallaxX != 100 || g.ParallaxY != 100 || g.OffsetX != 0 || g.OffsetY != 0 || g.Clipping {
			continue
		}

		for j := range g.Layers {
			l := &g.Layers[j]
			if l.IsPhysics() || l.Detail {
				continue
			}

			switch l.Kind {
			case LayerKindTiles:
				if len(l.Tiles) == 0 {
					continue
				}
				steps = append(steps, renderStep{
					isTile: true,
					tile: renderLayer{
						color: color.NRGBA{
							R: l.ColorR,
							G: l.ColorG,
							B: l.ColorB,
							A: l.ColorA,
						},
						imageID: l.ImageID,
						tiles:   l.Tiles,
						width:   l.Width,
						height:  l.Height,
					},
				})
			case LayerKindQuads:
				if len(l.Quads) == 0 {
					continue
				}
				steps = append(steps, renderStep{
					quad: renderQuadLayer{
						quads:   l.Quads,
						imageID: l.QuadImageID,
					},
				})
			}
		}
	}
	return steps
}

// extractTileLayers returns only the tile renderLayers from the render steps.
func extractTileLayers(steps []renderStep) []renderLayer {
	var layers []renderLayer
	for _, s := range steps {
		if s.isTile {
			layers = append(layers, s.tile)
		}
	}
	return layers
}

// tileRect is an axis-aligned bounding box in tile coordinates.
type tileRect struct {
	minX, minY, maxX, maxY uint32
}

// cropToNonAir computes the bounding box of all non-air tiles across all layers.
// Uses edge scanning: finds the first/last non-air row and column per layer
// instead of scanning every tile.
func cropToNonAir(layers []renderLayer) tileRect {
	r := tileRect{
		minX: math.MaxUint32,
		minY: math.MaxUint32,
	}
	for _, l := range layers {
		if len(l.tiles) == 0 {
			continue
		}
		w, h := l.width, l.height

		// Find minY: first row with a non-air tile
		var lminY, lmaxY, lminX, lmaxX int
		lminY = h // sentinel
		for y := 0; y < h; y++ {
			row := y * w
			for x := 0; x < w; x++ {
				if row+x < len(l.tiles) && l.tiles[row+x].ID != 0 {
					lminY = y
					goto foundMinY
				}
			}
		}
		continue // no non-air tiles in this layer
	foundMinY:

		// Find maxY: last row with a non-air tile
		lmaxY = lminY + 1
		for y := h - 1; y > lminY; y-- {
			row := y * w
			for x := 0; x < w; x++ {
				if row+x < len(l.tiles) && l.tiles[row+x].ID != 0 {
					lmaxY = y + 1
					goto foundMaxY
				}
			}
		}
	foundMaxY:

		// Find minX: first column with a non-air tile
		lminX = w
		for x := 0; x < w; x++ {
			for y := lminY; y < lmaxY; y++ {
				idx := y*w + x
				if idx < len(l.tiles) && l.tiles[idx].ID != 0 {
					lminX = x
					goto foundMinX
				}
			}
		}
	foundMinX:

		// Find maxX: last column with a non-air tile
		lmaxX = lminX + 1
		for x := w - 1; x > lminX; x-- {
			for y := lminY; y < lmaxY; y++ {
				idx := y*w + x
				if idx < len(l.tiles) && l.tiles[idx].ID != 0 {
					lmaxX = x + 1
					goto foundMaxX
				}
			}
		}
	foundMaxX:

		if uint32(lminX) < r.minX {
			r.minX = uint32(lminX)
		}
		if uint32(lminY) < r.minY {
			r.minY = uint32(lminY)
		}
		if uint32(lmaxX) > r.maxX {
			r.maxX = uint32(lmaxX)
		}
		if uint32(lmaxY) > r.maxY {
			r.maxY = uint32(lmaxY)
		}
	}
	return r
}

// scaleTileLen determines the pixel resolution per tile so that the intermediate
// image stays manageable. Starts at 64 and halves until the total pixel count
// fits within 2× the output area (down from 4×), trading minor quality for
// significantly less rendering and scaling work.
func scaleTileLen(cropW, cropH, targetSize uint32) uint32 {
	tileLen := uint32(64)
	for tileLen > 1 {
		pixels := uint64(tileLen) * uint64(tileLen) * uint64(cropW) * uint64(cropH)
		budget := uint64(2) * uint64(targetSize) * uint64(targetSize)
		if pixels <= budget {
			break
		}
		tileLen /= 2
	}
	return tileLen
}

// prepareTilesets scales each referenced tileset image to tileLen×16 square
// and clears the air tile (index 0). Returns a map from imageID → scaled NRGBA.
func prepareTilesets(m *Map, layers []renderLayer, tileLen uint32) map[int]*image.NRGBA {
	needed := map[int]bool{}
	for _, l := range layers {
		needed[l.imageID] = true
	}

	tilesets := make(map[int]*image.NRGBA, len(needed))
	resultSide := int(tileLen * tilesetGridSize)

	for imgID := range needed {
		if imgID < 0 || imgID >= len(m.Images) {
			// No image: use solid white tileset
			white := image.NewNRGBA(image.Rect(0, 0, resultSide, resultSide))
			for i := 0; i < len(white.Pix); i += 4 {
				white.Pix[i] = 255
				white.Pix[i+1] = 255
				white.Pix[i+2] = 255
				white.Pix[i+3] = 255
			}
			clearAirTile(white, tileLen)
			tilesets[imgID] = white
			continue
		}

		src := m.Images[imgID]
		if src.RGBA == nil || src.Width == 0 || src.Height == 0 {
			// External image not available: solid white
			white := image.NewNRGBA(image.Rect(0, 0, resultSide, resultSide))
			for i := 0; i < len(white.Pix); i += 4 {
				white.Pix[i] = 255
				white.Pix[i+1] = 255
				white.Pix[i+2] = 255
				white.Pix[i+3] = 255
			}
			clearAirTile(white, tileLen)
			tilesets[imgID] = white
			continue
		}

		// Scale tileset to resultSide × resultSide using area averaging
		scaled := image.NewNRGBA(image.Rect(0, 0, resultSide, resultSide))
		draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), src.RGBA, src.RGBA.Bounds(), draw.Src, nil)

		// Clear air tile (top-left tile)
		clearAirTile(scaled, tileLen)
		tilesets[imgID] = scaled
	}

	return tilesets
}

// clearAirTile zeros the top-left tile (index 0) in a scaled tileset.
func clearAirTile(img *image.NRGBA, tileLen uint32) {
	tl := int(tileLen)
	for y := 0; y < tl; y++ {
		off := y * img.Stride
		for x := 0; x < tl; x++ {
			p := off + x*4
			img.Pix[p] = 0
			img.Pix[p+1] = 0
			img.Pix[p+2] = 0
			img.Pix[p+3] = 0
		}
	}
}

// prepareQuadImages returns full-resolution images for use by quad layers.
func prepareQuadImages(m *Map, steps []renderStep) map[int]*image.NRGBA {
	needed := map[int]bool{}
	for _, s := range steps {
		if !s.isTile {
			needed[s.quad.imageID] = true
		}
	}

	images := make(map[int]*image.NRGBA, len(needed))
	for imgID := range needed {
		if imgID < 0 || imgID >= len(m.Images) {
			continue
		}
		src := m.Images[imgID]
		if src.RGBA == nil {
			continue
		}
		images[imgID] = src.RGBA
	}
	return images
}

// renderAllSteps composites all layers onto canvas in order, handling both
// tile layers and quad layers.
func renderAllSteps(
	canvas *image.NRGBA,
	steps []renderStep,
	tilesets map[int]*image.NRGBA,
	quadImages map[int]*image.NRGBA,
	crop *tileRect,
	tileLen uint32,
) {
	for i := range steps {
		if steps[i].isTile {
			renderSingleTileLayer(canvas, &steps[i].tile, tilesets, crop, tileLen)
		} else {
			renderSingleQuadLayer(canvas, &steps[i].quad, quadImages, crop, tileLen)
		}
	}
}

// renderSingleTileLayer composites one tile layer onto canvas.
func renderSingleTileLayer(
	canvas *image.NRGBA,
	l *renderLayer,
	tilesets map[int]*image.NRGBA,
	crop *tileRect,
	tileLen uint32,
) {
	tl := int(tileLen)
	canvasPix := canvas.Pix
	canvasStride := canvas.Stride

	tileset := tilesets[l.imageID]
	if tileset == nil {
		return
	}
	tsPix := tileset.Pix
	tsStride := tileset.Stride
	tsPixLen := len(tsPix)

	colorIsWhite := l.color.R == 255 && l.color.G == 255 && l.color.B == 255 && l.color.A == 255
	lcR := uint32(l.color.R)
	lcG := uint32(l.color.G)
	lcB := uint32(l.color.B)
	lcA := uint32(l.color.A)

	layerMaxY := l.height
	if uint32(layerMaxY) > crop.maxY {
		layerMaxY = int(crop.maxY)
	}
	layerMaxX := l.width
	if uint32(layerMaxX) > crop.maxX {
		layerMaxX = int(crop.maxX)
	}

	minY := int(crop.minY)
	minX := int(crop.minX)

	tlBytes := tl * 4 // bytes per tile row in NRGBA

	for layerY := minY; layerY < layerMaxY; layerY++ {
		for layerX := minX; layerX < layerMaxX; layerX++ {
			idx := layerY*l.width + layerX
			if idx >= len(l.tiles) {
				continue
			}
			tile := l.tiles[idx]
			if tile.ID == 0 {
				continue
			}

			tileX := int(tile.ID) % tilesetGridSize
			tileY := int(tile.ID) / tilesetGridSize

			baseDstY := (layerY - minY) * tl
			baseDstX := (layerX - minX) * tl
			baseSrcX := tileX * tl
			baseSrcY := tileY * tl

			// Fast path: no flags + white color → row-copy from tileset
			if tile.Flags == 0 && colorIsWhite {
				for iy := 0; iy < tl; iy++ {
					srcRowOff := (baseSrcY+iy)*tsStride + baseSrcX*4
					if srcRowOff < 0 || srcRowOff+tlBytes > tsPixLen {
						continue
					}
					dstRowOff := (baseDstY+iy)*canvasStride + baseDstX*4
					srcRow := tsPix[srcRowOff : srcRowOff+tlBytes]
					dstRow := canvasPix[dstRowOff : dstRowOff+tlBytes]
					// Check if the entire row is fully opaque (all alpha == 255)
					allOpaque := true
					for p := 3; p < tlBytes; p += 4 {
						if srcRow[p] != 255 {
							allOpaque = false
							break
						}
					}
					if allOpaque {
						// Direct row copy (RGB only, alpha stays 255)
						copy(dstRow, srcRow)
						continue
					}
					// Mixed row: pixel-by-pixel with simplified blending
					for ix := 0; ix < tl; ix++ {
						sp := ix * 4
						pa := srcRow[sp+3]
						if pa == 0 {
							continue
						}
						dp := ix * 4
						if pa == 255 {
							dstRow[dp] = srcRow[sp]
							dstRow[dp+1] = srcRow[sp+1]
							dstRow[dp+2] = srcRow[sp+2]
							continue
						}
						sa := uint32(pa)
						inv := 255 - sa
						dstRow[dp] = uint8((uint32(srcRow[sp])*sa + uint32(dstRow[dp])*inv) / 255)
						dstRow[dp+1] = uint8((uint32(srcRow[sp+1])*sa + uint32(dstRow[dp+1])*inv) / 255)
						dstRow[dp+2] = uint8((uint32(srcRow[sp+2])*sa + uint32(dstRow[dp+2])*inv) / 255)
					}
				}
				continue
			}

			// Slow path: flags or color modulation required
			flags := tile.Flags
			rotate := flags&TileFlagRotate != 0
			vflip := flags&TileFlagVFlip != 0
			hflip := flags&TileFlagHFlip != 0
			last := tileLen - 1

			for iy := 0; iy < tl; iy++ {
				dstRowOff := (baseDstY + iy) * canvasStride
				for ix := 0; ix < tl; ix++ {
					ty := uint32(iy)
					tx := uint32(ix)
					if rotate {
						ty, tx = last-tx, ty
					}
					if vflip {
						tx = last - tx
					}
					if hflip {
						ty = last - ty
					}

					srcOff := (baseSrcY+int(ty))*tsStride + (baseSrcX+int(tx))*4
					if srcOff < 0 || srcOff+3 >= tsPixLen {
						continue
					}

					var pr, pg, pb, pa uint8
					if colorIsWhite {
						pr = tsPix[srcOff]
						pg = tsPix[srcOff+1]
						pb = tsPix[srcOff+2]
						pa = tsPix[srcOff+3]
					} else {
						pr = uint8(uint32(tsPix[srcOff]) * lcR / 255)
						pg = uint8(uint32(tsPix[srcOff+1]) * lcG / 255)
						pb = uint8(uint32(tsPix[srcOff+2]) * lcB / 255)
						pa = uint8(uint32(tsPix[srcOff+3]) * lcA / 255)
					}

					if pa == 0 {
						continue
					}

					dstOff := dstRowOff + (baseDstX+ix)*4
					if pa == 255 {
						canvasPix[dstOff] = pr
						canvasPix[dstOff+1] = pg
						canvasPix[dstOff+2] = pb
						continue
					}

					sa := uint32(pa)
					inv := 255 - sa
					canvasPix[dstOff] = uint8((uint32(pr)*sa + uint32(canvasPix[dstOff])*inv) / 255)
					canvasPix[dstOff+1] = uint8((uint32(pg)*sa + uint32(canvasPix[dstOff+1])*inv) / 255)
					canvasPix[dstOff+2] = uint8((uint32(pb)*sa + uint32(canvasPix[dstOff+2])*inv) / 255)
				}
			}
		}
	}
}

// renderSingleQuadLayer composites one quad layer onto canvas.
func renderSingleQuadLayer(
	canvas *image.NRGBA,
	ql *renderQuadLayer,
	quadImages map[int]*image.NRGBA,
	crop *tileRect,
	tileLen uint32,
) {
	tex := quadImages[ql.imageID] // may be nil (vertex colors only)
	for i := range ql.quads {
		renderQuadOnCanvas(canvas, &ql.quads[i], tex, crop, tileLen)
	}
}

// renderQuadOnCanvas rasterizes a single quad as two triangles.
// Quad vertex layout (DDNet convention):
//
//	[0]=TL  [1]=TR
//	[2]=BL  [3]=BR
//
// Triangulation: (0,1,2) and (0,2,3), matching OpenGL GL_QUADS.
func renderQuadOnCanvas(
	canvas *image.NRGBA,
	q *Quad,
	tex *image.NRGBA,
	crop *tileRect,
	tileLen uint32,
) {
	tl := float64(tileLen)
	cropMinX := float64(crop.minX)
	cropMinY := float64(crop.minY)

	// Convert quad corner positions from tile coords to canvas pixel coords
	var px, py [4]float64
	for i := 0; i < 4; i++ {
		px[i] = (q.Points[i].X - cropMinX) * tl
		py[i] = (q.Points[i].Y - cropMinY) * tl
	}

	// Texture coords (normalized [0,1])
	var u, v [4]float64
	for i := 0; i < 4; i++ {
		u[i] = q.TexCoords[i].X
		v[i] = q.TexCoords[i].Y
	}

	// Triangle 1: vertices 0, 1, 2
	rasterizeTriangle(canvas, tex,
		px[0], py[0], u[0], v[0], q.Colors[0],
		px[1], py[1], u[1], v[1], q.Colors[1],
		px[2], py[2], u[2], v[2], q.Colors[2],
	)
	// Triangle 2: vertices 0, 2, 3
	rasterizeTriangle(canvas, tex,
		px[0], py[0], u[0], v[0], q.Colors[0],
		px[2], py[2], u[2], v[2], q.Colors[2],
		px[3], py[3], u[3], v[3], q.Colors[3],
	)
}

// rasterizeTriangle renders a textured, vertex-colored triangle onto canvas
// using scanline rasterization with barycentric interpolation.
func rasterizeTriangle(
	canvas *image.NRGBA,
	tex *image.NRGBA,
	px0, py0, u0, v0 float64, c0 color.NRGBA,
	px1, py1, u1, v1 float64, c1 color.NRGBA,
	px2, py2, u2, v2 float64, c2 color.NRGBA,
) {
	// Signed area (2×)
	area := (px1-px0)*(py2-py0) - (py1-py0)*(px2-px0)
	if area == 0 {
		return
	}
	invArea := 1.0 / area

	// Bounding box clipped to canvas
	bounds := canvas.Bounds()
	minPX := math.Min(px0, math.Min(px1, px2))
	maxPX := math.Max(px0, math.Max(px1, px2))
	minPY := math.Min(py0, math.Min(py1, py2))
	maxPY := math.Max(py0, math.Max(py1, py2))

	startX := int(math.Floor(minPX))
	endX := int(math.Ceil(maxPX))
	startY := int(math.Floor(minPY))
	endY := int(math.Ceil(maxPY))

	if startX < bounds.Min.X {
		startX = bounds.Min.X
	}
	if startY < bounds.Min.Y {
		startY = bounds.Min.Y
	}
	if endX > bounds.Max.X {
		endX = bounds.Max.X
	}
	if endY > bounds.Max.Y {
		endY = bounds.Max.Y
	}
	if startX >= endX || startY >= endY {
		return
	}

	canvasPix := canvas.Pix
	canvasStride := canvas.Stride

	var texPix []uint8
	var texStride, texW, texH int
	if tex != nil {
		texPix = tex.Pix
		texStride = tex.Stride
		tb := tex.Bounds()
		texW = tb.Dx()
		texH = tb.Dy()
	}

	// Pre-convert vertex colors to float64
	cr0, cg0, cb0, ca0 := float64(c0.R), float64(c0.G), float64(c0.B), float64(c0.A)
	cr1, cg1, cb1, ca1 := float64(c1.R), float64(c1.G), float64(c1.B), float64(c1.A)
	cr2, cg2, cb2, ca2 := float64(c2.R), float64(c2.G), float64(c2.B), float64(c2.A)

	for y := startY; y < endY; y++ {
		pyc := float64(y) + 0.5
		for x := startX; x < endX; x++ {
			pxc := float64(x) + 0.5

			// Barycentric coordinates via edge functions
			w0 := ((px1-pxc)*(py2-pyc) - (py1-pyc)*(px2-pxc)) * invArea
			w1 := ((px2-pxc)*(py0-pyc) - (py2-pyc)*(px0-pxc)) * invArea
			w2 := 1.0 - w0 - w1

			// Inside test (handle CW and CCW winding)
			if area > 0 {
				if w0 < 0 || w1 < 0 || w2 < 0 {
					continue
				}
			} else {
				if w0 > 0 || w1 > 0 || w2 > 0 {
					continue
				}
				w0, w1, w2 = -w0, -w1, -w2
			}

			// Interpolate vertex colors
			fR := w0*cr0 + w1*cr1 + w2*cr2
			fG := w0*cg0 + w1*cg1 + w2*cg2
			fB := w0*cb0 + w1*cb1 + w2*cb2
			fA := w0*ca0 + w1*ca1 + w2*ca2

			// Sample texture
			var tR, tG, tB, tA uint8
			if tex != nil {
				tu := w0*u0 + w1*u1 + w2*u2
				tv := w0*v0 + w1*v1 + w2*v2
				txp := int(tu * float64(texW))
				typ := int(tv * float64(texH))
				if txp < 0 {
					txp = 0
				}
				if typ < 0 {
					typ = 0
				}
				if txp >= texW {
					txp = texW - 1
				}
				if typ >= texH {
					typ = texH - 1
				}
				off := typ*texStride + txp*4
				tR = texPix[off]
				tG = texPix[off+1]
				tB = texPix[off+2]
				tA = texPix[off+3]
			} else {
				tR, tG, tB, tA = 255, 255, 255, 255
			}

			// Modulate: pixel = vertex_color × texture_color / 255
			pr := uint8(clampF64(fR * float64(tR) / 255.0))
			pg := uint8(clampF64(fG * float64(tG) / 255.0))
			pb := uint8(clampF64(fB * float64(tB) / 255.0))
			pa := uint8(clampF64(fA * float64(tA) / 255.0))

			if pa == 0 {
				continue
			}

			// Source-over compositing (opaque canvas: dst.A always 255)
			dstOff := y*canvasStride + x*4
			if pa == 255 {
				canvasPix[dstOff] = pr
				canvasPix[dstOff+1] = pg
				canvasPix[dstOff+2] = pb
				// canvasPix[dstOff+3] stays 255
				continue
			}

			sa := uint32(pa)
			inv := 255 - sa
			canvasPix[dstOff] = uint8((uint32(pr)*sa + uint32(canvasPix[dstOff])*inv) / 255)
			canvasPix[dstOff+1] = uint8((uint32(pg)*sa + uint32(canvasPix[dstOff+1])*inv) / 255)
			canvasPix[dstOff+2] = uint8((uint32(pb)*sa + uint32(canvasPix[dstOff+2])*inv) / 255)
			// canvasPix[dstOff+3] stays 255
		}
	}
}

func clampF64(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// fitInBoundingBox calculates the largest size that fits in maxW × maxH
// while preserving the aspect ratio of srcW × srcH.
func fitInBoundingBox(srcW, srcH, maxW, maxH int) (int, int) {
	if srcW <= 0 || srcH <= 0 {
		return maxW, maxH
	}
	wScale := float64(maxW) / float64(srcW)
	hScale := float64(maxH) / float64(srcH)
	scale := math.Min(wScale, hScale)
	w := int(math.Round(float64(srcW) * scale))
	h := int(math.Round(float64(srcH) * scale))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// scaleNRGBA performs bilinear interpolation on RGB channels only (alpha is
// always 255 on our opaque canvas), avoiding per-pixel interface dispatch.
func scaleNRGBA(src *image.NRGBA, dstW, dstH int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, dstW, dstH))
	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	if srcW == 0 || srcH == 0 || dstW == 0 || dstH == 0 {
		return dst
	}

	srcPix := src.Pix
	srcStride := src.Stride
	dstPix := dst.Pix
	dstStride := dst.Stride

	xRatio := float64(srcW) / float64(dstW)
	yRatio := float64(srcH) / float64(dstH)

	lastSrcX := srcW - 1
	lastSrcY := srcH - 1

	for dy := 0; dy < dstH; dy++ {
		sy := (float64(dy)+0.5)*yRatio - 0.5
		sy0 := int(sy)
		if sy0 < 0 {
			sy0 = 0
		}
		sy1 := sy0 + 1
		if sy1 > lastSrcY {
			sy1 = lastSrcY
		}
		fy := sy - float64(sy0)
		if fy < 0 {
			fy = 0
		}
		fy1 := uint32(fy * 256)
		fy0 := 256 - fy1

		srcRow0 := sy0 * srcStride
		srcRow1 := sy1 * srcStride
		dstRow := dy * dstStride

		for dx := 0; dx < dstW; dx++ {
			sx := (float64(dx)+0.5)*xRatio - 0.5
			sx0 := int(sx)
			if sx0 < 0 {
				sx0 = 0
			}
			sx1 := sx0 + 1
			if sx1 > lastSrcX {
				sx1 = lastSrcX
			}
			fx := sx - float64(sx0)
			if fx < 0 {
				fx = 0
			}
			fx1 := uint32(fx * 256)
			fx0 := 256 - fx1

			// Four source pixel offsets
			off00 := srcRow0 + sx0*4
			off10 := srcRow0 + sx1*4
			off01 := srcRow1 + sx0*4
			off11 := srcRow1 + sx1*4

			// Bilinear interpolation for RGB only (alpha stays 255)
			r := (uint32(srcPix[off00])*fx0*fy0 + uint32(srcPix[off10])*fx1*fy0 +
				uint32(srcPix[off01])*fx0*fy1 + uint32(srcPix[off11])*fx1*fy1 + 32768) >> 16
			g := (uint32(srcPix[off00+1])*fx0*fy0 + uint32(srcPix[off10+1])*fx1*fy0 +
				uint32(srcPix[off01+1])*fx0*fy1 + uint32(srcPix[off11+1])*fx1*fy1 + 32768) >> 16
			b := (uint32(srcPix[off00+2])*fx0*fy0 + uint32(srcPix[off10+2])*fx1*fy0 +
				uint32(srcPix[off01+2])*fx0*fy1 + uint32(srcPix[off11+2])*fx1*fy1 + 32768) >> 16

			dOff := dstRow + dx*4
			dstPix[dOff] = uint8(r)
			dstPix[dOff+1] = uint8(g)
			dstPix[dOff+2] = uint8(b)
			dstPix[dOff+3] = 255
		}
	}
	return dst
}

// fillCheckerboard paints the DDNet editor checkerboard background onto canvas.
// Each checker square is checkerSize×checkerSize pixels (16 tiles wide).
func fillCheckerboard(canvas *image.NRGBA, tileLen uint32) {
	// DDNet uses 32px checker cells at the default view. With our tile
	// resolution that maps to tileLen pixels per tile, we use 16-tile-wide
	// squares to match the editor feel.
	checkerSize := int(tileLen) * 16
	if checkerSize < 1 {
		checkerSize = 1
	}

	bounds := canvas.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()
	pix := canvas.Pix
	stride := canvas.Stride

	for y := 0; y < imgH; y++ {
		cy := (y / checkerSize) & 1
		rowOff := y * stride
		for x := 0; x < imgW; x++ {
			cx := (x / checkerSize) & 1
			off := rowOff + x*4
			if (cx ^ cy) == 0 {
				pix[off] = checkerLight
				pix[off+1] = checkerLight
				pix[off+2] = checkerLight
			} else {
				pix[off] = checkerDark
				pix[off+1] = checkerDark
				pix[off+2] = checkerDark
			}
			pix[off+3] = 255
		}
	}
}
