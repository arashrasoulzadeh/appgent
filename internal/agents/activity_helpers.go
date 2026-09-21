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
	if model := os.Getenv(envKey); model != "" {
		return model
	}
	// Falls back to the active provider's own resolved model (e.g.
	// GAPGPT_MODEL, OLLAMA_MODEL) rather than the bare AI_MODEL var — a
	// stale AI_MODEL left over from a previous AI_PROVIDER must not
	// override the current provider's model just because AI_MODEL_<AGENT>
	// wasn't set for this agent.
	return ai.ProviderConfigFromEnv().Model
}

// modelOrDefault returns model if non-empty, otherwise a sensible default
// model for the given provider (never the provider's own name, which is
// not a valid model identifier).
func modelOrDefault(model string, provider ai.Provider) string {
	if model != "" {
		return model
	}
	return ai.GetDefaultModelForProvider(provider.Name())
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