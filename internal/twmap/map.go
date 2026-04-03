package twmap

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
)

// ── map item type IDs (from datafile spec) ───────────────────────────────────

const (
	mapItemTypeVersion uint16 = 0
	mapItemTypeInfo    uint16 = 1
	mapItemTypeImage   uint16 = 2
	mapItemTypeGroup   uint16 = 4
	mapItemTypeLayer   uint16 = 5
)

// ── layer type constants ─────────────────────────────────────────────────────

const (
	layerTypeTilemap            int32 = 2
	layerTypeQuads              int32 = 3
	layerTypeDdraceSoundsLegacy int32 = 9
	layerTypeDdraceSounds       int32 = 10
)

// ── tile layer flag constants ────────────────────────────────────────────────

const (
	tileLayerFlagGame    uint32 = 1
	tileLayerFlagTele    uint32 = 2
	tileLayerFlagSpeedup uint32 = 4
	tileLayerFlagFront   uint32 = 8
	tileLayerFlagSwitch  uint32 = 16
	tileLayerFlagTune    uint32 = 32
)

// ── Tile is 4 bytes: id, flags, skip, unused ─────────────────────────────────

// Tile represents a single tile in a tilemap layer.
type Tile struct {
	ID    uint8
	Flags uint8
}

// TileFlags
const (
	TileFlagVFlip  uint8 = 1
	TileFlagHFlip  uint8 = 2
	TileFlagOpaque uint8 = 4
	TileFlagRotate uint8 = 8
)

// ── Quad ─────────────────────────────────────────────────────────────────────

// Quad represents a single quad in a quads layer.
// Positions use 17.15 fixed-point.
type Quad struct {
	Points    [5]Point // corners[4] + position
	Colors    [4]color.NRGBA
	TexCoords [4]Point
}

// Point is a 2D coordinate (fixed-point 17.15 in the datafile).
type Point struct {
	X, Y float64
}

// ── Map model (mirrors ddnet-rs/twmap TwMap) ─────────────────────────────────

// Map holds all parsed information from a Teeworlds/DDNet map file.
type Map struct {
	Info   Info
	Images []Image
	Groups []Group
}

// Info holds map metadata.
type Info struct {
	Author  string
	Version string
	Credits string
	License string
}

// Image is either embedded (with RGBA pixel data) or external (name only).
type Image struct {
	Name     string
	Width    int
	Height   int
	External bool
	RGBA     *image.NRGBA // nil for external images
}

// Group is a collection of layers with a shared offset/parallax.
type Group struct {
	Name      string
	OffsetX   int32
	OffsetY   int32
	ParallaxX int32
	ParallaxY int32
	Clipping  bool
	ClipX     int32
	ClipY     int32
	ClipW     int32
	ClipH     int32
	Layers    []Layer
}

// IsPhysicsGroup returns true if this group contains any physics layers.
func (g *Group) IsPhysicsGroup() bool {
	for _, l := range g.Layers {
		if l.IsPhysics() {
			return true
		}
	}
	return false
}

// LayerKind identifies the type of layer.
type LayerKind int

const (
	LayerKindTiles LayerKind = iota
	LayerKindGame
	LayerKindFront
	LayerKindTele
	LayerKindSpeedup
	LayerKindSwitch
	LayerKindTune
	LayerKindQuads
	LayerKindSounds
	LayerKindInvalid
)

// Layer represents a single layer in a group.
type Layer struct {
	Kind   LayerKind
	Detail bool

	// Tilemap fields (used when Kind is tiles/game/front/tele/speedup/switch/tune)
	Width   int
	Height  int
	ColorR  uint8
	ColorG  uint8
	ColorB  uint8
	ColorA  uint8
	ImageID int // -1 = no image
	Tiles   []Tile

	// Quads fields (used when Kind is LayerKindQuads)
	Quads       []Quad
	QuadImageID int // -1 = no image

	// Name (v3+ only)
	Name string
}

// IsPhysics returns true for game/front/tele/speedup/switch/tune layers.
func (l *Layer) IsPhysics() bool {
	switch l.Kind {
	case LayerKindGame, LayerKindFront, LayerKindTele, LayerKindSpeedup, LayerKindSwitch, LayerKindTune:
		return true
	}
	return false
}

// IsTilemap returns true for any tilemap-based layer.
func (l *Layer) IsTilemap() bool {
	switch l.Kind {
	case LayerKindTiles, LayerKindGame, LayerKindFront, LayerKindTele, LayerKindSpeedup, LayerKindSwitch, LayerKindTune:
		return true
	}
	return false
}

