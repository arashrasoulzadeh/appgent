package embedding

import (
	"context"
	"fmt"

	"github.com/arashrasoulzadeh/appgent/internal/ai"
)

type Client struct {
	provider ai.Provider
	model    string
}

func NewClient(apiKey, baseURL, model string) (*Client, error) {
	provider, err := ai.NewProviderFromConfig(ai.ProviderConfig{
		Type:    "openrouter",
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   model,
	})
	if err != nil {
		return nil, fmt.Errorf("create provider: %w", err)
	}
	return &Client{provider: provider, model: model}, nil
}

func NewClientFromProvider(provider ai.Provider, model string) *Client {
	return &Client{provider: provider, model: model}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if c.provider == nil {
		return nil, fmt.Errorf("provider not configured")
	}
	return c.provider.Embed(ctx, text)
}

func (c *Client) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		emb, err := c.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("embed text %d: %w", i, err)
		}
		embeddings[i] = emb
	}
	return embeddings, nil
}