package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	channelRoutingEpochSingletonID = 1
	channelRoutingEpochSchema      = 1
)

var (
	ErrChannelRoutingEpochUnavailable = errors.New("channel routing epoch is unavailable")
	channelRoutingSchemaMu            sync.Mutex
	channelRoutingSchemaReady         sync.Map
)

type channelRoutingManagedTransactionKey struct{}

func markChannelRoutingManagedTransaction(tx *gorm.DB) *gorm.DB {
	if tx == nil {
		return tx
	}
	return tx.WithContext(context.WithValue(tx.Statement.Context, channelRoutingManagedTransactionKey{}, true))
}

func isChannelRoutingManagedTransaction(tx *gorm.DB) bool {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return false
	}
	managed, _ := tx.Statement.Context.Value(channelRoutingManagedTransactionKey{}).(bool)
	return managed
}

// ChannelRoutingEpoch is the persistent cluster authority for channel/ability
// routing mutations. Every transaction that changes routing data advances this
// singleton before committing.
type ChannelRoutingEpoch struct {
	ID            int   `json:"id" gorm:"primaryKey"`
	SchemaVersion int   `json:"schema_version" gorm:"not null"`
	Epoch         int64 `json:"epoch" gorm:"type:bigint;not null"`
	UpdatedAt     int64 `json:"updated_at" gorm:"type:bigint;not null"`
}

func (ChannelRoutingEpoch) TableName() string { return "channel_routing_epochs" }

func ensureChannelRoutingSchema(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if _, ok := channelRoutingSchemaReady.Load(db); ok {
		return nil
	}
	channelRoutingSchemaMu.Lock()
	defer channelRoutingSchemaMu.Unlock()
	if _, ok := channelRoutingSchemaReady.Load(db); ok {
		return nil
	}
	if err := db.AutoMigrate(&ChannelRoutingEpoch{}, &ChannelRoutingOperation{}); err != nil {
		return err
	}
	candidate := ChannelRoutingEpoch{
		ID:            channelRoutingEpochSingletonID,
		SchemaVersion: channelRoutingEpochSchema,
		Epoch:         1,
		UpdatedAt:     getDBTimestampTx(db),
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
		return err
	}
	if _, err := getChannelRoutingEpoch(db); err != nil {
		return err
	}
	channelRoutingSchemaReady.Store(db, struct{}{})
	return nil
}

func getChannelRoutingEpoch(db *gorm.DB) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}
	var state ChannelRoutingEpoch
	if err := db.Where("id = ?", channelRoutingEpochSingletonID).First(&state).Error; err != nil {
		return 0, fmt.Errorf("%w: %w", ErrChannelRoutingEpochUnavailable, err)
	}
	if state.SchemaVersion != channelRoutingEpochSchema || state.Epoch <= 0 {
		return 0, fmt.Errorf("%w: invalid persisted state", ErrChannelRoutingEpochUnavailable)
	}
	return state.Epoch, nil
}

func GetCommittedChannelRoutingEpoch(db *gorm.DB) (int64, error) {
	if err := ensureChannelRoutingSchema(db); err != nil {
		return 0, err
	}
	return getChannelRoutingEpoch(db)
}

func advanceChannelRoutingEpoch(tx *gorm.DB) (int64, error) {
	if tx == nil {
		return 0, gorm.ErrInvalidDB
	}
	var state ChannelRoutingEpoch
	if err := lockForUpdate(tx).Where("id = ?", channelRoutingEpochSingletonID).First(&state).Error; err != nil {
		return 0, fmt.Errorf("%w: %w", ErrChannelRoutingEpochUnavailable, err)
	}
	if state.SchemaVersion != channelRoutingEpochSchema || state.Epoch <= 0 || state.Epoch == math.MaxInt64 {
		return 0, fmt.Errorf("%w: invalid or exhausted persisted state", ErrChannelRoutingEpochUnavailable)
	}
	nextEpoch := state.Epoch + 1
	result := tx.Model(&ChannelRoutingEpoch{}).
		Where("id = ? AND epoch = ?", channelRoutingEpochSingletonID, state.Epoch).
		Updates(map[string]any{
			"epoch":      nextEpoch,
			"updated_at": getDBTimestampTx(tx),
		})
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, fmt.Errorf("%w: concurrent epoch update", ErrChannelRoutingEpochUnavailable)
	}
	return nextEpoch, nil
}

func observeCommittedChannelRoutingEpoch(epoch int64) {
	if epoch <= 0 {
		return
	}
	channelSyncLock.Lock()
	if channelCacheDB != DB {
		channelCacheDB = DB
		channelCacheObservedCommittedEpoch = 0
		channelCachePublishedEpoch = 0
	}
	if epoch > channelCacheObservedCommittedEpoch {
		channelCacheObservedCommittedEpoch = epoch
	}
	channelCacheDataGeneration = uint64(channelCacheObservedCommittedEpoch)
	channelSyncLock.Unlock()
}

func runChannelRoutingTransaction(ctx context.Context, fn func(tx *gorm.DB) (bool, error)) (epoch int64, changed bool, err error) {
	if DB == nil {
		return 0, false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ensureChannelRoutingSchema(DB); err != nil {
		return 0, false, err
	}
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tx = markChannelRoutingManagedTransaction(tx)
		var mutationErr error
		changed, mutationErr = fn(tx)
		if mutationErr != nil || !changed {
			return mutationErr
		}
		epoch, mutationErr = advanceChannelRoutingEpoch(tx)
		return mutationErr
	})
	if err != nil {
		return 0, false, err
	}
	observeCommittedChannelRoutingEpoch(epoch)
	return epoch, changed, nil
}

func isRetryableSQLiteWriteError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked")
}

func resetChannelRoutingSchemaStateForTest(db *gorm.DB) {
	channelRoutingSchemaReady.Delete(db)
}
