// Command e2e-seed creates minimal test skin images and uploads them
// to a running asset-service instance. It is used by the Playwright
// E2E test suite to populate the database with known fixtures.
//
// Usage:
//
//	go run ./cmd/e2e-seed                           # default: http://localhost:8080
//	go run ./cmd/e2e-seed -addr http://localhost:9090
package main

import (
	"bytes"
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
	"net/url"
	"os"
)

// testSkin describes a fixture skin to upload.
type testSkin struct {
	Name     string
	License  string
	Creators []string
	Color    color.RGBA // Fill colour so thumbnails differ visually.
}

var skins = []testSkin{
	{"E2E-Alpha", "cc0", []string{"Alice"}, color.RGBA{200, 80, 80, 255}},
	{"E2E-Beta", "cc-by", []string{"Bob"}, color.RGBA{80, 200, 80, 255}},
	{"E2E-Gamma", "cc-by-sa", []string{"Charlie", "Dana"}, color.RGBA{80, 80, 200, 255}},
	{"E2E-Delta", "cc0", []string{"Eve"}, color.RGBA{200, 200, 80, 255}},
	{"E2E-Epsilon", "mit", []string{"Frank"}, color.RGBA{200, 80, 200, 255}},
}

func main() {
	addr := flag.String("addr", "http://localhost:8080", "base URL of the running asset-service")
	flag.Parse()

	for _, s := range skins {
		img := makeSkinPNG(s.Color)
		if err := upload(*addr, s, img); err != nil {
			log.Printf("WARN  %-20s %v", s.Name, err)
			continue
		}
		log.Printf("OK    %-20s (%s)", s.Name, s.License)
	}
}

// makeSkinPNG creates a 256×128 RGBA PNG filled with the given colour.
func makeSkinPNG(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 256, 128))
	for y := range 128 {
		for x := range 256 {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func upload(baseURL string, s testSkin, pngData []byte) error {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	// Part 1: metadata JSON
	metaH := make(map[string][]string)
	metaH["Content-Disposition"] = []string{`form-data; name="metadata"; filename="metadata.json"`}
	metaH["Content-Type"] = []string{"application/json"}
	metaPart, err := w.CreatePart(metaH)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(metaPart).Encode(map[string]any{
		"name":     s.Name,
		"license":  s.License,
		"creators": s.Creators,
	}); err != nil {
		return err
	}

	// Part 2: file
	fileH := make(map[string][]string)
	fileH["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s.png"`, s.Name)}
	fileH["Content-Type"] = []string{"application/octet-stream"}
	filePart, err := w.CreatePart(fileH)
	if err != nil {
		return err
	}
	if _, err := filePart.Write(pngData); err != nil {
		return err
	}
	w.Close()

	uploadURL, err := url.JoinPath(baseURL, "/api/upload/skin")
	if err != nil {
		return fmt.Errorf("build upload URL: %w", err)
	}

	resp, err := http.Post(uploadURL, w.FormDataContentType(), &body) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil
	case http.StatusConflict:
		return nil // already seeded
	default:
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
}

func init() {
	// Fail fast if called without a running server.
	if os.Getenv("E2E_SEED_SKIP") == "1" {
		os.Exit(0)
	}
}
