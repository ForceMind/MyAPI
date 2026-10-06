package service

import (
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeLogsDoNotInventMissingCacheModalities(t *testing.T) {
	for _, test := range []struct {
		name, quality string
		cached        int
		details       *dto.CachedTokenDetails
	}{
		{"missing", "unrecorded", 35, nil},
		{"complete", "complete", 35, dto.NewCachedTokenDetails(20, 10, 5)},
		{"partial", "partial", 40, dto.NewCachedTokenDetails(20, 10, 5)},
		{"explicit zero", "complete", 0, dto.NewCachedTokenDetails(0, 0, 0)},
		{"empty object is unknown", "partial", 0, &dto.CachedTokenDetails{}},
		{"negative subtype", "invalid", 35, dto.NewCachedTokenDetails(-1, 0, 0)},
		{"exceeds modality", "invalid", 35, dto.NewCachedTokenDetails(35, 0, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			info := &relaycommon.RelayInfo{StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{}}
			usage := &dto.RealtimeUsage{InputTokens: 60, TotalTokens: 60,
				InputTokenDetails: dto.InputTokenDetails{TextTokens: 30, AudioTokens: 20, ImageTokens: 10,
					CachedTokens: test.cached, CachedTokensDetails: test.details}}
			other := GenerateWssOtherInfo(ctx, info, usage, 1, 1, 1, 1, 1, 0, 1)
			assert.Equal(t, test.quality, other["cache_details_quality"])
			assert.Equal(t, test.cached, other["realtime_cached_tokens"])
			if test.details == nil || test.details.AudioTokens == nil {
				assert.NotContains(t, other, "cached_audio_tokens", "unknown is not audio cache zero")
			} else {
				assert.Equal(t, *test.details.AudioTokens, other["cached_audio_tokens"])
			}
		})
	}
}

func TestBillingProjectionKeepsCacheDetailSnapshotIndependent(t *testing.T) {
	billing := dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 60,
		InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 35,
			CachedTokensDetails: dto.NewCachedTokenDetails(20, 10, 5)}})
	require.NotNil(t, billing)
	projected := usageFromOpenAIBillingUsage(billing)
	*projected.PromptTokensDetails.CachedTokensDetails.TextTokens = 55
	*projected.InputTokensDetails.CachedTokensDetails.AudioTokens = 44
	assert.Equal(t, 20, *billing.OpenAIUsage.InputTokensDetails.CachedTokensDetails.TextTokens)
	assert.Equal(t, 10, *billing.OpenAIUsage.InputTokensDetails.CachedTokensDetails.AudioTokens)
	assert.Equal(t, 20, *projected.BillingUsage.OpenAIUsage.InputTokensDetails.CachedTokensDetails.TextTokens)
}
