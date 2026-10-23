package common

// RequestInputEstimate is diagnostic evidence, never provider usage or a charge.
type RequestInputEstimate struct {
	InputTokens int64  `json:"input_tokens"`
	Method      string `json:"method"`
	Complete    bool   `json:"complete"`
	LimitTokens int64  `json:"limit_tokens,omitempty"`
}
