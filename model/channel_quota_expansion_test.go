package model

import (
	"context"
	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestQuotaExpansionCursorIsAtomicAndNotOverwrittenByChannelSave(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelQuotaSamplingTarget{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	channel := Channel{Key: "synthetic", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	first, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	second, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	_, err = SyncChannelQuotaSamplingTargets(context.Background(), db, channel.Id, channel.Key, false,
		[]ChannelQuotaSamplingIdentity{{SubjectRef: first, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped}},
		ChannelQuotaSamplingExpansion{ExpectedCursor: 0, NextCursor: 32})
	require.NoError(t, err)
	_, err = SyncChannelQuotaSamplingTargets(context.Background(), db, channel.Id, channel.Key, false,
		[]ChannelQuotaSamplingIdentity{{SubjectRef: second, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped}},
		ChannelQuotaSamplingExpansion{ExpectedCursor: 0, NextCursor: 64})
	require.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&ChannelQuotaSamplingTarget{}).Where("subject_ref = ?", second).Count(&count).Error)
	assert.Zero(t, count, "stale expansion must not publish targets")
	require.NoError(t, channel.Save())
	var saved Channel
	require.NoError(t, db.First(&saved, channel.Id).Error)
	assert.EqualValues(t, 32, saved.QuotaSamplingCursor, "saving a stale channel must preserve the managed cursor")
	encoded, err := common.Marshal(saved)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "quota_sampling_cursor")
}
