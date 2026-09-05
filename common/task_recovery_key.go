package common

import (
	"encoding/hex"
	"errors"
	"os"
)

var ErrTaskRecoveryIdempotencySecret = errors.New("task recovery requires a dedicated 32-byte hex idempotency secret")

// taskRecoveryIdempotencySecret has no generated or session-secret fallback.
// This deployment setting must remain identical across nodes and restarts.
// V1 does not support changing it while the database retains its identity.
func taskRecoveryIdempotencySecret() ([]byte, error) {
	raw := os.Getenv("TASK_RECOVERY_IDEMPOTENCY_SECRET")
	if len(raw) != 64 {
		return nil, ErrTaskRecoveryIdempotencySecret
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return nil, ErrTaskRecoveryIdempotencySecret
	}
	return key, nil
}

// HashTaskRecoveryIdempotencyKey never persists or returns the raw client key.
// The protocol boundary is responsible for validating the HTTP header syntax.
func HashTaskRecoveryIdempotencyKey(rawKey string) (string, error) {
	if rawKey == "" {
		return "", errors.New("task submission idempotency key is empty")
	}
	key, err := taskRecoveryIdempotencySecret()
	if err != nil {
		return "", err
	}
	return GenerateHMACWithKey(key, "task-submission-idempotency-v1:"+rawKey), nil
}

// TaskRecoveryIdempotencyKeyVerifier binds a database to one deployment key
// without storing the key. A separate domain prevents its use as a client-key
// digest. It is an internal configuration verifier, not an authentication token.
func TaskRecoveryIdempotencyKeyVerifier() (string, error) {
	key, err := taskRecoveryIdempotencySecret()
	if err != nil {
		return "", err
	}
	return GenerateHMACWithKey(key, "myapi-task-recovery-database-identity-v1"), nil
}
