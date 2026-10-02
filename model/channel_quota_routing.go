package model

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

const (
	CodexQuotaRouteLimitSource      = "codex_relay_usage_limit"
	CodexQuotaRouteLimitCode        = "usage_limit_reached"
	codexQuotaWindowPrimarySource   = "codex_wham_usage_primary"
	codexQuotaWindowSecondarySource = "codex_wham_usage_secondary"
)

var ErrCodexQuotaRouteIdentity = errors.New("Codex quota routing identity is unavailable")

type CodexQuotaRouteState struct {
	ReasonCode string
	Blocked    bool
	ResetAt    int64
	ObservedAt int64
	Source     string
}

// HasVersionedCodexQuotaEvidence prevents a missing deployment keyring from
// turning unreadable confirmed account evidence into an empty legacy lookup.
func HasVersionedCodexQuotaEvidence(ctx context.Context, db *gorm.DB) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var matches []struct{ ID int }
	err := db.WithContext(ctx).Model(&ChannelQuotaSnapshot{}).Select("id").
		Where("metric_type = ? AND identity_quality = ? AND subject_ref <> ?", "codex_rate_limit", ChannelQuotaIdentityQualityProviderConfirmed, "").
		Limit(1).Find(&matches).Error
	return len(matches) > 0, err
}

// Retention keeps the latest account facts until newer observations replace them.
func protectedCodexRoutingEvidenceQuery(db *gorm.DB) *gorm.DB {
	return db.Table("channel_quota_snapshots AS held").Select("held.id").
		Where("held.metric_type = ? AND ((held.source IN ? AND held.status = ?) OR (held.source IN ? AND held.status = ? AND held.error_code = ?) OR (held.source = ? AND held.status = ? AND held.error_code = ?))",
			"codex_rate_limit", []string{codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource}, "success",
			[]string{codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource}, "unsupported", "window_absent",
			CodexQuotaRouteLimitSource, "error", CodexQuotaRouteLimitCode).
		Where(`NOT EXISTS (
			SELECT 1 FROM channel_quota_snapshots AS newer
			WHERE newer.metric_type = held.metric_type AND newer.source = held.source AND newer.status = held.status
				AND (held.status = ? OR COALESCE(newer.error_code, '') = COALESCE(held.error_code, ''))
				AND COALESCE(newer.subject_ref, '') = COALESCE(held.subject_ref, '')
				AND COALESCE(newer.identity_quality, '') = COALESCE(held.identity_quality, '')
				AND COALESCE(newer.account_ref, '') = COALESCE(held.account_ref, '')
				AND (newer.observed_at > held.observed_at OR (newer.observed_at = held.observed_at AND newer.id > held.id))
		)`, "success")
}

