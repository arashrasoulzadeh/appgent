package middleware

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type RateLimiter struct {
	mu           sync.Mutex
	requests     map[string][]time.Time
	maxRequests  int
	window       time.Duration
	cleanupInterval time.Duration
}

func NewRateLimiter(maxRequests int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests:        make(map[string][]time.Time),
		maxRequests:     maxRequests,
		window:          window,
		cleanupInterval: window,
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	validRequests := []time.Time{}
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}

	if len(validRequests) >= rl.maxRequests {
		rl.requests[key] = validRequests
		return false
	}

	validRequests = append(validRequests, now)
	rl.requests[key] = validRequests
	return true
}

// Remaining returns how many more requests key may make in the current
// window without actually consuming one.
func (rl *RateLimiter) Remaining(key string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.window)
	count := 0
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			count++
		}
	}
	remaining := rl.maxRequests - count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)
		for key, times := range rl.requests {
			valid := []time.Time{}
			for _, t := range times {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func RateLimitMiddleware(limiter *RateLimiter, keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !limiter.Allow(key) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"error": "rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func UserRateLimitKey(r *http.Request) string {
	if userID, ok := GetUserID(r); ok {
		return "user:" + userID
	}
	return "ip:" + r.RemoteAddr
}

func GlobalRateLimitKey(r *http.Request) string {
	return "global"
}

type rateLimitKey string

const RateLimitLimitKey rateLimitKey = "rate_limit_limit"
const RateLimitRemainingKey rateLimitKey = "rate_limit_remaining"
const RateLimitResetKey rateLimitKey = "rate_limit_reset"

func RateLimitHeadersMiddleware(limiter *RateLimiter, maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := UserRateLimitKey(r)
			remaining := limiter.Remaining(key)
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(maxRequests))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(window).Unix(), 10))
			next.ServeHTTP(w, r)
		})
	}
}

type RequestIDMiddleware struct {
	generator func() string
}

func NewRequestIDMiddleware() *RequestIDMiddleware {
	return &RequestIDMiddleware{
		generator: func() string {
			return uuid.New().String()
		},
	}
}

func (m *RequestIDMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = m.generator()
		}

		ctx := context.WithValue(r.Context(), RequestIDKeyVal, requestID)
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type RequestIDKey string

const RequestIDKeyVal RequestIDKey = "request_id"

func GetRequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKeyVal).(string); ok {
		return id
	}
	return ""
}