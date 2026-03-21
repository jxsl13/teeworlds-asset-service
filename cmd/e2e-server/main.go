// Command e2e-server starts a lightweight HTTP server that renders the
// application templates with mock data.  It is used exclusively by
// Playwright E2E tests to verify responsive layout and UI interactions
// without requiring a real database or OIDC provider.
package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ── Mock data types (mirror the template view models) ─────────────────────────

type variantView struct {
	ItemID string
	Label  string
}

type itemView struct {
	GroupID      string
	ItemType     string
	Name         string
	Creators     string
	CreatorList  []string
	License      string
	Size         string
	Date         string
	GroupKey     string
	Variants     []variantView
	HasThumbnail bool
}

type sortColumn struct {
	Field string
	Label string
	Dir   string
	Rank  int
}

type activeFilterView struct {
	Field string
	Label string
	Value string
}

type itemsPageData struct {
	Items         []itemView
	Total         int
	Limit         int
	Offset        int
	PageStart     int
	PageEnd       int
	PrevURL       string
	NextURL       string
	SortParam     string
	Columns       []sortColumn
	ActiveFilters []activeFilterView
	IsAdmin       bool
}

// ── Mock items ────────────────────────────────────────────────────────────────

func mockItems(assetType string) []itemView {
	items := []itemView{
		{GroupID: "aaa-111", ItemType: assetType, Name: "Greyfox", Creators: "nameless tee", CreatorList: []string{"nameless tee"}, License: "cc-by", Size: "32 KB", Date: "2025-01-15", GroupKey: "resolution", Variants: []variantView{{ItemID: "v1", Label: "256x128"}}, HasThumbnail: true},
		{GroupID: "bbb-222", ItemType: assetType, Name: "Twinbop", Creators: "Conan, Warpaint", CreatorList: []string{"Conan", "Warpaint"}, License: "cc0", Size: "48 KB", Date: "2025-02-20", GroupKey: "resolution", Variants: []variantView{{ItemID: "v2", Label: "256x128"}, {ItemID: "v3", Label: "512x256"}}, HasThumbnail: true},
		{GroupID: "ccc-333", ItemType: assetType, Name: "Brownbear", Creators: "ElkTee", CreatorList: []string{"ElkTee"}, License: "cc-by-sa", Size: "64 KB", Date: "2025-03-01", GroupKey: "resolution", Variants: []variantView{{ItemID: "v4", Label: "256x128"}}, HasThumbnail: false},
		{GroupID: "ddd-444", ItemType: assetType, Name: "Pinky", Creators: "Zilly", CreatorList: []string{"Zilly"}, License: "cc-by", Size: "28 KB", Date: "2025-03-10", GroupKey: "resolution", Variants: []variantView{{ItemID: "v5", Label: "256x128"}}, HasThumbnail: true},
		{GroupID: "eee-555", ItemType: assetType, Name: "Cammostripes", Creators: "Styx, Dune", CreatorList: []string{"Styx", "Dune"}, License: "cc0", Size: "56 KB", Date: "2025-03-15", GroupKey: "resolution", Variants: []variantView{{ItemID: "v6", Label: "256x128"}}, HasThumbnail: true},
	}
	if assetType == "map" {
		for i := range items {
			items[i].GroupKey = ""
			items[i].Variants = []variantView{{ItemID: fmt.Sprintf("m%d", i), Label: ""}}
		}
	}
	return items
}

func defaultColumns() []sortColumn {
	return []sortColumn{
		{Field: "name", Label: "Name", Dir: "asc", Rank: 0},
		{Field: "creators", Label: "Creators", Dir: "", Rank: 0},
		{Field: "license", Label: "License", Dir: "", Rank: 0},
		{Field: "size", Label: "Size", Dir: "", Rank: 0},
		{Field: "created_at", Label: "Date", Dir: "", Rank: 0},
	}
}

// ── Template rendering ────────────────────────────────────────────────────────

