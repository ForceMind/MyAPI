package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOpenAIPriceCheckConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/price-check.db")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable B2 database")
				}
				dsn := strings.TrimSpace(os.Getenv(engine.env))
				require.NotEmpty(t, dsn)
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, dsn)
				require.NoError(t, err)
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			oldDB := DB
			DB = db
			t.Cleanup(func() { DB = oldDB; require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&SystemTask{}, &SystemTaskLock{}, &OfficialPriceVersion{}))
			require.NoError(t, db.AutoMigrate(&SystemTask{}, &SystemTaskLock{}, &OfficialPriceVersion{}))
			var active int64
			require.NoError(t, db.Model(&SystemTask{}).Where("type = ? AND status IN ?", SystemTaskTypeOpenAIPriceCheck, activeSystemTaskStatuses()).Count(&active).Error)
			require.Zero(t, active, "refuse to interfere with active price checks")
			namespace := fmt.Sprintf("price-check-fixture:%s:%d", t.Name(), time.Now().UnixNano())
			runner := namespace
			newTask := func(t *testing.T) *SystemTask {
				t.Helper()
				task, err := CreateSystemTask(SystemTaskTypeOpenAIPriceCheck, nil, nil)
				require.NoError(t, err)
				t.Cleanup(func() {
					require.NoError(t, db.Where("task_id = ?", task.TaskID).Delete(&SystemTaskLock{}).Error)
					require.NoError(t, db.Where("task_id = ?", task.TaskID).Delete(&SystemTask{}).Error)
				})
				claimed, ok, err := ClaimSystemTask(task.ID, task.Type, runner, common.GetTimestamp()+60)
				require.NoError(t, err)
				require.True(t, ok)
				return claimed
			}
			ctx := context.Background()
			first := newTask(t)
			var digest string
			save := func(tx *gorm.DB) (*OpenAIPriceCheckResult, error) {
				version, err := StoreOfficialPriceVersion(ctx, tx, namespace, 100)
				if err != nil {
					return nil, err
				}
				digest = version.ContentSHA256
				return &OpenAIPriceCheckResult{CheckedAt: common.GetTimestamp(), SourceSHA256: digest, SourceFetchedAt: version.FetchedAt}, nil
			}
			require.NoError(t, CompleteOpenAIPriceCheck(ctx, first, runner, save, ""))
			t.Cleanup(func() {
				require.NoError(t, db.Where("content_sha256 = ?", digest).Delete(&OfficialPriceVersion{}).Error)
			})
			// The same immutable content stays at its original fetch time, but
			// every successful observation gets its own successful task receipt.
			second := newTask(t)
			require.NoError(t, CompleteOpenAIPriceCheck(ctx, second, runner, save, ""))
			latest, successful, err := ReadOpenAIPriceCheckTasks(ctx)
			require.NoError(t, err)
			assert.Equal(t, second.TaskID, latest.TaskID)
			assert.Equal(t, second.TaskID, successful.TaskID)
			var result OpenAIPriceCheckResult
			require.NoError(t, common.UnmarshalJsonStr(successful.Result, &result))
			assert.EqualValues(t, 100, result.SourceFetchedAt)
			assert.Greater(t, result.CheckedAt, result.SourceFetchedAt)
			failed := newTask(t)
			require.NoError(t, CompleteOpenAIPriceCheck(ctx, failed, runner, nil, "source_fetch_failed"))
			latest, successful, err = ReadOpenAIPriceCheckTasks(ctx)
			require.NoError(t, err)
			assert.Equal(t, failed.TaskID, latest.TaskID)
			assert.Equal(t, second.TaskID, successful.TaskID)
			for _, failure := range []string{"lost-fence", "expired", "save-error", "late-cancel", "late-lease-loss"} {
				t.Run(failure, func(t *testing.T) {
					task := newTask(t)
					defer func() {
						require.NoError(t, db.Where("task_id = ?", task.TaskID).Delete(&SystemTaskLock{}).Error)
						require.NoError(t, db.Where("task_id = ?", task.TaskID).Delete(&SystemTask{}).Error)
					}()
					if failure == "lost-fence" {
						task.FenceToken++
					}
					if failure == "expired" {
						require.NoError(t, db.Model(&SystemTaskLock{}).Where("task_id = ?", task.TaskID).Update("locked_until", common.GetTimestamp()-1).Error)
					}
					writeCtx, cancel := context.WithCancel(ctx)
					defer cancel()
					var rejectedDigest string
					err := CompleteOpenAIPriceCheck(writeCtx, task, runner, func(tx *gorm.DB) (*OpenAIPriceCheckResult, error) {
						version, err := StoreOfficialPriceVersion(writeCtx, tx, namespace+failure, 200)
						if err != nil {
							return nil, err
						}
						rejectedDigest = version.ContentSHA256
						if failure == "save-error" {
							return nil, errors.New("injected save failure")
						}
						if failure == "late-cancel" {
							cancel()
						}
						if failure == "late-lease-loss" {
							if err := tx.Model(&SystemTaskLock{}).Where("task_id = ?", task.TaskID).Update("fence_token", task.FenceToken+1).Error; err != nil {
								return nil, err
							}
						}
						return &OpenAIPriceCheckResult{CheckedAt: common.GetTimestamp(), SourceSHA256: version.ContentSHA256, SourceFetchedAt: version.FetchedAt}, nil
					}, "")
					require.Error(t, err)
					if rejectedDigest != "" {
						var count int64
						require.NoError(t, db.Model(&OfficialPriceVersion{}).Where("content_sha256 = ?", rejectedDigest).Count(&count).Error)
						assert.Zero(t, count, "failed transaction must not leave new evidence")
					}
					_, success, err := ReadOpenAIPriceCheckTasks(ctx)
					require.NoError(t, err)
					assert.Equal(t, second.TaskID, success.TaskID)
				})
			}
		})
	}
}

func TestOpenAIPriceCheckOptionRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"true", "false"} {
		require.NoError(t, validateOptionValue(OpenAIOfficialPriceCheckEnabledOptionKey, value))
	}
	for _, value := range []string{"", "1", "yes", "TRUE", "24h", " true "} {
		require.Error(t, validateOptionValue(OpenAIOfficialPriceCheckEnabledOptionKey, value))
	}
}
