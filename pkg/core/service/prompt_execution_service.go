package service

import (
	"context"
	"fmt"

	gochains "ctoup.com/coreapp/pkg/core/service/gochains"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/prompts"
)

type PromptExecutionService struct {
	chainFactory *gochains.ChainFactory
}

func NewPromptExecutionService() *PromptExecutionService {
	return &PromptExecutionService{
		chainFactory: gochains.NewChainFactory(memory.NewSimple()),
	}
}

type ExecutePromptParams struct {
	Parameters map[string]string
}

func ExecutePrompt(ctx context.Context, content string, parameters []string, parametersValues ExecutePromptParams) (string, error) {
	// Validate that all required parameters are provided
	for _, requiredParam := range parameters {
		if _, exists := parametersValues.Parameters[requiredParam]; !exists {
			return "", fmt.Errorf("missing required parameter: %s", requiredParam)
		}
	}

	tpl := prompts.NewPromptTemplate(
		content,
		parameters,
	)

	// Convert map[string]string to map[string]any
	paramsAny := make(map[string]any, len(parametersValues.Parameters))
	for k, v := range parametersValues.Parameters {
		paramsAny[k] = v
	}

	formattedPrompt, err := tpl.Format(paramsAny)

	return formattedPrompt, err
}
