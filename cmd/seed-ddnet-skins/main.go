// Command seed-ddnet-skins seeds a running asset-service with skins from
// the DDNet skin database (https://ddnet.org/skins/).
//
// It fetches metadata from skins.json, then downloads each skin image
// directly from the DDNet web server and uploads it.  Skins that have
// a UHD variant are uploaded twice (standard + UHD).
//
// Usage:
//
//	go run ./cmd/seed-ddnet-skins                           # default: http://localhost:8080
//	go run ./cmd/seed-ddnet-skins -addr http://localhost:9090
//	go run ./cmd/seed-ddnet-skins -type community           # only community skins
//	go run ./cmd/seed-ddnet-skins -type normal              # only normal skins
//	go run ./cmd/seed-ddnet-skins -concurrency 4            # parallel downloads
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	skinsJSONURL     = "https://ddnet.org/skins/skin/skins.json"
	normalBaseURL    = "https://ddnet.org/skins/skin/"
	normalUHDBaseURL = "https://ddnet.org/skins/skin/uhd/"
	communityBaseURL = "https://ddnet.org/skins/skin/community/"
	communityUHDBase = "https://ddnet.org/skins/skin/community/uhd/"
)

// ddnetSkin mirrors the structure of an entry in the DDNet skins.json file.
type ddnetSkin struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // "normal", "community", or "template"
	HD          ddnetSkinHD `json:"hd"`
	Creator     string      `json:"creator"`
	License     string      `json:"license"`
	Bodypart    string      `json:"bodypart"`
	Gameversion string      `json:"gameversion"`
	Date        string      `json:"date"`
	Skinpack    string      `json:"skinpack"`
	Imgtype     string      `json:"imgtype"`
}

type ddnetSkinHD struct {
	UHD bool `json:"uhd"`
}

type skinsDB struct {
	Skins []ddnetSkin `json:"skins"`
}

