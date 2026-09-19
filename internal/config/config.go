package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                 string
	PostgresDSN          string
	TemporalHostPort     string
	TemporalNamespace    string
	ObjectStorageEndpoint string
	ObjectStorageBucket  string
	ObjectStorageAccessKey string
	ObjectStorageSecretKey string
	JWTSigningSecret     string
	SessionCookieName    string
	SessionTTLHours      int
	CORSAllowedOrigin    string
	LogLevel             string
}

func Load() *Config {
	return &Config{
		Port:                 getEnv("API_PORT", "8080"),
		PostgresDSN:          getEnv("POSTGRES_DSN", ""),
		TemporalHostPort:     getEnv("TEMPORAL_HOST_PORT", "temporal:7233"),
		TemporalNamespace:    getEnv("TEMPORAL_NAMESPACE", "default"),
		ObjectStorageEndpoint: getEnv("OBJECT_STORAGE_ENDPOINT", "http://minio:9000"),
		ObjectStorageBucket:  getEnv("OBJECT_STORAGE_BUCKET", "appgent-runs"),
		ObjectStorageAccessKey: getEnv("OBJECT_STORAGE_ACCESS_KEY", "minioadmin"),
		ObjectStorageSecretKey: getEnv("OBJECT_STORAGE_SECRET_KEY", "minioadmin"),
		JWTSigningSecret:     getEnv("JWT_SIGNING_SECRET", "change-me-in-prod"),
		SessionCookieName:    getEnv("SESSION_COOKIE_NAME", "session"),
		SessionTTLHours:      getEnvInt("SESSION_TTL_HOURS", 168),
		CORSAllowedOrigin:    getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:3000"),
		LogLevel:             getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i
		}
	}
	return defaultValue
}