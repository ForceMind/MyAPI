package common

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRecoveryEnabledDefaultsOffWhenEnvironmentIsAbsent(t *testing.T) {
	t.Setenv("TASK_RECOVERY_ENABLED", "")
	t.Setenv("TASK_RECOVERY_NEW_SUBMISSIONS_ENABLED", "")
	t.Setenv("TASK_RECOVERY_OBLIGATION_RECOVERY_ENABLED", "")
	assert.False(t, taskRecoveryEnabledEnv())
	newSubmissions, obligationRecovery := taskRecoveryGatesEnv()
	assert.False(t, newSubmissions)
	assert.False(t, obligationRecovery)
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
			t.Setenv("TASK_RECOVERY_NEW_SUBMISSIONS_ENABLED", "")
			t.Setenv("TASK_RECOVERY_OBLIGATION_RECOVERY_ENABLED", "")
			assert.Equal(t, testCase.expected, taskRecoveryEnabledEnv())
			newSubmissions, obligationRecovery := taskRecoveryGatesEnv()
			assert.Equal(t, testCase.expected, newSubmissions)
			assert.Equal(t, testCase.expected, obligationRecovery)
		})
	}
}

func TestTaskRecoverySplitGatesKeepExistingObligationsIndependent(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	for _, testCase := range []struct {
		name                string
		legacy              string
		newSubmissions      string
		obligationRecovery  string
		expectedNew         bool
		expectedObligations bool
	}{
		{"legacy-maps-to-both", "true", "", "", true, true},
		{"new-submissions-only", "", "true", "false", true, false},
		{"recover-existing-only", "", "false", "true", false, true},
		{"explicit-split-overrides-legacy", "true", "false", "true", false, true},
		{"new-flags-disable-legacy", "true", "false", "false", false, false},
		{"explicit-off", "", "false", "false", false, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_ENABLED", testCase.legacy)
			t.Setenv("TASK_RECOVERY_NEW_SUBMISSIONS_ENABLED", testCase.newSubmissions)
			t.Setenv("TASK_RECOVERY_OBLIGATION_RECOVERY_ENABLED", testCase.obligationRecovery)
			newSubmissions, obligationRecovery := taskRecoveryGatesEnv()
			assert.Equal(t, testCase.expectedNew, newSubmissions)
			assert.Equal(t, testCase.expectedObligations, obligationRecovery)
			assert.Equal(t, testCase.expectedNew || testCase.expectedObligations, TaskRecoveryDeploymentRequested())
		})
	}
}

func TestTaskRecoverySplitGateRejectsMalformedValues(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	for _, name := range []string{
		"TASK_RECOVERY_ENABLED",
		"TASK_RECOVERY_NEW_SUBMISSIONS_ENABLED",
		"TASK_RECOVERY_OBLIGATION_RECOVERY_ENABLED",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_ENABLED", "")
			t.Setenv("TASK_RECOVERY_NEW_SUBMISSIONS_ENABLED", "")
			t.Setenv("TASK_RECOVERY_OBLIGATION_RECOVERY_ENABLED", "")
			t.Setenv(name, "TRUE")
			newSubmissions, obligationRecovery := taskRecoveryGatesEnv()
			assert.False(t, newSubmissions)
			assert.False(t, obligationRecovery)
			assert.False(t, TaskRecoveryDeploymentRequested())
		})
	}
}

func TestTaskRecoveryEnabledRequiresDedicatedSecret(t *testing.T) {
	t.Setenv("TASK_RECOVERY_ENABLED", "")
	t.Setenv("TASK_RECOVERY_NEW_SUBMISSIONS_ENABLED", "false")
	t.Setenv("TASK_RECOVERY_OBLIGATION_RECOVERY_ENABLED", "true")
	for _, raw := range []string{"", "random_string", strings.Repeat("a", 63), strings.Repeat("x", 64), " " + strings.Repeat("1a", 32)} {
		t.Run("invalid-secret-"+strconv.Itoa(len(raw)), func(t *testing.T) {
			t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", raw)
			newSubmissions, obligationRecovery := taskRecoveryGatesEnv()
			assert.False(t, newSubmissions)
			assert.False(t, obligationRecovery)
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
	previousLegacy := TaskRecoveryEnabled
	previousNew := TaskRecoveryNewSubmissionsEnabled
	previousRecovery := TaskRecoveryObligationRecoveryEnabled
	t.Cleanup(func() {
		TaskRecoveryEnabled = previousLegacy
		TaskRecoveryNewSubmissionsEnabled = previousNew
		TaskRecoveryObligationRecoveryEnabled = previousRecovery
	})
	TaskRecoveryEnabled = false
	TaskRecoveryNewSubmissionsEnabled = false
	TaskRecoveryObligationRecoveryEnabled = false
	assert.False(t, IsTaskRecoveryEnabled())
	assert.False(t, IsTaskRecoveryNewSubmissionEnabled())
	assert.False(t, IsTaskRecoveryObligationRecoveryEnabled())
	assert.False(t, IsTaskRecoveryIdentityRequired())
	TaskRecoveryEnabled = true
	TaskRecoveryNewSubmissionsEnabled = true
	TaskRecoveryObligationRecoveryEnabled = true
	assert.True(t, IsTaskRecoveryEnabled())
	assert.True(t, IsTaskRecoveryNewSubmissionEnabled())
	assert.True(t, IsTaskRecoveryObligationRecoveryEnabled())
	assert.True(t, IsTaskRecoveryIdentityRequired())
	assert.True(t, IsTaskRecoverySchemaCompatibilityEnabled())
}
