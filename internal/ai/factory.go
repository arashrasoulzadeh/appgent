package ai

import (
	"fmt"
	"os"
	"strings"
)

func NewProviderFromConfig(config ProviderConfig) (Provider, error) {
	switch strings.ToLower(config.Type) {
	case "openrouter":
		return NewOpenRouterProvider(config), nil
	case "openai":
		return NewOpenAIProvider(config), nil
	case "anthropic":
		return NewAnthropicProvider(config), nil
	case "ollama":
		return NewOllamaProvider(config), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", config.Type)
	}
}

func NewProviderFromEnv() (Provider, error) {
	providerType := strings.ToLower(os.Getenv("AI_PROVIDER"))
	if providerType == "" {
		providerType = "openrouter"
	}

	config := ProviderConfig{
		Type:    providerType,
		APIKey:  os.Getenv("AI_API_KEY"),
		BaseURL: os.Getenv("AI_BASE_URL"),
		Model:   os.Getenv("AI_MODEL"),
		Options: map[string]string{},
	}

	// Provider-specific env vars
	switch providerType {
	case "openrouter":
		if config.APIKey == "" {
			config.APIKey = os.Getenv("OPENROUTER_API_KEY")
		}
		if config.BaseURL == "" {
			config.BaseURL = os.Getenv("OPENROUTER_BASE_URL")
		}
		if config.Model == "" {
			config.Model = os.Getenv("OPENROUTER_MODEL_PLAN")
		}
	case "openai":
		if config.APIKey == "" {
			config.APIKey = os.Getenv("OPENAI_API_KEY")
		}
		if config.BaseURL == "" {
			config.BaseURL = os.Getenv("OPENAI_BASE_URL")
		}
		if config.Model == "" {
			config.Model = os.Getenv("OPENAI_MODEL")
		}
	case "anthropic":
		if config.APIKey == "" {
			config.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		if config.BaseURL == "" {
			config.BaseURL = os.Getenv("ANTHROPIC_BASE_URL")
		}
		if config.Model == "" {
			config.Model = os.Getenv("ANTHROPIC_MODEL")
		}
	case "ollama":
		if config.BaseURL == "" {
			config.BaseURL = os.Getenv("OLLAMA_BASE_URL")
		}
		if config.Model == "" {
			config.Model = os.Getenv("OLLAMA_MODEL")
		}
	}

	return NewProviderFromConfig(config)
}

func GetDefaultModelForProvider(providerType string) string {
	switch strings.ToLower(providerType) {
	case "openrouter":
		return "nvidia/nemotron-3-ultra-550b-a55b:free"
	case "openai":
		return "gpt-4o-mini"
	case "anthropic":
		return "claude-3-5-haiku-20241022"
	case "ollama":
		return "llama3.1:8b"
	default:
		return "nvidia/nemotron-3-ultra-550b-a55b:free"
	}
}