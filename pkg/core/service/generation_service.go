package service

import (
	"context"

	"ctoup.com/coreapp/pkg/core/service/gochains"
	"ctoup.com/coreapp/pkg/shared/event"

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

type QuestionGeneratorRequest struct {
	Position, SeekTraits string
	NumberOfQuestions    int
}

type SkillGeneratorRequest struct {
	Position       string
	JobDescription string
	CompanyValues  string // Add company values field
}

func (s *PromptExecutionService) GenerateAnswer(ctx context.Context, chainConfig *gochains.BaseChain, params map[string]any, userID string, clientChan chan<- event.ProgressEvent) (string, error) {

	chain := chains.LLMChain{
		Prompt: prompts.NewPromptTemplate(
			chainConfig.GetTemplateText(),
			chainConfig.GetParamDefinition(),
		),
		LLM:          chainConfig.GetModel(),
		Memory:       memory.NewSimple(),
		OutputParser: chainConfig.GetOutputParser(),
		OutputKey:    _llmChainDefaultOutputKey,
	}

	res, err := chains.Call(ctx, chain, params, chains.WithMaxTokens(chainConfig.GetMaxTokens()),
		chains.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			clientChan <- event.NewProgressEvent("MSG",
				string(chunk), 50)
			return nil
		}))

	if err != nil {
		return "", err
	}

	return res["text"].(string), nil
}
