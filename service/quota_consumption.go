package service

import (
	"math"
	"sort"

	"github.com/ForceMind/MyAPI/model"
)

// QuotaConsumptionObservation attaches interval measurements to their ending
// observation. It does not interpolate provider data into unobserved minutes.
type QuotaConsumptionObservation struct {
	Snapshot                 model.ChannelQuotaSnapshot
	Consumption              *float64
	RatePerMinute            *float64
	AvailableChangePerMinute *float64
	ObservedSeconds          int64
	Reset                    bool
	Recovery                 bool
	Gap                      bool
	BaselineChange           bool
	ContinuityBreak          bool
}

type QuotaConsumptionSummary struct {
	Observed             *float64 `json:"observed,omitempty"`
	Basis                string   `json:"basis"`
	PairCount            int      `json:"pair_count"`
	ObservedSeconds      int64    `json:"observed_seconds"`
	AverageRatePerMinute *float64 `json:"average_rate_per_minute,omitempty"`
	PeakRatePerMinute    *float64 `json:"peak_rate_per_minute,omitempty"`
	PeakRateObservedAt   *int64   `json:"peak_rate_observed_at,omitempty"`
	ResetBoundaries      int      `json:"reset_boundaries"`
	RecoveryCount        int      `json:"recovery_count"`
	InterruptedCount     int      `json:"interrupted_count"`
	GapCount             int      `json:"gap_count"`
	BaselineChangeCount  int      `json:"baseline_change_count"`
	Unit                 string   `json:"unit,omitempty"`
	Allocation           string   `json:"allocation"`
}

type QuotaConsumptionResult struct {
	Observations []QuotaConsumptionObservation
	Summary      QuotaConsumptionSummary
}

// QuotaConsumptionBasis selects the provider field used to derive interval
// consumption. Auto preserves the historical preference for reported usage;
// Available always derives consumption from the remaining-quota difference.
type QuotaConsumptionBasis string

const (
	QuotaConsumptionBasisAuto      QuotaConsumptionBasis = "auto"
	QuotaConsumptionBasisAvailable QuotaConsumptionBasis = "available"
)

func quotaFinite(value float64) bool     { return !math.IsNaN(value) && !math.IsInf(value, 0) }
func quotaNumber(value float64) *float64 { return &value }

// QuotaSnapshotUsable rejects malformed numeric fields before they participate
// in either a chart metric or a consumption calculation.
func QuotaSnapshotUsable(snapshot model.ChannelQuotaSnapshot) bool {
	return snapshot.Status == "success" && quotaFinite(snapshot.Available) &&
		(snapshot.Used == nil || (quotaFinite(*snapshot.Used) && *snapshot.Used >= 0)) &&
		(snapshot.Total == nil || (quotaFinite(*snapshot.Total) && *snapshot.Total >= 0))
}

func quotaSameIdentity(a, b model.ChannelQuotaSnapshot) bool {
	return a.ChannelId == b.ChannelId && a.MetricType == b.MetricType && a.WindowType == b.WindowType &&
		a.Source == b.Source && a.PlanType == b.PlanType && a.Unit == b.Unit &&
		a.Currency == b.Currency && a.WindowSeconds == b.WindowSeconds
}

const codexQuotaResetRoundingToleranceSeconds = int64(2)

