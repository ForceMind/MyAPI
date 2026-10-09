package model

import (
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/require"
)

func TestValidateTokenSnapshotMatchesCredentialAdmission(t *testing.T) {
	// Status persistence is covered by middleware's database-backed admission
	// tests. Avoid that optional side effect while checking these snapshots.
	previousRedis := common.RedisEnabled
	common.RedisEnabled = true
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	for _, test := range []struct {
		name    string
		token   *Token
		allowed bool
	}{
		{name: "nil"},
		{name: "enabled bounded", token: &Token{Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1}, allowed: true},
		{name: "enabled unlimited", token: &Token{Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}, allowed: true},
		{name: "disabled unlimited", token: &Token{Status: common.TokenStatusDisabled, ExpiredTime: -1, UnlimitedQuota: true}},
		{name: "expired status", token: &Token{Status: common.TokenStatusExpired, ExpiredTime: -1, RemainQuota: 1}},
		{name: "exhausted status", token: &Token{Status: common.TokenStatusExhausted, ExpiredTime: -1, RemainQuota: 1}},
		{name: "expired timestamp", token: &Token{Status: common.TokenStatusEnabled, ExpiredTime: time.Now().Add(-time.Hour).Unix(), RemainQuota: 1}},
		{name: "depleted bounded", token: &Token{Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateTokenSnapshot(test.token)
			if test.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrTokenInvalid)
			}
		})
	}
}
