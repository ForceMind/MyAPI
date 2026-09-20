package model

import (
	"context"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelQuotaAlertOutboxPersistsThresholdAndRecovery(t *testing.T) {
	previousDB := DB
	previousSettings := channelQuotaAlertSettings()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}, &ChannelQuotaAlertState{}, &ChannelQuotaAlertEvent{}))
	DB = db
	require.NoError(t, db.Create(&Channel{Id: 17}).Error)
	common.ChannelQuotaAlertEnabled = true
	common.ChannelQuotaAlertWarningPercent = 20
	common.ChannelQuotaAlertCriticalPercent = 10
	common.ChannelQuotaAlertCooldownSeconds = 3600
	common.ChannelQuotaAlertNotifyOnRecovery = true
	t.Cleanup(func() {
		DB = previousDB
		common.ChannelQuotaAlertEnabled = previousSettings.Enabled
		common.ChannelQuotaAlertWarningPercent = previousSettings.WarningPercent
		common.ChannelQuotaAlertCriticalPercent = previousSettings.CriticalPercent
		common.ChannelQuotaAlertCooldownSeconds = previousSettings.CooldownSeconds
		common.ChannelQuotaAlertNotifyOnRecovery = previousSettings.NotifyOnRecovery
	})

	total := 100.0
	ref := ChannelQuotaAccountRef("codex", "account-a")
	batch := func(sampleID string, observedAt int64, available float64) error {
		return RecordChannelQuotaSnapshotBatchWithContext(context.Background(), []ChannelQuotaSnapshot{{
			ChannelId: 17, AccountRef: ref, ObservedAt: observedAt, SampleID: sampleID,
			Available: available, Total: &total, MetricType: "codex_rate_limit", WindowType: "five_hour",
			Source: "codex_wham_usage_primary", Unit: "percent", WindowSeconds: 18000, Status: "success",
		}}, ChannelQuotaSnapshotBatchOptions{})
	}

	require.NoError(t, batch("first", 100, 15))
	require.NoError(t, batch("same-state", 101, 14))
	require.NoError(t, batch("recovery", 102, 80))

	var events []ChannelQuotaAlertEvent
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, "warning", events[0].Status)
	require.Equal(t, "threshold", events[0].Kind)
	require.Equal(t, channelQuotaAlertEventStatePending, events[0].State)
	require.Equal(t, "healthy", events[1].Status)
	require.Equal(t, "recovery", events[1].Kind)

	var state ChannelQuotaAlertState
	require.NoError(t, db.Where("channel_id = ?", 17).First(&state).Error)
	require.Equal(t, "healthy", state.CurrentStatus)
	require.Equal(t, int64(102), state.ObservedAt)
}

func TestChannelQuotaAlertOutboxSeparatesAccountSeries(t *testing.T) {
	previousDB := DB
	previousSettings := channelQuotaAlertSettings()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &ChannelQuotaSnapshot{}, &ChannelQuotaAlertState{}, &ChannelQuotaAlertEvent{}))
	DB = db
	require.NoError(t, db.Create(&Channel{Id: 18}).Error)
	common.ChannelQuotaAlertEnabled = true
	common.ChannelQuotaAlertWarningPercent = 20
	common.ChannelQuotaAlertCriticalPercent = 10
	common.ChannelQuotaAlertCooldownSeconds = 3600
	common.ChannelQuotaAlertNotifyOnRecovery = false
	t.Cleanup(func() {
		DB = previousDB
		common.ChannelQuotaAlertEnabled = previousSettings.Enabled
		common.ChannelQuotaAlertWarningPercent = previousSettings.WarningPercent
		common.ChannelQuotaAlertCriticalPercent = previousSettings.CriticalPercent
		common.ChannelQuotaAlertCooldownSeconds = previousSettings.CooldownSeconds
		common.ChannelQuotaAlertNotifyOnRecovery = previousSettings.NotifyOnRecovery
	})

	total := 100.0
	for index, account := range []string{"account-a", "account-b"} {
		require.NoError(t, RecordChannelQuotaSnapshotBatchWithContext(context.Background(), []ChannelQuotaSnapshot{{
			ChannelId: 18, AccountRef: ChannelQuotaAccountRef("codex", account), ObservedAt: int64(100 + index), SampleID: "sample-" + account,
			Available: 5, Total: &total, MetricType: "codex_rate_limit", WindowType: "five_hour", Source: "codex_wham_usage_primary", Unit: "percent", WindowSeconds: 18000, Status: "success",
		}}, ChannelQuotaSnapshotBatchOptions{}))
	}
	var states []ChannelQuotaAlertState
	var events []ChannelQuotaAlertEvent
	require.NoError(t, db.Order("id ASC").Find(&states).Error)
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, states, 2)
	require.Len(t, events, 2)
	require.NotEqual(t, states[0].SeriesKey, states[1].SeriesKey)
}

func TestChannelQuotaAlertDeliveryQueryAndSnapshotStayRedactedAndScoped(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ChannelQuotaSnapshot{}, &ChannelQuotaAlertEvent{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	total := 100.0
	snapshot := ChannelQuotaSnapshot{
		ChannelId: 27, AccountRef: ChannelQuotaAccountRef("codex", "account-secret"),
		ObservedAt: 1000, Available: 5, Total: &total, MetricType: "codex_rate_limit",
		WindowType: "five_hour", Source: "codex_wham_usage_primary", Unit: "percent", Status: "success",
	}
	require.NoError(t, db.Create(&snapshot).Error)
	event := ChannelQuotaAlertEvent{
		EventKey: "quota-alert-occurrence-v2:event", SeriesKey: channelQuotaAlertSeriesKey(&snapshot),
		SnapshotID: snapshot.Id, ChannelID: snapshot.ChannelId, Status: "critical", Kind: "threshold",
		State: channelQuotaAlertEventStatePending, ObservedAt: 1000, CreatedAt: 1000, UpdatedAt: 1000, LockVersion: 1,
	}
	require.NoError(t, db.Create(&event).Error)

	events, count, err := ListChannelQuotaAlertEvents(context.Background(), ChannelQuotaAlertEventFilter{
		State: "pending", Status: "critical", Kind: "threshold", ChannelID: 27,
	}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Len(t, events, 1)
	require.Empty(t, events[0].ClaimedBy)

	loaded, err := GetChannelQuotaAlertDeliverySnapshot(context.Background(), events[0])
	require.NoError(t, err)
	require.Equal(t, snapshot.Id, loaded.Id)
	require.Equal(t, snapshot.AccountRef, loaded.AccountRef)

	tampered := events[0]
	tampered.ChannelID++
	_, err = GetChannelQuotaAlertDeliverySnapshot(context.Background(), tampered)
	require.Error(t, err)
}

func TestListChannelQuotaAlertEventsRejectsUnboundedOrUnknownFilters(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previousDB })

	_, _, err = ListChannelQuotaAlertEvents(context.Background(), ChannelQuotaAlertEventFilter{State: "unknown"}, 0, 10)
	require.Error(t, err)
	_, _, err = ListChannelQuotaAlertEvents(context.Background(), ChannelQuotaAlertEventFilter{}, 0, 101)
	require.Error(t, err)
}
