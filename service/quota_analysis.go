package service

import "math"

const (
	QuotaETADepletesBeforeReset             = "depletes_before_reset"
	QuotaETAResetBeforeDepletion            = "reset_before_depletion"
	QuotaETAStableOrNoObservedConsumption   = "stable_or_no_observed_consumption"
	QuotaETAInsufficientData                = "insufficient_data"
	quotaAnalysisDefaultMethod              = "observed_window"
	quotaAnalysisMaximumRateWindowSeconds   = int64(180 * 24 * 60 * 60)
	quotaAnalysisMaximumDepletionETASeconds = int64(365 * 100 * 24 * 60 * 60)
)

// QuotaETA describes whether an observed consumption rate would exhaust the
// current provider quota before its next reported reset. A missing ResetAt
// means no future reset was reported, so a finite depletion remains the first
// known boundary.
type QuotaETA struct {
	Outcome              string `json:"outcome"`
	EstimatedDepletionAt *int64 `json:"estimated_depletion_at,omitempty"`
	SecondsToDepletion   *int64 `json:"seconds_to_depletion,omitempty"`
	ResetAt              *int64 `json:"reset_at,omitempty"`
	SecondsUntilReset    *int64 `json:"seconds_until_reset,omitempty"`
}

// QuotaRateMethodAnalysis is one independently usable rate estimate. Coverage
// is the fraction of the requested rate window backed by valid raw intervals;
// it is never inferred from display buckets or sampler configuration.
type QuotaRateMethodAnalysis struct {
	RatePerMinute   *float64 `json:"rate_per_minute"`
	RatePerHour     *float64 `json:"rate_per_hour"`
	Coverage        float64  `json:"coverage"`
	ObservedSeconds int64    `json:"observed_seconds"`
	IntervalCount   int      `json:"interval_count"`
	ObservedAt      *int64   `json:"observed_at"`
	ETA             QuotaETA `json:"eta"`
}

type QuotaRateMethods struct {
	LatestInterval QuotaRateMethodAnalysis `json:"latest_interval"`
	ObservedWindow QuotaRateMethodAnalysis `json:"observed_window"`
	EWMA           QuotaRateMethodAnalysis `json:"ewma"`
}

// QuotaAnalysis exposes all supported methods together so changing the
// presentation method does not require another history query.
type QuotaAnalysis struct {
	DefaultMethod       string           `json:"default_method"`
	RateWindowSeconds   int64            `json:"rate_window_seconds"`
	WindowStart         int64            `json:"window_start"`
	WindowEnd           int64            `json:"window_end"`
	AsOf                int64            `json:"as_of"`
	EWMAHalfLifeSeconds int64            `json:"ewma_half_life_seconds"`
	Complete            bool             `json:"complete"`
	Methods             QuotaRateMethods `json:"methods"`
}

type QuotaOverviewPoint struct {
	Timestamp       int64    `json:"timestamp"`
	Available       *float64 `json:"available"`
	ContinuityBreak bool     `json:"continuity_break"`
}

type quotaAnalysisInterval struct {
	ratePerMinute float64
	consumption   float64
	seconds       int64
	observedAt    int64
}