// ReadCodexQuotaRouteState uses confirmed account observations and exact limit markers.
// Read errors are returned to routing callers for a fail-closed decision.
func ReadCodexQuotaRouteState(ctx context.Context, db *gorm.DB, subjectRef, legacyAccountRef string, now int64) (CodexQuotaRouteState, error) {
	if db == nil || db.Dialector == nil {
		return CodexQuotaRouteState{}, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return CodexQuotaRouteState{}, err
	}
	if now <= 0 || subjectRef != "" && !canonicalChannelQuotaIdentitySubject(subjectRef) ||
		legacyAccountRef != "" && !canonicalLowerHex(legacyAccountRef, 64) ||
		subjectRef == "" && legacyAccountRef == "" {
		return CodexQuotaRouteState{}, ErrCodexQuotaRouteIdentity
	}

	identityQuery := func() *gorm.DB {
		query := db.WithContext(ctx).Model(&ChannelQuotaSnapshot{})
		if subjectRef != "" && legacyAccountRef != "" {
			return query.Where("((subject_ref = ? AND identity_quality = ?) OR ((subject_ref = ? OR subject_ref IS NULL) AND (identity_quality = ? OR identity_quality IS NULL) AND account_ref = ?))",
				subjectRef, ChannelQuotaIdentityQualityProviderConfirmed, "", "", legacyAccountRef)
		}
		if subjectRef != "" {
			return query.Where("subject_ref = ? AND identity_quality = ?", subjectRef, ChannelQuotaIdentityQualityProviderConfirmed)
		}
		return query.Where("(subject_ref = ? OR subject_ref IS NULL) AND (identity_quality = ? OR identity_quality IS NULL) AND account_ref = ?", "", "", legacyAccountRef)
	}
	latest := func(source, status, errorCode string) (ChannelQuotaSnapshot, bool, error) {
		var snapshot ChannelQuotaSnapshot
		query := identityQuery().Where("metric_type = ? AND source = ? AND status = ?", "codex_rate_limit", source, status)
		if errorCode != "" {
			query = query.Where("error_code = ?", errorCode)
		}
		err := query.Order("observed_at DESC, id DESC").Take(&snapshot).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChannelQuotaSnapshot{}, false, nil
		}
		return snapshot, err == nil, err
	}

	observedWindows := make([]ChannelQuotaSnapshot, 0, 2)
	for _, source := range []string{codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource} {
		snapshot, found, err := latest(source, "success", "")
		if err != nil {
			return CodexQuotaRouteState{}, err
		}
		if !found || snapshot.Status != "success" {
			continue
		}
		observedWindows = append(observedWindows, snapshot)
	}
	type absenceResult struct {
		snapshot ChannelQuotaSnapshot
		found    bool
	}
	absences := make(map[string]absenceResult, len(observedWindows))
	pairedAbsence := func(snapshot ChannelQuotaSnapshot, after int64) (bool, error) {
		absence, loaded := absences[snapshot.Source]
		if !loaded {
			latestAbsence, found, err := latest(snapshot.Source, "unsupported", "window_absent")
			if err != nil {
				return false, err
			}
			absence = absenceResult{snapshot: latestAbsence, found: found}
			absences[snapshot.Source] = absence
		}
		if !absence.found || absence.snapshot.SampleID == "" ||
			absence.snapshot.ObservedAt <= after || absence.snapshot.ObservedAt <= snapshot.ObservedAt || absence.snapshot.ObservedAt > now {
			return false, nil
		}
		for _, other := range observedWindows {
			if other.Source != snapshot.Source && other.Available > 0 &&
				other.ObservedAt == absence.snapshot.ObservedAt && other.SampleID == absence.snapshot.SampleID {
				return true, nil
			}
		}
		return false, nil
	}

	var blocked CodexQuotaRouteState
	for _, snapshot := range observedWindows {
		if snapshot.Available > 0 {
			continue
		}
		// A reset releases only a zero observation from the preceding window.
		// A zero sampled at/after that reset is fresh exhaustion, even when
		// the provider still reports an already-passed reset timestamp.
		if snapshot.ResetAt > 0 && snapshot.ResetAt <= now && snapshot.ObservedAt < snapshot.ResetAt {
			continue
		}
		retired, err := pairedAbsence(snapshot, 0)
		if err != nil {
			return CodexQuotaRouteState{}, err
		}
		if retired {
			continue
		}
		blocked.Blocked = true
		if blocked.Source == "" || snapshot.ResetAt == 0 || blocked.ResetAt > 0 && snapshot.ResetAt > blocked.ResetAt {
			blocked.ResetAt = snapshot.ResetAt
			blocked.ObservedAt = snapshot.ObservedAt
			blocked.Source = snapshot.Source
		}
	}
	if blocked.Blocked {
		return blocked, nil
	}

	marker, found, err := latest(CodexQuotaRouteLimitSource, "error", CodexQuotaRouteLimitCode)
	if err != nil {
		return CodexQuotaRouteState{}, err
	}
	if !found || marker.Status != "error" || marker.ErrorCode != CodexQuotaRouteLimitCode ||
		marker.ResetAt > 0 && marker.ResetAt <= now && marker.ObservedAt < marker.ResetAt {
		return CodexQuotaRouteState{}, nil
	}
	// A larger row ID only proves commit order. Every window previously
	// observed for this account must show fresh positive capacity after the
	// 429; one healthy window cannot clear an unknown limit in another.
	recovered := len(observedWindows) > 0
	for _, snapshot := range observedWindows {
		if snapshot.Available > 0 && snapshot.ObservedAt > marker.ObservedAt && snapshot.ObservedAt <= now {
			continue
		}
		retired, err := pairedAbsence(snapshot, marker.ObservedAt)
		if err != nil {
			return CodexQuotaRouteState{}, err
		}
		if !retired {
			recovered = false
			break
		}
	}
	if recovered {
		return CodexQuotaRouteState{}, nil
	}
	return CodexQuotaRouteState{Blocked: true, ResetAt: marker.ResetAt, ObservedAt: marker.ObservedAt, Source: CodexQuotaRouteLimitSource}, nil
}
