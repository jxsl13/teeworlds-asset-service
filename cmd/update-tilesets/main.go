// Command update-tilesets downloads all DDNet default external tilesets and the
// upstream license from the ddnet repo into internal/twmap/mapres/ for
// embedding into the map renderer.
//
// Usage:
//
//	go run ./cmd/update-tilesets
//	go run ./cmd/update-tilesets -dest internal/twmap/mapres
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	githubAPI  = "https://api.github.com/repos/ddnet/ddnet/contents/data/mapres"
	rawBase    = "https://raw.githubusercontent.com/ddnet/ddnet/master/data/mapres"
	licenseURL = "https://raw.githubusercontent.com/ddnet/ddnet/master/license.txt"
	userAgent  = "teeworlds-asset-service/1.0"
)

// githubEntry is the subset of a GitHub Contents API entry we need.
type githubEntry struct {
	Name string `json:"name"`
}

func main() {
	dest := flag.String("dest", "internal/twmap/mapres", "destination directory for tileset files")
	flag.Parse()

	if err := run(*dest); err != nil {
		log.Fatal(err)
	}
}

func run(dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	// ── Fetch upstream license ──────────────────────────────────────────
	licensePath := filepath.Join(dest, "LICENSE")
	if err := downloadFile(licensePath, licenseURL); err != nil {
		return fmt.Errorf("download license: %w", err)
	}
	fmt.Printf("Downloaded upstream license → %s\n", licensePath)

	// ── List tileset PNGs via GitHub API ────────────────────────────────
	entries, err := listContents()
	if err != nil {
		return fmt.Errorf("list github contents: %w", err)
	}

	var pngs []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name, ".png") {
			pngs = append(pngs, e.Name)
		}
	}
	sort.Strings(pngs)

	fmt.Printf("Downloading %d tilesets to %s/\n", len(pngs), dest)
	for _, name := range pngs {
		url := rawBase + "/" + name
		out := filepath.Join(dest, name)
		if err := downloadFile(out, url); err != nil {
			return fmt.Errorf("download %s: %w", name, err)
		}
		info, err := os.Stat(out)
		if err != nil {
			return err
		}
		fmt.Printf("  %s (%d bytes)\n", name, info.Size())
	}

	fmt.Printf("Done: %d files + LICENSE\n", len(pngs))
	return nil
}

func listContents() ([]githubEntry, error) {
	req, err := http.NewRequest(http.MethodGet, githubAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %s", resp.Status)
	}

	var entries []githubEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func downloadFile(dest, url string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return f.Close()
}
