package types

// RealtimeUsageCheckpoint stores bounded confirmed totals, not response bodies
// or history. Cache and reasoning are already included in their parent counts.
type RealtimeUsageCheckpoint struct {
	Responses   int `json:"responses"`
	Quota       int `json:"quota"`
	Input       int `json:"input_tokens"`
	Output      int `json:"output_tokens"`
	Total       int `json:"total_tokens"`
	InputText   int `json:"input_text_tokens"`
	InputAudio  int `json:"input_audio_tokens"`
	InputImage  int `json:"input_image_tokens"`
	OutputText  int `json:"output_text_tokens"`
	OutputAudio int `json:"output_audio_tokens"`
	OutputImage int `json:"output_image_tokens"`
}
