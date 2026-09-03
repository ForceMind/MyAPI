package model

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const orderingProfileKey = "access_profile_setting.profiles"

// The observation is at lock entry, not acquisition. It lets tests release
// the first operation once its contender has either queued (fixed behavior)
// or completed out of order (the regression), without timing assumptions.
type observedOptionLocker struct {
	mutex   sync.Mutex
	entries atomic.Int32
	queued  chan struct{}
	workers sync.WaitGroup
}

func (lock *observedOptionLocker) Lock() {
	if lock.entries.Add(1) == 2 {
		close(lock.queued)
	}
	lock.mutex.Lock()
}

func (lock *observedOptionLocker) Unlock() { lock.mutex.Unlock() }

func (lock *observedOptionLocker) start(operation func() error) <-chan error {
	result := make(chan error, 1)
	lock.workers.Add(1)
	go func() {
		defer lock.workers.Done()
		result <- operation()
	}()
	return result
}

type optionCommitBarrierPool struct {
	gorm.ConnPool
	once      atomic.Bool
	committed chan struct{}
	release   chan struct{}
}

func (pool *optionCommitBarrierPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := pool.ConnPool.(gorm.TxBeginner).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &optionCommitBarrierTx{Tx: tx, pool: pool}, nil
}

type optionCommitBarrierTx struct {
	*sql.Tx
	pool *optionCommitBarrierPool
}

func (tx *optionCommitBarrierTx) Commit() error {
	err := tx.Tx.Commit()
	if err == nil && tx.pool.once.CompareAndSwap(false, true) {
		close(tx.pool.committed)
		<-tx.pool.release
	}
	return err
}

func optionOrderingFixture(t *testing.T) (*gorm.DB, *observedOptionLocker) {
	t.Helper()
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousProfiles, err := common.Marshal(setting.GetAccessProfileSetting().Profiles)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	previousLock := optionMutationLock
	lock := &observedOptionLocker{queued: make(chan struct{})}
	optionMutationLock = lock
	t.Cleanup(func() {
		// Test-specific cleanups first release barriers, even on fatal assertions.
		// Joining is independent of whether result channels were already consumed.
		lock.workers.Wait()
		optionMutationLock = previousLock
		require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(string(previousProfiles)))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	initial, err := setting.NormalizeAccessProfileDefinitionsJSON(`{"standard":{"label":"Initial"}}`)
	require.NoError(t, err)
	require.NoError(t, db.Create(&Option{Key: orderingProfileKey, Value: initial}).Error)
	require.NoError(t, updateOptionMap(orderingProfileKey, initial))
	return db, lock
}

// Deadlines only turn unexpected deadlocks into failures; no assertion uses
// elapsed time or the absence of an event within a time interval.
func awaitOptionOperation(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("option operation did not complete")
		return nil
	}
}

func awaitOptionBarrier(t *testing.T, reached <-chan struct{}) {
	t.Helper()
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("option operation did not reach its barrier")
	}
}

func finishOrderedOptionOperations(t *testing.T, lock *observedOptionLocker, release func(), first, second <-chan error) {
	t.Helper()
	select {
	case <-lock.queued:
	case err := <-second:
		assert.NoError(t, err)
		second = nil
	case <-time.After(10 * time.Second):
		t.Fatal("contender neither queued nor completed")
	}
	release()
	require.NoError(t, awaitOptionOperation(t, first))
	if second != nil {
		require.NoError(t, awaitOptionOperation(t, second))
	}
}

func assertPublishedProfile(t *testing.T, db *gorm.DB, label string) {
	t.Helper()
	var stored Option
	require.NoError(t, db.First(&stored, Option{Key: orderingProfileKey}).Error)
	common.OptionMapRWMutex.RLock()
	published := common.OptionMap[orderingProfileKey]
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, stored.Value, published, "published option must match the last committed value")
	profile, ok := setting.GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, label, profile.Label)
	assert.Equal(t, stored.Value, config.GlobalConfig.ExportAllConfigs()[orderingProfileKey])
}

