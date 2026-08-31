package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeRequestEffectiveMaxTokensUsesBothCompatibleFields(t *testing.T) {
	maxTokens := uint(128)
	legacy := uint(4096)
	request := &ClaudeRequest{MaxTokens: &maxTokens, MaxTokensToSample: &legacy}

	assert.Equal(t, uint(4096), request.EffectiveMaxTokens())
	meta := request.GetTokenCountMeta()
	require.NotNil(t, meta)
	assert.Equal(t, 4096, meta.MaxTokens)
}

func TestClaudeRequestEffectiveMaxTokensUsesMaxTokensWhenGreater(t *testing.T) {
	maxTokens := uint(4096)
	legacy := uint(128)
	request := &ClaudeRequest{MaxTokens: &maxTokens, MaxTokensToSample: &legacy}

	assert.Equal(t, uint(4096), request.EffectiveMaxTokens())
	assert.Equal(t, 4096, request.GetTokenCountMeta().MaxTokens)
}
