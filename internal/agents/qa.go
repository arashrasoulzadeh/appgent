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

type QAAgent struct {
	client  *openrouter.Client
	model   string
	prompt  *template.Template
}

func NewQAAgent(client *openrouter.Client, model string) (*QAAgent, error) {
	promptPath := "internal/agents/prompts/qa.tmpl"
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		promptPath = "../../../internal/agents/prompts/qa.tmpl"
	}
	tmpl, err := template.ParseFiles(promptPath)
	if err != nil {
		return nil, fmt.Errorf("parse qa template: %w", err)
	}
	return &QAAgent{client: client, model: model, prompt: tmpl}, nil
}

func (a *QAAgent) Execute(ctx context.Context, in temporal.QAInput) (temporal.QAOutput, error) {
	// TODO: In real implementation, run deterministic checks here:
	// - npm install && next build (or next lint)
	// - broken internal link check
	// - a11y check (axe-core)
	// - PWA manifest/SW validation
	// For now, return mock results

	// Mock build output for prompt
	buildOutput := "Build succeeded with 0 errors"
	linkOutput := "All internal links valid"
	a11yOutput := "No a11y violations found"
	pwaOutput := "Manifest and SW valid"

	var buf bytes.Buffer
	err := a.prompt.Execute(&buf, map[string]interface{}{
		"AppKind":      in.AppKind,
		"BuildOutput":  buildOutput,
		"LinkOutput":   linkOutput,
		"A11yOutput":   a11yOutput,
		"PWAOutput":    pwaOutput,
	})
	if err != nil {
		return temporal.QAOutput{}, fmt.Errorf("execute template: %w", err)
	}

	messages := []openrouter.Message{
		{Role: "system", Content: "You are a QA engineer. Output ONLY valid JSON."},
		{Role: "user", Content: buf.String()},
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"passed": map[string]string{"type": "boolean"},
			"issues": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file":      map[string]string{"type": "string"},
						"line":      map[string]string{"type": "integer"},
						"severity":  map[string]interface{}{"type": "string", "enum": []string{"blocking", "warning"}},
						"message":   map[string]string{"type": "string"},
					},
					"required": []string{"file", "line", "severity", "message"},
					"additionalProperties": false,
				},
			},
		},
		"required": []string{"passed", "issues"},
		"additionalProperties": false,
	}

	resp, err := a.client.ChatCompletionWithJSONSchema(ctx, a.model, messages, schema, 0.1)
	if err != nil {
		return temporal.QAOutput{}, fmt.Errorf("openrouter call: %w", err)
	}

	var output temporal.QAOutput
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &output)
	if err != nil {
		return temporal.QAOutput{}, fmt.Errorf("unmarshal qa output: %w", err)
	}

	return output, nil
}

func QAActivity(ctx context.Context, in temporal.QAInput) (temporal.QAOutput, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("OPENROUTER_MODEL_QA")
	if model == "" {
		model = "nvidia/nemotron-3-ultra:free"
	}

	client := openrouter.NewClient(apiKey, os.Getenv("OPENROUTER_BASE_URL"))
	agent, err := NewQAAgent(client, model)
	if err != nil {
		return temporal.QAOutput{}, err
	}

	return agent.Execute(ctx, in)
}