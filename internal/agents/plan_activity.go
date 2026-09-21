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

	model := modelOrDefault(getModelForAgent("plan"), provider)
	stepID := startStep(ctx, in.RunID, "plan", 1, model, in)

	ragSvc := getRAGService(ctx)
	agent, err := NewPlanAgent(provider, model, ragSvc)
	if err != nil {
		finishStep(ctx, stepID, nil, err)
		return temporal.PlanOutput{}, err
	}

	out, err := agent.Execute(ctx, in)
	finishStep(ctx, stepID, out, err)
	return out, err
}
