package agents

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/arashrasoulzadeh/appgent/internal/ai"
	"github.com/arashrasoulzadeh/appgent/internal/embedding"
	"github.com/arashrasoulzadeh/appgent/internal/rag"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getAIProvider() (ai.Provider, error) {
	return ai.NewProviderFromEnv()
}

func getModelForAgent(agentType string) string {
	envKey := fmt.Sprintf("AI_MODEL_%s", strings.ToUpper(agentType))
	model := os.Getenv(envKey)
	if model != "" {
		return model
	}
	return os.Getenv("AI_MODEL")
}

func getRAGService(ctx context.Context) *rag.Service {
	dbURL := os.Getenv("POSTGRES_DSN")
	if dbURL == "" {
		return nil
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil
	}

	embedProvider, err := ai.NewProviderFromEnv()
	if err != nil {
		return nil
	}

	embedClient := embedding.NewClientFromProvider(embedProvider, os.Getenv("AI_MODEL_EMBEDDING"))
	return rag.NewService(pool, embedClient)
}