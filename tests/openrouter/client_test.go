package openrouter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arashrasoulzadeh/appgent/internal/openrouter"
)

func TestClientChatCompletion(t *testing.T) {
	var gotAuth, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotReferer = r.Header.Get("HTTP-Referer")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","choices":[{"index":0,"message":{"role":"assistant","content":"hello"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	defer srv.Close()

	c := openrouter.NewClient("secret-key", srv.URL)
	resp, err := c.ChatCompletion(context.Background(), openrouter.ChatCompletionRequest{
		Model:    "some-model",
		Messages: []openrouter.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "hello" {
		t.Errorf("content = %q, want hello", resp.Choices[0].Message.Content)
	}
	if gotAuth != "Bearer secret-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer secret-key")
	}
	if gotReferer == "" {
		t.Errorf("expected HTTP-Referer header to be set")
	}
}

func TestClientChatCompletionWithJSONSchemaShapesResponseFormat(t *testing.T) {
	var captured map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","choices":[{"index":0,"message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer srv.Close()

	c := openrouter.NewClient("k", srv.URL)
	schema := map[string]interface{}{"type": "object"}
	_, err := c.ChatCompletionWithJSONSchema(context.Background(), "m",
		[]openrouter.Message{{Role: "user", Content: "hi"}}, schema, 0.5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rf, ok := captured["response_format"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected response_format in request, got: %v", captured)
	}
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v, want json_schema", rf["type"])
	}
}

func TestClientChatCompletionDefaultBaseURL(t *testing.T) {
	c := openrouter.NewClient("k", "")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	// Exercise it indirectly: a request against an unreachable default host
	// should fail with a network error, not a nil pointer panic.
	_, err := c.ChatCompletion(context.Background(), openrouter.ChatCompletionRequest{Model: "m"})
	if err == nil {
		t.Log("no error contacting default OpenRouter host (unexpected but not fatal in this environment)")
	}
}

func TestClientChatCompletionErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	c := openrouter.NewClient("bad-key", srv.URL)
	_, err := c.ChatCompletion(context.Background(), openrouter.ChatCompletionRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}
