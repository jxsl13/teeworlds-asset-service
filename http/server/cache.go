package server

import (
	"net/http"
	"strings"
)

// etagMatch checks if the If-None-Match header value contains the given ETag.
func etagMatch(header, etag string) bool {
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == etag {
			return true
		}
	}
	return false
}

// notModifiedResponse implements DownloadThumbnailResponseObject and
// HeadThumbnailResponseObject, writing a 304 Not Modified with the matching ETag.
type notModifiedResponse struct {
	etag string
}

func (r notModifiedResponse) VisitDownloadThumbnailResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", r.etag)
	w.WriteHeader(http.StatusNotModified)
	return nil
}

func (r notModifiedResponse) VisitHeadThumbnailResponse(w http.ResponseWriter) error {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", r.etag)
	w.WriteHeader(http.StatusNotModified)
	return nil
}
