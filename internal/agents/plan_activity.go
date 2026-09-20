package agents

import (
	"context"
	"os"

	"github.com/arashrasoulzadeh/appgent/internal/embedding"
	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
	"github.com/arashrasoulzadeh/appgent/internal/rag"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getRAGService(ctx context.Context) *rag.Service {
	dbURL := os.Getenv("POSTGRES_DSN")
	if dbURL == "" {
		return nil
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	embedClient := embedding.NewClient(apiKey, os.Getenv("OPENROUTER_BASE_URL"), os.Getenv("OPENROUTER_MODEL_EMBEDDING"))

	return rag.NewService(pool, embedClient)
}

func PlanActivity(ctx context.Context, in temporal.PlanInput) (temporal.PlanOutput, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL_PLAN")
	if model == "" {
		model = "nvidia/nemotron-3-ultra:free"
	}

	client := openrouter.NewClient(apiKey, os.Getenv("OPENROUTER_BASE_URL"))
	ragSvc := getRAGService(ctx)
	agent, err := NewPlanAgent(client, model, ragSvc)
	if err != nil {
		return temporal.PlanOutput{}, err
	}

	return agent.Execute(ctx, in)
}