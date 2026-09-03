package model

import (
	"database/sql/driver"
	"math"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/types"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
			assert.Nil(t, loaded.Data, "an absent provider result must remain SQL NULL")
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

func TestTaskJSONValuesEncodeForPostgresSimpleProtocol(t *testing.T) {
	// Generate actual PostgreSQL GORM bindings without connecting to a server.
	// The existing SQLite connection is never used: ping, transactions and
	// execution are disabled for this synthetic INSERT.
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: DB.ConnPool}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	task := Task{
		TaskID:     "synthetic-pg-json-parameters",
		Properties: Properties{OriginModelName: "snapshot-model", UpstreamModelName: "provider-model"},
		PrivateData: TaskPrivateData{BillingContext: &TaskBillingContext{
			Version: 1, Complete: true, ModelRatio: 0, GroupRatio: 0.5, OriginModelName: "snapshot-model",
			OtherRatios: map[string]float64{"duration": 3},
		}},
	}
	statement := db.Create(&task).Statement
	require.NoError(t, statement.Error)
	values, ok := statement.Clauses["VALUES"].Expression.(clause.Values)
	require.True(t, ok)
	require.Len(t, values.Values, 1)
	wantJSON := map[string]any{"properties": task.Properties, "private_data": task.PrivateData}
	seen := map[string]bool{}
	for i, column := range values.Columns {
		if column.Name != "properties" && column.Name != "private_data" && column.Name != "data" {
			continue
		}
		seen[column.Name] = true
		t.Run(column.Name, func(t *testing.T) {
			binding := &gorm.Statement{DB: db}
			binding.AddVar(binding, values.Values[0][i])
			if column.Name == "data" {
				// GORM emits this nil RawMessage inline rather than binding an
				// empty string. Do not change the fixture to hide a JSON error.
				require.Empty(t, binding.Vars)
				encoded, err := pgtype.NewMap().Encode(0, pgtype.TextFormatCode, task.Data, []byte{})
				require.NoError(t, err)
				require.Nil(t, encoded)
				return
			}
			require.Len(t, binding.Vars, 1)
			require.Contains(t, statement.Vars, binding.Vars[0], "inspect an actual full INSERT binding")
			// This is the exact codec call used by pgx.convertSimpleArgument:
			// simple protocol has no server-provided parameter OID.
			encoded, err := pgtype.NewMap().Encode(0, pgtype.TextFormatCode, binding.Vars[0], []byte{})
			require.NoError(t, err)
			expected, err := common.Marshal(wantJSON[column.Name])
			require.NoError(t, err)
			// Explicit JSON OIDs (extended protocol) are a control case: the
			// value must also stay valid when the server supplies its type.
			typed, err := pgtype.NewMap().Encode(pgtype.JSONOID, pgtype.TextFormatCode, binding.Vars[0], []byte{})
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(typed))
			assert.JSONEq(t, string(expected), string(encoded), "simple-protocol %s parameter must be JSON text, not bytea hex", column.Name)
		})
	}
	assert.Equal(t, map[string]bool{"properties": true, "private_data": true, "data": true}, seen)
}

func TestTaskJSONValuesPreserveNullAndMarshalErrors(t *testing.T) {
	for _, value := range []driver.Valuer{Properties{}, TaskPrivateData{}} {
		encoded, err := value.Value()
		require.NoError(t, err)
		assert.Nil(t, encoded)
		wire, err := pgtype.NewMap().Encode(0, pgtype.TextFormatCode, value, []byte{})
		require.NoError(t, err)
		assert.Nil(t, wire)
	}
	invalid := TaskPrivateData{BillingContext: &TaskBillingContext{GroupRatio: math.NaN()}}
	_, expectedErr := common.Marshal(invalid)
	require.Error(t, expectedErr)
	value, err := invalid.Value()
	require.EqualError(t, err, expectedErr.Error())
	assert.Nil(t, value)
}
