package model

import (
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNewTaskBillingContextCapturesIndependentRates(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "snapshot-model", PriceData: types.PriceData{
		ModelPrice: -1, ModelRatio: 2,
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 0.5, GroupSpecialRatio: 0.5, HasSpecialRatio: true},
	}}
	info.PriceData.AddOtherRatio("duration", 3)
	context := NewTaskBillingContext(info)
	require.NotNil(t, context)
	info.PriceData.AddOtherRatio("duration", 7)
	info.PriceData.ModelRatio = 4
	info.PriceData.GroupRatioInfo.GroupRatio = 1
	assert.Equal(t, TaskBillingContextVersion, context.Version)
	assert.True(t, context.Complete)
	assert.Equal(t, 2.0, context.ModelRatio)
	assert.Equal(t, 0.5, context.GroupRatio)
	assert.Equal(t, map[string]float64{"duration": 3}, context.OtherRatios)
	assert.False(t, context.PerCallBilling)
}

// The caller supplies an isolated database. This same fixture runs against
// SQLite locally and the existing configured MySQL/PostgreSQL test databases.
func runTaskBillingSnapshotRoundTrip(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&Task{}))
	for _, tc := range []struct {
		name    string
		context TaskBillingContext
	}{
		{"priced", TaskBillingContext{Version: 1, Complete: true, ModelPrice: -1, ModelRatio: 2, GroupRatio: 0.5, OriginModelName: "snapshot-model", OtherRatios: map[string]float64{"duration": 3, "discount": 0.5}}},
		{"free_model", TaskBillingContext{Version: 1, Complete: true, ModelPrice: -1, ModelRatio: 0, GroupRatio: 0.5, OriginModelName: "snapshot-model", OtherRatios: map[string]float64{"duration": 3}}},
		{"free_group", TaskBillingContext{Version: 1, Complete: true, ModelPrice: -1, ModelRatio: 2, GroupRatio: 0, OriginModelName: "snapshot-model"}},
		{"per_call", TaskBillingContext{Version: 1, Complete: true, ModelPrice: 0.2, ModelRatio: 0, GroupRatio: 0.5, OriginModelName: "snapshot-model", PerCallBilling: true}},
		{"legacy", TaskBillingContext{ModelRatio: 2, GroupRatio: 0.5, OriginModelName: "snapshot-model"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := Task{TaskID: "billing-snapshot-" + tc.name, Properties: Properties{OriginModelName: "snapshot-model", UpstreamModelName: "provider-model"}, PrivateData: TaskPrivateData{BillingContext: &tc.context}}
			require.NoError(t, db.Create(&task).Error)
			t.Cleanup(func() { require.NoError(t, db.Delete(&Task{}, task.ID).Error) })
			var loaded Task
			require.NoError(t, db.First(&loaded, task.ID).Error)
			require.NotNil(t, loaded.PrivateData.BillingContext)
			assert.Equal(t, tc.context, *loaded.PrivateData.BillingContext)
			assert.Equal(t, task.Properties, loaded.Properties)
			var stored struct{ PrivateData string }
			require.NoError(t, db.Model(&Task{}).Select("private_data").Where("id = ?", task.ID).Scan(&stored).Error)
			var raw map[string]any
			require.NoError(t, common.UnmarshalJsonStr(stored.PrivateData, &raw))
			context, ok := raw["billing_context"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.context.ModelRatio, context["model_ratio"])
			assert.Equal(t, tc.context.GroupRatio, context["group_ratio"])
			if tc.context.Version == 1 {
				assert.Equal(t, float64(1), context["version"])
				assert.Equal(t, true, context["complete"])
			}
		})
	}
}

func TestTaskBillingSnapshotRoundTrip(t *testing.T) {
	runTaskBillingSnapshotRoundTrip(t, DB)
}

func TestTaskJSONScanAcceptsDriverRepresentationsAndClearsStaleValues(t *testing.T) {
	privateJSON := `{"billing_context":{"version":1,"complete":true,"model_ratio":0,"group_ratio":0.5,"origin_model_name":"snapshot-model","other_ratios":{"duration":3}}}`
	propertiesJSON := `{"origin_model_name":"snapshot-model","upstream_model_name":"provider-model"}`
	for _, tc := range []struct {
		name                string
		private, properties any
		empty, invalid      bool
	}{
		{"bytes", []byte(privateJSON), []byte(propertiesJSON), false, false},
		{"string", privateJSON, propertiesJSON, false, false},
		{"nil", nil, nil, true, false},
		{"empty bytes", []byte{}, []byte{}, true, false},
		{"empty string", "", "", true, false},
		{"empty object", "{}", "{}", true, false},
		{"null", "null", "null", true, false},
		{"invalid JSON", "{", "{", true, true},
		{"unsupported driver type", 12, 12, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			private := TaskPrivateData{Key: "stale-key", BillingContext: &TaskBillingContext{ModelRatio: 99}}
			properties := Properties{Input: "stale-input", OriginModelName: "stale-model"}
			privateErr, propertiesErr := private.Scan(tc.private), properties.Scan(tc.properties)
			if tc.invalid {
				require.Error(t, privateErr)
				require.Error(t, propertiesErr)
			} else {
				require.NoError(t, privateErr)
				require.NoError(t, propertiesErr)
			}
			if tc.empty {
				assert.Equal(t, TaskPrivateData{}, private)
				assert.Equal(t, Properties{}, properties)
				return
			}
			require.NotNil(t, private.BillingContext)
			assert.Equal(t, 1, private.BillingContext.Version)
			assert.True(t, private.BillingContext.Complete)
			assert.Zero(t, private.BillingContext.ModelRatio)
			assert.Equal(t, 0.5, private.BillingContext.GroupRatio)
			assert.Equal(t, map[string]float64{"duration": 3}, private.BillingContext.OtherRatios)
			assert.Empty(t, private.Key)
			assert.Equal(t, Properties{OriginModelName: "snapshot-model", UpstreamModelName: "provider-model"}, properties)
		})
	}
}
