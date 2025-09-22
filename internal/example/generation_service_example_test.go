package example

import (
	"context"
	"fmt"
	"testing"

	"ctoup.com/coreapp/pkg/core/service"
	gochains "ctoup.com/coreapp/pkg/core/service/gochains"
	"ctoup.com/coreapp/pkg/shared/llmmodels"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/outputparser"
)

func TestGenerationServiceWithUnstructuredOutput(t *testing.T) {
	// Setup the service
	// Define and run non-structured tests
	nonStructuredTestCases := []struct {
		name     string
		provider llmmodels.Provider
		model    string
	}{
		{"OpenAI_GPT4", llmmodels.ProviderOpenaAI, "gpt-4"},
		//{"OpenAI_GPT3.5_Turbo", llmmodels.ProviderOpenaAI, "gpt-3.5-turbo"},
		//{"Google_Gemini_Flash", llmmodels.ProviderGoogleAI, "gemini-1.5-flash"},
		//{"Anthropic_Claude3_Haiku", llmmodels.ProviderAnthropic, "claude-3-haiku-20240307"},
		//{"Mistral_Tiny", llmmodels.ProviderMistral, "mistral-tiny"},
	}

	for _, tc := range nonStructuredTestCases {
		t.Run(fmt.Sprintf("NonStructured_%s", tc.name), func(t *testing.T) {
			t.Parallel() // Run tests in parallel

			template := `Analyze the following job requirements:
Position: {{.position}}
Job Description: {{.job_description}}
Company Values: {{.company_values}}
Please provide a comprehensive skills analysis including technical skills, soft skills, experience requirements, and relevant certifications.`

			chain, err := gochains.NewChainBuilder().
				WithTemplate(template).
				WithParams([]string{"position", "job_description", "company_values"}).
				WithModel(tc.provider, tc.model).
				Build()
			require.NoError(t, err)

			params := map[string]any{
				"position":        "Senior Backend Engineer",
				"job_description": "Design, build, and maintain scalable and reliable backend services. Experience with Go, microservices, and cloud platforms like GCP or AWS is required.",
				"company_values":  "Ownership, impact, and continuous learning.",
			}

			result, err := service.GenerateTextAnswer(context.Background(), chain, params, nil)

			require.NoError(t, err)
			require.NotEmpty(t, result, "The generated text should not be empty")
			t.Logf("Non-structured response for %s: %s", tc.name, result)
		})
	}
}

func TestGenerationServiceWithStructuredOutput(t *testing.T) {

	// Define test cases for structured output
	structuredTestCases := []struct {
		name     string
		provider llmmodels.Provider
		model    string
	}{
		{"OpenAI_GPT4", llmmodels.ProviderOpenaAI, "gpt-4"},
		{"Anthropic_Claude3_Sonnet", llmmodels.ProviderAnthropic, "claude-3-7-sonnet-20250219"},
		//{"OpenAI_GPT3.5_Turbo", llmmodels.ProviderOpenaAI, "gpt-3.5-turbo"},
		{"OpenAI_GPT4_Turbo", llmmodels.ProviderOpenaAI, "gpt-4-turbo-preview"},
		{"Google_Gemini_Flash", llmmodels.ProviderGoogleAI, "gemini-2.5-flash"},
		{"Mistral_Tiny", llmmodels.ProviderMistral, "mistral-tiny"},
		//{"Anthropic_Claude3_Haiku", llmmodels.ProviderAnthropic, "claude-3-haiku-20240307"},
	}

	// Run structured tests
	for _, tc := range structuredTestCases {
		t.Run(fmt.Sprintf("Structured_%s", tc.name), func(t *testing.T) {
			t.Parallel() // Run tests in parallel

			// Create the chain for this test case
			template := `Analyze the following job requirements:
Position: {{.position}}
Job Description: {{.job_description}}
Company Values: {{.company_values}}
Please provide a comprehensive skills analysis including technical skills, soft skills, experience requirements, and relevant certifications.`

			responseSchemas := []outputparser.ResponseSchema{
				{Name: "technical_skills", Description: "List of required technical skills"},
				{Name: "soft_skills", Description: "List of important soft skills"},
				{Name: "experience_years", Description: "Minimum years of experience required"},
				{Name: "certifications", Description: "Relevant certifications if any"},
			}

			chain, err := gochains.NewChainBuilder().
				WithTemplate(template).
				WithParams([]string{"position", "job_description", "company_values"}).
				WithStructuredOutput(responseSchemas).
				WithModel(tc.provider, tc.model).
				WithMaxTokens(1000).
				WithTemperature(0.2).
				Build()
			require.NoError(t, err)

			// Define request params
			params := map[string]any{
				"position":        "Senior Backend Engineer",
				"job_description": "Design, build, and maintain scalable and reliable backend services. Experience with Go, microservices, and cloud platforms like GCP or AWS is required.",
				"company_values":  "Ownership, impact, and continuous learning.",
			}

			// Execute
			result, err := service.GenerateStructuredAnswer(context.Background(), chain, params, nil)
			if err != nil {
				chain, err = gochains.NewChainBuilder().
					WithTemplate(template).
					WithParams([]string{"position", "job_description", "company_values"}).
					WithCustomJSONFormat().
					WithModel(tc.provider, tc.model).
					Build()
				require.NoError(t, err)
				result, err := service.GenerateTextAnswer(context.Background(), chain, params, nil)
				require.NoError(t, err)
				structuredResult, err := service.ExtractAndValidateJSON(result, chain)

				require.NoError(t, err)
				t.Logf("Non-structured response for %s: %s, %v", tc.name, result, structuredResult)
				return
			}

			// Assert
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Contains(t, result, "technical_skills", "The key 'technical_skills' should be in the response")
			require.Contains(t, result, "soft_skills", "The key 'soft_skills' should be in the response")
			t.Logf("Structured response for %s: %+v", tc.name, result)
		})
	}
}
