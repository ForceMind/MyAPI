package common

import "testing"

func TestChannelQuotaSnapshotRetentionDaysEnv(t *testing.T) {
	t.Setenv("CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS", "")
	if got := channelQuotaSnapshotRetentionDaysEnv(); got != DefaultChannelQuotaSnapshotRetentionDays {
		t.Fatalf("default retention days = %d, want %d", got, DefaultChannelQuotaSnapshotRetentionDays)
	}
	t.Setenv("CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS", "30")
	if got := channelQuotaSnapshotRetentionDaysEnv(); got != 30 {
		t.Fatalf("configured retention days = %d, want 30", got)
	}
	for _, value := range []string{"-1", "36501"} {
		t.Setenv("CHANNEL_QUOTA_SNAPSHOT_RETENTION_DAYS", value)
		if got := channelQuotaSnapshotRetentionDaysEnv(); got != DefaultChannelQuotaSnapshotRetentionDays {
			t.Fatalf("invalid retention %q = %d, want default %d", value, got, DefaultChannelQuotaSnapshotRetentionDays)
		}
	}
}
