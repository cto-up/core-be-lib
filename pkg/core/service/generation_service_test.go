package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractJSONFromResponse(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedOutput string
		expectError    bool
	}{
		{
			name:           "Empty String",
			input:          "",
			expectedOutput: "",
			expectError:    true,
		},
		{
			name:           "Plain JSON",
			input:          `{"key": "value"}`,
			expectedOutput: `{"key": "value"}`,
			expectError:    false,
		},
		{
			name:           "JSON with Markdown Fence",
			input:          "```json\n{\"key\": \"value\"}\n```",
			expectedOutput: `{"key": "value"}`,
			expectError:    false,
		},
		{
			name:           "JSON with Markdown Fence and surrounding text",
			input:          "Here is the JSON:\n```json\n{\"key\": \"value\"}\n```\nLet me know if you need more info.",
			expectedOutput: `{"key": "value"}`,
			expectError:    false,
		},
		{
			name:           "JSON with extra whitespace inside fence",
			input:          "```json \n\n {\"key\": \"value\"} \n\n ```",
			expectedOutput: `{"key": "value"}`,
			expectError:    false,
		},
		{
			name:           "No JSON, just text",
			input:          "This is just some text.",
			expectedOutput: "This is just some text.",
			expectError:    false,
		},
		{
			name:           "Multiple markdown blocks (finds first)",
			input:          "```json\n{\"a\": 1}\n``` some text ```json\n{\"b\": 2}\n```",
			expectedOutput: `{"a": 1}`,
			expectError:    false,
		},
		{
			name:           "JSON without json language identifier",
			input:          "```\n{\"key\": \"value\"}\n```",
			expectedOutput: "```\n{\"key\": \"value\"}\n```", 
			expectError:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := extractJSONFromResponse(tc.input)

			if tc.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedOutput, output)
			}
		})
	}
}
