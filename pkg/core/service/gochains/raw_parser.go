package gochains

import (
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/schema"
)

// RawStringParser is an output parser that returns the input text as-is.
type RawStringParser struct{}

// NewRawStringParser creates a new RawStringParser.
func NewRawStringParser() *RawStringParser {
	return &RawStringParser{}
}

// Statically assert that RawStringParser implements the OutputParser interface.
var _ schema.OutputParser[any] = (*RawStringParser)(nil)

// Parse returns the input text.
func (p *RawStringParser) Parse(text string) (any, error) {
	return text, nil
}

// ParseWithPrompt returns the input text.
func (p *RawStringParser) ParseWithPrompt(text string, prompt llms.PromptValue) (any, error) {
	return text, nil
}

// GetFormatInstructions returns an empty string.
func (p *RawStringParser) GetFormatInstructions() string {
	return ""
}

// Type returns the type of the parser.
func (p *RawStringParser) Type() string {
	return "raw_string_parser"
}
