package gochains

import (
	"github.com/tmc/langchaingo/schema"
)

// ChainFactory creates different types of chains
type ChainFactory struct {
	memory schema.Memory
}

func NewChainFactory(memory schema.Memory) *ChainFactory {
	return &ChainFactory{
		memory: memory,
	}
}
