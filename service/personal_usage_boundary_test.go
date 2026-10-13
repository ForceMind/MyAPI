package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var personalCommercialTables = []string{
	"user_subscriptions",
	"subscription_plans",
	"subscription_pre_consume_records",
	"subscription_orders",
	"top_ups",
}

var personalCommercialOperations = []string{"query", "create", "update", "delete", "row", "raw"}

type personalCommercialGuard struct {
	tables map[string]struct{}
	hits   map[string]map[string]int
	err    error
}

func newPersonalCommercialGuard() *personalCommercialGuard {
	guard := &personalCommercialGuard{tables: make(map[string]struct{}), hits: make(map[string]map[string]int), err: errors.New("commercial table access blocked")}
	for _, table := range personalCommercialTables {
		guard.tables[table] = struct{}{}
	}
	guard.reset()
	return guard
}

func (g *personalCommercialGuard) reset() {
	g.hits = make(map[string]map[string]int, len(personalCommercialOperations))
	for _, operation := range personalCommercialOperations {
		g.hits[operation] = make(map[string]int, len(personalCommercialTables))
	}
}

func (g *personalCommercialGuard) table(statement *gorm.Statement) string {
	if statement == nil {
		return ""
	}
	if _, ok := g.tables[statement.Table]; ok {
		return statement.Table
	}
	for _, token := range strings.FieldsFunc(strings.ToLower(statement.SQL.String()), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	}) {
		if _, ok := g.tables[token]; ok {
			return token
		}
	}
	return ""
}

func (g *personalCommercialGuard) callback(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		if table := g.table(tx.Statement); table != "" {
			g.hits[operation][table]++
			tx.AddError(g.err)
		}
	}
}

func installPersonalCommercialGuard(t *testing.T, db *gorm.DB) *personalCommercialGuard {
	t.Helper()
	guard := newPersonalCommercialGuard()
	registrations := []struct {
		operation string
		register  func(string, func(*gorm.DB)) error
		remove    func(string) error
	}{
		{"query", func(name string, fn func(*gorm.DB)) error {
			return db.Callback().Query().Before("gorm:query").Register(name, fn)
		}, db.Callback().Query().Remove},
		{"create", func(name string, fn func(*gorm.DB)) error {
			return db.Callback().Create().Before("gorm:create").Register(name, fn)
		}, db.Callback().Create().Remove},
		{"update", func(name string, fn func(*gorm.DB)) error {
			return db.Callback().Update().Before("gorm:update").Register(name, fn)
		}, db.Callback().Update().Remove},
		{"delete", func(name string, fn func(*gorm.DB)) error {
			return db.Callback().Delete().Before("gorm:delete").Register(name, fn)
		}, db.Callback().Delete().Remove},
		{"row", func(name string, fn func(*gorm.DB)) error {
			return db.Callback().Row().Before("gorm:row").Register(name, fn)
		}, db.Callback().Row().Remove},
		{"raw", func(name string, fn func(*gorm.DB)) error {
			return db.Callback().Raw().Before("gorm:raw").Register(name, fn)
		}, db.Callback().Raw().Remove},
	}
	for _, registration := range registrations {
		name := "personal-commercial-" + registration.operation
		require.NoError(t, registration.register(name, guard.callback(registration.operation)))
		t.Cleanup(func() { require.NoError(t, registration.remove(name)) })
	}
	return guard
}

