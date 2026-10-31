package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ubaid/hisaab/internal/auth"
)

func TestInProcessLimiter_SlidingWindowEnforcement(t *testing.T) {
	limiter := auth.NewInProcessLimiter()
	ctx := context.Background()

	rate := auth.Rate{Limit: 3, Window: 100 * time.Millisecond}
	key := "test-participant-1"

	// Request 1: Allowed (remaining 2)
	allowed, remaining, _, err := limiter.Allow(ctx, key, rate)
	if err != nil || !allowed || remaining != 2 {
		t.Fatalf("req 1 failed: allowed=%v, rem=%d, err=%v", allowed, remaining, err)
	}

	// Request 2: Allowed (remaining 1)
	allowed, remaining, _, _ = limiter.Allow(ctx, key, rate)
	if !allowed || remaining != 1 {
		t.Fatalf("req 2 failed: allowed=%v, rem=%d", allowed, remaining)
	}

	// Request 3: Allowed (remaining 0)
	allowed, remaining, _, _ = limiter.Allow(ctx, key, rate)
	if !allowed || remaining != 0 {
		t.Fatalf("req 3 failed: allowed=%v, rem=%d", allowed, remaining)
	}

	// Request 4: Exceeded quota
	allowed, _, reset, _ := limiter.Allow(ctx, key, rate)
	if allowed {
		t.Fatalf("req 4 expected to be rate limited, but was allowed")
	}
	if reset <= 0 {
		t.Fatalf("expected positive reset duration, got %v", reset)
	}

	// Wait for window to slide
	time.Sleep(110 * time.Millisecond)

	// Request 5: Allowed after window expiration
	allowed, remaining, _, _ = limiter.Allow(ctx, key, rate)
	if !allowed || remaining != 2 {
		t.Fatalf("req 5 expected to be allowed after window reset, got allowed=%v, rem=%d", allowed, remaining)
	}
}

func TestRateLimitMiddleware_HeadersAndRejection(t *testing.T) {
	limiter := auth.NewInProcessLimiter()
	rate := auth.Rate{Limit: 2, Window: 1 * time.Second}

	keyFunc := func(r *http.Request) string {
		return r.Header.Get("X-Participant-Id")
	}

	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := auth.RateLimitMiddleware(limiter, keyFunc, rate)(baseHandler)

	req1 := httptest.NewRequest(http.MethodPost, "/settle", nil)
	req1.Header.Set("X-Participant-Id", "np-bap-1")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("req 1 failed: %d", rec1.Code)
	}
	if rec1.Header().Get("X-RateLimit-Limit") != "2" || rec1.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Fatalf("unexpected headers: %v", rec1.Header())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/settle", nil)
	req2.Header.Set("X-Participant-Id", "np-bap-1")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("req 2 failed: %d", rec2.Code)
	}

	// 3rd request from same participant rejected with 429
	req3 := httptest.NewRequest(http.MethodPost, "/settle", nil)
	req3.Header.Set("X-Participant-Id", "np-bap-1")
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec3.Code)
	}
	if rec3.Header().Get("Retry-After") == "" {
		t.Fatalf("missing Retry-After header on 429 response")
	}

	// Request from different participant allowed
	reqDiff := httptest.NewRequest(http.MethodPost, "/settle", nil)
	reqDiff.Header.Set("X-Participant-Id", "np-bpp-2")
	recDiff := httptest.NewRecorder()
	handler.ServeHTTP(recDiff, reqDiff)
	if recDiff.Code != http.StatusOK {
		t.Fatalf("different participant rejected: %d", recDiff.Code)
	}
}
