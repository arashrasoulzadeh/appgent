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

	ragSvc := getRAGService(ctx)
	agent, err := NewDesignAgent(provider, model, ragSvc)
	if err != nil {
		return temporal.DesignOutput{}, err
	}

	return agent.Execute(ctx, in)
}