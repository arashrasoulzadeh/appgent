package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
	"github.com/arashrasoulzadeh/appgent/internal/rag"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

type DesignAgent struct {
	client   *openrouter.Client
	model    string
	prompt   *template.Template
	ragSvc   *rag.Service
}

func NewDesignAgent(client *openrouter.Client, model string, ragSvc *rag.Service) (*DesignAgent, error) {
	promptPath := "internal/agents/prompts/design.tmpl"
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		promptPath = "../../../internal/agents/prompts/design.tmpl"
	}
	tmpl, err := template.ParseFiles(promptPath)
	if err != nil {
		return nil, fmt.Errorf("parse design template: %w", err)
	}
	return &DesignAgent{client: client, model: model, prompt: tmpl, ragSvc: ragSvc}, nil
}

func (a *DesignAgent) Execute(ctx context.Context, in temporal.DesignInput) (temporal.DesignOutput, error) {
	// Query RAG for similar design patterns if service is available
	var ragContext []temporal.DesignPattern
	if a.ragSvc != nil {
		queryText := fmt.Sprintf("Design for app with style: %s. Pages: %d, Components: %d", 
			in.Spec.StyleDirection, len(in.Spec.Pages), len(in.Spec.Components))
		patterns, err := a.ragSvc.QuerySimilar(ctx, queryText, 3)
		if err == nil {
			ragContext = patterns
		}
	}

	// Add RAG context to the input
	type DesignInputWithRAG struct {
		temporal.DesignInput
		RAGContext []temporal.DesignPattern
	}

	inputWithRAG := DesignInputWithRAG{
		DesignInput: in,
		RAGContext:  ragContext,
	}

	var buf bytes.Buffer
	err := a.prompt.Execute(&buf, inputWithRAG)
	if err != nil {
		return temporal.DesignOutput{}, fmt.Errorf("execute template: %w", err)
	}

	messages := []openrouter.Message{
		{Role: "system", Content: "You are an expert visual designer and design systems engineer. Output ONLY valid JSON."},
		{Role: "user", Content: buf.String()},
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tokens": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"colors": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"primary":   colorVariantSchema(),
							"secondary": colorVariantSchema(),
							"accent":    colorVariantSchema(),
							"neutral":   colorVariantSchema(),
							"success":   colorVariantSchema(),
							"warning":   colorVariantSchema(),
							"error":     colorVariantSchema(),
						},
						"required": []string{"primary", "secondary", "accent", "neutral", "success", "warning", "error"},
						"additionalProperties": false,
					},
					"spacing": map[string]interface{}{
						"type": "array",
						"items": map[string]string{"type": "string"},
					},
					"typography": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"fontFamily":    map[string]string{"type": "string"},
							"fontSizes":     fontSizesSchema(),
							"fontWeights":   fontWeightsSchema(),
							"lineHeights":   lineHeightsSchema(),
						},
						"required": []string{"fontFamily", "fontSizes", "fontWeights", "lineHeights"},
						"additionalProperties": false,
					},
					"borderRadius": map[string]interface{}{
						"type": "array",
						"items": map[string]string{"type": "string"},
					},
				},
				"required": []string{"colors", "spacing", "typography", "borderRadius"},
				"additionalProperties": false,
			},
			"copyTone":    map[string]string{"type": "string"},
			"layoutNotes": map[string]string{"type": "string"},
		},
		"required": []string{"tokens", "copyTone", "layoutNotes"},
		"additionalProperties": false,
	}

	resp, err := a.client.ChatCompletionWithJSONSchema(ctx, a.model, messages, schema, 0.3)
	if err != nil {
		return temporal.DesignOutput{}, fmt.Errorf("openrouter call: %w", err)
	}

	var output temporal.DesignOutput
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &output)
	if err != nil {
		return temporal.DesignOutput{}, fmt.Errorf("unmarshal design output: %w", err)
	}

	return output, nil
}

func colorVariantSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"light":    map[string]string{"type": "string"},
			"main":     map[string]string{"type": "string"},
			"dark":     map[string]string{"type": "string"},
			"contrast": map[string]string{"type": "string"},
		},
		"required": []string{"light", "main", "dark", "contrast"},
		"additionalProperties": false,
	}
}

func fontSizesSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"xs": map[string]string{"type": "string"},
			"sm": map[string]string{"type": "string"},
			"base": map[string]string{"type": "string"},
			"lg": map[string]string{"type": "string"},
			"xl": map[string]string{"type": "string"},
			"2xl": map[string]string{"type": "string"},
			"3xl": map[string]string{"type": "string"},
			"4xl": map[string]string{"type": "string"},
		},
		"required": []string{"xs", "sm", "base", "lg", "xl", "2xl", "3xl", "4xl"},
		"additionalProperties": false,
	}
}

func fontWeightsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"normal": map[string]string{"type": "integer"},
			"medium": map[string]string{"type": "integer"},
			"semibold": map[string]string{"type": "integer"},
			"bold": map[string]string{"type": "integer"},
		},
		"required": []string{"normal", "medium", "semibold", "bold"},
		"additionalProperties": false,
	}
}

func lineHeightsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"tight": map[string]string{"type": "string"},
			"normal": map[string]string{"type": "string"},
			"relaxed": map[string]string{"type": "string"},
		},
		"required": []string{"tight", "normal", "relaxed"},
		"additionalProperties": false,
	}
}