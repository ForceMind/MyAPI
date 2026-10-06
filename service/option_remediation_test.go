package service

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDryRunOptionRemediationsIsZeroWriteAndValueFree(t *testing.T) {
	db, restore := optionRemediationServiceFixture(t)
	defer restore()
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	require.NoError(t, db.AutoMigrate(&model.OptionRemediation{}, &model.OptionRemediationBackup{}))
	const canary = "c09-remediation-service-canary"
	require.NoError(t, db.Create(&model.Option{Key: "passkey.enabled", Value: " true "}).Error)
	require.NoError(t, db.Create(&model.Option{Key: "unknown.prefix", Value: canary}).Error)
	require.NoError(t, db.Create(&model.Option{Key: "GroupRatio", Value: `{"default":1}`}).Error)
	require.NoError(t, db.Create(&model.Option{Key: "group_ratio_setting.group_ratio", Value: `{"default":2}`}).Error)
	require.NoError(t, db.Create(&model.Option{Key: "global.thinking_model_blacklist", Value: "{bad"}).Error)

	writes := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("option-remediation-service-create", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("option-remediation-service-update", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register("option-remediation-service-delete", func(*gorm.DB) { writes++ }))
	optionMapBefore := snapshotOptionRemediationServiceOptionMap()
	quotaBefore := operation_setting.GetQuotaSetting()

	preview, err := DryRunOptionRemediations(nil, true, nil)
	require.NoError(t, err)
	assert.Zero(t, writes)
	assert.Equal(t, optionMapBefore, snapshotOptionRemediationServiceOptionMap())
	assert.Equal(t, *quotaBefore, *operation_setting.GetQuotaSetting())

	assert.Equal(t, OptionRemediationSchemaVersion, preview.SchemaVersion)
	assert.Equal(t, "dry_run", preview.Mode)
	assert.Equal(t, 2, preview.Summary.ByDecision[model.OptionRemediationDecisionRepair])
	assert.Equal(t, 1, preview.Summary.ByDecision[model.OptionRemediationDecisionReject])
	assert.Equal(t, preview.Summary.Returned, len(preview.Items))

	encoded, err := common.Marshal(preview)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), canary)
	assert.NotContains(t, string(encoded), `" true "`)
	assert.NotContains(t, string(encoded), `{"default":1}`)

	var trimItem, aliasItem *OptionRemediationDryRunItem
	for index := range preview.Items {
		item := &preview.Items[index]
		switch item.Key {
		case "passkey.enabled":
			trimItem = item
		case "group_ratio_setting.group_ratio":
			aliasItem = item
		}
		if item.KeyRedacted {
			assert.Empty(t, item.Key)
			assert.Empty(t, item.RuntimeValueHash)
		}
	}
	require.NotNil(t, trimItem)
	assert.Equal(t, model.OptionRemediationActionTrimSpace, trimItem.Action)
	assert.Equal(t, model.OptionRemediationRiskMedium, trimItem.Risk)
	assert.NotEmpty(t, trimItem.OldValueHash)
	assert.NotEmpty(t, trimItem.NewValueHash)
	assert.NotEqual(t, trimItem.OldValueHash, trimItem.NewValueHash)
	require.NotNil(t, aliasItem)
	assert.Equal(t, model.OptionRemediationActionGroupRatioSync, aliasItem.Action)
	assert.Equal(t, model.OptionRemediationRiskLow, aliasItem.Risk)

	// A remediation registry row is never a dry-run side effect.
	var remediationRows int64
	require.NoError(t, db.Model(&model.OptionRemediation{}).Count(&remediationRows).Error)
	assert.Zero(t, remediationRows)
}

func TestDryRunOptionRemediationsRejectsUnknownActions(t *testing.T) {
	_, restore := optionRemediationServiceFixture(t)
	defer restore()
	_, err := DryRunOptionRemediations([]string{"passkey.enabled"}, false, []string{"drop.table"})
	require.ErrorIs(t, err, model.ErrOptionRemediationInput)
}

func TestApplyOptionRemediationsRoundTripFromDryRun(t *testing.T) {
	db, restore := optionRemediationServiceFixture(t)
	defer restore()
	require.NoError(t, db.Exec("CREATE TABLE options (`key` text primary key, value text)").Error)
	require.NoError(t, db.AutoMigrate(&model.OptionRemediation{}, &model.OptionRemediationBackup{}))
	require.NoError(t, db.Create(&model.Option{Key: "GroupRatio", Value: `{"default":1}`}).Error)
	require.NoError(t, db.Create(&model.Option{Key: "group_ratio_setting.group_ratio", Value: `{"default":2}`}).Error)

	previousGroupRatio := ratio_setting.GroupRatio2JSONString()
	optionMapBefore := snapshotOptionRemediationServiceOptionMap()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatio))
		restoreOptionRemediationServiceOptionMap(optionMapBefore)
	})
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMapRWMutex.Unlock()

	// The apply request is rebuilt from the dry-run output exactly like an
	// operator would carry it over: key, action, and the two hashes.
	preview, err := DryRunOptionRemediations([]string{"GroupRatio"}, false, nil)
	require.NoError(t, err)
	var applyItem model.OptionRemediationApplyItem
	for _, item := range preview.Items {
		if item.Decision != model.OptionRemediationDecisionRepair {
			continue
		}
		applyItem = model.OptionRemediationApplyItem{
			Key:                  item.Key,
			Action:               item.Action,
			ExpectedOldValueHash: item.OldValueHash,
			ExpectedNewValueHash: item.NewValueHash,
		}
	}
	require.NotEmpty(t, applyItem.Key)

	response, err := ApplyOptionRemediations(11, []model.OptionRemediationApplyItem{applyItem})
	require.NoError(t, err)
	assert.Equal(t, 1, response.Summary.Applied)
	assert.Zero(t, response.Summary.Failed)
	require.Len(t, response.Items, 1)
	assert.Equal(t, model.OptionRemediationApplyApplied, response.Items[0].Status)
	assert.NotZero(t, response.Items[0].RemediationId)

	encoded, err := common.Marshal(response)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `{"default":1}`)

	remediations, total, err := ListOptionRemediations(0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, remediations, 1)
	assert.Equal(t, model.OptionRemediationStatusApplied, remediations[0].Status)
	assert.Equal(t, 11, remediations[0].OperatorUserId)
	encodedRegistry, err := common.Marshal(remediations)
	require.NoError(t, err)
	assert.NotContains(t, string(encodedRegistry), `{"default":1}`)

	// The runtime publication chain ran: the live ratio setting now matches
	// the repaired canonical value.
	assert.Equal(t, `{"default":1}`, ratio_setting.GroupRatio2JSONString())
}

func optionRemediationServiceFixture(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	previousDB, previousType, previousGinMode := model.DB, common.MainDatabaseType(), gin.Mode()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	gin.SetMode(gin.TestMode)
	restore := func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		gin.SetMode(previousGinMode)
		require.NoError(t, sqlDB.Close())
	}
	return db, restore
}

func snapshotOptionRemediationServiceOptionMap() map[string]string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if common.OptionMap == nil {
		return nil
	}
	snapshot := make(map[string]string, len(common.OptionMap))
	for key, value := range common.OptionMap {
		snapshot[key] = value
	}
	return snapshot
}

func restoreOptionRemediationServiceOptionMap(snapshot map[string]string) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	common.OptionMap = snapshot
}
