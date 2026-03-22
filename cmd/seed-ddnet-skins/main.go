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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"

	"github.com/jxsl13/teeworlds-asset-service/internal/seedutil"
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

	// Create an HTTP client with a cookie jar for the target server.
	// This lets the CSRF cookie be stored and sent automatically.
	uploadClient := seedutil.NewUploadClient()

	// Fetch CSRF token from the target server.
	csrfToken, err := seedutil.FetchCSRFToken(uploadClient, *addr)
	if err != nil {
		log.Fatalf("failed to fetch CSRF token: %v", err)
	}
	log.Printf("obtained CSRF token from %s", *addr)

	var (
		okCount   atomic.Int64
		failCount atomic.Int64
	)

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup

	for _, skin := range skins {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(s ddnetSkin) {
			defer wg.Done()
			defer func() { <-sem }()

			license := seedutil.MapLicense(s.License)
			creators := seedutil.ParseCreators(s.Creator)

			// Download and upload the standard-resolution skin.
			if err := throttle.Wait(ctx); err != nil {
				return
			}
			imgURL := skinImageURL(s, false)
			imgData, err := seedutil.FetchBytes(imgURL, 10<<20)
			if err != nil {
				log.Printf("FAIL  download  %-40s %v", s.Name, err)
				failCount.Add(1)
				return
			}

			if err := throttle.Wait(ctx); err != nil {
				return
			}
			err = seedutil.UploadAsset(uploadClient, csrfToken, *addr, "skin", s.Name, license, creators, s.Name+".png", imgData)
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
				if err := throttle.Wait(ctx); err != nil {
					return
				}
				uhdURL := skinImageURL(s, true)
				uhdData, err := seedutil.FetchBytes(uhdURL, 10<<20)
				if err != nil {
					log.Printf("FAIL  download  %-40s (UHD) %v", s.Name, err)
					failCount.Add(1)
					return
				}

				if err := throttle.Wait(ctx); err != nil {
					return
				}
				err = seedutil.UploadAsset(uploadClient, csrfToken, *addr, "skin", s.Name, license, creators, s.Name+".png", uhdData)
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
	if ctx.Err() != nil {
		log.Printf("interrupted: %d uploaded, %d failed before shutdown", ok, fail)
		os.Exit(1)
	}
	log.Printf("done: %d uploaded, %d failed", ok, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

// fetchSkinsDB downloads and parses the DDNet skins.json database.
func fetchSkinsDB() (*skinsDB, error) {
	resp, err := seedutil.HTTPGet(skinsJSONURL)
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
