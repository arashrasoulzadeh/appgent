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

	ragSvc := getRAGService(ctx)
	agent, err := NewCodeAgent(provider, model, ragSvc)
	if err != nil {
		return temporal.CodeOutput{}, err
	}

	return agent.Execute(ctx, in)
}