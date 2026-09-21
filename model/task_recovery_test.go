package model

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var errInjectedTaskRecoveryRollback = errors.New("injected task recovery transaction rollback")
var errInjectedTaskRecoveryClock = errors.New("injected task recovery database clock failure")
var errInjectedTaskRecoveryDispatchAttemptUpdate = errors.New("injected task recovery dispatch attempt update failure")
var errInjectedTaskRecoveryDispatchAttemptPanic = errors.New("injected task recovery dispatch attempt panic")
var errInjectedTaskSubmissionIntentAttemptCreate = errors.New("injected task submission intent attempt create failure")
var errInjectedTaskSubmissionIntentAttemptPanic = errors.New("injected task submission intent attempt panic")
var errInjectedTaskSubmissionIntentSavepoint = errors.New("injected task submission intent savepoint failure")
var errInjectedTaskSubmissionIntentRollback = errors.New("injected task submission intent rollback failure")

const b2SubmissionFixtureUserID = 1_000_000

func TestValidTaskSubmissionPublicID(t *testing.T) {
	assert.True(t, ValidTaskSubmissionPublicID("task_"+strings.Repeat("a", taskSubmissionPublicIDRandomLength)))
	for _, invalid := range []string{
		"",
		"task_" + strings.Repeat("a", taskSubmissionPublicIDRandomLength-1),
		"task_" + strings.Repeat("A", taskSubmissionPublicIDRandomLength),
		"task_" + strings.Repeat("a", taskSubmissionPublicIDRandomLength-1) + "-",
		"billing_evt_" + strings.Repeat("a", taskSubmissionPublicIDRandomLength),
	} {
		assert.False(t, ValidTaskSubmissionPublicID(invalid))
	}
}

type b2TaskRecoveryFixtureClock struct {
	now  int64
	fail bool
}

func installB2TaskRecoveryFixtureClock(t *testing.T, db *gorm.DB) *b2TaskRecoveryFixtureClock {
	t.Helper()
	clock := &b2TaskRecoveryFixtureClock{now: 1_700_000_000}
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:b2-task-recovery-clock", func(tx *gorm.DB) {
		switch tx.Statement.SQL.String() {
		case "SELECT FLOOR(EXTRACT(EPOCH FROM clock_timestamp()))::bigint", "SELECT UNIX_TIMESTAMP()", "SELECT strftime('%s','now')":
			if clock.fail {
				tx.AddError(errInjectedTaskRecoveryClock)
				return
			}
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT " + strconv.FormatInt(clock.now, 10))
			tx.Statement.Vars = nil
		}
	}))
	return clock
}

func openB2SubmissionSQLite(t *testing.T) *gorm.DB {
	return openB2SubmissionSQLiteWithConfig(t, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
}

func openB2SubmissionSQLitePrepared(t *testing.T) *gorm.DB {
	return openB2SubmissionSQLiteWithConfig(t, &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	})
}

func openB2SubmissionSQLiteWithConfig(t *testing.T, config *gorm.Config) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), config)
	require.NoError(t, err)
	require.NoError(t, registerTaskRecoveryGormGuards(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return db
}

func b2SubmissionModels() []interface{} {
	return []interface{}{
		&User{},
		&Token{},
		&Channel{},
		&UserSubscription{},
		&Task{},
		&TaskRecoveryIdentity{},
		&TaskSubmissionOperation{},
		&TaskSubmissionAttempt{},
		&TaskTerminalObservation{},
		&TaskBillingEvent{},
		&TaskBillingLogOutbox{},
		&QuotaMutationReceipt{},
		&UserQuotaMutationReceipt{},
		&QuotaWriterEpoch{},
		&QuotaProjectionObligation{},
		&Log{},
		&BillingLogProjectionIdentity{},
	}
}

func ensureB2SubmissionOwner(t *testing.T, db *gorm.DB, tokenID int) {
	t.Helper()
	var user User
	err := db.Where("id = ?", b2SubmissionFixtureUserID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = User{Id: b2SubmissionFixtureUserID, Username: "b2-fixture-user", Password: "fixture-password", DisplayName: "B2 Fixture"}
		require.NoError(t, db.Create(&user).Error)
	} else {
		require.NoError(t, err)
	}
	var token Token
	err = db.Where("id = ?", tokenID).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		token = Token{Id: tokenID, UserId: user.Id, Key: "b2-fixture-token-" + strconv.Itoa(tokenID), Name: "B2 fixture token"}
		require.NoError(t, db.Create(&token).Error)
	} else {
		require.NoError(t, err)
		assert.Equal(t, user.Id, token.UserId)
	}
}

func migrateB2SubmissionFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(b2SubmissionModels()...))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	// Startup migrations are intentionally repeatable on existing installs.
	require.NoError(t, db.AutoMigrate(b2SubmissionModels()...))
	require.NoError(t, EnsureLogProjectionSchemaWithDB(db))
	require.NoError(t, EnsureQuotaWriterEpochStateWithDB(db))
	setQuotaWriterStateForTest(t, db, QuotaWriterModeAuthoritative, 1)

	for _, model := range []interface{}{
		&TaskRecoveryIdentity{},
		&TaskSubmissionOperation{},
		&TaskSubmissionAttempt{},
		&TaskBillingEvent{},
		&TaskBillingLogOutbox{},
	} {
		require.True(t, db.Migrator().HasTable(model))
	}
	for _, index := range []struct {
		model interface{}
		name  string
	}{
		{&TaskSubmissionOperation{}, "uidx_task_submission_public_id"},
		{&TaskSubmissionOperation{}, "uidx_task_submission_idempotency"},
		{&TaskSubmissionOperation{}, "uidx_task_submission_task"},
		{&TaskSubmissionAttempt{}, "uidx_task_submission_attempt"},
		{&TaskBillingEvent{}, "uidx_task_billing_event_id"},
		{&TaskBillingEvent{}, "uidx_task_billing_event_key"},
		{&TaskBillingLogOutbox{}, "uidx_task_billing_outbox_event"},
	} {
		require.True(t, db.Migrator().HasIndex(index.model, index.name), "missing index %s", index.name)
	}
}

func newB2SubmissionOperation(t *testing.T, tokenID int, method, kind, rawKey, canonicalRequest string) *TaskSubmissionOperation {
	t.Helper()
	keyHash, err := HashTaskSubmissionIdempotencyKey(rawKey)
	require.NoError(t, err)
	return &TaskSubmissionOperation{
		UserID:             b2SubmissionFixtureUserID,
		TokenID:            tokenID,
		HTTPMethod:         method,
		OperationKind:      kind,
		IdempotencyKeyHash: keyHash,
		RequestFingerprint: FingerprintTaskSubmissionRequest([]byte(canonicalRequest)),
	}
}

func createB2SubmissionAttempt(t *testing.T, db *gorm.DB, operation *TaskSubmissionOperation, channelID int) *TaskSubmissionAttempt {
	t.Helper()
	attempt := &TaskSubmissionAttempt{
		OperationID:  operation.ID,
		AttemptNo:    1,
		ChannelID:    channelID,
		Provider:     "fixture",
		RequestClass: "video",
	}
	require.NoError(t, db.Create(attempt).Error)
	return attempt
}

func reserveB2SubmissionOperation(t *testing.T, db *gorm.DB, operationID, expectedVersion int64) {
	t.Helper()
	won, err := TransitionTaskSubmissionOperation(db, operationID, TaskSubmissionOperationTransition{
		From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved, ExpectedVersion: expectedVersion,
	})
	require.NoError(t, err)
	require.True(t, won)
}

func startB2SubmissionDispatch(t *testing.T, db *gorm.DB, operationID, operationVersion, attemptVersion int64) {
	t.Helper()
	won, err := StartTaskSubmissionDispatch(db, operationID, TaskSubmissionDispatchTransition{
		ExpectedOperationVersion: operationVersion, ExpectedAttemptVersion: attemptVersion,
	})
	require.NoError(t, err)
	require.True(t, won)
}

func b2BillingLogPayload(event *TaskBillingEvent) TaskBillingLogPayload {
	payload := TaskBillingLogPayload{
		Version:        TaskRecoveryPayloadVersion,
		BillingEventID: event.EventID,
		UserID:         event.UserID,
		CreatedAt:      event.CreatedAt,
		Content:        "task billing reserve",
		ModelName:      "fixture-model",
		ChannelID:      event.ChannelID,
		TokenID:        event.TokenID,
		Group:          "default",
		RequestID:      event.RequestID,
	}
	switch {
	case event.QuotaDelta < 0:
		payload.Type = LogTypeConsume
		payload.Quota = int(-event.QuotaDelta)
	case event.QuotaDelta > 0:
		payload.Type = LogTypeRefund
		payload.Quota = int(event.QuotaDelta)
	default:
		payload.Type = LogTypeSystem
	}
	return payload
}

