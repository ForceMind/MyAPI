package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesUsageEvidenceDistinguishesMissingCountersFromReportedZero(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, handler := range []struct {
		name   string
		stream bool
		run    func(*gin.Context, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{"responses", false, func(c *gin.Context, r *http.Response) (*dto.Usage, *types.NewAPIError) {
			return OaiResponsesHandler(c, &relaycommon.RelayInfo{}, r)
		}},
		{"compaction", false, OaiResponsesCompactionHandler},
		{"stream", true, func(c *gin.Context, r *http.Response) (*dto.Usage, *types.NewAPIError) {
			return OaiResponsesStreamHandler(c, &relaycommon.RelayInfo{DisablePing: true}, r)
		}},
	} {
		for _, sample := range []struct {
			name, raw  string
			incomplete bool
		}{
			{"empty object", `{}`, true},
			{"missing output", `{"input_tokens":10}`, true},
			{"missing input", `{"output_tokens":10}`, true},
			{"null counter", `{"input_tokens":null,"output_tokens":0}`, true},
			{"contradictory zero total", `{"input_tokens":10,"output_tokens":0,"total_tokens":0}`, true},
			{"explicit zero", `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`, false},
			{"two actual counters", `{"input_tokens":10,"output_tokens":0}`, false},
		} {
			t.Run(handler.name+"/"+sample.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				body := `{"usage":` + sample.raw + `}`
				contentType := "application/json"
				if handler.stream {
					body = "data: {\"type\":\"response.completed\",\"response\":" + body + "}\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				usage, apiErr := handler.run(c, &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))})
				require.Nil(t, apiErr)
				require.NotNil(t, usage.BillingUsage)
				encoded, err := common.Marshal(usage.BillingUsage)
				require.NoError(t, err)
				var evidence map[string]any
				require.NoError(t, common.Unmarshal(encoded, &evidence))
				assert.Equal(t, sample.incomplete, evidence["incomplete"] == true, "missing fields must not be promoted to reported zero usage")
				assert.False(t, usage.BillingUsage.Estimated, "partial upstream evidence remains distinct from a local estimate")
			})
		}
	}
}
