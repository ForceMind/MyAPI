package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strings"
)

const ChannelQuotaIdentityKeysEnv = "CHANNEL_QUOTA_IDENTITY_KEYS"

const (
	ChannelQuotaIdentityKindProviderAccount = "provider_account"
	ChannelQuotaIdentityKindCredential      = "credential"
)

const (
	channelQuotaIdentityKeyRoleActive  = "active"
	channelQuotaIdentityKeyRoleRetired = "retired"
	channelQuotaIdentityDigestDomain   = "my-api/channel-quota-identity/v1"
	channelQuotaIdentityKeyDomain      = "my-api/channel-quota-identity-key-fingerprint/v1"
	maxChannelQuotaIdentityRetiredKeys = 4
	minChannelQuotaIdentityKeyBytes    = sha256.Size
	maxChannelQuotaIdentityKeyBytes    = 64
	maxChannelQuotaIdentityConfigBytes = 1024
)

const MaxChannelQuotaIdentityMaterialBytes = 64 * 1024

var (
	channelQuotaIdentityKeyVersionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)
	channelQuotaIdentityProviderPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

var (
	ErrChannelQuotaIdentityKeysInvalid  = errors.New("channel quota identity key configuration is invalid")
	ErrChannelQuotaIdentityInputInvalid = errors.New("channel quota identity input is invalid")
)

// ChannelQuotaIdentityKey is one HMAC key in the deployment-wide quota
// identity keyring. Secret is intentionally excluded from JSON output.
type ChannelQuotaIdentityKey struct {
	Version string `json:"version"`
	Secret  []byte `json:"-"`
}

// ChannelQuotaIdentityKeyring contains exactly one active key and up to four
// retired keys. Retired keys are lookup-only and allow aliases created before
// a rotation to converge on the same opaque subject.
type ChannelQuotaIdentityKeyring struct {
	Active  ChannelQuotaIdentityKey   `json:"active"`
	Retired []ChannelQuotaIdentityKey `json:"retired,omitempty"`
}

// LoadChannelQuotaIdentityKeyring reads the dedicated cross-node key
// configuration. It never falls back to SessionSecret or CryptoSecret.
func LoadChannelQuotaIdentityKeyring() (ChannelQuotaIdentityKeyring, error) {
	return ParseChannelQuotaIdentityKeys(os.Getenv(ChannelQuotaIdentityKeysEnv))
}

// ParseChannelQuotaIdentityKeys parses a comma-separated keyring. Every entry
// has the exact form "role:version:base64url-key", where role is active or
// retired and the key uses unpadded base64url. Example:
//
//	active:v2:<key>,retired:v1:<key>
//
// Exactly one active entry is required. Version identifiers are 1-32 ASCII
// letters, digits, dots, underscores, or hyphens. Decoded keys must contain at
// least 32 bytes. Errors deliberately omit configuration contents.
func ParseChannelQuotaIdentityKeys(raw string) (ChannelQuotaIdentityKeyring, error) {
	var keyring ChannelQuotaIdentityKeyring
	if raw == "" || len(raw) > maxChannelQuotaIdentityConfigBytes || strings.TrimSpace(raw) != raw || containsControlCharacter(raw) {
		return keyring, ErrChannelQuotaIdentityKeysInvalid
	}

	versions := make(map[string]struct{})
	secrets := make(map[string]struct{})
	activeCount := 0
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.Split(entry, ":")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
		}
		role, version, encoded := parts[0], parts[1], parts[2]
		if !channelQuotaIdentityKeyVersionPattern.MatchString(version) {
			return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
		}
		secret, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(secret) < minChannelQuotaIdentityKeyBytes || len(secret) > maxChannelQuotaIdentityKeyBytes || base64.RawURLEncoding.EncodeToString(secret) != encoded {
			return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
		}
		if _, exists := versions[version]; exists {
			return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
		}
		secretDigest := sha256.Sum256(secret)
		secretFingerprint := hex.EncodeToString(secretDigest[:])
		if _, exists := secrets[secretFingerprint]; exists {
			return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
		}
		versions[version] = struct{}{}
		secrets[secretFingerprint] = struct{}{}
		key := ChannelQuotaIdentityKey{Version: version, Secret: secret}
		switch role {
		case channelQuotaIdentityKeyRoleActive:
			activeCount++
			if activeCount != 1 {
				return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
			}
			keyring.Active = key
		case channelQuotaIdentityKeyRoleRetired:
			keyring.Retired = append(keyring.Retired, key)
			if len(keyring.Retired) > maxChannelQuotaIdentityRetiredKeys {
				return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
			}
		default:
			return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
		}
	}
	if activeCount != 1 {
		return ChannelQuotaIdentityKeyring{}, ErrChannelQuotaIdentityKeysInvalid
	}
	return keyring, nil
}