func TestOptionWritesPublishInCommitOrder(t *testing.T) {
	for _, firstBulk := range []bool{false, true} {
		name := "single-then-bulk"
		if firstBulk {
			name = "bulk-then-single"
		}
		t.Run(name, func(t *testing.T) {
			db, lock := optionOrderingFixture(t)
			pool := &optionCommitBarrierPool{ConnPool: db.ConnPool, committed: make(chan struct{}), release: make(chan struct{})}
			release := sync.OnceFunc(func() { close(pool.release) })
			t.Cleanup(release)
			db.Config.ConnPool, db.Statement.ConnPool = pool, pool
			first := lock.start(func() error {
				if firstBulk {
					return UpdateOptionsBulk(map[string]string{orderingProfileKey: `{"standard":{"label":"First"}}`, "ordering_companion": "First"})
				}
				return UpdateOption(orderingProfileKey, `{"standard":{"label":"First"}}`)
			})
			awaitOptionBarrier(t, pool.committed)
			// Ordinary option readers must not be held behind database I/O.
			reader := lock.start(func() error {
				common.OptionMapRWMutex.RLock()
				defer common.OptionMapRWMutex.RUnlock()
				var profiles map[string]setting.AccessProfileDefinition
				err := common.UnmarshalJsonStr(common.OptionMap[orderingProfileKey], &profiles)
				if err == nil && profiles["standard"].Label != "Initial" {
					err = errors.New("profile published before the commit boundary returned")
				}
				return err
			})
			require.NoError(t, awaitOptionOperation(t, reader))
			second := lock.start(func() error {
				if firstBulk {
					return UpdateOption(orderingProfileKey, `{"standard":{"label":"Second"}}`)
				}
				return UpdateOptionsBulk(map[string]string{orderingProfileKey: `{"standard":{"label":"Second"}}`, "ordering_companion": "Second"})
			})
			finishOrderedOptionOperations(t, lock, release, first, second)
			assertPublishedProfile(t, db, "Second")
		})
	}
}

func TestOptionReloadCannotOverwriteNewerWrite(t *testing.T) {
	for _, reload := range []struct {
		name string
		run  func()
	}{{"reload", loadOptionsFromDatabase}, {"initialization", InitOptionMap}} {
		t.Run(reload.name, func(t *testing.T) {
			db, lock := optionOrderingFixture(t)
			snapshot := make(chan struct{})
			release := make(chan struct{})
			resume := sync.OnceFunc(func() { close(release) })
			t.Cleanup(resume)
			var once atomic.Bool
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:option-reload-snapshot", func(tx *gorm.DB) {
				if _, allOptions := tx.Statement.Dest.(*[]*Option); allOptions && once.CompareAndSwap(false, true) {
					close(snapshot)
					<-release
				}
			}))
			first := lock.start(func() error { reload.run(); return nil })
			awaitOptionBarrier(t, snapshot)
			second := lock.start(func() error { return UpdateOption(orderingProfileKey, `{"standard":{"label":"Newer"}}`) })
			finishOrderedOptionOperations(t, lock, resume, first, second)
			assertPublishedProfile(t, db, "Newer")
		})
	}
}

func TestOptionSaveFailureDoesNotPublishOrHoldSequence(t *testing.T) {
	db, lock := optionOrderingFixture(t)
	writeErr := errors.New("injected ordering save failure")
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:ordering-save-failure", func(tx *gorm.DB) { tx.AddError(writeErr) }))
	assert.ErrorIs(t, UpdateOptionsBulk(map[string]string{orderingProfileKey: `{"standard":{"label":"Rejected"}}`, "ordering_companion": "Rejected"}), writeErr)
	assertPublishedProfile(t, db, "Initial")
	var companionCount int64
	require.NoError(t, db.Model(&Option{}).Where(commonKeyCol+" = ?", "ordering_companion").Count(&companionCount).Error)
	assert.Zero(t, companionCount, "failed bulk transaction must not leave a partially created option")
	require.NoError(t, db.Callback().Update().Remove("test:ordering-save-failure"))
	next := lock.start(func() error { return UpdateOption(orderingProfileKey, `{"standard":{"label":"Recovered"}}`) })
	require.NoError(t, awaitOptionOperation(t, next))
	assertPublishedProfile(t, db, "Recovered")
}

func TestOptionReloadFailureDoesNotPublishOrHoldSequence(t *testing.T) {
	db, lock := optionOrderingFixture(t)
	require.NoError(t, db.Model(&Option{}).Where(commonKeyCol+" = ?", orderingProfileKey).Update("value", `{"standard":{"label":"External"}}`).Error)
	readErr := errors.New("injected ordering read failure")
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:ordering-read-failure", func(tx *gorm.DB) {
		if _, allOptions := tx.Statement.Dest.(*[]*Option); allOptions {
			tx.AddError(readErr) // A failed query may still have returned a partial snapshot.
		}
	}))
	loadOptionsFromDatabase()
	profile, ok := setting.GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, "Initial", profile.Label)
	common.OptionMapRWMutex.RLock()
	var published map[string]setting.AccessProfileDefinition
	err := common.UnmarshalJsonStr(common.OptionMap[orderingProfileKey], &published)
	common.OptionMapRWMutex.RUnlock()
	require.NoError(t, err)
	assert.Equal(t, "Initial", published["standard"].Label)
	require.NoError(t, db.Callback().Query().Remove("test:ordering-read-failure"))
	next := lock.start(func() error { return UpdateOption(orderingProfileKey, `{"standard":{"label":"Recovered"}}`) })
	require.NoError(t, awaitOptionOperation(t, next))
	assertPublishedProfile(t, db, "Recovered")
}
