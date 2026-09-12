package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormclickhouse "gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func useLogDedupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
	require.NoError(t, registerLogCreateGuard(db))
	require.NoError(t, db.Exec("CREATE INDEX idx_logs_billing_canonical ON logs (billing_event_id, billing_projection_digest, id)").Error)
	require.NoError(t, db.Exec("CREATE INDEX idx_logs_row_key ON logs (log_row_key)").Error)
	require.NoError(t, db.Exec("CREATE TABLE channels (id INTEGER PRIMARY KEY, name TEXT)").Error)
	require.NoError(t, db.Exec("INSERT INTO channels (id, name) VALUES (?, ?), (?, ?)", 11, "channel-11", 12, "channel-12").Error)

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousMemoryCache := common.MemoryCacheEnabled
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.MemoryCacheEnabled = previousMemoryCache
		initCol()
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
	return db
}

func insertLogWithoutHooks(t *testing.T, db *gorm.DB, logRecord *Log) {
	t.Helper()
	require.NotEmpty(t, logRecord.LogRowKey)
	logRecord.Id = 0
	require.NoError(t, db.Set(logCreateMarker, true).Session(&gorm.Session{SkipHooks: true}).Create(logRecord).Error)
}

func canonicalConflictPair(first, second *Log) (*Log, *Log) {
	first.BillingProjectionDigest = ComputeBillingProjectionDigest(first)
	second.BillingProjectionDigest = ComputeBillingProjectionDigest(second)
	if first.UserId < second.UserId {
		return first, second
	}
	return second, first
}

func TestBillingProjectionIdentityValidation(t *testing.T) {
	db := useLogDedupTestDB(t)
	logRecord := Log{
		UserId:         7,
		CreatedAt:      1_700_000_000,
		Type:           LogTypeConsume,
		Content:        "immutable projection",
		BillingEventID: "billing-identity",
		RequestId:      "request-identity",
	}
	require.NoError(t, CreateLog(db, &logRecord))
	assert.Len(t, logRecord.BillingProjectionDigest, 65)
	assert.True(t, strings.HasPrefix(logRecord.BillingProjectionDigest, billingProjectionDigestVersion))
	assert.NotEmpty(t, logRecord.LogRowKey)

	replay := logRecord
	replay.Id = 0
	replay.LogRowKey = ""
	require.NoError(t, CreateLog(db, &replay))
	assert.Equal(t, logRecord.BillingProjectionDigest, replay.BillingProjectionDigest)
	assert.NotEqual(t, logRecord.LogRowKey, replay.LogRowKey)

	invalidDigest := logRecord
	invalidDigest.Id = 0
	invalidDigest.LogRowKey = ""
	invalidDigest.BillingProjectionDigest = "1" + strings.Repeat("0", 64)
	err := CreateLog(db, &invalidDigest)
	require.ErrorIs(t, err, ErrBillingProjectionDigestMismatch)

	conflict := logRecord
	conflict.Id = 0
	conflict.LogRowKey = ""
	conflict.BillingProjectionDigest = ""
	conflict.Content = "conflicting projection"
	err = CreateLog(db, &conflict)
	require.ErrorIs(t, err, ErrBillingProjectionConflict)

	direct := Log{Content: "unguarded"}
	require.ErrorContains(t, db.Create(&direct).Error, "CreateLog or CreateLogs")
	require.ErrorContains(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&direct).Error, "CreateLog or CreateLogs")
	require.ErrorContains(t, db.Table("logs").Create(map[string]interface{}{"content": "map bypass"}).Error, "CreateLog or CreateLogs")

	batchA := &Log{BillingEventID: "batch-conflict", UserId: 1, CreatedAt: 1, LogRowKey: "batch-a"}
	batchB := &Log{BillingEventID: "batch-conflict", UserId: 2, CreatedAt: 1, LogRowKey: "batch-b"}
	require.ErrorIs(t, CreateLogs(db, []*Log{batchA, batchB}), ErrBillingProjectionConflict)

	historical := Log{Content: "historical row"}
	require.NoError(t, CreateLog(db, &historical))
	assert.Empty(t, historical.BillingProjectionDigest)
	assert.NotEmpty(t, historical.LogRowKey)
}

