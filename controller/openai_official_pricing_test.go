package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOfficialPricingSnapshotEndpointDoesNotPublishPrices(t *testing.T) {
	oldFetch := fetchOpenAIOfficialPricing
	t.Cleanup(func() { fetchOpenAIOfficialPricing = oldFetch })
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{"ModelRatio": `{"fixture-model":9}`, "CacheRatio": `{"fixture-model":0.5}`}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
	})
	var fetchedContext context.Context
	fetchOpenAIOfficialPricing = func(ctx context.Context) (*service.OpenAIOfficialPriceSnapshot, error) {
		fetchedContext = ctx
		return &service.OpenAIOfficialPriceSnapshot{SourceURL: "https://developers.openai.com/api/docs/pricing.md", Currency: "USD", UnitTokens: 1_000_000, ServiceTier: "standard", Models: []service.OpenAIOfficialModelPrice{{Model: "fixture-model"}}}, nil
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_sync/openai", nil)
	GetOpenAIOfficialPricing(c)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, c.Request.Context(), fetchedContext)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var result struct {
		Success bool
		Data    service.OpenAIOfficialPriceSnapshot
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success)
	assert.Equal(t, "standard", result.Data.ServiceTier)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, `{"fixture-model":9}`, common.OptionMap["ModelRatio"])
	assert.Equal(t, `{"fixture-model":0.5}`, common.OptionMap["CacheRatio"])
	common.OptionMapRWMutex.RUnlock()
}

func TestOfficialPriceSaveRejectsPostedRatesAndUsesServerSource(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldFetch, oldFreeze := fetchOpenAIOfficialPricing, freezeOpenAIPriceSource
	t.Cleanup(func() { fetchOpenAIOfficialPricing, freezeOpenAIPriceSource = oldFetch, oldFreeze })
	fetched, saved := 0, 0
	snapshot := &service.OpenAIOfficialPriceSnapshot{ContentSHA256: strings.Repeat("a", 64), Scope: "text-token-price-source-not-published"}
	fetchOpenAIOfficialPricing = func(context.Context) (*service.OpenAIOfficialPriceSnapshot, error) { fetched++; return snapshot, nil }
	freezeOpenAIPriceSource = func(_ context.Context, db *gorm.DB, source *service.OpenAIOfficialPriceSnapshot) (*service.OpenAIOfficialPriceSnapshot, error) {
		saved++
		assert.Same(t, model.DB, db)
		assert.Same(t, snapshot, source)
		return snapshot, nil
	}
	for _, test := range []struct {
		path, body string
		want       int
	}{
		{"/api/ratio_sync/openai/versions", `{"input_price":0}`, http.StatusBadRequest},
		{"/api/ratio_sync/openai/versions?url=https://evil.invalid", "", http.StatusBadRequest},
		{"/api/ratio_sync/openai/versions", "", http.StatusOK},
	} {
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		SaveOpenAIOfficialPriceSource(c)
		assert.Equal(t, test.want, response.Code)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	}
	assert.Equal(t, 1, fetched, "rejected client inputs must not trigger source fetching")
	assert.Equal(t, 1, saved)
}

func TestFrozenOfficialPriceReadRejectsUnknownCorruptAndInvalidVersions(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldLoad := loadFrozenOpenAIPriceSource
	t.Cleanup(func() { loadFrozenOpenAIPriceSource = oldLoad })
	for _, test := range []struct {
		name, digest string
		loadError    error
		want         int
		called       bool
	}{
		{"invalid", "bad", nil, http.StatusBadRequest, false},
		{"uppercase", strings.Repeat("A", 64), nil, http.StatusBadRequest, false},
		{"unknown", strings.Repeat("a", 64), gorm.ErrRecordNotFound, http.StatusNotFound, true},
		{"corrupt", strings.Repeat("b", 64), errors.New("source digest mismatch"), http.StatusInternalServerError, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			loadFrozenOpenAIPriceSource = func(_ context.Context, _ *gorm.DB, digest string) (*service.OpenAIOfficialPriceSnapshot, error) {
				called = true
				assert.Equal(t, test.digest, digest)
				return nil, test.loadError
			}
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Params = gin.Params{{Key: "digest", Value: test.digest}}
			c.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_sync/openai/versions/"+test.digest, nil)
			GetFrozenOpenAIOfficialPriceSource(c)
			assert.Equal(t, test.want, response.Code)
			assert.Equal(t, test.called, called)
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			var result map[string]any
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.NotContains(t, result, "data", "never return latest/zero rates for an invalid version")
		})
	}
}