func main() {
	repoRoot := os.Getenv("REPO_ROOT")
	if repoRoot == "" {
		repoRoot = "."
	}

	addr := os.Getenv("E2E_ADDR")
	if addr == "" {
		addr = ":3333"
	}

	tplDir := filepath.Join(repoRoot, "http", "server", "templates")
	staticDir := filepath.Join(repoRoot, "http", "server", "static")

	layoutTpl := template.Must(template.ParseFiles(filepath.Join(tplDir, "layout.html")))
	itemsTpl := template.Must(template.ParseFiles(filepath.Join(tplDir, "items.html")))

	allTypes := []string{"emoticon", "entity", "gameskin", "hud", "map", "skin", "template", "theme"}

	// Render the items fragment for a given type and admin state.
	renderItems := func(assetType string, admin bool) (string, error) {
		items := mockItems(assetType)
		data := itemsPageData{
			Items:     items,
			Total:     len(items),
			Limit:     100,
			Offset:    0,
			PageStart: 1,
			PageEnd:   len(items),
			PrevURL:   "/" + assetType + "?limit=100&offset=0",
			NextURL:   "/" + assetType + "?limit=100&offset=100",
			SortParam: "name:asc",
			Columns:   defaultColumns(),
			IsAdmin:   admin,
		}
		var buf bytes.Buffer
		if err := itemsTpl.Execute(&buf, data); err != nil {
			return "", err
		}
		return buf.String(), nil
	}

	// Render the full layout page.
	renderLayout := func(activeType string, admin bool, userName string) (string, error) {
		var buf bytes.Buffer
		err := layoutTpl.Execute(&buf, map[string]any{
			"ItemTypes":      allTypes,
			"ActiveType":     activeType,
			"Query":          "",
			"ContentURL":     "/" + activeType,
			"SortParam":      "name:asc",
			"IsAdmin":        admin,
			"UserName":       userName,
			"CanUpload":      admin,
			"SiteTitle":      "Teeworlds Asset Database",
			"SiteSubtitle":   "Community database for skins, maps, gameskins & more",
			"HasHeaderImage": false,
			"HasFavicon":     false,
		})
		return buf.String(), err
	}

	mux := http.NewServeMux()

	// Static assets.
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))))

	// Placeholder thumbnail handler (returns a 1x1 transparent PNG).
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/thumbnail") {
			w.Header().Set("Content-Type", "image/png")
			// 1x1 transparent PNG
			w.Write([]byte{
				0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
				0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
				0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
				0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
				0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
				0x54, 0x78, 0x9C, 0x62, 0x00, 0x00, 0x00, 0x02,
				0x00, 0x01, 0xE5, 0x27, 0xDE, 0xFC, 0x00, 0x00,
				0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42,
				0x60, 0x82,
			})
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=mock.bin")
		w.Write([]byte("mock-download"))
	})

	// Admin metadata / edit endpoints (return JSON mocks).
	mux.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/metadata") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"item_id":"v1","group_value":"256x128","size":32768,"checksum":"abc123","created_at":"2025-01-15T00:00:00Z","updated_at":"2025-01-15T00:00:00Z"}]`)
			return
		}
		if strings.Contains(r.URL.Path, "/items") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"item_id":"v1","group_value":"256x128","size":32768}]`)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	// Asset type pages – render items fragment for HTMX or full page.
	for _, t := range allTypes {
		assetType := t
		mux.HandleFunc("/"+assetType, func(w http.ResponseWriter, r *http.Request) {
			isHx := r.Header.Get("HX-Request") == "true"
			admin := r.URL.Query().Get("admin") == "1"

			// Also check cookie for admin state.
			if c, err := r.Cookie("e2e_admin"); err == nil && c.Value == "1" {
				admin = true
			}

			if isHx {
				html, err := renderItems(assetType, admin)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				fmt.Fprint(w, html)
				return
			}

			// Full page.
			userName := ""
			if admin {
				userName = "TestAdmin"
			}
			html, err := renderLayout(assetType, admin, userName)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, html)
		})
	}

	// Root redirect.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		admin := r.URL.Query().Get("admin") == "1"
		if c, err := r.Cookie("e2e_admin"); err == nil && c.Value == "1" {
			admin = true
		}
		userName := ""
		if admin {
			userName = "TestAdmin"
		}
		html, err := renderLayout("skin", admin, userName)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	})

	// Items count endpoint for the "search" simulation.
	mux.HandleFunc("/e2e/items-count", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, strconv.Itoa(len(mockItems("skin"))))
	})

	log.Printf("E2E test server listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
