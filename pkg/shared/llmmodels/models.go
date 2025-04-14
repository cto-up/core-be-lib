package llmmodels

import (
	"context"
	"errors"
	"os"

	"fmt"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/anthropic"
	"github.com/tmc/langchaingo/llms/googleai"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

// Provider represents AI provider
type Provider string

const (
	ProviderOpenaAI   Provider = "OPENAI"
	ProviderGoogleAI  Provider = "GOOGLEAI"
	ProviderAnthropic Provider = "ANTHROPIC"
	ProviderOllama    Provider = "OLLAMA"
)

// IsValid checks if the provider is valid
func (at Provider) IsValid() bool {
	switch at {
	case ProviderAnthropic, ProviderGoogleAI, ProviderOpenaAI, ProviderOllama:
		return true
	default:
		return false
	}
}

// String returns the string representation of the provider
func (at Provider) String() string {
	return string(at)
}

// Values returns all possible values of Provider
func (Provider) Values() []Provider {
	return []Provider{
		ProviderGoogleAI,
		ProviderAnthropic,
		ProviderOpenaAI,
		ProviderOllama,
	}
}

// Parse converts a string to Provider
func (Provider) Parse(s string) (Provider, error) {
	switch strings.ToUpper(s) {
	case string(ProviderGoogleAI):
		return ProviderGoogleAI, nil
	case string(ProviderOpenaAI):
		return ProviderOpenaAI, nil
	case string(ProviderAnthropic):
		return ProviderAnthropic, nil
	case string(ProviderOllama):
		return ProviderOllama, nil
	default:
		return "", fmt.Errorf("invalid provider: %s", s)
	}
}

const (
	EMBEDDING_MODEL_TEXT_EMBEDDING_ADA_002                       string = "text-embedding-ada-002"
	EMBEDDING_MODEL_TEXT_NOMIC_EMBED_TEXT                        string = "nomic-embed-text"
	EMBEDDING_MODEL_TEXT_E5_MISTRAL_7B_INSTRUCT                  string = "hellord/e5-mistral-7b-instruct"
	EMBEDDING_MODEL_TEXT_INTFLOAT_MULTILINGUAL_E5_LARGE_INSTRUCT string = "jeffh/intfloat-multilingual-e5-large-instruct:f16"
	EMBEDDING_MODEL_TEXT_MULTILINGXUAL_E5_LARGE_INSTRUCT         string = "aroxima/multilingual-e5-large-instruct"
)

func newOpenAILLM(model string, json bool) (*openai.LLM, error) {
	if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey == "" {
		return nil, errors.New("OPENAI_API_KEY not set")
	}
	if json {
		return openai.New(openai.WithModel(model), openai.WithResponseFormat(&openai.ResponseFormat{
			Type: openai.ResponseFormatJSON.Type,
		}))
	}
	return openai.New(openai.WithModel(model))
}

func newOllamaLLM(model string, serverURL string) (*ollama.LLM, error) {
	return ollama.New(ollama.WithModel(model), ollama.WithServerURL(serverURL))
}

func newGeminiLLM(model string) (*googleai.GoogleAI, error) {
	geminiKey := os.Getenv("GOOGLEAI_API_KEY")
	if geminiKey == "" {
		return nil, errors.New("GOOGLEAI_API_KEY not set")
	}
	return googleai.New(context.Background(), googleai.WithAPIKey(geminiKey), googleai.WithDefaultModel(model))
}

func newAnthropicLLM(model string) (*anthropic.LLM, error) {
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY not set")
	}
	return anthropic.New(anthropic.WithModel(model))
}

func NewLLM(provider Provider, model string, json bool) (llms.Model, error) {

	switch provider {
	case ProviderOpenaAI:
		return newOpenAILLM(model, json)
	case ProviderOllama:
		ollamaServerURL := os.Getenv("OLLAMA_SERVER_URL")
		if ollamaServerURL == "" {
			model := &openai.LLM{}
			return model, errors.New("OLLAMA_SERVER_URL not set")
		}
		return newOllamaLLM(model, ollamaServerURL)
	case ProviderGoogleAI:
		return newGeminiLLM(model)
	case ProviderAnthropic:
		return newAnthropicLLM(model)
	default:
		return nil, errors.New("unsupported model" + model + " for provider " + provider.String())
	}
}
