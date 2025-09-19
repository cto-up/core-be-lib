package service

import (
	"context"
	"testing"

	"ctoup.com/coreapp/pkg/core/db/repository"
	"ctoup.com/coreapp/pkg/core/db/testutils"

	"github.com/stretchr/testify/require"
)

func TestPromptExecutionService(t *testing.T) {
	store := testutils.NewTestStore(t)
	service := NewPromptExecutionService(store)

	// Create a test prompt
	prompt := repository.CorePrompt{
		UserID:     "test-user",
		TenantID:   "test-tenant",
		Name:       "greeting",
		Content:    "Hello {{.name}}, welcome to {{.company}}!",
		Parameters: []string{"name", "company"},
		Tags:       []string{"greeting", "welcome"},
	}

	tests := []struct {
		name           string
		params         ExecutePromptParams
		expectedResult string
		expectedError  string
	}{
		{
			name: "execute by id - success",
			params: ExecutePromptParams{
				Parameters: map[string]string{
					"name":    "John",
					"company": "Acme",
				},
			},
			expectedResult: "Hello John, welcome to Acme!",
		},
		{
			name: "execute by name - success",
			params: ExecutePromptParams{
				Parameters: map[string]string{
					"name":    "John",
					"company": "Acme",
				},
			},
			expectedResult: "Hello John, welcome to Acme!",
		},
		{
			name: "missing parameter",
			params: ExecutePromptParams{
				Parameters: map[string]string{
					"name": "John",
				},
			},
			expectedError: "missing required parameter: company",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := service.ExecutePrompt(context.Background(), prompt.Content, prompt.Parameters, tt.params)

			if tt.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
