package agents_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/arashrasoulzadeh/appgent/internal/agents"
	"github.com/arashrasoulzadeh/appgent/internal/ai"
	"github.com/arashrasoulzadeh/appgent/internal/temporal"
)

// TestMain chdirs into the repo root before running tests. The agent
// constructors (NewPlanAgent, NewDesignAgent, ...) locate their prompt
// templates via a path relative to the process's working directory, with a
// fallback of "../../../internal/agents/prompts/*.tmpl" tuned for test files
// that live three directories below the repo root (e.g.
// internal/agents/foo_test.go run from within a package at that depth).
// Since these black-box tests live in tests/agents/ instead, neither
// relative path resolves unless we run from the repo root, so we chdir here
// rather than duplicating the repo's prompt-loading logic in test code.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic("chdir to repo root: " + err.Error())
	}
	os.Exit(m.Run())
}

// These tests exercise the real agent implementations end-to-end, but
// replace the AI provider with an httptest.Server that plays the role of
// OpenRouter, so no live network calls or Docker are involved. Working
// directory for `go test ./tests/agents/...` is that package's directory, so
// prompt templates are located via the "../../internal/agents/prompts/*"
// relative fallback path the agents already implement for non-repo-root
// working directories; we run from repo root explicitly by chdir-ing via
// os.Chdir is avoided — instead we rely on the package's built-in relative
// fallback by running `go test ./...` from repo root, which the harness
// requires per project convention.

func newJSONServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id": "test-1",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": body,
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestPlanAgentExecute(t *testing.T) {
	planJSON := `{"pages":[{"name":"Home","path":"/","description":"landing"}],"components":[{"name":"Header","type":"layout","description":"top nav"}],"dataModel":[],"styleDirection":"clean"}`
	srv := newJSONServer(t, planJSON)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewPlanAgent(provider, "test-model", nil)
	if err != nil {
		t.Fatalf("NewPlanAgent: %v", err)
	}

	out, err := agent.Execute(context.Background(), temporal.PlanInput{
		AppKind:    "website",
		UserPrompt: "a portfolio site",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(out.Pages) != 1 || out.Pages[0].Name != "Home" {
		t.Errorf("unexpected plan output: %+v", out)
	}
	if out.StyleDirection != "clean" {
		t.Errorf("StyleDirection = %q, want clean", out.StyleDirection)
	}
}

func TestPlanAgentExecute_InvalidJSON(t *testing.T) {
	srv := newJSONServer(t, "not valid json")
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewPlanAgent(provider, "test-model", nil)
	if err != nil {
		t.Fatalf("NewPlanAgent: %v", err)
	}

	_, err = agent.Execute(context.Background(), temporal.PlanInput{AppKind: "website", UserPrompt: "x"})
	if err == nil {
		t.Fatal("expected error unmarshaling invalid JSON plan output")
	}
}

func TestDesignAgentExecute(t *testing.T) {
	designJSON := `{
		"tokens": {
			"colors": {
				"primary":{"light":"#eee","main":"#111","dark":"#000","contrast":"#fff"},
				"secondary":{"light":"#eee","main":"#111","dark":"#000","contrast":"#fff"},
				"accent":{"light":"#eee","main":"#111","dark":"#000","contrast":"#fff"},
				"neutral":{"light":"#eee","main":"#111","dark":"#000","contrast":"#fff"},
				"success":{"light":"#eee","main":"#111","dark":"#000","contrast":"#fff"},
				"warning":{"light":"#eee","main":"#111","dark":"#000","contrast":"#fff"},
				"error":{"light":"#eee","main":"#111","dark":"#000","contrast":"#fff"}
			},
			"spacing": ["0","4px"],
			"typography": {"fontFamily":"sans","fontSizes":{"xs":"1","sm":"1","base":"1","lg":"1","xl":"1","2xl":"1","3xl":"1","4xl":"1"},"fontWeights":{"normal":400,"medium":500,"semibold":600,"bold":700},"lineHeights":{"tight":"1","normal":"1","relaxed":"1"}},
			"borderRadius": ["0","4px"]
		},
		"copyTone": "friendly",
		"layoutNotes": "standard"
	}`
	srv := newJSONServer(t, designJSON)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewDesignAgent(provider, "test-model", nil)
	if err != nil {
		t.Fatalf("NewDesignAgent: %v", err)
	}

	out, err := agent.Execute(context.Background(), temporal.DesignInput{
		Spec: temporal.PlanOutput{StyleDirection: "clean, modern"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.CopyTone != "friendly" {
		t.Errorf("CopyTone = %q, want friendly", out.CopyTone)
	}
	if out.Tokens.Colors.Primary.Main != "#111" {
		t.Errorf("unexpected primary color: %+v", out.Tokens.Colors.Primary)
	}
}

func TestCodeAgentExecute(t *testing.T) {
	body := ">>>FILE: src/app/page.tsx\n" +
		"export default function Home(){return null}\n" +
		">>>ENDFILE"
	srv := newJSONServer(t, body)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewCodeAgent(provider, "test-model", nil)
	if err != nil {
		t.Fatalf("NewCodeAgent: %v", err)
	}

	out, err := agent.Execute(context.Background(), temporal.CodeInput{
		Spec:    temporal.PlanOutput{StyleDirection: "clean"},
		AppKind: "website",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, ok := out.Files["src/app/page.tsx"]; !ok {
		t.Errorf("expected generated file, got: %+v", out.Files)
	}
}

// TestCodeAgentExecute_UnescapedContent is the regression test for why the
// output format moved off JSON: real generated code is full of quotes,
// apostrophes, and newlines that a model constantly fails to JSON-escape
// correctly (seen in production: "invalid character '\n' in string
// literal", "invalid character ”' in string escape code"). The delimited
// format needs no escaping, so a response like this — which would have
// broken json.Unmarshal — must parse cleanly.
func TestCodeAgentExecute_UnescapedContent(t *testing.T) {
	body := `>>>FILE: src/app/page.tsx
export default function Home() {
  const msg = "she said "hi" to me, then left \ came back"
  return (
    <main>
      <h1>Don't panic</h1>
      <p>{msg}</p>
    </main>
  )
}
>>>ENDFILE

>>>FILE: src/app/globals.css
@tailwind base;
@tailwind components;
@tailwind utilities;
>>>ENDFILE`
	srv := newJSONServer(t, body)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewCodeAgent(provider, "test-model", nil)
	if err != nil {
		t.Fatalf("NewCodeAgent: %v", err)
	}

	out, err := agent.Execute(context.Background(), temporal.CodeInput{
		Spec:    temporal.PlanOutput{},
		AppKind: "website",
	})
	if err != nil {
		t.Fatalf("Execute: %v (this content would have broken JSON parsing)", err)
	}
	if len(out.Files) != 2 {
		t.Fatalf("expected 2 files, got %d: %+v", len(out.Files), out.Files)
	}
	page, ok := out.Files["src/app/page.tsx"]
	if !ok {
		t.Fatal("missing src/app/page.tsx")
	}
	if !strings.Contains(page, `Don't panic`) {
		t.Errorf("expected raw apostrophe preserved unescaped, got: %s", page)
	}
	if !strings.Contains(page, `she said "hi" to me, then left \ came back`) {
		t.Errorf("expected raw quotes and backslash preserved unescaped, got: %s", page)
	}
	if _, ok := out.Files["src/app/globals.css"]; !ok {
		t.Error("missing src/app/globals.css")
	}
}

// TestCodeAgentExecute_WithQAFeedback is a regression test for a real
// production bug: code.tmpl's QA Feedback section used `{{if .QAFeedback}}`
// followed by `{{range .}}` — `{{if}}` doesn't change the template's `.`
// context, so `{{range .}}` tried to range over the whole root CodeInput
// struct (not a slice) instead of `.QAFeedback`, and Go's html/template
// throws "range can't iterate over ..." at EXECUTE time. This crashed
// every single QA-retry code generation in production whenever
// QAFeedback was non-empty — the previous test above only covers the
// QAFeedback-empty (first-pass) path, so it never caught this; this test
// must exercise the real template with QAFeedback actually populated.
func TestCodeAgentExecute_WithQAFeedback(t *testing.T) {
	body := ">>>FILE: src/app/page.tsx\n" +
		"export default function Home(){return null}\n" +
		">>>ENDFILE"
	srv := newJSONServer(t, body)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewCodeAgent(provider, "test-model", nil)
	if err != nil {
		t.Fatalf("NewCodeAgent: %v", err)
	}

	_, err = agent.Execute(context.Background(), temporal.CodeInput{
		Spec:       temporal.PlanOutput{StyleDirection: "clean"},
		AppKind:    "website",
		PriorFiles: map[string]string{"src/app/page.tsx": "export default function Home() {}"},
		QAFeedback: []temporal.QAIssue{
			{File: "src/app/page.tsx", Line: 3, Severity: "blocking", Message: "Component 'X' is imported but was never generated."},
		},
	})
	if err != nil {
		t.Fatalf("Execute with non-empty QAFeedback: %v (must not fail template execution)", err)
	}
}

func TestCodeAgentExecute_BundleTooLarge(t *testing.T) {
	// Build a delimited file block whose content exceeds the 500KB per-call
	// cap enforced by CodeAgent.Execute.
	big := make([]byte, 500*1024+1024)
	for i := range big {
		big[i] = 'a'
	}
	body := ">>>FILE: big.txt\n" + string(big) + "\n>>>ENDFILE"
	srv := newJSONServer(t, body)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewCodeAgent(provider, "test-model", nil)
	if err != nil {
		t.Fatalf("NewCodeAgent: %v", err)
	}

	_, err = agent.Execute(context.Background(), temporal.CodeInput{
		Spec:    temporal.PlanOutput{},
		AppKind: "website",
	})
	if err == nil {
		t.Fatal("expected bundle-too-large error")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected a size-cap error, got: %v", err)
	}
}

func TestQAAgentExecute_PassedOverriddenByBlockingIssue(t *testing.T) {
	// Regression test: the LLM might inconsistently report passed=true while
	// also listing a blocking-severity issue. QAAgent.Execute must not trust
	// the self-reported "passed" field in that case.
	qaJSON := `{"passed":true,"issues":[{"file":"a.tsx","line":1,"severity":"blocking","message":"broken build"}]}`
	srv := newJSONServer(t, qaJSON)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewQAAgent(provider, "test-model")
	if err != nil {
		t.Fatalf("NewQAAgent: %v", err)
	}

	out, err := agent.Execute(context.Background(), temporal.QAInput{AppKind: "website"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Passed {
		t.Errorf("expected Passed=false when a blocking issue is present, got true")
	}
	if len(out.Issues) != 1 {
		t.Errorf("expected 1 issue to survive, got %d", len(out.Issues))
	}
}

func TestQAAgentExecute_PassedTrueWithOnlyWarnings(t *testing.T) {
	qaJSON := `{"passed":true,"issues":[{"file":"a.tsx","line":1,"severity":"warning","message":"missing alt"}]}`
	srv := newJSONServer(t, qaJSON)
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewQAAgent(provider, "test-model")
	if err != nil {
		t.Fatalf("NewQAAgent: %v", err)
	}

	out, err := agent.Execute(context.Background(), temporal.QAInput{AppKind: "website"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Passed {
		t.Errorf("expected Passed=true when only warning-severity issues are present")
	}
}

func TestQAAgentExecute_ProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewQAAgent(provider, "test-model")
	if err != nil {
		t.Fatalf("NewQAAgent: %v", err)
	}

	_, err = agent.Execute(context.Background(), temporal.QAInput{AppKind: "website"})
	if err == nil {
		t.Fatal("expected error when provider call fails")
	}
}

// fakeBuildRunner lets tests control what QAAgent.Execute's real-build check
// sees without shelling out to Docker.
type fakeBuildRunner struct {
	err      error
	buildLog string
}

func (f *fakeBuildRunner) Build(_ context.Context, _ uuid.UUID, _ map[string]string) (map[string]string, string, error) {
	if f.err != nil {
		return nil, f.buildLog, f.err
	}
	return map[string]string{}, f.buildLog, nil
}

// TestQAAgentExecute_RealBuildFailure_SkipsLLMAndAttributesPerFile is a
// regression test for a real production failure that reached "deploy" with
// no warning: QAAgent.Execute never actually ran a build (or looked at
// in.Files at all) before this fix — buildOutput was a hardcoded "succeeded"
// string, so the LLM was told every check already passed regardless of what
// was actually generated, and a "Module not found" build failure (missing
// Header/Footer/ContactForm) only ever surfaced much later, silently, at
// publish time. Now a real build runs first; on failure, the LLM call is
// skipped entirely (deterministic, zero tokens) and per-file QAIssues are
// synthesized from Next.js's standard error format so the retry loop can
// actually route feedback to the right target.
func TestQAAgentExecute_RealBuildFailure_SkipsLLMAndAttributesPerFile(t *testing.T) {
	// No LLM server configured at all — if Execute tried to call the
	// provider here, this would fail with a connection error, proving the
	// real-build check short-circuits before ever reaching the LLM call.
	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: "http://127.0.0.1:1"})
	agent, err := agents.NewQAAgent(provider, "test-model")
	if err != nil {
		t.Fatalf("NewQAAgent: %v", err)
	}

	buildLog := "./src/app/contact/page.tsx\n" +
		"Module not found: Can't resolve '@/components/ContactForm'\n" +
		"./src/app/layout.tsx\n" +
		"Module not found: Can't resolve '@/components/Header'\n"
	agents.SetBuilder(&fakeBuildRunner{err: fmt.Errorf("exit status 1"), buildLog: buildLog})
	defer agents.SetBuilder(nil)

	out, err := agent.Execute(context.Background(), temporal.QAInput{
		AppKind: "website",
		Files:   map[string]string{"src/app/contact/page.tsx": "..."},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Passed {
		t.Fatal("expected Passed=false on a real build failure")
	}
	if len(out.Issues) != 2 {
		t.Fatalf("expected 2 per-file issues, got %d: %+v", len(out.Issues), out.Issues)
	}
	byFile := map[string]string{}
	for _, issue := range out.Issues {
		byFile[issue.File] = issue.Message
	}
	if !strings.Contains(byFile["src/app/contact/page.tsx"], "ContactForm") {
		t.Errorf("expected contact page issue to mention ContactForm, got: %+v", out.Issues)
	}
	if !strings.Contains(byFile["src/app/layout.tsx"], "Header") {
		t.Errorf("expected layout issue to mention Header, got: %+v", out.Issues)
	}
}

// TestQAAgentExecute_NoBuilderConfigured_FallsBackToLLMPath keeps the
// pre-existing behavior intact when SetBuilder was never called (e.g. these
// other unit tests) or in.Files is empty — must not panic or error.
func TestQAAgentExecute_NoBuilderConfigured_FallsBackToLLMPath(t *testing.T) {
	qaJSON := `{"passed":true,"issues":[]}`
	srv := newJSONServer(t, qaJSON)
	defer srv.Close()

	agents.SetBuilder(nil)
	provider := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	agent, err := agents.NewQAAgent(provider, "test-model")
	if err != nil {
		t.Fatalf("NewQAAgent: %v", err)
	}

	out, err := agent.Execute(context.Background(), temporal.QAInput{AppKind: "website"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !out.Passed {
		t.Errorf("expected Passed=true, got issues: %+v", out.Issues)
	}
}