func quotaResetBetween(a, b model.ChannelQuotaSnapshot) bool {
	// Crossing the previously announced reset is a real boundary even if the
	// following payload reports a nearby timestamp or another empty window.
	if a.ResetAt > 0 && a.ObservedAt < a.ResetAt && b.ObservedAt >= a.ResetAt {
		return true
	}
	if a.ResetAt == b.ResetAt {
		return false
	}
	// WHAM's future reset metadata can differ by one/two seconds due to
	// rounding. This exception is intentionally restricted to the normalized
	// Codex percent series; it does not weaken other provider reset semantics,
	// permit missing timestamps, or make failed/out-of-order data comparable.
	if a.MetricType != "codex_rate_limit" || a.Unit != "percent" ||
		!quotaSameIdentity(a, b) || !QuotaSnapshotUsable(a) || !QuotaSnapshotUsable(b) ||
		b.ObservedAt <= a.ObservedAt || a.ResetAt <= 0 || b.ResetAt <= 0 ||
		a.ResetAt <= a.ObservedAt || b.ResetAt <= b.ObservedAt {
		return true
	}
	difference := b.ResetAt - a.ResetAt
	if difference >= -codexQuotaResetRoundingToleranceSeconds && difference <= codexQuotaResetRoundingToleranceSeconds {
		return false
	}
	// An unused WHAM window may not have started yet: successive zero-usage
	// snapshots can move its future reset with the polling time. Preserve that
	// provider metadata without claiming a reset or inventing consumption.
	// Require explicit normalized zero usage, not a missing value inferred as
	// zero. The ordinary baseline/failure/gap guards still apply afterwards.
	if a.Used != nil && b.Used != nil && *a.Used == 0 && *b.Used == 0 &&
		a.Total != nil && b.Total != nil && *a.Total == 100 && *b.Total == 100 &&
		a.Available == 100 && b.Available == 100 && quotaSameBaseline(a, b) {
		return false
	}
	return true
}

func quotaSameBaseline(a, b model.ChannelQuotaSnapshot) bool {
	if (a.Total == nil) != (b.Total == nil) || (a.Used == nil) != (b.Used == nil) {
		return false
	}
	if a.Total != nil && math.Abs(*a.Total-*b.Total) > 1e-9*math.Max(1, math.Max(math.Abs(*a.Total), math.Abs(*b.Total))) {
		return false
	}
	return true
}

func quotaSameConsumptionBaseline(a, b model.ChannelQuotaSnapshot, basis QuotaConsumptionBasis) bool {
	if basis == QuotaConsumptionBasisAvailable {
		if (a.Total == nil) != (b.Total == nil) {
			return false
		}
		if a.Total != nil && math.Abs(*a.Total-*b.Total) > 1e-9*math.Max(1, math.Max(math.Abs(*a.Total), math.Abs(*b.Total))) {
			return false
		}
		return true
	}
	return quotaSameBaseline(a, b)
}

func quotaComparable(a, b model.ChannelQuotaSnapshot, basis QuotaConsumptionBasis) bool {
	return QuotaSnapshotUsable(a) && QuotaSnapshotUsable(b) && quotaSameIdentity(a, b) &&
		!quotaResetBetween(a, b) && quotaSameConsumptionBaseline(a, b, basis) && b.ObservedAt > a.ObservedAt
}

// quotaLocalCadence derives the historical cadence from neighboring intervals,
// not today's sampler configuration. The slower side protects a genuine change
// from one-minute to fifteen-minute sampling. Two isolated observations have
// no cadence evidence; their rate remains an average over the reported span.
func quotaLocalCadence(rows []model.ChannelQuotaSnapshot, end int, basis QuotaConsumptionBasis) int64 {
	medianSide := func(from, step int) int64 {
		spans := make([]int64, 0, 3)
		for i := from; i > 0 && i < len(rows) && len(spans) < 3; i += step {
			if !quotaComparable(rows[i-1], rows[i], basis) {
				break
			}
			spans = append(spans, rows[i].ObservedAt-rows[i-1].ObservedAt)
		}
		if len(spans) == 0 {
			return 0
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i] < spans[j] })
		return spans[(len(spans)-1)/2]
	}
	left, right := medianSide(end-1, -1), medianSide(end+1, 1)
	if right > left {
		return right
	}
	return left
}

// DeriveQuotaConsumption is the sole consumption/rate calculation used by the
// history endpoint and the cross-channel summary. A reset, failure, changed
// baseline, or unusually long sampling gap cannot create a charge or recovery.
func DeriveQuotaConsumption(snapshots []model.ChannelQuotaSnapshot, unit string) QuotaConsumptionResult {
	return DeriveQuotaConsumptionWithBasis(snapshots, unit, QuotaConsumptionBasisAuto)
}

