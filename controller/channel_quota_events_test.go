package controller

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
)

func TestChannelQuotaEventsUseActualNativeSamplerIdentity(t *testing.T) {
	db := setupCodexUsageHistoryTestDB(t, 982)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaAlertState{}, &model.ChannelQuotaAlertEvent{}))
	oldEnabled, oldRecovery := common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertNotifyOnRecovery
	common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertNotifyOnRecovery = true, true
	t.Cleanup(func() {
		common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertNotifyOnRecovery = oldEnabled, oldRecovery
	})
	identity := model.ChannelQuotaResolvedIdentity{SubjectRef: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("x", 32))), Quality: model.ChannelQuotaIdentityQualityProviderConfirmed}
	const exhausted = `{"plan_type":"pro","rate_limit":{"allowed":false,"limit_reached":true,"primary_window":{"used_percent":100,"reset_at":1900000000,"limit_window_seconds":18000},"secondary_window":null}}`
	const recovered = `{"plan_type":"pro","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":10,"reset_at":1900000000,"limit_window_seconds":18000},"secondary_window":null}}`
	require.NoError(t, recordQualifiedCodexUsageSnapshotsForIdentity(982, identity, 1700000000, "exhausted", 200, []byte(exhausted), true))
	require.NoError(t, recordQualifiedCodexUsageSnapshotsForIdentity(982, identity, 1700000001, "failed", 503, nil, true))
	require.NoError(t, recordQualifiedCodexUsageSnapshotsForIdentity(982, identity, 1700000002, "recovery", 200, []byte(recovered), true))
	var events []model.ChannelQuotaAlertEvent
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, events, 2)
	require.Equal(t, "exhausted", events[0].Status)
	require.Equal(t, "recovery", events[1].Kind)
	require.Equal(t, events[0].SeriesKey, events[1].SeriesKey)
	evidence := model.ReadChannelQuotaAlertEvidence(context.Background(), events[1])
	require.NotNil(t, evidence)
	require.Equal(t, float64(90), evidence.Available)
	require.Equal(t, int64(1900000000), evidence.ResetAt)
	require.NotContains(t, events[0].EvidenceJSON, identity.SubjectRef)
}

func TestChannelQuotaEventsKeepMixedNativeWindowsSeparate(t *testing.T) {
	db := setupCodexUsageHistoryTestDB(t, 983)
	require.NoError(t, db.AutoMigrate(&model.ChannelQuotaAlertState{}, &model.ChannelQuotaAlertEvent{}))
	oldEnabled, oldRecovery := common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertNotifyOnRecovery
	common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertNotifyOnRecovery = true, true
	t.Cleanup(func() {
		common.ChannelQuotaAlertEnabled, common.ChannelQuotaAlertNotifyOnRecovery = oldEnabled, oldRecovery
	})
	identity := model.ChannelQuotaResolvedIdentity{SubjectRef: base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("z", 32))), Quality: model.ChannelQuotaIdentityQualityProviderConfirmed}
	const low = `{"plan_type":"pro","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":95,"reset_at":1900000000,"limit_window_seconds":18000},"secondary_window":{"used_percent":95,"reset_at":1900600000,"limit_window_seconds":604800}}}`
	const mixed = `{"plan_type":"pro","rate_limit":{"allowed":false,"limit_reached":true,"primary_window":{"used_percent":100,"reset_at":1900000000,"limit_window_seconds":18000},"secondary_window":{"used_percent":10,"reset_at":1900600000,"limit_window_seconds":604800}}}`
	require.NoError(t, recordQualifiedCodexUsageSnapshotsForIdentity(983, identity, 1700000000, "low-windows", 200, []byte(low), true))
	require.NoError(t, recordQualifiedCodexUsageSnapshotsForIdentity(983, identity, 1700000000, "mixed-windows", 200, []byte(mixed), true))
	var events []model.ChannelQuotaAlertEvent
	require.NoError(t, db.Order("id ASC").Find(&events).Error)
	require.Len(t, events, 4, "distinct same-second samples retain each window transition")
	require.Equal(t, "exhausted", events[2].Status)
	require.Equal(t, "five_hour", model.ReadChannelQuotaAlertEvidence(context.Background(), events[2]).WindowType)
	require.Equal(t, "recovery", events[3].Kind)
	require.Equal(t, "weekly", model.ReadChannelQuotaAlertEvidence(context.Background(), events[3]).WindowType)
	require.NotEqual(t, events[2].SeriesKey, events[3].SeriesKey, "weekly recovery cannot recover the exhausted five-hour window")
}
