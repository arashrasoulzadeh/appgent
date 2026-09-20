package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/auth"
	"github.com/arashrasoulzadeh/appgent/internal/logger"
)

type contextKey string

const (
	UserIDKey  contextKey = "user_id"
	EmailKey   contextKey = "email"
)

func AuthMiddleware(tokenService *auth.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := tokenService.GetTokenFromCookie(r)
			if err != nil {
				http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
				return
			}

			claims, err := tokenService.ValidateToken(token)
			if err != nil {
				http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, EmailKey, claims.Email)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserID(r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(UserIDKey).(string)
	return userID, ok
}

func GetEmail(r *http.Request) (string, bool) {
	email, ok := r.Context().Value(EmailKey).(string)
	return email, ok
}

func CORSMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func JSONMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func LoggingMiddleware(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			requestID := ""
			if id := r.Context().Value(RequestIDKeyVal); id != nil {
				requestID = id.(string)
			}

			log.Info("HTTP request started",
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", requestID,
			)

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)
			log.Info("HTTP request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"request_id", requestID,
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}