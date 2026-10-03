package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUsageReviewActualRouteOwnershipAndRecovery(t *testing.T) {
	require.NoError(t, i18n.Init())
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/review.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.LegacyUsageReservation{}, &model.UsageReviewDecision{}, &model.AccountQuotaReservationHead{}, &model.AccountQuotaTerminalRecoveryObligation{}, &model.AccountQuotaSettlementIntent{}, &model.AccountQuotaSettlementFact{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldRedis, oldMemory := common.RedisEnabled, common.MemoryCacheEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldCritical, oldGlobal := common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = false, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = oldRedis, oldMemory
		common.CriticalRateLimitEnable, common.GlobalApiRateLimitEnable = oldCritical, oldGlobal
		common.SetDatabaseTypes(oldMain, oldLog)
		_ = pool.Close()
	})
	var owner, root model.User
	for _, entry := range []struct {
		name string
		role int
	}{{"owner", common.RoleCommonUser}, {"outsider", common.RoleCommonUser}, {"admin", common.RoleAdminUser}, {"root", common.RoleRootUser}} {
		pat := "usage-review-" + entry.name
		user := model.User{Username: pat, AffCode: pat, Role: entry.role, Status: common.UserStatusEnabled, AccessToken: &pat}
		require.NoError(t, db.Create(&user).Error)
		if entry.name == "owner" {
			owner = user
		}
		if entry.name == "root" {
			root = user
		}
	}
	_, err = model.PrepareLegacyUsageReservation(context.Background(), db, model.LegacyUsageReservation{RequestID: "review-request", UserID: owner.Id, TokenID: 42, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
	require.NoError(t, err)
	require.NoError(t, model.UpdateLegacyUsageReservation(context.Background(), db, "review-request", 100, 100, model.LegacyUsageUnknown, "missing", nil))
	engine := gin.New()
	engine.Use(func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyAuditLogged, true); c.Next() })
	SetApiRouter(engine)
	request := func(method, identity, origin, body string, override ...string) *httptest.ResponseRecorder {
		path := "http://myapi.local/api/usage-review/review-request"
		if method == http.MethodPost {
			path += "/reconcile"
		}
		if len(override) == 1 {
			path = "http://myapi.local" + override[0]
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if identity != "" {
			req.Header.Set("Authorization", "Bearer usage-review-"+identity)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "", "", "").Code)
	assert.Equal(t, http.StatusNotFound, request(http.MethodGet, "outsider", "", "").Code)
	assert.Equal(t, http.StatusOK, request(http.MethodGet, "owner", "", "").Code)
	for _, actor := range []string{"owner", "admin"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodPost, actor, "http://myapi.local", "{}").Code)
	}
	for _, origin := range []string{"", "http://evil.invalid"} {
		response := request(http.MethodPost, "root", origin, "{}")
		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
	}
	assert.Equal(t, http.StatusBadRequest, request(http.MethodPost, "root", "http://myapi.local", `{"actual_quota":0,"evidence_reference":"fixture receipt"}`).Code)
	var count int64
	require.NoError(t, db.Model(&model.UsageReviewDecision{}).Count(&count).Error)
	assert.Zero(t, count)
	body := `{"actual_quota":100,"evidence_reference":"synthetic receipt with frozen price review","confirmed_reliable_evidence":true}`
	for range 2 {
		assert.Equal(t, http.StatusOK, request(http.MethodPost, "root", "http://myapi.local", body).Code)
	}
	require.NoError(t, db.Model(&model.UsageReviewDecision{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	var record model.LegacyUsageReservation
	require.NoError(t, db.Where("request_id = ?", "review-request").First(&record).Error)
	assert.Equal(t, model.LegacyUsageSettled, record.State)
	assert.Equal(t, root.Id, record.ReviewedBy)
	conflict := strings.Replace(body, `"actual_quota":100`, `"actual_quota":120`, 1)
	assert.Equal(t, http.StatusConflict, request(http.MethodPost, "root", "http://myapi.local", conflict).Code)
	_, err = model.PrepareLegacyUsageReservation(context.Background(), db, model.LegacyUsageReservation{RequestID: "dispatch-route", UserID: owner.Id, TokenID: 42, FundingSource: "wallet", ReservedQuota: 100, TokenReservedQuota: 100})
	require.NoError(t, err)
	require.NoError(t, model.SetTextDispatchEvidence(context.Background(), db, "dispatch-route", owner.Id, 42, 0, `{"version":1}`, true))
	pendingPath := "/api/usage-reviews/pending?writer=legacy&after=0"
	for _, actor := range []string{"owner", "admin"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodGet, actor, "", "", pendingPath).Code)
	}
	assert.Equal(t, http.StatusOK, request(http.MethodGet, "root", "", "", pendingPath).Code)
	assert.Equal(t, http.StatusBadRequest, request(http.MethodGet, "root", "", "", pendingPath+"&limit=999999").Code)
	recoverPath := "/api/usage-review/dispatch-route/recover-dispatch"
	for _, actor := range []string{"owner", "admin"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodPost, actor, "http://myapi.local", body, recoverPath).Code)
	}
	assert.Equal(t, http.StatusForbidden, request(http.MethodPost, "root", "http://evil.invalid", body, recoverPath).Code)
	assert.Equal(t, http.StatusBadRequest, request(http.MethodPost, "root", "http://myapi.local", body, recoverPath).Code)
	recoveryBody := strings.TrimSuffix(body, "}") + `,"confirmed_request_finished":true}`
	for range 2 {
		assert.Equal(t, http.StatusOK, request(http.MethodPost, "root", "http://myapi.local", recoveryBody, recoverPath).Code)
	}

}
