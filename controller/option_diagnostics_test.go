package controller

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

func TestGetOptionDiagnosticsRejectsInvalidQueryBeforeReadingDiagnostics(t *testing.T) {
	queryReads := optionDiagnosticsControllerFixture(t)
	for _, rawQuery := range []string{
		"include_valid=1",
		"include_valid=true&include_valid=false",
		"include_valid=false&include_valid=false",
		"include_valid=false&repair=true",
		"include_valid=false&repair=%zz",
		"include_valid=true&include_valid=%zz",
		"include_valid=false;repair=true",
		"repair=true;include_valid=false",
		"bad%zz=x",
	} {
		t.Run(rawQuery, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/option/diagnostics?"+rawQuery, nil)
			GetOptionDiagnostics(context)

			var response struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, i18n.MsgInvalidParams, response.Message)
			assert.Zero(t, *queryReads, "invalid query must be rejected before the service reads the database")
		})
	}
}

func optionDiagnosticsControllerFixture(t *testing.T) *int {
	t.Helper()
	previousDB, previousType, previousGinMode, previousTranslateMessage := model.DB, common.MainDatabaseType(), gin.Mode(), common.TranslateMessage
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	gin.SetMode(gin.TestMode)
	common.TranslateMessage = func(_ *gin.Context, key string, _ ...map[string]any) string { return key }
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		gin.SetMode(previousGinMode)
		common.TranslateMessage = previousTranslateMessage
		require.NoError(t, sqlDB.Close())
	})

	queryReads := 0
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("option-diagnostics-controller-row-count", func(*gorm.DB) {
		queryReads++
	}))
	return &queryReads
}
