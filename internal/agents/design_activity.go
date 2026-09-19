package agents

import (
	"context"
	"os"

	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

func DesignActivity(ctx context.Context, in temporal.DesignInput) (temporal.DesignOutput, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL_DESIGN")
	if model == "" {
		model = "nvidia/nemotron-3-ultra:free"
	}

	client := openrouter.NewClient(apiKey, os.Getenv("OPENROUTER_BASE_URL"))
	agent, err := NewDesignAgent(client, model)
	if err != nil {
		return temporal.DesignOutput{}, err
	}

	return agent.Execute(ctx, in)
}