package server_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jxsl13/teeworlds-asset-service/config"
	"github.com/jxsl13/teeworlds-asset-service/http/api"
	httpserver "github.com/jxsl13/teeworlds-asset-service/http/server"
	"github.com/jxsl13/teeworlds-asset-service/http/server/middleware/clientip"
	sqlpkg "github.com/jxsl13/teeworlds-asset-service/sql"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// devEnv reads key=value pairs from docker/dev.env.
func devEnv(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open("../../docker/dev.env")
	if err != nil {
		t.Fatalf("open dev.env: %v", err)
	}
	defer f.Close()
	m := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, _ := strings.Cut(line, "=")
		m[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return m
}

func connectPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	env := devEnv(t)
	dsn := fmt.Sprintf("postgres://%s:%s@localhost:%s/%s?sslmode=disable",
		env["DB_USER"], env["DB_PASSWORD"], env["DB_PORT"], env["DB_NAME"])
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping: could not create pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping: DB not reachable: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// setupServer creates a fully wired httptest.Server with routes mounted.
// It truncates all asset tables before each test for isolation.
func setupServer(t *testing.T) *httptest.Server {
	return setupServerWithStorageLimit(t, 1<<30)
}

// setupServerWithRateLimit is like setupServer but enforces per-IP group creation limits.
func setupServerWithRateLimit(t *testing.T, maxGroups int, window time.Duration) *httptest.Server {
	t.Helper()
	pool := connectPool(t)
	ctx := context.Background()
	if err := sqlpkg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := pool.Exec(ctx, "TRUNCATE search_value, search_value_weight, asset_item_metadata, asset_item, asset_group CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE storage_stats SET total_size = 0"); err != nil {
		t.Fatalf("reset storage_stats: %v", err)
	}

	storagePath := t.TempDir()
	tempUploadPath := t.TempDir()
	for _, at := range []string{"skin", "gameskin", "hud", "entity", "emoticon", "theme", "template", "map"} {
		if err := os.MkdirAll(filepath.Join(storagePath, at), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", at, err)
		}
	}

	resolutions := config.DefaultResolutions
	thumbnails := config.DefaultThumbnailSizes()
	maxUploadSizes := make(map[string]int64)
	for k, v := range resolutions {
		maxUploadSizes[k] = config.DefaultMaxUploadSize(k, v)
	}
	maxUploadSizes["map"] = 64 << 20

	srv, err := httpserver.New(pool, storagePath, tempUploadPath, 1<<30, resolutions, maxUploadSizes, thumbnails, maxGroups, window, false, false, 100, config.Branding{SiteTitle: "Test"})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	r := chi.NewRouter()
	r.Use(clientip.Middleware)
	strict := api.NewStrictHandler(srv, nil)
	api.HandlerWithOptions(strict, api.ChiServerOptions{BaseRouter: r})

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts
}

// setupServerWithStorageLimit is like setupServer but allows setting a custom storage limit.
func setupServerWithStorageLimit(t *testing.T, maxStorageSize int64) *httptest.Server {
	t.Helper()
	pool := connectPool(t)
	ctx := context.Background()
	if err := sqlpkg.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Truncate all asset tables for test isolation.
	// TRUNCATE doesn't fire row-level triggers, so storage_stats must be reset manually.
	if _, err := pool.Exec(ctx, "TRUNCATE search_value, search_value_weight, asset_item_metadata, asset_item, asset_group CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE storage_stats SET total_size = 0"); err != nil {
		t.Fatalf("reset storage_stats: %v", err)
	}

	storagePath := t.TempDir()
	tempUploadPath := t.TempDir()

	// Create asset type subdirs in storage (the upload handler expects them).
	for _, at := range []string{"skin", "gameskin", "hud", "entity", "emoticon", "theme", "template", "map"} {
		if err := os.MkdirAll(filepath.Join(storagePath, at), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", at, err)
		}
	}

	resolutions := config.DefaultResolutions
	thumbnails := config.DefaultThumbnailSizes()
	maxUploadSizes := make(map[string]int64)
	for k, v := range resolutions {
		maxUploadSizes[k] = config.DefaultMaxUploadSize(k, v)
	}
	maxUploadSizes["map"] = 64 << 20

	srv, err := httpserver.New(pool, storagePath, tempUploadPath, maxStorageSize, resolutions, maxUploadSizes, thumbnails, 0, 0, false, false, 100, config.Branding{SiteTitle: "Test"})
	if err != nil {
		t.Fatalf("New server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	r := chi.NewRouter()
	r.Use(clientip.Middleware)
	strict := api.NewStrictHandler(srv, nil)
	api.HandlerWithOptions(strict, api.ChiServerOptions{BaseRouter: r})

	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts
}

// makePNG generates a valid RGBA PNG of the given dimensions.
// The seed parameter makes each generated image unique (different checksum).
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

// makeNoisyPNG generates a large, hard-to-compress PNG of the given dimensions.
// The pseudo-random pixel pattern resists PNG compression, producing files
// roughly 3-4 bytes per pixel. Used to fill storage quickly in limit tests.
func makeNoisyPNG(width, height int, seed byte) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	s := uint32(seed)*2654435761 + 1 // unique multiplier per seed
	for y := range height {
		for x := range width {
			// Simple xorshift-based hash for high entropy per pixel.
			h := s ^ uint32(x*2657+y*7919)
			h ^= h << 13
			h ^= h >> 17
			h ^= h << 5
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(h),
				G: uint8(h >> 8),
				B: uint8(h >> 16),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// makeMap generates a minimal valid Teeworlds v3 datafile (.map).
// The map contains: version item (version=1), info item (5 fields),
// one group with one game layer (10x10 tiles), and tile data.
// The seed byte varies the tile data to produce unique checksums.
func makeMap(seed byte) []byte {
	var items []testItem
	var dataBlocks [][]byte

	// Helper to add a data block and return its index.
	addData := func(d []byte) int32 {
		idx := int32(len(dataBlocks))
		dataBlocks = append(dataBlocks, d)
		return idx
	}

	// Data block 0: empty string (NUL byte) — used for info string fields.
	emptyStr := addData([]byte{0})

	// Item 0: MapVersion (type=0, id=0) — data: [1] (version=1)
	items = append(items, testItem{typeID: 0, id: 0, data: []int32{1}})

	// Item 1: MapInfo (type=1, id=0) — data: [version=1, author, mapVersion, credits, license, settings(opt)]
	// The 5 required fields: version, authorIdx, versionIdx, creditsIdx, licenseIdx
	items = append(items, testItem{typeID: 1, id: 0, data: []int32{1, emptyStr, emptyStr, emptyStr, emptyStr}})

	// Tile data: 10x10 game tiles, 4 bytes each (id, flags, skip, unused).
	const mapW, mapH = 10, 10
	tileData := make([]byte, mapW*mapH*4)
	for i := 0; i < mapW*mapH; i++ {
		tileData[i*4] = seed + byte(i%7) // tile ID
	}
	tileDataIdx := addData(tileData)

	// Item 2: MapGroup (type=4, id=0)
	// data: [version, offX, offY, paraX, paraY, startLayer, numLayers]
	items = append(items, testItem{typeID: 4, id: 0, data: []int32{1, 0, 0, 100, 100, 0, 1}})

	// Item 3: MapLayer (type=5, id=0) — tilemap game layer
	// data: [layerVer=0, type=2(tilemap), flags=0,
	//        tilemapVer=2, width, height, tileFlags=GAME(1),
	//        colorR, colorG, colorB, colorA, colorEnv, colorEnvOff, image, dataIdx]
	items = append(items, testItem{typeID: 5, id: 0, data: []int32{
		0,                  // layer version
		2,                  // type = tilemap
		0,                  // flags
		2,                  // tilemap version
		mapW,               // width
		mapH,               // height
		1,                  // tileFlags = GAME
		255, 255, 255, 255, // RGBA color
		-1,          // color env
		0,           // color env offset
		-1,          // image = none
		tileDataIdx, // data index
	}})

	// Build item types index — collect unique types.
	typeMap := make(map[uint16][]int)
	for i, it := range items {
		typeMap[it.typeID] = append(typeMap[it.typeID], i)
	}
	var itemTypes []testItemType
	itemIdx := 0
	// Sort types by typeID for determinism.
	for _, tid := range []uint16{0, 1, 2, 4, 5} {
		indices, ok := typeMap[tid]
		if !ok {
			continue
		}
		itemTypes = append(itemTypes, testItemType{typeID: int32(tid), start: int32(itemIdx), num: int32(len(indices))})
		itemIdx += len(indices)
	}

	// Serialize items block.
	var itemsBlock bytes.Buffer
	var itemOffsets []int32
	for _, it := range items {
		itemOffsets = append(itemOffsets, int32(itemsBlock.Len()))
		// item header: (typeID<<16 | id), size
		typeIDAndID := int32(uint32(it.typeID)<<16 | uint32(it.id))
		size := int32(len(it.data) * 4)
		_ = binary.Write(&itemsBlock, binary.LittleEndian, typeIDAndID)
		_ = binary.Write(&itemsBlock, binary.LittleEndian, size)
		for _, d := range it.data {
			_ = binary.Write(&itemsBlock, binary.LittleEndian, d)
		}
	}

	// Serialize data block (v3 = uncompressed, concatenated).
	var dataBlock bytes.Buffer
	var dataOffsets []int32
	for _, d := range dataBlocks {
		dataOffsets = append(dataOffsets, int32(dataBlock.Len()))
		dataBlock.Write(d)
	}

	// Build the full datafile.
	var buf bytes.Buffer

	// Version header: magic + version.
	buf.Write([]byte{'D', 'A', 'T', 'A'})
	_ = binary.Write(&buf, binary.LittleEndian, int32(3)) // datafile version 3

	// Header rest.
	numItemTypes := int32(len(itemTypes))
	numItems := int32(len(items))
	numData := int32(len(dataBlocks))
	sizeItems := int32(itemsBlock.Len())
	sizeData := int32(dataBlock.Len())
	// size = total bytes after this field until EOF
	headerRestSize := 7*4 + // remaining header fields (but 'size' field itself excluded)
		int(numItemTypes)*3*4 + // item types
		int(numItems)*4 + // item offsets
		int(numData)*4 + // data offsets (no data sizes for v3)
		int(sizeItems) +
		int(sizeData)
	swaplen := int32(headerRestSize) // All data needs byte-swapping in theory, but Go uses LE.

	_ = binary.Write(&buf, binary.LittleEndian, int32(headerRestSize)) // size
	_ = binary.Write(&buf, binary.LittleEndian, swaplen)
	_ = binary.Write(&buf, binary.LittleEndian, numItemTypes)
	_ = binary.Write(&buf, binary.LittleEndian, numItems)
	_ = binary.Write(&buf, binary.LittleEndian, numData)
	_ = binary.Write(&buf, binary.LittleEndian, sizeItems)
	_ = binary.Write(&buf, binary.LittleEndian, sizeData)

	// Item types.
	for _, it := range itemTypes {
		_ = binary.Write(&buf, binary.LittleEndian, it.typeID)
		_ = binary.Write(&buf, binary.LittleEndian, it.start)
		_ = binary.Write(&buf, binary.LittleEndian, it.num)
	}

	// Item offsets.
	for _, off := range itemOffsets {
		_ = binary.Write(&buf, binary.LittleEndian, off)
	}

	// Data offsets.
	for _, off := range dataOffsets {
		_ = binary.Write(&buf, binary.LittleEndian, off)
	}

	// Items block.
	buf.Write(itemsBlock.Bytes())

	// Data block.
	buf.Write(dataBlock.Bytes())

	return buf.Bytes()
}

type testItem struct {
	typeID uint16
	id     uint16
	data   []int32
}

type testItemType struct {
	typeID int32
	start  int32
	num    int32
}

type uploadResult struct {
	ItemID string `json:"item_id"`
}

// uploadAsset sends a multipart upload request and returns the HTTP response.
func uploadAsset(ts *httptest.Server, assetType, name, license string, creators []string, filename string, fileData []byte) (*http.Response, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Part 1: metadata (JSON)
	metaHeader := make(map[string][]string)
	metaHeader["Content-Disposition"] = []string{`form-data; name="metadata"; filename="metadata.json"`}
	metaHeader["Content-Type"] = []string{"application/json"}
	metaPart, err := writer.CreatePart(metaHeader)
	if err != nil {
		return nil, fmt.Errorf("create metadata part: %w", err)
	}
	meta := map[string]any{
		"name":     name,
		"license":  license,
		"creators": creators,
	}
	if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}

	// Part 2: file
	fileHeader := make(map[string][]string)
	fileHeader["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)}
	fileHeader["Content-Type"] = []string{"application/octet-stream"}
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return nil, fmt.Errorf("create file part: %w", err)
	}
	if _, err := filePart.Write(fileData); err != nil {
		return nil, fmt.Errorf("write file data: %w", err)
	}
	writer.Close()

	url := fmt.Sprintf("%s/api/upload/%s", ts.URL, assetType)
	return http.Post(url, writer.FormDataContentType(), &body)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestUploadSkin(t *testing.T) {
	ts := setupServer(t)

	pngData := makePNG(256, 128, 1) // smallest allowed skin resolution

	resp, err := uploadAsset(ts, "skin", "TestSkin_Upload", "cc0", []string{"tester"}, "test_skin.png", pngData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var result uploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ItemID == "" {
		t.Fatal("expected non-empty item_id")
	}

	// Uploading the same file again should return 409 (duplicate checksum).
	resp2, err := uploadAsset(ts, "skin", "TestSkin_Upload", "cc0", []string{"tester"}, "test_skin.png", pngData)
	if err != nil {
		t.Fatalf("duplicate upload: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 409 on duplicate, got %d: %s", resp2.StatusCode, body)
	}
}

func TestUploadSkinMultipleResolutions(t *testing.T) {
	ts := setupServer(t)

	resolutions := []struct{ w, h int }{
		{256, 128},
		{512, 256},
		{1024, 512},
	}

	name := "MultiResSkin"
	for i, r := range resolutions {
		t.Run(fmt.Sprintf("%dx%d", r.w, r.h), func(t *testing.T) {
			pngData := makePNG(r.w, r.h, byte(10+i))
			resp, err := uploadAsset(ts, "skin", name, "cc-by", []string{"artist1", "artist2"},
				fmt.Sprintf("skin_%dx%d.png", r.w, r.h), pngData)
			if err != nil {
				t.Fatalf("upload: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
			}
		})
	}
}

func TestUploadSkinInvalidResolution(t *testing.T) {
	ts := setupServer(t)

	// 100x100 is not a valid skin resolution
	pngData := makePNG(100, 100, 20)
	resp, err := uploadAsset(ts, "skin", "BadSkin", "cc0", []string{"tester"}, "bad.png", pngData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

func TestUploadEmoticon(t *testing.T) {
	ts := setupServer(t)

	pngData := makePNG(256, 256, 30) // smallest emoticon resolution

	resp, err := uploadAsset(ts, "emoticon", "TestEmoticon", "mit", []string{"emote_artist"}, "emote.png", pngData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
}

func TestUploadHud(t *testing.T) {
	ts := setupServer(t)

	pngData := makePNG(256, 256, 40) // smallest HUD resolution

	resp, err := uploadAsset(ts, "hud", "TestHUD", "gpl-2", []string{"hud_creator"}, "game_hud.png", pngData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
}

func TestUploadGameskin(t *testing.T) {
	ts := setupServer(t)

	pngData := makePNG(1024, 512, 50) // smallest gameskin resolution

	resp, err := uploadAsset(ts, "gameskin", "TestGameskin", "cc-by-sa", []string{"gs_artist"}, "gameskin.png", pngData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
}

func TestUploadEntity(t *testing.T) {
	ts := setupServer(t)

	pngData := makePNG(256, 256, 60) // smallest entity resolution

	resp, err := uploadAsset(ts, "entity", "TestEntity", "cc0", []string{"entity_maker"}, "entities.png", pngData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}
}

func TestUploadMissingMetadata(t *testing.T) {
	ts := setupServer(t)

	// Send only a file part without metadata
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileHeader := make(map[string][]string)
	fileHeader["Content-Disposition"] = []string{`form-data; name="file"; filename="test.png"`}
	fileHeader["Content-Type"] = []string{"application/octet-stream"}
	filePart, _ := writer.CreatePart(fileHeader)
	_, _ = filePart.Write(makePNG(256, 128, 70))
	writer.Close()

	url := fmt.Sprintf("%s/api/upload/skin", ts.URL)
	resp, err := http.Post(url, writer.FormDataContentType(), &body)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, respBody)
	}
}

func TestUploadInvalidAssetType(t *testing.T) {
	ts := setupServer(t)

	pngData := makePNG(256, 128, 80)
	resp, err := uploadAsset(ts, "invalid_type", "Test", "cc0", []string{"tester"}, "test.png", pngData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	// oapi-codegen should reject unknown asset types before reaching the handler
	if resp.StatusCode == http.StatusCreated {
		t.Fatal("expected error for invalid asset type, got 201")
	}
}

func TestUploadMap(t *testing.T) {
	ts := setupServer(t)

	mapData := makeMap(1)
	resp, err := uploadAsset(ts, "map", "TestMap_Upload", "cc0", []string{"mapper"}, "test.map", mapData)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, body)
	}

	var result uploadResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ItemID == "" {
		t.Fatal("expected non-empty item_id")
	}

	// Uploading the same map again should return 409 (duplicate checksum).
	resp2, err := uploadAsset(ts, "map", "TestMap_Upload", "cc0", []string{"mapper"}, "test.map", mapData)
	if err != nil {
		t.Fatalf("duplicate upload: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 409 on duplicate, got %d: %s", resp2.StatusCode, body)
	}
}

func TestUploadMapInvalidFile(t *testing.T) {
	ts := setupServer(t)

	// Random bytes are not a valid Teeworlds map.
	resp, err := uploadAsset(ts, "map", "BadMap", "cc0", []string{"tester"}, "bad.map", []byte("not a map"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, body)
	}
}

// TestUploadAndList uploads several assets via HTTP and then verifies
// they appear in the list endpoint for their respective types.
func TestUploadAndList(t *testing.T) {
	ts := setupServer(t)

	// Upload one of each image type.
	uploads := []struct {
		assetType string
		name      string
		filename  string
		data      []byte
	}{
		{"skin", "ListTestSkin", "skin.png", makePNG(256, 128, 90)},
		{"gameskin", "ListTestGameskin", "gameskin.png", makePNG(1024, 512, 91)},
		{"emoticon", "ListTestEmoticon", "emote.png", makePNG(256, 256, 92)},
		{"hud", "ListTestHud", "hud.png", makePNG(256, 256, 93)},
		{"entity", "ListTestEntity", "entity.png", makePNG(256, 256, 94)},
		{"map", "ListTestMap", "map.map", makeMap(95)},
	}

	itemIDs := make(map[string]string) // assetType -> item_id
	for _, u := range uploads {
		resp, err := uploadAsset(ts, u.assetType, u.name, "cc0", []string{"lister"}, u.filename, u.data)
		if err != nil {
			t.Fatalf("upload %s: %v", u.assetType, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("upload %s: expected 201, got %d: %s", u.assetType, resp.StatusCode, body)
		}
		var result uploadResult
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("upload %s: decode: %v", u.assetType, err)
		}
		itemIDs[u.assetType] = result.ItemID
	}

	// Verify each type appears in the list endpoint.
	for _, u := range uploads {
		t.Run("list_"+u.assetType, func(t *testing.T) {
			url := fmt.Sprintf("%s/api/%s", ts.URL, u.assetType)
			resp, err := http.Get(url)
			if err != nil {
				t.Fatalf("list %s: %v", u.assetType, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("list %s: expected 200, got %d: %s", u.assetType, resp.StatusCode, body)
			}

			var listResp struct {
				Results []struct {
					ItemValue struct {
						Name string `json:"name"`
					} `json:"item_value"`
				} `json:"results"`
				Total int `json:"total"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
				t.Fatalf("list %s: decode: %v", u.assetType, err)
			}
			if listResp.Total == 0 {
				t.Fatalf("list %s: expected at least 1 item, got total=0", u.assetType)
			}

			found := false
			for _, item := range listResp.Results {
				if item.ItemValue.Name == u.name {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("list %s: item %q not found in results", u.assetType, u.name)
			}
		})
	}

	// Verify each item can be downloaded.
	for _, u := range uploads {
		t.Run("download_"+u.assetType, func(t *testing.T) {
			url := fmt.Sprintf("%s/api/%s/%s/download", ts.URL, u.assetType, itemIDs[u.assetType])
			resp, err := http.Get(url)
			if err != nil {
				t.Fatalf("download %s: %v", u.assetType, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("download %s: expected 200, got %d: %s", u.assetType, resp.StatusCode, body)
			}
			downloaded, _ := io.ReadAll(resp.Body)
			if len(downloaded) == 0 {
				t.Fatalf("download %s: empty response body", u.assetType)
			}
			if len(downloaded) != len(u.data) {
				t.Fatalf("download %s: size mismatch: uploaded %d, downloaded %d", u.assetType, len(u.data), len(downloaded))
			}
		})
	}
}

// TestUploadStorageLimitExceeded configures a 10 MB storage limit, fills the
// storage with valid uploads, then verifies that every asset type returns
// HTTP 507 (Insufficient Storage) once the limit is reached.
func TestUploadStorageLimitExceeded(t *testing.T) {
	const storageLimit = 10 * 1024 * 1024 // 10 MB
	ts := setupServerWithStorageLimit(t, storageLimit)

	// Phase 1: Bulk fill with large entities (~2.8 MB each), then plug the
	// remaining gap with smaller skins (~98 KB each). Each step uploads until
	// the server returns 507 so the next step fills any remaining space.
	fillSteps := []struct {
		assetType string
		width     int
		height    int
		prefix    string
	}{
		{"entity", 1024, 1024, "fill_entity"}, // ~2.8 MB → 3 fit
		{"skin", 256, 128, "fill_skin"},       // ~98 KB → fills remaining ~1.6 MB
	}

	for _, step := range fillSteps {
		for i := 0; ; i++ {
			data := makeNoisyPNG(step.width, step.height, byte(i%256))
			name := fmt.Sprintf("%s_%d", step.prefix, i)
			resp, err := uploadAsset(ts, step.assetType, name, "cc0", []string{"filler"}, name+".png", data)
			if err != nil {
				t.Fatalf("fill %s %d: %v", step.assetType, i, err)
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == http.StatusInsufficientStorage {
				t.Logf("storage full after %s_%d", step.prefix, i)
				break
			}
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
				t.Fatalf("fill %s %d: want 201/409/507, got %d: %s", step.assetType, i, resp.StatusCode, body)
			}
		}
	}

	// Phase 2: Attempt one upload of each asset type — all must return 507.
	// After filling, remaining capacity is <98 KB, so all noisy images exceed it.
	// For maps: pad with trailing data so the file exceeds the remaining capacity.
	paddedMap := makeMap(215)
	paddedMap = append(paddedMap, make([]byte, 100*1024)...) // +100 KB padding

	overflowUploads := []struct {
		assetType string
		filename  string
		data      []byte
	}{
		{"skin", "overflow_skin.png", makeNoisyPNG(256, 128, 210)},
		{"gameskin", "overflow_gameskin.png", makeNoisyPNG(1024, 512, 211)},
		{"emoticon", "overflow_emoticon.png", makeNoisyPNG(256, 256, 212)},
		{"hud", "overflow_hud.png", makeNoisyPNG(256, 256, 213)},
		{"entity", "overflow_entity.png", makeNoisyPNG(1024, 1024, 214)},
		{"map", "overflow.map", paddedMap},
	}

	for _, u := range overflowUploads {
		t.Run("507_"+u.assetType, func(t *testing.T) {
			resp, err := uploadAsset(ts, u.assetType, "overflow_"+u.assetType, "cc0", []string{"tester"}, u.filename, u.data)
			if err != nil {
				t.Fatalf("overflow upload %s: %v", u.assetType, err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusInsufficientStorage {
				t.Fatalf("expected 507 for %s, got %d: %s", u.assetType, resp.StatusCode, body)
			}

			var errResp struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(body, &errResp); err != nil {
				t.Fatalf("decode error response for %s: %v (body: %s)", u.assetType, err, body)
			}
			if errResp.Error == "" {
				t.Fatalf("expected non-empty error message for %s", u.assetType)
			}
		})
	}
}

// TestSearchSingleChar verifies that searching for a single character finds
// items whose names contain that character (substring match for short queries).
func TestSearchSingleChar(t *testing.T) {
	ts := setupServer(t)

	// Upload skins with names that contain "0".
	names := []string{"fill_skin_0", "fill_skin_10", "abc"}
	for i, name := range names {
		pngData := makePNG(256, 128, byte(50+i))
		resp, err := uploadAsset(ts, "skin", name, "cc0", []string{"tester"}, name+".png", pngData)
		if err != nil {
			t.Fatalf("upload %s: %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("upload %s: expected 201, got %d", name, resp.StatusCode)
		}
	}

	// Search for "0" — should find "fill_skin_0" and "fill_skin_10" but not "abc".
	searchURL := fmt.Sprintf("%s/api/search/skin?q=0&limit=20", ts.URL)
	resp, err := http.Get(searchURL)
	if err != nil {
		t.Fatalf("search request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: expected 200, got %d: %s", resp.StatusCode, body)
	}

	var searchResp struct {
		Results []struct {
			ItemValue map[string]interface{} `json:"item_value"`
		} `json:"results"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		t.Fatalf("decode search response: %v (body: %s)", err, body)
	}

	t.Logf("search for '0': total=%d, body=%s", searchResp.Total, body)

	if searchResp.Total < 2 {
		t.Fatalf("expected at least 2 results for query '0', got %d: %s", searchResp.Total, body)
	}

	// Verify "abc" is NOT in results.
	for _, r := range searchResp.Results {
		name, _ := r.ItemValue["name"].(string)
		if name == "abc" {
			t.Fatalf("unexpected result 'abc' for query '0'")
		}
	}
}

// TestUploadRateLimit verifies that an IP cannot create more than the configured
// number of new asset groups within the rate-limit window.
func TestUploadRateLimit(t *testing.T) {
	const maxGroups = 3
	ts := setupServerWithRateLimit(t, maxGroups, 24*time.Hour)

	// Upload maxGroups unique assets from the same IP — all should succeed.
	for i := range maxGroups {
		pngData := makePNG(256, 128, byte(100+i))
		name := fmt.Sprintf("rate_skin_%d", i)
		resp, err := uploadAsset(ts, "skin", name, "cc0", []string{"tester"}, name+".png", pngData)
		if err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("upload %d: expected 201, got %d: %s", i, resp.StatusCode, body)
		}
	}

	// The next upload (new group name) must be rejected with 429.
	pngData := makePNG(256, 128, byte(200))
	resp, err := uploadAsset(ts, "skin", "rate_skin_over_limit", "cc0", []string{"tester"}, "over.png", pngData)
	if err != nil {
		t.Fatalf("over-limit upload: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 429 on over-limit upload, got %d: %s", resp.StatusCode, body)
	}

	// Adding a new VARIANT to an existing group must still succeed (not a new group).
	existingVariantData := makePNG(512, 256, byte(201))
	resp2, err := uploadAsset(ts, "skin", "rate_skin_0", "cc0", []string{"tester"}, "variant.png", existingVariantData)
	if err != nil {
		t.Fatalf("variant upload: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("variant upload: expected 201 (existing group), got %d: %s", resp2.StatusCode, body)
	}
}
