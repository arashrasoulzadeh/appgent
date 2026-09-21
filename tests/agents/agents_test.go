package agents_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
