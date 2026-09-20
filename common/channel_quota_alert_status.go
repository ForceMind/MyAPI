package common

import "math"

// ChannelQuotaAlertStatus derives the alert status only from a successful,
// provider-reported total. Unknown, non-finite, or non-positive totals remain
// unavailable rather than being interpreted as healthy.
func ChannelQuotaAlertStatus(available float64, total *float64, settings ChannelQuotaAlertSettings) string {
	if total == nil || math.IsNaN(available) || math.IsInf(available, 0) ||
		math.IsNaN(*total) || math.IsInf(*total, 0) || *total <= 0 {
		return ""
	}
	ratio := available / *total * 100
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) {
		return ""
	}
	switch {
	case ratio <= settings.CriticalPercent:
		return "critical"
	case ratio <= settings.WarningPercent:
		return "warning"
	default:
		return "healthy"
	}
}
