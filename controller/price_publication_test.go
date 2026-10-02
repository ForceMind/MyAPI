package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relay/helper"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	relaytypes "github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPricePublicationHTTPFreezesSourceBeforeRollback(t *testing.T) {
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/publication-http.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.OfficialPriceVersion{}, &model.PricePublication{}))
	oldDB, oldUnit := model.DB, common.QuotaPerUnit
	model.DB = db
	common.QuotaPerUnit = 500000
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		// If an assertion interrupts the happy path, undo only this fixture's
		// latest publication before restoring the process globals.
		state, e := model.ReadPricePublicationSnapshot(context.Background())
		if e == nil && state.State.Revision == 1 {
			digest, e := state.Digest()
			if e == nil {
				_, _ = model.ApplyPricePublication(context.Background(), model.PricePublicationCommand{ID: strings.Repeat("c", 64), ActorID: 1, ExpectedDigest: digest, Action: "rollback", RollbackOf: strings.Repeat("a", 64)})
			}
		}
		model.DB = oldDB
		common.QuotaPerUnit = oldUnit
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"group_ratio_setting.group_ratio": `{"default":1}`}))
	stateResponse := httptest.NewRecorder()
	stateContext, _ := gin.CreateTestContext(stateResponse)
	stateContext.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_sync/openai/publications", nil)
	GetPricePublicationState(stateContext)
	require.Equal(t, http.StatusOK, stateResponse.Code)
	assert.Contains(t, stateResponse.Body.String(), `"receipts":[]`, "a new installation must expose an empty list rather than null")
	document := `Prices per 1M tokens.
### Standard pricing data
| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| publication-http-fixture | $2 | $0.1 | $2.5 | $10 | $4 | $0.2 | $5 | $15 |

Short context: ≤272K input tokens. Long context: >272K input tokens.
`
	source, err := model.StoreOfficialPriceVersion(context.Background(), db, document, 100)
	require.NoError(t, err)
	previewResponse := httptest.NewRecorder()
	previewCtx, _ := gin.CreateTestContext(previewResponse)
	previewCtx.Request = httptest.NewRequest(http.MethodGet, "/preview", nil)
	previewCtx.Params = gin.Params{{Key: "digest", Value: source.ContentSHA256}}
	PreviewOpenAIPricePublication(previewCtx)
	require.Equal(t, http.StatusOK, previewResponse.Code)
	var preview struct {
		Data service.PricePublicationPreview
	}
	require.NoError(t, common.Unmarshal(previewResponse.Body.Bytes(), &preview))
	require.Len(t, preview.Data.Rows, 1)
	require.True(t, preview.Data.Rows[0].Eligible)
	command := service.PricePublicationRequest{ID: strings.Repeat("a", 64), Action: "publish", ExpectedDigest: preview.Data.ExpectedDigest, SourceSHA256: source.ContentSHA256, Confirmed: true, Models: []service.PricePublicationSelection{{Model: "publication-http-fixture", Locked: common.GetPointer(false)}}}
	post := func(request service.PricePublicationRequest) *httptest.ResponseRecorder {
		body, err := common.Marshal(request)
		require.NoError(t, err)
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/openai/publications", strings.NewReader(string(body)))
		ctx.Set("id", 1)
		ApplyOpenAIPricePublication(ctx)
		return response
	}
	response := post(command)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, http.StatusOK, post(command).Code, "same confirmed request must be idempotent")
	quoteCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	quoteCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	quoteCtx.Set("group", "default")
	info := &relaycommon.RelayInfo{OriginModelName: "publication-http-fixture", UsingGroup: "default", UserGroup: "default", Request: &dto.OpenAIResponsesRequest{}, BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"input":"hello"}`)}}
	_, err = helper.ModelPriceHelper(quoteCtx, info, 100, &relaytypes.TokenCountMeta{MaxTokens: 10})
	require.NoError(t, err)
	require.NotNil(t, info.TieredBillingSnapshot)
	assert.Equal(t, command.ID, info.TieredBillingSnapshot.OfficialPricePublicationID)
	assert.Equal(t, source.ContentSHA256, info.TieredBillingSnapshot.OfficialPriceSourceSHA256)
	current, err := model.ReadPricePublicationSnapshot(context.Background())
	require.NoError(t, err)
	digest, err := current.Digest()
	require.NoError(t, err)
	rollback := service.PricePublicationRequest{ID: strings.Repeat("b", 64), ExpectedDigest: digest, Action: "rollback", RollbackOf: command.ID, Confirmed: true}
	response = post(rollback)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	ok, quota, _ := service.TryTieredSettle(info, billingexpr.TokenParams{P: 100, C: 10, Len: 100})
	require.True(t, ok)
	assert.Equal(t, 150, quota, "in-flight request retains the published expression despite rollback")
	assert.Equal(t, command.ID, info.TieredBillingSnapshot.OfficialPricePublicationID)
	_, active := model.PublishedModelPriceForExpression(info.OriginModelName, info.TieredBillingSnapshot.ExprString)
	assert.False(t, active, "new requests must not receive the rolled-back publication")
}

func TestPricePublicationHTTPRejectsClientRatesAndUnconfirmedWrites(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, body := range []string{`{"rates":{"input":0}}`, `{"confirmed":false}`, `{} {}`, `{"confirmed":true,"id":"wrong"}`, strings.Repeat("x", 16385)} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/openai/publications", strings.NewReader(body))
		ctx.Set("id", 1)
		ApplyOpenAIPricePublication(ctx)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
}
