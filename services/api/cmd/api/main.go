package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/arashrasoulzadeh/appgent/internal/auth"
	"github.com/arashrasoulzadeh/appgent/internal/config"
	"github.com/arashrasoulzadeh/appgent/internal/db"
	"github.com/arashrasoulzadeh/appgent/internal/handlers"
	"github.com/arashrasoulzadeh/appgent/internal/logger"
	"github.com/arashrasoulzadeh/appgent/internal/middleware"
	"github.com/arashrasoulzadeh/appgent/internal/sandbox"
	"github.com/arashrasoulzadeh/appgent/internal/services"
	"github.com/arashrasoulzadeh/appgent/internal/storage"
	temporalclient "go.temporal.io/sdk/client"
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

	temporalClient, err := temporalclient.Dial(temporalclient.Options{
		HostPort:  cfg.TemporalHostPort,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		log.Error("Failed to connect to Temporal", "error", err)
		os.Exit(1)
	}
	defer temporalClient.Close()

	// Initialize services
	tokenService := auth.NewTokenService(
		cfg.JWTSigningSecret,
		cfg.SessionCookieName,
		cfg.SessionTTLHours,
		false, // secure = false for local dev
	)

	storageClient, err := storage.NewClient(
		cfg.ObjectStorageEndpoint,
		cfg.ObjectStorageAccessKey,
		cfg.ObjectStorageSecretKey,
		false,
	)
	if err != nil {
		log.Error("Failed to connect to object storage", "error", err)
		os.Exit(1)
	}
	appService := services.NewAppService(pool, temporalClient)
	appService.SetProvisioner(sandbox.NewStaticExportProvisioner(storageClient, cfg.ObjectStorageBucket))

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(tokenService, appService)
	appHandler := handlers.NewAppHandler(appService)
	deploymentHandler := handlers.NewDeploymentHandler(appService)
	previewHandler := handlers.NewPreviewHandler(appService, storageClient, cfg.ObjectStorageBucket)
	sseHandler := handlers.NewSSEHandler(appService)
	healthHandler := handlers.NewHealthHandler(pool)

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter(100, time.Minute) // 100 requests per minute per user

	// Setup routes
	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /healthz", healthHandler.Healthz)
	// A deployment's whole point is being a shareable public URL, unlike a
	// run preview (which stays behind auth, below) — deliberately public.
	mux.HandleFunc("GET /api/v1/apps/{app_id}/live/{path...}", previewHandler.ServeLiveFile)

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
	protected.HandleFunc("GET /api/v1/apps/{app_id}/runs/{run_id}/preview/{path...}", previewHandler.ServeRunFile)
	protected.HandleFunc("GET /api/v1/apps/{app_id}/runs/{run_id}/files", previewHandler.GetRunFiles)

	// Apply middleware chain
	handler := middleware.CORSMiddleware(cfg.CORSAllowedOrigin)(
		middleware.JSONMiddleware(
			middleware.NewRequestIDMiddleware().Middleware(
				middleware.LoggingMiddleware(log)(
					middleware.RateLimitMiddleware(rateLimiter, middleware.UserRateLimitKey)(
						http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							// Route to protected or public. Deployment
							// "live" URLs are the one dynamic public path
							// (.../apps/{id}/live/...) — everything else
							// dynamic requires auth.
							if r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/healthz" || isLiveDeploymentPath(r.URL.Path) {
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

var liveDeploymentPathRe = regexp.MustCompile(`^/api/v1/apps/[^/]+/live(/.*)?$`)

func isLiveDeploymentPath(path string) bool {
	return liveDeploymentPathRe.MatchString(path)
}
