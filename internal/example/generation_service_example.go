package example

import (
	"context"
	"fmt"

	"ctoup.com/coreapp/pkg/core/service"
	gochains "ctoup.com/coreapp/pkg/core/service/gochains"
	"ctoup.com/coreapp/pkg/shared/event"
	"ctoup.com/coreapp/pkg/shared/llmmodels"
)

// Example usage methods showing how to create and use different chain types

type QuestionGeneratorRequest struct {
	Position, SeekTraits string
	NumberOfQuestions    int
}

type SkillGeneratorRequest struct {
	Position       string
	JobDescription string
	CompanyValues  string
}

type SimpleAnswerRequest struct {
	Topic string
}

// CreateSkillAnalysisChain creates a structured chain for skill analysis
func CreateSkillAnalysisChain() (*gochains.BaseChain, error) {
	template := `Analyze the following job requirements:

Position: {{.position}}
Job Description: {{.job_description}}
Company Values: {{.company_values}}

Please provide a comprehensive skills analysis including technical skills, soft skills, experience requirements, and relevant certifications.`

	/*responseSchemas := []outputparser.ResponseSchema{
		{Name: "technical_skills", Description: "List of required technical skills"},
		{Name: "soft_skills", Description: "List of important soft skills"},
		{Name: "experience_years", Description: "Minimum years of experience required"},
		{Name: "certifications", Description: "Relevant certifications if any"},
	}*/

	chain, err := gochains.NewBaseChain(
		template,
		[]string{"position", "job_description", "company_values"},
		"",
		1000,
		0.2,
		llmmodels.ProviderGoogleAI,
		"gemini-2.5-flash")

	return chain, err

	/*return gochains.NewChainBuilder().
	WithTemplate(template).
	WithParams([]string{"position", "job_description", "company_values"}).
	WithStructuredOutput(responseSchemas).
	WithModel(llmmodels.ProviderGoogleAI, "gemini-2.5-flash").
	//WithModel(llmmodels.ProviderMistral, "mistral-tiny"). // Supports json_object
	//WithModel(llmmodels.ProviderOpenaAI, "gpt-4"). // Does not support json_object
	//WithModel(llmmodels.ProviderOpenaAI, "gpt-4-turbo-preview"). // Supports json_object
	WithMaxTokens(1000).
	WithTemperature(0.2).
	Build()*/
}

// CreateQuestionGeneratorChain creates a structured chain for question generation

/*
func CreateQuestionGeneratorChain() (*gochains.BaseChain, error) {
	template := `Generate interview questions for the following requirements:

Position: {{.position}}
Seek Traits: {seek_traits}
Number of Questions: {number_of_questions}

Create relevant interview questions that assess the candidate's fit for this role.`

	responseSchemas := []outputparser.ResponseSchema{
		{Name: "questions", Description: "List of interview questions separated by newlines"},
		{Name: "difficulty_level", Description: "Overall difficulty level (Beginner/Intermediate/Advanced)"},
		{Name: "focus_areas", Description: "Main focus areas covered by the questions"},
		{Name: "estimated_time", Description: "Estimated time needed for the interview"},
	}

	return gochains.NewChainBuilder().
		WithTemplate(template).
		WithParams([]string{"position", "seek_traits", "number_of_questions"}).
		WithStructuredOutput(responseSchemas).
		WithModel(llmmodels.ProviderOpenaAI, "gpt-4-turbo-preview").
		WithMaxTokens(1500).
		WithTemperature(0.3).
		Build()
}

// CreateSimpleChain creates a regular chain (backwards compatible)
func CreateSimpleChain(template string, params []string) (*gochains.BaseChain, error) {
	return gochains.NewChainBuilder().
		WithTemplate(template).
		WithParams(params).
		WithModel(llmmodels.ProviderOpenaAI, "gpt-3.5-turbo").
		WithMaxTokens(800).
		WithTemperature(0.7).
		Build()
}
*/
// GenerateSimpleAnswer demonstrates how to generate a simple, non-structured text answer.
func GenerateSimpleAnswer(
	ctx context.Context,
	s *service.PromptExecutionService,
	clientChan chan<- event.ProgressEvent,
) (interface{}, error) {

	// 1. Sample values for the simple answer generation
	content := "Explain {{.topic}} in simple terms for a beginner."
	parameters := []string{"topic"}
	formatInstructions := "" // Not used for simple, non-JSON chains
	maxTokens := 250
	temperature := 0.7
	provider := llmmodels.ProviderOpenaAI
	llm := "gpt-3.5-turbo"

	request := SimpleAnswerRequest{
		Topic: "Quantum Computing",
	}

	parametersValues := map[string]any{
		"topic": request.Topic,
	}

	// 2. Create chain config using the direct NewBaseChain constructor
	chainConfig, err := gochains.NewBaseChain(
		content,
		parameters,
		formatInstructions,
		maxTokens,
		temperature,
		provider,
		llm,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create simple chain: %w", err)
	}

	// 3. Call the generic GenerateAnswer service method
	return s.GenerateAnswer(ctx,
		chainConfig,
		parametersValues,
		"a-user-id", // Sample user ID
		clientChan,
	)
}

// GenerateSkillsAnalysis generates skills analysis using structured output
func GenerateSkillsAnalysis(
	ctx context.Context,
	s *service.PromptExecutionService,
	request SkillGeneratorRequest,
	userID string,
	clientChan chan<- event.ProgressEvent,
) (map[string]any, error) {

	chainConfig, err := CreateSkillAnalysisChain()
	if err != nil {
		return nil, fmt.Errorf("failed to create skill analysis chain: %w", err)
	}

	params := map[string]any{
		"position":        request.Position,
		"job_description": request.JobDescription,
		"company_values":  request.CompanyValues,
	}

	return s.GenerateStructuredAnswer(ctx, chainConfig, params, userID, clientChan)
}

/*
// GenerateInterviewQuestions generates interview questions using structured output
func GenerateInterviewQuestions(
	ctx context.Context,
	s *service.PromptExecutionService,
	request QuestionGeneratorRequest,
	userID string,
	clientChan chan<- event.ProgressEvent,
) (map[string]any, error) {

	chainConfig, err := CreateQuestionGeneratorChain()
	if err != nil {
		return nil, fmt.Errorf("failed to create question generator chain: %w", err)
	}

	params := map[string]any{
		"position":            request.Position,
		"seek_traits":         request.SeekTraits,
		"number_of_questions": request.NumberOfQuestions,
	}

	return s.GenerateStructuredAnswer(ctx, chainConfig, params, userID, clientChan)
}
*/
