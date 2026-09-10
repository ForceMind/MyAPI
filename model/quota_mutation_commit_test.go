package model

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var errTaskQuotaCommitAcknowledgement = errors.New("injected lost commit acknowledgement")

// The driver commits successfully, then loses its acknowledgement. This is not
// a rollback failure: the caller must resolve the immutable receipt, not refund.
type taskQuotaCommitAcknowledgementPool struct{ *sql.DB }

func (pool *taskQuotaCommitAcknowledgementPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := pool.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &taskQuotaCommitAcknowledgementTx{Tx: tx}, nil
}

type taskQuotaCommitAcknowledgementTx struct{ *sql.Tx }

func (tx *taskQuotaCommitAcknowledgementTx) Commit() error {
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	return errTaskQuotaCommitAcknowledgement
}

func TestTaskQuotaReservationCommitAcknowledgementSQLite(t *testing.T) {
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db := openB2SubmissionSQLite(t)
	migrateB2SubmissionFixture(t, db)
	runTaskQuotaCommitAcknowledgementContract(t, db)
}

func TestTaskQuotaReservationCommitAcknowledgementConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
		t.Skip("disposable B2 database tests require MYAPI_B2_DATABASE_TESTS=1")
	}
	for _, engine := range []struct{ name, env string }{
		{"mysql", "MYAPI_B2_MYSQL_DSN"},
		{"postgres", "MYAPI_B2_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			require.NotEmpty(t, dsn, "%s must be configured when MYAPI_B2_DATABASE_TESTS=1", engine.env)
			dialector, err := b2SubmissionDatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			require.NoError(t, registerTaskRecoveryGormGuards(db))
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			runTaskQuotaCommitAcknowledgementContract(t, db)
		})
	}
}

func runTaskQuotaCommitAcknowledgementContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	input := newTaskQuotaReservationFixture(t, db, "lost-commit-ack")
	pool, err := db.DB()
	require.NoError(t, err)
	brokenAcknowledgement := db.Session(&gorm.Session{NewDB: true, Initialized: true})
	brokenAcknowledgement.Statement.ConnPool = &taskQuotaCommitAcknowledgementPool{DB: pool}
	receipt, err := ReserveTaskQuota(brokenAcknowledgement, input)
	require.ErrorIs(t, err, errTaskQuotaCommitAcknowledgement)
	assert.Nil(t, receipt, "an uncertain commit must not be returned as an acknowledged reservation")
	assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)
	stored, err := FindTaskQuotaReservation(db.Session(&gorm.Session{NewDB: true}), input.OperationID, input.UserID, input.TokenID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	replayed, err := ReserveTaskQuota(db, input)
	require.NoError(t, err)
	assert.Equal(t, stored, replayed)
	assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)
	readFailure := errors.New("injected unreadable primary")
	callback := "test:quota-receipt-read-unavailable"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_mutation_receipts" {
			tx.AddError(readFailure)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })
	_, err = FindTaskQuotaReservation(db, input.OperationID, input.UserID, input.TokenID)
	require.ErrorIs(t, err, readFailure, "unreadable is not proof of an unapplied mutation")
	assertTaskQuotaWallet(t, db, input, 900, 400, 100, 1)
}
