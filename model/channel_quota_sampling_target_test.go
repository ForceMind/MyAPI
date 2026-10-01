package model

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSyncChannelQuotaSamplingTargetsDeactivatesEmptyCredentialSet(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSamplingTarget{}))
	require.NoError(t, db.Create(&Channel{Id: 7, Key: "fixture"}).Error)
	subject, err := newChannelQuotaIdentitySubjectRef()
	require.NoError(t, err)
	targets, err := SyncChannelQuotaSamplingTargets(context.Background(), db, 7, "fixture", true, []ChannelQuotaSamplingIdentity{{SubjectRef: subject, IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped}})
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.True(t, targets[0].Active)

	targets, err = SyncChannelQuotaSamplingTargets(context.Background(), db, 7, "fixture", true, nil)
	require.NoError(t, err)
	assert.Empty(t, targets)
	var active int64
	require.NoError(t, db.Model(&ChannelQuotaSamplingTarget{}).Where("channel_id = ? AND active = ?", 7, true).Count(&active).Error)
	assert.Zero(t, active)
}

func TestSyncChannelQuotaSamplingTargetsIncompleteDeactivatesUnselectedSubjects(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSamplingTarget{}))
	require.NoError(t, db.Create(&Channel{Id: 8, Key: "fixture"}).Error)
	subjects := make([]string, 3)
	for index := range subjects {
		subjects[index], err = newChannelQuotaIdentitySubjectRef()
		require.NoError(t, err)
		require.NoError(t, EnsureChannelQuotaSamplingTarget(context.Background(), db, 8, ChannelQuotaSamplingIdentity{
			SubjectRef: subjects[index], IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped,
		}))
	}

	targets, err := SyncChannelQuotaSamplingTargets(context.Background(), db, 8, "fixture", false, []ChannelQuotaSamplingIdentity{
		{SubjectRef: subjects[0], IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped},
		{SubjectRef: subjects[1], IdentityQuality: ChannelQuotaIdentityQualityCredentialScoped},
	})
	require.NoError(t, err)
	require.Len(t, targets, 2)
	returned := map[string]bool{targets[0].SubjectRef: true, targets[1].SubjectRef: true}
	assert.True(t, returned[subjects[0]])
	assert.True(t, returned[subjects[1]])
	assert.False(t, returned[subjects[2]])

	var active int64
	require.NoError(t, db.Model(&ChannelQuotaSamplingTarget{}).Where("channel_id = ? AND active = ?", 8, true).Count(&active).Error)
	assert.Equal(t, int64(2), active, "credentials beyond the hard cap remain historical rows but cannot bias active scheduling")
}