// ── Parse: construct Map from an io.Reader ───────────────────────────────────

// Parse reads and parses a Teeworlds/DDNet map from r.
// It fully decodes all tile data and embedded images so the returned Map
// can be used for thumbnail generation.
func Parse(r io.Reader) (*Map, error) {
	df, err := parseDatafile(r)
	if err != nil {
		return nil, fmt.Errorf("datafile: %w", err)
	}
	return parseMap(df)
}

// ParseInfo reads only the metadata (author, version, credits, license)
// from a Teeworlds/DDNet map without decoding images or layers.
func ParseInfo(r io.Reader) (Info, error) {
	df, err := parseDatafile(r)
	if err != nil {
		return Info{}, fmt.Errorf("datafile: %w", err)
	}
	if err := checkMapVersion(df); err != nil {
		return Info{}, err
	}
	return parseInfo(df)
}

func parseMap(df *datafile) (*Map, error) {
	m := &Map{}

	// ── version ──────────────────────────────────────────────────────────
	if err := checkMapVersion(df); err != nil {
		return nil, err
	}

	// ── info ─────────────────────────────────────────────────────────────
	info, err := parseInfo(df)
	if err != nil {
		return nil, err
	}
	m.Info = info

	// ── images ───────────────────────────────────────────────────────────
	images, err := parseImages(df)
	if err != nil {
		return nil, err
	}
	m.Images = images

	// ── groups & layers ──────────────────────────────────────────────────
	groups, err := parseGroups(df)
	if err != nil {
		return nil, err
	}
	m.Groups = groups

	return m, nil
}

// ── version check ────────────────────────────────────────────────────────────

func checkMapVersion(df *datafile) error {
	item := df.findItem(mapItemTypeVersion, 0)
	if item == nil {
		return ErrMissingVersion
	}
	if len(item.Data) < 1 {
		return fmt.Errorf("%w: empty version item", ErrInvalidVersion)
	}
	version := item.Data[0]
	if version != 1 {
		return fmt.Errorf("%w: got %d, want 1", ErrInvalidVersion, version)
	}
	return nil
}

// ── info parsing ─────────────────────────────────────────────────────────────

func parseInfo(df *datafile) (Info, error) {
	item := df.findItem(mapItemTypeInfo, 0)
	if item == nil {
		return Info{}, ErrMissingInfo
	}
	if len(item.Data) < 5 {
		return Info{}, fmt.Errorf("info item too short: %d int32s, want >= 5", len(item.Data))
	}

	readStr := func(dataIdx int32) string {
		if dataIdx < 0 || int(dataIdx) >= df.numData {
			return ""
		}
		data, err := df.readData(int(dataIdx))
		if err != nil || len(data) == 0 {
			return ""
		}
		// c-string: strip trailing NUL
		if data[len(data)-1] == 0 {
			data = data[:len(data)-1]
		}
		return string(data)
	}

	return Info{
		Author:  readStr(item.Data[1]),
		Version: readStr(item.Data[2]),
		Credits: readStr(item.Data[3]),
		License: readStr(item.Data[4]),
	}, nil
}

// ── image parsing ────────────────────────────────────────────────────────────

func parseImages(df *datafile) ([]Image, error) {
	items := df.itemsOfType(mapItemTypeImage)
	images := make([]Image, 0, len(items))

	for i, item := range items {
		if len(item.Data) < 6 {
			return nil, fmt.Errorf("image %d: too short (%d int32s)", i, len(item.Data))
		}
		ver := item.Data[0]
		if ver < 1 {
			return nil, fmt.Errorf("image %d: invalid version %d", i, ver)
		}

		width := int(item.Data[1])
		height := int(item.Data[2])
		external := item.Data[3] != 0
		nameIdx := item.Data[4]
		dataIdx := item.Data[5]

		// Validate embedded image dimensions
		if !external {
			if width <= 0 {
				return nil, fmt.Errorf("image %d: invalid width %d", i, width)
			}
			if height <= 0 {
				return nil, fmt.Errorf("image %d: invalid height %d", i, height)
			}
		}

		// Read name
		name := ""
		if nameIdx >= 0 && int(nameIdx) < df.numData {
			nameData, err := df.readData(int(nameIdx))
			if err == nil && len(nameData) > 0 {
				if nameData[len(nameData)-1] == 0 {
					nameData = nameData[:len(nameData)-1]
				}
				name = string(nameData)
			}
		}

		img := Image{
			Name:     name,
			Width:    width,
			Height:   height,
			External: external,
		}

		// Decode embedded image data (RGBA pixels)
		if !external && dataIdx >= 0 && int(dataIdx) < df.numData {
			pixelData, err := df.readData(int(dataIdx))
			if err == nil && len(pixelData) == width*height*4 {
				nrgba := image.NewNRGBA(image.Rect(0, 0, width, height))
				copy(nrgba.Pix, pixelData)
				img.RGBA = nrgba
			}
		}

		images = append(images, img)
	}
	return images, nil
}