type logConsumerFixture struct {
	now           int64
	canonical     Log
	noncanonical  Log
	expectedQuota int
	expectedRPM   int
	expectedTPM   int
}

func createLogConsumerFixture(t *testing.T, db *gorm.DB) logConsumerFixture {
	t.Helper()
	now := time.Now().Unix()
	first := Log{
		UserId:            7,
		CreatedAt:         now - 10,
		Type:              LogTypeConsume,
		Content:           "conflict-alpha",
		Username:          "alice",
		TokenName:         "token-alpha",
		ModelName:         "model-alpha",
		Quota:             100,
		PromptTokens:      10,
		CompletionTokens:  5,
		ChannelId:         11,
		TokenId:           31,
		Group:             "group-alpha",
		RequestId:         "request-conflict",
		UpstreamRequestId: "upstream-alpha",
		BillingEventID:    "billing-conflict",
		LogRowKey:         "conflict-row-a",
		Other:             `{"variant":"alpha"}`,
	}
	second := first
	second.UserId = 8
	second.Content = "conflict-beta"
	second.Username = "bob"
	second.TokenName = "token-beta"
	second.ModelName = "model-beta"
	second.Quota = 900
	second.PromptTokens = 90
	second.CompletionTokens = 9
	second.ChannelId = 12
	second.TokenId = 32
	second.Group = "group-beta"
	second.UpstreamRequestId = "upstream-beta"
	second.LogRowKey = "conflict-row-b"
	second.Other = `{"variant":"beta"}`
	canonical, noncanonical := canonicalConflictPair(&first, &second)
	insertLogWithoutHooks(t, db, &first)
	insertLogWithoutHooks(t, db, &second)

	exact := Log{
		UserId:            9,
		CreatedAt:         now - 11,
		Type:              LogTypeConsume,
		Content:           "exact replay",
		Username:          "exact-user",
		TokenName:         "exact-token",
		ModelName:         "exact-model",
		Quota:             50,
		PromptTokens:      8,
		CompletionTokens:  7,
		ChannelId:         0,
		TokenId:           41,
		Group:             "exact-group",
		RequestId:         "request-exact",
		UpstreamRequestId: "upstream-exact",
		BillingEventID:    "billing-exact",
		Other:             `{"exact":true}`,
	}
	for _, rowKey := range []string{"exact-row-c", "exact-row-a", "exact-row-b"} {
		replay := exact
		replay.LogRowKey = rowKey
		require.NoError(t, CreateLog(db, &replay))
	}

	differentEvent := Log{
		UserId:            10,
		CreatedAt:         now - 12,
		Type:              LogTypeConsume,
		Content:           "different event",
		Username:          "different-user",
		TokenName:         "different-token",
		ModelName:         "different-model",
		Quota:             200,
		PromptTokens:      20,
		CompletionTokens:  5,
		TokenId:           42,
		Group:             "different-group",
		RequestId:         "request-different",
		UpstreamRequestId: "upstream-different",
		BillingEventID:    "billing-different",
		LogRowKey:         "different-row",
	}
	require.NoError(t, CreateLog(db, &differentEvent))

	for index, rowKey := range []string{"empty-row-e", "empty-row-d", "empty-row-c", "empty-row-b", "empty-row-a"} {
		empty := Log{
			UserId:            11,
			CreatedAt:         now - 5,
			Type:              LogTypeConsume,
			Content:           "physical empty event",
			Username:          "empty-user",
			TokenName:         "empty-token",
			ModelName:         "empty-model",
			Quota:             30,
			PromptTokens:      3,
			CompletionTokens:  2,
			TokenId:           43,
			Group:             "empty-group",
			RequestId:         "request-empty",
			UpstreamRequestId: "upstream-empty",
			LogRowKey:         rowKey,
			Other:             fmt.Sprintf(`{"index":%d}`, index),
		}
		require.NoError(t, CreateLog(db, &empty))
	}

	return logConsumerFixture{
		now:           now,
		canonical:     *canonical,
		noncanonical:  *noncanonical,
		expectedQuota: canonical.Quota + exact.Quota + differentEvent.Quota + 5*30,
		expectedRPM:   8,
		expectedTPM:   canonical.PromptTokens + canonical.CompletionTokens + 15 + 25 + 5*5,
	}
}

