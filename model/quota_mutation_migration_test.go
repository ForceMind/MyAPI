package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These fixtures reproduce populated quota columns without quota_version.
// They test additive migration, not a complete historical deployment upgrade.
type taskQuotaLegacyUser struct {
	Id          int
	Username    string
	Password    string
	AffCode     string
	AccessToken *string
	Quota       int
	UsedQuota   int
}

func (taskQuotaLegacyUser) TableName() string { return "users" }

type taskQuotaLegacyToken struct {
	Id          int
	UserId      int
	Key         string
	RemainQuota int
	UsedQuota   int
}

func (taskQuotaLegacyToken) TableName() string { return "tokens" }

type taskQuotaLegacySubscription struct {
	Id          int
	UserId      int
	AmountTotal int64 `gorm:"type:bigint;not null;default:0"`
	AmountUsed  int64 `gorm:"type:bigint;not null;default:0"`
}

func (taskQuotaLegacySubscription) TableName() string { return "user_subscriptions" }

func createTaskQuotaLegacyFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&taskQuotaLegacyUser{}, &taskQuotaLegacyToken{}, &taskQuotaLegacySubscription{}))
	require.NoError(t, db.Create(&taskQuotaLegacyUser{Id: 41001, Username: "qr-legacy", Password: "synthetic-fixture", AffCode: "qr-legacy", Quota: 900, UsedQuota: 100}).Error)
	require.NoError(t, db.Create(&taskQuotaLegacyToken{Id: 41001, UserId: 41001, Key: "qr-legacy", RemainQuota: 700, UsedQuota: 300}).Error)
	require.NoError(t, db.Create(&taskQuotaLegacySubscription{Id: 41001, UserId: 41001, AmountTotal: 1200, AmountUsed: 400}).Error)
}

func assertTaskQuotaLegacyFixtureMigrated(t *testing.T, db *gorm.DB) {
	t.Helper()
	var user User
	var token Token
	var subscription UserSubscription
	require.NoError(t, db.First(&user, 41001).Error)
	require.NoError(t, db.First(&token, 41001).Error)
	require.NoError(t, db.First(&subscription, 41001).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Equal(t, 100, user.UsedQuota)
	assert.Zero(t, user.QuotaVersion)
	assert.Equal(t, 700, token.RemainQuota)
	assert.Equal(t, 300, token.UsedQuota)
	assert.Zero(t, token.QuotaVersion)
	assert.Equal(t, int64(1200), subscription.AmountTotal)
	assert.Equal(t, int64(400), subscription.AmountUsed)
	assert.Zero(t, subscription.QuotaVersion)
}

type legacyQuotaMutationReceipt struct {
	ID                          int64                        `json:"id" gorm:"primaryKey"`
	ReceiptVersion              int                          `json:"receipt_version" gorm:"not null;<-:create"`
	MutationKey                 string                       `json:"mutation_key" gorm:"type:varchar(128);not null;uniqueIndex:uidx_quota_mutation_receipt_key;<-:create"`
	RequestFingerprint          string                       `json:"request_fingerprint" gorm:"type:char(64);not null;<-:create"`
	OperationRequestFingerprint string                       `json:"operation_request_fingerprint" gorm:"type:char(64);not null;<-:create"`
	OperationID                 int64                        `json:"operation_id" gorm:"not null;uniqueIndex:uidx_quota_mutation_receipt_operation;<-:create"`
	OperationPublicID           string                       `json:"operation_public_id" gorm:"type:varchar(48);not null;<-:create"`
	ExpectedOperationVersion    int64                        `json:"expected_operation_version" gorm:"type:bigint;not null;<-:create"`
	OperationVersionBefore      int64                        `json:"operation_version_before" gorm:"type:bigint;not null;<-:create"`
	OperationVersionAfter       int64                        `json:"operation_version_after" gorm:"type:bigint;not null;<-:create"`
	BillingEventID              string                       `json:"billing_event_id" gorm:"type:varchar(64);not null;uniqueIndex:uidx_quota_mutation_receipt_event;<-:create"`
	BillingEventKey             string                       `json:"billing_event_key" gorm:"type:varchar(128);not null;<-:create"`
	BillingEventVersion         int64                        `json:"billing_event_version" gorm:"type:bigint;not null;<-:create"`
	UserID                      int                          `json:"user_id" gorm:"not null;index:idx_quota_mutation_receipt_lookup,priority:1;<-:create"`
	TokenID                     int                          `json:"token_id" gorm:"not null;index:idx_quota_mutation_receipt_lookup,priority:2;<-:create"`
	ChannelID                   int                          `json:"channel_id" gorm:"not null;<-:create"`
	BillingSource               string                       `json:"billing_source" gorm:"type:varchar(32);not null;<-:create"`
	SubscriptionID              int                          `json:"subscription_id,omitempty" gorm:"index;<-:create"`
	Quota                       int64                        `json:"quota" gorm:"type:bigint;not null;<-:create"`
	BillingContext              TaskQuotaBillingContext      `json:"billing_context" gorm:"type:text;not null;<-:create"`
	Before                      QuotaMutationAccountSnapshot `json:"before" gorm:"column:before_snapshot;type:text;not null;<-:create"`
	After                       QuotaMutationAccountSnapshot `json:"after" gorm:"column:after_snapshot;type:text;not null;<-:create"`
	CreatedAt                   int64                        `json:"created_at" gorm:"type:bigint;not null;index;<-:create"`
}

func (legacyQuotaMutationReceipt) TableName() string { return "quota_mutation_receipts" }

func createLegacyQuotaMutationReceiptFixture(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&legacyQuotaMutationReceipt{}))
	legacy := &legacyQuotaMutationReceipt{
		ID:                          1,
		ReceiptVersion:              1,
		MutationKey:                 "task:legacy_op_1:reserve:v1",
		RequestFingerprint:          "f000000000000000000000000000000000000000000000000000000000000001",
		OperationRequestFingerprint: "f000000000000000000000000000000000000000000000000000000000000001",
		OperationID:                 88001,
		OperationPublicID:           "top_legacy_1",
		ExpectedOperationVersion:    1,
		OperationVersionBefore:      1,
		OperationVersionAfter:       2,
		BillingEventID:              "evt_legacy_1",
		BillingEventKey:             "task:legacy_op_1:reserve:v1",
		BillingEventVersion:         1,
		UserID:                      100,
		TokenID:                     200,
		ChannelID:                   1,
		BillingSource:               "wallet",
		Quota:                       500,
		BillingContext:              TaskQuotaBillingContext{ModelRatio: 1},
		Before: QuotaMutationAccountSnapshot{
			User:  QuotaMutationUserSnapshot{ID: 100, Quota: 1000},
			Token: QuotaMutationTokenSnapshot{ID: 200, RemainQuota: 1000},
		},
		After: QuotaMutationAccountSnapshot{
			User:  QuotaMutationUserSnapshot{ID: 100, Quota: 500},
			Token: QuotaMutationTokenSnapshot{ID: 200, RemainQuota: 500},
		},
		CreatedAt: 1700000000,
	}
	require.NoError(t, db.Create(legacy).Error)
}