// ── group & layer parsing ────────────────────────────────────────────────────

func parseGroups(df *datafile) ([]Group, error) {
	groupItems := df.itemsOfType(mapItemTypeGroup)
	layerItems := df.itemsOfType(mapItemTypeLayer)

	var hasGame bool
	groups := make([]Group, 0, len(groupItems))

	for gi, gItem := range groupItems {
		if len(gItem.Data) < 7 {
			return nil, fmt.Errorf("group %d: too short (%d int32s)", gi, len(gItem.Data))
		}
		ver := gItem.Data[0]
		if ver < 1 {
			return nil, fmt.Errorf("group %d: invalid version %d", gi, ver)
		}

		g := Group{
			OffsetX:   gItem.Data[1],
			OffsetY:   gItem.Data[2],
			ParallaxX: gItem.Data[3],
			ParallaxY: gItem.Data[4],
		}

		startLayer := int(gItem.Data[5])
		numLayers := int(gItem.Data[6])

		if startLayer < 0 || numLayers < 0 {
			return nil, fmt.Errorf("group %d: invalid layer range", gi)
		}

		// v2: clipping
		if ver >= 2 && len(gItem.Data) >= 12 {
			g.Clipping = gItem.Data[7] != 0
			g.ClipX = gItem.Data[8]
			g.ClipY = gItem.Data[9]
			g.ClipW = gItem.Data[10]
			g.ClipH = gItem.Data[11]
		}

		// v3: name
		if ver >= 3 && len(gItem.Data) >= 15 {
			g.Name = parseI32String(gItem.Data[12:15])
		}

		// Parse layers belonging to this group
		absEnd := startLayer + numLayers
		if absEnd > len(layerItems) {
			return nil, fmt.Errorf("group %d: layer range [%d, %d) exceeds layer count %d", gi, startLayer, absEnd, len(layerItems))
		}

		for li := startLayer; li < absEnd; li++ {
			layer, err := parseLayer(df, &layerItems[li])
			if err != nil {
				return nil, fmt.Errorf("group %d, layer %d: %w", gi, li, err)
			}
			if layer.Kind == LayerKindGame {
				hasGame = true
			}
			g.Layers = append(g.Layers, layer)
		}

		groups = append(groups, g)
	}

	if !hasGame {
		return nil, ErrNoGameLayer
	}
	return groups, nil
}

func parseLayer(df *datafile, item *itemView) (Layer, error) {
	if len(item.Data) < 3 {
		return Layer{}, fmt.Errorf("layer too short (%d int32s)", len(item.Data))
	}

	layerType := item.Data[1]
	flags := uint32(item.Data[2])

	switch layerType {
	case layerTypeTilemap:
		return parseTilemapLayer(df, item.Data, flags)
	case layerTypeQuads:
		return parseQuadsLayer(df, item.Data)
	case layerTypeDdraceSounds, layerTypeDdraceSoundsLegacy:
		return Layer{Kind: LayerKindSounds}, nil
	default:
		return Layer{Kind: LayerKindInvalid}, nil
	}
}

