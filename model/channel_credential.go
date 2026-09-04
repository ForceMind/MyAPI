package model

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const channelCredentialGateTimeout = 5 * time.Second

type channelCredentialGate struct {
	token chan struct{}
	refs  int
}

type channelCredentialGateLease struct {
	channelID int
}

type channelCredentialGateContextKey struct{}

var channelCredentialGates = struct {
	sync.Mutex
	items map[int]*channelCredentialGate
}{items: make(map[int]*channelCredentialGate)}

// InsertChannelWithAbilities creates a single channel and its routing abilities
// in one transaction. Validation remains the controller's responsibility.
func InsertChannelWithAbilities(ctx context.Context, channel *Channel) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	if channel == nil {
		return errors.New("channel is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	err := DB.WithContext(ctx).
		Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).
		Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(channel).Error; err != nil {
				return sanitizeDBError(err)
			}
			return sanitizeDBError(channel.AddAbilities(tx))
		})
	return sanitizeDBError(err)
}

// WithChannelCredentialUpdateGate serializes all in-process credential work
// for one channel. Waiting is canceled with ctx; the callback receives a
// marked context so nested UpdateChannelCredentialIfUnchanged calls reuse the
// same lease instead of deadlocking.
func WithChannelCredentialUpdateGate(ctx context.Context, channelID int, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if fn == nil {
		return errors.New("channel credential update callback is required")
	}
	if lease, ok := ctx.Value(channelCredentialGateContextKey{}).(channelCredentialGateLease); ok && lease.channelID == channelID {
		return fn(ctx)
	}

	gate := retainChannelCredentialGate(channelID)
	if err := ctx.Err(); err != nil {
		releaseChannelCredentialGateReference(channelID, gate)
		return err
	}
	select {
	case gate.token <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate.token
			releaseChannelCredentialGateReference(channelID, gate)
			return err
		}
	case <-ctx.Done():
		releaseChannelCredentialGateReference(channelID, gate)
		return ctx.Err()
	}
	defer func() {
		<-gate.token
		releaseChannelCredentialGateReference(channelID, gate)
	}()

	gateCtx := context.WithValue(ctx, channelCredentialGateContextKey{}, channelCredentialGateLease{channelID: channelID})
	return fn(gateCtx)
}

func retainChannelCredentialGate(channelID int) *channelCredentialGate {
	channelCredentialGates.Lock()
	defer channelCredentialGates.Unlock()
	gate := channelCredentialGates.items[channelID]
	if gate == nil {
		gate = &channelCredentialGate{token: make(chan struct{}, 1)}
		channelCredentialGates.items[channelID] = gate
	}
	gate.refs++
	return gate
}

func releaseChannelCredentialGateReference(channelID int, gate *channelCredentialGate) {
	channelCredentialGates.Lock()
	defer channelCredentialGates.Unlock()
	gate.refs--
	if gate.refs == 0 && channelCredentialGates.items[channelID] == gate {
		delete(channelCredentialGates.items, channelID)
	}
}

// UpdateChannelCredentialIfUnchanged atomically replaces a channel credential
// only if it still matches the caller's snapshot. All in-process writers use
// the per-channel gate, while the database predicate remains the authority for
// writers in other processes. A successful write synchronously publishes the
// new key and advances the cache generation before releasing the gate.
func UpdateChannelCredentialIfUnchanged(ctx context.Context, channelID int, channelType int, expectedKey string, newKey string) (bool, error) {
	if DB == nil {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if lease, ok := ctx.Value(channelCredentialGateContextKey{}).(channelCredentialGateLease); ok && lease.channelID == channelID {
		return updateChannelCredentialIfUnchanged(ctx, channelID, channelType, expectedKey, newKey)
	}

	gateCtx, cancel := context.WithTimeout(ctx, channelCredentialGateTimeout)
	defer cancel()
	var updated bool
	err := WithChannelCredentialUpdateGate(gateCtx, channelID, func(lockedCtx context.Context) error {
		var err error
		updated, err = updateChannelCredentialIfUnchanged(lockedCtx, channelID, channelType, expectedKey, newKey)
		return err
	})
	return updated, err
}

func updateChannelCredentialIfUnchanged(ctx context.Context, channelID int, channelType int, expectedKey string, newKey string) (bool, error) {
	result := DB.WithContext(ctx).
		Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).
		Model(&Channel{}).
		Where(map[string]any{"id": channelID, "type": channelType, "key": expectedKey}).
		Update("key", newKey)
	if result.Error != nil {
		return false, sanitizeDBError(result.Error)
	}
	updated := result.RowsAffected == 1
	if !updated {
		// MySQL commonly reports zero affected rows for an idempotent same-value
		// UPDATE. Confirm the current value before treating that as success. This
		// also preserves a real CAS loss when another writer changed K0 to K1.
		var currentKey string
		err := DB.WithContext(ctx).
			Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).
			Model(&Channel{}).
			Select("key").
			Where(map[string]any{"id": channelID, "type": channelType}).
			Take(&currentKey).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		if err != nil {
			return false, sanitizeDBError(err)
		}
		if currentKey != newKey {
			return false, nil
		}
	}

	if common.MemoryCacheEnabled {
		channelSyncLock.Lock()
		if channel, ok := channelsIDM[channelID]; ok {
			cached := *channel
			cached.Key = newKey
			cached.Keys = nil
			channelsIDM[channelID] = &cached
		}
		channelCredentialCacheGeneration++
		channelSyncLock.Unlock()
	}
	return true, nil
}