func assertQuotaMutationReceiptMigrated(t *testing.T, db *gorm.DB) {
	t.Helper()
	var r QuotaMutationReceipt
	require.NoError(t, db.First(&r, 1).Error)
	assert.Equal(t, "task:legacy_op_1:reserve:v1", r.MutationKey)
	assert.Equal(t, int64(88001), r.OperationID)
	assert.Equal(t, string(TaskBillingEventTypeReserve), r.MutationType)

	migrator := db.Migrator()
	assert.False(t, migrator.HasIndex(&QuotaMutationReceipt{}, "uidx_quota_mutation_receipt_operation"))
	assert.True(t, migrator.HasIndex(&QuotaMutationReceipt{}, "uidx_quota_mutation_receipt_op_type"))

	// Inserting another receipt for op 88001 with terminal_settlement must succeed (compound index allows it)
	rawInsertSettle := db.Exec(`
		INSERT INTO quota_mutation_receipts (
			receipt_version, mutation_type, mutation_key, request_fingerprint, operation_request_fingerprint,
			operation_id, operation_public_id, expected_operation_version, operation_version_before, operation_version_after,
			billing_event_id, billing_event_key, billing_event_version, user_id, token_id, channel_id, billing_source,
			quota, billing_context, before_snapshot, after_snapshot, created_at
		) VALUES (
			1, 'terminal_settlement', 'task:legacy_op_1:settle:v1', 'f000000000000000000000000000000000000000000000000000000000000002', 'f000000000000000000000000000000000000000000000000000000000000001',
			88001, 'task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 2, 2, 3,
			'billing_evt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'task:legacy_op_1:settle:v1', 1, 100, 200, 1, 'wallet',
			400, '{}', '{}', '{}', 1700000010
		)
	`)
	require.NoError(t, rawInsertSettle.Error)

	// Inserting duplicate settlement for op 88001 must fail due to compound unique index
	rawInsertDup := db.Exec(`
		INSERT INTO quota_mutation_receipts (
			receipt_version, mutation_type, mutation_key, request_fingerprint, operation_request_fingerprint,
			operation_id, operation_public_id, expected_operation_version, operation_version_before, operation_version_after,
			billing_event_id, billing_event_key, billing_event_version, user_id, token_id, channel_id, billing_source,
			quota, billing_context, before_snapshot, after_snapshot, created_at
		) VALUES (
			1, 'terminal_settlement', 'task:legacy_op_1:settle:dup', 'f000000000000000000000000000000000000000000000000000000000000003', 'f000000000000000000000000000000000000000000000000000000000000001',
			88001, 'task_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 2, 2, 3,
			'billing_evt_baaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 'task:legacy_op_1:settle:dup', 1, 100, 200, 1, 'wallet',
			400, '{}', '{}', '{}', 1700000020
		)
	`)
	require.Error(t, rawInsertDup.Error)
}

