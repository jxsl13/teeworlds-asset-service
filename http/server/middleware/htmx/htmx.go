package htmx

import (
	"context"
	"net/http"

	gohtmx "github.com/donseba/go-htmx"
)

// Middleware parses HTMX request headers and stores them in the
// request context so that handlers can inspect them.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hxh := gohtmx.HxRequestHeaderFromRequest(r)
		ctx := context.WithValue(r.Context(), gohtmx.ContextRequestHeader, hxh)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext returns the parsed HTMX request headers, or a zero value if
// the middleware was not applied.
func FromContext(ctx context.Context) gohtmx.HxRequestHeader {
	if h, ok := ctx.Value(gohtmx.ContextRequestHeader).(gohtmx.HxRequestHeader); ok {
		return h
	}
	return gohtmx.HxRequestHeader{}
}

// IsHTMXRequest reports whether the current request was initiated by HTMX.
func IsHTMXRequest(ctx context.Context) bool {
	return FromContext(ctx).HxRequest
}
