package model

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestQuotaTaskStateWritesRespectContextWithExhaustedConnectionPool(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&SystemTask{}, &SystemTaskLock{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	heldConnection, err := sqlDB.Conn(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = heldConnection.Close(); _ = sqlDB.Close() })
	for _, test := range []struct {
		name  string
		write func(context.Context) error
	}{
		{"progress", func(ctx context.Context) error {
			return UpdateSystemTaskStateWithContext(ctx, "fixture-task", "fixture-runner", map[string]int{"progress": 0})
		}},
		{"renew", func(ctx context.Context) error {
			return RenewSystemTaskLockWithContext(ctx, "fixture-task", "fixture-runner", time.Now().Add(time.Minute).Unix())
		}},
		{"finish", func(ctx context.Context) error {
			return FinishSystemTaskWithContext(ctx, "fixture-task", "fixture-runner", SystemTaskStatusSucceeded, nil, "")
		}},
		{"release", func(ctx context.Context) error {
			return ReleaseSystemTaskLockWithContext(ctx, "fixture-task", "fixture-runner")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			require.ErrorIs(t, test.write(ctx), context.DeadlineExceeded)
		})
	}
}
