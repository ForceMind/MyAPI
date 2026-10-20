package model

import (
	"context"
	"math"
	"os"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func logTokenStatisticsContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	input, output, cached := int64(1000), int64(100), int64(800)
	encoded, err := common.Marshal(map[string]interface{}{
		"usage_accuracy": "reported", "token_usage_evidence": LogTokenUsageEvidence{1, &input, &output, &cached},
	})
	require.NoError(t, err)
	first := Log{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 100, ModelName: "model_1", TokenName: "owner'key", Group: "vip", ChannelId: 7, RequestId: "req-1", UpstreamRequestId: "up-1", BillingEventID: "statistics-event", Other: string(encoded)}
	require.NoError(t, CreateLog(db, &first))
	replay := first
	replay.Id, replay.LogRowKey = 0, ""
	require.NoError(t, CreateLog(db, &replay))
	for _, row := range []Log{
		{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 101, ModelName: "model_1", TokenName: "owner'key", Group: "vip", ChannelId: 7, Other: `{"usage_accuracy":"reported","token_usage_evidence":{"schema":1,"input_tokens":100,"output_tokens":20,"cache_read_tokens":0}}`},
		{UserId: 1, Username: "alice", Type: LogTypeError, CreatedAt: 102, ModelName: "model_1", TokenName: "owner'key", Group: "vip", ChannelId: 7, Other: `{"request_outcome":"completed","usage_outcome":"missing"}`},
		{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 103, ModelName: "model_1", TokenName: "owner'key", Group: "vip", ChannelId: 7, PromptTokens: 50, CompletionTokens: 5, Other: `{"usage_accuracy":"reported","cache_tokens":0}`},
		{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 104, ModelName: "model_1", TokenName: "owner'key", Group: "vip", ChannelId: 7, PromptTokens: 999},
		{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 105, ModelName: "model_1", TokenName: "owner'key", Group: "vip", ChannelId: 7, Other: `{"usage_accuracy":"reported","token_usage_evidence":{"schema":1,"input_tokens":0,"output_tokens":0,"cache_read_tokens":0}}`},
		{UserId: 2, Username: "bob", Type: LogTypeConsume, CreatedAt: 103, ModelName: "other-model", PromptTokens: 9000, Other: `{"usage_accuracy":"reported"}`},
		{UserId: 1, Username: "alice", Type: LogTypeTopup, CreatedAt: 103, Quota: 99},
		{UserId: 1, Username: "alice", Type: LogTypeConsume, CreatedAt: 200, PromptTokens: 8000, Other: `{"usage_accuracy":"reported"}`},
	} {
		require.NoError(t, CreateLog(db, &row))
	}
	filter := LogTokenStatisticsFilter{UserID: 1, Start: 100, End: 105}
	stat, err := GetLogTokenStatistics(t.Context(), db, filter)
	require.NoError(t, err)
	assert.EqualValues(t, 6, stat.Records)
	assert.EqualValues(t, 4, stat.KnownRecords)
	assert.EqualValues(t, 2, stat.UnknownRecords)
	assert.Equal(t, "1150", stat.KnownInputTokens)
	assert.Equal(t, "125", stat.KnownOutputTokens)
	assert.Equal(t, "1275", stat.KnownTotalTokens)
	assert.Nil(t, stat.TotalTokens)
	assert.EqualValues(t, 3, stat.CacheKnownRecords)
	assert.EqualValues(t, 3, stat.CacheUnknownRecords)
	assert.Equal(t, "800", stat.CacheReadTokens)
	assert.Equal(t, "1100", stat.CacheInputTokens)
	require.NotNil(t, stat.CacheHitRate)
	assert.InDelta(t, 8.0/11, *stat.CacheHitRate, 1e-12)

	filter.Model, filter.Token, filter.Group = "model_1", "owner'key", "vip"
	filter.RequestID, filter.UpstreamRequestID = "req-1", "up-1"
	stat, err = GetLogTokenStatistics(t.Context(), db, filter)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stat.Records)
	require.NotNil(t, stat.TotalTokens)
	assert.Equal(t, "1100", *stat.TotalTokens)

	// Owner scope uses the ID, and cannot be widened with admin-only filters.
	stat, err = GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{UserID: 2, Username: "alice", Channel: 999, Start: 100, End: 105})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stat.Records)
	assert.Equal(t, "9000", stat.KnownTotalTokens)
	stat, err = GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{Username: "alice", Channel: 7, Model: "model%", Group: "vip", Start: 100, End: 105})
	require.NoError(t, err)
	assert.EqualValues(t, 6, stat.Records)

	stat, err = GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{Type: LogTypeError, UserID: 1})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stat.UnknownRecords)
	assert.Nil(t, stat.TotalTokens)
	stat, err = GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{Type: LogTypeTopup})
	require.NoError(t, err)
	assert.Zero(t, stat.Records)
	require.NotNil(t, stat.TotalTokens)
	assert.Equal(t, "0", *stat.TotalTokens)
	assert.Nil(t, stat.CacheHitRate)

	var stored Log
	require.NoError(t, db.First(&stored, first.Id).Error)
	assert.Equal(t, first.Other, stored.Other)
	assert.Equal(t, first.BillingProjectionDigest, stored.BillingProjectionDigest)
	assert.Equal(t, first.Quota, stored.Quota)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = GetLogTokenStatistics(cancelled, db, filter)
	require.Error(t, err)
}