func TestLogConsumersUseUnifiedCanonicalRuleBeforeFiltersAndPagination(t *testing.T) {
	db := useLogDedupTestDB(t)
	fixture := createLogConsumerFixture(t, db)

	seenRows := map[string]struct{}{}
	seenEvents := map[string]struct{}{}
	for page := 0; page < 4; page++ {
		logs, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", page*2, 2, 0, "", "", "")
		require.NoError(t, err)
		assert.EqualValues(t, 8, total)
		require.Len(t, logs, 2)
		for _, logRecord := range logs {
			if _, exists := seenRows[logRecord.LogRowKey]; exists {
				t.Fatalf("row key %q repeated across pages", logRecord.LogRowKey)
			}
			seenRows[logRecord.LogRowKey] = struct{}{}
			if logRecord.BillingEventID != "" {
				if _, exists := seenEvents[logRecord.BillingEventID]; exists {
					t.Fatalf("billing event %q repeated across pages", logRecord.BillingEventID)
				}
				seenEvents[logRecord.BillingEventID] = struct{}{}
			}
		}
	}
	assert.Len(t, seenRows, 8)
	assert.Len(t, seenEvents, 3)

	canonicalLogs, canonicalTotal, err := GetAllLogs(
		LogTypeConsume,
		fixture.canonical.CreatedAt,
		fixture.canonical.CreatedAt,
		fixture.canonical.ModelName,
		fixture.canonical.Username,
		fixture.canonical.TokenName,
		0,
		10,
		fixture.canonical.ChannelId,
		fixture.canonical.Group,
		fixture.canonical.RequestId,
		fixture.canonical.UpstreamRequestId,
	)
	require.NoError(t, err)
	assert.EqualValues(t, 1, canonicalTotal)
	require.Len(t, canonicalLogs, 1)
	assert.Equal(t, fixture.canonical.BillingProjectionDigest, canonicalLogs[0].BillingProjectionDigest)

	noncanonicalLogs, noncanonicalTotal, err := GetAllLogs(
		LogTypeConsume,
		0,
		0,
		fixture.noncanonical.ModelName,
		fixture.noncanonical.Username,
		fixture.noncanonical.TokenName,
		0,
		10,
		fixture.noncanonical.ChannelId,
		fixture.noncanonical.Group,
		fixture.noncanonical.RequestId,
		fixture.noncanonical.UpstreamRequestId,
	)
	require.NoError(t, err)
	assert.Zero(t, noncanonicalTotal)
	assert.Empty(t, noncanonicalLogs)

	userLogs, userTotal, err := GetUserLogs(
		fixture.canonical.UserId,
		LogTypeConsume,
		fixture.canonical.CreatedAt,
		fixture.canonical.CreatedAt,
		fixture.canonical.ModelName,
		fixture.canonical.TokenName,
		0,
		10,
		fixture.canonical.Group,
		fixture.canonical.RequestId,
		fixture.canonical.UpstreamRequestId,
	)
	require.NoError(t, err)
	assert.EqualValues(t, 1, userTotal)
	require.Len(t, userLogs, 1)
	assert.Equal(t, fixture.canonical.BillingProjectionDigest, userLogs[0].BillingProjectionDigest)

	tokenLogs, err := GetLogByTokenId(fixture.canonical.TokenId)
	require.NoError(t, err)
	require.Len(t, tokenLogs, 1)
	assert.Equal(t, fixture.canonical.BillingProjectionDigest, tokenLogs[0].BillingProjectionDigest)
}

func TestLogAggregatesUseCanonicalProjectionWithoutChangingGrossSemantics(t *testing.T) {
	db := useLogDedupTestDB(t)
	fixture := createLogConsumerFixture(t, db)

	stat, err := SumUsedQuota(LogTypeRefund, fixture.now-60, fixture.now, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, fixture.expectedQuota, stat.Quota)
	assert.Equal(t, fixture.expectedRPM, stat.Rpm)
	assert.Equal(t, fixture.expectedTPM, stat.Tpm)

	filtered, err := SumUsedQuota(
		LogTypeConsume,
		fixture.canonical.CreatedAt,
		fixture.canonical.CreatedAt,
		fixture.canonical.ModelName,
		fixture.canonical.Username,
		fixture.canonical.TokenName,
		fixture.canonical.ChannelId,
		fixture.canonical.Group,
	)
	require.NoError(t, err)
	assert.Equal(t, fixture.canonical.Quota, filtered.Quota)
	assert.Equal(t, 1, filtered.Rpm)
	assert.Equal(t, fixture.canonical.PromptTokens+fixture.canonical.CompletionTokens, filtered.Tpm)

	tokens := SumUsedToken(LogTypeRefund, fixture.now-60, fixture.now, "", "", "")
	assert.Equal(t, fixture.expectedTPM, tokens)
}

