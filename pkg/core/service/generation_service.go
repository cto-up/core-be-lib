package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"ctoup.com/coreapp/pkg/core/service/gochains"
	"ctoup.com/coreapp/pkg/shared/event"
	"ctoup.com/coreapp/pkg/shared/util"

	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/prompts"
)

type ScanState int

const (
	StateOut ScanState = iota
	StateIn
)

const (
	_llmChainDefaultOutputKey = "text"
)

// GenerateAnswer handles both regular and structured responses based on chain type
func (s *PromptExecutionService) GenerateAnswer(
	ctx context.Context,
	chainConfig *gochains.BaseChain,
	params map[string]any,
	userID string,
	clientChan chan<- event.ProgressEvent,
) (interface{}, error) {

	// Create the chain using the enhanced BaseChain
	chain := chains.LLMChain{
		Prompt: prompts.NewPromptTemplate(
			chainConfig.GetTemplateText(),
			chainConfig.GetParamDefinition(),
		),
		LLM:          chainConfig.GetModel(),
		Memory:       memory.NewSimple(),
		OutputKey:    _llmChainDefaultOutputKey,
		OutputParser: chainConfig.GetOutputParser(), // Set default parser
	}

	// For structured output, override with a raw parser to handle raw JSON from new models
	if chainConfig.GetChainType() == gochains.ChainTypeStructured {
		chain.OutputParser = gochains.NewRawStringParser()
	}

	// Use optimal temperature for the chain type
	temperature := chainConfig.GetOptimalTemperature()

	var res map[string]any
	var err error

	// Execute chain with or without streaming
	if clientChan != nil {
		res, err = chains.Call(ctx, chain, params,
			chains.WithMaxTokens(chainConfig.GetMaxTokens()),
			chains.WithTemperature(temperature),
			chains.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
				clientChan <- event.NewProgressEvent("MSG", string(chunk), 50)
				return nil
			}))
	} else {
		res, err = chains.Call(ctx, chain, params,
			chains.WithMaxTokens(chainConfig.GetMaxTokens()),
			chains.WithTemperature(temperature))
	}

	if err != nil {
		// if structured, check if we have a string response and try to parse it
		if chainConfig.GetChainType() == gochains.ChainTypeStructured {
			errMsg := err.Error()
			// Check if errMsg includes 'json_object' is not supported with this model
			errorReasons := []string{
				"'json_object' is not supported with this model",
				"'json_schema' is not present",
				"format not supported",
				"cannot process",
			}

			if util.ContainsAny(errMsg, errorReasons) {
				fmt.Println("Does not support json_object.")
			}
		}
		return nil, fmt.Errorf("chain execution failed: %w", err)
	}

	// Handle response based on chain type
	switch chainConfig.GetChainType() {
	case gochains.ChainTypeStructured:
		responseString, ok := res["text"].(string)
		if !ok {
			// This case might happen if the output parser somehow still converted it to a map
			if structuredResult, ok := res["text"].(map[string]any); ok {
				if err := chainConfig.ValidateStructuredResponse(structuredResult); err != nil {
					return nil, fmt.Errorf("structured response validation failed: %w", err)
				}
				return structuredResult, nil
			}
			return nil, fmt.Errorf("expected string for structured output, got %T", res["text"])
		}
		cleanedString, err := extractJSONFromResponse(responseString)
		if err != nil {
			// if we have an error, return the raw string
			return responseString, err
		}

		var structuredResult map[string]any
		if err := json.Unmarshal([]byte(cleanedString), &structuredResult); err != nil {
			return nil, fmt.Errorf("failed to unmarshal structured output from string: %w. String was: %s, cleaned string was: %s", err, responseString, cleanedString)
		}

		if err := chainConfig.ValidateStructuredResponse(structuredResult); err != nil {
			return nil, fmt.Errorf("structured response validation failed: %w", err)
		}
		return structuredResult, nil

	default:
		// Default behavior - return as string
		return res["text"].(string), nil
	}
}

// extractJSONFromResponse handles markdown code fences or raw JSON and other pre-processing
func extractJSONFromResponse(jsonString string) (string, error) {
	if jsonString == "" {
		return "", fmt.Errorf("empty string response for structured output")
	}

	// 1. Pre-process the string to handle markdown code fences with surrounding text.
	cleanedString := jsonString
	// This regex pattern matches anything (non-greedily) between ```json and ```.
	// The `(?s)` flag makes `.` match newlines as well.
	// re := regexp.MustCompile("(?s)```json(.*)```")
	re := regexp.MustCompile("(?s)```json(.*?)```")
	matches := re.FindStringSubmatch(jsonString)

	if len(matches) > 1 {
		// Trim leading/trailing whitespace and newlines from the captured content.
		cleanedString = strings.TrimSpace(matches[1])
	}
	return cleanedString, nil
}

// GenerateTextAnswer specifically returns string responses (backwards compatible)
func (s *PromptExecutionService) GenerateTextAnswer(
	ctx context.Context,
	chainConfig *gochains.BaseChain,
	params map[string]any,
	userID string,
	clientChan chan<- event.ProgressEvent,
) (string, error) {

	result, err := s.GenerateAnswer(ctx, chainConfig, params, userID, clientChan)
	if err != nil {
		return "", err
	}

	// Convert result to string based on type
	switch v := result.(type) {
	case string:
		return v, nil
	case map[string]string:
		// For structured responses, you might want to format them
		return fmt.Sprintf("Structured result: %+v", v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// GenerateStructuredAnswer specifically returns structured responses
func (s *PromptExecutionService) GenerateStructuredAnswer(
	ctx context.Context,
	chainConfig *gochains.BaseChain,
	params map[string]any,
	userID string,
	clientChan chan<- event.ProgressEvent,
) (map[string]any, error) {

	if !chainConfig.IsStructured() {
		return nil, fmt.Errorf("chain is not configured for structured output")
	}

	result, err := s.GenerateAnswer(ctx, chainConfig, params, userID, clientChan)
	if err != nil {
		return nil, err
	}

	if structuredResult, ok := result.(map[string]any); ok {
		return structuredResult, nil
	}

	return nil, fmt.Errorf("expected structured response but got %T", result)
}

// ConvertAnswerToString converts any value to string, handling both string and JSON marshaling
func (s *PromptExecutionService) ConvertAnswerToString(value any) (string, error) {
	// Check if the result is already a string
	if resultStr, ok := value.(string); ok {
		return resultStr, nil
	}

	// If not a string, marshal as JSON
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("failed to marshal value to JSON: %w", err)
	}

	return string(jsonBytes), nil
}
