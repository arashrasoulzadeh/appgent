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
	case "gapgpt":
		return NewGapGPTProvider(config), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", config.Type)
	}
}

// ProviderConfigFromEnv resolves the active provider's config the same way
// NewProviderFromEnv does, including provider-specific env vars taking
// priority over the generic AI_* fallbacks. Exported so callers that need
// just the resolved model (e.g. internal/agents.getModelForAgent's fallback
// when AI_MODEL_<AGENT> is unset) use the same precedence instead of
// re-reading AI_MODEL directly, which would ignore GAPGPT_MODEL/
// OLLAMA_MODEL/etc. when AI_PROVIDER isn't "openrouter".
func ProviderConfigFromEnv() ProviderConfig {
	providerType := strings.ToLower(os.Getenv("AI_PROVIDER"))
	if providerType == "" {
		providerType = "openrouter"
	}

	config := ProviderConfig{
		Type:    providerType,
		Options: map[string]string{},
	}

	// Provider-specific env vars take priority over the generic AI_*
	// fallbacks below — otherwise a leftover AI_MODEL from a previous
	// provider setup (e.g. an OpenRouter free-tier model id) silently
	// overrides GAPGPT_MODEL/OLLAMA_MODEL/etc. whenever it's still set,
	// even though AI_PROVIDER was switched to a different provider.
	switch providerType {
	case "openrouter":
		config.APIKey = os.Getenv("OPENROUTER_API_KEY")
		config.BaseURL = os.Getenv("OPENROUTER_BASE_URL")
		config.Model = os.Getenv("OPENROUTER_MODEL_PLAN")
	case "openai":
		config.APIKey = os.Getenv("OPENAI_API_KEY")
		config.BaseURL = os.Getenv("OPENAI_BASE_URL")
		config.Model = os.Getenv("OPENAI_MODEL")
	case "anthropic":
		config.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		config.BaseURL = os.Getenv("ANTHROPIC_BASE_URL")
		config.Model = os.Getenv("ANTHROPIC_MODEL")
	case "ollama":
		config.BaseURL = os.Getenv("OLLAMA_BASE_URL")
		config.Model = os.Getenv("OLLAMA_MODEL")
	case "gapgpt":
		config.APIKey = os.Getenv("GAPGPT_API_KEY")
		config.BaseURL = os.Getenv("GAPGPT_BASE_URL")
		config.Model = os.Getenv("GAPGPT_MODEL")
	}

	if config.APIKey == "" {
		config.APIKey = os.Getenv("AI_API_KEY")
	}
	if config.BaseURL == "" {
		config.BaseURL = os.Getenv("AI_BASE_URL")
	}
	if config.Model == "" {
		config.Model = os.Getenv("AI_MODEL")
	}

	return config
}

func NewProviderFromEnv() (Provider, error) {
	return NewProviderFromConfig(ProviderConfigFromEnv())
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
	case "gapgpt":
		return "gpt-4o-mini"
	default:
		return "nvidia/nemotron-3-ultra-550b-a55b:free"
	}
}