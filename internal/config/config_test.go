package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env vars that might affect defaults
	os.Unsetenv("API_PORT")
	os.Unsetenv("POSTGRES_DSN")
	os.Unsetenv("TEMPORAL_HOST_PORT")
	os.Unsetenv("TEMPORAL_NAMESPACE")
	os.Unsetenv("OBJECT_STORAGE_ENDPOINT")
	os.Unsetenv("OBJECT_STORAGE_BUCKET")
	os.Unsetenv("OBJECT_STORAGE_ACCESS_KEY")
	os.Unsetenv("OBJECT_STORAGE_SECRET_KEY")
	os.Unsetenv("JWT_SIGNING_SECRET")
	os.Unsetenv("SESSION_COOKIE_NAME")
	os.Unsetenv("SESSION_TTL_HOURS")
	os.Unsetenv("CORS_ALLOWED_ORIGIN")
	os.Unsetenv("LOG_LEVEL")

	cfg := Load()

	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "", cfg.PostgresDSN)
	assert.Equal(t, "temporal:7233", cfg.TemporalHostPort)
	assert.Equal(t, "default", cfg.TemporalNamespace)
	assert.Equal(t, "http://minio:9000", cfg.ObjectStorageEndpoint)
	assert.Equal(t, "appgent-runs", cfg.ObjectStorageBucket)
	assert.Equal(t, "minioadmin", cfg.ObjectStorageAccessKey)
	assert.Equal(t, "minioadmin", cfg.ObjectStorageSecretKey)
	assert.Equal(t, "change-me-in-prod", cfg.JWTSigningSecret)
	assert.Equal(t, "session", cfg.SessionCookieName)
	assert.Equal(t, 168, cfg.SessionTTLHours)
	assert.Equal(t, "http://localhost:3000", cfg.CORSAllowedOrigin)
	assert.Equal(t, "info", cfg.LogLevel)
}

func TestLoad_FromEnv(t *testing.T) {
	os.Setenv("API_PORT", "9090")
	os.Setenv("POSTGRES_DSN", "postgres://test:test@localhost:5432/test")
	os.Setenv("TEMPORAL_HOST_PORT", "custom:7233")
	os.Setenv("TEMPORAL_NAMESPACE", "custom-ns")
	os.Setenv("OBJECT_STORAGE_ENDPOINT", "http://custom:9000")
	os.Setenv("OBJECT_STORAGE_BUCKET", "custom-bucket")
	os.Setenv("OBJECT_STORAGE_ACCESS_KEY", "custom-key")
	os.Setenv("OBJECT_STORAGE_SECRET_KEY", "custom-secret")
	os.Setenv("JWT_SIGNING_SECRET", "custom-secret")
	os.Setenv("SESSION_COOKIE_NAME", "custom-session")
	os.Setenv("SESSION_TTL_HOURS", "24")
	os.Setenv("CORS_ALLOWED_ORIGIN", "http://custom:3000")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("API_PORT")
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("TEMPORAL_HOST_PORT")
		os.Unsetenv("TEMPORAL_NAMESPACE")
		os.Unsetenv("OBJECT_STORAGE_ENDPOINT")
		os.Unsetenv("OBJECT_STORAGE_BUCKET")
		os.Unsetenv("OBJECT_STORAGE_ACCESS_KEY")
		os.Unsetenv("OBJECT_STORAGE_SECRET_KEY")
		os.Unsetenv("JWT_SIGNING_SECRET")
		os.Unsetenv("SESSION_COOKIE_NAME")
		os.Unsetenv("SESSION_TTL_HOURS")
		os.Unsetenv("CORS_ALLOWED_ORIGIN")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg := Load()

	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "postgres://test:test@localhost:5432/test", cfg.PostgresDSN)
	assert.Equal(t, "custom:7233", cfg.TemporalHostPort)
	assert.Equal(t, "custom-ns", cfg.TemporalNamespace)
	assert.Equal(t, "http://custom:9000", cfg.ObjectStorageEndpoint)
	assert.Equal(t, "custom-bucket", cfg.ObjectStorageBucket)
	assert.Equal(t, "custom-key", cfg.ObjectStorageAccessKey)
	assert.Equal(t, "custom-secret", cfg.ObjectStorageSecretKey)
	assert.Equal(t, "custom-secret", cfg.JWTSigningSecret)
	assert.Equal(t, "custom-session", cfg.SessionCookieName)
	assert.Equal(t, 24, cfg.SessionTTLHours)
	assert.Equal(t, "http://custom:3000", cfg.CORSAllowedOrigin)
	assert.Equal(t, "debug", cfg.LogLevel)
}

func TestLoad_SessionTTLHours_InvalidDefaults(t *testing.T) {
	os.Setenv("SESSION_TTL_HOURS", "invalid")
	defer os.Unsetenv("SESSION_TTL_HOURS")

	cfg := Load()

	assert.Equal(t, 168, cfg.SessionTTLHours) // Should default to 168
}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_KEY", "test-value")
	defer os.Unsetenv("TEST_KEY")

	assert.Equal(t, "test-value", getEnv("TEST_KEY", "default"))
	assert.Equal(t, "default", getEnv("NONEXISTENT_KEY", "default"))
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	assert.Equal(t, 42, getEnvInt("TEST_INT", 0))
	assert.Equal(t, 0, getEnvInt("NONEXISTENT_INT", 0))

	os.Setenv("TEST_INVALID", "not-a-number")
	defer os.Unsetenv("TEST_INVALID")
	assert.Equal(t, 0, getEnvInt("TEST_INVALID", 0))
}