package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := middleware.NewRateLimiter(3, time.Minute)

	assert.True(t, rl.Allow("key1"))
	assert.True(t, rl.Allow("key1"))
	assert.True(t, rl.Allow("key1"))
	assert.False(t, rl.Allow("key1"), "4th request within window should be rejected")

	// A different key has its own independent budget.
	assert.True(t, rl.Allow("key2"))
}

func TestRateLimiter_Allow_WindowExpires(t *testing.T) {
	rl := middleware.NewRateLimiter(1, 30*time.Millisecond)

	assert.True(t, rl.Allow("key1"))
	assert.False(t, rl.Allow("key1"))

	time.Sleep(50 * time.Millisecond)
	assert.True(t, rl.Allow("key1"), "request should be allowed again after window elapses")
}

func TestRateLimiter_Remaining(t *testing.T) {
	rl := middleware.NewRateLimiter(2, time.Minute)

	assert.Equal(t, 2, rl.Remaining("key1"))

	rl.Allow("key1")
	assert.Equal(t, 1, rl.Remaining("key1"))

	rl.Allow("key1")
	assert.Equal(t, 0, rl.Remaining("key1"))

	// Remaining must never go negative even if Allow is somehow called past
	// the limit (e.g. via a race), and must not itself consume a slot.
	rl.Allow("key1")
	assert.Equal(t, 0, rl.Remaining("key1"))
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := middleware.NewRateLimiter(1, time.Minute)
	handler := middleware.RateLimitMiddleware(rl, func(r *http.Request) string { return "fixed-key" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
	assert.Equal(t, "60", w2.Header().Get("Retry-After"))
}

func TestUserRateLimitKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "user-123")
	req = req.WithContext(ctx)

	assert.Equal(t, "user:user-123", middleware.UserRateLimitKey(req))

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "1.2.3.4:5678"
	assert.Equal(t, "ip:1.2.3.4:5678", middleware.UserRateLimitKey(req2))
}

func TestGlobalRateLimitKey(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Equal(t, "global", middleware.GlobalRateLimitKey(req))
}

func TestRateLimitHeadersMiddleware(t *testing.T) {
	rl := middleware.NewRateLimiter(5, time.Minute)
	handler := middleware.RateLimitHeadersMiddleware(rl, 5, time.Minute)(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), middleware.UserIDKey, "user-123")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, "5", w.Header().Get("X-RateLimit-Limit"))
	assert.Equal(t, "5", w.Header().Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, w.Header().Get("X-RateLimit-Reset"))
}

func TestRequestIDMiddleware(t *testing.T) {
	m := middleware.NewRequestIDMiddleware()
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := middleware.GetRequestIDFromContext(r.Context())
		assert.NotEmpty(t, id)
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("generates request id when absent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		require.NotEmpty(t, w.Header().Get("X-Request-ID"))
	})

	t.Run("reuses incoming request id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Request-ID", "my-request-id")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, "my-request-id", w.Header().Get("X-Request-ID"))
	})
}

func TestGetRequestIDFromContext_Empty(t *testing.T) {
	assert.Equal(t, "", middleware.GetRequestIDFromContext(context.Background()))
}
