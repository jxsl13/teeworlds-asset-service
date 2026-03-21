package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
)

// staticETagHandler serves embedded static files with content-based ETags.
// ETags are computed once at startup and cached in a path→etag map,
// so subsequent requests only perform a map lookup.
type staticETagHandler struct {
	fsys  fs.FS
	etags map[string]string // path → `"<hex>"`
}

// NewStaticHandler creates an http.Handler that serves files from the
// embedded static FS with stable, content-based ETags.  The entire FS
// is walked at construction time to warm the cache.
func NewStaticHandler() http.Handler {
	sub, _ := fs.Sub(staticFS, "static")
	h := &staticETagHandler{
		fsys:  sub,
		etags: make(map[string]string),
	}
	h.warm()
	return h
}

// warm walks the embedded FS and computes a SHA-256 ETag for every file.
func (h *staticETagHandler) warm() {
	_ = fs.WalkDir(h.fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := h.fsys.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()

		hash := sha256.New()
		if _, err := io.Copy(hash, f); err != nil {
			return err
		}
		etag := `"` + hex.EncodeToString(hash.Sum(nil)[:16]) + `"`
		h.etags[p] = etag
		slog.Debug("static etag cached", "path", p, "etag", etag)
		return nil
	})
	slog.Info("static etag cache warmed", "files", len(h.etags))
}

func (h *staticETagHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Normalize path: strip leading slash, reject directory traversal.
	p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if p == "" {
		p = "."
	}

	etag, ok := h.etags[p]
	if !ok {
		http.NotFound(w, r)
		return
	}

	// 304 Not Modified?
	if etagMatch(r.Header.Get("If-None-Match"), etag) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	f, err := h.fsys.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	// Content-Type from extension.
	if ct := mime.TypeByExtension(path.Ext(p)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
	io.Copy(w, f)
}
