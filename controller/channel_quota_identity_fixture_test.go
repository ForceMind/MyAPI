package controller

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelQuotaIdentityFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("q", 32)))
	t.Setenv(common.ChannelQuotaIdentityKeysEnv, "active:v1:"+secret)
	require.NoError(t, db.AutoMigrate(
		&model.ChannelQuotaIdentityKeyRegistry{},
		&model.ChannelQuotaIdentityKeyVersion{},
		&model.ChannelQuotaIdentityAlias{},
		&model.ChannelQuotaSamplingTarget{},
	))
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	require.NoError(t, err)
	require.NoError(t, model.EnsureChannelQuotaIdentityKeyring(context.Background(), db, keyring))
}