func TestHistoricalDigestFallbackUsesFullProjectionTuple(t *testing.T) {
	db := useLogDedupTestDB(t)
	base := Log{
		UserId:         17,
		CreatedAt:      1_700_000_000,
		Type:           LogTypeConsume,
		Username:       "historical-user",
		BillingEventID: "historical-no-digest",
		RequestId:      "historical-request",
	}
	later := base
	later.Content = "b-content"
	later.LogRowKey = "historical-b"
	earlier := base
	earlier.Content = "a-content"
	earlier.LogRowKey = "historical-a"
	insertLogWithoutHooks(t, db, &later)
	insertLogWithoutHooks(t, db, &earlier)

	digestPreferred := base
	digestPreferred.BillingEventID = "historical-mixed-digest"
	digestPreferred.Content = "z-digest-row"
	digestPreferred.LogRowKey = "digest-row"
	digestPreferred.BillingProjectionDigest = ComputeBillingProjectionDigest(&digestPreferred)
	missingDigest := digestPreferred
	missingDigest.Content = "a-missing-digest"
	missingDigest.LogRowKey = "missing-digest-row"
	missingDigest.BillingProjectionDigest = ""
	insertLogWithoutHooks(t, db, &missingDigest)
	insertLogWithoutHooks(t, db, &digestPreferred)

	logs, total, err := GetAllLogs(LogTypeConsume, 0, 0, "", "historical-user", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, logs, 2)
	byEvent := map[string]*Log{}
	for _, logRecord := range logs {
		byEvent[logRecord.BillingEventID] = logRecord
	}
	assert.Equal(t, "a-content", byEvent["historical-no-digest"].Content)
	assert.Equal(t, "a-missing-digest", byEvent["historical-mixed-digest"].Content)
}

func cleanupPair(t *testing.T, eventID string, target int64, oldCanonical bool) (Log, Log) {
	t.Helper()
	oldUserID, newUserID := 21, 22
	if !oldCanonical {
		oldUserID, newUserID = 22, 21
	}
	oldRow := Log{
		UserId:         oldUserID,
		CreatedAt:      target - 100,
		Type:           LogTypeConsume,
		Content:        "old",
		Username:       "cleanup-user",
		ModelName:      "cleanup-model",
		Quota:          100,
		PromptTokens:   10,
		BillingEventID: eventID,
		RequestId:      "cleanup-request",
		LogRowKey:      eventID + "-old",
	}
	newRow := oldRow
	newRow.UserId = newUserID
	newRow.CreatedAt = target + 100
	newRow.Content = "new"
	newRow.Quota = 900
	newRow.PromptTokens = 90
	newRow.LogRowKey = eventID + "-new"
	oldRow.BillingProjectionDigest = ComputeBillingProjectionDigest(&oldRow)
	newRow.BillingProjectionDigest = ComputeBillingProjectionDigest(&newRow)
	return oldRow, newRow
}

