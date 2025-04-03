package llmmodels

import (
	"errors"
	"os"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
)

const (
	MODEL_GPT_3_5_VERSION                                        string = "gpt-3.5-turbo-0125"
	MODEL_GPT_4_VERSION                                          string = "gpt-4-1106-preview"
	MODEL_GPT_3_5                                                string = "openai-gpt-3.5"
	MODEL_GPT_4                                                  string = "openai-gpt-4"
	MODEL_GPT_4O_MINI                                            string = "gpt-4o-mini"
	MODEL_LLAMA_2_VERSION                                        string = "llama2:13b"
	MODEL_MISTRAL_7B_VERSION                                     string = "mistral"
	MODEL_LLAMA_2                                                string = "llama-2-13b"
	MODEL_MISTRAL_7B                                             string = "mistral7b"
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

func NewLLM(model string, json bool) (llms.Model, error) {
	switch model {
	case MODEL_GPT_3_5:
		return newOpenAILLM(MODEL_GPT_3_5_VERSION, json)
	case MODEL_GPT_4:
		return newOpenAILLM(MODEL_GPT_4_VERSION, json)
	case MODEL_LLAMA_2:
		return newOllamaLLM(MODEL_LLAMA_2_VERSION, "http://127.0.0.1:11434")
	case MODEL_MISTRAL_7B:
		return newOllamaLLM(MODEL_MISTRAL_7B_VERSION, "http://127.0.0.1:11434")
	default:
		return nil, errors.New("unsupported model type")
	}
}
