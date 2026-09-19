package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	ctx := context.Background()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("POSTGRES_DSN not set")
	}

	email := os.Getenv("ADMIN_SEED_EMAIL")
	if email == "" {
		email = "admin"
	}

	password := os.Getenv("ADMIN_SEED_PASSWORD")
	if password == "" {
		password = "admin"
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect to db: %v", err)
	}
	defer pool.Close()

	if err := seedUser(ctx, pool, email, password); err != nil {
		log.Fatalf("seed user: %v", err)
	}

	fmt.Printf("Seeded admin user: %s / %s\n", email, password)
}

func seedUser(ctx context.Context, pool *pgxpool.Pool, email, password string) error {
	var exists bool
	err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", email).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check user: %w", err)
	}
	if exists {
		fmt.Printf("User %s already exists, skipping seed\n", email)
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = pool.Exec(ctx, "INSERT INTO users (email, password_hash) VALUES ($1, $2)", email, string(hash))
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}

	return nil
}