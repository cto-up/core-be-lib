package gochains

import (
	"fmt"

	"ctoup.com/coreapp/pkg/shared/llmmodels"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/outputparser"
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/schema"
)

const (
	_llmChainDefaultOutputKey = "text"
)

// ChainType defines the type of chain and output parsing.
type ChainType string

const (
	ChainTypeDefault    ChainType = "default"
	ChainTypeStructured ChainType = "structured"
)

// BaseChain provides common functionality for all chains.
type BaseChain struct {
	templateText       string
	paramDefinition    []string
	outputParser       schema.OutputParser[any]
	maxTokens          int
	temperature        float64
	model              llms.Model
	chainType          ChainType
	responseSchemas    []outputparser.ResponseSchema
	formatInstructions string
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

func (bc *BaseChain) SetModel(model llms.Model) {
	bc.model = model
}

func (bc *BaseChain) GetChainType() ChainType {
	return bc.chainType
}
func (bc *BaseChain) SetChainType(chainType ChainType) {
	bc.chainType = chainType
}

func (bc *BaseChain) GetResponseSchemas() []outputparser.ResponseSchema {
	return bc.responseSchemas
}

func (bc *BaseChain) IsStructured() bool {
	return bc.chainType == ChainTypeStructured
}

// NewBaseChain creates a standard BaseChain.
func NewBaseChain(templateText string, paramDefinition []string, formatInstructions string, maxTokens int, temperature float64, provider llmmodels.Provider, model string) (*BaseChain, error) {
	llmmodel, err := llmmodels.NewLLM(provider, model, false)
	if err != nil {
		return nil, err
	}

	outputParser := &BaseOutputParser{
		formatInstructions: formatInstructions,
		parserType:         "default_parser",
	}

	return &BaseChain{
		templateText:       templateText,
		paramDefinition:    paramDefinition,
		outputParser:       outputParser,
		maxTokens:          maxTokens,
		temperature:        temperature,
		model:              llmmodel,
		chainType:          ChainTypeDefault,
		formatInstructions: formatInstructions,
	}, nil
}

// NewStructuredChain creates a BaseChain with structured or custom JSON output parsing.
func NewStructuredChain(
	templateText string,
	paramDefinition []string,
	responseSchemas []outputparser.ResponseSchema,
	customFormatInstr string,
	maxTokens int,
	temperature float64,
	provider llmmodels.Provider,
	model string,
) (*BaseChain, error) {
	llmmodel, err := llmmodels.NewLLM(provider, model, true)
	if err != nil {
		return nil, err
	}

	var outputParser schema.OutputParser[any]
	var formatInstructions string

	if len(responseSchemas) > 0 {
		// Use standard structured output with predefined schemas.
		structuredParser := outputparser.NewStructured(responseSchemas)
		outputParser = structuredParser
		formatInstructions = structuredParser.GetFormatInstructions()
	} else if customFormatInstr != "" {
		// Use a custom JSON format with a dedicated parser.
		outputParser = &BaseOutputParser{
			formatInstructions: customFormatInstr,
			parserType:         "structured_parser",
		}
		formatInstructions = customFormatInstr
	} else {
		return nil, fmt.Errorf("structured chain requires either response schemas or custom format instructions")
	}

	enhancedTemplate := enhanceTemplateForStructuredOutput(templateText, formatInstructions)

	return &BaseChain{
		templateText:       enhancedTemplate,
		paramDefinition:    paramDefinition,
		outputParser:       outputParser,
		maxTokens:          maxTokens,
		temperature:        temperature,
		model:              llmmodel,
		chainType:          ChainTypeStructured,
		responseSchemas:    responseSchemas,
		formatInstructions: formatInstructions,
	}, nil
}

// enhanceTemplateForStructuredOutput adds structured output instructions
func enhanceTemplateForStructuredOutput(originalTemplate string, formatInstructions string) string {
	return originalTemplate + "\n\n" +
		"Your response must be a single, valid JSON object. Do not include any other text or markdown formatting. " +
		"The JSON object must conform to this structure:\n" +
		formatInstructions
}

// NewLLMChain creates a new LLMChain with common configuration.
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

// CreatePromptTemplate creates the appropriate prompt template based on chain type.
func (bc *BaseChain) CreatePromptTemplate() *prompts.PromptTemplate {
	tmp := prompts.NewPromptTemplate(bc.templateText, bc.paramDefinition)
	return &tmp
}

// GetOptimalTemperature returns the optimal temperature for the chain type.
func (bc *BaseChain) GetOptimalTemperature() float64 {
	switch bc.chainType {
	case ChainTypeStructured:
		// Lower temperature for structured output to ensure consistency.
		if bc.temperature > 0.3 {
			return 0.2
		}
		return bc.temperature
	default:
		return bc.temperature
	}
}

// ValidateStructuredResponse validates that the response contains expected keys.
func (bc *BaseChain) ValidateStructuredResponse(response map[string]any) error {
	if bc.chainType != ChainTypeStructured {
		return nil // No validation needed for non-structured chains.
	}

	for _, schema := range bc.responseSchemas {
		if _, exists := response[schema.Name]; !exists {
			return fmt.Errorf("missing required field: %s", schema.Name)
		}
	}
	return nil
}

// GetFormatInstructions returns the format instructions for the chain.
func (bc *BaseChain) GetFormatInstructions() string {
	return bc.formatInstructions
}

// Builder pattern for creating chains with fluent interface.

// ChainBuilder provides a fluent interface for building chains.
type ChainBuilder struct {
	templateText      string
	paramDefinition   []string
	maxTokens         int
	temperature       float64
	provider          llmmodels.Provider
	model             string
	isJson            bool
	chainType         ChainType
	responseSchemas   []outputparser.ResponseSchema
	customFormatInstr string
}

// NewChainBuilder creates a new chain builder.
func NewChainBuilder() *ChainBuilder {
	return &ChainBuilder{
		maxTokens:   1000,
		temperature: 0.7,
		chainType:   ChainTypeDefault,
	}
}

func (cb *ChainBuilder) WithTemplate(template string) *ChainBuilder {
	cb.templateText = template
	return cb
}

func (cb *ChainBuilder) WithParams(params []string) *ChainBuilder {
	cb.paramDefinition = params
	return cb
}

func (cb *ChainBuilder) WithMaxTokens(tokens int) *ChainBuilder {
	cb.maxTokens = tokens
	return cb
}

func (cb *ChainBuilder) WithTemperature(temp float64) *ChainBuilder {
	cb.temperature = temp
	return cb
}

func (cb *ChainBuilder) WithModel(provider llmmodels.Provider, model string) *ChainBuilder {
	cb.provider = provider
	cb.model = model
	return cb
}

func (cb *ChainBuilder) WithJSON(isJson bool) *ChainBuilder {
	cb.isJson = isJson
	return cb
}

func (cb *ChainBuilder) WithStructuredOutput(schemas []outputparser.ResponseSchema) *ChainBuilder {
	cb.chainType = ChainTypeStructured
	cb.responseSchemas = schemas
	cb.isJson = true // Structured output requires JSON mode.
	return cb
}

const _structuredFormatInstructionTemplate = "The output should be a json object formatted in the following schema: \n```json\n{\n%s}\n```" // nolint
const _structuredLineTemplate = "\"%s\": %s // %s\n"

func (cb *ChainBuilder) WithCustomJSONFormat() *ChainBuilder {
	cb.chainType = ChainTypeDefault

	jsonLines := ""
	for _, rs := range cb.responseSchemas {
		jsonLines += "\t" + fmt.Sprintf(
			_structuredLineTemplate,
			rs.Name,
			"string", /* type of the filed*/
			rs.Description,
		)
	}
	cb.customFormatInstr = fmt.Sprintf(_structuredFormatInstructionTemplate, jsonLines)
	cb.templateText = enhanceTemplateForStructuredOutput(cb.templateText, jsonLines)

	cb.isJson = false
	return cb
}

func (cb *ChainBuilder) Build() (*BaseChain, error) {
	switch cb.chainType {
	case ChainTypeStructured:
		return NewStructuredChain(
			cb.templateText,
			cb.paramDefinition,
			cb.responseSchemas,
			cb.customFormatInstr,
			cb.maxTokens,
			cb.temperature,
			cb.provider,
			cb.model,
		)
	default:
		return NewBaseChain(
			cb.templateText,
			cb.paramDefinition,
			"",
			cb.maxTokens,
			cb.temperature,
			cb.provider,
			cb.model,
		)
	}
}
