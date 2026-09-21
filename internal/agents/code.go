package agents

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/arashrasoulzadeh/appgent/internal/ai"
	"github.com/arashrasoulzadeh/appgent/internal/rag"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

// fileBlockRe matches the delimited file-block output format described in
// prompts/code.tmpl:
//
//	>>>FILE: path/to/file.tsx
//	...raw content, unescaped...
//	>>>ENDFILE
//
// Deliberately NOT JSON: asking a model to JSON-encode multi-line source
// code as a string value is a well-known reliability trap — any raw
// newline, quote, or backslash the model forgets to escape produces
// invalid JSON, and free/weaker models get this wrong constantly on
// anything beyond trivial files (seen in production: "invalid character
// '\n' in string literal", "invalid character ”' in string escape
// code"). A delimited plain-text format needs no escaping at all, so
// there's nothing for the model to get wrong here.
var fileBlockRe = regexp.MustCompile(`(?s)>>>FILE:\s*(\S+)\s*\n(.*?)\n>>>ENDFILE`)

func parseFileBlocks(content string) (map[string]string, error) {
	matches := fileBlockRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no >>>FILE blocks found in response")
	}
	files := make(map[string]string, len(matches))
	for _, m := range matches {
		files[strings.TrimSpace(m[1])] = m[2]
	}
	return files, nil
}

type CodeAgent struct {
	provider ai.Provider
	model    string
	prompt   *template.Template
	ragSvc   *rag.Service
}

func NewCodeAgent(provider ai.Provider, model string, ragSvc *rag.Service) (*CodeAgent, error) {
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
	return &CodeAgent{provider: provider, model: model, prompt: tmpl, ragSvc: ragSvc}, nil
}

func (a *CodeAgent) Execute(ctx context.Context, in temporal.CodeInput) (temporal.CodeOutput, error) {
	var ragContext []temporal.DesignPattern
	if a.ragSvc != nil && in.PriorFiles == nil {
		queryText := fmt.Sprintf("Components for: %s. Pages: %d", in.Spec.StyleDirection, len(in.Spec.Pages))
		patterns, err := a.ragSvc.QuerySimilar(ctx, queryText, 3)
		if err == nil {
			ragContext = patterns
		}
	}

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

	messages := []ai.Message{
		{Role: "system", Content: "You are an expert Next.js, TypeScript, and Tailwind CSS developer. Output ONLY the delimited file-block format described in the prompt — no JSON, no markdown code fences around the whole response, no commentary."},
		{Role: "user", Content: buf.String()},
	}

	resp, err := a.provider.ChatCompletion(ctx, ai.ChatCompletionRequest{
		Model:       a.model,
		Messages:    messages,
		Temperature: 0.2,
		MaxTokens:   8192,
	})
	if err != nil {
		return temporal.CodeOutput{}, fmt.Errorf("AI provider call: %w", err)
	}

	files, err := parseFileBlocks(resp.Choices[0].Message.Content)
	if err != nil {
		return temporal.CodeOutput{}, fmt.Errorf("parse code output: %w", err)
	}

	totalSize := 0
	for _, content := range files {
		totalSize += len(content)
	}
	// 500KB, not 2MB: each call now generates one page or the shared/root
	// files, not the whole app, since code generation fans out in parallel
	// (see GenerateAppWorkflow's generateCode helper).
	if totalSize > 500*1024 {
		return temporal.CodeOutput{}, fmt.Errorf("generated file(s) too large: %d bytes (max 500KB per call)", totalSize)
	}

	return temporal.CodeOutput{Files: files}, nil
}
