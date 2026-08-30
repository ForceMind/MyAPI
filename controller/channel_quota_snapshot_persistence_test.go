package controller

import (
	"encoding/json"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaSnapshotSyncSummaryIncludesPersistenceFailures(t *testing.T) {
	payload, err := json.Marshal(channelQuotaSnapshotSyncSummary{
		Considered:    3,
		Sampled:       2,
		Failed:        1,
		PersistFailed: 2,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"considered":3,"sampled":2,"failed":1,"skipped":0,"persist_failed":2}`, string(payload))
}

func TestChannelQuotaSnapshotSyncSummaryIncludesUnsupportedCount(t *testing.T) {
	payload, err := json.Marshal(channelQuotaSnapshotSyncSummary{
		Considered:  2,
		Failed:      1,
		Unsupported: 1,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"considered":2,"sampled":0,"failed":1,"unsupported":1,"skipped":0,"persist_failed":0}`, string(payload))
}

func TestRecordChannelBalanceSnapshotReturnsPersistenceError(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	// Intentionally do not migrate channel_quota_snapshots. The helper must
	// surface the write failure to scheduled sampling instead of silently
	// dropping the observation.
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	err = recordChannelBalanceSnapshot(&model.Channel{Id: 913, Type: 1}, channelBalanceResult{Balance: 2.5}, nil)
	require.Error(t, err)
}

func TestRecordChannelBalanceSnapshotRecordsProviderFailure(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	err = recordChannelBalanceSnapshot(
		&model.Channel{Id: 914, Type: 1},
		channelBalanceResult{},
		gorm.ErrInvalidDB,
	)
	require.NoError(t, err)

	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", 914).First(&snapshot).Error)
	require.Equal(t, "error", snapshot.Status)
	require.Equal(t, "query_failed", snapshot.ErrorCode)
}

func TestRecordChannelBalanceSnapshotRecordsUnsupportedProvider(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaSnapshot{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	err = recordChannelBalanceSnapshot(
		&model.Channel{Id: 915, Type: constant.ChannelTypeAnthropic},
		channelBalanceResult{},
		errChannelQuotaUnsupported,
	)
	require.NoError(t, err)

	var snapshot model.ChannelQuotaSnapshot
	require.NoError(t, db.Where("channel_id = ?", 915).First(&snapshot).Error)
	require.Equal(t, "unsupported", snapshot.Status)
	require.Equal(t, "quota_unsupported", snapshot.ErrorCode)
}
