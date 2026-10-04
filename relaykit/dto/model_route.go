package dto

// ModelRoute is an explicit mapping for already-enabled public model names.
// Prefix matching never enables a model or derives a target by guessing a suffix.
type ModelRoute struct {
	PublicModel   string `json:"public_model"`
	UpstreamModel string `json:"upstream_model"`
	Endpoint      string `json:"endpoint"`
	Match         string `json:"match"`
	Priority      int64  `json:"priority"`
}
