package server

import (
	"context"
	"net/http"

	"github.com/donseba/go-htmx"
)

// HtmxMiddleware parses HTMX request headers and stores them in the
// request context so that strict handlers can inspect them.
func HtmxMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hxh := htmx.HxRequestHeaderFromRequest(r)
		ctx := context.WithValue(r.Context(), htmx.ContextRequestHeader, hxh)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// hxFromContext returns the parsed HTMX request headers stored by
// HtmxMiddleware, or a zero value if the middleware was not applied.
func hxFromContext(ctx context.Context) htmx.HxRequestHeader {
	if h, ok := ctx.Value(htmx.ContextRequestHeader).(htmx.HxRequestHeader); ok {
		return h
	}
	return htmx.HxRequestHeader{}
}

// isHxRequest reports whether the current request was initiated by HTMX.
func isHxRequest(ctx context.Context) bool {
	return hxFromContext(ctx).HxRequest
}
