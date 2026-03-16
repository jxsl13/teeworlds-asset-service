package server

import (
	"log/slog"
	"net/http"
	"time"
)

// RequestLogger is an HTTP middleware that logs every request with structured
// fields: method, path, status, duration and response size.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(ww, r)

		slog.Info("http",
			"method", r.Method,
			"path", r.URL.RequestURI(),
			"status", ww.status,
			"bytes", ww.bytes,
			"duration", time.Since(start),
		)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController and middleware access the underlying writer.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