func parseTilemapLayer(df *datafile, data []int32, layerFlags uint32) (Layer, error) {
	// data[0]=layerVersion, [1]=type, [2]=flags
	// data[3]=tilemapVersion, [4]=width, [5]=height, [6]=tileFlags
	// data[7..10]=color, [11]=colorEnv, [12]=colorEnvOffset, [13]=image, [14]=dataIdx
	if len(data) < 15 {
		return Layer{}, fmt.Errorf("tilemap layer too short (%d int32s)", len(data))
	}

	ver := data[3]
	if ver < 2 {
		return Layer{}, fmt.Errorf("unsupported tilemap version %d", ver)
	}

	width := int(data[4])
	height := int(data[5])
	tileFlags := uint32(data[6])

	if width <= 0 || height <= 0 {
		return Layer{}, fmt.Errorf("invalid dimensions %dx%d", width, height)
	}

	detail := layerFlags&1 != 0

	l := Layer{
		Width:   width,
		Height:  height,
		Detail:  detail,
		ColorR:  uint8(data[7]),
		ColorG:  uint8(data[8]),
		ColorB:  uint8(data[9]),
		ColorA:  uint8(data[10]),
		ImageID: int(data[13]),
	}

	// Determine kind from tile flags
	switch {
	case tileFlags&tileLayerFlagGame != 0:
		l.Kind = LayerKindGame
	case tileFlags&tileLayerFlagFront != 0:
		l.Kind = LayerKindFront
	case tileFlags&tileLayerFlagTele != 0:
		l.Kind = LayerKindTele
	case tileFlags&tileLayerFlagSpeedup != 0:
		l.Kind = LayerKindSpeedup
	case tileFlags&tileLayerFlagSwitch != 0:
		l.Kind = LayerKindSwitch
	case tileFlags&tileLayerFlagTune != 0:
		l.Kind = LayerKindTune
	default:
		l.Kind = LayerKindTiles
	}

	// Determine which data index to read based on version and kind.
	dataIdx := 14 // base data index
	if l.Kind == LayerKindTiles || l.Kind == LayerKindGame {
		// For regular tile layers and game layer, data is always at index 14
		dataIdx = 14
	} else if ver >= 3 {
		// v3+: name[3] at [15..17], then special data indices follow
		dataIdx = specialDataOffset(ver, tileFlags)
	} else {
		dataIdx = specialDataOffset(ver, tileFlags)
	}

	// v3: name
	if ver >= 3 && len(data) >= 18 {
		l.Name = parseI32String(data[15:18])
	}

	// Decode tile data
	if dataIdx >= 0 && dataIdx < len(data) {
		rawIdx := data[dataIdx]
		if rawIdx >= 0 && int(rawIdx) < df.numData {
			tileData, err := df.readData(int(rawIdx))
			if err == nil {
				l.Tiles = decodeTiles(tileData, width*height, l.Kind, ver)
			}
		}
	}

	return l, nil
}

// specialDataOffset returns the item data offset for a special layer's data index.
func specialDataOffset(version int32, tileFlags uint32) int {
	var base int
	switch {
	case version >= 3:
		base = 18 // after name[3] at [15,16,17]
	case version >= 2:
		base = 15 // no name
	default:
		return -1
	}
	switch {
	case tileFlags&tileLayerFlagTele != 0:
		return base + 0
	case tileFlags&tileLayerFlagSpeedup != 0:
		return base + 1
	case tileFlags&tileLayerFlagFront != 0:
		return base + 2
	case tileFlags&tileLayerFlagSwitch != 0:
		return base + 3
	case tileFlags&tileLayerFlagTune != 0:
		return base + 4
	default:
		return 14
	}
}

// decodeTiles converts raw decompressed tile bytes into Tile structs.
// For game/tiles layers with version >= 4, data may be TW-compressed.
func decodeTiles(data []byte, expectedCount int, kind LayerKind, version int32) []Tile {
	// TW 0.7 compression: version >= 4, game/tiles layers only
	if version >= 4 && (kind == LayerKindGame || kind == LayerKindTiles) {
		data = decompressTWTiles(data)
	}

	// Standard tiles: 4 bytes per tile (id, flags, skip, unused)
	if len(data) < expectedCount*4 {
		return nil
	}
	tiles := make([]Tile, expectedCount)
	for i := 0; i < expectedCount; i++ {
		off := i * 4
		tiles[i] = Tile{
			ID:    data[off],
			Flags: data[off+1],
		}
	}
	return tiles
}

// decompressTWTiles implements Teeworlds' custom tile compression.
// Format: each 4-byte entry is [id, flags, skip_count, unused].
// "skip_count" means repeat the tile that many additional times.
func decompressTWTiles(data []byte) []byte {
	if len(data)%4 != 0 {
		return data
	}
	var out []byte
	for i := 0; i+3 < len(data); i += 4 {
		id := data[i]
		flags := data[i+1]
		skip := int(data[i+2])
		// write the tile once
		out = append(out, id, flags, 0, data[i+3])
		// then repeat it "skip" more times
		for j := 0; j < skip; j++ {
			out = append(out, id, flags, 0, data[i+3])
		}
	}
	return out
}

// parseI32String decodes a Teeworlds "3 x int32" encoded string (big-endian, XOR 128).
func parseI32String(nums []int32) string {
	buf := make([]byte, 0, len(nums)*4)
	for _, n := range nums {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(n))
		buf = append(buf, b[:]...)
	}
	// All zeros = empty
	allZero := true
	for _, b := range buf {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return ""
	}
	// Strip guaranteed trailing NUL
	if len(buf) > 0 && buf[len(buf)-1] == 0 {
		buf = buf[:len(buf)-1]
	}
	// XOR decode
	for i := range buf {
		buf[i] = byte(int(buf[i]) + 128)
	}
	// Strip trailing NULs
	for len(buf) > 0 && buf[len(buf)-1] == 0 {
		buf = buf[:len(buf)-1]
	}
	return string(buf)
}

