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

	model := getModelForAgent("qa")
	if model == "" {
		model = provider.Name()
	}

	agent, err := NewQAAgent(provider, model)
	if err != nil {
		return temporal.QAOutput{}, err
	}

	return agent.Execute(ctx, in)
}