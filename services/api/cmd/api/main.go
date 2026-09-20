package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/config"
	"github.com/arashrasoulzadeh/appgent/internal/db"
	"github.com/arashrasoulzadeh/appgent/internal/handlers"
	"github.com/arashrasoulzadeh/appgent/internal/logger"
	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/arashrasoulzadeh/appgent/internal/auth"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()

	// Initialize structured logger
	log := logger.New(cfg.LogLevel, "json", os.Stdout)
	log.Info("Starting API server", "port", cfg.Port)

	pool := db.MustNewPool(ctx, cfg.PostgresDSN)
	defer pool.Close()

	// Schema migrations are applied out-of-band via the `migrate` service
	// (see docs/deploy.md / docs/dev-setup.md) against internal/db/migrations,
	// not by the API process itself.

	// Initialize services
	tokenService := auth.NewTokenService(
		cfg.JWTSigningSecret,
		cfg.SessionCookieName,
		cfg.SessionTTLHours,
		false, // secure = false for local dev
	)

	appService := services.NewAppService(pool)

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(tokenService, appService)
	appHandler := handlers.NewAppHandler(appService)
	deploymentHandler := handlers.NewDeploymentHandler(appService)
	previewHandler := handlers.NewPreviewHandler(appService)
	sseHandler := handlers.NewSSEHandler(appService)
	healthHandler := handlers.NewHealthHandler(pool)

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter(100, time.Minute) // 100 requests per minute per user

	// Setup routes
	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /healthz", healthHandler.Healthz)

	// Protected routes
	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)
	protected.HandleFunc("GET /api/v1/auth/me", authHandler.Me)

	protected.HandleFunc("GET /api/v1/apps", appHandler.ListApps)
	protected.HandleFunc("POST /api/v1/apps", appHandler.CreateApp)
	protected.HandleFunc("GET /api/v1/apps/{app_id}", appHandler.GetApp)
	protected.HandleFunc("DELETE /api/v1/apps/{app_id}", appHandler.DeleteApp)
	protected.HandleFunc("POST /api/v1/apps/{app_id}/regenerate", appHandler.RegenerateApp)

	protected.HandleFunc("GET /api/v1/apps/{app_id}/runs", appHandler.ListRuns)
	protected.HandleFunc("GET /api/v1/apps/{app_id}/runs/{run_id}", appHandler.GetRun)
	protected.HandleFunc("GET /api/v1/apps/{app_id}/runs/{run_id}/events", sseHandler.RunEvents)

	protected.HandleFunc("POST /api/v1/apps/{app_id}/deploy", deploymentHandler.DeployApp)
	protected.HandleFunc("GET /api/v1/apps/{app_id}/deployments", deploymentHandler.ListDeployments)

	protected.HandleFunc("GET /api/v1/apps/{app_id}/runs/{run_id}/preview", previewHandler.GetPreviewURL)

	// Apply middleware chain
	handler := middleware.CORSMiddleware(cfg.CORSAllowedOrigin)(
		middleware.JSONMiddleware(
			middleware.NewRequestIDMiddleware().Middleware(
				middleware.LoggingMiddleware(log)(
					middleware.RateLimitMiddleware(rateLimiter, middleware.UserRateLimitKey)(
						http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							// Route to protected or public
							if r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/healthz" {
								mux.ServeHTTP(w, r)
							} else {
								middleware.AuthMiddleware(tokenService)(protected).ServeHTTP(w, r)
							}
						}),
					),
				),
			),
		),
	)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Info("Starting API server", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error("Server shutdown error", "error", err)
	}
	log.Info("Server stopped")
}
