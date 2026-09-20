package agents

import (
	"context"
	"os"

	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

func CodeActivity(ctx context.Context, in temporal.CodeInput) (temporal.CodeOutput, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL_CODE")
	if model == "" {
		model = "nvidia/nemotron-3-ultra:free"
	}

	client := openrouter.NewClient(apiKey, os.Getenv("OPENROUTER_BASE_URL"))
	ragSvc := getRAGService(ctx)
	agent, err := NewCodeAgent(client, model, ragSvc)
	if err != nil {
		return temporal.CodeOutput{}, err
	}

	return agent.Execute(ctx, in)
}