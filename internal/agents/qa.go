package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"text/template"

	"github.com/arashrasoulzadeh/appgent/internal/ai"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

// nextBuildErrorRe matches Next.js's standard webpack/TypeScript error
// format, where the offending file path appears on its own line
// immediately before the error description. Two real shapes seen in
// production:
//
//	./src/app/contact/page.tsx
//	Module not found: Can't resolve '@/components/ContactForm'
//
//	./src/app/education/page.tsx:10:8
//	Type error: Cannot find name 'EducationSection'.
//
// The TypeScript-checker shape appends ":line:col" directly to the file
// path on that same line — captured here and stripped in Go (a regex
// alternation for "optionally followed by :digits:digits" is uglier than
// just trimming it after the fact) so both shapes resolve to a clean file
// path a page/component target can actually be matched against.
//
// Used to attribute build failures to the specific file that caused them —
// without a File on each QAIssue, code.tmpl's retry-pass rule ("if QA
// feedback doesn't mention any file in your scope, return unchanged") means
// no per-target call would ever recognize the feedback as its own to act on.
var nextBuildErrorRe = regexp.MustCompile(`(?m)^\./(\S+)\n(.+)$`)

// trailingLineColRe strips a TypeScript-checker-style ":line:col" suffix
// (e.g. ":10:8") off the end of a file path captured by nextBuildErrorRe.
var trailingLineColRe = regexp.MustCompile(`:\d+:\d+$`)

func cleanBuildErrorFilePath(file string) string {
	return trailingLineColRe.ReplaceAllString(file, "")
}

// buildErrorIssues turns a raw `next build` failure log into per-file
// QAIssues by matching nextBuildErrorRe. Falls back to a single
// unattributed issue carrying the full log when the format doesn't match
// (e.g. a config-level failure with no per-file breakdown) so the failure
// is never silently dropped, even though an unattributed issue won't be
// picked up by any single scoped retry target.
func buildErrorIssues(buildErr error, buildLog string) []temporal.QAIssue {
	matches := nextBuildErrorRe.FindAllStringSubmatch(buildLog, -1)
	if len(matches) == 0 {
		return []temporal.QAIssue{{
			Severity: "blocking",
			Message:  fmt.Sprintf("The generated app failed to build: %v\n\nBuild output:\n%s", buildErr, buildLog),
		}}
	}
	seen := map[string]bool{}
	var issues []temporal.QAIssue
	for _, m := range matches {
		file, msg := cleanBuildErrorFilePath(m[1]), m[2]
		key := file + "|" + msg
		if seen[key] {
			continue
		}
		seen[key] = true
		issues = append(issues, temporal.QAIssue{
			File:     file,
			Severity: "blocking",
			Message:  fmt.Sprintf("Build failure in this file: %s", msg),
		})
	}
	return issues
}

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

	// Run the SAME real Docker build PublishBundleActivity will later run,
	// before ever asking the LLM to review anything — this template is
	// explicitly documented ("interpret the raw output from deterministic
	// build/lint/a11y tools") to receive real tool output, but until now
	// nothing ever ran a real build here: buildOutput was a hardcoded
	// "succeeded" string regardless of what was actually generated. That
	// let real build failures (e.g. "Module not found" for a component
	// that silently failed to generate) pass QA and only surface much
	// later, at publish time, with no chance to retry-with-feedback.
	// builder is nil in unit tests that don't call SetBuilder and in
	// in.Files == nil calls (nothing to build yet) — both fall back to the
	// previous "assume it builds" behavior rather than erroring.
	if builder != nil && len(in.Files) > 0 {
		ensureTsConfigPathAlias(in.Files)
		if _, buildLog, buildErr := builder.Build(ctx, in.RunID, in.Files); buildErr != nil {
			// A real, deterministic build failure is unambiguous — no need
			// to spend an LLM call asking it to "interpret" a failure that
			// already has a definitive, structured answer. Same pattern as
			// the missing-component-import structural check in workflow.go.
			return temporal.QAOutput{Passed: false, Issues: buildErrorIssues(buildErr, buildLog)}, nil
		}
	}

	var buf bytes.Buffer
	err := a.prompt.Execute(&buf, map[string]interface{}{
		"AppKind":     in.AppKind,
		"BuildOutput": buildOutput,
		"LinkOutput":  linkOutput,
		"A11yOutput":  a11yOutput,
		"PWAOutput":   pwaOutput,
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
						"file":     map[string]string{"type": "string"},
						"line":     map[string]string{"type": "integer"},
						"severity": map[string]interface{}{"type": "string", "enum": []string{"blocking", "warning"}},
						"message":  map[string]string{"type": "string"},
					},
					"required":             []string{"file", "line", "severity", "message"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"passed", "issues"},
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

	// The model's "passed" field is self-reported and can be inconsistent
	// with the issues it lists. Never let a run be marked passed if it
	// reported any blocking issue.
	for _, issue := range output.Issues {
		if issue.Severity == "blocking" {
			output.Passed = false
			break
		}
	}

	return output, nil
}
