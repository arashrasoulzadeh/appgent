package ai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/ai"
)

// TestGetDefaultModelForProvider checks the pure lookup table used as the
// fallback model for each provider type.
func TestGetDefaultModelForProvider(t *testing.T) {
	cases := []struct {
		providerType string
		want         string
	}{
		{"openrouter", "nvidia/nemotron-3-ultra-550b-a55b:free"},
		{"OpenRouter", "nvidia/nemotron-3-ultra-550b-a55b:free"}, // case-insensitive
		{"openai", "gpt-4o-mini"},
		{"anthropic", "claude-3-5-haiku-20241022"},
		{"ollama", "llama3.1:8b"},
		{"unknown-provider", "nvidia/nemotron-3-ultra-550b-a55b:free"},
	}
	for _, tc := range cases {
		if got := ai.GetDefaultModelForProvider(tc.providerType); got != tc.want {
			t.Errorf("GetDefaultModelForProvider(%q) = %q, want %q", tc.providerType, got, tc.want)
		}
	}
}

func TestNewProviderFromConfig(t *testing.T) {
	cases := []struct {
		name     string
		cfgType  string
		wantName string
		wantErr  bool
	}{
		{"openrouter", "openrouter", "openrouter", false},
		{"openai", "openai", "openai", false},
		{"anthropic", "anthropic", "anthropic", false},
		{"ollama", "ollama", "ollama", false},
		{"unknown", "does-not-exist", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ai.NewProviderFromConfig(ai.ProviderConfig{Type: tc.cfgType})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for type %q, got nil", tc.cfgType)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if p.Name() != tc.wantName {
				t.Errorf("Name() = %q, want %q", p.Name(), tc.wantName)
			}
		})
	}
}

// TestOpenRouterChatCompletionWithJSONSchema is a regression test for a bug
// where the JSON schema was marshaled as a bare top-level "json_schema"
// field instead of being nested under "response_format", which meant the
// OpenRouter/OpenAI API silently ignored structured-output enforcement.
func TestOpenRouterChatCompletionWithJSONSchema(t *testing.T) {
	var captured map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","choices":[{"index":0,"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer srv.Close()

	p := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "test-key", BaseURL: srv.URL})

	schema := map[string]interface{}{"type": "object"}
	_, err := p.ChatCompletionWithJSONSchema(context.Background(), "some-model",
		[]ai.Message{{Role: "user", Content: "hi"}}, schema, 0.2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := captured["json_schema"]; ok {
		t.Errorf("request body should not have a top-level json_schema field, got: %v", captured)
	}

	rf, ok := captured["response_format"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response_format object in request body, got: %v", captured)
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want json_schema", rf["type"])
	}
	js, ok := rf["json_schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response_format.json_schema object, got: %v", rf)
	}
	if js["name"] != "response" {
		t.Errorf("json_schema.name = %v, want response", js["name"])
	}
}

// TestOpenAIChatCompletionWithJSONSchema is the same regression test for the
// OpenAI provider, which shares the same request-shaping bug.
func TestOpenAIChatCompletionWithJSONSchema(t *testing.T) {
	var captured map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","choices":[{"index":0,"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer srv.Close()

	p := ai.NewOpenAIProvider(ai.ProviderConfig{APIKey: "test-key", BaseURL: srv.URL})

	schema := map[string]interface{}{"type": "object"}
	_, err := p.ChatCompletionWithJSONSchema(context.Background(), "some-model",
		[]ai.Message{{Role: "user", Content: "hi"}}, schema, 0.2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := captured["json_schema"]; ok {
		t.Errorf("request body should not have a top-level json_schema field, got: %v", captured)
	}
	if _, ok := captured["response_format"]; !ok {
		t.Errorf("expected response_format in request body, got: %v", captured)
	}
}

func TestOpenRouterChatCompletionErrors(t *testing.T) {
	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		}))
		defer srv.Close()

		p := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
		_, err := p.ChatCompletion(context.Background(), ai.ChatCompletionRequest{Model: "m"})
		if err == nil {
			t.Fatal("expected error for 500 response")
		}
	})

	t.Run("empty choices", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"1","choices":[]}`))
		}))
		defer srv.Close()

		p := ai.NewOpenRouterProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
		_, err := p.ChatCompletion(context.Background(), ai.ChatCompletionRequest{Model: "m"})
		if err == nil {
			t.Fatal("expected error for empty choices")
		}
	})
}

// TestAnthropicDefaultMaxTokens is a regression test: Anthropic's API
// requires max_tokens to be a positive integer, so when the caller doesn't
// set one, the provider must substitute a sane default rather than sending 0.
func TestAnthropicDefaultMaxTokens(t *testing.T) {
	var captured map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()

	p := ai.NewAnthropicProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	_, err := p.ChatCompletion(context.Background(), ai.ChatCompletionRequest{
		Model:    "claude-3-5-haiku-20241022",
		Messages: []ai.Message{{Role: "user", Content: "hi"}},
		// MaxTokens intentionally left at zero value.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mt, ok := captured["max_tokens"].(float64)
	if !ok || mt <= 0 {
		t.Fatalf("expected a positive max_tokens in request, got: %v", captured["max_tokens"])
	}
}

// TestAnthropicSystemPromptExtraction verifies the leading system message is
// hoisted into the top-level "system" field, and not duplicated in the
// "messages" array sent to the Anthropic API.
func TestAnthropicSystemPromptExtraction(t *testing.T) {
	var captured map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn"}`))
	}))
	defer srv.Close()

	p := ai.NewAnthropicProvider(ai.ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	_, err := p.ChatCompletion(context.Background(), ai.ChatCompletionRequest{
		Model: "claude-3-5-haiku-20241022",
		Messages: []ai.Message{
			{Role: "system", Content: "be terse"},
			{Role: "user", Content: "hi"},
		},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured["system"] != "be terse" {
		t.Errorf("system = %v, want %q", captured["system"], "be terse")
	}
	msgs, ok := captured["messages"].([]interface{})
	if !ok || len(msgs) != 1 {
		t.Fatalf("expected exactly 1 message (system stripped), got: %v", captured["messages"])
	}
}
