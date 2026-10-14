package personalusage

import "net/http"

type AdmissionDecision uint8

const (
	ConfiguredFunding AdmissionDecision = iota + 1
	NeedFundingState
	SelfUseFunding
	InvalidPolicy
	FundingUnavailable
)

type FundingState struct {
	Valid    bool
	Disabled bool
}

func EvaluateAdmission(noBalance bool, revision int64, fundingState *FundingState) AdmissionDecision {
	if !noBalance {
		return ConfiguredFunding
	}
	if revision <= 0 {
		return InvalidPolicy
	}
	if fundingState == nil {
		return NeedFundingState
	}
	if !fundingState.Valid {
		return FundingUnavailable
	}
	if fundingState.Disabled {
		return SelfUseFunding
	}
	return ConfiguredFunding
}

func SupportsMeteredRequest(method, path, rawQuery string) bool {
	if method != http.MethodPost || rawQuery != "" {
		return false
	}
	switch path {
	case "/v1/chat/completions", "/v1/responses", "/v1/responses/compact":
		return true
	default:
		return false
	}
}
