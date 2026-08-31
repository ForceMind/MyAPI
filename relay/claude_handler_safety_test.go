package relay

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/model_setting"
	"github.com/stretchr/testify/require"
)

func TestValidateClaudeMaxTokens(t *testing.T) {
	require.NoError(t, validateClaudeMaxTokens(uint(model_setting.ClaudeMaxTokensLimit)))
	require.Error(t, validateClaudeMaxTokens(uint(model_setting.ClaudeMaxTokensLimit)+1))
}
