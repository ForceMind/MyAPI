package common

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRecoveryRequestFingerprintKnownAnswerAndSecretBinding(t *testing.T) {
	material := TaskRecoveryRequestFingerprintMaterial{
		TokenID:             41,
		HTTPMethod:          "POST",
		OperationKind:       "video.create",
		RouteBinding:        "none",
		ContentFamily:       "application/json",
		CanonicalBodyDigest: sha256.Sum256([]byte("fixture-body-digest-source")),
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	assert.Equal(t, "f56d9513ebcaa26d10283844d6487dd822f69ebfaf275a16135b16f2058a05af", hashTaskRecoveryRequestFingerprintWithKey(key, material))

	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	first, err := HashTaskRecoveryRequestFingerprint(material)
	require.NoError(t, err)
	second, err := HashTaskRecoveryRequestFingerprint(material)
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Len(t, first, sha256.Size*2)

	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("2b", 32))
	rotated, err := HashTaskRecoveryRequestFingerprint(material)
	require.NoError(t, err)
	assert.NotEqual(t, first, rotated)
}

func TestTaskRecoveryRequestFingerprintSeparatesEveryDomainField(t *testing.T) {
	base := TaskRecoveryRequestFingerprintMaterial{
		TokenID:             41,
		HTTPMethod:          "POST",
		OperationKind:       "video.create",
		RouteBinding:        "none",
		ContentFamily:       "application/json",
		CanonicalBodyDigest: sha256.Sum256([]byte("body-one")),
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	baseline := hashTaskRecoveryRequestFingerprintWithKey(key, base)
	mutations := []TaskRecoveryRequestFingerprintMaterial{
		base,
		base,
		base,
		base,
		base,
		base,
	}
	mutations[0].TokenID++
	mutations[1].HTTPMethod = "PATCH"
	mutations[2].OperationKind = "video.remix"
	mutations[3].RouteBinding = "origin-task:task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mutations[4].ContentFamily = "application/cbor"
	mutations[5].CanonicalBodyDigest = sha256.Sum256([]byte("body-two"))
	for _, mutation := range mutations {
		assert.NotEqual(t, baseline, hashTaskRecoveryRequestFingerprintWithKey(key, mutation))
	}
}

func TestTaskRecoveryRequestFingerprintRejectsInvalidMaterialAndSecret(t *testing.T) {
	valid := TaskRecoveryRequestFingerprintMaterial{
		TokenID:             41,
		HTTPMethod:          "POST",
		OperationKind:       "video.create",
		RouteBinding:        "none",
		ContentFamily:       "application/json",
		CanonicalBodyDigest: sha256.Sum256([]byte("body")),
	}
	invalid := valid
	invalid.TokenID = 0
	_, err := HashTaskRecoveryRequestFingerprint(invalid)
	assert.ErrorIs(t, err, ErrTaskRecoveryRequestFingerprint)

	invalid = valid
	invalid.RouteBinding = strings.Repeat("secret-canary", 10)
	_, err = HashTaskRecoveryRequestFingerprint(invalid)
	require.ErrorIs(t, err, ErrTaskRecoveryRequestFingerprint)
	assert.Equal(t, ErrTaskRecoveryRequestFingerprint.Error(), err.Error())
	assert.NotContains(t, err.Error(), "secret-canary")

	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", "missing")
	_, err = HashTaskRecoveryRequestFingerprint(valid)
	assert.ErrorIs(t, err, ErrTaskRecoveryIdempotencySecret)
}