// AnalyzeQuotaConsumption calculates rates only from valid raw intervals
// produced by DeriveQuotaConsumption. A continuity boundary discards the
// preceding segment, preventing reset, recovery, failure, gap, or baseline
// changes from influencing the current estimate.
func AnalyzeQuotaConsumption(consumption QuotaConsumptionResult, windowStart, windowEnd, halfLifeSeconds int64) QuotaAnalysis {
	windowSeconds := windowEnd - windowStart
	if windowSeconds < 0 || windowSeconds > quotaAnalysisMaximumRateWindowSeconds {
		windowSeconds = 0
	}
	if halfLifeSeconds <= 0 {
		halfLifeSeconds = 1
	}
	analysis := QuotaAnalysis{
		DefaultMethod:       quotaAnalysisDefaultMethod,
		RateWindowSeconds:   windowSeconds,
		WindowStart:         windowStart,
		WindowEnd:           windowEnd,
		AsOf:                windowEnd,
		EWMAHalfLifeSeconds: halfLifeSeconds,
		Complete:            true,
	}
	analysis.Methods.LatestInterval.ETA.Outcome = QuotaETAInsufficientData
	analysis.Methods.ObservedWindow.ETA.Outcome = QuotaETAInsufficientData
	analysis.Methods.EWMA.ETA.Outcome = QuotaETAInsufficientData
	if windowSeconds <= 0 {
		return analysis
	}

	intervals := make([]quotaAnalysisInterval, 0, len(consumption.Observations))
	var lastObservedIntervals []quotaAnalysisInterval
	var current *modelQuotaAnalysisSnapshot
	for _, observation := range consumption.Observations {
		snapshot := observation.Snapshot
		if snapshot.ObservedAt < windowStart || snapshot.ObservedAt > windowEnd {
			continue
		}
		current = &modelQuotaAnalysisSnapshot{
			usable:     QuotaSnapshotUsable(snapshot),
			available:  snapshot.Available,
			observedAt: snapshot.ObservedAt,
			resetAt:    snapshot.ResetAt,
		}
		if observation.ContinuityBreak {
			if current.usable {
				lastObservedIntervals = nil
			} else if len(intervals) > 0 {
				lastObservedIntervals = append(lastObservedIntervals[:0], intervals...)
			}
			intervals = intervals[:0]
		}
		if observation.Consumption == nil || observation.RatePerMinute == nil || observation.ObservedSeconds <= 0 {
			continue
		}
		intervalStart := snapshot.ObservedAt - observation.ObservedSeconds
		if intervalStart < windowStart || !quotaFinite(*observation.RatePerMinute) {
			continue
		}
		intervals = append(intervals, quotaAnalysisInterval{
			ratePerMinute: *observation.RatePerMinute,
			consumption:   *observation.Consumption,
			seconds:       observation.ObservedSeconds,
			observedAt:    snapshot.ObservedAt,
		})
	}
	if len(intervals) == 0 && current != nil && !current.usable {
		intervals = lastObservedIntervals
	}
	if len(intervals) == 0 {
		return analysis
	}

	latest := intervals[len(intervals)-1]
	analysis.Methods.LatestInterval = quotaRateMethod(
		latest.ratePerMinute,
		latest.seconds,
		1,
		latest.observedAt,
		windowSeconds,
		current,
	)

	var totalConsumption float64
	var observedSeconds int64
	observedFinite := true
	for _, interval := range intervals {
		totalConsumption += interval.consumption
		if !quotaFinite(totalConsumption) {
			observedFinite = false
		}
		observedSeconds += interval.seconds
	}
	if observedFinite && observedSeconds > 0 {
		observedRate := quotaScaledRatio(totalConsumption, 60, float64(observedSeconds))
		analysis.Methods.ObservedWindow = quotaRateMethod(
			observedRate,
			observedSeconds,
			len(intervals),
			latest.observedAt,
			windowSeconds,
			current,
		)
	}

	latestObservedAt := latest.observedAt
	lambda := math.Ln2 / float64(halfLifeSeconds)
	var ewmaRate, totalWeight float64
	for _, interval := range intervals {
		age := latestObservedAt - interval.observedAt
		if age < 0 {
			continue
		}
		endDecay := math.Exp(-lambda * float64(age))
		integratedDuration := -math.Expm1(-lambda*float64(interval.seconds)) / lambda
		weight := endDecay * integratedDuration
		if !quotaFinite(weight) || weight <= 0 {
			continue
		}
		nextWeight := totalWeight + weight
		if !quotaFinite(nextWeight) || nextWeight <= 0 {
			continue
		}
		if totalWeight == 0 {
			ewmaRate = interval.ratePerMinute
		} else {
			ewmaRate += (interval.ratePerMinute - ewmaRate) * (weight / nextWeight)
		}
		totalWeight = nextWeight
	}
	if quotaFinite(ewmaRate) && totalWeight > 0 {
		analysis.Methods.EWMA = quotaRateMethod(
			ewmaRate,
			observedSeconds,
			len(intervals),
			latest.observedAt,
			windowSeconds,
			current,
		)
	}
	return analysis
}

// modelQuotaAnalysisSnapshot intentionally retains only fields required by
// ETA generation. It prevents analysis code from accidentally depending on
// provider metadata unrelated to the current normalized series.
type modelQuotaAnalysisSnapshot struct {
	usable     bool
	available  float64
	observedAt int64
	resetAt    int64
}