// DeriveQuotaConsumptionWithBasis applies the same continuity and safety
// guards as DeriveQuotaConsumption while allowing callers to explicitly use
// remaining quota as the interval source.
func DeriveQuotaConsumptionWithBasis(snapshots []model.ChannelQuotaSnapshot, unit string, basis QuotaConsumptionBasis) QuotaConsumptionResult {
	rows := append([]model.ChannelQuotaSnapshot(nil), snapshots...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ObservedAt == rows[j].ObservedAt {
			return rows[i].Id < rows[j].Id
		}
		return rows[i].ObservedAt < rows[j].ObservedAt
	})
	result := QuotaConsumptionResult{Summary: QuotaConsumptionSummary{Basis: "unavailable", Unit: unit, Allocation: "interval_end"}, Observations: make([]QuotaConsumptionObservation, 0, len(rows))}
	bases := map[string]bool{}
	var lastUsable *model.ChannelQuotaSnapshot
	for i, snapshot := range rows {
		point := QuotaConsumptionObservation{Snapshot: snapshot}
		if !QuotaSnapshotUsable(snapshot) {
			point.ContinuityBreak = true
			result.Summary.InterruptedCount++
			result.Observations = append(result.Observations, point)
			continue
		}
		if lastUsable != nil && quotaSameIdentity(*lastUsable, snapshot) && quotaResetBetween(*lastUsable, snapshot) {
			point.Reset = true
			point.ContinuityBreak = true
			result.Summary.ResetBoundaries++
		}
		lastUsable = &rows[i]
		if i == 0 {
			result.Observations = append(result.Observations, point)
			continue
		}
		previous := rows[i-1]
		switch {
		case !QuotaSnapshotUsable(previous):
			point.ContinuityBreak = true
		case !quotaSameIdentity(previous, snapshot), !quotaSameConsumptionBaseline(previous, snapshot, basis):
			point.BaselineChange = true
			point.ContinuityBreak = true
			result.Summary.BaselineChangeCount++
		case point.Reset:
		case snapshot.ObservedAt <= previous.ObservedAt:
			point.Gap = true
			point.ContinuityBreak = true
			result.Summary.GapCount++
		default:
			span := snapshot.ObservedAt - previous.ObservedAt
			cadence := quotaLocalCadence(rows, i, basis)
			threshold := int64(180)
			if cadence*3 > threshold {
				threshold = cadence * 3
			}
			if cadence > 0 && span > threshold {
				point.Gap = true
				point.ContinuityBreak = true
				result.Summary.GapCount++
				break
			}
			delta, intervalBasis := previous.Available-snapshot.Available, "available"
			if basis != QuotaConsumptionBasisAvailable && previous.Used != nil && snapshot.Used != nil {
				delta = *snapshot.Used - *previous.Used
				intervalBasis = "used"
			}
			rate := delta / (float64(span) / 60)
			availableRate := (snapshot.Available - previous.Available) / (float64(span) / 60)
			if !quotaFinite(delta) || !quotaFinite(rate) || !quotaFinite(availableRate) {
				point.ContinuityBreak = true
				result.Summary.InterruptedCount++
				break
			}
			point.AvailableChangePerMinute = quotaNumber(availableRate)
			if delta < 0 {
				point.Recovery = true
				point.ContinuityBreak = true
				result.Summary.RecoveryCount++
				break
			}
			if result.Summary.Observed == nil {
				result.Summary.Observed = quotaNumber(0)
			}
			if !quotaFinite(*result.Summary.Observed + delta) {
				point.ContinuityBreak = true
				result.Summary.InterruptedCount++
				break
			}
			point.Consumption = quotaNumber(delta)
			point.RatePerMinute = quotaNumber(rate)
			point.ObservedSeconds = span
			*result.Summary.Observed += delta
			result.Summary.PairCount++
			result.Summary.ObservedSeconds += span
			bases[intervalBasis] = true
			if result.Summary.PeakRatePerMinute == nil || rate > *result.Summary.PeakRatePerMinute {
				result.Summary.PeakRatePerMinute = quotaNumber(rate)
				at := snapshot.ObservedAt
				result.Summary.PeakRateObservedAt = &at
			}
		}
		result.Observations = append(result.Observations, point)
	}
	if result.Summary.ObservedSeconds > 0 {
		result.Summary.AverageRatePerMinute = quotaNumber(*result.Summary.Observed / (float64(result.Summary.ObservedSeconds) / 60))
	}
	if len(bases) == 1 {
		for basis := range bases {
			result.Summary.Basis = basis
		}
	} else if len(bases) > 1 {
		result.Summary.Basis = "mixed"
	}
	return result
}