func TestLogTokenStatisticsSQLite(t *testing.T) {
	logTokenStatisticsContract(t, useLogDedupTestDB(t))
}

func TestLogTokenStatisticsEvidenceBoundaries(t *testing.T) {
	for _, item := range []struct {
		name, metadata, input, total, cached string
		known, cacheKnown                    bool
	}{
		{"missing-usage-overrides-counters", `{"usage_accuracy":"unknown","usage_outcome":"missing"}`, "0", "0", "0", false, false},
		{"estimated-is-not-reported", `{"usage_accuracy":"estimated"}`, "0", "0", "0", false, false},
		{"unmarked-history-is-unknown", `{}`, "0", "0", "0", false, false},
		{"malformed-metadata", `{`, "0", "0", "0", false, false},
		{"claude-cache-counted-once", `{"usage_accuracy":"reported","usage_semantic":"anthropic","cache_tokens":100,"cache_creation_tokens":20,"cache_creation_tokens_5m":5,"cache_creation_tokens_1h":15}`, "130", "135", "100", true, true},
		{"normalized-input-not-added-twice", `{"usage_accuracy":"reported","usage_semantic":"anthropic","input_tokens_total":130,"cache_tokens":100,"cache_write_tokens":20}`, "130", "135", "100", true, true},
		{"impossible-cache-does-not-create-rate", `{"usage_accuracy":"reported","cache_tokens":999}`, "10", "15", "0", true, false},
		{"nullable-cache-zero-is-unknown", `{"usage_accuracy":"reported","token_usage_evidence":{"schema":1,"input_tokens":10,"output_tokens":5,"cache_read_tokens":null}}`, "10", "15", "0", true, false},
		{"explicit-zero-cache-is-known", `{"usage_accuracy":"reported","token_usage_evidence":{"schema":1,"input_tokens":10,"output_tokens":5,"cache_read_tokens":0}}`, "10", "15", "0", true, true},
		{"incomplete-counter-is-unknown", `{"token_usage_evidence":{"schema":1,"input_tokens":10,"output_tokens":null,"cache_read_tokens":0}}`, "0", "0", "0", false, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			db := useLogDedupTestDB(t)
			require.NoError(t, CreateLog(db, &Log{Type: LogTypeConsume, PromptTokens: 10, CompletionTokens: 5, Other: item.metadata}))
			stat, err := GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{})
			require.NoError(t, err)
			assert.Equal(t, item.input, stat.KnownInputTokens)
			assert.Equal(t, item.total, stat.KnownTotalTokens)
			assert.Equal(t, item.cached, stat.CacheReadTokens)
			assert.Equal(t, item.known, stat.KnownRecords == 1)
			assert.Equal(t, item.cacheKnown, stat.CacheKnownRecords == 1)
		})
	}
}

func TestLogTokenStatisticsExactWideTotals(t *testing.T) {
	db := useLogDedupTestDB(t)
	v := int64(math.MaxInt64)
	metadata, err := common.Marshal(map[string]interface{}{"usage_accuracy": "reported", "token_usage_evidence": LogTokenUsageEvidence{1, &v, &v, &v}})
	require.NoError(t, err)
	require.NoError(t, CreateLog(db, &Log{Type: LogTypeConsume, Other: string(metadata)}))
	stat, err := GetLogTokenStatistics(t.Context(), db, LogTokenStatisticsFilter{})
	require.NoError(t, err)
	require.NotNil(t, stat.TotalTokens)
	assert.Equal(t, "18446744073709551614", *stat.TotalTokens)
	require.NotNil(t, stat.CacheHitRate)
	assert.Equal(t, 1.0, *stat.CacheHitRate)
}

func TestLogTokenStatisticsConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_S1_DATABASE_TESTS") != "1" {
		t.Skip("requires explicitly enabled disposable S1 databases")
	}
	for _, engine := range []struct{ name, env string }{{"mysql", "MYAPI_S1_MYSQL_DSN"}, {"postgres", "MYAPI_S1_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			require.NotEmpty(t, dsn)
			dialector, err := s1DatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			for _, table := range []string{"logs", clickHouseIdentityTable} {
				require.NotContains(t, tables, table, "refusing an existing log schema")
			}
			require.NoError(t, db.AutoMigrate(&Log{}, &BillingLogProjectionIdentity{}))
			logTokenStatisticsContract(t, db)
		})
	}
}
