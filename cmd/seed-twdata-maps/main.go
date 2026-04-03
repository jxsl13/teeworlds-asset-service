// Command seed-twdata-maps seeds a running asset-service with maps from the
// twdata.pati.ga community map database.
//
// It fetches one of the available map list JSON files (ddnet-full, kog, ddnet,
// unique, teeworlds, ddnet-testing, heinrich5991), parses the map name → URL
// mapping, downloads each .map file and uploads it to the asset-service.
//
// After downloading each map, the binary is parsed to extract embedded metadata
// (author, credits, license) from the map's info item.  When present, these
// values are used as the creator and license for the upload; otherwise the
// defaults ("Unknown" / "unknown") are used as a fallback.
//
// Usage:
//
//	go run ./cmd/seed-twdata-maps                              # default: ddnet-full
//	go run ./cmd/seed-twdata-maps -source kog                  # KoG maps
//	go run ./cmd/seed-twdata-maps -source teeworlds            # vanilla maps
//	go run ./cmd/seed-twdata-maps -addr http://localhost:9090
//	go run ./cmd/seed-twdata-maps -concurrency 4
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jxsl13/teeworlds-asset-service/internal/seedutil"
	"github.com/jxsl13/teeworlds-asset-service/internal/twmap"
)

const (
	// Base URL for the twdata.pati.ga map list API.
	twdataBaseURL = "https://twdata.pati.ga/maplists"

	// twdataLicense is the fallback license when a map's embedded info item
	// does not contain a license field.
	twdataLicense = "unknown"
)

// validSources lists all known map list sources on twdata.pati.ga.
var validSources = []string{
	"ddnet-full",
	"ddnet-testing",
	"ddnet",
	"heinrich5991",
	"kog",
	"teeworlds",
	"unique",
}

// twdataMapList is the JSON structure: {"MapName": {"urls": ["url1", ...]}}
type twdataMapList map[string]struct {
	URLs []string `json:"urls"`
}

