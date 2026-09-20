#!/bin/bash
set -e

echo "Starting E2E test environment..."

# Start test infrastructure
docker compose -f docker-compose.test.yml up -d

echo "Waiting for services to be ready..."
sleep 10

# Wait for PostgreSQL
until docker compose -f docker-compose.test.yml exec postgres pg_isready -U appgent; do
  echo "Waiting for PostgreSQL..."
  sleep 2
done

# Wait for Temporal
until docker compose -f docker-compose.test.yml exec temporal tctl workflow list >/dev/null 2>&1; do
  echo "Waiting for Temporal..."
  sleep 2
done

# Wait for MinIO
until curl -f http://localhost:9000/minio/health/live >/dev/null 2>&1; do
  echo "Waiting for MinIO..."
  sleep 2
done

echo "Infrastructure ready!"

# Run migrations
export POSTGRES_DSN="postgres://appgent:appgent@localhost:5432/appgent?sslmode=disable"
go run ./cmd/seed

# Build and start API server
go build -o api ./services/api/cmd/api
./api &
API_PID=$!

# Wait for API to be ready
sleep 5
until curl -f http://localhost:8080/healthz >/dev/null 2>&1; do
  echo "Waiting for API server..."
  sleep 2
done

# Build and start worker
go build -o worker ./services/worker/cmd/worker
./worker &
WORKER_PID=$!

# Build frontend
cd apps/web
pnpm install --frozen-lockfile
pnpm build
cd ../..

# Start frontend dev server
cd apps/web
pnpm dev &
FRONTEND_PID=$!

# Wait for frontend to be ready
sleep 10
until curl -f http://localhost:3000/login >/dev/null 2>&1; do
  echo "Waiting for frontend..."
  sleep 2
done

echo "All services ready! Running E2E tests..."

# Run E2E tests
pnpm test:e2e

# Cleanup
kill $FRONTEND_PID $WORKER_PID $API_PID 2>/dev/null || true
docker compose -f docker-compose.test.yml down

echo "E2E tests completed!"