package agents

import (
	"context"

	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

func DesignActivity(ctx context.Context, in temporal.DesignInput) (temporal.DesignOutput, error) {
	provider, err := getAIProvider()
	if err != nil {
		return temporal.DesignOutput{}, err
	}

	model := modelOrDefault(getModelForAgent("design"), provider)
	stepID := startStep(ctx, in.RunID, "design", 1, model, in)

	ragSvc := getRAGService(ctx)
	agent, err := NewDesignAgent(provider, model, ragSvc)
	if err != nil {
		finishStep(ctx, stepID, nil, err)
		return temporal.DesignOutput{}, err
	}

	out, err := agent.Execute(ctx, in)
	finishStep(ctx, stepID, out, err)
	return out, err
}
