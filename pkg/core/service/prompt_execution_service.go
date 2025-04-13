package service

import (
	"context"
	"fmt"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
	"github.com/tmc/langchaingo/prompts"
)

type PromptExecutionService struct {
	store *db.Store
}

func NewPromptExecutionService(store *db.Store) *PromptExecutionService {
	return &PromptExecutionService{
		store: store,
	}
}

type ExecutePromptParams struct {
	Parameters map[string]string
}

func (s *PromptExecutionService) ExecutePrompt(ctx context.Context, prompt repository.CorePrompt, params ExecutePromptParams) (string, error) {
	// Validate that all required parameters are provided
	for _, requiredParam := range prompt.Parameters {
		if _, exists := params.Parameters[requiredParam]; !exists {
			return "", fmt.Errorf("missing required parameter: %s", requiredParam)
		}
	}

	tpl := prompts.NewPromptTemplate(
		prompt.Content,
		prompt.Parameters,
	)

	// Convert map[string]string to map[string]any
	paramsAny := make(map[string]any, len(params.Parameters))
	for k, v := range params.Parameters {
		paramsAny[k] = v
	}

	formattedPrompt, err := tpl.Format(paramsAny)

	return formattedPrompt, err
}
