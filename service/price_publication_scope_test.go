package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func publicationScopeFixture(body string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{OriginModelName: "fixture", Request: &dto.OpenAIResponsesRequest{}, BillingRequestInput: &billingexpr.RequestInput{Body: []byte(body)},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{OfficialPricePublicationID: strings.Repeat("a", 64)}}
}

func TestPublishedPriceRequestRejectsUnqualifiedCategories(t *testing.T) {
	for _, body := range []string{`{"input":"hello"}`, `{"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"service_tier":"default"}`} {
		require.NoError(t, ValidatePublishedPriceRequest(publicationScopeFixture(body)))
	}
	for _, body := range []string{
		`{"input":"hello","service_tier":"fast"}`, `{"input":"hello","service_tier":null}`,
		`{"input":"hello","tools":[{"type":"web_search"}]}`, `{"input":"hello","previous_response_id":"old"}`,
		`{"input":"hello","modalities":["audio"]}`, `{"input":[{"role":"user","content":[{"type":"input_image","image_url":"https://example.test/image.png"}]}]}`,
		`{"messages":[{"role":"user","content":"hi"}],"input":[{"type":"input_file","file_id":"opaque"}]}`,
		`{"input":{"role":"tool","content":"unqualified tool history"}}`, `{"input":123}`, `{}`,
	} {
		t.Run(body, func(t *testing.T) { require.Error(t, ValidatePublishedPriceRequest(publicationScopeFixture(body))) })
	}
	manual := publicationScopeFixture(`{"input":"hello","service_tier":"fast"}`)
	manual.TieredBillingSnapshot.OfficialPricePublicationID = ""
	require.NoError(t, ValidatePublishedPriceRequest(manual), "existing administrator expressions retain their request rules")
}

func TestPublishedPriceChecksEverySelectedChannel(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := publicationScopeFixture(`{"input":"hello"}`)
	require.NoError(t, ValidatePublishedPriceSelectedChannel(ctx, info))
	common.SetContextKey(ctx, constant.ContextKeyChannelParamOverride, map[string]interface{}{"service_tier": "fast"})
	require.Error(t, ValidatePublishedPriceSelectedChannel(ctx, info))
	common.SetContextKey(ctx, constant.ContextKeyChannelParamOverride, map[string]interface{}{})
	common.SetContextKey(ctx, constant.ContextKeyChannelModelMapping, `{"fixture":"another-model"}`)
	require.Error(t, ValidatePublishedPriceSelectedChannel(ctx, info))
}

func TestPublishedTextPriceHoldsUnquotedReportedUsage(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := publicationScopeFixture(`{"input":"hello"}`)
	usage := &dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(&dto.Usage{InputTokens: 100, OutputTokens: 10, TotalTokens: 110, InputTokensDetails: &dto.InputTokenDetails{ImageTokens: 20}})}
	assert.Equal(t, "price_scope_unqualified", publishedPriceUsageReviewReason(ctx, info, usage))
	info.TieredBillingSnapshot.OfficialPricePublicationID = ""
	assert.Empty(t, publishedPriceUsageReviewReason(ctx, info, usage))
}