func TestLogCleanupDeletesLogicalBillingEventsByCanonicalAge(t *testing.T) {
	db := useLogDedupTestDB(t)
	target := time.Now().Unix() - 1_000
	expiredOld, expiredNew := cleanupPair(t, "cleanup-expired", target, true)
	retainedOld, retainedNew := cleanupPair(t, "cleanup-retained", target, false)
	for _, logRecord := range []*Log{&expiredOld, &expiredNew, &retainedOld, &retainedNew} {
		insertLogWithoutHooks(t, db, logRecord)
	}
	for _, row := range []Log{
		{CreatedAt: target - 50, Type: LogTypeConsume, Content: "empty-old-a", LogRowKey: "cleanup-empty-old-a"},
		{CreatedAt: target - 40, Type: LogTypeConsume, Content: "empty-old-b", LogRowKey: "cleanup-empty-old-b"},
		{CreatedAt: target + 40, Type: LogTypeConsume, Content: "empty-new", LogRowKey: "cleanup-empty-new"},
	} {
		require.NoError(t, CreateLog(db, &row))
	}

	before, _, err := GetAllLogs(LogTypeConsume, 0, 0, "cleanup-model", "cleanup-user", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, before, 2)
	beforeCanonical := map[string]string{}
	for _, logRecord := range before {
		beforeCanonical[logRecord.BillingEventID] = logRecord.Content
	}
	assert.Equal(t, expiredOld.Content, beforeCanonical["cleanup-expired"])
	assert.Equal(t, retainedNew.Content, beforeCanonical["cleanup-retained"])

	oldCount, err := CountOldLog(t.Context(), target)
	require.NoError(t, err)
	assert.EqualValues(t, 4, oldCount)

	deleted, err := DeleteOldLogBatch(t.Context(), target, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted, "one logical event batch must remove every physical replay")
	var expiredPhysical int64
	require.NoError(t, db.Model(&Log{}).Where("billing_event_id = ?", "cleanup-expired").Count(&expiredPhysical).Error)
	assert.Zero(t, expiredPhysical)

	remainingOld, err := CountOldLog(t.Context(), target)
	require.NoError(t, err)
	assert.EqualValues(t, 2, remainingOld)
	deleted, err = DeleteOldLogBatch(t.Context(), target, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, deleted)
	remainingOld, err = CountOldLog(t.Context(), target)
	require.NoError(t, err)
	assert.Zero(t, remainingOld)

	var retainedPhysical int64
	require.NoError(t, db.Model(&Log{}).Where("billing_event_id = ?", "cleanup-retained").Count(&retainedPhysical).Error)
	assert.EqualValues(t, 2, retainedPhysical, "an old replay must remain when the canonical event is not expired")
	after, _, err := GetAllLogs(LogTypeConsume, 0, 0, "cleanup-model", "cleanup-user", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.Equal(t, retainedNew.Content, after[0].Content, "cleanup must not expose a different replay as canonical")
}

func TestLogProjectionBackfillBatchUsesCheckpoint(t *testing.T) {
	db := useLogDedupTestDB(t)
	for id := 1; id <= 3; id++ {
		row := &Log{Id: id, BillingEventID: fmt.Sprintf("legacy-%d", id), Content: "legacy", RequestId: "legacy"}
		require.NoError(t, db.Set(logCreateMarker, true).Session(&gorm.Session{SkipHooks: true}).Create(row).Error)
	}
	state, updated, err := BackfillLogProjectionIdentityBatch(t.Context(), db, LogProjectionBackfillState{}, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, updated)
	assert.Equal(t, 2, state.LastID)
	assert.False(t, state.Complete)
	state, updated, err = BackfillLogProjectionIdentityBatch(t.Context(), db, state, 2)
	require.NoError(t, err)
	assert.Equal(t, 1, updated)
	assert.Equal(t, 3, state.LastID)
	assert.True(t, state.Complete)
	var missing int64
	require.NoError(t, db.Model(&Log{}).Where("log_row_key = '' OR billing_projection_digest = ''").Count(&missing).Error)
	assert.Zero(t, missing)
}

func TestLogProjectionBackfillCheckpointRetriesFailedRow(t *testing.T) {
	db := useLogDedupTestDB(t)
	for id := 1; id <= 2; id++ {
		row := &Log{Id: id, BillingEventID: fmt.Sprintf("retry-%d", id), LogRowKey: ""}
		require.NoError(t, db.Set(logCreateMarker, true).Session(&gorm.Session{SkipHooks: true}).Create(row).Error)
	}
	failure := errors.New("injected second row failure")
	updateCalls := 0
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:log-backfill-second-row", func(tx *gorm.DB) {
		updateCalls++
		if updateCalls == 2 {
			tx.AddError(failure)
		}
	}))
	state, updated, err := BackfillLogProjectionIdentityBatch(t.Context(), db, LogProjectionBackfillState{}, 2)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, updated)
	assert.Equal(t, 1, state.LastID)
	require.NoError(t, db.Callback().Update().Remove("test:log-backfill-second-row"))
	state, updated, err = BackfillLogProjectionIdentityBatch(t.Context(), db, state, 2)
	require.NoError(t, err)
	assert.Equal(t, 1, updated)
	assert.Equal(t, 2, state.LastID)
}

