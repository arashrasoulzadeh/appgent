package main

import (
	"context"
	"log"
	"os"

	"github.com/arashrasoulzadeh/appgent/internal/embedding"
	"github.com/arashrasoulzadeh/appgent/internal/rag"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Fatal("POSTGRES_DSN not set")
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENROUTER_API_KEY not set")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect to db: %v", err)
	}
	defer pool.Close()

	embedClient := embedding.NewClient(apiKey, os.Getenv("OPENROUTER_BASE_URL"), os.Getenv("OPENROUTER_MODEL_EMBEDDING"))

	log.Println("Seeding initial design patterns...")
	if err := rag.SeedInitialPatterns(ctx, pool, embedClient); err != nil {
		log.Fatalf("seed patterns: %v", err)
	}

	log.Println("Design patterns seeded successfully")
}