func verifyPersonalCommercialGuard(t *testing.T, db *gorm.DB, guard *personalCommercialGuard) {
	t.Helper()
	type row struct{ ID int }
	for _, table := range personalCommercialTables {
		var rows []row
		require.ErrorIs(t, db.Table(table).Where("1 = 0").Find(&rows).Error, guard.err)
		require.ErrorIs(t, db.Table(table).Create(&row{ID: -1}).Error, guard.err)
		require.ErrorIs(t, db.Table(table).Where("1 = 0").UpdateColumn("id", gorm.Expr("id")).Error, guard.err)
		require.ErrorIs(t, db.Table(table).Where("1 = 0").Delete(&row{}).Error, guard.err)
		sqlRows, err := db.Table(table).Where("1 = 0").Select("id").Rows()
		require.ErrorIs(t, err, guard.err)
		require.Nil(t, sqlRows)
		require.ErrorIs(t, db.Exec("UPDATE "+table+" SET id = id WHERE 1 = 0").Error, guard.err)
	}
	for _, operation := range personalCommercialOperations {
		for _, table := range personalCommercialTables {
			require.Equal(t, 1, guard.hits[operation][table], "%s guard must observe its %s positive control", operation, table)
		}
	}
	guard.reset()
}

func TestPersonalUsageAdmissionAvoidsCommercialTablesAcrossWriters(t *testing.T) {
	for _, mode := range []model.QuotaWriterMode{model.QuotaWriterModeLegacy, model.QuotaWriterModeAuthoritative} {
		t.Run(string(mode), func(t *testing.T) {
			db := setupPostConsumeModeDB(t, mode)
			require.NoError(t, db.AutoMigrate(&model.Option{}, &model.SubscriptionPreConsumeRecord{}, &model.SubscriptionOrder{}, &model.TopUp{}))
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				_, err := model.InitializeUserFundingStateTx(tx, operation_setting.UserFundingModeDisabled)
				return err
			}))
			label := "pb-l"
			if mode == model.QuotaWriterModeAuthoritative {
				label = "pb-a"
			}
			user, token := seedAuthoritativeBilling(t, db, label, 0, 100, false)
			require.NoError(t, db.Model(user).Updates(map[string]any{"self_use_no_balance": true, "usage_policy_revision": 1}).Error)
			user.SetSetting(dto.UserSetting{BillingPreference: "subscription_only"})
			require.NoError(t, db.Model(user).Update("setting", user.Setting).Error)

			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			var session *BillingSession
			t.Cleanup(func() {
				if session != nil {
					require.NoError(t, session.Refund(ctx))
				}
			})
			guard := installPersonalCommercialGuard(t, db)
			verifyPersonalCommercialGuard(t, db, guard)

			info := authoritativeRelay(user, token, "personal-boundary-"+string(mode))
			info.UserSetting.BillingPreference = "subscription_only"
			created, apiErr := NewBillingSession(ctx, info, 60)
			session = created
			require.Nil(t, apiErr)
			require.NotNil(t, session)
			assert.Equal(t, BillingSourceSelfUse, session.funding.Source())
			assert.Equal(t, BillingSourceSelfUse, info.BillingSource)
			for _, operation := range personalCommercialOperations {
				for _, table := range personalCommercialTables {
					assert.Zero(t, guard.hits[operation][table], "%s must not access %s during SQLite admission/pre-consume", operation, table)
				}
			}
			wallet, remaining, used := loadPostConsumeBalances(t, db, user, token)
			assert.Zero(t, wallet)
			assert.Equal(t, int64(40), remaining)
			assert.Equal(t, int64(60), used)
		})
	}
}

func TestSupportsSelfUseMeteredRequestNilAndExistingEligibility(t *testing.T) {
	assert.False(t, SupportsSelfUseMeteredRequest(nil))
	assert.False(t, SupportsSelfUseMeteredRequest(&http.Request{}))
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/responses/compact"} {
		assert.True(t, SupportsSelfUseMeteredRequest(httptest.NewRequest(http.MethodPost, path, nil)), path)
	}
	assert.False(t, SupportsSelfUseMeteredRequest(httptest.NewRequest(http.MethodGet, "/v1/responses", nil)))
	assert.False(t, SupportsSelfUseMeteredRequest(httptest.NewRequest(http.MethodPost, "/v1/responses?trace=1", nil)))
	assert.False(t, SupportsSelfUseMeteredRequest(httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)))
}