func main() {
	addr := flag.String("addr", "http://localhost:8080", "base URL of the running asset-service")
	skinType := flag.String("type", "", "filter by skin type: normal, community, or empty for all")
	concurrency := flag.Int("concurrency", 8, "number of parallel download/upload workers")
	flag.Parse()

	log.Printf("fetching skin database from %s", skinsJSONURL)
	db, err := fetchSkinsDB()
	if err != nil {
		log.Fatalf("failed to fetch skins database: %v", err)
	}
	log.Printf("found %d skins in database", len(db.Skins))

	// Filter by type if requested.
	var skins []ddnetSkin
	for _, s := range db.Skins {
		if *skinType != "" && s.Type != *skinType {
			continue
		}
		skins = append(skins, s)
	}
	log.Printf("processing %d skins (filter: %q)", len(skins), *skinType)

	var (
		okCount   atomic.Int64
		failCount atomic.Int64
	)

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	for _, skin := range skins {
		wg.Add(1)
		sem <- struct{}{}
		go func(s ddnetSkin) {
			defer wg.Done()
			defer func() { <-sem }()

			license := mapLicense(s.License)
			creators := parseCreators(s.Creator)

			// Download and upload the standard-resolution skin.
			imgURL := skinImageURL(s, false)
			imgData, err := downloadImage(imgURL)
			if err != nil {
				log.Printf("FAIL  download  %-40s %v", s.Name, err)
				failCount.Add(1)
				return
			}

			err = uploadSkin(*addr, s.Name, license, creators, s.Name+".png", imgData)
			if err != nil {
				log.Printf("FAIL  upload    %-40s %v", s.Name, err)
				failCount.Add(1)
				return
			}

			okCount.Add(1)
			log.Printf("OK    %-40s (%s)", s.Name, license)

			// If the skin has a UHD version, download and upload it too.
			// The server groups uploads by name; the higher resolution
			// becomes an additional variant of the same item.
			if s.HD.UHD {
				uhdURL := skinImageURL(s, true)
				uhdData, err := downloadImage(uhdURL)
				if err != nil {
					log.Printf("FAIL  download  %-40s (UHD) %v", s.Name, err)
					failCount.Add(1)
					return
				}

				err = uploadSkin(*addr, s.Name, license, creators, s.Name+".png", uhdData)
				if err != nil {
					log.Printf("FAIL  upload    %-40s (UHD) %v", s.Name, err)
					failCount.Add(1)
					return
				}

				okCount.Add(1)
				log.Printf("OK    %-40s (UHD, %s)", s.Name, license)
			}
		}(skin)
	}

	wg.Wait()

	ok := okCount.Load()
	fail := failCount.Load()
	log.Printf("done: %d uploaded, %d failed", ok, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// fetchSkinsDB downloads and parses the DDNet skins.json database.
func fetchSkinsDB() (*skinsDB, error) {
	resp, err := httpGet(skinsJSONURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching skins.json", resp.StatusCode)
	}

	var db skinsDB
	if err := json.NewDecoder(resp.Body).Decode(&db); err != nil {
		return nil, fmt.Errorf("decode skins.json: %w", err)
	}
	return &db, nil
}

// skinImageURL constructs the download URL for a skin PNG.
// When uhd is true it returns the UHD variant URL.
func skinImageURL(s ddnetSkin, uhd bool) string {
	encoded := url.PathEscape(s.Name) + ".png"
	if s.Type == "community" {
		if uhd {
			return communityUHDBase + encoded
		}
		return communityBaseURL + encoded
	}
	if uhd {
		return normalUHDBaseURL + encoded
	}
	return normalBaseURL + encoded
}

// downloadImage fetches an image from the given URL.
func downloadImage(imageURL string) ([]byte, error) {
	resp, err := httpGet(imageURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, imageURL)
	}

	// Limit to 10 MiB to avoid unbounded reads.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}
	return data, nil
}

// uploadSkin uploads a single skin to the asset-service.
func uploadSkin(baseURL, name, license string, creators []string, filename string, data []byte) error {
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
		"name":     name,
		"license":  license,
		"creators": creators,
	}
	if err := json.NewEncoder(metaPart).Encode(meta); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}

	// Part 2: file
	fileHeader := make(map[string][]string)
	fileHeader["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename)}
	fileHeader["Content-Type"] = []string{"application/octet-stream"}
	filePart, err := writer.CreatePart(fileHeader)
	if err != nil {
		return fmt.Errorf("create file part: %w", err)
	}
	if _, err := filePart.Write(data); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	writer.Close()

	uploadURL := fmt.Sprintf("%s/api/upload/skin", baseURL)
	resp, err := http.Post(uploadURL, writer.FormDataContentType(), &body) //nolint:gosec // URL is from CLI flag
	if err != nil {
		return fmt.Errorf("POST %s: %w", uploadURL, err)
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

// mapLicense converts a DDNet license string to an asset-service license enum value.
func mapLicense(ddnetLicense string) string {
	normalized := strings.TrimSpace(strings.ToLower(ddnetLicense))
	switch {
	case normalized == "cc0":
		return "cc0"
	case normalized == "cc by" || normalized == "cc-by":
		return "cc-by"
	case normalized == "cc by-sa" || normalized == "cc-by-sa":
		return "cc-by-sa"
	case normalized == "cc by-nd" || normalized == "cc-by-nd":
		return "cc-by-nd"
	case normalized == "cc by-nc" || normalized == "cc-by-nc":
		return "cc-by-nc"
	case normalized == "cc by-nc-sa" || normalized == "cc-by-nc-sa":
		return "cc-by-nc-sa"
	case normalized == "cc by-nc-nd" || normalized == "cc-by-nc-nd":
		return "cc-by-nc-nd"
	case normalized == "gpl-2" || normalized == "gpl2":
		return "gpl-2"
	case normalized == "gpl-3" || normalized == "gpl3":
		return "gpl-3"
	case normalized == "mit":
		return "mit"
	case normalized == "apache-2" || normalized == "apache 2.0" || normalized == "apache-2.0":
		return "apache-2"
	case normalized == "zlib":
		return "zlib"
	case normalized == "unknown" || normalized == "":
		return "unknown"
	default:
		return "custom"
	}
}

// reParenAttrib matches parenthetical attributions like
// "(toast skin by DianChi)" or "(source from Tater)".
var reParenAttrib = regexp.MustCompile(`\((?:[^)]*?\b(?:by|from)\s+)([^)]+)\)`)

// parseCreators splits a creator string into a list.
// DDNet uses various separators and attribution patterns:
//   - comma: "A, B"
//   - ampersand: "A & B", "A&B"
//   - plus: "A + B"
//   - and: "A and B"
//   - feat: "A .feat B"
//   - "Hat by" / "Skin by" attribution: "A Hat by B"
//   - parenthetical: "A (skin by B)", "A (source from B)"
func parseCreators(creator string) []string {
	if strings.TrimSpace(creator) == "" {
		return []string{"Unknown"}
	}

	s := creator

	// Extract names from parenthetical attributions and flatten.
	s = reParenAttrib.ReplaceAllString(s, ", $1")

	// Handle "Hat by" / "Skin by" mid-string attributions.
	for _, sep := range []string{" Hat by ", " hat by ", " Skin by ", " skin by "} {
		s = strings.ReplaceAll(s, sep, ",")
	}

	// Handle ".feat" separator.
	s = strings.ReplaceAll(s, ".feat ", ",")

	// Normalize separators.
	s = strings.ReplaceAll(s, " & ", ",")
	s = strings.ReplaceAll(s, "&", ",")
	s = strings.ReplaceAll(s, " + ", ",")
	s = strings.ReplaceAll(s, " and ", ",")

	var result []string
	for _, part := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(part)
		trimmed = strings.TrimRight(trimmed, ".,;:")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{"Unknown"}
	}
	return result
}

// httpGet performs an HTTP GET with a reasonable timeout and user-agent.
func httpGet(rawURL string) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "asset-service-ddnet-seeder/1.0")
	return client.Do(req)
}
