package jimeng

import (
	"fmt"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToRequestPayloadKeepsMappedReqKeyAuthoritative(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		upstreamModel string
		images        []string
		wantReqKey    string
	}{
		{
			name:          "pro ignores image count",
			upstreamModel: "jimeng_v30_pro",
			images:        []string{"first", "last"},
			wantReqKey:    "jimeng_ti2v_v30_pro",
		},
		{
			name:          "first and last frame",
			upstreamModel: "jimeng_v30p",
			images:        []string{"first", "last"},
			wantReqKey:    "jimeng_i2v_first_tail_v30",
		},
		{
			name:          "single image",
			upstreamModel: "jimeng_v30p",
			images:        []string{"first"},
			wantReqKey:    "jimeng_i2v_first_v30",
		},
		{
			name:          "text only",
			upstreamModel: "jimeng_v30p",
			wantReqKey:    "jimeng_t2v_v30p",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{
				Model:  "client-model-a",
				Prompt: "make a clip",
				Images: testCase.images,
				Metadata: map[string]any{
					"model":      "metadata-model",
					"model_name": "metadata-model-name",
					"req_key":    "metadata-req-key",
					"prompt":     "metadata prompt",
					"frames":     float64(99999999),
					"seed":       float64(42),
				},
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: testCase.upstreamModel}}

			payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req, info)
			require.NoError(t, err)
			assert.Equal(t, testCase.wantReqKey, payload.ReqKey)
			assert.Equal(t, "make a clip", payload.Prompt)
			assert.Equal(t, 121, payload.Frames)
			assert.Equal(t, int64(42), payload.Seed, "non-model provider metadata must be preserved")
			assert.Contains(t, req.Metadata, "model", "conversion must not mutate caller metadata")
		})
	}
}

func TestConvertToRequestPayloadIgnoresMetadataFrames(t *testing.T) {
	for _, metadataFrames := range []any{float64(0), float64(-1), float64(241), float64(1e9)} {
		t.Run(fmt.Sprint(metadataFrames), func(t *testing.T) {
			req := relaycommon.TaskSubmitReq{
				Prompt:   "make a clip",
				Duration: 10,
				Metadata: map[string]any{"frames": metadataFrames},
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "jimeng_vgfm_t2v_l20"}}

			payload, err := (&TaskAdaptor{}).convertToRequestPayload(&req, info)
			require.NoError(t, err)
			assert.Equal(t, 241, payload.Frames)
		})
	}
}
