package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSelectedModelRouteChecksBothNamesAndPriceQualification(t *testing.T) {
	require.NoError(t, i18n.Init())
	settings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: []dto.ModelRoute{{PublicModel: "public", UpstreamModel: "actual", Endpoint: "/v1/responses", Match: "exact"}}})
	require.NoError(t, err)
	channel := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI, Models: "public", Key: "fixture", OtherSettings: string(settings)}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"public": true})
	denied := prepareSelectedModelRoute(c, channel, "public", false)
	require.NotNil(t, denied)
	require.Equal(t, http.StatusForbidden, denied.StatusCode)
	common.SetContextKey(c, constant.ContextKeyTokenModelLimit, map[string]bool{"public": true, "actual": true})
	require.Nil(t, prepareSelectedModelRoute(c, channel, "public", false))
	require.JSONEq(t, `{"public":"actual"}`, common.GetContextKeyString(c, constant.ContextKeyChannelModelMapping))
	info := &relaycommon.RelayInfo{OriginModelName: "public", TieredBillingSnapshot: &billingexpr.BillingSnapshot{OfficialPricePublicationID: "fixture-publication"}}
	require.ErrorContains(t, service.ValidatePublishedPriceSelectedChannel(c, info), "different upstream model")
	channel.Models = "different"
	denied = prepareSelectedModelRoute(c, channel, "public", false)
	require.NotNil(t, denied)
	require.Equal(t, http.StatusForbidden, denied.StatusCode)
	require.Nil(t, prepareSelectedModelRoute(c, channel, "public", true), "explicit administrator diagnostic can test a not-yet-enabled public name without enabling it")
}
