// Command seed-ddnet-maps seeds a running asset-service with maps from the
// official DDNet map repository (https://github.com/ddnet/ddnet-maps).
//
// It fetches the maps.txt metadata file for each map type (novice, moderate,
// brutal, insane, etc.), parses the structured "stars|name|creator" lines to
// obtain the real map name and its creator(s), then downloads the .map file
// from the GitHub raw URL and uploads it to the asset-service.
//
// The votes.cfg file is also parsed (using the twcfg parser) as a fallback
// to discover maps that may be listed there but missing from maps.txt.
//
// Usage:
//
//	go run ./cmd/seed-ddnet-maps                              # default: http://localhost:8080
//	go run ./cmd/seed-ddnet-maps -addr http://localhost:9090
//	go run ./cmd/seed-ddnet-maps -type novice                 # only novice maps
//	go run ./cmd/seed-ddnet-maps -concurrency 4               # parallel downloads
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jxsl13/teeworlds-asset-service/internal/seedutil"
	"github.com/jxsl13/teeworlds-asset-service/internal/twcfg"
	"github.com/jxsl13/teeworlds-asset-service/internal/twmap"
)

const (
	// GitHub raw base URL for the ddnet-maps repository (master branch).
	ghRawBase = "https://raw.githubusercontent.com/ddnet/ddnet-maps/master"

	// ddnetMapsLicense is the license to attribute to maps from the ddnet-maps
	// repository.  The repository (https://github.com/ddnet/ddnet-maps) has no
	// LICENSE file and no per-map license metadata in maps.txt or votes.cfg.
	// Each map is a community creation by its respective creator(s) with no
	// declared license.  We therefore attribute "unknown".
	//
	// Note: the DDNet *client* repository (ddnet/ddnet) uses the zlib license,
	// but that covers the game engine source code — not the community-submitted
	// map files distributed via ddnet-maps.
	ddnetMapsLicense = "unknown"
)

// All DDNet map type directories under types/.
var mapTypes = []string{
	"brutal",
	"ddmax.easy",
	"ddmax.next",
	"ddmax.nut",
	"ddmax.pro",
	"dummy",
	"event",
	"fun",
	"insane",
	"moderate",
	"novice",
	"oldschool",
	"race",
	"solo",
}

// ddnetMap holds metadata for a single map.
type ddnetMap struct {
	Name     string   // display name (from maps.txt or votes.cfg)
	Creators []string // one or more creator names
	Type     string   // map type directory name (e.g. "novice")
}

func main() {
	addr := flag.String("addr", "http://localhost:8080", "base URL of the running asset-service")
	filterType := flag.String("type", "", "filter by map type (e.g. novice, brutal); empty = all")
	concurrency := flag.Int("concurrency", 8, "number of parallel download/upload workers")
	rps := flag.Int("rps", 5, "max requests per second (0 = unlimited; auto-disabled for localhost)")
	flag.Parse()

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

	types := mapTypes
	if *filterType != "" {
		types = []string{*filterType}
	}

	// Collect all maps across all types.
	var allMaps []ddnetMap
	for _, t := range types {
		maps, err := fetchMapType(t)
		if err != nil {
			log.Printf("WARN  failed to fetch type %q: %v", t, err)
			continue
		}
		allMaps = append(allMaps, maps...)
	}
	log.Printf("discovered %d maps across %d type(s)", len(allMaps), len(types))

	if len(allMaps) == 0 {
		log.Println("no maps to process")
		return
	}

	// Deduplicate maps by name (same map can appear in different types).
	allMaps = deduplicateMaps(allMaps)
	log.Printf("after deduplication: %d unique maps", len(allMaps))

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

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	for _, m := range allMaps {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(m ddnetMap) {
			defer wg.Done()
			defer func() { <-sem }()

			if _, ok := existing[m.Name]; ok {
				skipCount.Add(1)
				log.Printf("SKIP  %-40s (%s, already exists)", m.Name, m.Type)
				return
			}

			if err := throttle.Wait(ctx); err != nil {
				return
			}
			var mapData []byte
			if err := seedutil.Retry(ctx, func() error {
				var e error
				mapData, e = downloadMap(m.Type, m.Name)
				return e
			}, http.StatusBadGateway, http.StatusConflict, http.StatusInternalServerError); err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("FAIL  download  %-40s (%s) %v", m.Name, m.Type, err)
				failCount.Add(1)
				return
			}

			// Validate map structure before uploading to avoid
			// sending invalid maps to the server (reduces load).
			if err := twmap.Validate(bytes.NewReader(mapData)); err != nil {
				log.Printf("SKIP  %-40s (%s, invalid: %v)", m.Name, m.Type, err)
				skipCount.Add(1)
				return
			}

			if err := throttle.Wait(ctx); err != nil {
				return
			}
			if err := seedutil.Retry(ctx, func() error {
				return seedutil.UploadAsset(uploadClient, csrfToken, *addr, "map", m.Name, ddnetMapsLicense, m.Creators, m.Name+".map", mapData)
			}, http.StatusBadGateway, http.StatusInternalServerError); err != nil {
				if ctx.Err() != nil {
					return
				}
				var httpErr *seedutil.HTTPStatusError
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
					skipCount.Add(1)
					log.Printf("SKIP  %-40s (%s, already exists)", m.Name, m.Type)
					return
				}
				log.Printf("FAIL  upload    %-40s (%s) %v", m.Name, m.Type, err)
				failCount.Add(1)
				return
			}

			okCount.Add(1)
			log.Printf("OK    %-40s (%s, by %s)", m.Name, m.Type, strings.Join(m.Creators, ", "))
		}(m)
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

