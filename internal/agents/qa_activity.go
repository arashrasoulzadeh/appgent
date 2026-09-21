package agents

import (
	"context"

	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

func QAActivity(ctx context.Context, in temporal.QAInput) (temporal.QAOutput, error) {
	provider, err := getAIProvider()
	if err != nil {
		return temporal.QAOutput{}, err
	}

	model := modelOrDefault(getModelForAgent("qa"), provider)
	stepID := startStep(ctx, in.RunID, "qa", in.Attempt, model, in)

	agent, err := NewQAAgent(provider, model)
	if err != nil {
		finishStep(ctx, stepID, nil, err)
		return temporal.QAOutput{}, err
	}

	out, err := agent.Execute(ctx, in)
	finishStep(ctx, stepID, out, err)
	return out, err
}