func TestTaskQuotaReservationLegacyMigrationSQLite(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	createTaskQuotaLegacyFixture(t, db)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &UserSubscription{}))
		assertTaskQuotaLegacyFixtureMigrated(t, db)
	}
}

func TestQuotaMutationReceiptLegacyMigrationSQLite(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	createLegacyQuotaMutationReceiptFixture(t, db)
	for range 2 {
		require.NoError(t, ensureQuotaMutationReceiptSchemaWithDB(db))
		require.NoError(t, db.AutoMigrate(&QuotaMutationReceipt{}))
	}
	assertQuotaMutationReceiptMigrated(t, db)
}

func TestTaskQuotaReservationLegacyMigrationConfiguredDatabases(t *testing.T) {
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
			createTaskQuotaLegacyFixture(t, db)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &UserSubscription{}))
				assertTaskQuotaLegacyFixtureMigrated(t, db)
			}
		})
	}
}

func TestQuotaMutationReceiptLegacyMigrationConfiguredDatabases(t *testing.T) {
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
			createLegacyQuotaMutationReceiptFixture(t, db)
			for range 2 {
				require.NoError(t, ensureQuotaMutationReceiptSchemaWithDB(db))
				require.NoError(t, db.AutoMigrate(&QuotaMutationReceipt{}))
			}
			assertQuotaMutationReceiptMigrated(t, db)
		})
	}
}

func assertUserQuotaMutationReceiptMigrated(t *testing.T, db *gorm.DB) {
	t.Helper()
	migrator := db.Migrator()
	assert.True(t, migrator.HasTable(&UserQuotaMutationReceipt{}))
	assert.True(t, migrator.HasIndex(&UserQuotaMutationReceipt{}, "uidx_user_quota_mutation_key"))
	assert.True(t, migrator.HasIndex(&UserQuotaMutationReceipt{}, "idx_user_quota_mutation_lookup"))

	// Insert receipt via authoritative create context
	tx := userQuotaMutationReceiptCreateDB(db)
	receipt := &UserQuotaMutationReceipt{
		ReceiptVersion:     UserQuotaMutationReceiptVersion,
		MutationType:       "test_migration",
		BusinessEventKey:   "migration:test:key:1",
		RequestFingerprint: "f000000000000000000000000000000000000000000000000000000000000001",
		UserID:             100,
		Delta:              500,
		QuotaBefore:        1000,
		QuotaAfter:         1500,
		QuotaVersionBefore: 0,
		QuotaVersionAfter:  1,
		ReasonCode:         "initial_migration",
		OperatorUserID:     0,
		Metadata:           "{}",
	}
	require.NoError(t, tx.Create(receipt).Error)
	assert.NotZero(t, receipt.ID)

	// Verify duplicate business_event_key is rejected by unique constraint
	dupReceipt := &UserQuotaMutationReceipt{
		ReceiptVersion:     UserQuotaMutationReceiptVersion,
		MutationType:       "test_migration",
		BusinessEventKey:   "migration:test:key:1",
		RequestFingerprint: "f000000000000000000000000000000000000000000000000000000000000002",
		UserID:             100,
		Delta:              200,
		QuotaBefore:        1500,
		QuotaAfter:         1700,
		QuotaVersionBefore: 1,
		QuotaVersionAfter:  2,
		ReasonCode:         "duplicate_key",
		OperatorUserID:     0,
		Metadata:           "{}",
	}
	dupTx := userQuotaMutationReceiptCreateDB(db)
	require.Error(t, dupTx.Create(dupReceipt).Error)
}

func TestUserQuotaMutationReceiptMigrationSQLite(t *testing.T) {
	db := openB2SubmissionSQLite(t)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&UserQuotaMutationReceipt{}))
	}
	assertUserQuotaMutationReceiptMigrated(t, db)
}

func TestUserQuotaMutationReceiptMigrationConfiguredDatabases(t *testing.T) {
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
			for range 2 {
				require.NoError(t, db.AutoMigrate(&UserQuotaMutationReceipt{}))
			}
			assertUserQuotaMutationReceiptMigrated(t, db)
		})
	}
}
