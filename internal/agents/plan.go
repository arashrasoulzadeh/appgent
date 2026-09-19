package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

type PlanAgent struct {
	client  *openrouter.Client
	model   string
	prompt  *template.Template
}

func NewPlanAgent(client *openrouter.Client, model string) (*PlanAgent, error) {
	promptPath := "internal/agents/prompts/plan.tmpl"
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		promptPath = "../../../internal/agents/prompts/plan.tmpl"
	}
	tmpl, err := template.ParseFiles(promptPath)
	if err != nil {
		return nil, fmt.Errorf("parse plan template: %w", err)
	}
	return &PlanAgent{client: client, model: model, prompt: tmpl}, nil
}

func (a *PlanAgent) Execute(ctx context.Context, in temporal.PlanInput) (temporal.PlanOutput, error) {
	var buf bytes.Buffer
	err := a.prompt.Execute(&buf, in)
	if err != nil {
		return temporal.PlanOutput{}, fmt.Errorf("execute template: %w", err)
	}

	messages := []openrouter.Message{
		{Role: "system", Content: "You are an expert product planner and software architect. Output ONLY valid JSON."},
		{Role: "user", Content: buf.String()},
	}

	// Define JSON schema for structured output
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"pages": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string"},
						"path":        map[string]string{"type": "string"},
						"description": map[string]string{"type": "string"},
					},
					"required": []string{"name", "path", "description"},
					"additionalProperties": false,
				},
			},
			"components": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":        map[string]string{"type": "string"},
						"type":        map[string]string{"type": "string"},
						"description": map[string]string{"type": "string"},
						"props":       map[string]string{"type": "object"},
					},
					"required": []string{"name", "type", "description"},
					"additionalProperties": false,
				},
			},
			"dataModel": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"name":   map[string]string{"type": "string"},
						"fields": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"name": map[string]string{"type": "string"},
									"type": map[string]string{"type": "string"},
								},
								"required": []string{"name", "type"},
								"additionalProperties": false,
							},
						},
					},
					"required": []string{"name", "fields"},
					"additionalProperties": false,
				},
			},
			"styleDirection": map[string]string{"type": "string"},
		},
		"required": []string{"pages", "components", "dataModel", "styleDirection"},
		"additionalProperties": false,
	}

	resp, err := a.client.ChatCompletionWithJSONSchema(ctx, a.model, messages, schema, 0.3)
	if err != nil {
		return temporal.PlanOutput{}, fmt.Errorf("openrouter call: %w", err)
	}

	var output temporal.PlanOutput
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &output)
	if err != nil {
		return temporal.PlanOutput{}, fmt.Errorf("unmarshal plan output: %w", err)
	}

	return output, nil
}