func main() {
	addr := flag.String("addr", "http://localhost:8080", "base URL of the running asset-service")
	source := flag.String("source", "ddnet-full", "map list source: "+strings.Join(validSources, ", "))
	concurrency := flag.Int("concurrency", 8, "number of parallel download/upload workers")
	rps := flag.Int("rps", 5, "max requests per second (0 = unlimited; auto-disabled for localhost)")
	flag.Parse()

	if !isValidSource(*source) {
		log.Fatalf("invalid source %q; valid sources: %s", *source, strings.Join(validSources, ", "))
	}

	// Cancel on SIGINT / SIGTERM for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Throttle requests when targeting a non-localhost server.
	effectiveRPS := *rps
	if seedutil.IsLocalhost(*addr) {
		effectiveRPS = 0
	}
	throttle := seedutil.NewThrottle(effectiveRPS)
	defer throttle.Stop()
	if effectiveRPS > 0 {
		log.Printf("throttling to %d requests/sec (target is not localhost)", effectiveRPS)
	}

	// Fetch and parse the map list JSON.
	jsonURL := fmt.Sprintf("%s/%s.json", twdataBaseURL, *source)
	log.Printf("fetching map list from %s", jsonURL)

	mapList, err := fetchMapList(jsonURL)
	if err != nil {
		log.Fatalf("failed to fetch map list: %v", err)
	}
	log.Printf("discovered %d maps from source %q", len(mapList), *source)

	if len(mapList) == 0 {
		log.Println("no maps to process")
		return
	}

	// Flatten into a slice for ordered processing.
	type mapEntry struct {
		Name string
		URL  string
	}
	var entries []mapEntry
	for name, info := range mapList {
		if len(info.URLs) == 0 {
			continue
		}
		entries = append(entries, mapEntry{Name: name, URL: info.URLs[0]})
	}
	log.Printf("processing %d maps with download URLs", len(entries))

	// Pre-fetch existing map names to skip already-uploaded maps.
	existing, err := seedutil.FetchExistingNames(*addr, "map")
	if err != nil {
		log.Fatalf("failed to fetch existing maps: %v", err)
	}
	log.Printf("found %d existing maps on server", len(existing))

	// Create HTTP client with cookie jar for CSRF handling.
	uploadClient := seedutil.NewUploadClient()

	csrfToken, err := seedutil.FetchCSRFToken(uploadClient, *addr)
	if err != nil {
		log.Fatalf("failed to fetch CSRF token: %v", err)
	}
	log.Printf("obtained CSRF token from %s", *addr)

	var (
		okCount   atomic.Int64
		failCount atomic.Int64
		skipCount atomic.Int64
	)

	// Feed entries into a channel so workers can pull the next entry
	// immediately after a skip without waiting for the throttle.
	entryCh := make(chan mapEntry, len(entries))
	for _, e := range entries {
		entryCh <- e
	}
	close(entryCh)

	var wg sync.WaitGroup
	for range *concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			skipThrottle := false
			for e := range entryCh {
				if ctx.Err() != nil {
					return
				}

				if _, ok := existing[e.Name]; ok {
					skipCount.Add(1)
					log.Printf("SKIP  %-40s (already exists)", e.Name)
					skipThrottle = true
					continue
				}

				if !skipThrottle {
					if err := throttle.Wait(ctx); err != nil {
						return
					}
				}
				skipThrottle = false

				var mapData []byte
				if err := seedutil.Retry(ctx, func() error {
					var dlErr error
					mapData, dlErr = seedutil.FetchBytes(e.URL, 100<<20) // 100 MiB limit
					return dlErr
				}, http.StatusBadGateway, http.StatusConflict, http.StatusInternalServerError); err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("FAIL  download  %-40s %v", e.Name, err)
					failCount.Add(1)
					continue
				}

				if err := throttle.Wait(ctx); err != nil {
					return
				}

				// Extract author/license metadata from the map binary.
				creators := []string{"Unknown"}
				license := twdataLicense
				info, infoErr := twmap.ParseInfo(bytes.NewReader(mapData))
				if infoErr == nil {
					if c := extractCreators(info); len(c) > 0 {
						creators = c
					}
					if info.License != "" {
						license = seedutil.MapLicense(info.License)
					}
				}

				if err := seedutil.Retry(ctx, func() error {
					return seedutil.UploadAsset(uploadClient, csrfToken, *addr, "map", e.Name, license, creators, e.Name+".map", mapData)
				}, http.StatusBadGateway, http.StatusInternalServerError); err != nil {
					if ctx.Err() != nil {
						return
					}
					var httpErr *seedutil.HTTPStatusError
					if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
						skipCount.Add(1)
						skipThrottle = true
						log.Printf("SKIP  %-40s (already exists)", e.Name)
						continue
					}
					log.Printf("FAIL  upload    %-40s %v", e.Name, err)
					failCount.Add(1)
					continue
				}

				okCount.Add(1)
				log.Printf("OK    %-40s (by %s, license %s)", e.Name, strings.Join(creators, ", "), license)
			}
		}()
	}

	wg.Wait()

	ok := okCount.Load()
	fail := failCount.Load()
	skip := skipCount.Load()
	if ctx.Err() != nil {
		log.Printf("interrupted: %d uploaded, %d skipped, %d failed before shutdown", ok, skip, fail)
		os.Exit(1)
	}
	log.Printf("done: %d uploaded, %d skipped (already exist), %d failed", ok, skip, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// fetchMapList downloads and parses a twdata.pati.ga map list JSON file.
func fetchMapList(jsonURL string) (twdataMapList, error) {
	resp, err := seedutil.HTTPGet(jsonURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, jsonURL)
	}

	var list twdataMapList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode map list JSON: %w", err)
	}
	return list, nil
}

// isValidSource checks whether the given source name is in the known list.
func isValidSource(source string) bool {
	return slices.Contains(validSources, source)
}

// extractCreators builds a creator list from the map's embedded Info.
// It combines the Author and Credits fields, splitting on common separators
// (", ", " & ", " and ").  Returns nil if both fields are empty.
func extractCreators(info twmap.Info) []string {
	var parts []string
	if info.Author != "" {
		parts = append(parts, splitCreators(info.Author)...)
	}
	if info.Credits != "" {
		parts = append(parts, splitCreators(info.Credits)...)
	}

	// Deduplicate while preserving order.
	seen := make(map[string]struct{}, len(parts))
	creators := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lower := strings.ToLower(p)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		creators = append(creators, p)
	}
	return creators
}

// splitCreators splits a creator string on common multi-author separators.
func splitCreators(s string) []string {
	// Normalise " & ", " and " and "! " to comma first, then split.
	s = strings.ReplaceAll(s, " & ", ", ")
	s = strings.ReplaceAll(s, " and ", ", ")
	s = strings.ReplaceAll(s, "! ", ", ")
	s = strings.ReplaceAll(s, "; ", ", ")
	return strings.Split(s, ", ")
}
