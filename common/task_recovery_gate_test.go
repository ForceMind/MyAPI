package common

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRecoveryEnabledDefaultsOffWhenEnvironmentIsAbsent(t *testing.T) {
	previous, wasSet := os.LookupEnv("TASK_RECOVERY_ENABLED")
	require.NoError(t, os.Unsetenv("TASK_RECOVERY_ENABLED"))
	t.Cleanup(func() {
		if wasSet {
			require.NoError(t, os.Setenv("TASK_RECOVERY_ENABLED", previous))
			return
		}
		require.NoError(t, os.Unsetenv("TASK_RECOVERY_ENABLED"))
	})
	assert.False(t, taskRecoveryEnabledEnv())
}

func TestTaskRecoveryEnabledEnvironmentIsStrictAndFailClosed(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	for _, testCase := range []struct {
		name     string
		raw      string
		expected bool
	}{
		{"empty", "", false},
		{"explicit-false", "false", false},
		{"explicit-true", "true", true},
		{"numeric-true-rejected", "1", false},
		{"short-true-rejected", "t", false},
		{"uppercase-true-rejected", "TRUE", false},
		{"whitespace-true-rejected", " true ", false},
		{"arbitrary-value-rejected", "yes", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_ENABLED", testCase.raw)
			assert.Equal(t, testCase.expected, taskRecoveryEnabledEnv())
		})
	}
}

func TestTaskRecoveryEnabledRequiresDedicatedSecret(t *testing.T) {
	t.Setenv("TASK_RECOVERY_ENABLED", "true")
	for _, raw := range []string{"", "random_string", strings.Repeat("a", 63), strings.Repeat("x", 64), " " + strings.Repeat("1a", 32)} {
		t.Run("invalid-secret-"+strconv.Itoa(len(raw)), func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", raw)
			assert.False(t, taskRecoveryEnabledEnv())
			_, err := HashTaskRecoveryIdempotencyKey("fixture-client-key")
			require.ErrorIs(t, err, ErrTaskRecoveryIdempotencySecret)
		})
	}
}

func TestTaskRecoveryKeyStableAcrossSessionSecretRotation(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	previousSession, previousCrypto := SessionSecret, CryptoSecret
	t.Cleanup(func() { SessionSecret, CryptoSecret = previousSession, previousCrypto })

	first, err := HashTaskRecoveryIdempotencyKey("fixture-client-key")
	require.NoError(t, err)
	firstVerifier, err := TaskRecoveryIdempotencyKeyVerifier()
	require.NoError(t, err)
	SessionSecret, CryptoSecret = "next-process-session", "next-process-crypto"
	second, err := HashTaskRecoveryIdempotencyKey("fixture-client-key")
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.NotEqual(t, first, firstVerifier)

	// Hex spelling is not part of the key identity.
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1A", 32))
	sameVerifier, err := TaskRecoveryIdempotencyKeyVerifier()
	require.NoError(t, err)
	assert.Equal(t, firstVerifier, sameVerifier)
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("2b", 32))
	otherVerifier, err := TaskRecoveryIdempotencyKeyVerifier()
	require.NoError(t, err)
	assert.NotEqual(t, firstVerifier, otherVerifier)
}

func TestTaskRecoveryEnabledAccessor(t *testing.T) {
	previous := TaskRecoveryEnabled
	t.Cleanup(func() { TaskRecoveryEnabled = previous })
	TaskRecoveryEnabled = false
	assert.False(t, IsTaskRecoveryEnabled())
	TaskRecoveryEnabled = true
	assert.True(t, IsTaskRecoveryEnabled())
}
