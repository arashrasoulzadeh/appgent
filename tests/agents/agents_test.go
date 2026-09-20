package agents_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	filesJSON := `{"src/app/page.tsx":"export default function Home(){return null}"}`
	srv := newJSONServer(t, filesJSON)
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

func TestCodeAgentExecute_BundleTooLarge(t *testing.T) {
	// Build a JSON payload whose single file content exceeds the 2MB cap
	// enforced by CodeAgent.Execute.
	big := make([]byte, 2*1024*1024+1024)
	for i := range big {
		big[i] = 'a'
	}
	payload, _ := json.Marshal(map[string]string{"big.txt": string(big)})
	srv := newJSONServer(t, string(payload))
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
