package model

import (
	"context"
	"errors"
	"strings"
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

var ErrChannelCredentialSubjectUnknown = errors.New("channel credential subject is unknown")
var ErrChannelCredentialSubjectConflict = errors.New("channel credential subject conflicts with existing evidence")

// LookupTrustedChannelCredentialSubject returns the provider-confirmed account
// subject previously linked to one exact credential by a successful native
// quota observation. The observation is identity evidence, so its age and a
// later quota-sampling failure do not erase the confirmed account binding.
// channelID > 0 restricts the proof to that channel; channelID 0 accepts an
// active, unambiguous proof from any channel.
func LookupTrustedChannelCredentialSubject(ctx context.Context, channelID int, credentialSubjectRef string) (string, error) {
	if DB == nil || !canonicalChannelQuotaIdentitySubject(strings.TrimSpace(credentialSubjectRef)) || channelID < 0 {
		return "", ErrChannelCredentialSubjectUnknown
	}
	if ctx == nil {
		ctx = context.Background()
	}
	trustedSubject := ""
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := lockForUpdate(tx).Where(
			"subject_ref = ? AND identity_quality = ? AND active = ?",
			credentialSubjectRef, ChannelQuotaIdentityQualityCredentialScoped, true,
		)
		if channelID > 0 {
			query = query.Where("channel_id = ?", channelID)
		}
		var targets []ChannelQuotaSamplingTarget
		if err := query.Order("channel_id ASC, id ASC").Find(&targets).Error; err != nil {
			return err
		}
		if len(targets) == 0 {
			return ErrChannelCredentialSubjectUnknown
		}
		for _, target := range targets {
			if !canonicalChannelQuotaIdentitySubject(target.ConfirmedSubjectRef) {
				return ErrChannelCredentialSubjectUnknown
			}
			if trustedSubject != "" && trustedSubject != target.ConfirmedSubjectRef {
				return ErrChannelCredentialSubjectUnknown
			}
			var count int64
			if err := tx.Model(&ChannelQuotaSnapshot{}).
				Where("channel_id = ? AND subject_ref = ? AND identity_quality = ?", target.ChannelID, target.ConfirmedSubjectRef, ChannelQuotaIdentityQualityProviderConfirmed).
				Where("codex_observation_qualified = ? AND status = ?", true, "success").
				Where("metric_type = ? AND source IN ?", "codex_rate_limit", []string{codexQuotaWindowPrimarySource, codexQuotaWindowSecondarySource}).
				Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				return ErrChannelCredentialSubjectUnknown
			}
			trustedSubject = target.ConfirmedSubjectRef
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return trustedSubject, nil
}

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
	_, _, err := runChannelRoutingTransaction(ctx, func(tx *gorm.DB) (bool, error) {
		tx = tx.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
		if err := tx.Create(channel).Error; err != nil {
			return false, sanitizeDBError(err)
		}
		return true, sanitizeDBError(channel.AddAbilities(tx))
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

	publishChannelCredentialCache(channelID, newKey)
	return true, nil
}

// UpdateChannelCredentialAndSubjectIfUnchanged commits a credential CAS and
// moves its already verified sampling-target binding in the same transaction.
// The provider-confirmed subject itself is immutable across this operation.
func UpdateChannelCredentialAndSubjectIfUnchanged(ctx context.Context, channelID, channelType int, expectedKey, newKey, oldCredentialSubject, newCredentialSubject, confirmedSubject string) (bool, error) {
	if DB == nil || channelID <= 0 || expectedKey == "" || newKey == "" ||
		!canonicalChannelQuotaIdentitySubject(oldCredentialSubject) || !canonicalChannelQuotaIdentitySubject(newCredentialSubject) ||
		!canonicalChannelQuotaIdentitySubject(confirmedSubject) {
		return false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if lease, ok := ctx.Value(channelCredentialGateContextKey{}).(channelCredentialGateLease); !ok || lease.channelID != channelID {
		gateCtx, cancel := context.WithTimeout(ctx, channelCredentialGateTimeout)
		defer cancel()
		var updated bool
		err := WithChannelCredentialUpdateGate(gateCtx, channelID, func(lockedCtx context.Context) error {
			var err error
			updated, err = updateChannelCredentialAndSubjectIfUnchanged(lockedCtx, channelID, channelType, expectedKey, newKey, oldCredentialSubject, newCredentialSubject, confirmedSubject)
			return err
		})
		return updated, err
	}
	return updateChannelCredentialAndSubjectIfUnchanged(ctx, channelID, channelType, expectedKey, newKey, oldCredentialSubject, newCredentialSubject, confirmedSubject)
}

func updateChannelCredentialAndSubjectIfUnchanged(ctx context.Context, channelID, channelType int, expectedKey, newKey, oldCredentialSubject, newCredentialSubject, confirmedSubject string) (bool, error) {
	updated := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := lockForUpdate(tx).Select("id", "key").
			Where(map[string]any{"id": channelID, "type": channelType, "key": expectedKey}).Take(&channel).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return sanitizeDBError(err)
		}
		var target ChannelQuotaSamplingTarget
		if err := lockForUpdate(tx).
			Where("channel_id = ? AND subject_ref = ? AND active = ? AND confirmed_subject_ref = ?",
				channelID, oldCredentialSubject, true, confirmedSubject).
			Take(&target).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelCredentialSubjectUnknown
			}
			return sanitizeDBError(err)
		}
		if oldCredentialSubject != newCredentialSubject {
			var existing ChannelQuotaSamplingTarget
			err := lockForUpdate(tx).Where("channel_id = ? AND subject_ref = ?", channelID, newCredentialSubject).Take(&existing).Error
			if err == nil {
				return ErrChannelCredentialSubjectConflict
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return sanitizeDBError(err)
			}
		}
		result := tx.Model(&Channel{}).
			Where(map[string]any{"id": channelID, "type": channelType, "key": expectedKey}).
			Update("key", newKey)
		if result.Error != nil {
			return sanitizeDBError(result.Error)
		}
		if result.RowsAffected != 1 && expectedKey != newKey {
			return nil
		}
		if oldCredentialSubject != newCredentialSubject {
			now, err := taskRecoveryDBTimestamp(tx)
			if err != nil {
				return err
			}
			nextEvidenceVersion := target.EvidenceVersion + 1
			if target.EvidenceVersion <= 0 {
				nextEvidenceVersion = 1
			}
			if nextEvidenceVersion <= 0 {
				return errors.New("channel credential evidence version exhausted")
			}
			result = tx.Model(&ChannelQuotaSamplingTarget{}).
				Where("id = ? AND channel_id = ? AND subject_ref = ? AND active = ? AND confirmed_subject_ref = ?",
					target.ID, channelID, oldCredentialSubject, true, confirmedSubject).
				Updates(map[string]any{
					"subject_ref": newCredentialSubject, "evidence_version": nextEvidenceVersion,
					"verified_snapshot_id": 0, "updated_at": now,
				})
			if result.Error != nil {
				return sanitizeDBError(result.Error)
			}
			if result.RowsAffected != 1 {
				return ErrChannelCredentialSubjectConflict
			}
		}
		updated = true
		return nil
	})
	if err != nil || !updated {
		return false, err
	}
	publishChannelCredentialCache(channelID, newKey)
	return true, nil
}

func publishChannelCredentialCache(channelID int, newKey string) {
	if !common.MemoryCacheEnabled {
		return
	}
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
