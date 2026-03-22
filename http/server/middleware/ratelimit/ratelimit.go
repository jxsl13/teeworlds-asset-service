package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jxsl13/teeworlds-asset-service/http/server/middleware/clientip"
)

// ipBucket is a token-bucket state for a single client IP.
type ipBucket struct {
	tokens    float64
	lastRefil time.Time
}

// RateLimiter is an HTTP middleware that enforces a per-IP token-bucket rate
// limit. IPs that exhaust their bucket receive HTTP 429.
//
// Memory safety: a background goroutine evicts buckets that have been idle
// for longer than cleanupAfter, so the map cannot grow indefinitely.
type RateLimiter struct {
	rate         float64
	burst        float64
	cleanupAfter time.Duration

	mu      sync.Mutex
	buckets map[string]*ipBucket
}

// New creates a RateLimiter and starts its background cleanup goroutine.
// The goroutine stops when ctx is cancelled.
func New(ctx context.Context, rate float64, burst int, cleanupAfter time.Duration) *RateLimiter {
	rl := &RateLimiter{
		rate:         rate,
		burst:        float64(burst),
		cleanupAfter: cleanupAfter,
		buckets:      make(map[string]*ipBucket),
	}
	go rl.cleanupLoop(ctx, cleanupAfter/2)
	return rl
}

// Middleware returns an http.Handler middleware that applies the rate limit.
// It must be used after clientip.Middleware so the IP is available in the context.
// Loopback addresses are exempt from rate limiting.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		addr := clientip.FromContext(r.Context())
		if !addr.IsValid() || addr.IsLoopback() {
			next.ServeHTTP(w, r)
			return
		}

		key := addr.String()
		if !rl.allow(key) {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(ip string) bool {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[ip]
	if !ok {
		b = &ipBucket{tokens: rl.burst, lastRefil: now}
		rl.buckets[ip] = b
	}

	elapsed := now.Sub(b.lastRefil).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst {
		b.tokens = rl.burst
	}
	b.lastRefil = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *RateLimiter) cleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.evictIdle()
		case <-ctx.Done():
			return
		}
	}
}

func (rl *RateLimiter) evictIdle() {
	cutoff := time.Now().Add(-rl.cleanupAfter)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for ip, b := range rl.buckets {
		if b.lastRefil.Before(cutoff) {
			delete(rl.buckets, ip)
		}
	}
}