func TestLogCanonicalQueryUsesCandidateIndexWithoutGrouping(t *testing.T) {
	db := useLogDedupTestDB(t)
	var logs []Log
	statement := deduplicatedLogs(db.Session(&gorm.Session{DryRun: true})).
		Where("logs.user_id = ? AND logs.created_at >= ?", 7, 1_700_000_000).
		Find(&logs).
		Statement
	require.NotEmpty(t, statement.SQL.String())
	assert.NotContains(t, strings.ToUpper(statement.SQL.String()), "GROUP BY")

	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+statement.SQL.String(), statement.Vars...).Scan(&plan).Error)
	joined := ""
	for _, row := range plan {
		joined += row.Detail + "\n"
	}
	assert.Contains(t, joined, "idx_logs_billing_canonical")
	assert.NotContains(t, strings.ToUpper(joined), "TEMP B-TREE FOR GROUP BY")
}

func TestDeduplicatedLogsGenerateSupportedDialectSQL(t *testing.T) {
	previousLogDB, previousLogType := LOG_DB, common.LogDatabaseType()
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
		initCol()
	})

	sqliteDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	inertSQLDB, err := sqliteDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, inertSQLDB.Close()) })

	tests := []struct {
		name      string
		kind      common.DatabaseType
		dialector gorm.Dialector
		contains  []string
	}{
		{"sqlite", common.DatabaseTypeSQLite, sqlite.Open(":memory:"), []string{"NOT EXISTS", "log_row_key"}},
		{"mysql-5.7-compatible", common.DatabaseTypeMySQL, mysql.New(mysql.Config{Conn: inertSQLDB, SkipInitializeWithVersion: true}), []string{"NOT EXISTS", "log_row_key"}},
		{"postgres-9.6-compatible", common.DatabaseTypePostgreSQL, postgres.New(postgres.Config{Conn: inertSQLDB}), []string{"NOT EXISTS", "log_row_key"}},
		{
			"clickhouse",
			common.DatabaseTypeClickHouse,
			gormclickhouse.New(gormclickhouse.Config{Conn: inertSQLDB, SkipInitializeWithVersion: true}),
			[]string{"UNION ALL", "argMin(", "GROUP BY billing_event_id", "tupleElement(canonical, 23)", "billing_projection_digest", "log_row_key"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dryDB, openErr := gorm.Open(test.dialector, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
			require.NoError(t, openErr)
			LOG_DB = dryDB
			common.SetLogDatabaseType(test.kind)
			initCol()
			var logs []Log
			statement := deduplicatedLogs(dryDB).
				Where("logs.user_id = ?", 7).
				Order(clickHouseLogOrder("logs.")).
				Limit(10).
				Find(&logs).
				Statement
			generatedSQL := statement.SQL.String()
			require.NotEmpty(t, generatedSQL)
			for _, fragment := range test.contains {
				assert.Contains(t, generatedSQL, fragment)
			}
			upperSQL := strings.ToUpper(generatedSQL)
			assert.NotContains(t, upperSQL, "MIN(ID)")
			assert.NotContains(t, upperSQL, "DISTINCT ON")
			assert.NotContains(t, upperSQL, " FINAL")
			assert.NotContains(t, upperSQL, " OVER (")
			assert.NotContains(t, upperSQL, "ROW_NUMBER")
			if test.kind != common.DatabaseTypeClickHouse {
				assert.NotContains(t, upperSQL, "GROUP BY")
			} else {
				predicate, args := logCleanupPredicate([]string{"event-a", "event-b"}, []string{"row-a"})
				deleteResult := dryDB.Exec(
					"ALTER TABLE logs DELETE WHERE "+predicate+" SETTINGS mutations_sync = 1",
					args...,
				)
				deleteStatement := deleteResult.Statement.SQL.String()
				assert.NotContains(t, deleteStatement, "IN ?")
				assert.Contains(t, deleteStatement, "IN (?,?)")
				assert.Equal(t, []interface{}{"event-a", "event-b", "row-a"}, deleteResult.Statement.Vars)
			}
		})
	}

	projection := clickHouseCanonicalProjectionDefinition()
	assert.Contains(t, projection, "argMin(")
	assert.Contains(t, projection, "billing_projection_digest")
	assert.Contains(t, projection, "log_row_key")
	assert.Contains(t, projection, "GROUP BY billing_event_id")
}

func runLogDedupDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	stamp := time.Now().Unix()
	first := Log{
		UserId:           301,
		CreatedAt:        stamp,
		Type:             LogTypeConsume,
		Content:          "configured-alpha",
		Username:         "configured-alpha",
		ModelName:        "configured-alpha",
		Quota:            123,
		PromptTokens:     12,
		CompletionTokens: 3,
		BillingEventID:   "configured-billing-event",
		RequestId:        "configured-request",
		LogRowKey:        "configured-row-alpha",
	}
	second := first
	second.UserId = 302
	second.Content = "configured-beta"
	second.Username = "configured-beta"
	second.ModelName = "configured-beta"
	second.Quota = 987
	second.LogRowKey = "configured-row-beta"
	canonical, noncanonical := canonicalConflictPair(&first, &second)
	insertLogWithoutHooks(t, db, &first)
	insertLogWithoutHooks(t, db, &second)

	logs, total, err := GetAllLogs(LogTypeConsume, stamp, stamp, canonical.ModelName, canonical.Username, "", 0, 10, 0, "", "configured-request", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, canonical.BillingProjectionDigest, logs[0].BillingProjectionDigest)

	logs, total, err = GetAllLogs(LogTypeConsume, stamp, stamp, noncanonical.ModelName, noncanonical.Username, "", 0, 10, 0, "", "configured-request", "")
	require.NoError(t, err)
	assert.Zero(t, total)
	assert.Empty(t, logs)

	stat, err := SumUsedQuota(LogTypeConsume, stamp, stamp, canonical.ModelName, canonical.Username, "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, canonical.Quota, stat.Quota)

	if db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		require.NoError(t, db.Exec("ALTER TABLE logs DELETE WHERE billing_event_id = ? SETTINGS mutations_sync = 1", first.BillingEventID).Error)
	} else {
		require.NoError(t, db.Where("billing_event_id = ?", first.BillingEventID).Delete(&Log{}).Error)
	}
}

func TestLogCleanupUsesLegacyRelationalIDWithoutStarvation(t *testing.T) {
	db := useLogDedupTestDB(t)
	target := time.Now().Unix()
	for id := 1; id <= 3; id++ {
		require.NoError(t, db.Exec(
			"INSERT INTO logs (id, created_at, billing_event_id, billing_projection_digest, log_row_key, content, request_id) VALUES (?, ?, '', '', '', ?, ?)",
			id, target-10, fmt.Sprintf("legacy-%d", id), fmt.Sprintf("legacy-request-%d", id),
		).Error)
	}

	for expectedRemaining := int64(2); expectedRemaining >= 0; expectedRemaining-- {
		result, err := DeleteOldLogBatchDetailed(t.Context(), target, 1)
		require.NoError(t, err)
		assert.Equal(t, int64(1), result.Deleted)
		assert.Zero(t, result.Skipped)
		remaining, countErr := CountOldLog(t.Context(), target)
		require.NoError(t, countErr)
		assert.Equal(t, expectedRemaining, remaining)
	}
}

func runLogProjectionBackfillDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	state := LogProjectionBackfillState{}
	for pass := 0; pass < 100 && !state.Complete; pass++ {
		next, _, err := BackfillLogProjectionIdentityBatch(t.Context(), db, state, 2)
		require.NoError(t, err)
		state = next
	}
	require.True(t, state.Complete, "relational identity backfill must reach the end checkpoint")

	indexesComplete := false
	for pass := 0; pass < len(logProjectionIndexSpecs)+1 && !indexesComplete; pass++ {
		done, manualReview, err := CreateNextLogProjectionIndex(t.Context(), db)
		require.NoError(t, err)
		require.False(t, manualReview)
		indexesComplete = done
	}
	require.True(t, indexesComplete)
	for _, spec := range logProjectionIndexSpecs {
		assert.True(t, db.Migrator().HasIndex(&Log{}, spec.Name), "missing background index %s", spec.Name)
	}
}

