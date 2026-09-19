package main

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func TestSeed(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// Clean up before test
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email = 'testadmin'")

	// Set env vars for seed
	os.Setenv("POSTGRES_DSN", dsn)
	os.Setenv("ADMIN_SEED_EMAIL", "testadmin")
	os.Setenv("ADMIN_SEED_PASSWORD", "testpass")

	// Run seed logic
	err = seedUser(ctx, pool, "testadmin", "testpass")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Verify user exists with correct hash
	var hash string
	err = pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE email = $1", "testadmin").Scan(&hash)
	if err != nil {
		t.Fatalf("query user: %v", err)
	}

	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte("testpass"))
	if err != nil {
		t.Fatalf("password mismatch: %v", err)
	}

	// Test idempotency - second run should not error
	err = seedUser(ctx, pool, "testadmin", "testpass")
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
}