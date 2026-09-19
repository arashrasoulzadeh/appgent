package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenService_GenerateAndValidate(t *testing.T) {
	ts := NewTokenService("test-secret", "session", 1, false)

	token, err := ts.GenerateToken("user-123", "test@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := ts.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.True(t, claims.ExpiresAt.Time.After(time.Now()))
}

func TestTokenService_ValidateExpiredToken(t *testing.T) {
	ts := NewTokenService("test-secret", "session", -1, false) // Negative TTL = expired

	token, err := ts.GenerateToken("user-123", "test@example.com")
	require.NoError(t, err)

	claims, err := ts.ValidateToken(token)
	assert.Error(t, err)
	assert.Nil(t, claims)
	assert.ErrorIs(t, err, ErrExpiredToken)
}

func TestTokenService_ValidateInvalidToken(t *testing.T) {
	ts := NewTokenService("test-secret", "session", 1, false)

	_, err := ts.ValidateToken("invalid-token")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestTokenService_ValidateWrongSecret(t *testing.T) {
	ts1 := NewTokenService("secret-1", "session", 1, false)
	ts2 := NewTokenService("secret-2", "session", 1, false)

	token, err := ts1.GenerateToken("user-123", "test@example.com")
	require.NoError(t, err)

	_, err = ts2.ValidateToken(token)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestTokenService_SetAndClearCookie(t *testing.T) {
	ts := NewTokenService("test-secret", "test-session", 1, false)

	w := httptest.NewRecorder()
	ts.SetCookie(w, "test-token")

	cookies := w.Result().Cookies()
	require.Len(t, cookies, 1)
	cookie := cookies[0]
	assert.Equal(t, "test-session", cookie.Name)
	assert.Equal(t, "test-token", cookie.Value)
	assert.Equal(t, "/", cookie.Path)
	assert.True(t, cookie.HttpOnly)
	assert.False(t, cookie.Secure) // false for local dev
	assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite)

	// Clear cookie
	w2 := httptest.NewRecorder()
	ts.ClearCookie(w2)

	cookies2 := w2.Result().Cookies()
	require.Len(t, cookies2, 1)
	cookie2 := cookies2[0]
	assert.Equal(t, "test-session", cookie2.Name)
	assert.Equal(t, "", cookie2.Value)
	assert.Equal(t, -1, cookie2.MaxAge)
}

func TestTokenService_GetTokenFromCookie(t *testing.T) {
	ts := NewTokenService("test-secret", "test-session", 1, false)

	// With cookie
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "test-session", Value: "my-token"})

	token, err := ts.GetTokenFromCookie(req)
	require.NoError(t, err)
	assert.Equal(t, "my-token", token)

	// Without cookie
	req2 := httptest.NewRequest("GET", "/", nil)
	_, err = ts.GetTokenFromCookie(req2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNoToken)
}

func TestClaims_RegisteredClaims(t *testing.T) {
	ts := NewTokenService("test-secret", "session", 1, false)

	token, err := ts.GenerateToken("user-123", "test@example.com")
	require.NoError(t, err)

	// Parse without validation to check claims
	parsedToken, _, err := new(jwt.Parser).ParseUnverified(token, &Claims{})
	require.NoError(t, err)

	claims, ok := parsedToken.Claims.(*Claims)
	require.True(t, ok)
	assert.Equal(t, "user-123", claims.Subject)
	assert.Equal(t, "user-123", claims.UserID)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.ExpiresAt)
	assert.NotNil(t, claims.NotBefore)
}