func TestRelationalBackfillQuarantinesConflictingEventAndContinues(t *testing.T) {
	db := useLogDedupTestDB(t)
	target := time.Now().Unix()
	conflictA := Log{Id: 1, UserId: 1, CreatedAt: target - 10, Type: LogTypeConsume, Content: "alpha", Quota: 10, BillingEventID: "quarantined-event", RequestId: "q", LogRowKey: "q-a"}
	conflictB := conflictA
	conflictB.Id = 2
	conflictB.UserId = 2
	conflictB.Content = "beta"
	conflictB.Quota = 20
	conflictB.LogRowKey = "q-b"
	safe := Log{Id: 3, UserId: 3, CreatedAt: target - 10, Type: LogTypeConsume, Content: "safe", Quota: 30, BillingEventID: "safe-event", RequestId: "safe", LogRowKey: "safe-row"}
	for _, row := range []*Log{&conflictA, &conflictB, &safe} {
		row.BillingProjectionDigest = ComputeBillingProjectionDigest(row)
		require.NoError(t, db.Set(logCreateMarker, true).Session(&gorm.Session{SkipHooks: true}).Create(row).Error)
	}

	state, processed, err := BackfillLogProjectionIdentityBatch(t.Context(), db, LogProjectionBackfillState{}, 10)
	require.NoError(t, err)
	assert.Zero(t, processed)
	assert.Equal(t, 3, state.LastID)
	assert.True(t, state.Complete)
	assert.Equal(t, int64(1), state.QuarantinedEvents)

	var identity BillingLogProjectionIdentity
	require.NoError(t, db.Where("billing_event_id = ?", "quarantined-event").First(&identity).Error)
	assert.Equal(t, BillingLogProjectionIdentityStatusQuarantined, identity.Status)
	assert.Contains(t, identity.Reason, "immutable projections")

	logs, total, err := GetAllLogs(LogTypeConsume, target-20, target, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, "safe-event", logs[0].BillingEventID)
	stat, err := SumUsedQuota(LogTypeConsume, target-20, target, "", "", "", 0, "")
	require.NoError(t, err)
	assert.Equal(t, 30, stat.Quota)

	remaining, err := CountOldLog(t.Context(), target)
	require.NoError(t, err)
	assert.Equal(t, int64(1), remaining)
	deleted, err := DeleteOldLogBatchDetailed(t.Context(), target, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted.Deleted)
	var quarantinedRows int64
	require.NoError(t, db.Model(&Log{}).Where("billing_event_id = ?", "quarantined-event").Count(&quarantinedRows).Error)
	assert.Equal(t, int64(2), quarantinedRows)
}

func TestLogCleanupSkipsUnsafePrefixWithoutStarvingSafeRows(t *testing.T) {
	db := useLogDedupTestDB(t)
	target := time.Now().Unix()
	require.NoError(t, db.Exec(
		"INSERT INTO logs (id, created_at, billing_event_id, billing_projection_digest, log_row_key, content, request_id) VALUES (?, ?, '', '', '', 'unsafe', 'a-unsafe')",
		-1, target-20,
	).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO logs (id, created_at, billing_event_id, billing_projection_digest, log_row_key, content, request_id) VALUES (?, ?, '', '', '', 'safe', 'z-safe')",
		1, target-10,
	).Error)

	unsafe, err := CountUnsafeOldLogs(t.Context(), target)
	require.NoError(t, err)
	assert.Equal(t, int64(1), unsafe)
	remaining, err := CountOldLog(t.Context(), target)
	require.NoError(t, err)
	assert.Equal(t, int64(1), remaining)
	result, err := DeleteOldLogBatchDetailed(t.Context(), target, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Deleted)
	assert.Zero(t, result.Skipped)
	remaining, err = CountOldLog(t.Context(), target)
	require.NoError(t, err)
	assert.Zero(t, remaining)
	unsafe, err = CountUnsafeOldLogs(t.Context(), target)
	require.NoError(t, err)
	assert.Equal(t, int64(1), unsafe)
}
