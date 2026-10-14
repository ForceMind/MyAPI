package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func subscriptionShareControllerFixture(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/share-api.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	entities := []any{&model.User{}, &model.Token{}, &model.TokenBudget{}, &model.Channel{}, &model.ChannelQuotaSnapshot{}}
	entities = append(entities, model.SubscriptionShareModels()...)
	require.NoError(t, db.AutoMigrate(entities...))
	for _, user := range []model.User{{Id: 1, Username: "share-api-root", AffCode: "share-api-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled}, {Id: 2, Username: "share-api-owner", AffCode: "share-api-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}, {Id: 3, Username: "share-api-other", AffCode: "share-api-other", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}} {
		require.NoError(t, db.Create(&user).Error)
	}
	require.NoError(t, db.Create(&model.Token{Id: 11, UserId: 2, Key: "share-api-key", Status: common.TokenStatusEnabled, ExpiredTime: -1}).Error)
	return db
}

func subscriptionShareControllerRequest(actor, token int, method, suffix, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, "/api/token/"+strconv.Itoa(token)+"/subscription-share"+suffix, strings.NewReader(body))
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token)}}
	c.Set("id", actor)
	if method == http.MethodGet {
		GetSubscriptionShares(c)
	} else {
		UpdateSubscriptionShare(c)
	}
	return recorder
}

func TestSubscriptionShareAPIOwnershipAndNoStore(t *testing.T) {
	subscriptionShareControllerFixture(t)
	response := subscriptionShareControllerRequest(2, 11, http.MethodGet, "", "")
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Contains(t, response.Body.String(), `"pools":[]`)
	assert.NotContains(t, response.Body.String(), "share-api-key")
	for _, request := range []struct{ actor, token int }{{3, 11}, {2, 999}} {
		response := subscriptionShareControllerRequest(request.actor, request.token, http.MethodGet, "", "")
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Contains(t, response.Body.String(), "subscription_share_not_found")
	}
}

func TestSubscriptionShareAPIRejectsAmbiguousMutationWithoutWriting(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	for _, body := range []string{
		`{"id":"bad","pool_id":0,"snapshot_id":1,"expected_revision":0,"confirmed_approximate":true}`,
		`{"id":"bad","pool_id":0,"snapshot_id":1,"expected_revision":0,"share_bps":0,"confirmed_approximate":false}`,
		`{"id":"bad","pool_id":0,"snapshot_id":1,"expected_revision":0,"share_bps":20,"confirmed_approximate":true,"extra":1}`,
		`{"id":"bad","pool_id":0,"snapshot_id":1,"expected_revision":0,"share_bps":20,"confirmed_approximate":true} {}`,
	} {
		response := subscriptionShareControllerRequest(1, 11, http.MethodPut, "", body)
		assert.Equal(t, http.StatusBadRequest, response.Code, body)
	}
	response := subscriptionShareControllerRequest(1, 11, http.MethodGet, "?window=all", "")
	assert.Equal(t, http.StatusBadRequest, response.Code)
	var count int64
	require.NoError(t, db.Model(&model.SubscriptionShareAllocation{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestSubscriptionShareAPIUnavailableDatabaseDoesNotLookLikeNotFound(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	response := subscriptionShareControllerRequest(2, 11, http.MethodGet, "", "")
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Contains(t, response.Body.String(), "subscription_share_unavailable")
	assert.NotContains(t, response.Body.String(), "database is closed")
}

func TestSubscriptionShareTokenDeletionReturnsActionableConflict(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	pool := model.SubscriptionSharePool{DescriptorKey: "delete-api-fixture", Enabled: true}
	require.NoError(t, db.Create(&pool).Error)
	require.NoError(t, db.Create(&model.SubscriptionShareAllocation{PoolID: pool.ID, TokenID: 11, UserID: 2, ShareBPS: 2000}).Error)
	for _, batch := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", 2)
		c.Params = gin.Params{{Key: "id", Value: "11"}}
		c.Request = httptest.NewRequest(http.MethodDelete, "/api/token/11", strings.NewReader(`{"ids":[11]}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("Accept-Language", "en")
		if batch {
			DeleteTokenBatch(c)
		} else {
			DeleteToken(c)
		}
		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "subscription_share_delete_denied")
		assert.Contains(t, recorder.Body.String(), "0%")
		assert.NotContains(t, recorder.Body.String(), "allocation must be set")
		var token model.Token
		require.NoError(t, db.First(&token, 11).Error)
	}
}

func TestSubscriptionShareCloseAPIRequiresExplicitUnambiguousConfirmation(t *testing.T) {
	db := subscriptionShareControllerFixture(t)
	pool := model.SubscriptionSharePool{DescriptorKey: "close-api-fixture", Enabled: true, Revision: 1}
	require.NoError(t, db.Create(&pool).Error)
	for _, body := range []string{
		`{"id":"bad","confirmed_close":true}`,
		`{"id":"bad","expected_revision":1,"confirmed_close":false}`,
		`{"id":"bad","expected_revision":1,"confirmed_close":true,"share_bps":0}`,
		`{"id":"bad","expected_revision":1,"confirmed_close":true} {}`,
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", 1)
		c.Params = gin.Params{{Key: "pool_id", Value: strconv.FormatInt(pool.ID, 10)}}
		c.Request = httptest.NewRequest(http.MethodPost, "/api/subscription-share/pools/1/close", strings.NewReader(body))
		CloseSubscriptionSharePool(c)
		assert.Equal(t, http.StatusBadRequest, recorder.Code, body)
		assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	}
	var unchanged model.SubscriptionSharePool
	require.NoError(t, db.First(&unchanged, pool.ID).Error)
	assert.True(t, unchanged.Enabled)
	assert.EqualValues(t, 1, unchanged.Revision)
	var audits int64
	require.NoError(t, db.Model(&model.SubscriptionShareAudit{}).Count(&audits).Error)
	assert.Zero(t, audits)
}
