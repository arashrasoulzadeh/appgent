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

	model := getModelForAgent("code")
	if model == "" {
		model = provider.Name()
	}

	ragSvc := getRAGService(ctx)
	agent, err := NewCodeAgent(provider, model, ragSvc)
	if err != nil {
		return temporal.CodeOutput{}, err
	}

	return agent.Execute(ctx, in)
}