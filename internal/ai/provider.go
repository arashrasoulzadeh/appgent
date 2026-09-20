package ai

import (
	"context"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	JSONSchema     *JSONSchema     `json:"-"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Name   string                 `json:"name"`
	Schema map[string]interface{} `json:"schema"`
	Strict bool                   `json:"strict"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type Provider interface {
	ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error)
	ChatCompletionWithJSONSchema(ctx context.Context, model string, messages []Message, schema map[string]interface{}, temperature float64) (*ChatCompletionResponse, error)
	Embed(ctx context.Context, text string) ([]float32, error)
	Name() string
	AvailableModels() []string
}

type ProviderConfig struct {
	Type     string            `json:"type"`     // openrouter, openai, anthropic, ollama
	APIKey   string            `json:"api_key"`
	BaseURL  string            `json:"base_url"`
	Model    string            `json:"model"`
	Options  map[string]string `json:"options"`
}