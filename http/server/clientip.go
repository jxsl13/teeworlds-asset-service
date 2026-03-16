package server

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type contextKey int

const clientIPKey contextKey = iota

// ClientIPMiddleware extracts the client IP address from the request and
// stores it in the context. It respects X-Forwarded-For and X-Real-Ip
// headers (set by reverse proxies) and falls back to RemoteAddr.
func ClientIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		ctx := context.WithValue(r.Context(), clientIPKey, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func clientIPFromContext(ctx context.Context) string {
	if ip, ok := ctx.Value(clientIPKey).(string); ok {
		return ip
	}
	return ""
}

func clientIP(r *http.Request) string {
	// Check X-Real-Ip first (typically set by nginx).
	if ip := strings.TrimSpace(r.Header.Get("X-Real-Ip")); ip != "" {
		return ip
	}
	// X-Forwarded-For may contain "client, proxy1, proxy2" — use leftmost.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return ip
		}
	}
	// Fall back to RemoteAddr (host:port).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