func runB2SubmissionDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousLogDB, previousLogType := LOG_DB, common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseType(db.Dialector.Name()))
	initCol()
	t.Cleanup(func() {
		LOG_DB = previousLogDB
		common.SetLogDatabaseType(previousLogType)
		initCol()
	})
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	realDatabaseNow, err := taskRecoveryDBTimestamp(db)
	require.NoError(t, err)
	require.Positive(t, realDatabaseNow)
	fixtureClock := installB2TaskRecoveryFixtureClock(t, db)
	migrateB2SubmissionFixture(t, db)
	for _, tokenID := range []int{101, 102, 201, 202, 203, 204, 205, 206, 207, 301, 302, 303, 304, 305, 306, 401, 602, 603} {
		ensureB2SubmissionOwner(t, db, tokenID)
	}
	t.Run("database-key-binding", func(t *testing.T) {
		runB2TaskRecoveryIdentityContract(t, db)
	})
	t.Run("database-clock-failure-never-falls-back", func(t *testing.T) {
		fixtureClock.fail = true
		_, err := taskRecoveryDBTimestamp(db)
		assert.ErrorIs(t, err, errInjectedTaskRecoveryClock)
		operation := newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindVideoCreate, "database-clock-failure", `{}`)
		assert.ErrorIs(t, db.Create(operation).Error, errInjectedTaskRecoveryClock)
		fixtureClock.fail = false
		var count int64
		require.NoError(t, db.Model(&TaskSubmissionOperation{}).Where("token_id = ?", 602).Count(&count).Error)
		assert.Zero(t, count)

		persisted := newB2SubmissionOperation(t, 603, "POST", TaskSubmissionOperationKindVideoCreate, "database-clock-transition-failure", `{}`)
		require.NoError(t, db.Create(persisted).Error)
		fixtureClock.fail = true
		won, err := TransitionTaskSubmissionOperation(db, persisted.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved, ExpectedVersion: 1,
		})
		assert.ErrorIs(t, err, errInjectedTaskRecoveryClock)
		assert.False(t, won)
		fixtureClock.fail = false
		require.NoError(t, db.First(persisted, persisted.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusPrepared, persisted.Status)
		assert.Equal(t, int64(1), persisted.LockVersion)
	})

	t.Run("gorm-table-write-guard-protects-durable-records", func(t *testing.T) {
		operation := newB2SubmissionOperation(t, 401, "POST", TaskSubmissionOperationKindVideoCreate, "gorm-table-write-guard", `{}`)
		require.NoError(t, db.Create(operation).Error)
		attempt := createB2SubmissionAttempt(t, db, operation, 41)
		event := &TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: -41,
		}
		require.NoError(t, db.Create(event).Error)
		outbox, err := NewTaskBillingLogOutbox(event, b2BillingLogPayload(event))
		require.NoError(t, err)
		require.NoError(t, db.Create(outbox).Error)

		for _, update := range []struct {
			name, table, column string
			id, value           interface{}
			err                 error
		}{
			{"identity", "task_recovery_identities", "key_verifier", 1, strings.Repeat("f", taskRecoveryDigestLength), ErrTaskRecoveryIdentityImmutable},
			{"operation", "task_submission_operations", "request_fingerprint", operation.ID, strings.Repeat("f", taskRecoveryDigestLength), ErrTaskRecoveryInvalidRecord},
			{"attempt", "task_submission_attempts", "channel_id", attempt.ID, 42, ErrTaskRecoveryInvalidRecord},
			{"event", "task_billing_events", "quota_delta", event.ID, -42, ErrTaskRecoveryInvalidRecord},
			{"outbox", "task_billing_log_outboxes", "billing_event_id", outbox.ID, "billing_evt_" + strings.Repeat("f", taskSubmissionPublicIDRandomLength), ErrTaskRecoveryInvalidRecord},
		} {
			t.Run(update.name, func(t *testing.T) {
				result := db.Table(update.table).Where("id = ?", update.id).Update(update.column, update.value)
				assert.ErrorIs(t, result.Error, update.err)
				assert.Zero(t, result.RowsAffected)
			})
		}
		aliasUpdate := db.Table("task_submission_operations AS tso").Where("tso.id = ?", operation.ID).
			UpdateColumn("request_fingerprint", strings.Repeat("e", taskRecoveryDigestLength))
		assert.ErrorIs(t, aliasUpdate.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, aliasUpdate.RowsAffected)

		variableAliasUpdate := db.Table("? AS tso", clause.Table{Name: "task_submission_operations"}).
			Where("tso.id = ?", operation.ID).
			UpdateColumn("request_fingerprint", strings.Repeat("d", taskRecoveryDigestLength))
		assert.ErrorIs(t, variableAliasUpdate.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, variableAliasUpdate.RowsAffected)

		nestedVariableAliasUpdate := db.Table("? AS tso", clause.NamedExpr{
			SQL: "@target",
			Vars: []interface{}{map[string]interface{}{
				"target": clause.Table{Name: "task_submission_operations"},
			}},
		}).Where("tso.id = ?", operation.ID).
			UpdateColumn("request_fingerprint", strings.Repeat("c", taskRecoveryDigestLength))
		assert.ErrorIs(t, nestedVariableAliasUpdate.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, nestedVariableAliasUpdate.RowsAffected)

		chainedOperation := newB2SubmissionOperation(t, 207, "POST", TaskSubmissionOperationKindVideoCreate, "gorm-marker-leak", `{}`)
		require.NoError(t, db.Create(chainedOperation).Error)
		chainedDB := db.Where("1 = 1")
		reserveB2SubmissionOperation(t, chainedDB, chainedOperation.ID, 1)
		chainedUpdate := chainedDB.Table("task_submission_operations").Where("id = ?", chainedOperation.ID).
			UpdateColumn("request_fingerprint", strings.Repeat("b", taskRecoveryDigestLength))
		assert.ErrorIs(t, chainedUpdate.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, chainedUpdate.RowsAffected)
		chainedDeleteOperation := newB2SubmissionOperation(t, 207, "POST", TaskSubmissionOperationKindVideoCreate, "gorm-marker-leak-delete", `{}`)
		require.NoError(t, db.Create(chainedDeleteOperation).Error)
		chainedDeleteDB := db.Where("1 = 1")
		reserveB2SubmissionOperation(t, chainedDeleteDB, chainedDeleteOperation.ID, 1)
		chainedDelete := chainedDeleteDB.Table("task_submission_operations").Where("id = ?", chainedDeleteOperation.ID).Delete(nil)
		assert.ErrorIs(t, chainedDelete.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, chainedDelete.RowsAffected)

		result := db.Table("task_billing_events").Where("id = ?", event.ID).Delete(nil)
		assert.ErrorIs(t, result.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, result.RowsAffected)

		var unchangedOperation TaskSubmissionOperation
		var unchangedAttempt TaskSubmissionAttempt
		var unchangedEvent TaskBillingEvent
		var unchangedOutbox TaskBillingLogOutbox
		require.NoError(t, db.First(&unchangedOperation, operation.ID).Error)
		require.NoError(t, db.First(&unchangedAttempt, attempt.ID).Error)
		require.NoError(t, db.First(&unchangedEvent, event.ID).Error)
		require.NoError(t, db.First(&unchangedOutbox, outbox.ID).Error)
		assert.Equal(t, operation.RequestFingerprint, unchangedOperation.RequestFingerprint)
		assert.Equal(t, attempt.ChannelID, unchangedAttempt.ChannelID)
		assert.Equal(t, event.QuotaDelta, unchangedEvent.QuotaDelta)
		assert.Equal(t, outbox.BillingEventID, unchangedOutbox.BillingEventID)
	})

	t.Run("stable-public-id-and-idempotency-scope", func(t *testing.T) {
		first := newB2SubmissionOperation(t, 101, "post", TaskSubmissionOperationKindVideoCreate, "same-client-key", `{"model":"fixture-a"}`)
		require.NoError(t, db.Create(first).Error)
		assert.True(t, strings.HasPrefix(first.PublicID, "task_"))
		assert.Len(t, first.PublicID, len("task_")+taskSubmissionPublicIDRandomLength)
		assert.Equal(t, "POST", first.HTTPMethod)
		assert.Equal(t, TaskSubmissionOperationKindVideoCreate, first.OperationKind)

		duplicateScope := newB2SubmissionOperation(t, 101, "POST", TaskSubmissionOperationKindVideoCreate, "same-client-key", `{"model":"fixture-b"}`)
		require.Error(t, db.Create(duplicateScope).Error)

		keyHash, err := HashTaskSubmissionIdempotencyKey("same-client-key")
		require.NoError(t, err)
		found, err := FindTaskSubmissionOperationByIdempotencyScope(db, TaskSubmissionIdempotencyScope{
			TokenID:            101,
			HTTPMethod:         "post",
			OperationKind:      "VIDEO.CREATE",
			IdempotencyKeyHash: keyHash,
		})
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, first.ID, found.ID)
		assert.NotEqual(t, duplicateScope.RequestFingerprint, found.RequestFingerprint)

		for _, operation := range []*TaskSubmissionOperation{
			newB2SubmissionOperation(t, 102, "POST", TaskSubmissionOperationKindVideoCreate, "same-client-key", `{"scope":"token"}`),
			newB2SubmissionOperation(t, 101, "PUT", TaskSubmissionOperationKindVideoCreate, "same-client-key", `{"scope":"method"}`),
			newB2SubmissionOperation(t, 101, "POST", TaskSubmissionOperationKindVideoRemix, "same-client-key", `{"scope":"kind"}`),
			newB2SubmissionOperation(t, 101, "POST", TaskSubmissionOperationKindVideoCreate, "different-client-key", `{"scope":"key"}`),
		} {
			require.NoError(t, db.Create(operation).Error)
			assert.NotEqual(t, first.PublicID, operation.PublicID)
		}

		byPublicID, err := GetTaskSubmissionOperationByPublicID(db, first.PublicID)
		require.NoError(t, err)
		require.NotNil(t, byPublicID)
		assert.Equal(t, first.ID, byPublicID.ID)
	})

	t.Run("create-or-load-replays-immutable-v1-records-without-poisoning-transactions", func(t *testing.T) {
		candidate := newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindVideoCreate, "create-or-load", `{"model":"fixture"}`)
		operation, err := CreateOrLoadTaskSubmissionOperation(db, candidate)
		require.NoError(t, err)
		require.NotNil(t, operation)

		replay, err := CreateOrLoadTaskSubmissionOperation(db,
			newB2SubmissionOperation(t, 602, "post", TaskSubmissionOperationKindVideoCreate, "create-or-load", `{"model":"fixture"}`))
		require.NoError(t, err)
		assert.Equal(t, operation.ID, replay.ID)
		conflict, err := CreateOrLoadTaskSubmissionOperation(db,
			newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindVideoCreate, "create-or-load", `{"model":"different"}`))
		assert.ErrorIs(t, err, ErrTaskSubmissionIdempotencyConflict)
		assert.Equal(t, operation.ID, conflict.ID)

		attempt, err := CreateOrLoadTaskSubmissionAttempt(db, &TaskSubmissionAttempt{
			OperationID: operation.ID, AttemptNo: 1, ChannelID: 14, Provider: "Fixture", RequestClass: "video",
		})
		require.NoError(t, err)
		attemptReplay, err := CreateOrLoadTaskSubmissionAttempt(db, &TaskSubmissionAttempt{
			OperationID: operation.ID, AttemptNo: 1, ChannelID: 14, Provider: "fixture", RequestClass: "video",
		})
		require.NoError(t, err)
		assert.Equal(t, attempt.ID, attemptReplay.ID)
		attemptConflict, err := CreateOrLoadTaskSubmissionAttempt(db, &TaskSubmissionAttempt{
			OperationID: operation.ID, AttemptNo: 1, ChannelID: 15, Provider: "fixture", RequestClass: "video",
		})
		assert.ErrorIs(t, err, ErrTaskSubmissionAttemptConflict)
		assert.Equal(t, attempt.ID, attemptConflict.ID)

		reserveB2SubmissionOperation(t, db, operation.ID, 1)
		startB2SubmissionDispatch(t, db, operation.ID, 2, 1)
		attemptReplay, err = CreateOrLoadTaskSubmissionAttempt(db, &TaskSubmissionAttempt{
			OperationID: operation.ID, AttemptNo: 1, ChannelID: 14, Provider: "fixture", RequestClass: "video",
		})
		require.NoError(t, err)
		assert.Equal(t, TaskSubmissionAttemptStatusDispatching, attemptReplay.Status)

		eventCandidate := &TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: -10,
		}
		event, err := CreateOrLoadTaskBillingEvent(db, eventCandidate)
		require.NoError(t, err)
		eventReplay, err := CreateOrLoadTaskBillingEvent(db, &TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: -10,
		})
		require.NoError(t, err)
		assert.Equal(t, event.ID, eventReplay.ID)
		eventConflict, err := CreateOrLoadTaskBillingEvent(db, &TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: -11,
		})
		assert.ErrorIs(t, err, ErrTaskBillingEventConflict)
		assert.Equal(t, event.ID, eventConflict.ID)

		outboxCandidate, err := NewTaskBillingLogOutbox(event, TaskBillingLogPayload{
			Content: "create or load receipt", ModelName: "fixture-model", Group: "default",
		})
		require.NoError(t, err)
		outbox, err := CreateOrLoadTaskBillingLogOutbox(db, outboxCandidate)
		require.NoError(t, err)
		outboxReplay, err := CreateOrLoadTaskBillingLogOutbox(db, outboxCandidate)
		require.NoError(t, err)
		assert.Equal(t, outbox.ID, outboxReplay.ID)
		outboxConflictCandidate := *outboxCandidate
		outboxConflictCandidate.Payload.Content = "different immutable receipt"
		outboxConflict, err := CreateOrLoadTaskBillingLogOutbox(db, &outboxConflictCandidate)
		assert.ErrorIs(t, err, ErrTaskBillingLogOutboxConflict)
		assert.Equal(t, outbox.ID, outboxConflict.ID)

		// Raw SQL intentionally simulates a privileged historical corruption;
		// ordinary GORM table writes are rejected by the durable-record guard.
		require.NoError(t, db.Exec("UPDATE task_billing_log_outboxes SET state = ?, attempt_count = ?, delivered_at = ? WHERE id = ?",
			TaskBillingLogOutboxStateDelivered, 1, nil, outbox.ID).Error)
		_, err = CreateOrLoadTaskBillingLogOutbox(db, outboxCandidate)
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)

		require.NoError(t, db.Exec("UPDATE task_billing_events SET state = ?, attempt_count = ?, applied_at = ? WHERE id = ?",
			TaskBillingEventStateApplied, 1, nil, event.ID).Error)
		_, err = CreateOrLoadTaskBillingEvent(db, eventCandidate)
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)

		err = db.Transaction(func(tx *gorm.DB) error {
			_, conflictErr := CreateOrLoadTaskSubmissionOperation(tx,
				newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindVideoCreate, "create-or-load", `{"model":"different"}`))
			require.ErrorIs(t, conflictErr, ErrTaskSubmissionIdempotencyConflict)
			_, createErr := CreateOrLoadTaskSubmissionOperation(tx,
				newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindSunoMusic, "usable-after-conflict", `{"model":"fixture"}`))
			return createErr
		})
		require.NoError(t, err)
	})

	t.Run("atomic-t0-intent-owns-once-and-replays-the-persisted-route", func(t *testing.T) {
		operationCandidate := newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindVideoCreate, "atomic-t0-intent", `{"model":"fixture"}`)
		intent, err := CreateOrLoadTaskSubmissionIntent(db, operationCandidate, &TaskSubmissionAttempt{
			AttemptNo: 1, ChannelID: 61, Provider: "Fixture", RequestClass: "video",
		})
		require.NoError(t, err)
		require.NotNil(t, intent)
		require.NotNil(t, intent.Operation)
		require.NotNil(t, intent.Attempt)
		assert.True(t, intent.Owner)
		assert.Equal(t, intent.Operation.ID, intent.Attempt.OperationID)
		assert.Equal(t, TaskSubmissionOperationStatusPrepared, intent.Operation.Status)
		assert.Equal(t, TaskSubmissionAttemptStatusPrepared, intent.Attempt.Status)
		assert.Equal(t, 61, intent.Attempt.ChannelID)
		assert.Equal(t, "fixture", intent.Attempt.Provider)

		replay, err := CreateOrLoadTaskSubmissionIntent(db,
			newB2SubmissionOperation(t, 602, "post", TaskSubmissionOperationKindVideoCreate, "atomic-t0-intent", `{"model":"fixture"}`),
			&TaskSubmissionAttempt{AttemptNo: 1, ChannelID: 62, Provider: "replacement", RequestClass: "music"})
		require.NoError(t, err)
		require.NotNil(t, replay)
		require.NotNil(t, replay.Operation)
		require.NotNil(t, replay.Attempt)
		assert.False(t, replay.Owner)
		assert.Equal(t, intent.Operation.ID, replay.Operation.ID)
		assert.Equal(t, intent.Attempt.ID, replay.Attempt.ID)
		assert.Equal(t, 61, replay.Attempt.ChannelID)
		assert.Equal(t, "fixture", replay.Attempt.Provider)

		legacyOperation, err := CreateOrLoadTaskSubmissionOperation(db,
			newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindSunoMusic, "atomic-t0-legacy-prepared", `{"model":"fixture"}`))
		require.NoError(t, err)
		legacyAttempt, err := CreateOrLoadTaskSubmissionAttempt(db, &TaskSubmissionAttempt{
			OperationID: legacyOperation.ID, AttemptNo: 1, ChannelID: 63, Provider: "legacy", RequestClass: "music",
		})
		require.NoError(t, err)
		legacyReplay, err := CreateOrLoadTaskSubmissionIntent(db,
			newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindSunoMusic, "atomic-t0-legacy-prepared", `{"model":"fixture"}`),
			&TaskSubmissionAttempt{AttemptNo: 1, ChannelID: 64, Provider: "replacement", RequestClass: "video"})
		require.NoError(t, err)
		require.NotNil(t, legacyReplay)
		assert.False(t, legacyReplay.Owner)
		assert.Equal(t, legacyOperation.ID, legacyReplay.Operation.ID)
		assert.Equal(t, legacyAttempt.ID, legacyReplay.Attempt.ID)
		assert.Equal(t, 63, legacyReplay.Attempt.ChannelID)

		missingAttemptOperation, err := CreateOrLoadTaskSubmissionOperation(db,
			newB2SubmissionOperation(t, 603, "POST", TaskSubmissionOperationKindSunoLyrics, "atomic-t0-missing-attempt", `{"model":"fixture"}`))
		require.NoError(t, err)
		missingAttempt, err := CreateOrLoadTaskSubmissionIntent(db,
			newB2SubmissionOperation(t, 603, "POST", TaskSubmissionOperationKindSunoLyrics, "atomic-t0-missing-attempt", `{"model":"fixture"}`),
			&TaskSubmissionAttempt{AttemptNo: 1, ChannelID: 64, Provider: "replacement", RequestClass: "music"})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.Nil(t, missingAttempt)
		var missingAttemptCount int64
		require.NoError(t, db.Model(&TaskSubmissionAttempt{}).Where("operation_id = ?", missingAttemptOperation.ID).Count(&missingAttemptCount).Error)
		assert.Zero(t, missingAttemptCount, "a damaged legacy operation must not be silently repaired with a new route")

		persistedCandidate := *intent.Operation
		persistedCandidateResult, err := CreateOrLoadTaskSubmissionIntent(db, &persistedCandidate,
			&TaskSubmissionAttempt{AttemptNo: 1, ChannelID: 65, Provider: "replacement", RequestClass: "video"})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.Nil(t, persistedCandidateResult)
		var persistedAttemptCount int64
		require.NoError(t, db.Model(&TaskSubmissionAttempt{}).Where("operation_id = ?", intent.Operation.ID).Count(&persistedAttemptCount).Error)
		assert.Equal(t, int64(1), persistedAttemptCount, "a caller-provided persisted operation must never obtain a second owner attempt")

		conflict, err := CreateOrLoadTaskSubmissionIntent(db,
			newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindVideoCreate, "atomic-t0-intent", `{"model":"different"}`),
			&TaskSubmissionAttempt{AttemptNo: 1, ChannelID: 65, Provider: "replacement", RequestClass: "video"})
		assert.ErrorIs(t, err, ErrTaskSubmissionIdempotencyConflict)
		require.NotNil(t, conflict)
		require.NotNil(t, conflict.Operation)
		assert.False(t, conflict.Owner)
		assert.Equal(t, intent.Operation.ID, conflict.Operation.ID)
		assert.Nil(t, conflict.Attempt)
	})

	if db.Dialector.Name() != "sqlite" {
		t.Run("atomic-t0-intent-concurrent-loser-reuses-the-winner", func(t *testing.T) {
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(2)
			defer sqlDB.SetMaxOpenConns(1)

			arrived := make(chan struct{}, 2)
			release := make(chan struct{})
			barrierEnabled := true
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:b2-task-submission-intent-concurrent-insert", func(tx *gorm.DB) {
				if barrierEnabled && taskRecoveryGormStatementTable(tx) == "task_submission_operations" {
					arrived <- struct{}{}
					<-release
				}
			}))

			type outcome struct {
				intent *TaskSubmissionIntentResult
				err    error
			}
			outcomes := make(chan outcome, 2)
			operationCandidates := []*TaskSubmissionOperation{
				newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindSunoLyrics, "atomic-t0-concurrent", `{"model":"fixture"}`),
				newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindSunoLyrics, "atomic-t0-concurrent", `{"model":"fixture"}`),
			}
			for index, channelID := range []int{67, 68} {
				go func(operationCandidate *TaskSubmissionOperation, channelID int) {
					intent, err := CreateOrLoadTaskSubmissionIntent(db,
						operationCandidate,
						&TaskSubmissionAttempt{AttemptNo: 1, ChannelID: channelID, Provider: "fixture", RequestClass: "music"})
					outcomes <- outcome{intent: intent, err: err}
				}(operationCandidates[index], channelID)
			}
			<-arrived
			<-arrived
			close(release)

			first := <-outcomes
			second := <-outcomes
			barrierEnabled = false
			require.NoError(t, first.err)
			require.NoError(t, second.err)
			require.NotNil(t, first.intent)
			require.NotNil(t, second.intent)
			require.NotNil(t, first.intent.Operation)
			require.NotNil(t, second.intent.Operation)
			require.NotNil(t, first.intent.Attempt)
			require.NotNil(t, second.intent.Attempt)
			assert.NotEqual(t, first.intent.Owner, second.intent.Owner)
			assert.Equal(t, first.intent.Operation.ID, second.intent.Operation.ID)
			assert.Equal(t, first.intent.Attempt.ID, second.intent.Attempt.ID)
			assert.Equal(t, first.intent.Attempt.ChannelID, second.intent.Attempt.ChannelID)
		})
	}

	t.Run("atomic-t0-intent-rolls-back-an-operation-when-attempt-creation-fails", func(t *testing.T) {
		operationCandidate := newB2SubmissionOperation(t, 602, "POST", TaskSubmissionOperationKindVideoRemix, "atomic-t0-attempt-failure", `{"model":"fixture"}`)
		var operationCountBefore, attemptCountBefore int64
		require.NoError(t, db.Model(&TaskSubmissionOperation{}).Count(&operationCountBefore).Error)
		require.NoError(t, db.Model(&TaskSubmissionAttempt{}).Count(&attemptCountBefore).Error)

		failAttemptCreate := false
		operationVisibleAtFailure := false
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:b2-task-submission-intent-attempt-failure", func(tx *gorm.DB) {
			if !failAttemptCreate || taskRecoveryGormStatementTable(tx) != "task_submission_attempts" {
				return
			}
			var count int64
			query := tx.Session(&gorm.Session{NewDB: true}).Model(&TaskSubmissionOperation{}).Where(
				"token_id = ? AND http_method = ? AND operation_kind = ? AND idempotency_key_hash = ?",
				operationCandidate.TokenID, operationCandidate.HTTPMethod, operationCandidate.OperationKind, operationCandidate.IdempotencyKeyHash,
			).Count(&count)
			if query.Error != nil {
				tx.AddError(query.Error)
				return
			}
			operationVisibleAtFailure = count == 1
			tx.AddError(errInjectedTaskSubmissionIntentAttemptCreate)
		}))

		outerErr := db.Session(&gorm.Session{DisableNestedTransaction: true}).Transaction(func(tx *gorm.DB) error {
			failAttemptCreate = true
			failed, err := CreateOrLoadTaskSubmissionIntent(tx, operationCandidate, &TaskSubmissionAttempt{
				AttemptNo: 1, ChannelID: 66, Provider: "fixture", RequestClass: "video",
			})
			failAttemptCreate = false
			assert.ErrorIs(t, err, errInjectedTaskSubmissionIntentAttemptCreate)
			assert.Nil(t, failed)

			loaded, loadErr := FindTaskSubmissionOperationByIdempotencyScope(tx, TaskSubmissionIdempotencyScope{
				TokenID: operationCandidate.TokenID, HTTPMethod: operationCandidate.HTTPMethod,
				OperationKind: operationCandidate.OperationKind, IdempotencyKeyHash: operationCandidate.IdempotencyKeyHash,
			})
			require.NoError(t, loadErr)
			assert.Nil(t, loaded)
			return nil
		})
		require.NoError(t, outerErr)
		assert.True(t, operationVisibleAtFailure, "operation must have been inserted before the attempt failure was injected")

		var operationCountAfter, attemptCountAfter int64
		require.NoError(t, db.Model(&TaskSubmissionOperation{}).Count(&operationCountAfter).Error)
		require.NoError(t, db.Model(&TaskSubmissionAttempt{}).Count(&attemptCountAfter).Error)
		assert.Equal(t, operationCountBefore, operationCountAfter)
		assert.Equal(t, attemptCountBefore, attemptCountAfter)
	})

	t.Run("atomic-t0-intent-does-not-enter-an-outer-transaction-when-savepoint-creation-fails", func(t *testing.T) {
		failSavepointCreate := false
		require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test:b2-task-submission-intent-savepoint-failure", func(tx *gorm.DB) {
			if failSavepointCreate && strings.HasPrefix(tx.Statement.SQL.String(), "SAVEPOINT task_submission_intent_") {
				tx.AddError(errInjectedTaskSubmissionIntentSavepoint)
			}
		}))

		outerTx := db.Session(&gorm.Session{DisableNestedTransaction: true}).Begin()
		require.NoError(t, outerTx.Error)
		t.Cleanup(func() { _ = outerTx.Rollback().Error })
		candidate := newB2SubmissionOperation(t, 603, "POST", TaskSubmissionOperationKindSunoLyrics, "atomic-t0-savepoint-failure", `{"model":"fixture"}`)
		failSavepointCreate = true
		result, err := CreateOrLoadTaskSubmissionIntent(outerTx, candidate, &TaskSubmissionAttempt{
			AttemptNo: 1, ChannelID: 67, Provider: "fixture", RequestClass: "music",
		})
		failSavepointCreate = false
		assert.ErrorIs(t, err, errInjectedTaskSubmissionIntentSavepoint)
		assert.Nil(t, result)
		require.NoError(t, outerTx.Commit().Error)

		persisted, err := FindTaskSubmissionOperationByIdempotencyScope(db, TaskSubmissionIdempotencyScope{
			TokenID: candidate.TokenID, HTTPMethod: candidate.HTTPMethod,
			OperationKind: candidate.OperationKind, IdempotencyKeyHash: candidate.IdempotencyKeyHash,
		})
		require.NoError(t, err)
		assert.Nil(t, persisted)
	})

	t.Run("atomic-t0-intent-invalidates-an-outer-transaction-when-savepoint-rollback-fails", func(t *testing.T) {
		failAttemptCreate := false
		failSavepointRollback := false
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:b2-task-submission-intent-rollback-failure-attempt", func(tx *gorm.DB) {
			if failAttemptCreate && taskRecoveryGormStatementTable(tx) == "task_submission_attempts" {
				tx.AddError(errInjectedTaskSubmissionIntentAttemptCreate)
			}
		}))
		require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test:b2-task-submission-intent-rollback-failure", func(tx *gorm.DB) {
			if failSavepointRollback && strings.HasPrefix(strings.ToUpper(tx.Statement.SQL.String()), "ROLLBACK TO SAVEPOINT TASK_SUBMISSION_INTENT_") {
				tx.AddError(errInjectedTaskSubmissionIntentRollback)
			}
		}))

		outerTx := db.Session(&gorm.Session{DisableNestedTransaction: true}).Begin()
		require.NoError(t, outerTx.Error)
		t.Cleanup(func() { _ = outerTx.Rollback().Error })
		candidate := newB2SubmissionOperation(t, 603, "POST", TaskSubmissionOperationKindSunoLyrics, "atomic-t0-rollback-failure", `{"model":"fixture"}`)
		failAttemptCreate = true
		failSavepointRollback = true
		result, err := CreateOrLoadTaskSubmissionIntent(outerTx, candidate, &TaskSubmissionAttempt{
			AttemptNo: 1, ChannelID: 68, Provider: "fixture", RequestClass: "music",
		})
		failAttemptCreate = false
		failSavepointRollback = false
		assert.ErrorIs(t, err, errInjectedTaskSubmissionIntentAttemptCreate)
		assert.Nil(t, result)
		assert.ErrorIs(t, outerTx.Commit().Error, sql.ErrTxDone)

		persisted, err := FindTaskSubmissionOperationByIdempotencyScope(db, TaskSubmissionIdempotencyScope{
			TokenID: candidate.TokenID, HTTPMethod: candidate.HTTPMethod,
			OperationKind: candidate.OperationKind, IdempotencyKeyHash: candidate.IdempotencyKeyHash,
		})
		require.NoError(t, err)
		assert.Nil(t, persisted)
	})

	t.Run("atomic-t0-intent-rolls-back-before-a-recovered-panic-can-commit", func(t *testing.T) {
		panicAttemptCreate := false
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:b2-task-submission-intent-panic-attempt", func(tx *gorm.DB) {
			if panicAttemptCreate && taskRecoveryGormStatementTable(tx) == "task_submission_attempts" {
				panic(errInjectedTaskSubmissionIntentAttemptPanic)
			}
		}))

		outerTx := db.Session(&gorm.Session{DisableNestedTransaction: true}).Begin()
		require.NoError(t, outerTx.Error)
		t.Cleanup(func() { _ = outerTx.Rollback().Error })
		candidate := newB2SubmissionOperation(t, 603, "POST", TaskSubmissionOperationKindSunoLyrics, "atomic-t0-panic", `{"model":"fixture"}`)
		recovered := false
		func() {
			defer func() {
				value := recover()
				require.NotNil(t, value)
				panicError, ok := value.(error)
				require.True(t, ok)
				assert.ErrorIs(t, panicError, errInjectedTaskSubmissionIntentAttemptPanic)
				recovered = true
			}()
			panicAttemptCreate = true
			_, _ = CreateOrLoadTaskSubmissionIntent(outerTx, candidate, &TaskSubmissionAttempt{
				AttemptNo: 1, ChannelID: 69, Provider: "fixture", RequestClass: "music",
			})
		}()
		panicAttemptCreate = false
		require.True(t, recovered)

		persisted, err := FindTaskSubmissionOperationByIdempotencyScope(outerTx, TaskSubmissionIdempotencyScope{
			TokenID: candidate.TokenID, HTTPMethod: candidate.HTTPMethod,
			OperationKind: candidate.OperationKind, IdempotencyKeyHash: candidate.IdempotencyKeyHash,
		})
		require.NoError(t, err)
		assert.Nil(t, persisted)
		require.NoError(t, outerTx.Commit().Error)

		persisted, err = FindTaskSubmissionOperationByIdempotencyScope(db, TaskSubmissionIdempotencyScope{
			TokenID: candidate.TokenID, HTTPMethod: candidate.HTTPMethod,
			OperationKind: candidate.OperationKind, IdempotencyKeyHash: candidate.IdempotencyKeyHash,
		})
		require.NoError(t, err)
		assert.Nil(t, persisted)
	})

	t.Run("operation-cas-and-formal-task-link", func(t *testing.T) {
		operation := newB2SubmissionOperation(t, 201, "POST", TaskSubmissionOperationKindVideoCreate, "operation-cas", `{}`)
		require.NoError(t, db.Create(operation).Error)
		negativeCreatedAt := newB2SubmissionOperation(t, 205, "POST", TaskSubmissionOperationKindVideoCreate, "negative-created-at", `{}`)
		negativeCreatedAt.CreatedAt = -1
		assert.ErrorIs(t, db.Create(negativeCreatedAt).Error, ErrTaskRecoveryInvalidRecord)
		operationSave := *operation
		operationSave.PublicID = "task_" + strings.Repeat("x", taskSubmissionPublicIDRandomLength)
		operationSave.Status = TaskSubmissionOperationStatusReserved
		assert.ErrorIs(t, db.Save(&operationSave).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskSubmissionOperation{}).Where("id = ?", operation.ID).Updates(TaskSubmissionOperation{
			PublicID: operationSave.PublicID, Status: TaskSubmissionOperationStatusReserved,
		}).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskSubmissionOperation{}).Where("id = ?", operation.ID).Updates(map[string]interface{}{
			"public_id": operationSave.PublicID, "status": TaskSubmissionOperationStatusReserved,
		}).Error, ErrTaskRecoveryInvalidRecord)
		updateColumns := db.Model(&TaskSubmissionOperation{}).Where("id = ?", operation.ID).UpdateColumns(map[string]interface{}{
			"request_fingerprint": strings.Repeat("f", taskRecoveryDigestLength),
		})
		assert.ErrorIs(t, updateColumns.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, updateColumns.RowsAffected)
		var unchangedOperation TaskSubmissionOperation
		require.NoError(t, db.First(&unchangedOperation, operation.ID).Error)
		assert.Equal(t, operation.PublicID, unchangedOperation.PublicID)
		assert.Equal(t, operation.RequestFingerprint, unchangedOperation.RequestFingerprint)
		assert.Equal(t, TaskSubmissionOperationStatusPrepared, unchangedOperation.Status)
		assert.ErrorIs(t, db.Delete(&unchangedOperation).Error, ErrTaskRecoveryInvalidRecord)
		won, err := TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved,
			ExpectedVersion: 1, TransitionedAt: -1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		for _, invalidTime := range []int64{fixtureClock.now - 1, fixtureClock.now + 1} {
			won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
				From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved,
				ExpectedVersion: 1, TransitionedAt: invalidTime,
			})
			assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
			assert.False(t, won)
		}
		createdAt := fixtureClock.now
		fixtureClock.now = createdAt - 1
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved, ExpectedVersion: 1,
		})
		require.NoError(t, err)
		assert.False(t, won)
		fixtureClock.now = createdAt
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusReserved,
			ExpectedVersion: taskRecoveryMaxInt64, TransitionedAt: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)

		attempt := createB2SubmissionAttempt(t, db, operation, 16)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusPrepared,
			To:              TaskSubmissionOperationStatusReserved,
			ExpectedVersion: 1,
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusPrepared,
			To:              TaskSubmissionOperationStatusRejected,
			ExpectedVersion: 1,
		})
		require.NoError(t, err)
		assert.False(t, won)

		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusReserved,
			To:              TaskSubmissionOperationStatusDispatching,
			ExpectedVersion: 2,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusPrepared, To: TaskSubmissionAttemptStatusDispatching, ExpectedVersion: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		won, err = StartTaskSubmissionDispatch(db, operation.ID, TaskSubmissionDispatchTransition{
			ExpectedOperationVersion: 2, ExpectedAttemptVersion: 2,
		})
		require.NoError(t, err)
		assert.False(t, won)
		var afterAtomicLossOperation TaskSubmissionOperation
		var afterAtomicLossAttempt TaskSubmissionAttempt
		require.NoError(t, db.First(&afterAtomicLossOperation, operation.ID).Error)
		require.NoError(t, db.First(&afterAtomicLossAttempt, attempt.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusReserved, afterAtomicLossOperation.Status)
		assert.Equal(t, int64(2), afterAtomicLossOperation.LockVersion)
		assert.Equal(t, TaskSubmissionAttemptStatusPrepared, afterAtomicLossAttempt.Status)
		assert.Equal(t, int64(1), afterAtomicLossAttempt.LockVersion)
		won, err = StartTaskSubmissionDispatch(db, operation.ID, TaskSubmissionDispatchTransition{
			ExpectedOperationVersion: 2, ExpectedAttemptVersion: 1, TransitionedAt: -1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = StartTaskSubmissionDispatch(db, operation.ID, TaskSubmissionDispatchTransition{
			ExpectedOperationVersion: taskRecoveryMaxInt64, ExpectedAttemptVersion: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		startB2SubmissionDispatch(t, db, operation.ID, 2, 1)
		var dispatchedOperation TaskSubmissionOperation
		var dispatchedAttempt TaskSubmissionAttempt
		require.NoError(t, db.First(&dispatchedOperation, operation.ID).Error)
		require.NoError(t, db.First(&dispatchedAttempt, attempt.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusDispatching, dispatchedOperation.Status)
		assert.Equal(t, TaskSubmissionAttemptStatusDispatching, dispatchedAttempt.Status)
		require.NotNil(t, dispatchedOperation.DispatchStartedAt)
		require.NotNil(t, dispatchedAttempt.StartedAt)
		assert.Equal(t, *dispatchedOperation.DispatchStartedAt, *dispatchedAttempt.StartedAt)

		failDispatchAttemptUpdate := false
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:b2-task-recovery-dispatch-attempt-failure", func(tx *gorm.DB) {
			if failDispatchAttemptUpdate && taskRecoveryGormStatementTable(tx) == "task_submission_attempts" {
				tx.AddError(errInjectedTaskRecoveryDispatchAttemptUpdate)
			}
		}))
		atomicOperation := newB2SubmissionOperation(t, 205, "POST", TaskSubmissionOperationKindVideoCreate, "dispatch-savepoint", `{}`)
		require.NoError(t, db.Create(atomicOperation).Error)
		atomicAttempt := createB2SubmissionAttempt(t, db, atomicOperation, 18)
		reserveB2SubmissionOperation(t, db, atomicOperation.ID, 1)
		err = db.Session(&gorm.Session{DisableNestedTransaction: true}).Transaction(func(tx *gorm.DB) error {
			failDispatchAttemptUpdate = true
			defer func() { failDispatchAttemptUpdate = false }()
			won, dispatchErr := StartTaskSubmissionDispatch(tx, atomicOperation.ID, TaskSubmissionDispatchTransition{
				ExpectedOperationVersion: 2, ExpectedAttemptVersion: 1,
			})
			assert.ErrorIs(t, dispatchErr, errInjectedTaskRecoveryDispatchAttemptUpdate)
			assert.False(t, won)
			var unchangedAtomicOperation TaskSubmissionOperation
			var unchangedAtomicAttempt TaskSubmissionAttempt
			require.NoError(t, tx.First(&unchangedAtomicOperation, atomicOperation.ID).Error)
			require.NoError(t, tx.First(&unchangedAtomicAttempt, atomicAttempt.ID).Error)
			assert.Equal(t, TaskSubmissionOperationStatusReserved, unchangedAtomicOperation.Status)
			assert.Equal(t, int64(2), unchangedAtomicOperation.LockVersion)
			assert.Equal(t, TaskSubmissionAttemptStatusPrepared, unchangedAtomicAttempt.Status)
			assert.Equal(t, int64(1), unchangedAtomicAttempt.LockVersion)
			return nil
		})
		require.NoError(t, err)
		var persistedAtomicOperation TaskSubmissionOperation
		var persistedAtomicAttempt TaskSubmissionAttempt
		require.NoError(t, db.First(&persistedAtomicOperation, atomicOperation.ID).Error)
		require.NoError(t, db.First(&persistedAtomicAttempt, atomicAttempt.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusReserved, persistedAtomicOperation.Status)
		assert.Equal(t, TaskSubmissionAttemptStatusPrepared, persistedAtomicAttempt.Status)

		panicDispatchAttemptUpdate := false
		require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:b2-task-recovery-dispatch-attempt-panic", func(tx *gorm.DB) {
			if panicDispatchAttemptUpdate && taskRecoveryGormStatementTable(tx) == "task_submission_attempts" {
				panic(errInjectedTaskRecoveryDispatchAttemptPanic)
			}
		}))
		panicOperation := newB2SubmissionOperation(t, 206, "POST", TaskSubmissionOperationKindVideoCreate, "dispatch-savepoint-panic", `{}`)
		require.NoError(t, db.Create(panicOperation).Error)
		panicAttempt := createB2SubmissionAttempt(t, db, panicOperation, 19)
		reserveB2SubmissionOperation(t, db, panicOperation.ID, 1)
		outerTx := db.Session(&gorm.Session{DisableNestedTransaction: true}).Begin()
		require.NoError(t, outerTx.Error)
		outerTxOpen := true
		defer func() {
			if outerTxOpen {
				_ = outerTx.Rollback().Error
			}
		}()
		var recovered interface{}
		func() {
			panicDispatchAttemptUpdate = true
			defer func() {
				panicDispatchAttemptUpdate = false
				recovered = recover()
			}()
			_, _ = StartTaskSubmissionDispatch(outerTx, panicOperation.ID, TaskSubmissionDispatchTransition{
				ExpectedOperationVersion: 2, ExpectedAttemptVersion: 1,
			})
		}()
		panicErr, ok := recovered.(error)
		require.True(t, ok)
		assert.ErrorIs(t, panicErr, errInjectedTaskRecoveryDispatchAttemptPanic)
		require.NoError(t, outerTx.Commit().Error)
		outerTxOpen = false
		var persistedPanicOperation TaskSubmissionOperation
		var persistedPanicAttempt TaskSubmissionAttempt
		require.NoError(t, db.First(&persistedPanicOperation, panicOperation.ID).Error)
		require.NoError(t, db.First(&persistedPanicAttempt, panicAttempt.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusReserved, persistedPanicOperation.Status)
		assert.Equal(t, int64(2), persistedPanicOperation.LockVersion)
		assert.Equal(t, TaskSubmissionAttemptStatusPrepared, persistedPanicAttempt.Status)
		assert.Equal(t, int64(1), persistedPanicAttempt.LockVersion)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusDispatching,
			To:              TaskSubmissionOperationStatusAccepted,
			ExpectedVersion: 3,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)

		wrongOwnerTask := Task{TaskID: operation.PublicID, UserId: operation.UserID + 1, Status: TaskStatusSubmitted}
		require.NoError(t, db.Create(&wrongOwnerTask).Error)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusDispatching, To: TaskSubmissionOperationStatusAccepted,
			TaskID: &wrongOwnerTask.ID, ExpectedVersion: 3,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)

		formalTask := Task{TaskID: operation.PublicID, UserId: operation.UserID, Status: TaskStatusSubmitted}
		require.NoError(t, db.Create(&formalTask).Error)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusDispatching,
			To:              TaskSubmissionOperationStatusAccepted,
			TaskID:          &formalTask.ID,
			ExpectedVersion: 3,
		})
		require.NoError(t, err)
		assert.True(t, won)

		var persisted TaskSubmissionOperation
		require.NoError(t, db.First(&persisted, operation.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusAccepted, persisted.Status)
		require.NotNil(t, persisted.TaskID)
		assert.Equal(t, formalTask.ID, *persisted.TaskID)
		assert.Nil(t, persisted.RetentionUntil, "an accepted active task must not receive a retention deadline")

		secondOperation := newB2SubmissionOperation(t, 203, "POST", TaskSubmissionOperationKindVideoCreate, "operation-task-unique", `{}`)
		require.NoError(t, db.Create(secondOperation).Error)
		require.Error(t, db.Model(secondOperation).Update("task_id", formalTask.ID).Error)
	})

	t.Run("unknown-states-require-explicit-resolution", func(t *testing.T) {
		operation := newB2SubmissionOperation(t, 202, "POST", TaskSubmissionOperationKindVideoCreate, "operation-unknown", `{}`)
		require.NoError(t, db.Create(operation).Error)
		createB2SubmissionAttempt(t, db, operation, 17)
		reserveB2SubmissionOperation(t, db, operation.ID, 1)
		startB2SubmissionDispatch(t, db, operation.ID, 2, 1)
		won, err := TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusDispatching, To: TaskSubmissionOperationStatusSubmissionUnknown, ExpectedVersion: 3,
		})
		require.NoError(t, err)
		assert.True(t, won)
		deadline := int64(999)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusSubmissionUnknown,
			To:              TaskSubmissionOperationStatusAccepted,
			RetentionUntil:  &deadline,
			ExpectedVersion: 4,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:             TaskSubmissionOperationStatusSubmissionUnknown,
			To:               TaskSubmissionOperationStatusRejected,
			ResolutionSource: TaskSubmissionResolutionSourceManualAudit,
			ExpectedVersion:  4,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusSubmissionUnknown,
			To:              TaskSubmissionOperationStatusSucceeded,
			ExpectedVersion: 4,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)

		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:            TaskSubmissionOperationStatusSubmissionUnknown,
			To:              TaskSubmissionOperationStatusRejected,
			ExpectedVersion: 4,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From:             TaskSubmissionOperationStatusSubmissionUnknown,
			To:               TaskSubmissionOperationStatusRejected,
			ResolutionSource: TaskSubmissionResolutionSourceProviderVerified,
			ExpectedVersion:  4,
		})
		require.NoError(t, err)
		assert.True(t, won)
		require.NoError(t, db.First(operation, operation.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusRejected, operation.Status)
		assert.Equal(t, fixtureClock.now+TaskSubmissionTerminalRetentionSeconds, *operation.RetentionUntil)
	})

	t.Run("outcome-unknown-requires-explicit-terminal-resolution", func(t *testing.T) {
		operation := newB2SubmissionOperation(t, 204, "POST", TaskSubmissionOperationKindVideoCreate, "outcome-unknown", `{}`)
		require.NoError(t, db.Create(operation).Error)
		createB2SubmissionAttempt(t, db, operation, 17)
		reserveB2SubmissionOperation(t, db, operation.ID, 1)
		startB2SubmissionDispatch(t, db, operation.ID, 2, 1)
		formalTask := Task{TaskID: operation.PublicID, UserId: operation.UserID, ChannelId: 17, Status: TaskStatusSubmitted}
		require.NoError(t, db.Create(&formalTask).Error)
		won, err := TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusDispatching, To: TaskSubmissionOperationStatusAccepted,
			TaskID: &formalTask.ID, ExpectedVersion: 3,
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusAccepted, To: TaskSubmissionOperationStatusOutcomeUnknown, ExpectedVersion: 4,
		})
		require.NoError(t, err)
		assert.True(t, won)

		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusOutcomeUnknown, To: TaskSubmissionOperationStatusSucceeded,
			ExpectedVersion: 5,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusOutcomeUnknown, To: TaskSubmissionOperationStatusSucceeded,
			ResolutionSource: TaskSubmissionResolutionSourceProviderVerified,
			ExpectedVersion:  5,
		})
		require.NoError(t, err)
		assert.True(t, won)
		require.NoError(t, db.First(operation, operation.ID).Error)
		assert.Equal(t, TaskSubmissionOperationStatusSucceeded, operation.Status)
		assert.Equal(t, TaskSubmissionResolutionSourceProviderVerified, operation.ResolutionSource)
		require.NotNil(t, operation.RetentionUntil)
		assert.Equal(t, fixtureClock.now+TaskSubmissionTerminalRetentionSeconds, *operation.RetentionUntil)
	})

	t.Run("attempt-event-and-outbox-uniqueness", func(t *testing.T) {
		operation := newB2SubmissionOperation(t, 301, "POST", TaskSubmissionOperationKindVideoCreate, "child-records", `{}`)
		require.NoError(t, db.Create(operation).Error)
		assert.ErrorIs(t, db.Create(&TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: 9, BillingSource: "wallet", QuotaDelta: 0,
		}).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Create(&TaskSubmissionAttempt{
			OperationID: operation.ID, AttemptNo: 2, ChannelID: 9, Provider: "fixture", RequestClass: "video",
		}).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Create(&TaskSubmissionAttempt{
			OperationID: 9_999_999, AttemptNo: 1, ChannelID: 9, Provider: "fixture", RequestClass: "video",
		}).Error, ErrTaskSubmissionOperationNotFound)

		unknownOperation := newB2SubmissionOperation(t, 302, "POST", TaskSubmissionOperationKindVideoCreate, "attempt-unknown-operation", `{}`)
		require.NoError(t, db.Create(unknownOperation).Error)
		createB2SubmissionAttempt(t, db, unknownOperation, 9)
		reserveB2SubmissionOperation(t, db, unknownOperation.ID, 1)
		startB2SubmissionDispatch(t, db, unknownOperation.ID, 2, 1)
		won, err := TransitionTaskSubmissionOperation(db, unknownOperation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusDispatching, To: TaskSubmissionOperationStatusSubmissionUnknown, ExpectedVersion: 3,
		})
		require.NoError(t, err)
		assert.True(t, won)
		assert.ErrorIs(t, db.Create(&TaskSubmissionAttempt{
			OperationID: unknownOperation.ID, AttemptNo: 1, ChannelID: 9, Provider: "fixture", RequestClass: "video",
		}).Error, ErrTaskRecoveryInvalidRecord)

		terminalOperation := newB2SubmissionOperation(t, 303, "POST", TaskSubmissionOperationKindVideoCreate, "attempt-terminal-operation", `{}`)
		require.NoError(t, db.Create(terminalOperation).Error)
		negativeTerminalAt := int64(-1)
		negativeRetention := negativeTerminalAt + TaskSubmissionTerminalRetentionSeconds
		won, err = TransitionTaskSubmissionOperation(db, terminalOperation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusCanceled,
			TransitionedAt: negativeTerminalAt, RetentionUntil: &negativeRetention, ExpectedVersion: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		unsafeRetention := int64(10) + TaskSubmissionTerminalRetentionSeconds
		won, err = TransitionTaskSubmissionOperation(db, terminalOperation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusCanceled,
			RetentionUntil: &unsafeRetention, ExpectedVersion: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		terminalEpoch := fixtureClock.now
		fixtureClock.now = taskRecoveryMaxInt64 - TaskSubmissionTerminalRetentionSeconds + 1
		won, err = TransitionTaskSubmissionOperation(db, terminalOperation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusCanceled, ExpectedVersion: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		fixtureClock.now = terminalEpoch
		won, err = TransitionTaskSubmissionOperation(db, terminalOperation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusPrepared, To: TaskSubmissionOperationStatusCanceled,
			ExpectedVersion: 1,
		})
		require.NoError(t, err)
		assert.True(t, won)
		require.NoError(t, db.First(terminalOperation, terminalOperation.ID).Error)
		assert.Equal(t, fixtureClock.now+TaskSubmissionTerminalRetentionSeconds, *terminalOperation.RetentionUntil)
		assert.ErrorIs(t, db.Create(&TaskSubmissionAttempt{
			OperationID: terminalOperation.ID, AttemptNo: 1, ChannelID: 9, Provider: "fixture", RequestClass: "video",
		}).Error, ErrTaskRecoveryInvalidRecord)

		attempt := TaskSubmissionAttempt{
			OperationID:  operation.ID,
			AttemptNo:    1,
			ChannelID:    9,
			Provider:     "FixtureProvider",
			RequestClass: "video",
		}
		negativeAttemptTime := attempt
		negativeAttemptTime.UpdatedAt = -1
		assert.ErrorIs(t, db.Create(&negativeAttemptTime).Error, ErrTaskRecoveryInvalidRecord)
		require.NoError(t, db.Create(&attempt).Error)
		attemptSave := attempt
		attemptSave.AttemptNo = 2
		attemptSave.Status = TaskSubmissionAttemptStatusDispatching
		assert.ErrorIs(t, db.Save(&attemptSave).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskSubmissionAttempt{}).Where("id = ?", attempt.ID).Updates(TaskSubmissionAttempt{
			AttemptNo: 2, Status: TaskSubmissionAttemptStatusDispatching,
		}).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskSubmissionAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]interface{}{
			"attempt_no": 2, "status": TaskSubmissionAttemptStatusDispatching,
		}).Error, ErrTaskRecoveryInvalidRecord)
		updateColumn := db.Model(&TaskSubmissionAttempt{}).Where("id = ?", attempt.ID).UpdateColumn("channel_id", 99)
		assert.ErrorIs(t, updateColumn.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, updateColumn.RowsAffected)
		var unchangedAttempt TaskSubmissionAttempt
		require.NoError(t, db.First(&unchangedAttempt, attempt.ID).Error)
		assert.Equal(t, 1, unchangedAttempt.AttemptNo)
		assert.Equal(t, attempt.ChannelID, unchangedAttempt.ChannelID)
		assert.Equal(t, TaskSubmissionAttemptStatusPrepared, unchangedAttempt.Status)
		assert.ErrorIs(t, db.Delete(&unchangedAttempt).Error, ErrTaskRecoveryInvalidRecord)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusPrepared, To: TaskSubmissionAttemptStatusDispatching,
			ExpectedVersion: 1, TransitionedAt: -1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusPrepared, To: TaskSubmissionAttemptStatusDispatching,
			ExpectedVersion: taskRecoveryMaxInt64, TransitionedAt: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		duplicateAttempt := attempt
		duplicateAttempt.ID = 0
		duplicateAttempt.CreatedAt = 0
		duplicateAttempt.UpdatedAt = 0
		require.Error(t, db.Create(&duplicateAttempt).Error)

		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusPrepared, To: TaskSubmissionAttemptStatusDispatching,
			ExpectedVersion: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		reserveB2SubmissionOperation(t, db, operation.ID, 1)
		startB2SubmissionDispatch(t, db, operation.ID, 2, 1)
		won, err = StartTaskSubmissionDispatch(db, operation.ID, TaskSubmissionDispatchTransition{
			ExpectedOperationVersion: 2, ExpectedAttemptVersion: 1,
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusDispatching, To: TaskSubmissionAttemptStatusSubmissionUnknown,
			ProviderOperationID: strings.Repeat("p", 192), ExpectedVersion: 2,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusDispatching, To: TaskSubmissionAttemptStatusSubmissionUnknown,
			UpstreamRequestID: strings.Repeat("r", 129), ExpectedVersion: 2,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusDispatching, To: TaskSubmissionAttemptStatusSubmissionUnknown,
			ProviderOperationID: "provider-task-301", UpstreamRequestID: "upstream-request-301",
			OutcomeCode: "read_unknown", ExpectedVersion: 2,
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusSubmissionUnknown, To: TaskSubmissionAttemptStatusAccepted,
			ProviderOperationID: "conflicting-provider-task", ExpectedVersion: 3,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskSubmissionAttempt(db, attempt.ID, TaskSubmissionAttemptTransition{
			From: TaskSubmissionAttemptStatusSubmissionUnknown, To: TaskSubmissionAttemptStatusAccepted,
			OutcomeCode: "provider_verified", ExpectedVersion: 3,
		})
		require.NoError(t, err)
		assert.True(t, won)
		var resolvedAttempt TaskSubmissionAttempt
		require.NoError(t, db.First(&resolvedAttempt, attempt.ID).Error)
		assert.Equal(t, TaskSubmissionAttemptStatusAccepted, resolvedAttempt.Status)
		assert.Equal(t, "provider-task-301", resolvedAttempt.ProviderOperationID)
		assert.Equal(t, "upstream-request-301", resolvedAttempt.UpstreamRequestID)

		event := TaskBillingEvent{
			OperationID:   &operation.ID,
			EventType:     TaskBillingEventTypeReserve,
			UserID:        operation.UserID,
			TokenID:       operation.TokenID,
			ChannelID:     attempt.ChannelID,
			BillingSource: "wallet",
			QuotaDelta:    -500,
			RequestID:     "fixture-request",
		}
		require.NoError(t, db.Create(&event).Error)
		assert.NotEmpty(t, event.EventID)
		assert.Equal(t, "task:"+operation.PublicID+":reserve:v1", event.EventKey)
		assert.Equal(t, TaskRecoveryPayloadVersion, event.PayloadVersion)
		assert.Equal(t, taskBillingEventPayloadFromRecord(&event), event.Payload)
		negativeChannelEvent := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeRefund,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: -1,
			BillingSource: "wallet", QuotaDelta: 1,
		}
		assert.ErrorIs(t, db.Create(&negativeChannelEvent).Error, ErrTaskRecoveryInvalidRecord)
		negativeEventTime := event
		negativeEventTime.ID = 0
		negativeEventTime.EventID = ""
		negativeEventTime.EventKey = ""
		negativeEventTime.EventType = TaskBillingEventTypeRefund
		negativeEventTime.QuotaDelta = 1
		negativeEventTime.Payload = TaskBillingEventPayload{}
		negativeEventTime.PayloadVersion = 0
		negativeEventTime.LockVersion = 0
		negativeEventTime.CreatedAt = -1
		assert.ErrorIs(t, db.Create(&negativeEventTime).Error, ErrTaskRecoveryInvalidRecord)
		wrongOwnerEvent := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeRefund,
			UserID: operation.UserID + 1, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: 1,
		}
		assert.ErrorIs(t, db.Create(&wrongOwnerEvent).Error, ErrTaskRecoveryInvalidRecord)
		wrongTokenEvent := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeRefund,
			UserID: operation.UserID, TokenID: operation.TokenID + 1, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: 1,
		}
		assert.ErrorIs(t, db.Create(&wrongTokenEvent).Error, ErrTaskRecoveryInvalidRecord)
		forgedEventKey := TaskBillingEvent{
			EventKey: "caller-controlled-key", OperationID: &operation.ID,
			EventType: TaskBillingEventTypeRefund, UserID: operation.UserID, TokenID: operation.TokenID,
			ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: 1,
		}
		assert.ErrorIs(t, db.Create(&forgedEventKey).Error, ErrTaskRecoveryInvalidRecord)
		overflowEvent := event
		overflowEvent.ID = 0
		overflowEvent.EventID = ""
		overflowEvent.EventKey += ":overflow"
		overflowEvent.QuotaDelta = taskBillingQuotaMax + 1
		overflowEvent.Payload = TaskBillingEventPayload{}
		overflowEvent.PayloadVersion = 0
		overflowEvent.LockVersion = 0
		overflowEvent.CreatedAt = 0
		overflowEvent.UpdatedAt = 0
		assert.ErrorIs(t, db.Create(&overflowEvent).Error, ErrTaskRecoveryInvalidRecord)
		underflowEvent := overflowEvent
		underflowEvent.EventKey = event.EventKey + ":underflow"
		underflowEvent.QuotaDelta = taskBillingQuotaMin - 1
		assert.ErrorIs(t, db.Create(&underflowEvent).Error, ErrTaskRecoveryInvalidRecord)
		duplicateEventKey := event
		duplicateEventKey.ID = 0
		duplicateEventKey.EventID = ""
		duplicateEventKey.Payload = TaskBillingEventPayload{}
		duplicateEventKey.PayloadVersion = 0
		duplicateEventKey.CreatedAt = 0
		duplicateEventKey.UpdatedAt = 0
		require.Error(t, db.Create(&duplicateEventKey).Error)
		duplicateStableID := event
		duplicateStableID.ID = 0
		duplicateStableID.EventKey = ""
		duplicateStableID.EventType = TaskBillingEventTypeRefund
		duplicateStableID.QuotaDelta = 1
		duplicateStableID.Payload = TaskBillingEventPayload{}
		duplicateStableID.PayloadVersion = 0
		duplicateStableID.CreatedAt = 0
		duplicateStableID.UpdatedAt = 0
		require.Error(t, db.Create(&duplicateStableID).Error)

		formalTask := Task{TaskID: operation.PublicID, UserId: operation.UserID, ChannelId: attempt.ChannelID, Status: TaskStatusSubmitted}
		require.NoError(t, db.Create(&formalTask).Error)
		won, err = TransitionTaskSubmissionOperation(db, operation.ID, TaskSubmissionOperationTransition{
			From: TaskSubmissionOperationStatusDispatching, To: TaskSubmissionOperationStatusAccepted,
			TaskID: &formalTask.ID, ExpectedVersion: 3,
		})
		require.NoError(t, err)
		assert.True(t, won)
		taskReferencedDuplicate := TaskBillingEvent{
			TaskID: &formalTask.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: event.QuotaDelta, RequestID: event.RequestID,
		}
		require.Error(t, db.Create(&taskReferencedDuplicate).Error)
		assert.Equal(t, event.EventKey, taskReferencedDuplicate.EventKey)
		require.NotNil(t, taskReferencedDuplicate.OperationID)
		assert.Equal(t, operation.ID, *taskReferencedDuplicate.OperationID)

		outboxPayload := b2BillingLogPayload(&event)
		assert.ErrorIs(t, db.Create(&TaskBillingLogOutbox{BillingEventID: event.EventID}).Error, ErrTaskRecoveryInvalidRecord)
		for name, mutate := range map[string]func(*TaskBillingLogPayload){
			"user":  func(payload *TaskBillingLogPayload) { payload.UserID++ },
			"token": func(payload *TaskBillingLogPayload) { payload.TokenID++ },
			"quota": func(payload *TaskBillingLogPayload) { payload.Quota++ },
			"type":  func(payload *TaskBillingLogPayload) { payload.Type = LogTypeRefund },
		} {
			t.Run("reject-projection-"+name, func(t *testing.T) {
				wrongPayload := outboxPayload
				mutate(&wrongPayload)
				assert.ErrorIs(t, db.Create(&TaskBillingLogOutbox{
					BillingEventID: event.EventID, Payload: wrongPayload,
				}).Error, ErrTaskRecoveryInvalidRecord)
			})
		}
		generatedOutbox, err := NewTaskBillingLogOutbox(&event, TaskBillingLogPayload{
			Content: "task billing reserve", ModelName: "fixture-model", Group: "default",
		})
		require.NoError(t, err)
		assert.Equal(t, outboxPayload, generatedOutbox.Payload)
		negativeOutboxTime := *generatedOutbox
		negativeOutboxTime.CreatedAt = -1
		assert.ErrorIs(t, db.Create(&negativeOutboxTime).Error, ErrTaskRecoveryInvalidRecord)
		outbox := *generatedOutbox
		require.NoError(t, db.Create(&outbox).Error)
		var persistedEvent TaskBillingEvent
		require.NoError(t, db.First(&persistedEvent, event.ID).Error)
		assert.Equal(t, event.Payload, persistedEvent.Payload)
		var persistedOutbox TaskBillingLogOutbox
		require.NoError(t, db.First(&persistedOutbox, outbox.ID).Error)
		assert.Equal(t, outboxPayload, persistedOutbox.Payload)
		assert.ErrorIs(t, db.Delete(&persistedEvent).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Delete(&persistedOutbox).Error, ErrTaskRecoveryInvalidRecord)

		eventSave := persistedEvent
		eventSave.EventKey = "forged-save-key"
		eventSave.State = TaskBillingEventStateApplied
		assert.ErrorIs(t, db.Save(&eventSave).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskBillingEvent{}).Where("id = ?", event.ID).Updates(TaskBillingEvent{
			EventKey: "forged-struct-key", State: TaskBillingEventStateApplied,
		}).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskBillingEvent{}).Where("id = ?", event.ID).Updates(map[string]interface{}{
			"event_key": "forged-map-key", "state": TaskBillingEventStateApplied,
		}).Error, ErrTaskRecoveryInvalidRecord)
		updateEventColumns := db.Model(&TaskBillingEvent{}).Where("id = ?", event.ID).UpdateColumns(map[string]interface{}{
			"quota_delta": -999,
		})
		assert.ErrorIs(t, updateEventColumns.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, updateEventColumns.RowsAffected)
		outboxSave := persistedOutbox
		outboxSave.BillingEventID = "forged-save-event"
		outboxSave.State = TaskBillingLogOutboxStateDelivered
		assert.ErrorIs(t, db.Save(&outboxSave).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskBillingLogOutbox{}).Where("id = ?", outbox.ID).Updates(TaskBillingLogOutbox{
			BillingEventID: "forged-struct-event", State: TaskBillingLogOutboxStateDelivered,
		}).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskBillingLogOutbox{}).Where("id = ?", outbox.ID).Updates(map[string]interface{}{
			"billing_event_id": "forged-map-event", "state": TaskBillingLogOutboxStateDelivered,
		}).Error, ErrTaskRecoveryInvalidRecord)
		updateOutboxColumn := db.Model(&TaskBillingLogOutbox{}).Where("id = ?", outbox.ID).UpdateColumn("billing_event_id", "billing_evt_"+strings.Repeat("x", taskSubmissionPublicIDRandomLength))
		assert.ErrorIs(t, updateOutboxColumn.Error, ErrTaskRecoveryInvalidRecord)
		assert.Zero(t, updateOutboxColumn.RowsAffected)
		require.NoError(t, db.First(&persistedEvent, event.ID).Error)
		assert.Equal(t, event.EventKey, persistedEvent.EventKey)
		assert.Equal(t, event.QuotaDelta, persistedEvent.QuotaDelta)
		assert.Equal(t, TaskBillingEventStatePending, persistedEvent.State)
		require.NoError(t, db.First(&persistedOutbox, outbox.ID).Error)
		assert.Equal(t, event.EventID, persistedOutbox.BillingEventID)
		assert.Equal(t, TaskBillingLogOutboxStatePending, persistedOutbox.State)
		assert.ErrorIs(t, db.Model(&TaskBillingEvent{}).Where("id = ?", event.ID).
			Update("payload", TaskBillingEventPayload{Version: TaskRecoveryPayloadVersion}).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Model(&TaskBillingLogOutbox{}).Where("id = ?", outbox.ID).
			Update("payload", TaskBillingLogPayload{Version: TaskRecoveryPayloadVersion}).Error, ErrTaskRecoveryInvalidRecord)
		duplicateOutbox := outbox
		duplicateOutbox.ID = 0
		duplicateOutbox.CreatedAt = 0
		duplicateOutbox.UpdatedAt = 0
		require.Error(t, db.Create(&duplicateOutbox).Error)

		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker", LeaseSeconds: 50},
			ExpectedVersion: 1,
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "stale-worker", LeaseSeconds: 50},
			ExpectedVersion: 1,
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateApplied,
			WorkerID: "event-worker", ExpectedVersion: 2,
		})
		require.NoError(t, err)
		assert.True(t, won)

		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStatePending, To: TaskBillingLogOutboxStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "outbox-worker", LeaseSeconds: 50},
			ExpectedVersion: 1,
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateClaimed, To: TaskBillingLogOutboxStateDelivered,
			WorkerID: "outbox-worker", ExpectedVersion: 2,
		})
		require.NoError(t, err)
		assert.True(t, won)

		// Log delivery is at-least-once. Its projection key is indexed but not
		// unique, so a retry remains writable and readers can deduplicate it.
		for i := 0; i < 2; i++ {
			require.NoError(t, CreateLog(db, &Log{
				BillingEventID: event.EventID,
				RequestId:      "log-replay",
				CreatedAt:      1_700_000_000,
			}))
		}
		var replays []Log
		require.NoError(t, db.Where("billing_event_id = ?", event.EventID).Order("id").Find(&replays).Error)
		require.Len(t, replays, 2)
		assert.Equal(t, replays[0].BillingProjectionDigest, replays[1].BillingProjectionDigest)
		assert.NotEqual(t, replays[0].LogRowKey, replays[1].LogRowKey)
		visible, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "log-replay", "")
		require.NoError(t, err)
		assert.EqualValues(t, 1, total)
		require.Len(t, visible, 1)
	})

	t.Run("billing-processing-leases-are-owned-due-and-reclaimable", func(t *testing.T) {
		operation := newB2SubmissionOperation(t, 306, "POST", TaskSubmissionOperationKindVideoCreate, "processing-leases", `{}`)
		require.NoError(t, db.Create(operation).Error)
		attempt := createB2SubmissionAttempt(t, db, operation, 10)
		event := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: 0,
		}
		require.NoError(t, db.Create(&event).Error)
		outbox, err := NewTaskBillingLogOutbox(&event, TaskBillingLogPayload{
			Content: "processing lease", ModelName: "fixture-model", Group: "default",
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(outbox).Error)
		leaseEpoch := fixtureClock.now + 10_000
		at := func(offset int64) int64 {
			fixtureClock.now = leaseEpoch + offset
			return fixtureClock.now
		}
		for _, invalidLeaseSeconds := range []int64{0, -1, TaskRecoveryMaxProcessingLeaseSeconds + 1} {
			won, err := TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
				From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
				Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker-a", LeaseSeconds: invalidLeaseSeconds},
				ExpectedVersion: 1, TransitionedAt: at(90),
			})
			assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
			assert.False(t, won)
		}
		fixtureClock.now = taskRecoveryMaxInt64 - TaskRecoveryMaxProcessingLeaseSeconds + 1
		won, err := TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker-a", LeaseSeconds: TaskRecoveryMaxProcessingLeaseSeconds},
			ExpectedVersion: 1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		at(95)
		boundaryEvent := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeTerminalSettlement,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: 0,
		}
		require.NoError(t, db.Create(&boundaryEvent).Error)
		won, err = TransitionTaskBillingEvent(db, boundaryEvent.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "boundary-worker", LeaseSeconds: TaskRecoveryMaxProcessingLeaseSeconds},
			ExpectedVersion: 1, TransitionedAt: at(96),
		})
		require.NoError(t, err)
		assert.True(t, won)
		var boundaryPersisted TaskBillingEvent
		require.NoError(t, db.First(&boundaryPersisted, boundaryEvent.ID).Error)
		assert.Equal(t, leaseEpoch+96+TaskRecoveryMaxProcessingLeaseSeconds, boundaryPersisted.ClaimedUntil)
		won, err = TransitionTaskBillingEvent(db, boundaryEvent.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateRetryable,
			WorkerID: "boundary-worker", RetryDelaySeconds: TaskRecoveryMaxRetryDelaySeconds, LastErrorCode: "boundary_retry",
			ExpectedVersion: 2, TransitionedAt: at(97),
		})
		require.NoError(t, err)
		assert.True(t, won)
		require.NoError(t, db.First(&boundaryPersisted, boundaryEvent.ID).Error)
		assert.Equal(t, leaseEpoch+97+TaskRecoveryMaxRetryDelaySeconds, boundaryPersisted.NextAttemptAt)

		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker-a", LeaseSeconds: 100},
			ExpectedVersion: 1, TransitionedAt: -1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker-a", LeaseSeconds: 100},
			ExpectedVersion: taskRecoveryMaxInt64,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker-a", LeaseSeconds: 100},
			ExpectedVersion: 1, TransitionedAt: at(100),
		})
		require.NoError(t, err)
		assert.True(t, won)
		var persistedEvent TaskBillingEvent
		require.NoError(t, db.First(&persistedEvent, event.ID).Error)
		assert.Equal(t, TaskBillingEventStateClaimed, persistedEvent.State)
		assert.Equal(t, "event-worker-a", persistedEvent.ClaimedBy)
		assert.Equal(t, leaseEpoch+200, persistedEvent.ClaimedUntil)
		assert.Equal(t, 1, persistedEvent.AttemptCount)
		assert.Equal(t, int64(2), persistedEvent.LockVersion)

		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateApplied,
			WorkerID: "wrong-worker", ExpectedVersion: 2, TransitionedAt: at(150),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = ReclaimExpiredTaskBillingEvent(db, event.ID,
			TaskRecoveryProcessingLease{WorkerID: "event-worker-b", LeaseSeconds: 101}, 2, at(199))
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateApplied,
			WorkerID: "event-worker-a", ExpectedVersion: 2, TransitionedAt: at(200),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = ReclaimExpiredTaskBillingEvent(db, event.ID,
			TaskRecoveryProcessingLease{WorkerID: "event-worker-b", LeaseSeconds: 100}, 2, at(200))
		require.NoError(t, err)
		assert.True(t, won)
		require.NoError(t, db.First(&persistedEvent, event.ID).Error)
		assert.Equal(t, "event-worker-b", persistedEvent.ClaimedBy)
		assert.Equal(t, 2, persistedEvent.AttemptCount)
		assert.Equal(t, "lease_expired", persistedEvent.LastErrorCode)
		assert.Equal(t, int64(3), persistedEvent.LockVersion)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateApplied,
			WorkerID: "event-worker-a", ExpectedVersion: 2, TransitionedAt: at(220),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateRetryable,
			WorkerID: "event-worker-b", RetryDelaySeconds: 30, LastErrorCode: "sink_unavailable",
			ExpectedVersion: 3, TransitionedAt: at(220),
		})
		require.NoError(t, err)
		assert.True(t, won)
		require.NoError(t, db.First(&persistedEvent, event.ID).Error)
		assert.Equal(t, TaskBillingEventStateRetryable, persistedEvent.State)
		assert.Empty(t, persistedEvent.ClaimedBy)
		assert.Zero(t, persistedEvent.ClaimedUntil)
		assert.Equal(t, leaseEpoch+250, persistedEvent.NextAttemptAt)
		assert.Equal(t, "sink_unavailable", persistedEvent.LastErrorCode)
		assert.Equal(t, leaseEpoch+220, persistedEvent.LastErrorAt)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateRetryable, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker-c", LeaseSeconds: 110},
			ExpectedVersion: 4, TransitionedAt: at(240),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateRetryable, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "event-worker-c", LeaseSeconds: 100},
			ExpectedVersion: 4, TransitionedAt: at(250),
		})
		require.NoError(t, err)
		assert.True(t, won)
		for _, invalidRetryDelay := range []int64{0, -1, TaskRecoveryMaxRetryDelaySeconds + 1} {
			won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
				From: TaskBillingEventStateClaimed, To: TaskBillingEventStateRetryable,
				WorkerID: "event-worker-c", RetryDelaySeconds: invalidRetryDelay, LastErrorCode: "invalid_delay",
				ExpectedVersion: 5, TransitionedAt: at(260),
			})
			assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
			assert.False(t, won)
		}
		fixtureClock.now = taskRecoveryMaxInt64 - TaskRecoveryMaxRetryDelaySeconds + 1
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateRetryable,
			WorkerID: "event-worker-c", RetryDelaySeconds: TaskRecoveryMaxRetryDelaySeconds, LastErrorCode: "overflow",
			ExpectedVersion: 5,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskBillingEvent(db, event.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateApplied,
			WorkerID: "event-worker-c", ExpectedVersion: 5, TransitionedAt: at(300),
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStatePending, To: TaskBillingLogOutboxStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "outbox-worker-a", LeaseSeconds: 100},
			ExpectedVersion: 1, TransitionedAt: -1,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		assert.False(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStatePending, To: TaskBillingLogOutboxStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "outbox-worker-a", LeaseSeconds: 100},
			ExpectedVersion: taskRecoveryMaxInt64,
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStatePending, To: TaskBillingLogOutboxStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "outbox-worker-a", LeaseSeconds: 100},
			ExpectedVersion: 1, TransitionedAt: at(1_000),
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateClaimed, To: TaskBillingLogOutboxStateDelivered,
			WorkerID: "wrong-worker", ExpectedVersion: 2, TransitionedAt: at(1_050),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = ReclaimExpiredTaskBillingLogOutbox(db, outbox.ID,
			TaskRecoveryProcessingLease{WorkerID: "outbox-worker-b", LeaseSeconds: 101}, 2, at(1_099))
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateClaimed, To: TaskBillingLogOutboxStateDelivered,
			WorkerID: "outbox-worker-a", ExpectedVersion: 2, TransitionedAt: at(1_100),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = ReclaimExpiredTaskBillingLogOutbox(db, outbox.ID,
			TaskRecoveryProcessingLease{WorkerID: "outbox-worker-b", LeaseSeconds: 100}, 2, at(1_100))
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateClaimed, To: TaskBillingLogOutboxStateDelivered,
			WorkerID: "outbox-worker-a", ExpectedVersion: 2, TransitionedAt: at(1_150),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateClaimed, To: TaskBillingLogOutboxStateRetryable,
			WorkerID: "outbox-worker-b", RetryDelaySeconds: 25, LastErrorCode: "log_sink_unavailable",
			ExpectedVersion: 3, TransitionedAt: at(1_150),
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateRetryable, To: TaskBillingLogOutboxStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "outbox-worker-c", LeaseSeconds: 140},
			ExpectedVersion: 4, TransitionedAt: at(1_160),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateRetryable, To: TaskBillingLogOutboxStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "outbox-worker-c", LeaseSeconds: 125},
			ExpectedVersion: 4, TransitionedAt: at(1_175),
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, outbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStateClaimed, To: TaskBillingLogOutboxStateDelivered,
			WorkerID: "outbox-worker-c", ExpectedVersion: 5, TransitionedAt: at(1_200),
		})
		require.NoError(t, err)
		assert.True(t, won)
		var persistedOutbox TaskBillingLogOutbox
		require.NoError(t, db.First(&persistedOutbox, outbox.ID).Error)
		assert.Equal(t, TaskBillingLogOutboxStateDelivered, persistedOutbox.State)
		assert.Empty(t, persistedOutbox.ClaimedBy)
		assert.Zero(t, persistedOutbox.ClaimedUntil)
		assert.Equal(t, 3, persistedOutbox.AttemptCount)
		assert.Equal(t, int64(6), persistedOutbox.LockVersion)
		require.NotNil(t, persistedOutbox.DeliveredAt)
		assert.Equal(t, leaseEpoch+1_200, *persistedOutbox.DeliveredAt)

		manualReviewEvent := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeSubmissionAdjustment,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: 0,
		}
		require.NoError(t, db.Create(&manualReviewEvent).Error)
		won, err = TransitionTaskBillingEvent(db, manualReviewEvent.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "review-worker", LeaseSeconds: 100},
			ExpectedVersion: 1, TransitionedAt: at(1_400),
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingEvent(db, manualReviewEvent.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateClaimed, To: TaskBillingEventStateManualReview,
			WorkerID: "review-worker", LastErrorCode: "ledger_conflict",
			ExpectedVersion: 2, TransitionedAt: at(1_450),
		})
		require.NoError(t, err)
		assert.True(t, won)
		won, err = TransitionTaskBillingEvent(db, manualReviewEvent.ID, TaskBillingEventTransition{
			From: TaskBillingEventStateManualReview, To: TaskBillingEventStatePending,
			ExpectedVersion: 3, TransitionedAt: at(1_451),
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidTransition)
		assert.False(t, won)
		var persistedReviewEvent TaskBillingEvent
		require.NoError(t, db.First(&persistedReviewEvent, manualReviewEvent.ID).Error)
		assert.Equal(t, TaskBillingEventStateManualReview, persistedReviewEvent.State)
		assert.Empty(t, persistedReviewEvent.ClaimedBy)
		assert.Equal(t, "ledger_conflict", persistedReviewEvent.LastErrorCode)

		cappedEvent := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeRefund,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: 0,
		}
		require.NoError(t, db.Create(&cappedEvent).Error)
		cappedOutbox, err := NewTaskBillingLogOutbox(&cappedEvent, TaskBillingLogPayload{
			Content: "attempt cap", ModelName: "fixture-model", Group: "default",
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(cappedOutbox).Error)
		// Simulate a privileged malformed legacy row; application-level GORM
		// writes must instead use the guarded state transition APIs.
		require.NoError(t, db.Exec("UPDATE task_billing_events SET attempt_count = ? WHERE id = ?", taskRecoveryAttemptCountMax, cappedEvent.ID).Error)
		require.NoError(t, db.Exec("UPDATE task_billing_log_outboxes SET attempt_count = ? WHERE id = ?", taskRecoveryAttemptCountMax, cappedOutbox.ID).Error)
		won, err = TransitionTaskBillingEvent(db, cappedEvent.ID, TaskBillingEventTransition{
			From: TaskBillingEventStatePending, To: TaskBillingEventStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "capped-worker", LeaseSeconds: 100},
			ExpectedVersion: 1, TransitionedAt: at(2_000),
		})
		require.NoError(t, err)
		assert.False(t, won)
		won, err = TransitionTaskBillingLogOutbox(db, cappedOutbox.ID, TaskBillingLogOutboxTransition{
			From: TaskBillingLogOutboxStatePending, To: TaskBillingLogOutboxStateClaimed,
			Lease:           TaskRecoveryProcessingLease{WorkerID: "capped-worker", LeaseSeconds: 100},
			ExpectedVersion: 1, TransitionedAt: at(2_000),
		})
		require.NoError(t, err)
		assert.False(t, won)
	})

	t.Run("billing-event-sign-boundaries-manual-keys-and-free-projection", func(t *testing.T) {
		operation := newB2SubmissionOperation(t, 304, "POST", TaskSubmissionOperationKindVideoCreate, "billing-boundaries", `{}`)
		require.NoError(t, db.Create(operation).Error)
		attempt := createB2SubmissionAttempt(t, db, operation, 11)
		reserveBoundary := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: taskBillingQuotaMin,
		}
		require.NoError(t, db.Create(&reserveBoundary).Error)
		reserveOutbox, err := NewTaskBillingLogOutbox(&reserveBoundary, TaskBillingLogPayload{
			Content: "boundary reserve", ModelName: "fixture-model", Group: "default",
		})
		require.NoError(t, err)
		assert.Equal(t, LogTypeConsume, reserveOutbox.Payload.Type)
		assert.Equal(t, int(taskBillingQuotaMax), reserveOutbox.Payload.Quota)

		refundBoundary := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeRefund,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: taskBillingQuotaMax,
		}
		require.NoError(t, db.Create(&refundBoundary).Error)
		refundOutbox, err := NewTaskBillingLogOutbox(&refundBoundary, TaskBillingLogPayload{
			Content: "boundary refund", ModelName: "fixture-model", Group: "default",
		})
		require.NoError(t, err)
		assert.Equal(t, LogTypeRefund, refundOutbox.Payload.Type)
		assert.Equal(t, int(taskBillingQuotaMax), refundOutbox.Payload.Quota)
		mutatedEvent := reserveBoundary
		mutatedEvent.QuotaDelta = taskBillingQuotaMin - 1
		_, err = NewTaskBillingLogOutbox(&mutatedEvent, TaskBillingLogPayload{
			Content: "invalid mutated event", ModelName: "fixture-model", Group: "default",
		})
		assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
		for name, details := range map[string]TaskBillingLogPayload{
			"prompt-tokens": {
				Content: "invalid prompt tokens", ModelName: "fixture-model", Group: "default", PromptTokens: int(taskBillingQuotaMax) + 1,
			},
			"completion-tokens": {
				Content: "invalid completion tokens", ModelName: "fixture-model", Group: "default", CompletionTokens: int(taskBillingQuotaMax) + 1,
			},
			"use-time": {
				Content: "invalid use time", ModelName: "fixture-model", Group: "default", UseTime: int(taskBillingQuotaMax) + 1,
			},
		} {
			t.Run("reject-projection-overflow-"+name, func(t *testing.T) {
				_, err := NewTaskBillingLogOutbox(&reserveBoundary, details)
				assert.ErrorIs(t, err, ErrTaskRecoveryInvalidRecord)
			})
		}

		for name, invalidEvent := range map[string]TaskBillingEvent{
			"underflow": {
				OperationID: &operation.ID, EventType: TaskBillingEventTypeSubmissionAdjustment,
				UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: taskBillingQuotaMin - 1,
			},
			"overflow": {
				OperationID: &operation.ID, EventType: TaskBillingEventTypeTerminalSettlement,
				UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: taskBillingQuotaMax + 1,
			},
			"positive-reserve": {
				OperationID: &operation.ID, EventType: TaskBillingEventTypeReserve,
				UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: 1,
			},
			"negative-refund": {
				OperationID: &operation.ID, EventType: TaskBillingEventTypeRefund,
				UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: -1,
			},
		} {
			t.Run(name, func(t *testing.T) {
				assert.ErrorIs(t, db.Create(&invalidEvent).Error, ErrTaskRecoveryInvalidRecord)
			})
		}

		freeOperation := newB2SubmissionOperation(t, 305, "POST", TaskSubmissionOperationKindVideoCreate, "free-reserve", `{}`)
		require.NoError(t, db.Create(freeOperation).Error)
		freeAttempt := createB2SubmissionAttempt(t, db, freeOperation, 12)
		freeReserve := TaskBillingEvent{
			OperationID: &freeOperation.ID, EventType: TaskBillingEventTypeReserve,
			UserID: freeOperation.UserID, TokenID: freeOperation.TokenID, ChannelID: freeAttempt.ChannelID, BillingSource: "wallet", QuotaDelta: 0,
		}
		require.NoError(t, db.Create(&freeReserve).Error)
		freeOutbox, err := NewTaskBillingLogOutbox(&freeReserve, TaskBillingLogPayload{
			Content: "free reserve receipt", ModelName: "fixture-model", Group: "default",
		})
		require.NoError(t, err)
		assert.Equal(t, LogTypeSystem, freeOutbox.Payload.Type)
		assert.Zero(t, freeOutbox.Payload.Quota)
		require.NoError(t, db.Create(freeOutbox).Error)

		manualFirst := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeManualResolution,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID, BillingSource: "wallet", QuotaDelta: 0,
			AuditCommandID: "audit-command-001", ResolutionSource: TaskSubmissionResolutionSourceManualAudit,
		}
		require.NoError(t, db.Create(&manualFirst).Error)
		manualSecond := manualFirst
		manualSecond.ID = 0
		manualSecond.EventID = ""
		manualSecond.EventKey = ""
		manualSecond.AuditCommandID = "audit-command-002"
		manualSecond.Payload = TaskBillingEventPayload{}
		manualSecond.PayloadVersion = 0
		manualSecond.LockVersion = 0
		manualSecond.CreatedAt = 0
		manualSecond.UpdatedAt = 0
		require.NoError(t, db.Create(&manualSecond).Error)
		assert.NotEqual(t, manualFirst.EventKey, manualSecond.EventKey)
		manualReplay := manualFirst
		manualReplay.ID = 0
		manualReplay.EventID = ""
		manualReplay.Payload = TaskBillingEventPayload{}
		manualReplay.PayloadVersion = 0
		manualReplay.LockVersion = 0
		manualReplay.CreatedAt = 0
		manualReplay.UpdatedAt = 0
		require.Error(t, db.Create(&manualReplay).Error)
		assert.Equal(t, manualFirst.EventKey, manualReplay.EventKey)
		manualWithoutCommand := manualFirst
		manualWithoutCommand.ID = 0
		manualWithoutCommand.EventID = ""
		manualWithoutCommand.EventKey = ""
		manualWithoutCommand.AuditCommandID = ""
		manualWithoutCommand.Payload = TaskBillingEventPayload{}
		manualWithoutCommand.PayloadVersion = 0
		manualWithoutCommand.LockVersion = 0
		manualWithoutCommand.CreatedAt = 0
		manualWithoutCommand.UpdatedAt = 0
		assert.ErrorIs(t, db.Create(&manualWithoutCommand).Error, ErrTaskRecoveryInvalidRecord)
		assert.ErrorIs(t, db.Create(&TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeRefund,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "wallet", QuotaDelta: 0, ResolutionSource: TaskSubmissionResolutionSourceManualAudit,
		}).Error, ErrTaskRecoveryInvalidRecord)

		subscription := UserSubscription{Id: 700, UserId: operation.UserID, PlanId: 1, Status: "active"}
		require.NoError(t, db.Create(&subscription).Error)
		subscriptionEvent := TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeSubmissionAdjustment,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "subscription", SubscriptionID: subscription.Id, QuotaDelta: 0,
		}
		require.NoError(t, db.Create(&subscriptionEvent).Error)
		foreignSubscription := UserSubscription{Id: 701, UserId: operation.UserID + 1, PlanId: 1, Status: "active"}
		require.NoError(t, db.Create(&foreignSubscription).Error)
		assert.ErrorIs(t, db.Create(&TaskBillingEvent{
			OperationID: &operation.ID, EventType: TaskBillingEventTypeTerminalSettlement,
			UserID: operation.UserID, TokenID: operation.TokenID, ChannelID: attempt.ChannelID,
			BillingSource: "subscription", SubscriptionID: foreignSubscription.Id, QuotaDelta: 0,
		}).Error, ErrTaskRecoveryInvalidRecord)
	})

	t.Run("transaction-rollback", func(t *testing.T) {
		var outboxCountBefore int64
		require.NoError(t, db.Model(&TaskBillingLogOutbox{}).Count(&outboxCountBefore).Error)
		operation := newB2SubmissionOperation(t, 401, "POST", TaskSubmissionOperationKindVideoCreate, "rollback", `{}`)
		err := db.Transaction(func(tx *gorm.DB) error {
			require.NoError(t, tx.Create(operation).Error)
			attempt := createB2SubmissionAttempt(t, tx, operation, 13)
			event := TaskBillingEvent{
				OperationID:   &operation.ID,
				EventType:     TaskBillingEventTypeReserve,
				UserID:        operation.UserID,
				TokenID:       operation.TokenID,
				ChannelID:     attempt.ChannelID,
				BillingSource: "wallet",
				QuotaDelta:    -100,
			}
			require.NoError(t, tx.Create(&event).Error)
			require.NoError(t, tx.Create(&TaskBillingLogOutbox{BillingEventID: event.EventID, Payload: b2BillingLogPayload(&event)}).Error)
			return errInjectedTaskRecoveryRollback
		})
		assert.ErrorIs(t, err, errInjectedTaskRecoveryRollback)
		var operationCount, eventCount, outboxCount int64
		require.NoError(t, db.Model(&TaskSubmissionOperation{}).Where("public_id = ?", operation.PublicID).Count(&operationCount).Error)
		require.NoError(t, db.Model(&TaskBillingEvent{}).Where("operation_id = ?", operation.ID).Count(&eventCount).Error)
		require.NoError(t, db.Model(&TaskBillingLogOutbox{}).Count(&outboxCount).Error)
		assert.Zero(t, operationCount)
		assert.Zero(t, eventCount)
		assert.Equal(t, outboxCountBefore, outboxCount)
	})
}

