package gochains

import (
	"ctoup.com/coreapp/pkg/shared/llmmodels"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/schema"
)

const (
	_llmChainDefaultOutputKey = "text"
	_markdownFormat           = "The output must be in plain markdown format without enclosing it with markdown tag or fence"
)

// BaseChain provides common functionality for all chains
type BaseChain struct {
	templateText    string
	paramDefinition []string
	outputParser    schema.OutputParser[any]
	maxTokens       int
	temperature     float64
	model           llms.Model
}

// Getters
func (bc *BaseChain) GetTemplateText() string {
	return bc.templateText
}

func (bc *BaseChain) GetParamDefinition() []string {
	return bc.paramDefinition
}

func (bc *BaseChain) GetOutputParser() schema.OutputParser[any] {
	return bc.outputParser
}

func (bc *BaseChain) GetMaxTokens() int {
	return bc.maxTokens
}

func (bc *BaseChain) GetTemperature() float64 {
	return bc.temperature
}

func (bc *BaseChain) GetModel() llms.Model {
	return bc.model
}

func NewBaseChain(templateText string, paramDefinition []string, formatInstructions string, maxTokens int, temperature float64, provider llmmodels.Provider, model string, isJson bool) (*BaseChain, error) {
	llmmodel, err := llmmodels.NewLLM(provider, model, isJson)
	if err != nil {
		return nil, err
	}

	outputParser := &BaseOutputParser{
		formatInstructions: formatInstructions,
		parserType:         "default_parser",
	}

	return &BaseChain{
		templateText:    templateText,
		paramDefinition: paramDefinition,
		outputParser:    outputParser,
		maxTokens:       maxTokens,
		temperature:     temperature,
		model:           llmmodel,
	}, nil
}

// NewLLMChain creates a new LLMChain with common configuration
func (bc *BaseChain) NewLLMChain(llm llms.Model, memory schema.Memory) chains.LLMChain {
	return chains.LLMChain{
		Prompt: prompts.NewPromptTemplate(
			bc.templateText,
			bc.paramDefinition,
		),
		LLM:          llm,
		Memory:       memory,
		OutputParser: bc.outputParser,
		OutputKey:    _llmChainDefaultOutputKey,
	}
}