// ── map discovery ────────────────────────────────────────────────────────────

// fetchMapType discovers all maps for a given type by parsing maps.txt and
// falling back to votes.cfg for any maps not found in maps.txt.
func fetchMapType(mapType string) ([]ddnetMap, error) {
	// Primary: parse maps.txt (structured format).
	mapsTxt, err := seedutil.FetchText(fmt.Sprintf("%s/types/%s/maps.txt", ghRawBase, url.PathEscape(mapType)))
	if err != nil {
		return nil, fmt.Errorf("fetch maps.txt for %s: %w", mapType, err)
	}

	primary := parseMapsText(mapsTxt, mapType)

	// Secondary: parse votes.cfg to find additional maps.
	votesCfg, err := seedutil.FetchText(fmt.Sprintf("%s/types/%s/votes.cfg", ghRawBase, url.PathEscape(mapType)))
	if err != nil {
		// votes.cfg is optional; just use maps.txt results.
		return primary, nil
	}

	voteMaps := parseVotesCfg(votesCfg, mapType)

	// Merge: add vote maps not already in primary.
	seen := make(map[string]struct{}, len(primary))
	for _, m := range primary {
		seen[m.Name] = struct{}{}
	}
	for _, m := range voteMaps {
		if _, exists := seen[m.Name]; !exists {
			primary = append(primary, m)
			seen[m.Name] = struct{}{}
		}
	}

	return primary, nil
}

// parseMapsText parses a maps.txt file.  The format is:
//
//	─── SECTION HEADER ───
//	stars|MapName|Creator(s)
//
// Lines that don't match the pattern are silently skipped (section headers,
// blank lines, etc.).
func parseMapsText(text, mapType string) []ddnetMap {
	var result []ddnetMap
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}

		// parts[0] = stars (digit), parts[1] = name, parts[2] = creators
		name := strings.TrimSpace(parts[1])
		creatorsRaw := strings.TrimSpace(parts[2])

		if name == "" {
			continue
		}

		result = append(result, ddnetMap{
			Name:     name,
			Creators: seedutil.ParseCreators(creatorsRaw),
			Type:     mapType,
		})
	}
	return result
}

// parseVotesCfg uses the twcfg parser to extract map entries from votes.cfg.
// It looks for add_vote commands whose second argument contains change_map.
func parseVotesCfg(text, mapType string) []ddnetMap {
	cmds, err := twcfg.Parse(strings.NewReader(text), nil)
	if err != nil {
		log.Printf("WARN  failed to parse votes.cfg for %s: %v", mapType, err)
		return nil
	}

	var result []ddnetMap
	for _, cmd := range cmds {
		if !strings.EqualFold(cmd.Name, "add_vote") || len(cmd.Args) < 2 {
			continue
		}

		label := cmd.Args[0]
		action := cmd.Args[1]

		// Skip info-only votes (decorators / stats).
		if action == "info" {
			continue
		}

		// Extract map name from change_map directive.
		mapName := extractChangeMap(action)
		if mapName == "" {
			continue
		}

		// Extract creator from the vote label: "MapName by Creator | stars ★"
		creators := extractCreatorsFromVoteLabel(label)

		result = append(result, ddnetMap{
			Name:     mapName,
			Creators: creators,
			Type:     mapType,
		})
	}
	return result
}

// reChangeMap extracts the map name from a change_map directive.
// Matches: change_map "Map Name"
var reChangeMap = regexp.MustCompile(`change_map\s+"([^"]+)"`)

// extractChangeMap extracts the map name from a votes.cfg action string
// like: sv_reset_file types/novice/flexreset.cfg; change_map "Kobra 3"
func extractChangeMap(action string) string {
	m := reChangeMap.FindStringSubmatch(action)
	if len(m) >= 2 {
		return m[1]
	}
	// Fallback: unquoted change_map
	idx := strings.Index(action, "change_map ")
	if idx >= 0 {
		rest := strings.TrimSpace(action[idx+len("change_map "):])
		if semi := strings.IndexByte(rest, ';'); semi >= 0 {
			rest = rest[:semi]
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

// reByCreator matches "MapName by Creator | stars" or "MapName by Creator"
var reByCreator = regexp.MustCompile(`\bby\s+(.+?)(?:\s*\|.*)?$`)

// extractCreatorsFromVoteLabel extracts creator names from a vote label like
// "Kobra 3 by Zerodin | 4/5 ★".
func extractCreatorsFromVoteLabel(label string) []string {
	m := reByCreator.FindStringSubmatch(label)
	if len(m) >= 2 {
		return seedutil.ParseCreators(m[1])
	}
	return []string{"Unknown"}
}

// deduplicateMaps removes duplicate maps by name, keeping the first occurrence.
func deduplicateMaps(maps []ddnetMap) []ddnetMap {
	seen := make(map[string]struct{}, len(maps))
	result := make([]ddnetMap, 0, len(maps))
	for _, m := range maps {
		if _, exists := seen[m.Name]; exists {
			continue
		}
		seen[m.Name] = struct{}{}
		result = append(result, m)
	}
	return result
}

// ── download ─────────────────────────────────────────────────────────────────

// downloadMap fetches a .map file from the GitHub raw URL.
func downloadMap(mapType, mapName string) ([]byte, error) {
	rawURL := fmt.Sprintf("%s/types/%s/maps/%s.map",
		ghRawBase,
		url.PathEscape(mapType),
		url.PathEscape(mapName),
	)
	return seedutil.FetchBytes(rawURL, 100<<20) // 100 MiB limit
}
