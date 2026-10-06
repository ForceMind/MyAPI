package model

import (
	"context"
	"math"

	"github.com/ForceMind/MyAPI/common"
)

// ChannelQuotaAlertEvidence contains only the immutable normalized observation
// needed to explain one event. SeriesRef, held separately by the event, isolates
// account + metric + window. It is never a per-Key consumption percentage.
type ChannelQuotaAlertEvidence struct {
	Version       int     `json:"version"`
	SnapshotID    int     `json:"snapshot_id"`
	ChannelID     int     `json:"channel_id"`
	ObservedAt    int64   `json:"observed_at"`
	Available     float64 `json:"available"`
	Total         float64 `json:"total"`
	Unit          string  `json:"unit"`
	Currency      string  `json:"currency,omitempty"`
	MetricType    string  `json:"metric_type"`
	WindowType    string  `json:"window_type"`
	WindowSeconds int64   `json:"window_seconds,omitempty"`
	ResetAt       int64   `json:"reset_at,omitempty"`
}

func channelQuotaAlertEvidence(snapshot *ChannelQuotaSnapshot) ChannelQuotaAlertEvidence {
	return ChannelQuotaAlertEvidence{
		Version: 1, SnapshotID: snapshot.Id, ChannelID: snapshot.ChannelId,
		ObservedAt: snapshot.ObservedAt, Available: snapshot.Available, Total: *snapshot.Total,
		Unit: snapshot.Unit, Currency: snapshot.Currency, MetricType: snapshot.MetricType,
		WindowType: snapshot.WindowType, WindowSeconds: snapshot.WindowSeconds, ResetAt: snapshot.ResetAt,
	}
}

// ReadChannelQuotaAlertEvidence never substitutes a later account observation.
// Old events can use their exact retained source; absent sources stay unknown.
func ReadChannelQuotaAlertEvidence(ctx context.Context, event ChannelQuotaAlertEvent) *ChannelQuotaAlertEvidence {
	var evidence ChannelQuotaAlertEvidence
	if event.EvidenceJSON != "" {
		if len(event.EvidenceJSON) > 2048 || common.UnmarshalJsonStr(event.EvidenceJSON, &evidence) != nil {
			return nil
		}
	} else {
		snapshot, err := GetChannelQuotaAlertDeliverySnapshot(ctx, event)
		if err != nil || snapshot.Status != "success" || snapshot.Total == nil {
			return nil
		}
		evidence = channelQuotaAlertEvidence(snapshot)
	}
	if evidence.Version != 1 || evidence.SnapshotID != event.SnapshotID || evidence.ChannelID != event.ChannelID || evidence.ObservedAt != event.ObservedAt ||
		math.IsNaN(evidence.Available) || math.IsInf(evidence.Available, 0) || math.IsNaN(evidence.Total) || math.IsInf(evidence.Total, 0) || evidence.Total <= 0 {
		return nil
	}
	return &evidence
}
