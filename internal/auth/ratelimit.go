package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Rate defines the maximum allowed operations within a given time duration.
type Rate struct {
	Limit  int           // maximum requests allowed
	Window time.Duration // sliding window duration
}

// RateLimiter defines the contract for request rate governance specified in ADR-003.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit Rate) (allowed bool, remaining int, reset time.Duration, err error)
}

type windowEntry struct {
	timestamps []time.Time
}

// InProcessLimiter implements RateLimiter using an in-memory sliding window log.
type InProcessLimiter struct {
	mu      sync.Mutex
	windows map[string]*windowEntry
}

// NewInProcessLimiter initializes an in-process sliding window rate limiter.
func NewInProcessLimiter() *InProcessLimiter {
	return &InProcessLimiter{
		windows: make(map[string]*windowEntry),
	}
}

// Allow evaluates whether a request identified by key is within quota.
func (l *InProcessLimiter) Allow(ctx context.Context, key string, limit Rate) (bool, int, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-limit.Window)

	entry, exists := l.windows[key]
	if !exists {
		entry = &windowEntry{timestamps: make([]time.Time, 0, limit.Limit)}
		l.windows[key] = entry
	}

	// Purge timestamps older than sliding window cutoff
	validIdx := 0
	for i, t := range entry.timestamps {
		if t.After(cutoff) {
			validIdx = i
			break
		}
		if i == len(entry.timestamps)-1 {
			validIdx = len(entry.timestamps)
		}
	}
	entry.timestamps = entry.timestamps[validIdx:]

	count := len(entry.timestamps)
	if count >= limit.Limit {
		// Calculate time until oldest timestamp in window expires
		oldest := entry.timestamps[0]
		resetDuration := oldest.Add(limit.Window).Sub(now)
		if resetDuration < 0 {
			resetDuration = 0
		}
		return false, 0, resetDuration, nil
	}

	entry.timestamps = append(entry.timestamps, now)
	remaining := limit.Limit - (count + 1)
	return true, remaining, limit.Window, nil
}

// RateLimitMiddleware returns an HTTP middleware wrapping a handler with rate governance.
func RateLimitMiddleware(limiter RateLimiter, keyFunc func(r *http.Request) string, rate Rate) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if key == "" {
				key = r.RemoteAddr
			}

			allowed, remaining, reset, err := limiter.Allow(r.Context(), key, rate)
			if err != nil {
				http.Error(w, `{"error":"rate limiter failure"}`, http.StatusInternalServerError)
				return
			}

			resetSeconds := int(reset.Seconds())
			if resetSeconds < 1 {
				resetSeconds = 1
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rate.Limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", resetSeconds))

			if !allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", resetSeconds))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":               "rate limit exceeded",
					"code":                "RATE_LIMIT_EXCEEDED",
					"limit":               rate.Limit,
					"retry_after_seconds": resetSeconds,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
