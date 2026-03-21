// Command seed populates a running asset-service instance with procedurally
// generated but structurally valid assets (skins, gameskins, maps, etc.).
//
// Usage:
//
//	go run ./cmd/seed                           # default: http://localhost:8080
//	go run ./cmd/seed -addr http://localhost:9090
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", "http://localhost:8080", "base URL of the running asset-service")
	flag.Parse()

	assets := seedAssets()

	ok, fail := 0, 0
	for _, a := range assets {
		err := upload(*addr, a)
		if err != nil {
			log.Printf("FAIL  %-10s %-30s %v", a.AssetType, a.Name, err)
			fail++
		} else {
			log.Printf("OK    %-10s %s", a.AssetType, a.Name)
			ok++
		}
	}
	log.Printf("done: %d uploaded, %d failed", ok, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// ── seed data definition ─────────────────────────────────────────────────────

type asset struct {
	AssetType string
	Name      string
	License   string
	Creators  []string
	Filename  string
	Data      []byte
}

func seedAssets() []asset {
	return []asset{
		// ── Skins (256x128) ──────────────────────────────────────────────
		skin("tee_default", "cc0", []string{"TeeWorlds"}, 0),
		skin("tee_bluekitty", "cc-by", []string{"Kintaro"}, 1),
		skin("tee_brownie", "cc-by", []string{"Kintaro"}, 2),
		skin("tee_cammo", "cc-by", []string{"Kintaro"}, 3),
		skin("tee_coala", "cc-by", []string{"Kintaro"}, 4),
		skin("tee_dino", "cc-by-sa", []string{"Ravie"}, 5),
		skin("tee_force", "cc0", []string{"Kintaro", "TeeWorlds"}, 6),
		skin("tee_greenhill", "cc-by", []string{"Kintaro"}, 7),
		skin("tee_limekitty", "cc-by", []string{"Kintaro"}, 8),
		skin("tee_pengu", "cc0", []string{"TeeWorlds"}, 9),
		skin("tee_pinky", "cc-by", []string{"Kintaro"}, 10),
		skin("tee_redbopp", "cc-by", []string{"Kintaro"}, 11),
		skin("tee_redstripe", "cc-by", []string{"Kintaro"}, 12),
		skin("tee_saddo", "cc-by", []string{"Kintaro"}, 13),
		skin("tee_twinbop", "cc-by", []string{"Kintaro"}, 14),
		skin("tee_warpaint", "cc-by-sa", []string{"Ravie", "TeeWorlds"}, 15),

		// ── HD skins (512x256) ───────────────────────────────────────────
		skinHD("tee_default_hd", "cc0", []string{"TeeWorlds"}, 16),
		skinHD("tee_xmas_hd", "cc-by", []string{"Kintaro", "jao"}, 17),
		skinHD("tee_santa_hd", "cc-by-sa", []string{"Ravie"}, 18),

		// ── Gameskins (1024x512) ─────────────────────────────────────────
		gameskin("game_default", "cc0", []string{"TeeWorlds"}, 0),
		gameskin("game_winter", "cc-by", []string{"Ravie"}, 1),
		gameskin("game_jungle", "cc-by-sa", []string{"jao", "Ravie"}, 2),

		// ── Emoticons (256x256) ──────────────────────────────────────────
		emoticon("emoticon_default", "cc0", []string{"TeeWorlds"}, 0),
		emoticon("emoticon_nature", "cc-by", []string{"Kintaro"}, 1),
		emoticon("emoticon_kawaii", "cc-by-sa", []string{"Ravie"}, 2),

		// ── HUDs (256x256) ───────────────────────────────────────────────
		hud("hud_default", "cc0", []string{"TeeWorlds"}, 0),
		hud("hud_minimal", "cc-by", []string{"jao"}, 1),
		hud("hud_retro", "cc-by-sa", []string{"Kintaro"}, 2),

		// ── Entities (1024x1024) ─────────────────────────────────────────
		entity("entities_ddnet", "cc0", []string{"DDNet"}, 0),
		entity("entities_race", "cc-by", []string{"DDNet", "jao"}, 1),
		entity("entities_vanilla", "cc0", []string{"TeeWorlds"}, 2),

		// ── Maps ─────────────────────────────────────────────────────────
		twMap("Kobra 4", "cc-by", []string{"Silex"}, 1),
		twMap("Sunny Side Up", "cc-by-sa", []string{"Ravie", "jao"}, 2),
		twMap("Multeasymap", "cc0", []string{"DDNet"}, 3),
		twMap("Stronghold", "cc-by", []string{"Silex", "Ravie"}, 4),
		twMap("Blizzard", "cc-by-sa", []string{"jao"}, 5),
		twMap("run_lake01", "cc0", []string{"TeeWorlds"}, 6),
		twMap("ctf_midnight", "cc-by", []string{"Kintaro"}, 7),
		twMap("dm_warehouse", "cc0", []string{"TeeWorlds"}, 8),
	}
}

// ── asset constructors ───────────────────────────────────────────────────────

func skin(name, license string, creators []string, seed byte) asset {
	return asset{
		AssetType: "skin",
		Name:      name,
		License:   license,
		Creators:  creators,
		Filename:  name + ".png",
		Data:      makeSkinPNG(256, 128, seed),
	}
}

func skinHD(name, license string, creators []string, seed byte) asset {
	return asset{
		AssetType: "skin",
		Name:      name,
		License:   license,
		Creators:  creators,
		Filename:  name + ".png",
		Data:      makeSkinPNG(512, 256, seed),
	}
}

func gameskin(name, license string, creators []string, seed byte) asset {
	return asset{
		AssetType: "gameskin",
		Name:      name,
		License:   license,
		Creators:  creators,
		Filename:  name + ".png",
		Data:      makePNG(1024, 512, seed),
	}
}

func emoticon(name, license string, creators []string, seed byte) asset {
	return asset{
		AssetType: "emoticon",
		Name:      name,
		License:   license,
		Creators:  creators,
		Filename:  name + ".png",
		Data:      makePNG(256, 256, seed),
	}
}

func hud(name, license string, creators []string, seed byte) asset {
	return asset{
		AssetType: "hud",
		Name:      name,
		License:   license,
		Creators:  creators,
		Filename:  name + ".png",
		Data:      makePNG(256, 256, seed),
	}
}

func entity(name, license string, creators []string, seed byte) asset {
	return asset{
		AssetType: "entity",
		Name:      name,
		License:   license,
		Creators:  creators,
		Filename:  name + ".png",
		Data:      makePNG(1024, 1024, seed),
	}
}

func twMap(name, license string, creators []string, seed byte) asset {
	return asset{
		AssetType: "map",
		Name:      name,
		License:   license,
		Creators:  creators,
		Filename:  name + ".map",
		Data:      makeMap(seed),
	}
}

// ── image generators ─────────────────────────────────────────────────────────

// makeSkinPNG generates an NRGBA PNG that represents a valid Teeworlds 0.6 skin.
// It places distinct coloured regions at the sprite-sheet positions that
// twskin.RenderIdleTee reads:
//
//	body         (0,0)   3x3 cells  = 96x96 px  (at 256x128)
//	body outline (3,0)   3x3 cells  = 96x96 px
//	foot         (6,1)   2x1 cells  = 64x32 px
//	foot outline (6,2)   2x1 cells  = 64x32 px
//	eye normal   (2,3)   1x1 cell   = 32x32 px
//
// The seed parameter varies colours so each skin produces a unique checksum.
func makeSkinPNG(width, height int, seed byte) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))

	cellW := width / 8
	cellH := height / 4

	// Fill entire image with a subtle base colour.
	base := color.NRGBA{R: seed * 7, G: seed * 13, B: seed * 29, A: 255}
	for y := range height {
		for x := range width {
			img.SetNRGBA(x, y, base)
		}
	}

	fillRect := func(gx, gy, gw, gh int, c color.NRGBA) {
		for y := gy * cellH; y < (gy+gh)*cellH; y++ {
			for x := gx * cellW; x < (gx+gw)*cellW; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}

	// Body (opaque — needed for RenderIdleTee to produce visible output).
	fillRect(0, 0, 3, 3, color.NRGBA{
		R: 100 + seed%100, G: 60 + seed%120, B: 40 + seed%80, A: 255,
	})
	// Body outline.
	fillRect(3, 0, 3, 3, color.NRGBA{
		R: 30 + seed%50, G: 20 + seed%40, B: 20 + seed%40, A: 255,
	})
	// Foot.
	fillRect(6, 1, 2, 1, color.NRGBA{
		R: 100 + seed%100, G: 60 + seed%120, B: 40 + seed%80, A: 255,
	})
	// Foot outline.
	fillRect(6, 2, 2, 1, color.NRGBA{
		R: 30 + seed%50, G: 20 + seed%40, B: 20 + seed%40, A: 255,
	})
	// Eye.
	fillRect(2, 3, 1, 1, color.NRGBA{R: 30, G: 30, B: 30, A: 255})

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// makePNG generates a colourful RGBA PNG with unique content per seed.
func makePNG(width, height int, seed byte) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{
				R: uint8((x + y + int(seed)) % 256),
				G: uint8((y + int(seed)*37) % 256),
				B: seed,
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// ── map generator ────────────────────────────────────────────────────────────

// makeMap generates a minimal valid Teeworlds v3 datafile (.map).
func makeMap(seed byte) []byte {
	type dfItem struct {
		typeID uint16
		id     uint16
		data   []int32
	}
	type dfItemType struct {
		typeID, start, num int32
	}

	var items []dfItem
	var dataBlocks [][]byte

	addData := func(d []byte) int32 {
		idx := int32(len(dataBlocks))
		dataBlocks = append(dataBlocks, d)
		return idx
	}

	emptyStr := addData([]byte{0})

	// Version (type=0): version=1
	items = append(items, dfItem{typeID: 0, id: 0, data: []int32{1}})

	// Info (type=1): [ver, author, version, credits, license]
	items = append(items, dfItem{typeID: 1, id: 0, data: []int32{1, emptyStr, emptyStr, emptyStr, emptyStr}})

	// Tile data: 50x50 game tiles (vary by seed for unique checksums).
	const mapW, mapH = 50, 50
	tileData := make([]byte, mapW*mapH*4)
	for i := range mapW * mapH {
		tileData[i*4] = seed + byte(i%251) // tile ID
	}
	tileDataIdx := addData(tileData)

	// Group (type=4): [ver, offX, offY, paraX, paraY, startLayer, numLayers]
	items = append(items, dfItem{typeID: 4, id: 0, data: []int32{1, 0, 0, 100, 100, 0, 1}})

	// Game layer (type=5): tilemap with GAME flag
	items = append(items, dfItem{typeID: 5, id: 0, data: []int32{
		0, 2, 0, // layerVer, type=tilemap, flags
		2, mapW, mapH, 1, // tilemapVer, w, h, tileFlags=GAME
		255, 255, 255, 255, // RGBA
		-1, 0, -1, // colorEnv, colorEnvOff, image
		tileDataIdx, // dataIdx
	}})

	// Build item type index.
	typeMap := make(map[uint16][]int)
	for i, it := range items {
		typeMap[it.typeID] = append(typeMap[it.typeID], i)
	}
	var iTypes []dfItemType
	idx := 0
	for _, tid := range []uint16{0, 1, 4, 5} {
		indices := typeMap[tid]
		if len(indices) == 0 {
			continue
		}
		iTypes = append(iTypes, dfItemType{int32(tid), int32(idx), int32(len(indices))})
		idx += len(indices)
	}

	// Serialize items block.
	var itemsBlock bytes.Buffer
	var itemOffsets []int32
	for _, it := range items {
		itemOffsets = append(itemOffsets, int32(itemsBlock.Len()))
		typeIDAndID := int32(uint32(it.typeID)<<16 | uint32(it.id))
		sz := int32(len(it.data) * 4)
		_ = binary.Write(&itemsBlock, binary.LittleEndian, typeIDAndID)
		_ = binary.Write(&itemsBlock, binary.LittleEndian, sz)
		for _, d := range it.data {
			_ = binary.Write(&itemsBlock, binary.LittleEndian, d)
		}
	}

	// Serialize data block (v3 = uncompressed).
	var dataBlock bytes.Buffer
	var dataOffsets []int32
	for _, d := range dataBlocks {
		dataOffsets = append(dataOffsets, int32(dataBlock.Len()))
		dataBlock.Write(d)
	}

	// Build datafile.
	var buf bytes.Buffer
	buf.Write([]byte{'D', 'A', 'T', 'A'})
	_ = binary.Write(&buf, binary.LittleEndian, int32(3)) // version

	nIT := int32(len(iTypes))
	nItems := int32(len(items))
	nData := int32(len(dataBlocks))
	sItems := int32(itemsBlock.Len())
	sData := int32(dataBlock.Len())
	rest := int32(7*4 + int(nIT)*3*4 + int(nItems)*4 + int(nData)*4 + int(sItems) + int(sData))

	_ = binary.Write(&buf, binary.LittleEndian, rest) // size
	_ = binary.Write(&buf, binary.LittleEndian, rest) // swaplen
	_ = binary.Write(&buf, binary.LittleEndian, nIT)
	_ = binary.Write(&buf, binary.LittleEndian, nItems)
	_ = binary.Write(&buf, binary.LittleEndian, nData)
	_ = binary.Write(&buf, binary.LittleEndian, sItems)
	_ = binary.Write(&buf, binary.LittleEndian, sData)

	for _, it := range iTypes {
		_ = binary.Write(&buf, binary.LittleEndian, it.typeID)
		_ = binary.Write(&buf, binary.LittleEndian, it.start)
		_ = binary.Write(&buf, binary.LittleEndian, it.num)
	}
	for _, off := range itemOffsets {
		_ = binary.Write(&buf, binary.LittleEndian, off)
	}
	for _, off := range dataOffsets {
		_ = binary.Write(&buf, binary.LittleEndian, off)
	}
	buf.Write(itemsBlock.Bytes())
	buf.Write(dataBlock.Bytes())
	return buf.Bytes()
}

// ── HTTP upload ──────────────────────────────────────────────────────────────

func upload(baseURL string, a asset) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Part 1: metadata JSON
	metaHeader := make(map[string][]string)
	metaHeader["Content-Disposition"] = []string{`form-data; name="metadata"; filename="metadata.json"`}
	metaHeader["Content-Type"] = []string{"application/json"}
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return fmt.Errorf("create metadata part: %w", err)
	}
	meta := map[string]any{
		"name":     a.Name,
		"license":  a.License,
		"creators": a.Creators,
	}
	if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	// Part 2: file
	fileHeader := make(map[string][]string)
	fileHeader["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, a.Filename)}
	fileHeader["Content-Type"] = []string{"application/octet-stream"}
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return fmt.Errorf("create file part: %w", err)
	}
	if _, err := filePart.Write(a.Data); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("%s/api/upload/%s", baseURL, a.AssetType)
	resp, err := http.Post(url, writer.FormDataContentType(), &body)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusConflict:
		// Already exists — not an error for seeding.
		return nil
	default:
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
}
