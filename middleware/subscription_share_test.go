package middleware

import (
	"net/http"
	"net/http/httptest"
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

func subscriptionShareAdmissionFixture(t *testing.T) (*model.Token, *model.Token) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	model.InitColumnNamesForTest()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/subscription-share-admission.db"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.SubscriptionSharePool{},
		&model.SubscriptionShareAllocation{},
	))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})

	user := model.User{Username: "share-admission-owner", AffCode: "share-admission-owner", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	bound := model.Token{UserId: user.Id, Key: "share-admission-bound", Status: common.TokenStatusEnabled, ExpiredTime: -1}
	unbound := model.Token{UserId: user.Id, Key: "share-admission-unbound", Status: common.TokenStatusEnabled, ExpiredTime: -1}
	require.NoError(t, db.Create(&bound).Error)
	require.NoError(t, db.Create(&unbound).Error)
	pool := model.SubscriptionSharePool{
		DescriptorKey:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SubjectRef:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotID:        1,
		ChannelID:         1,
		ChannelName:       "subscription fixture",
		WindowSource:      "codex_wham_usage_primary",
		WindowType:        "five_hour",
		WindowSeconds:     18000,
		PlanType:          "plus",
		Revision:          1,
		CurrentGeneration: 1,
		ConfiguredAt:      1,
		MaxAgeSeconds:     300,
		Enabled:           true,
		Approximate:       true,
	}
	require.NoError(t, db.Create(&pool).Error)
	require.NoError(t, db.Create(&model.SubscriptionShareAllocation{PoolID: pool.ID, TokenID: bound.Id, UserID: user.Id, ShareBPS: 5000, CreatedAt: 1, UpdatedAt: 1}).Error)
	return &bound, &unbound
}

func runSubscriptionShareAdmission(t *testing.T, token *model.Token, method, target string) (bool, common.SubscriptionShareAdmission, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	allowed := checkSubscriptionShareAdmission(ctx, token)
	admission, present := common.SubscriptionShareAdmissionFromContext(ctx.Request.Context())
	require.True(t, present)
	return allowed, admission, recorder
}

func TestSubscriptionShareAdmissionAllowsOnlyReadModelsAndSupportedTextRoutes(t *testing.T) {
	bound, _ := subscriptionShareAdmissionFixture(t)
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/models"},
		{http.MethodGet, "/v1/models/gpt-5"},
		{http.MethodGet, "/v1beta/models"},
		{http.MethodGet, "/v1beta/openai/models"},
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/v1/responses"},
		{http.MethodPost, "/v1/responses/compact"},
		{http.MethodPost, "/v1/alpha/search"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			allowed, admission, recorder := runSubscriptionShareAdmission(t, bound, test.method, test.path)
			assert.True(t, allowed)
			assert.Equal(t, bound.Id, admission.TokenID)
			assert.True(t, admission.Bound)
			assert.Equal(t, http.StatusOK, recorder.Code)
		})
	}
}

func TestSubscriptionShareAdmissionRejectsUnsupportedBoundKeyRoutes(t *testing.T) {
	bound, _ := subscriptionShareAdmissionFixture(t)
	tests := []struct {
		name   string
		method string
		target string
	}{
		{"realtime", http.MethodGet, "/v1/realtime"},
		{"media", http.MethodPost, "/v1/images/generations"},
		{"tasks", http.MethodPost, "/v1/video/generations"},
		{"query", http.MethodPost, "/v1/responses?trace=true"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, admission, recorder := runSubscriptionShareAdmission(t, bound, test.method, test.target)
			assert.False(t, allowed)
			assert.Equal(t, bound.Id, admission.TokenID)
			assert.True(t, admission.Bound)
			assert.Equal(t, http.StatusForbidden, recorder.Code)
		})
	}
}

func TestSubscriptionShareAdmissionLeavesUnboundKeyRoutesUnrestricted(t *testing.T) {
	_, unbound := subscriptionShareAdmissionFixture(t)
	for _, test := range []struct {
		method string
		target string
	}{
		{http.MethodGet, "/v1/realtime"},
		{http.MethodPost, "/v1/images/generations"},
		{http.MethodPost, "/v1/tasks/arbitrary?mode=legacy"},
	} {
		allowed, admission, recorder := runSubscriptionShareAdmission(t, unbound, test.method, test.target)
		assert.True(t, allowed)
		assert.Equal(t, unbound.Id, admission.TokenID)
		assert.False(t, admission.Bound)
		assert.Equal(t, http.StatusOK, recorder.Code)
	}
}
