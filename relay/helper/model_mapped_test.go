package helper

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelMappedHelperMappingSemantics(t *testing.T) {
	tests := []struct {
		name, mapping, origin, want string
		mapped                      bool
		wantErr                     string
	}{
		{"direct", `{"a":"b"}`, "a", "b", true, ""},
		{"chain", `{"a":"b","b":"c"}`, "a", "c", true, ""},
		{"self", `{"a":"a"}`, "a", "a", false, ""},
		{"tail self", `{"a":"b","b":"b"}`, "a", "b", true, ""},
		{"cycle", `{"a":"b","b":"a"}`, "a", "", false, "model_mapping_contains_cycle"},
		{"malformed", `{`, "a", "", false, "unmarshal_model_mapping_failed"},
		{"empty", ``, "a", "a", false, ""},
		{"empty object", `{}`, "a", "a", false, ""},
		{"null", `null`, "a", "a", false, ""},
		{"empty target", `{"a":""}`, "a", "a", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("model_mapping", tt.mapping)
			info := &relaycommon.RelayInfo{OriginModelName: tt.origin, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tt.origin}}
			request := &dto.GeneralOpenAIRequest{Model: tt.origin}
			err := ModelMappedHelper(c, info, request)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				require.Equal(t, tt.origin, info.UpstreamModelName)
				require.Equal(t, tt.origin, request.Model)
				require.Equal(t, tt.name == "cycle", info.IsModelMapped)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, info.UpstreamModelName)
			require.Equal(t, tt.want, request.Model)
			require.Equal(t, tt.mapped, info.IsModelMapped)
		})
	}
}
