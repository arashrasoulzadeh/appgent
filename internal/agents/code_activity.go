package agents

import (
	"context"

	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

func CodeActivity(ctx context.Context, in temporal.CodeInput) (temporal.CodeOutput, error) {
	provider, err := getAIProvider()
	if err != nil {
		return temporal.CodeOutput{}, err
	}

	model := modelOrDefault(getModelForAgent("code"), provider)
	stepID := startStep(ctx, in.RunID, "code", in.Attempt, model, in)

	ragSvc := getRAGService(ctx)
	agent, err := NewCodeAgent(provider, model, ragSvc)
	if err != nil {
		finishStep(ctx, stepID, nil, err)
		return temporal.CodeOutput{}, err
	}

	out, err := agent.Execute(ctx, in)
	finishStep(ctx, stepID, out, err)
	return out, err
}
