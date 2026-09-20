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
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()

	// Initialize structured logger
	log := logger.New(cfg.LogLevel, "json", os.Stdout)
	log.Info("Starting API server", "port", cfg.Port)

	pool := db.MustNewPool(ctx, cfg.PostgresDSN)
	defer pool.Close()

	// Run migrations
	if err := runMigrations(ctx, pool); err != nil {
		log.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}

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

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return err
	}

	migrations := []struct {
		version string
		sql     string
	}{
		{"0001_users", usersMigration},
		{"0002_apps", appsMigration},
		{"0003_generation_runs", generationRunsMigration},
		{"0004_agent_steps", agentStepsMigration},
		{"0005_deployments", deploymentsMigration},
		{"0006_design_patterns", designPatternsMigration},
	}

	for _, m := range migrations {
		var applied bool
		err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", m.version).Scan(&applied)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, m.sql)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}

		_, err = tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.version)
		if err != nil {
			tx.Rollback(ctx)
			return err
		}

		if err := tx.Commit(ctx); err != nil {
			return err
		}

		logger.DefaultLogger.Info("Applied migration", "version", m.version)
	}

	return nil
}

const usersMigration = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text UNIQUE NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

INSERT INTO users (id, email, password_hash) 
VALUES ('00000000-0000-0000-0000-000000000001', 'admin', '$2a$10$dummyhash')
ON CONFLICT (email) DO NOTHING;
`

const appsMigration = `
CREATE TYPE app_kind AS ENUM ('website', 'pwa');
CREATE TYPE app_status AS ENUM ('draft', 'generating', 'ready', 'needs_review', 'failed');

CREATE TABLE IF NOT EXISTS apps (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    slug       text UNIQUE NOT NULL,
    kind       app_kind NOT NULL,
    status     app_status NOT NULL DEFAULT 'draft',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_apps_user_id ON apps(user_id);
`

const generationRunsMigration = `
CREATE TYPE run_status AS ENUM ('queued', 'running', 'succeeded', 'needs_review', 'failed');

CREATE TABLE IF NOT EXISTS generation_runs (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id                uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    version               int NOT NULL,
    user_prompt           text NOT NULL,
    status                run_status NOT NULL DEFAULT 'queued',
    temporal_workflow_id  text NOT NULL,
    bundle_path           text,
    preview_url           text,
    error                 text,
    started_at            timestamptz,
    finished_at           timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    UNIQUE (app_id, version)
);
CREATE INDEX IF NOT EXISTS idx_generation_runs_app_id ON generation_runs(app_id);
`

const agentStepsMigration = `
CREATE TYPE agent_type AS ENUM ('plan', 'design', 'code', 'qa');
CREATE TYPE agent_step_status AS ENUM ('pending', 'running', 'succeeded', 'failed');

CREATE TABLE IF NOT EXISTS agent_steps (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id       uuid NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    agent_type   agent_type NOT NULL,
    attempt      int NOT NULL DEFAULT 1,
    input        jsonb NOT NULL,
    output       jsonb,
    model_used   text NOT NULL,
    tokens_used  int,
    status       agent_step_status NOT NULL DEFAULT 'pending',
    error        text,
    started_at   timestamptz,
    finished_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agent_steps_run_id ON agent_steps(run_id);
`

const deploymentsMigration = `
CREATE TYPE deployment_status AS ENUM ('deploying', 'live', 'failed', 'retired');

CREATE TABLE IF NOT EXISTS deployments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    app_id       uuid NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    run_id       uuid NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    url          text,
    status       deployment_status NOT NULL DEFAULT 'deploying',
    deployed_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_deployments_app_id ON deployments(app_id);
`

const designPatternsMigration = `
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS design_patterns (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind       text NOT NULL,
    title      text NOT NULL,
    content    text NOT NULL,
    embedding  vector(1536) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_design_patterns_embedding ON design_patterns
    USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
`