package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AnthropicProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewAnthropicProvider(config ProviderConfig) *AnthropicProvider {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	return &AnthropicProvider{
		apiKey:  config.APIKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

func (p *AnthropicProvider) AvailableModels() []string {
	return []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
	}
}

type anthropicRequest struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature,omitempty"`
	Messages    []Message `json:"messages"`
	System      string    `json:"system,omitempty"`
}

type anthropicResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Content      []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *AnthropicProvider) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	systemPrompt := ""
	messages := req.Messages
	if len(messages) > 0 && messages[0].Role == "system" {
		systemPrompt = messages[0].Content
		messages = messages[1:]
	}

	anthropicReq := anthropicRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    messages,
		System:      systemPrompt,
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	var content string
	for _, c := range anthResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &ChatCompletionResponse{
		ID: anthResp.ID,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: anthResp.StopReason,
			},
		},
		Usage: Usage{
			PromptTokens:     anthResp.Usage.InputTokens,
			CompletionTokens: anthResp.Usage.OutputTokens,
			TotalTokens:      anthResp.Usage.InputTokens + anthResp.Usage.OutputTokens,
		},
	}, nil
}

func (p *AnthropicProvider) ChatCompletionWithJSONSchema(ctx context.Context, model string, messages []Message, schema map[string]interface{}, temperature float64) (*ChatCompletionResponse, error) {
	// Add JSON schema instruction to system prompt
	systemPrompt := "You must respond with valid JSON matching the provided schema. Output ONLY the JSON object, no additional text."
	if len(messages) > 0 && messages[0].Role == "system" {
		systemPrompt = messages[0].Content + "\n\n" + systemPrompt
		messages = messages[1:]
	}

	req := ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: temperature,
	}

	// Prepend system message
	allMessages := append([]Message{{Role: "system", Content: systemPrompt}}, messages...)
	req.Messages = allMessages

	return p.ChatCompletion(ctx, req)
}

func (p *AnthropicProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	// Anthropic doesn't provide embeddings API, fallback to a simple hash-based embedding
	// In production, you'd use a separate embedding provider
	return nil, fmt.Errorf("embeddings not supported by Anthropic provider")
}