func TestOfficialPriceSaveFailuresDoNotReturnFallbackSource(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldFetch, oldFreeze := fetchOpenAIOfficialPricing, freezeOpenAIPriceSource
	t.Cleanup(func() { fetchOpenAIOfficialPricing, freezeOpenAIPriceSource = oldFetch, oldFreeze })
	for _, test := range []struct {
		name              string
		fetchErr, saveErr error
		want              int
		code              string
	}{
		{"fetch failure", errors.New("source unavailable"), nil, http.StatusBadGateway, "official_pricing_unavailable"},
		{"save failure", nil, errors.New("database unavailable"), http.StatusInternalServerError, "official_pricing_source_save_failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fetchOpenAIOfficialPricing = func(context.Context) (*service.OpenAIOfficialPriceSnapshot, error) {
				return &service.OpenAIOfficialPriceSnapshot{}, test.fetchErr
			}
			freezeOpenAIPriceSource = func(context.Context, *gorm.DB, *service.OpenAIOfficialPriceSnapshot) (*service.OpenAIOfficialPriceSnapshot, error) {
				return nil, test.saveErr
			}
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/ratio_sync/openai/versions", nil)
			SaveOpenAIOfficialPriceSource(c)
			assert.Equal(t, test.want, response.Code)
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			var result map[string]any
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.Equal(t, test.code, result["code"])
			assert.NotContains(t, result, "data")
		})
	}
}

func TestFrozenOfficialPriceReadReturnsSelectedVersionWithoutFetchingLatest(t *testing.T) {
	oldFetch, oldLoad := fetchOpenAIOfficialPricing, loadFrozenOpenAIPriceSource
	t.Cleanup(func() { fetchOpenAIOfficialPricing, loadFrozenOpenAIPriceSource = oldFetch, oldLoad })
	digest := strings.Repeat("a", 64)
	fetchOpenAIOfficialPricing = func(context.Context) (*service.OpenAIOfficialPriceSnapshot, error) {
		t.Error("version read must not fetch latest")
		return nil, errors.New("unexpected latest fetch")
	}
	loadFrozenOpenAIPriceSource = func(ctx context.Context, db *gorm.DB, selected string) (*service.OpenAIOfficialPriceSnapshot, error) {
		assert.Equal(t, digest, selected)
		assert.Same(t, model.DB, db)
		return &service.OpenAIOfficialPriceSnapshot{ContentSHA256: digest, Scope: "text-token-price-source-not-published"}, nil
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Params = gin.Params{{Key: "digest", Value: digest}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_sync/openai/versions/"+digest, nil)
	GetFrozenOpenAIOfficialPriceSource(c)
	assert.Equal(t, http.StatusOK, response.Code)
	var result struct {
		Success bool
		Data    service.OpenAIOfficialPriceSnapshot
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.True(t, result.Success)
	assert.Equal(t, digest, result.Data.ContentSHA256)
	assert.Equal(t, "text-token-price-source-not-published", result.Data.Scope)
}

func TestOfficialPricingSnapshotEndpointFailsWithoutFallbackPrices(t *testing.T) {
	require.NoError(t, i18n.Init())
	oldFetch := fetchOpenAIOfficialPricing
	t.Cleanup(func() { fetchOpenAIOfficialPricing = oldFetch })
	fetchOpenAIOfficialPricing = func(context.Context) (*service.OpenAIOfficialPriceSnapshot, error) {
		return nil, errors.New("synthetic source unavailable")
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_sync/openai", nil)
	GetOpenAIOfficialPricing(c)
	assert.Equal(t, http.StatusBadGateway, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	var result map[string]any
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	assert.Equal(t, false, result["success"])
	assert.Equal(t, "official_pricing_unavailable", result["code"])
	assert.NotContains(t, result, "data")
}
