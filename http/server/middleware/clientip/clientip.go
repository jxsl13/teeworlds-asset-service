package clientip

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type contextKey int

const clientIPKey contextKey = iota

// Middleware extracts the client IP address from the request and stores it
// in the context as a netip.Addr. It respects X-Forwarded-For and X-Real-Ip
// headers (set by reverse proxies) and falls back to RemoteAddr.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr := parseClientIP(r)
		ctx := context.WithValue(r.Context(), clientIPKey, addr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext returns the client IP stored by [Middleware].
// Returns a zero-value netip.Addr if the middleware was not applied or the
// IP could not be parsed.
func FromContext(ctx context.Context) netip.Addr {
	if addr, ok := ctx.Value(clientIPKey).(netip.Addr); ok {
		return addr
	}
	return netip.Addr{}
}

func parseClientIP(r *http.Request) netip.Addr {
	// Check X-Real-Ip first (typically set by nginx).
	if ip := strings.TrimSpace(r.Header.Get("X-Real-Ip")); ip != "" {
		if addr, err := netip.ParseAddr(ip); err == nil {
			return addr
		}
	}
	// X-Forwarded-For may contain "client, proxy1, proxy2" — use leftmost.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			if addr, err := netip.ParseAddr(ip); err == nil {
				return addr
			}
		}
	}
	// Fall back to RemoteAddr (host:port).
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, _ := netip.ParseAddr(host)
	return addr
}
