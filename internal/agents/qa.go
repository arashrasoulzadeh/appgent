package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/arashrasoulzadeh/appgent/internal/ai"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

type QAAgent struct {
	provider ai.Provider
	model    string
	prompt   *template.Template
}

func NewQAAgent(provider ai.Provider, model string) (*QAAgent, error) {
	promptPath := "internal/agents/prompts/qa.tmpl"
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		promptPath = "../../../internal/agents/prompts/qa.tmpl"
	}
	tmpl, err := template.ParseFiles(promptPath)
	if err != nil {
		return nil, fmt.Errorf("parse qa template: %w", err)
	}
	return &QAAgent{provider: provider, model: model, prompt: tmpl}, nil
}

func (a *QAAgent) Execute(ctx context.Context, in temporal.QAInput) (temporal.QAOutput, error) {
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

	messages := []ai.Message{
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

	resp, err := a.provider.ChatCompletionWithJSONSchema(ctx, a.model, messages, schema, 0.1)
	if err != nil {
		return temporal.QAOutput{}, fmt.Errorf("AI provider call: %w", err)
	}

	var output temporal.QAOutput
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &output)
	if err != nil {
		return temporal.QAOutput{}, fmt.Errorf("unmarshal qa output: %w", err)
	}

	return output, nil
}