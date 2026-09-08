package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"strconv"
)

const taskRecoveryRequestFingerprintVersion = "myapi-task-recovery-request-fingerprint-v1"

var ErrTaskRecoveryRequestFingerprint = errors.New("task recovery request fingerprint input is invalid")

// TaskRecoveryRequestFingerprintMaterial is the normalized, non-secret scope
// of one submission request. It intentionally excludes the raw idempotency key,
// client/network identity, routing choice, model, price, and time.
type TaskRecoveryRequestFingerprintMaterial struct {
	TokenID             int64
	HTTPMethod          string
	OperationKind       string
	RouteBinding        string
	ContentFamily       string
	CanonicalBodyDigest [sha256.Size]byte
}

// HashTaskRecoveryRequestFingerprint binds normalized request material to the
// deployment's dedicated, restart-stable task-recovery secret. Every field is
// length-framed under a versioned domain and the result is lowercase HMAC-SHA256
// hex. The canonical body itself is never accepted or returned here.
func HashTaskRecoveryRequestFingerprint(material TaskRecoveryRequestFingerprintMaterial) (string, error) {
	if !validTaskRecoveryRequestFingerprintMaterial(material) {
		return "", ErrTaskRecoveryRequestFingerprint
	}
	key, err := taskRecoveryIdempotencySecret()
	if err != nil {
		return "", err
	}
	return hashTaskRecoveryRequestFingerprintWithKey(key, material), nil
}

func hashTaskRecoveryRequestFingerprintWithKey(key []byte, material TaskRecoveryRequestFingerprintMaterial) string {
	hasher := hmac.New(sha256.New, key)
	writeTaskRecoveryRequestFingerprintFrame(hasher, []byte(taskRecoveryRequestFingerprintVersion))
	writeTaskRecoveryRequestFingerprintFrame(hasher, []byte(strconv.FormatInt(material.TokenID, 10)))
	writeTaskRecoveryRequestFingerprintFrame(hasher, []byte(material.HTTPMethod))
	writeTaskRecoveryRequestFingerprintFrame(hasher, []byte(material.OperationKind))
	writeTaskRecoveryRequestFingerprintFrame(hasher, []byte(material.RouteBinding))
	writeTaskRecoveryRequestFingerprintFrame(hasher, []byte(material.ContentFamily))
	writeTaskRecoveryRequestFingerprintFrame(hasher, material.CanonicalBodyDigest[:])
	return hex.EncodeToString(hasher.Sum(nil))
}

func validTaskRecoveryRequestFingerprintMaterial(material TaskRecoveryRequestFingerprintMaterial) bool {
	return material.TokenID > 0 && material.TokenID <= 1<<31-1 &&
		len(material.HTTPMethod) > 0 && len(material.HTTPMethod) <= 16 &&
		len(material.OperationKind) > 0 && len(material.OperationKind) <= 48 &&
		len(material.RouteBinding) > 0 && len(material.RouteBinding) <= 64 &&
		len(material.ContentFamily) > 0 && len(material.ContentFamily) <= 64
}

func writeTaskRecoveryRequestFingerprintFrame(hasher hash.Hash, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	hasher.Write(length[:])
	hasher.Write(value)
}
