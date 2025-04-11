package service

import (
	"context"
	"fmt"
	"strings"

	"ctoup.com/coreapp/pkg/core/db"
	"ctoup.com/coreapp/pkg/core/db/repository"
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

	// Replace parameters in content
	result := prompt.Content
	for param, value := range params.Parameters {
		result = strings.ReplaceAll(result, fmt.Sprintf("{{%s}}", param), value)
	}

	return result, nil
}