// ChannelQuotaIdentityLookupHMAC derives a lowercase-hex HMAC over
// length-framed provider, identity kind, and identity material. Length framing
// makes component boundaries unambiguous, while the protocol prefix isolates
// these digests from every other HMAC domain in the process.
func ChannelQuotaIdentityLookupHMAC(key ChannelQuotaIdentityKey, provider, identityKind string, material []byte) (string, error) {
	if !validChannelQuotaIdentityKey(key) || !channelQuotaIdentityProviderPattern.MatchString(provider) ||
		(identityKind != ChannelQuotaIdentityKindProviderAccount && identityKind != ChannelQuotaIdentityKindCredential) ||
		len(material) == 0 || len(material) > MaxChannelQuotaIdentityMaterialBytes {
		return "", ErrChannelQuotaIdentityInputInvalid
	}
	hash := hmac.New(sha256.New, key.Secret)
	writeChannelQuotaIdentityFrame(hash.Write, []byte(channelQuotaIdentityDigestDomain))
	writeChannelQuotaIdentityFrame(hash.Write, []byte(provider))
	writeChannelQuotaIdentityFrame(hash.Write, []byte(identityKind))
	writeChannelQuotaIdentityFrame(hash.Write, material)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ChannelQuotaIdentityKeyFingerprint returns a fixed-domain lowercase-hex
// fingerprint suitable for binding a version to one deployment key. It is not
// used as the identity lookup digest and cannot replace the secret key.
func ChannelQuotaIdentityKeyFingerprint(key ChannelQuotaIdentityKey) (string, error) {
	if !validChannelQuotaIdentityKey(key) {
		return "", ErrChannelQuotaIdentityKeysInvalid
	}
	hash := sha256.New()
	writeChannelQuotaIdentityFrame(hash.Write, []byte(channelQuotaIdentityKeyDomain))
	writeChannelQuotaIdentityFrame(hash.Write, key.Secret)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeChannelQuotaIdentityFrame(write func([]byte) (int, error), value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = write(length[:])
	_, _ = write(value)
}

func containsControlCharacter(value string) bool {
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}

func validChannelQuotaIdentityKey(key ChannelQuotaIdentityKey) bool {
	return channelQuotaIdentityKeyVersionPattern.MatchString(key.Version) &&
		len(key.Secret) >= minChannelQuotaIdentityKeyBytes && len(key.Secret) <= maxChannelQuotaIdentityKeyBytes
}

// ValidateChannelQuotaIdentityKeyring checks programmatically constructed
// keyrings with the same invariants as the environment parser.
func ValidateChannelQuotaIdentityKeyring(keyring ChannelQuotaIdentityKeyring) error {
	if !validChannelQuotaIdentityKey(keyring.Active) || len(keyring.Retired) > maxChannelQuotaIdentityRetiredKeys {
		return ErrChannelQuotaIdentityKeysInvalid
	}
	versions := make(map[string]struct{}, 1+len(keyring.Retired))
	fingerprints := make(map[string]struct{}, 1+len(keyring.Retired))
	keys := append([]ChannelQuotaIdentityKey{keyring.Active}, keyring.Retired...)
	for _, key := range keys {
		if !validChannelQuotaIdentityKey(key) {
			return ErrChannelQuotaIdentityKeysInvalid
		}
		if _, exists := versions[key.Version]; exists {
			return ErrChannelQuotaIdentityKeysInvalid
		}
		fingerprint, err := ChannelQuotaIdentityKeyFingerprint(key)
		if err != nil {
			return ErrChannelQuotaIdentityKeysInvalid
		}
		if _, exists := fingerprints[fingerprint]; exists {
			return ErrChannelQuotaIdentityKeysInvalid
		}
		versions[key.Version] = struct{}{}
		fingerprints[fingerprint] = struct{}{}
	}
	return nil
}
