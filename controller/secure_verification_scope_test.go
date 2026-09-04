package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllowedSecurityProofScopesIncludeCodexLocalImport(t *testing.T) {
	for _, scope := range []string{
		securityProofScopeChannelKeyRead,
		securityProofScopeCodexLocalImport,
		securityProofScopePasskeyRegister,
		securityProofScopePasskeyDelete,
	} {
		assert.True(t, isAllowedSecurityProofScope(scope), "scope=%s", scope)
	}
	assert.False(t, isAllowedSecurityProofScope("channel.codex.local_import.other"))
}