func quotaRateMethod(ratePerMinute float64, observedSeconds int64, intervalCount int, observedAt, windowSeconds int64, current *modelQuotaAnalysisSnapshot) QuotaRateMethodAnalysis {
	method := QuotaRateMethodAnalysis{
		ObservedSeconds: observedSeconds,
		IntervalCount:   intervalCount,
		ObservedAt:      &observedAt,
		ETA:             quotaETA(ratePerMinute, current),
	}
	if !quotaFinite(ratePerMinute) {
		method.ETA = QuotaETA{Outcome: QuotaETAInsufficientData}
		return method
	}
	method.RatePerMinute = quotaNumber(ratePerMinute)
	hourly := ratePerMinute * 60
	if quotaFinite(hourly) {
		method.RatePerHour = quotaNumber(hourly)
	}
	if windowSeconds > 0 {
		method.Coverage = math.Min(1, float64(observedSeconds)/float64(windowSeconds))
	}
	return method
}

func quotaETA(ratePerMinute float64, current *modelQuotaAnalysisSnapshot) QuotaETA {
	eta := QuotaETA{Outcome: QuotaETAInsufficientData}
	if current == nil || !current.usable || !quotaFinite(current.available) || current.available < 0 || !quotaFinite(ratePerMinute) {
		return eta
	}
	if current.resetAt > current.observedAt {
		resetAt := current.resetAt
		secondsUntilReset := resetAt - current.observedAt
		eta.ResetAt = &resetAt
		eta.SecondsUntilReset = &secondsUntilReset
	}
	if current.available == 0 {
		seconds := int64(0)
		depletionAt := current.observedAt
		eta.SecondsToDepletion = &seconds
		eta.EstimatedDepletionAt = &depletionAt
		eta.Outcome = QuotaETADepletesBeforeReset
		return eta
	}
	if ratePerMinute <= 0 {
		eta.Outcome = QuotaETAStableOrNoObservedConsumption
		return eta
	}
	secondsFloat := quotaScaledRatio(current.available, 60, ratePerMinute)
	if !quotaFinite(secondsFloat) || secondsFloat < 0 || secondsFloat > float64(quotaAnalysisMaximumDepletionETASeconds) {
		return eta
	}
	seconds := int64(math.Ceil(secondsFloat))
	if current.observedAt > int64(1<<63-1)-seconds {
		return eta
	}
	depletionAt := current.observedAt + seconds
	if eta.ResetAt != nil && depletionAt >= *eta.ResetAt {
		eta.Outcome = QuotaETAResetBeforeDepletion
		return eta
	}
	eta.SecondsToDepletion = &seconds
	eta.EstimatedDepletionAt = &depletionAt
	eta.Outcome = QuotaETADepletesBeforeReset
	return eta
}

// quotaScaledRatio computes value*scale/divisor without overflowing an
// intermediate product when the final IEEE-754 result is representable.
func quotaScaledRatio(value, scale, divisor float64) float64 {
	if value == 0 {
		return 0
	}
	if !quotaFinite(value) || !quotaFinite(scale) || !quotaFinite(divisor) || divisor == 0 {
		return math.NaN()
	}
	valueFraction, valueExponent := math.Frexp(value)
	divisorFraction, divisorExponent := math.Frexp(divisor)
	return math.Ldexp(valueFraction/divisorFraction*scale, valueExponent-divisorExponent)
}

// BuildQuotaOverviewPoints creates a bounded remaining-quota projection from
// already loaded observations. Contiguous index buckets retain their latest
// sample and OR every continuity marker, so compression never draws across a
// failure, reset, recovery, gap, or baseline change and never interpolates an
// unobserved value.
func BuildQuotaOverviewPoints(observations []QuotaConsumptionObservation, limit int) []QuotaOverviewPoint {
	if limit <= 0 || len(observations) == 0 {
		return []QuotaOverviewPoint{}
	}
	if limit > 120 {
		limit = 120
	}
	points := make([]QuotaOverviewPoint, 0, len(observations))
	for _, observation := range observations {
		point := QuotaOverviewPoint{
			Timestamp:       observation.Snapshot.ObservedAt,
			ContinuityBreak: observation.ContinuityBreak,
		}
		if QuotaSnapshotUsable(observation.Snapshot) {
			point.Available = quotaNumber(observation.Snapshot.Available)
		} else {
			point.ContinuityBreak = true
		}
		points = append(points, point)
	}
	if len(points) <= limit {
		return points
	}
	compressed := make([]QuotaOverviewPoint, 0, limit)
	for index := 0; index < limit; index++ {
		start := index * len(points) / limit
		end := (index + 1) * len(points) / limit
		selected := points[end-1]
		for _, point := range points[start:end] {
			selected.ContinuityBreak = selected.ContinuityBreak || point.ContinuityBreak
		}
		compressed = append(compressed, selected)
	}
	return compressed
}
