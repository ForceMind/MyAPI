package model

import (
	"context"
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
	"gorm.io/gorm/logger"
)

func TestUpstreamAcceptanceRunConfiguredDatabases(t *testing.T) {
	for _, engine := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "MYAPI_B2_MYSQL_DSN"}, {"postgres", "MYAPI_B2_POSTGRES_DSN"}} {
		t.Run(engine.name, func(t *testing.T) {
			var dialector gorm.Dialector = sqlite.Open(t.TempDir() + "/acceptance.db")
			if engine.env != "" {
				if os.Getenv("MYAPI_B2_DATABASE_TESTS") != "1" {
					t.Skip("requires explicitly configured disposable loopback B2 database")
				}
				var err error
				dialector, err = b2SubmissionDatabaseDialector(engine.name, strings.TrimSpace(os.Getenv(engine.env)))
				require.NoError(t, err)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			pool, err := db.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(2)
			previous := DB
			DB = db
			t.Cleanup(func() {
				DB = previous
				require.NoError(t, pool.Close())
			})
			require.NoError(t, db.AutoMigrate(&UpstreamAcceptanceRun{}))
			prefix := fmt.Sprintf("acceptance-%x-", time.Now().UnixNano())
			ctx := context.Background()
			newRun := func(t *testing.T, suffix string) *UpstreamAcceptanceRun {
				t.Helper()
				run := &UpstreamAcceptanceRun{
					ID: prefix + suffix, OwnerID: "process-a", ChannelID: 17, AccountRef: "opaque-account-a",
					CreatedAtUnix: 100, DeadlineAtUnix: 200, MaxTotal: 10,
					MaxAuth: 2, MaxRefresh: 2, MaxSample: 2, MaxModels: 2, MaxGeneration: 2,
				}
				t.Cleanup(func() { require.NoError(t, db.Where("id = ?", run.ID).Delete(&UpstreamAcceptanceRun{}).Error) })
				return run
			}

			t.Run("classification_and_attempt_budgets", func(t *testing.T) {
				run := newRun(t, "counts")
				run.MaxTotal = 6
				require.NoError(t, CreateUpstreamAcceptanceRun(ctx, run))
				for _, kind := range []string{UpstreamAcceptanceDispatchAuth, UpstreamAcceptanceDispatchRefresh, UpstreamAcceptanceDispatchSample, UpstreamAcceptanceDispatchModels, UpstreamAcceptanceDispatchGeneration} {
					_, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, kind, 101)
					require.NoError(t, err)
				}
				// A failed sample still consumed its attempt; retry consumes another.
				got, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, UpstreamAcceptanceDispatchSample, 102)
				require.NoError(t, err)
				assert.Equal(t, int64(6), got.Total)
				assert.Equal(t, int64(1), got.Auth)
				assert.Equal(t, int64(1), got.Refresh)
				assert.Equal(t, int64(2), got.Sample)
				assert.Equal(t, int64(1), got.Models)
				assert.Equal(t, int64(1), got.Generation)
				assert.Equal(t, int64(102), got.LastDispatchAtUnix)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, UpstreamAcceptanceDispatchAuth, 103)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				stored, err := GetUpstreamAcceptanceRun(ctx, run.ID)
				require.NoError(t, err)
				assert.Equal(t, got, stored)
			})

			t.Run("scope_and_per_kind_limits", func(t *testing.T) {
				run := newRun(t, "scope")
				run.MaxAuth = 0
				run.MaxSample = 1
				require.NoError(t, CreateUpstreamAcceptanceRun(ctx, run))
				for _, attempt := range []struct {
					name, owner, account, kind string
					channel                    int
					want                       error
				}{
					{"restart_owner", "process-b", run.AccountRef, "sample", 17, ErrUpstreamAcceptanceDenied},
					{"other_channel", run.OwnerID, run.AccountRef, "sample", 18, ErrUpstreamAcceptanceDenied},
					{"other_account", run.OwnerID, "opaque-account-b", "sample", 17, ErrUpstreamAcceptanceDenied},
					{"disabled_kind", run.OwnerID, run.AccountRef, "auth", 17, ErrUpstreamAcceptanceDenied},
					{"unknown_kind", run.OwnerID, run.AccountRef, "other", 17, ErrUpstreamAcceptanceInvalid},
				} {
					t.Run(attempt.name, func(t *testing.T) {
						_, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, attempt.owner, attempt.channel, attempt.account, attempt.kind, 101)
						assert.ErrorIs(t, err, attempt.want)
					})
				}
				stored, err := GetUpstreamAcceptanceRun(ctx, run.ID)
				require.NoError(t, err)
				assert.Zero(t, stored.Total)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 101)
				require.NoError(t, err)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 102)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				stored, err = GetUpstreamAcceptanceRun(ctx, run.ID)
				require.NoError(t, err)
				assert.Equal(t, int64(1), stored.Total)
				assert.Equal(t, int64(1), stored.Sample)
			})

			t.Run("generation_slot_stop_and_drain", func(t *testing.T) {
				run := newRun(t, "stop")
				require.NoError(t, CreateUpstreamAcceptanceRun(ctx, run))
				_, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "generation", 101)
				require.NoError(t, err)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "generation", 102)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				_, err = StopUpstreamAcceptanceRun(ctx, run.ID, "process-b", 102, "manual")
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				stopping, err := StopUpstreamAcceptanceRun(ctx, run.ID, run.OwnerID, 103, "manual")
				require.NoError(t, err)
				assert.Equal(t, UpstreamAcceptanceRunStopping, stopping.Status)
				assert.Equal(t, int64(1), stopping.GenerationInFlight)
				repeated, err := StopUpstreamAcceptanceRun(ctx, run.ID, run.OwnerID, 104, "shutdown")
				require.NoError(t, err)
				assert.Equal(t, stopping, repeated)
				_, err = CompleteUpstreamAcceptanceStop(ctx, run.ID, run.OwnerID, 104)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				for _, kind := range []string{"auth", "refresh", "sample", "models", "generation"} {
					_, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, kind, 104)
					assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				}
				assert.ErrorIs(t, FinishUpstreamAcceptanceGeneration(ctx, run.ID, "process-b", 105), ErrUpstreamAcceptanceDenied)
				require.NoError(t, FinishUpstreamAcceptanceGeneration(ctx, run.ID, run.OwnerID, 105))
				assert.ErrorIs(t, FinishUpstreamAcceptanceGeneration(ctx, run.ID, run.OwnerID, 105), ErrUpstreamAcceptanceDenied)
				stopped, err := CompleteUpstreamAcceptanceStop(ctx, run.ID, run.OwnerID, 106)
				require.NoError(t, err)
				assert.Equal(t, UpstreamAcceptanceRunStopped, stopped.Status)
				assert.Equal(t, int64(1), stopped.Total)
				assert.Equal(t, int64(1), stopped.Generation)
				assert.Zero(t, stopped.GenerationInFlight)
				again, err := StopUpstreamAcceptanceRun(ctx, run.ID, run.OwnerID, 107, "shutdown")
				require.NoError(t, err)
				assert.Equal(t, stopped, again)
				again, err = CompleteUpstreamAcceptanceStop(ctx, run.ID, run.OwnerID, 108)
				require.NoError(t, err)
				assert.Equal(t, stopped, again)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "generation", 109)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
			})

			t.Run("generation_release_preserves_attempt_count", func(t *testing.T) {
				run := newRun(t, "release")
				require.NoError(t, CreateUpstreamAcceptanceRun(ctx, run))
				_, err := CompleteUpstreamAcceptanceStop(ctx, run.ID, run.OwnerID, 101)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "generation", 101)
				require.NoError(t, err)
				require.NoError(t, FinishUpstreamAcceptanceGeneration(ctx, run.ID, run.OwnerID, 102))
				second, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "generation", 103)
				require.NoError(t, err)
				assert.Equal(t, int64(2), second.Total)
				assert.Equal(t, int64(2), second.Generation)
				assert.Equal(t, int64(1), second.GenerationInFlight)
				require.NoError(t, FinishUpstreamAcceptanceGeneration(ctx, run.ID, run.OwnerID, 104))
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "generation", 105)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
			})

			t.Run("deadline_is_terminal_and_persisted", func(t *testing.T) {
				run := newRun(t, "expiry")
				require.NoError(t, CreateUpstreamAcceptanceRun(ctx, run))
				_, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 199)
				require.NoError(t, err)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 200)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				// Reopening and rerunning migration do not reset state or counters.
				reopened, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
				require.NoError(t, err)
				reopenedPool, err := reopened.DB()
				require.NoError(t, err)
				DB = reopened
				t.Cleanup(func() { DB = db; require.NoError(t, reopenedPool.Close()) })
				require.NoError(t, reopened.AutoMigrate(&UpstreamAcceptanceRun{}))
				stored, err := GetUpstreamAcceptanceRun(ctx, run.ID)
				require.NoError(t, err)
				assert.Equal(t, UpstreamAcceptanceRunExpired, stored.Status)
				assert.Equal(t, "deadline", stored.StopReason)
				assert.Equal(t, int64(1), stored.Total)
				assert.Equal(t, int64(1), stored.Sample)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 199)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, "process-b", run.ChannelID, run.AccountRef, "sample", 201)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
			})

			t.Run("concurrent_last_budget_claim", func(t *testing.T) {
				run := newRun(t, "concurrent")
				run.MaxTotal = 1
				run.MaxAuth, run.MaxRefresh, run.MaxSample, run.MaxModels, run.MaxGeneration = 0, 0, 1, 0, 0
				require.NoError(t, CreateUpstreamAcceptanceRun(ctx, run))
				start := make(chan struct{})
				results := make(chan error, 2)
				for contender := 0; contender < 2; contender++ {
					go func() {
						<-start
						_, err := ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 101)
						results <- err
					}()
				}
				close(start)
				first, second := <-results, <-results
				if first != nil {
					first, second = second, first
				}
				require.NoError(t, first)
				// SQLite may reject the competing writer as busy; both failures
				// deny dispatch. No retry can reclaim the exhausted budget.
				require.Error(t, second)
				assert.Contains(t, []error{ErrUpstreamAcceptanceDenied, ErrUpstreamAcceptanceStorage}, second)
				stored, err := GetUpstreamAcceptanceRun(ctx, run.ID)
				require.NoError(t, err)
				assert.Equal(t, int64(1), stored.Total)
				assert.Equal(t, int64(1), stored.Sample)
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 102)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
			})

			t.Run("validation_and_fail_closed_storage", func(t *testing.T) {
				for _, invalid := range []struct {
					name   string
					change func(*UpstreamAcceptanceRun)
				}{
					{"negative_limit", func(r *UpstreamAcceptanceRun) { r.MaxAuth = -1 }},
					{"unbounded_total", func(r *UpstreamAcceptanceRun) { r.MaxTotal = UpstreamAcceptanceMaxCounter + 1 }},
					{"class_above_total", func(r *UpstreamAcceptanceRun) { r.MaxModels = r.MaxTotal + 1 }},
					{"existing_counter", func(r *UpstreamAcceptanceRun) { r.Total = 1 }},
					{"deadline", func(r *UpstreamAcceptanceRun) { r.DeadlineAtUnix = r.CreatedAtUnix }},
					{"raw_account", func(r *UpstreamAcceptanceRun) { r.AccountRef = "synthetic@example.invalid" }},
				} {
					t.Run(invalid.name, func(t *testing.T) {
						run := newRun(t, invalid.name)
						invalid.change(run)
						assert.ErrorIs(t, CreateUpstreamAcceptanceRun(ctx, run), ErrUpstreamAcceptanceInvalid)
						_, err := GetUpstreamAcceptanceRun(ctx, run.ID)
						assert.ErrorIs(t, err, ErrUpstreamAcceptanceNotFound)
					})
				}
				run := newRun(t, "storage")
				require.NoError(t, CreateUpstreamAcceptanceRun(ctx, run))
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				_, err := ClaimUpstreamAcceptanceDispatch(cancelled, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 101)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceStorage)
				DB = nil
				_, err = ClaimUpstreamAcceptanceDispatch(ctx, run.ID, run.OwnerID, run.ChannelID, run.AccountRef, "sample", 101)
				assert.ErrorIs(t, err, ErrUpstreamAcceptanceStorage)
				DB = db
				stored, err := GetUpstreamAcceptanceRun(ctx, run.ID)
				require.NoError(t, err)
				assert.Zero(t, stored.Total)
				encoded, err := common.Marshal(stored)
				require.NoError(t, err)
				assert.NotContains(t, string(encoded), run.OwnerID)
				assert.NotContains(t, string(encoded), "owner_id")
			})
		})
	}
}
