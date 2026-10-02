package model

import (
	"context"
	"errors"
	"math"

	"github.com/ForceMind/MyAPI/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrAccountQuotaThresholdInvalid = errors.New("invalid account quota threshold")
var ErrAccountQuotaThresholdCombination = errors.New("account thresholds cannot be combined with the current direct API Token or USD budget modes")

type AccountQuotaThresholdPolicyInput struct {
	Enabled             bool  `json:"enabled"`
	MinimumRemainingBPS int   `json:"minimum_remaining_bps"`
	MaxAgeSeconds       int64 `json:"max_age_seconds"`
}

type AccountQuotaThresholdState struct {
	Eligible   bool
	Reason     string
	ObservedAt int64
	ResetAt    int64
}

// Uses the same opaque provider-account identity as exact-429 routing, but
// never falls back from a failed/incomplete new batch to older success.
func ReadCodexAccountThresholdState(ctx context.Context, db *gorm.DB, subjectRef, legacyAccountRef string, now int64, policy common.AccountQuotaThreshold) (AccountQuotaThresholdState, error) {
	state := AccountQuotaThresholdState{Reason: "threshold_unavailable"}
	if db == nil {
		return state, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !common.ValidAccountQuotaThreshold(policy) || now <= 0 {
		return state, ErrAccountQuotaThresholdInvalid
	}
	if subjectRef != "" && !canonicalChannelQuotaIdentitySubject(subjectRef) || legacyAccountRef != "" && !canonicalLowerHex(legacyAccountRef, 64) || subjectRef == "" && legacyAccountRef == "" {
		return state, ErrCodexQuotaRouteIdentity
	}
	identity := func() *gorm.DB {
		query := db.WithContext(ctx).Model(&ChannelQuotaSnapshot{}).Where("metric_type = ? AND source IN ?", "codex_rate_limit", []string{"codex_wham_usage", codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource})
		if subjectRef != "" && legacyAccountRef != "" {
			return query.Where("((subject_ref = ? AND identity_quality = ?) OR ((subject_ref = ? OR subject_ref IS NULL) AND (identity_quality = ? OR identity_quality IS NULL) AND account_ref = ?))", subjectRef, ChannelQuotaIdentityQualityProviderConfirmed, "", "", legacyAccountRef)
		}
		if subjectRef != "" {
			return query.Where("subject_ref = ? AND identity_quality = ?", subjectRef, ChannelQuotaIdentityQualityProviderConfirmed)
		}
		return query.Where("(subject_ref = ? OR subject_ref IS NULL) AND (identity_quality = ? OR identity_quality IS NULL) AND account_ref = ?", "", "", legacyAccountRef)
	}
	var latest ChannelQuotaSnapshot
	err := identity().Order("observed_at DESC, id DESC").Take(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.ObservedAt = latest.ObservedAt
	if latest.ObservedAt <= 0 || latest.ObservedAt > now || now-latest.ObservedAt > policy.MaxAgeSeconds {
		state.Reason = "threshold_stale"
		return state, nil
	}
	if latest.SampleID == "" || !latest.CodexThresholdQualified {
		return state, nil
	}
	var batch []ChannelQuotaSnapshot
	if err := identity().Where("sample_id = ? AND observed_at = ?", latest.SampleID, latest.ObservedAt).Order("id ASC").Limit(65).Find(&batch).Error; err != nil {
		return state, err
	}
	if len(batch) == 0 || len(batch) > 64 {
		return state, nil
	}
	active := map[string]bool{}
	minimum := decimal.NewFromInt(int64(policy.MinimumRemainingBPS)).Shift(-2)
	for _, row := range batch {
		if !row.CodexThresholdQualified {
			return state, nil
		}
		if row.Status == "unsupported" && row.ErrorCode == "window_absent" {
			continue
		}
		if row.Status != "success" || (row.Source != codexQuotaWindowPrimarySource && row.Source != codexQuotaWindowSecondarySource) || active[row.Source] || row.Unit != "percent" || row.Currency != "" || row.Total == nil || *row.Total != 100 || row.Used == nil || math.IsNaN(*row.Used) || math.IsInf(*row.Used, 0) || *row.Used < 0 || *row.Used > 100 || math.IsNaN(row.Available) || math.IsInf(row.Available, 0) || math.Abs(row.Available-(100-*row.Used)) > 1e-9 {
			return state, nil
		}
		if row.WindowSeconds <= 0 || row.WindowType == "" || row.WindowType == "none" || row.WindowType == "unknown" || row.ResetAt <= now || row.ResetAt <= row.ObservedAt || row.ResetAt-row.ObservedAt > row.WindowSeconds {
			state.Reason = "threshold_reset_or_unknown"
			return state, nil
		}
		active[row.Source] = true
		if state.ResetAt == 0 || row.ResetAt < state.ResetAt {
			state.ResetAt = row.ResetAt
		}
		// Avoid binary subtraction turning a reported 99.99% used into slightly
		// more than 0.01% remaining and admitting a boundary request.
		if decimal.NewFromInt(100).Sub(decimal.NewFromFloat(*row.Used)).LessThanOrEqual(minimum) {
			state.Reason = "threshold_reached"
			return state, nil
		}
	}
	if len(active) == 0 {
		return state, nil
	}
	state.Eligible, state.Reason = true, "eligible"
	return state, nil
}

// Newer credential-scoped failures cannot borrow another credential's healthy
// account sample. Failures remain scoped and do not poison other accounts.
func CodexThresholdCredentialFailedAfter(ctx context.Context, db *gorm.DB, credentialSubject string, observedAt int64) (bool, error) {
	if credentialSubject == "" {
		return false, nil
	}
	if !canonicalChannelQuotaIdentitySubject(credentialSubject) {
		return false, ErrCodexQuotaRouteIdentity
	}
	var latest ChannelQuotaSnapshot
	err := db.WithContext(ctx).Where("subject_ref = ? AND identity_quality = ? AND metric_type = ? AND source = ?", credentialSubject, ChannelQuotaIdentityQualityCredentialScoped, "codex_rate_limit", "codex_wham_usage").Order("observed_at DESC, id DESC").Take(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return latest.ObservedAt >= observedAt, nil
}
