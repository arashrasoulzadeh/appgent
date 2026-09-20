package agents

import (
	"context"

	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

func PlanActivity(ctx context.Context, in temporal.PlanInput) (temporal.PlanOutput, error) {
	provider, err := getAIProvider()
	if err != nil {
		return temporal.PlanOutput{}, err
	}

	model := getModelForAgent("plan")
	if model == "" {
		model = provider.Name()
	}

	ragSvc := getRAGService(ctx)
	agent, err := NewPlanAgent(provider, model, ragSvc)
	if err != nil {
		return temporal.PlanOutput{}, err
	}

	return agent.Execute(ctx, in)
}