package agents

import (
	"context"
	"os"

	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

func PlanActivity(ctx context.Context, in temporal.PlanInput) (temporal.PlanOutput, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL_PLAN")
	if model == "" {
		model = "nvidia/nemotron-3-ultra:free"
	}

	client := openrouter.NewClient(apiKey, os.Getenv("OPENROUTER_BASE_URL"))
	agent, err := NewPlanAgent(client, model)
	if err != nil {
		return temporal.PlanOutput{}, err
	}

	return agent.Execute(ctx, in)
}