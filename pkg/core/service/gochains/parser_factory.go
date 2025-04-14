package gochains

import (
	"github.com/tmc/langchaingo/llms"
)

type OutputParserConfig struct {
	FormatInstructions string
}

// BaseOutputParser provides common functionality for all parsers
type BaseOutputParser struct {
	formatInstructions string
	parserType         string
}

func (p *BaseOutputParser) GetFormatInstructions() string {
	return p.formatInstructions
}

func (p *BaseOutputParser) Parse(text string) (any, error) {
	return text, nil
}

func (p *BaseOutputParser) ParseWithPrompt(text string, _ llms.PromptValue) (any, error) {
	return text, nil
}

func (p *BaseOutputParser) Type() string {
	return p.parserType
}

func (p *BaseOutputParser) GetFormat() string {
	return p.formatInstructions
}
