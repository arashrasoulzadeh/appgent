package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCORSMiddleware(t *testing.T) {
	handler := CORSMiddleware("http://localhost:3000")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test preflight
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "http://localhost:3000", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))

	// Test actual request
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Origin", "http://localhost:3000")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, "http://localhost:3000", w2.Header().Get("Access-Control-Allow-Origin"))
}

func TestJSONMiddleware(t *testing.T) {
	handler := JSONMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret", "session", 1, false)
	token, err := ts.GenerateToken("user-123", "test@example.com")
	require.NoError(t, err)

	handler := AuthMiddleware(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := GetUserID(r)
		require.True(t, ok)
		assert.Equal(t, "user-123", userID)

		email, ok := GetEmail(r)
		require.True(t, ok)
		assert.Equal(t, "test@example.com", email)

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	ts := auth.NewTokenService("test-secret", "session", 1, false)

	handler := AuthMiddleware(ts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No cookie
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "unauthorized")

	// Invalid token
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: "session", Value: "invalid-token"})
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusUnauthorized, w2.Code)

	// Expired token
	ts2 := auth.NewTokenService("test-secret", "session", -1, false)
	expiredToken, _ := ts2.GenerateToken("user-123", "test@example.com")

	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.AddCookie(&http.Cookie{Name: "session", Value: expiredToken})
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	assert.Equal(t, http.StatusUnauthorized, w3.Code)
}

func TestAuthMiddleware_WrongSecret(t *testing.T) {
	ts1 := auth.NewTokenService("secret-1", "session", 1, false)
	ts2 := auth.NewTokenService("secret-2", "session", 1, false)

	token, _ := ts1.GenerateToken("user-123", "test@example.com")

	handler := AuthMiddleware(ts2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestLoggingMiddleware(t *testing.T) {
	// We can't easily test the internal log.Printf, so just verify it doesn't panic
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetUserID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, UserIDKey, "user-123")
	req = req.WithContext(ctx)

	userID, ok := GetUserID(req)
	assert.True(t, ok)
	assert.Equal(t, "user-123", userID)

	// Without value
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	userID2, ok2 := GetUserID(req2)
	assert.False(t, ok2)
	assert.Empty(t, userID2)
}

func TestGetEmail(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, EmailKey, "test@example.com")
	req = req.WithContext(ctx)

	email, ok := GetEmail(req)
	assert.True(t, ok)
	assert.Equal(t, "test@example.com", email)

	// Without value
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	email2, ok2 := GetEmail(req2)
	assert.False(t, ok2)
	assert.Empty(t, email2)
}