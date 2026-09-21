.PHONY: help build test lint fmt vet coverage run-api run-worker run-frontend dev clean

# Default target
help:
	@echo "Appgent - Development Commands"
	@echo ""
	@echo "Building:"
	@echo "  make build        - Build all Go binaries"
	@echo "  make build-api    - Build API server"
	@echo "  make build-worker - Build Temporal worker"
	@echo ""
	@echo "Testing:"
	@echo "  make test         - Run all Go tests"
	@echo "  make test-e2e     - Run E2E tests (requires infra)"
	@echo "  make coverage     - Run tests with coverage"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint         - Run golangci-lint"
	@echo "  make fmt          - Format Go code"
	@echo "  make vet          - Run go vet"
	@echo ""
	@echo "Development:"
	@echo "  make dev          - Start all services with docker-compose"
	@echo "  make run-api      - Run API server locally"
	@echo "  make run-worker   - Run Temporal worker locally"
	@echo "  make run-frontend - Run Next.js frontend locally"
	@echo "  make dev-full     - Start all services locally"
	@echo ""
	@echo "Maintenance:"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make migrate      - Run database migrations"
	@echo "  make seed         - Seed database with admin user"

# Go variables
GO := go
GOFLAGS := -v
BUILD_FLAGS := -ldflags="-s -w"

# Build targets
build: build-api build-worker build-seed

build-api:
	$(GO) build $(BUILD_FLAGS) -o bin/api ./services/api/cmd/api

build-worker:
	$(GO) build $(BUILD_FLAGS) -o bin/worker ./services/worker/cmd/worker

build-seed:
	$(GO) build $(BUILD_FLAGS) -o bin/seed ./cmd/seed
	$(GO) build $(BUILD_FLAGS) -o bin/seed-patterns ./cmd/seed-patterns

# Test targets
test:
	$(GO) test -race -coverprofile=coverage.out -covermode=atomic ./...

test-e2e:
	cd apps/web && pnpm test:e2e

coverage:
	$(GO) test -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Code quality
lint:
	golangci-lint run --timeout=5m

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# Development
dev:
	docker compose up -d postgres temporal minio
	@echo "Waiting for services..."
	@sleep 5
	@echo "Services ready! Run 'make run-api', 'make run-worker', 'make run-frontend' in separate terminals"

dev-full:
	@echo "Starting all services..."
	@$(MAKE) run-api & \
	$(MAKE) run-worker & \
	$(MAKE) run-frontend

run-api:
	POSTGRES_DSN="postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable" \
	TEMPORAL_HOST_PORT="temporal:7233" \
	TEMPORAL_NAMESPACE="default" \
	OBJECT_STORAGE_ENDPOINT="http://minio:9000" \
	OBJECT_STORAGE_BUCKET="appgent-runs" \
	OBJECT_STORAGE_ACCESS_KEY="minioadmin" \
	OBJECT_STORAGE_SECRET_KEY="minioadmin" \
	JWT_SIGNING_SECRET="change-me-in-prod" \
	SESSION_COOKIE_NAME="session" \
	SESSION_TTL_HOURS="168" \
	CORS_ALLOWED_ORIGIN="http://localhost:3000" \
	ADMIN_SEED_EMAIL="admin" \
	ADMIN_SEED_PASSWORD="admin" \
	OPENROUTER_API_KEY="sk-or-test" \
	OPENROUTER_BASE_URL="https://openrouter.ai/api/v1" \
	OPENROUTER_MODEL_PLAN="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_DESIGN="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_CODE="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_QA="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_EMBEDDING="nvidia/nemotron-3-ultra-550b-a55b:free" \
	MAX_QA_RETRIES="3" \
	WORKER_MAX_CONCURRENT_ACTIVITIES="10" \
	LOG_LEVEL="info" \
	$(GO) run ./services/api/cmd/api

run-worker:
	POSTGRES_DSN="postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable" \
	TEMPORAL_HOST_PORT="temporal:7233" \
	TEMPORAL_NAMESPACE="default" \
	OBJECT_STORAGE_ENDPOINT="http://minio:9000" \
	OBJECT_STORAGE_BUCKET="appgent-runs" \
	OBJECT_STORAGE_ACCESS_KEY="minioadmin" \
	OBJECT_STORAGE_SECRET_KEY="minioadmin" \
	OPENROUTER_API_KEY="sk-or-test" \
	OPENROUTER_BASE_URL="https://openrouter.ai/api/v1" \
	OPENROUTER_MODEL_PLAN="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_DESIGN="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_CODE="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_QA="nvidia/nemotron-3-ultra-550b-a55b:free" \
	OPENROUTER_MODEL_EMBEDDING="nvidia/nemotron-3-ultra-550b-a55b:free" \
	MAX_QA_RETRIES="3" \
	WORKER_MAX_CONCURRENT_ACTIVITIES="10" \
	LOG_LEVEL="info" \
	$(GO) run ./services/worker/cmd/worker

run-frontend:
	cd apps/web && pnpm dev

migrate:
	@echo "Running migrations..."
	$(GO) run ./cmd/migrate

seed:
	POSTGRES_DSN="postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable" \
	ADMIN_SEED_EMAIL="admin" \
	ADMIN_SEED_PASSWORD="admin" \
	$(GO) run ./cmd/seed

seed-patterns:
	POSTGRES_DSN="postgres://appgent:appgent@localhost:5433/appgent?sslmode=disable" \
	OPENROUTER_API_KEY="sk-or-test" \
	OPENROUTER_BASE_URL="https://openrouter.ai/api/v1" \
	OPENROUTER_MODEL_EMBEDDING="nvidia/nemotron-3-ultra-550b-a55b:free" \
	$(GO) run ./cmd/seed-patterns

# Cleanup
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	rm -rf apps/web/.next
	rm -rf apps/web/node_modules
	rm -rf apps/web/playwright-report
	rm -rf apps/web/test-results

# Docker
docker-build:
	docker build -t appgent-api:latest -f services/api/Dockerfile .
	docker build -t appgent-worker:latest -f services/worker/Dockerfile .
	docker build -t appgent-web:latest -f apps/web/Dockerfile .

docker-up:
	docker compose up -d

docker-down:
	docker compose down -v

# Release
release-dry-run:
	@echo "Version: $$(date +'%Y.%m.%d')-$$(git rev-parse --short HEAD)"

.PHONY: help build test lint fmt vet coverage run-api run-worker run-frontend dev dev-full clean migrate seed seed-patterns docker-build docker-up docker-down release-dry-run