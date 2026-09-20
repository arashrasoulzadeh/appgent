package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
	"github.com/arashrasoulzadeh/appgent/internal/rag"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

type CodeAgent struct {
	client   *openrouter.Client
	model    string
	prompt   *template.Template
	ragSvc   *rag.Service
}

func NewCodeAgent(client *openrouter.Client, model string, ragSvc *rag.Service) (*CodeAgent, error) {
	promptPath := "internal/agents/prompts/code.tmpl"
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		promptPath = "../../../internal/agents/prompts/code.tmpl"
	}
	funcMap := template.FuncMap{
		"hasSuffix": strings.HasSuffix,
	}
	tmpl, err := template.New("code.tmpl").Funcs(funcMap).ParseFiles(promptPath)
	if err != nil {
		return nil, fmt.Errorf("parse code template: %w", err)
	}
	return &CodeAgent{client: client, model: model, prompt: tmpl, ragSvc: ragSvc}, nil
}

func (a *CodeAgent) Execute(ctx context.Context, in temporal.CodeInput) (temporal.CodeOutput, error) {
	// Query RAG for component patterns if service is available
	var ragContext []temporal.DesignPattern
	if a.ragSvc != nil && in.PriorFiles == nil { // Only on first pass
		queryText := fmt.Sprintf("Components for: %s. Pages: %d", in.Spec.StyleDirection, len(in.Spec.Pages))
		patterns, err := a.ragSvc.QuerySimilar(ctx, queryText, 3)
		if err == nil {
			ragContext = patterns
		}
	}

	// Add RAG context to input
	type CodeInputWithRAG struct {
		temporal.CodeInput
		RAGContext []temporal.DesignPattern
	}

	inputWithRAG := CodeInputWithRAG{
		CodeInput:  in,
		RAGContext: ragContext,
	}

	var buf bytes.Buffer
	err := a.prompt.Execute(&buf, inputWithRAG)
	if err != nil {
		return temporal.CodeOutput{}, fmt.Errorf("execute template: %w", err)
	}

	messages := []openrouter.Message{
		{Role: "system", Content: "You are an expert Next.js, TypeScript, and Tailwind CSS developer. Output ONLY valid JSON mapping file paths to contents."},
		{Role: "user", Content: buf.String()},
	}

	resp, err := a.client.ChatCompletion(ctx, openrouter.ChatCompletionRequest{
		Model:       a.model,
		Messages:    messages,
		Temperature: 0.2,
		MaxTokens:   8192,
	})
	if err != nil {
		return temporal.CodeOutput{}, fmt.Errorf("openrouter call: %w", err)
	}

	var files map[string]string
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &files)
	if err != nil {
		return temporal.CodeOutput{}, fmt.Errorf("unmarshal code output: %w", err)
	}

	totalSize := 0
	for _, content := range files {
		totalSize += len(content)
	}
	if totalSize > 2*1024*1024 {
		return temporal.CodeOutput{}, fmt.Errorf("generated bundle too large: %d bytes (max 2MB)", totalSize)
	}

	return temporal.CodeOutput{Files: files}, nil
}