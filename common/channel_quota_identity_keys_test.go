package common

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func channelQuotaIdentityTestKey(fill byte) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string(fill), 32)))
}

func TestParseChannelQuotaIdentityKeys(t *testing.T) {
	active := channelQuotaIdentityTestKey('a')
	retired := channelQuotaIdentityTestKey('b')
	keyring, err := ParseChannelQuotaIdentityKeys("active:v2:" + active + ",retired:v1:" + retired)
	require.NoError(t, err)
	assert.Equal(t, "v2", keyring.Active.Version)
	assert.Len(t, keyring.Active.Secret, 32)
	require.Len(t, keyring.Retired, 1)
	assert.Equal(t, "v1", keyring.Retired[0].Version)

	t.Setenv(ChannelQuotaIdentityKeysEnv, "active:v2:"+active)
	loaded, err := LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	assert.Equal(t, keyring.Active, loaded.Active)
}

func TestParseChannelQuotaIdentityKeysRejectsInvalidConfigurationWithoutLeakingSecrets(t *testing.T) {
	keyA := channelQuotaIdentityTestKey('a')
	keyB := channelQuotaIdentityTestKey('b')
	keyC := channelQuotaIdentityTestKey('c')
	keyD := channelQuotaIdentityTestKey('d')
	keyE := channelQuotaIdentityTestKey('e')
	keyF := channelQuotaIdentityTestKey('f')
	tests := map[string]string{
		"empty":             "",
		"surrounding space": " active:v1:" + keyA,
		"missing active":    "retired:v1:" + keyA,
		"multiple active":   "active:v1:" + keyA + ",active:v2:" + keyB,
		"too many retired":  "active:v6:" + keyF + ",retired:v1:" + keyA + ",retired:v2:" + keyB + ",retired:v3:" + keyC + ",retired:v4:" + keyD + ",retired:v5:" + keyE,
		"duplicate version": "active:v1:" + keyA + ",retired:v1:" + keyB,
		"duplicate key":     "active:v2:" + keyA + ",retired:v1:" + keyA,
		"unknown role":      "primary:v1:" + keyA,
		"control":           "active:v1:" + keyA + "\n",
		"invalid version":   "active:v/1:" + keyA,
		"uppercase version": "active:V1:" + keyA,
		"short key":         "active:v1:" + base64.RawURLEncoding.EncodeToString([]byte("short")),
		"oversized key":     "active:v1:" + base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 65))),
		"padded key":        "active:v1:" + base64.URLEncoding.EncodeToString([]byte(strings.Repeat("a", 32))),
		"invalid base64":    "active:v1:not+base64",
		"extra field":       "active:v1:" + keyA + ":extra",
		"oversized config":  strings.Repeat("x", 1025),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseChannelQuotaIdentityKeys(raw)
			require.ErrorIs(t, err, ErrChannelQuotaIdentityKeysInvalid)
			assert.NotContains(t, err.Error(), keyA)
			assert.NotContains(t, err.Error(), keyB)
		})
	}
}

func TestChannelQuotaIdentityLookupHMACIsStableAndDomainSeparated(t *testing.T) {
	keyring, err := ParseChannelQuotaIdentityKeys("active:v1:" + channelQuotaIdentityTestKey('a'))
	require.NoError(t, err)
	first, err := ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai", ChannelQuotaIdentityKindCredential, []byte("credential-one"))
	require.NoError(t, err)
	second, err := ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai", ChannelQuotaIdentityKindCredential, []byte("credential-one"))
	require.NoError(t, err)
	account, err := ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai", ChannelQuotaIdentityKindProviderAccount, []byte("credential-one"))
	require.NoError(t, err)
	otherProvider, err := ChannelQuotaIdentityLookupHMAC(keyring.Active, "anthropic", ChannelQuotaIdentityKindCredential, []byte("credential-one"))
	require.NoError(t, err)
	boundaryVariant, err := ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai-credential", ChannelQuotaIdentityKindCredential, []byte("one"))
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Len(t, first, 64)
	assert.NotEqual(t, first, account)
	assert.NotEqual(t, first, otherProvider)
	assert.NotEqual(t, first, boundaryVariant)
	_, err = ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai", ChannelQuotaIdentityKindCredential, nil)
	assert.ErrorIs(t, err, ErrChannelQuotaIdentityInputInvalid)
	_, err = ChannelQuotaIdentityLookupHMAC(keyring.Active, "OpenAI", ChannelQuotaIdentityKindCredential, []byte("credential"))
	assert.ErrorIs(t, err, ErrChannelQuotaIdentityInputInvalid)
	_, err = ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai", ChannelQuotaIdentityKindCredential, make([]byte, MaxChannelQuotaIdentityMaterialBytes+1))
	assert.ErrorIs(t, err, ErrChannelQuotaIdentityInputInvalid)
}

func TestChannelQuotaIdentityLookupHMACDoesNotUseProcessSecrets(t *testing.T) {
	previousSession, previousCrypto := SessionSecret, CryptoSecret
	t.Cleanup(func() { SessionSecret, CryptoSecret = previousSession, previousCrypto })
	keyring, err := ParseChannelQuotaIdentityKeys("active:v1:" + channelQuotaIdentityTestKey('a'))
	require.NoError(t, err)
	before, err := ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai", ChannelQuotaIdentityKindCredential, []byte("credential"))
	require.NoError(t, err)
	SessionSecret = "rotated-session-secret"
	CryptoSecret = "rotated-crypto-secret"
	after, err := ChannelQuotaIdentityLookupHMAC(keyring.Active, "openai", ChannelQuotaIdentityKindCredential, []byte("credential"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestValidateChannelQuotaIdentityKeyringRejectsProgrammaticInvalidValues(t *testing.T) {
	valid := ChannelQuotaIdentityKey{Version: "v1", Secret: []byte(strings.Repeat("a", 32))}
	tests := map[string]ChannelQuotaIdentityKeyring{
		"missing active":    {},
		"uppercase version": {Active: ChannelQuotaIdentityKey{Version: "V1", Secret: valid.Secret}},
		"duplicate version": {Active: valid, Retired: []ChannelQuotaIdentityKey{{Version: "v1", Secret: []byte(strings.Repeat("b", 32))}}},
		"duplicate secret":  {Active: valid, Retired: []ChannelQuotaIdentityKey{{Version: "v2", Secret: valid.Secret}}},
		"oversized key":     {Active: ChannelQuotaIdentityKey{Version: "v1", Secret: make([]byte, 65)}},
	}
	for name, keyring := range tests {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, ValidateChannelQuotaIdentityKeyring(keyring), ErrChannelQuotaIdentityKeysInvalid)
		})
	}

	fingerprint, err := ChannelQuotaIdentityKeyFingerprint(valid)
	require.NoError(t, err)
	assert.Len(t, fingerprint, 64)
	assert.Equal(t, strings.ToLower(fingerprint), fingerprint)
}