func parseQuadsLayer(df *datafile, data []int32) (Layer, error) {
	// data[0]=layerVersion, [1]=type, [2]=flags
	// data[3]=quadsVersion, [4]=numQuads, [5]=dataIdx, [6]=imageIdx
	if len(data) < 7 {
		return Layer{}, fmt.Errorf("quads layer too short (%d int32s)", len(data))
	}

	ver := data[3]
	if ver < 1 {
		return Layer{}, fmt.Errorf("invalid quads version %d", ver)
	}

	numQuads := int(data[4])
	if numQuads < 0 {
		return Layer{}, fmt.Errorf("invalid num_quads %d", numQuads)
	}

	dataIdx := data[5]
	imageIdx := int(data[6])
	detail := uint32(data[2])&1 != 0

	l := Layer{
		Kind:        LayerKindQuads,
		Detail:      detail,
		QuadImageID: imageIdx,
	}

	// v2: name
	if ver >= 2 && len(data) >= 10 {
		l.Name = parseI32String(data[7:10])
	}

	// Decode quads
	if dataIdx >= 0 && int(dataIdx) < df.numData && numQuads > 0 {
		quadData, err := df.readData(int(dataIdx))
		if err == nil {
			l.Quads = decodeQuads(quadData, numQuads)
		}
	}

	return l, nil
}

const quadBinarySize = 152 // 5 positions * 8 + 4 colors * 4 + 4 texcoords * 8 + 4 env fields * 4

func decodeQuads(data []byte, numQuads int) []Quad {
	// Binary quad: 5 points (2*i32 each) = 40 bytes
	//   + 4 colors (4*i32 each) = 64 bytes
	//   + 4 tex coords (2*i32 each) = 32 bytes
	//   + position_env + position_env_offset + color_env + color_env_offset = 16 bytes
	//   = 152 bytes total
	const (
		pointSize = 8  // 2 * int32
		colorSize = 16 // 4 * int32
	)

	if len(data) < numQuads*quadBinarySize {
		return nil
	}

	quads := make([]Quad, numQuads)
	for qi := range numQuads {
		base := qi * quadBinarySize
		off := base

		var q Quad

		// 5 corners (4 corners + 1 position)
		for i := 0; i < 5; i++ {
			q.Points[i] = readPoint(data[off:])
			off += pointSize
		}

		// 4 colors
		for i := 0; i < 4; i++ {
			q.Colors[i] = readColor(data[off:])
			off += colorSize
		}

		// 4 texture coordinates
		for i := 0; i < 4; i++ {
			q.TexCoords[i] = readTexCoord(data[off:])
			off += pointSize
		}

		quads[qi] = q
	}
	return quads
}

// readPoint reads a 22.10 fixed-point 2D point, returning tile coordinates
// (world coords / 32). This is equivalent to dividing the raw int32 by 32768.
func readPoint(data []byte) Point {
	x := int32(binary.LittleEndian.Uint32(data[0:4]))
	y := int32(binary.LittleEndian.Uint32(data[4:8]))
	return Point{
		X: float64(x) / 32768.0,
		Y: float64(y) / 32768.0,
	}
}

// readTexCoord reads a 22.10 fixed-point texture coordinate, returning
// normalized [0,1] values (standard fx2f: raw / 1024).
func readTexCoord(data []byte) Point {
	x := int32(binary.LittleEndian.Uint32(data[0:4]))
	y := int32(binary.LittleEndian.Uint32(data[4:8]))
	return Point{
		X: float64(x) / 1024.0,
		Y: float64(y) / 1024.0,
	}
}

// readColor reads an RGBA color from 4 consecutive int32s.
func readColor(data []byte) color.NRGBA {
	r := int32(binary.LittleEndian.Uint32(data[0:4]))
	g := int32(binary.LittleEndian.Uint32(data[4:8]))
	b := int32(binary.LittleEndian.Uint32(data[8:12]))
	a := int32(binary.LittleEndian.Uint32(data[12:16]))
	return color.NRGBA{
		R: clampByte(r),
		G: clampByte(g),
		B: clampByte(b),
		A: clampByte(a),
	}
}

func clampByte(v int32) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
