package router

import (
	"net/http"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Restore compatible counters at the real relay boundary and prove they
// settle exactly once; neither a display-only assertion nor a mock charge.
func TestResponsesChatCompatibleUsageSettlesExactlyOnce(t *testing.T) {
	for _, sample := range []struct {
		name, event string
	}{
		{"canonical", `{"type":"response.completed","response":{"model":"synthetic-usage-model","status":"completed","usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":8}}}}`},
		{"details_alias", `{"type":"response.completed","response":{"model":"synthetic-usage-model","status":"completed","usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23,"prompt_tokens_details":{"cached_tokens":8}}}}`},
		{"root_usage", `{"type":"response.completed","response":{"model":"synthetic-usage-model","status":"completed"},"usage":{"input_tokens":20,"output_tokens":3,"total_tokens":23,"input_tokens_details":{"cached_tokens":8}}}`},
	} {
		for _, stream := range []bool{false, true} {
			name := sample.name + "/buffered"
			if stream {
				name = sample.name + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				fixture := newCodexSessionHeadersFixture(t, "synthetic-usage-model", false, false, "", func(f *codexSessionHeadersFixture) {
					f.responseSSE = "data: " + sample.event + "\n\ndata: [DONE]\n\n"
				})
				oldCache, oldCompletion := ratio_setting.CacheRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
				require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"synthetic-usage-model":0.5}`))
				require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"synthetic-usage-model":2}`))
				t.Cleanup(func() {
					require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(oldCache))
					require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
				})
				var before model.Token
				require.NoError(t, model.DB.Where("key = ?", fixture.tokenKey).First(&before).Error)
				body, err := common.Marshal(map[string]any{"model": "synthetic-usage-model", "messages": []map[string]string{{"role": "user", "content": "synthetic hello"}}, "stream": stream})
				require.NoError(t, err)
				response := fixture.request(t, "/v1/chat/completions", body, false)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				assert.Contains(t, response.Body.String(), `"cached_tokens":8`)
				assert.NotContains(t, response.Body.String(), `"billing_usage"`)
				var after model.Token
				require.NoError(t, model.DB.First(&after, before.Id).Error)
				// (20 - 8) * 1 + 8 * 0.5 + 3 * 2 = 22 quota.
				assert.EqualValues(t, 22, before.RemainQuota-after.RemainQuota)
				assert.EqualValues(t, 22, after.UsedQuota-before.UsedQuota)
				assert.EqualValues(t, 1, fixture.calls.Load())
				assert.Zero(t, fixture.external.Load())
			})
		}
	}
}