func TestB2SubmissionSQLite(t *testing.T) {
	runB2SubmissionDatabaseContract(t, openB2SubmissionSQLite(t))
}

func TestB2SubmissionSQLitePreparedOuterTransaction(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLitePrepared(t)
	migrateB2SubmissionFixture(t, db)
	ensureB2SubmissionOwner(t, db, 801)

	outerTx := db.Session(&gorm.Session{DisableNestedTransaction: true}).Begin()
	require.NoError(t, outerTx.Error)
	t.Cleanup(func() { _ = outerTx.Rollback().Error })
	preparedPool, prepared := outerTx.Statement.ConnPool.(*gorm.PreparedStmtTX)
	require.True(t, prepared, "the outer transaction must exercise GORM's prepared transaction wrapper")

	candidate := newB2SubmissionOperation(t, 801, "POST", TaskSubmissionOperationKindVideoCreate, "prepared-outer-transaction", `{"model":"fixture"}`)
	intent, err := CreateOrLoadTaskSubmissionIntent(outerTx, candidate, &TaskSubmissionAttempt{
		AttemptNo: 1, ChannelID: 81, Provider: "fixture", RequestClass: "video",
	})
	require.NoError(t, err)
	require.NotNil(t, intent)
	assert.True(t, intent.Owner)
	currentPreparedPool, stillPrepared := outerTx.Statement.ConnPool.(*gorm.PreparedStmtTX)
	require.True(t, stillPrepared, "transaction-control SQL must not disable prepared SQL for the caller")
	assert.Same(t, preparedPool, currentPreparedPool)
	var operationCount int64
	require.NoError(t, outerTx.Model(&TaskSubmissionOperation{}).Where("id = ?", intent.Operation.ID).Count(&operationCount).Error)
	assert.Equal(t, int64(1), operationCount)
	require.NoError(t, outerTx.Commit().Error)

	persisted, err := FindTaskSubmissionOperationByIdempotencyScope(db, TaskSubmissionIdempotencyScope{
		TokenID: candidate.TokenID, HTTPMethod: candidate.HTTPMethod,
		OperationKind: candidate.OperationKind, IdempotencyKeyHash: candidate.IdempotencyKeyHash,
	})
	require.NoError(t, err)
	require.NotNil(t, persisted)
}

func TestClickHouseLogSchemaReservesBillingEventProjectionKey(t *testing.T) {
	assert.Contains(t, clickHouseLogCreateTableSQL(0), "billing_event_id String DEFAULT ''")
}

func TestB2LogProjectionMigrationPreservesExistingRowsSQLite(t *testing.T) {
	migrateB2LegacyLogsFixture(t, openB2SubmissionSQLite(t))
}
