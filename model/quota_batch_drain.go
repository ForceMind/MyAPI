package model

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const quotaBalanceBatchDrainSchemaVersion = 3
const quotaBalanceSubjectKindUnknown = 0

const (
	quotaBalanceCompensationLeaseSeconds = 30
	quotaBalanceJournalRetention         = 7 * 24 * time.Hour
	quotaBalanceGenerationRetention      = 7 * 24 * 60 * 60
	quotaBalanceCleanupBatchSize         = 200
	quotaBalanceMigrationRunBudget       = 1000
	quotaBalanceSubjectPreparationLimit  = 100
	quotaBalanceSubjectPreparationBudget = 500 * time.Millisecond
	quotaBalanceSubjectDrainLimit        = 200
	quotaBalanceSubjectDrainBudget       = time.Second
)

const (
	quotaBalanceBatchDrainPending      = "pending"
	quotaBalanceBatchDrainApplying     = "applying"
	quotaBalanceBatchDrainCompensating = "compensating"
	quotaBalanceBatchDrainApplied      = "applied"
	quotaBalanceBatchDrainFailed       = "failed"
	quotaBalanceBatchDrainCancelled    = "cancelled"
	quotaBalanceBatchDrainUnknown      = "unknown"
)

var (
	ErrQuotaBalanceMutationUnknown    = errors.New("quota balance mutation outcome is unknown")
	ErrQuotaBalanceGenerationConflict = errors.New("quota balance generation conflicts with persisted identity")
	ErrQuotaBalanceSubjectBusy        = errors.New("quota balance subject is busy")
)

type QuotaBalanceBatchPayload struct {
	UserQuota      map[int]int    `json:"user_quota,omitempty"`
	TokenQuota     map[int]int    `json:"token_quota,omitempty"`
	TokenCacheKeys map[int]string `json:"token_cache_keys,omitempty"`
}

func (payload *QuotaBalanceBatchPayload) Scan(value interface{}) error {
	*payload = QuotaBalanceBatchPayload{}
	data, err := taskRecoveryTextValue(value)
	if err != nil || len(data) == 0 {
		return err
	}
	return common.Unmarshal(data, payload)
}

func (payload QuotaBalanceBatchPayload) Value() (driver.Value, error) {
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return string(data), nil
}

// QuotaBalanceBatchDrain is the durable handoff between the legacy in-memory
// balance queue and the main database. Applying the payload and marking the
// generation applied happen in one transaction.
type QuotaBalanceBatchDrain struct {
	ID                 int64                    `json:"id" gorm:"primaryKey"`
	SchemaVersion      int                      `json:"schema_version" gorm:"not null"`
	GenerationKey      string                   `json:"generation_key" gorm:"type:varchar(96);not null;uniqueIndex"`
	WriterEpoch        int64                    `json:"writer_epoch" gorm:"type:bigint;not null"`
	Payload            QuotaBalanceBatchPayload `json:"payload" gorm:"type:text"`
	PayloadFingerprint string                   `json:"payload_fingerprint" gorm:"type:char(64);not null;default:''"`
	SubjectKind        int                      `json:"subject_kind" gorm:"not null;default:0;index:idx_quota_balance_subject_preparation,priority:1"`
	SubjectID          int                      `json:"subject_id" gorm:"not null;default:0;index:idx_quota_balance_subject_preparation,priority:2"`
	CacheApplied       bool                     `json:"cache_applied" gorm:"not null;default:false;index:idx_quota_balance_subject_preparation,priority:4"`
	State              string                   `json:"state" gorm:"type:varchar(16);not null;index;index:idx_quota_balance_subject_preparation,priority:3"`
	Attempts           int                      `json:"attempts" gorm:"not null"`
	LastError          string                   `json:"last_error" gorm:"type:text;not null"`
	LeaseOwner         string                   `json:"lease_owner" gorm:"type:varchar(128);not null;default:''"`
	LeaseUntil         int64                    `json:"lease_until" gorm:"type:bigint;not null;default:0;index"`
	LockVersion        int64                    `json:"lock_version" gorm:"type:bigint;not null;default:1"`
	CreatedAt          int64                    `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64                    `json:"updated_at" gorm:"type:bigint;not null;index"`
}

func (QuotaBalanceBatchDrain) TableName() string { return "quota_balance_batch_drains" }

type QuotaBalanceBatchSubject struct {
	ID           int64 `json:"id" gorm:"primaryKey"`
	GenerationID int64 `json:"generation_id" gorm:"type:bigint;not null;uniqueIndex:uidx_quota_balance_generation_subject,priority:1;index:idx_quota_balance_subject_generation,priority:3;index:idx_quota_balance_subject_lookup,priority:3"`
	SubjectKind  int   `json:"subject_kind" gorm:"not null;uniqueIndex:uidx_quota_balance_generation_subject,priority:2;index:idx_quota_balance_subject_generation,priority:1;index:idx_quota_balance_subject_lookup,priority:1"`
	SubjectID    int   `json:"subject_id" gorm:"not null;uniqueIndex:uidx_quota_balance_generation_subject,priority:3;index:idx_quota_balance_subject_generation,priority:2;index:idx_quota_balance_subject_lookup,priority:2"`
}

func (QuotaBalanceBatchSubject) TableName() string { return "quota_balance_batch_subjects" }

func quotaBalanceGenerationSubjects(generationID int64, payload QuotaBalanceBatchPayload) ([]QuotaBalanceBatchSubject, error) {
	if generationID <= 0 {
		return nil, ErrQuotaBalanceGenerationConflict
	}
	subjects := make([]QuotaBalanceBatchSubject, 0, len(payload.UserQuota)+len(payload.TokenQuota))
	userKind, err := quotaBalanceStoredSubjectKind(BatchUpdateTypeUserQuota)
	if err != nil {
		return nil, err
	}
	tokenKind, err := quotaBalanceStoredSubjectKind(BatchUpdateTypeTokenQuota)
	if err != nil {
		return nil, err
	}
	for id := range payload.UserQuota {
		if id <= 0 {
			return nil, ErrQuotaBalanceGenerationConflict
		}
		subjects = append(subjects, QuotaBalanceBatchSubject{GenerationID: generationID, SubjectKind: userKind, SubjectID: id})
	}
	for id := range payload.TokenQuota {
		if id <= 0 {
			return nil, ErrQuotaBalanceGenerationConflict
		}
		subjects = append(subjects, QuotaBalanceBatchSubject{GenerationID: generationID, SubjectKind: tokenKind, SubjectID: id})
	}
	if len(subjects) == 0 {
		return nil, ErrQuotaBalanceGenerationConflict
	}
	return subjects, nil
}

func ensureQuotaBalanceBatchSubjects(db *gorm.DB, generation *QuotaBalanceBatchDrain) error {
	if db == nil || generation == nil {
		return gorm.ErrInvalidDB
	}
	subjects, err := quotaBalanceGenerationSubjects(generation.ID, generation.Payload)
	if err != nil {
		return err
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&subjects).Error
}

var (
	quotaBalanceSubjectLockTTL              = 30 * time.Second
	quotaBalanceSubjectLockRenewalDisabled  atomic.Bool
	quotaBalanceFlushInflight               atomic.Int64
	quotaBalanceGenerationSequence          atomic.Uint64
	quotaBalancePendingGeneration           *QuotaBalanceBatchDrain
	quotaBalanceBatchPersistAfterCreateHook func() error
	quotaBalanceBatchBeforeCreateHook       func(*QuotaBalanceBatchDrain) error
	quotaBalanceBatchBeforeConfirmHook      func(*QuotaBalanceBatchDrain) error
	quotaBalanceBatchCacheResponseHook      func(*QuotaBalanceBatchDrain, cacheQuotaResult, error) (cacheQuotaResult, error)
	quotaBalanceBatchJournalReadHook        func(context.Context, string) (string, error, bool)
	quotaBalanceBatchAfterCompensateHook    func(*QuotaBalanceBatchDrain) error
	quotaBalanceMigrationBatchHook          func(int)
)

type quotaBalanceSubjectLock struct {
	Kind        int
	ID          int
	Key         string
	Token       string
	renewCancel context.CancelFunc
	done        chan struct{}
	releaseOnce sync.Once
	lost        atomic.Bool
}

func quotaBalanceSubjectLockKey(kind, id int) (string, error) {
	switch kind {
	case BatchUpdateTypeUserQuota:
		return fmt.Sprintf("quota:balance:subject:user:%d", id), nil
	case BatchUpdateTypeTokenQuota:
		return fmt.Sprintf("quota:balance:subject:token:%d", id), nil
	default:
		return "", ErrQuotaBalanceGenerationConflict
	}
}

func newQuotaBalanceSubjectLockToken() (string, error) {
	data := make([]byte, 24)
	if _, err := cryptorand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func startQuotaBalanceSubjectLockRenewal(parent context.Context, lock *quotaBalanceSubjectLock) {
	baseCtx := context.Background()
	if parent != nil {
		baseCtx = parent
	}
	renewCtx, cancel := context.WithCancel(baseCtx)
	lock.renewCancel = cancel
	lock.done = make(chan struct{})
	go func() {
		defer close(lock.done)
		interval := quotaBalanceSubjectLockTTL / 3
		if interval < 10*time.Millisecond {
			interval = 10 * time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		const script = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
  return 1
end
return 0`
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				if quotaBalanceSubjectLockRenewalDisabled.Load() {
					continue
				}
				timeout := interval / 4
				if timeout < 25*time.Millisecond {
					timeout = 25 * time.Millisecond
				}
				if timeout > 250*time.Millisecond {
					timeout = 250 * time.Millisecond
				}
				ctx, cancel := context.WithTimeout(renewCtx, timeout)
				result, err := common.RDB.Eval(ctx, script, []string{lock.Key}, lock.Token, quotaBalanceSubjectLockTTL.Milliseconds()).Int()
				cancel()
				if err != nil || result != 1 {
					if renewCtx.Err() == nil {
						lock.lost.Store(true)
					}
					return
				}
			}
		}
	}()
}

func acquireQuotaBalanceSubjectLock(kind, id int) (*quotaBalanceSubjectLock, error) {
	return acquireQuotaBalanceSubjectLockContext(nil, kind, id)
}

func acquireQuotaBalanceSubjectLockContext(parent context.Context, kind, id int) (*quotaBalanceSubjectLock, error) {
	if !common.RedisEnabled || common.RDB == nil || id <= 0 {
		return nil, ErrBatchQuotaCacheUnavailable
	}
	key, err := quotaBalanceSubjectLockKey(kind, id)
	if err != nil {
		return nil, err
	}
	token, err := newQuotaBalanceSubjectLockToken()
	if err != nil {
		return nil, err
	}
	baseCtx := context.Background()
	if parent != nil {
		baseCtx = parent
	}
	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	for {
		acquired, err := common.RDB.SetNX(ctx, key, token, quotaBalanceSubjectLockTTL).Result()
		if err != nil {
			if parent != nil && parent.Err() != nil {
				return nil, parent.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, ErrQuotaBalanceSubjectBusy
			}
			return nil, fmt.Errorf("%w: subject lock acquire: %v", ErrBatchQuotaCacheUnavailable, err)
		}
		if acquired {
			lock := &quotaBalanceSubjectLock{Kind: kind, ID: id, Key: key, Token: token}
			startQuotaBalanceSubjectLockRenewal(parent, lock)
			return lock, nil
		}
		select {
		case <-ctx.Done():
			if parent != nil && parent.Err() != nil {
				return nil, parent.Err()
			}
			return nil, ErrQuotaBalanceSubjectBusy
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func tryAcquireQuotaBalanceSubjectLock(kind, id int) (*quotaBalanceSubjectLock, bool, error) {
	if !common.RedisEnabled || common.RDB == nil || id <= 0 {
		return nil, false, ErrBatchQuotaCacheUnavailable
	}
	key, err := quotaBalanceSubjectLockKey(kind, id)
	if err != nil {
		return nil, false, err
	}
	token, err := newQuotaBalanceSubjectLockToken()
	if err != nil {
		return nil, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	acquired, err := common.RDB.SetNX(ctx, key, token, quotaBalanceSubjectLockTTL).Result()
	if err != nil {
		return nil, false, fmt.Errorf("%w: subject lock try-acquire: %v", ErrBatchQuotaCacheUnavailable, err)
	}
	if !acquired {
		return nil, false, nil
	}
	lock := &quotaBalanceSubjectLock{Kind: kind, ID: id, Key: key, Token: token}
	startQuotaBalanceSubjectLockRenewal(nil, lock)
	return lock, true, nil
}

func verifyQuotaBalanceSubjectLock(lock *quotaBalanceSubjectLock) error {
	if lock == nil || lock.Key == "" || lock.Token == "" || lock.lost.Load() {
		return fmt.Errorf("%w: subject lock lease lost", ErrQuotaBalanceMutationUnknown)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	value, err := common.RDB.Get(ctx, lock.Key).Result()
	if err != nil || value != lock.Token {
		lock.lost.Store(true)
		return fmt.Errorf("%w: subject lock ownership lost", ErrQuotaBalanceMutationUnknown)
	}
	return nil
}

func releaseQuotaBalanceSubjectLockContext(ctx context.Context, lock *quotaBalanceSubjectLock) error {
	if lock == nil || common.RDB == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	releaseCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	lock.releaseOnce.Do(func() {
		if lock.renewCancel != nil {
			lock.renewCancel()
		}
	})
	if lock.done != nil {
		select {
		case <-lock.done:
		case <-releaseCtx.Done():
			return releaseCtx.Err()
		}
	}
	if err := releaseCtx.Err(); err != nil {
		return err
	}
	const script = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0`
	_, err := common.RDB.Eval(releaseCtx, script, []string{lock.Key}, lock.Token).Result()
	return err
}

func releaseQuotaBalanceSubjectLock(lock *quotaBalanceSubjectLock) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = releaseQuotaBalanceSubjectLockContext(ctx, lock)
}

func quotaBalancePayloadSubject(payload QuotaBalanceBatchPayload) (int, int, error) {
	if len(payload.UserQuota) == 1 && len(payload.TokenQuota) == 0 {
		for id := range payload.UserQuota {
			if id > 0 {
				return BatchUpdateTypeUserQuota, id, nil
			}
		}
	}
	if len(payload.TokenQuota) == 1 && len(payload.UserQuota) == 0 {
		for id := range payload.TokenQuota {
			if id > 0 {
				return BatchUpdateTypeTokenQuota, id, nil
			}
		}
	}
	return 0, 0, ErrQuotaBalanceGenerationConflict
}

func quotaBalanceStoredSubjectKind(kind int) (int, error) {
	if kind != BatchUpdateTypeUserQuota && kind != BatchUpdateTypeTokenQuota {
		return quotaBalanceSubjectKindUnknown, ErrQuotaBalanceGenerationConflict
	}
	return kind + 1, nil
}

func quotaBalanceGenerationSubject(generation *QuotaBalanceBatchDrain) (int, int, error) {
	if generation == nil {
		return 0, 0, ErrQuotaBalanceGenerationConflict
	}
	kind, id, err := quotaBalancePayloadSubject(generation.Payload)
	if err != nil {
		return 0, 0, err
	}
	if generation.SubjectID != 0 {
		storedKind, encodeErr := quotaBalanceStoredSubjectKind(kind)
		if encodeErr != nil || generation.SubjectKind != storedKind || generation.SubjectID != id {
			return 0, 0, ErrQuotaBalanceGenerationConflict
		}
	}
	return kind, id, nil
}

func quotaBalanceBatchPayloadFingerprint(payload QuotaBalanceBatchPayload) (string, error) {
	data, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func quotaBalanceTokenCacheLocators(db *gorm.DB, tokenQuota map[int]int) (map[int]string, error) {
	if len(tokenQuota) == 0 {
		return nil, nil
	}
	locators := make(map[int]string, len(tokenQuota))
	for tokenID := range tokenQuota {
		var token Token
		if err := db.Select("id", "key").Where("id = ?", tokenID).First(&token).Error; err != nil {
			return nil, err
		}
		if token.Id != tokenID || token.Key == "" {
			return nil, ErrQuotaBalanceGenerationConflict
		}
		locators[tokenID] = getTokenCacheKey(token.Key)
	}
	return locators, nil
}

func validateQuotaBalancePayload(payload QuotaBalanceBatchPayload) error {
	if len(payload.UserQuota) == 0 && len(payload.TokenQuota) == 0 {
		return ErrQuotaBalanceGenerationConflict
	}
	for id, delta := range payload.UserQuota {
		if id <= 0 || delta == 0 {
			return ErrQuotaBalanceGenerationConflict
		}
	}
	for id, delta := range payload.TokenQuota {
		locator, ok := payload.TokenCacheKeys[id]
		if id <= 0 || delta == 0 || !ok || len(locator) <= len("token:") || locator[:len("token:")] != "token:" {
			return ErrQuotaBalanceGenerationConflict
		}
	}
	if len(payload.TokenCacheKeys) != len(payload.TokenQuota) {
		return ErrQuotaBalanceGenerationConflict
	}
	return nil
}

func prepareQuotaBalanceBatchGeneration(payload QuotaBalanceBatchPayload) (*QuotaBalanceBatchDrain, error) {
	if DB == nil || !DB.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
		return nil, fmt.Errorf("quota balance batch drain table is unavailable")
	}
	state, err := GetQuotaWriterEpochState(DB)
	if err != nil {
		return nil, err
	}
	if QuotaWriterMode(state.Mode) != QuotaWriterModeLegacy {
		return nil, ErrLegacyQuotaWriterModeDisabled
	}
	if len(payload.TokenQuota) > 0 && len(payload.TokenCacheKeys) == 0 {
		locators, err := quotaBalanceTokenCacheLocators(DB, payload.TokenQuota)
		if err != nil {
			return nil, err
		}
		payload.TokenCacheKeys = locators
	}
	if err := validateQuotaBalancePayload(payload); err != nil {
		return nil, err
	}
	fingerprint, err := quotaBalanceBatchPayloadFingerprint(payload)
	if err != nil {
		return nil, err
	}
	subjectKind := quotaBalanceSubjectKindUnknown
	subjectID := 0
	if payloadKind, payloadID, subjectErr := quotaBalancePayloadSubject(payload); subjectErr == nil {
		subjectKind, err = quotaBalanceStoredSubjectKind(payloadKind)
		if err != nil {
			return nil, err
		}
		subjectID = payloadID
	}
	now := GetDBTimestamp()
	generation := &QuotaBalanceBatchDrain{
		SchemaVersion: quotaBalanceBatchDrainSchemaVersion, SubjectKind: subjectKind, SubjectID: subjectID,
		GenerationKey: fmt.Sprintf("legacy-balance:%d:%d:%d", state.Epoch, time.Now().UnixNano(), quotaBalanceGenerationSequence.Add(1)),
		WriterEpoch:   state.Epoch, Payload: payload, PayloadFingerprint: fingerprint,
		State: quotaBalanceBatchDrainPending, CacheApplied: false, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
	}
	if quotaBalanceBatchBeforeCreateHook != nil {
		if err := quotaBalanceBatchBeforeCreateHook(generation); err != nil {
			return nil, err
		}
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(generation).Error; err != nil {
			return err
		}
		return ensureQuotaBalanceBatchSubjects(tx, generation)
	}); err != nil {
		return nil, err
	}
	return generation, nil
}

func finishQuotaBalanceBatchPreparation(generation *QuotaBalanceBatchDrain, cacheApplied bool, state string, message string) error {
	if generation == nil || generation.ID <= 0 {
		return fmt.Errorf("invalid quota balance generation")
	}
	if len(message) > 4096 {
		message = message[:4096]
	}
	result := DB.Model(&QuotaBalanceBatchDrain{}).Where("id = ? AND state = ? AND cache_applied = ?", generation.ID, quotaBalanceBatchDrainPending, false).
		Updates(map[string]interface{}{"cache_applied": cacheApplied, "state": state, "last_error": message, "updated_at": GetDBTimestamp()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("quota balance generation preparation compare-and-swap lost")
	}
	generation.CacheApplied = cacheApplied
	generation.State = state
	return nil
}

type quotaBalanceJournalOutcome string

const (
	quotaBalanceJournalPrepared    quotaBalanceJournalOutcome = "prepared"
	quotaBalanceJournalApplied     quotaBalanceJournalOutcome = "applied"
	quotaBalanceJournalCompensated quotaBalanceJournalOutcome = "compensated"
	quotaBalanceJournalCancelled   quotaBalanceJournalOutcome = "cancelled"
	quotaBalanceJournalAbsent      quotaBalanceJournalOutcome = "absent"
)

func prepareQuotaBalanceJournal(generationKey string) error {
	if !common.RedisEnabled || common.RDB == nil {
		return ErrBatchQuotaCacheUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	created, err := common.RDB.SetNX(ctx, quotaBalanceJournalRedisKey(generationKey), string(quotaBalanceJournalPrepared), quotaBalanceJournalRetention).Result()
	if err != nil {
		return fmt.Errorf("%w: journal prepare: %v", ErrQuotaBalanceMutationUnknown, err)
	}
	if created {
		return nil
	}
	journal, err := readQuotaBalanceJournalDetached(generationKey)
	if err != nil {
		return err
	}
	if journal == quotaBalanceJournalPrepared || journal == quotaBalanceJournalApplied {
		if err := common.RDB.Expire(ctx, quotaBalanceJournalRedisKey(generationKey), quotaBalanceJournalRetention).Err(); err != nil {
			return fmt.Errorf("%w: journal retention: %v", ErrQuotaBalanceMutationUnknown, err)
		}
		return nil
	}
	return fmt.Errorf("%w: journal prepare found %s", ErrQuotaBalanceMutationUnknown, journal)
}

func cancelPreparedQuotaBalanceJournal(generationKey string) (quotaBalanceJournalOutcome, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return "", ErrBatchQuotaCacheUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	const script = `
local marker = redis.call('GET', KEYS[1])
if marker == 'prepared' then
  redis.call('SET', KEYS[1], 'cancelled', 'EX', 604800)
  return 1
end
if marker == 'applied' then return 2 end
if marker == 'compensated' then return 3 end
if marker == 'cancelled' then return 4 end
if not marker then return 0 end
return -1`
	result, err := common.RDB.Eval(ctx, script, []string{quotaBalanceJournalRedisKey(generationKey)}).Int()
	if err != nil {
		return "", fmt.Errorf("%w: journal cancel: %v", ErrQuotaBalanceMutationUnknown, err)
	}
	switch result {
	case 0:
		return quotaBalanceJournalAbsent, nil
	case 1, 4:
		return quotaBalanceJournalCancelled, nil
	case 2:
		return quotaBalanceJournalApplied, nil
	case 3:
		return quotaBalanceJournalCompensated, nil
	default:
		return "", fmt.Errorf("%w: invalid journal cancel result %d", ErrQuotaBalanceMutationUnknown, result)
	}
}

func validateQuotaBalanceGenerationIdentity(expected, stored *QuotaBalanceBatchDrain) error {
	if expected == nil || stored == nil || expected.ID <= 0 || stored.ID != expected.ID ||
		stored.SchemaVersion != expected.SchemaVersion || stored.GenerationKey != expected.GenerationKey ||
		stored.WriterEpoch != expected.WriterEpoch || stored.PayloadFingerprint != expected.PayloadFingerprint ||
		!reflect.DeepEqual(stored.Payload, expected.Payload) {
		return ErrQuotaBalanceGenerationConflict
	}
	fingerprint, err := quotaBalanceBatchPayloadFingerprint(stored.Payload)
	if err != nil || fingerprint != stored.PayloadFingerprint {
		return ErrQuotaBalanceGenerationConflict
	}
	return nil
}

func loadQuotaBalanceGenerationDetached(expected *QuotaBalanceBatchDrain) (*QuotaBalanceBatchDrain, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	var stored QuotaBalanceBatchDrain
	if err := DB.WithContext(ctx).Where("id = ?", expected.ID).First(&stored).Error; err != nil {
		return nil, err
	}
	if err := validateQuotaBalanceGenerationIdentity(expected, &stored); err != nil {
		return nil, err
	}
	return &stored, nil
}

func readQuotaBalanceJournalDetached(generationKey string) (quotaBalanceJournalOutcome, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return "", ErrBatchQuotaCacheUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	journalKey := quotaBalanceJournalRedisKey(generationKey)
	for attempt := 0; attempt < 3; attempt++ {
		var marker string
		var err error
		handled := false
		if quotaBalanceBatchJournalReadHook != nil {
			marker, err, handled = quotaBalanceBatchJournalReadHook(ctx, journalKey)
		}
		if !handled {
			marker, err = common.RDB.Get(ctx, journalKey).Result()
		}
		if err == nil {
			switch marker {
			case string(quotaBalanceJournalPrepared):
				return quotaBalanceJournalPrepared, nil
			case string(quotaBalanceJournalApplied):
				return quotaBalanceJournalApplied, nil
			case string(quotaBalanceJournalCompensated):
				return quotaBalanceJournalCompensated, nil
			case string(quotaBalanceJournalCancelled):
				return quotaBalanceJournalCancelled, nil
			default:
				return "", fmt.Errorf("invalid quota balance journal marker %q", marker)
			}
		}
		if !errors.Is(err, redis.Nil) {
			return "", fmt.Errorf("%w: journal read: %v", ErrQuotaBalanceMutationUnknown, err)
		}
		if pingErr := common.RDB.Ping(ctx).Err(); pingErr != nil {
			return "", fmt.Errorf("%w: journal absence could not be verified: %v", ErrQuotaBalanceMutationUnknown, pingErr)
		}
		if attempt < 2 {
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("%w: journal read timeout", ErrQuotaBalanceMutationUnknown)
			case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
			}
		}
	}
	return quotaBalanceJournalAbsent, nil
}

func transitionQuotaBalanceGeneration(expected *QuotaBalanceBatchDrain, fromState string, fromCacheApplied bool, toState string, toCacheApplied bool, message string) (bool, error) {
	if len(message) > 4096 {
		message = message[:4096]
	}
	result := DB.Model(&QuotaBalanceBatchDrain{}).
		Where("id = ? AND generation_key = ? AND writer_epoch = ? AND payload_fingerprint = ? AND state = ? AND cache_applied = ?",
			expected.ID, expected.GenerationKey, expected.WriterEpoch, expected.PayloadFingerprint, fromState, fromCacheApplied).
		Updates(map[string]interface{}{"state": toState, "cache_applied": toCacheApplied, "last_error": message, "updated_at": GetDBTimestamp()})
	return result.RowsAffected == 1, result.Error
}

func completeQuotaBalanceGeneration(expected *QuotaBalanceBatchDrain) (bool, error) {
	for attempt := 0; attempt < 6; attempt++ {
		stored, err := loadQuotaBalanceGenerationDetached(expected)
		if err != nil {
			return false, fmt.Errorf("%w: generation readback: %v", ErrQuotaBalanceMutationUnknown, err)
		}
		switch stored.State {
		case quotaBalanceBatchDrainApplied:
			if !stored.CacheApplied {
				return false, ErrQuotaBalanceGenerationConflict
			}
			return true, nil
		case quotaBalanceBatchDrainPending, quotaBalanceBatchDrainFailed:
			if !stored.CacheApplied {
				return false, nil
			}
			if err := applyQuotaBalanceBatchGeneration(stored.ID); err != nil {
				if attempt == 5 {
					return false, fmt.Errorf("%w: generation drain: %v", ErrQuotaBalanceMutationUnknown, err)
				}
			} else {
				continue
			}
		case quotaBalanceBatchDrainApplying:
			if !stored.CacheApplied {
				return false, ErrQuotaBalanceGenerationConflict
			}
			if attempt == 5 {
				return false, fmt.Errorf("%w: generation remains applying", ErrQuotaBalanceMutationUnknown)
			}
		case quotaBalanceBatchDrainCancelled:
			return false, nil
		case quotaBalanceBatchDrainCompensating:
			if stored.LeaseUntil <= GetDBTimestamp() {
				if err := recoverQuotaBalanceCompensations(); err != nil {
					return false, err
				}
				continue
			}
			return false, fmt.Errorf("%w: generation is compensating", ErrQuotaBalanceMutationUnknown)
		default:
			return false, ErrQuotaBalanceGenerationConflict
		}
		time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
	}
	return false, fmt.Errorf("%w: generation completion was not observed", ErrQuotaBalanceMutationUnknown)
}

func confirmAndDrainQuotaBalanceGeneration(expected *QuotaBalanceBatchDrain) error {
	confirmErr := finishQuotaBalanceBatchPreparation(expected, true, quotaBalanceBatchDrainPending, "")
	if confirmErr != nil {
		completed, resolveErr := completeQuotaBalanceGeneration(expected)
		if completed {
			return nil
		}
		if resolveErr != nil {
			return resolveErr
		}
		return confirmErr
	}
	if err := applyQuotaBalanceBatchGeneration(expected.ID); err != nil {
		completed, resolveErr := completeQuotaBalanceGeneration(expected)
		if completed {
			return nil
		}
		if resolveErr != nil {
			return resolveErr
		}
		return err
	}
	return nil
}

func cancelPendingQuotaBalanceGeneration(expected *QuotaBalanceBatchDrain, message string) error {
	won, err := transitionQuotaBalanceGeneration(expected, quotaBalanceBatchDrainPending, false, quotaBalanceBatchDrainCancelled, false, message)
	if err != nil {
		return err
	}
	if won {
		return nil
	}
	completed, resolveErr := completeQuotaBalanceGeneration(expected)
	if completed {
		return nil
	}
	if resolveErr != nil {
		return resolveErr
	}
	stored, readErr := loadQuotaBalanceGenerationDetached(expected)
	if readErr == nil && stored.State == quotaBalanceBatchDrainCancelled && !stored.CacheApplied {
		return nil
	}
	return fmt.Errorf("%w: generation cancel compare-and-swap lost", ErrQuotaBalanceMutationUnknown)
}

func quotaBalanceGenerationTouchesSubject(generation *QuotaBalanceBatchDrain, kind, id int) bool {
	if generation == nil {
		return false
	}
	if kind == BatchUpdateTypeUserQuota {
		_, ok := generation.Payload.UserQuota[id]
		return ok
	}
	if kind == BatchUpdateTypeTokenQuota {
		_, ok := generation.Payload.TokenQuota[id]
		return ok
	}
	return false
}

func drainQuotaBalanceGenerationsForSubject(kind, id int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return drainQuotaBalanceGenerationsForSubjectWithDB(DB.WithContext(ctx), kind, id)
}

func drainQuotaBalanceGenerationsForSubjectWithDB(db *gorm.DB, kind, id int) error {
	if db == nil || id <= 0 {
		return gorm.ErrInvalidDB
	}
	storedKind, err := quotaBalanceStoredSubjectKind(kind)
	if err != nil {
		return err
	}
	baseCtx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		baseCtx = db.Statement.Context
	}
	if err := baseCtx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(baseCtx, quotaBalanceSubjectDrainBudget)
	defer cancel()
	deadline := time.Now().Add(quotaBalanceSubjectDrainBudget)
	db = db.WithContext(ctx)
	if !db.Migrator().HasTable(&QuotaWorkCursor{}) {
		return ErrQuotaWorkIncomplete
	}
	workCursor, err := loadQuotaWorkCursor(ctx, db, quotaWorkCursorBalanceMigration)
	if err != nil {
		return err
	}
	if !workCursor.Complete {
		return ErrQuotaWorkIncomplete
	}
	processed := 0
	for processed < quotaBalanceSubjectDrainLimit {
		if err := baseCtx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return ErrQuotaWorkIncomplete
		}
		pageSize := min(25, quotaBalanceSubjectDrainLimit-processed)
		var generations []QuotaBalanceBatchDrain
		err := db.Table("quota_balance_batch_drains AS generations").Select("generations.*").
			Joins("JOIN quota_balance_batch_subjects AS subjects ON subjects.generation_id = generations.id").
			Where("subjects.subject_kind = ? AND subjects.subject_id = ? AND generations.cache_applied = ? AND generations.state IN ?",
				storedKind, id, true, []string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainFailed, quotaBalanceBatchDrainApplying}).
			Order("generations.id ASC").Limit(pageSize).Scan(&generations).Error
		if err != nil {
			return err
		}
		if len(generations) == 0 {
			return nil
		}
		for index := range generations {
			if err := baseCtx.Err(); err != nil {
				return err
			}
			if time.Now().After(deadline) {
				return ErrQuotaWorkIncomplete
			}
			processed++
			if err := applyQuotaBalanceBatchGenerationWithDB(db, generations[index].ID); err != nil {
				return err
			}
		}
	}
	var remaining QuotaBalanceBatchSubject
	result := db.Table("quota_balance_batch_subjects AS subjects").Select("subjects.id").
		Joins("JOIN quota_balance_batch_drains AS generations ON generations.id = subjects.generation_id").
		Where("subjects.subject_kind = ? AND subjects.subject_id = ? AND generations.cache_applied = ? AND generations.state IN ?",
			storedKind, id, true, []string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainFailed, quotaBalanceBatchDrainApplying}).
		Limit(1).Scan(&remaining)
	if result.Error != nil {
		return result.Error
	}
	if remaining.ID > 0 {
		return ErrQuotaWorkIncomplete
	}
	return nil
}

func recoverQuotaBalanceSubjectPreparations(kind, id int, excludeID int64, subjectLock *quotaBalanceSubjectLock) error {
	ctx, cancel := context.WithTimeout(context.Background(), quotaBalanceSubjectPreparationBudget)
	defer cancel()
	return recoverQuotaBalanceSubjectPreparationsWithDB(ctx, DB, kind, id, excludeID, subjectLock)
}

func recoverQuotaBalanceSubjectPreparationsWithDB(ctx context.Context, db *gorm.DB, kind, id int, excludeID int64, subjectLock *quotaBalanceSubjectLock) error {
	if db == nil || (kind != BatchUpdateTypeUserQuota && kind != BatchUpdateTypeTokenQuota) || id <= 0 {
		return gorm.ErrInvalidDB
	}
	storedKind, err := quotaBalanceStoredSubjectKind(kind)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, quotaBalanceSubjectPreparationBudget)
	defer cancel()
	db = db.WithContext(ctx)
	var unindexed QuotaBalanceBatchDrain
	result := db.Select("id").Where("subject_kind = ? AND subject_id = ? AND state IN ? AND cache_applied = ?", quotaBalanceSubjectKindUnknown, 0,
		[]string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainCompensating}, false).Limit(1).Find(&unindexed)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return ErrQuotaWorkIncomplete
	}
	var cursor int64
	processed := 0
	for processed < quotaBalanceSubjectPreparationLimit {
		if err := verifyQuotaBalanceSubjectLock(subjectLock); err != nil {
			return err
		}
		remaining := quotaBalanceSubjectPreparationLimit - processed
		var generations []QuotaBalanceBatchDrain
		if err := db.Where("id > ? AND id <> ? AND subject_kind = ? AND subject_id = ? AND state = ? AND cache_applied = ?",
			cursor, excludeID, storedKind, id, quotaBalanceBatchDrainPending, false).Order("id ASC").Limit(min(25, remaining)).Find(&generations).Error; err != nil {
			return err
		}
		if len(generations) == 0 {
			return nil
		}
		for index := range generations {
			cursor = generations[index].ID
			processed++
			if err := recoverQuotaBalanceBatchPreparation(&generations[index], subjectLock); err != nil {
				return err
			}
		}
	}
	var remaining QuotaBalanceBatchDrain
	result = db.Select("id").Where("id > ? AND id <> ? AND subject_kind = ? AND subject_id = ? AND state = ? AND cache_applied = ?",
		cursor, excludeID, storedKind, id, quotaBalanceBatchDrainPending, false).Limit(1).Find(&remaining)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return ErrQuotaWorkIncomplete
	}
	return nil
}

func hasOtherUnconfirmedQuotaBalanceGeneration(kind, id int, excludeID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), quotaBalanceSubjectPreparationBudget)
	defer cancel()
	if DB == nil {
		return false, gorm.ErrInvalidDB
	}
	storedKind, err := quotaBalanceStoredSubjectKind(kind)
	if err != nil {
		return false, err
	}
	db := DB.WithContext(ctx)
	var unindexed QuotaBalanceBatchDrain
	result := db.Select("id").Where("subject_kind = ? AND subject_id = ? AND state IN ? AND cache_applied = ?", quotaBalanceSubjectKindUnknown, 0,
		[]string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainCompensating}, false).Limit(1).Find(&unindexed)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected > 0 {
		return false, ErrQuotaWorkIncomplete
	}
	var generation QuotaBalanceBatchDrain
	result = db.Select("id").Where("id <> ? AND subject_kind = ? AND subject_id = ? AND state IN ? AND cache_applied = ?", excludeID, storedKind, id,
		[]string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainCompensating}, false).Order("id ASC").Limit(1).Find(&generation)
	return result.RowsAffected > 0, result.Error
}

func invalidateQuotaBalanceSubjectCache(subjectLock *quotaBalanceSubjectLock, cacheKey, fenceKey string) error {
	if subjectLock == nil || cacheKey == "" {
		return ErrQuotaBalanceMutationUnknown
	}
	const script = `
if redis.call('GET', KEYS[1]) ~= ARGV[1] then
  return 0
end
if ARGV[2] ~= '' then
  redis.call('SET', ARGV[2], ARGV[1], 'EX', ARGV[3])
end
redis.call('DEL', KEYS[2])
return 1`
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	result, err := common.RDB.Eval(ctx, script, []string{subjectLock.Key, cacheKey}, subjectLock.Token, fenceKey, tokenCacheFenceSeconds).Int()
	if err != nil {
		return err
	}
	if result != 1 {
		subjectLock.lost.Store(true)
		return fmt.Errorf("%w: atomic cache invalidation lost subject ownership", ErrQuotaBalanceMutationUnknown)
	}
	return nil
}

func rehydrateLegacyBalanceCache(kind, id int, tokenKey string, currentGenerationID int64, subjectLock *quotaBalanceSubjectLock) error {
	if err := verifyQuotaBalanceSubjectLock(subjectLock); err != nil {
		return err
	}
	if err := recoverQuotaBalanceSubjectPreparations(kind, id, currentGenerationID, subjectLock); err != nil {
		return err
	}
	if err := drainQuotaBalanceGenerationsForSubject(kind, id); err != nil {
		return err
	}
	if err := verifyQuotaBalanceSubjectLock(subjectLock); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	switch kind {
	case BatchUpdateTypeUserQuota:
		if err := invalidateQuotaBalanceSubjectCache(subjectLock, getUserCacheKey(id), ""); err != nil {
			return err
		}
		var user User
		if err := DB.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		return writeUserCacheWithQuotaBalanceOwner(user.ToBaseUser(), true, subjectLock)
	case BatchUpdateTypeTokenQuota:
		if tokenKey == "" {
			var token Token
			if err := DB.WithContext(ctx).Select("id", "key").Where("id = ?", id).First(&token).Error; err != nil {
				return err
			}
			tokenKey = token.Key
		}
		if err := invalidateQuotaBalanceSubjectCache(subjectLock, getTokenCacheKey(tokenKey), getTokenCacheFenceKey(tokenKey)); err != nil {
			return err
		}
		var token Token
		if err := DB.WithContext(ctx).Where(commonKeyCol+" = ?", tokenKey).First(&token).Error; err != nil {
			return err
		}
		if token.Id != id || getTokenCacheKey(token.Key) != getTokenCacheKey(tokenKey) {
			return ErrQuotaBalanceGenerationConflict
		}
		_, err := cacheInitTokenWithQuotaBalanceOwner(token, subjectLock)
		return err
	default:
		return ErrQuotaBalanceGenerationConflict
	}
}

func invalidateQuotaBalanceGenerationSubjects(generation *QuotaBalanceBatchDrain, subjectLock *quotaBalanceSubjectLock) error {
	kind, id, err := quotaBalanceGenerationSubject(generation)
	if err != nil {
		return err
	}
	tokenKey := ""
	if kind == BatchUpdateTypeTokenQuota {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var token Token
		if err := DB.WithContext(ctx).Select("id", "key").Where("id = ?", id).First(&token).Error; err != nil {
			return err
		}
		locator := generation.Payload.TokenCacheKeys[id]
		if token.Id != id || token.Key == "" || getTokenCacheKey(token.Key) != locator {
			return ErrQuotaBalanceGenerationConflict
		}
		tokenKey = token.Key
	}
	return rehydrateLegacyBalanceCache(kind, id, tokenKey, generation.ID, subjectLock)
}

func markQuotaBalanceGenerationUnknown(generation *QuotaBalanceBatchDrain, message string) error {
	if len(message) > 4096 {
		message = message[:4096]
	}
	result := DB.Model(&QuotaBalanceBatchDrain{}).
		Where("id = ? AND state NOT IN ?", generation.ID, []string{quotaBalanceBatchDrainApplied, quotaBalanceBatchDrainCancelled}).
		Updates(map[string]interface{}{
			"state": quotaBalanceBatchDrainUnknown, "last_error": message, "lease_owner": "", "lease_until": int64(0),
			"lock_version": gorm.Expr("lock_version + ?", 1), "updated_at": GetDBTimestamp(),
		})
	return result.Error
}

func safelyCancelGenerationWithMissingJournal(generation *QuotaBalanceBatchDrain, message string, subjectLock *quotaBalanceSubjectLock) error {
	kind, id, subjectErr := quotaBalanceGenerationSubject(generation)
	if subjectErr != nil {
		_ = markQuotaBalanceGenerationUnknown(generation, message+": invalid subject identity")
		return subjectErr
	}
	blocked, err := hasOtherUnconfirmedQuotaBalanceGeneration(kind, id, generation.ID)
	if err != nil {
		_ = markQuotaBalanceGenerationUnknown(generation, message+": unconfirmed generation scan failed: "+err.Error())
		return fmt.Errorf("%w: unconfirmed generation scan: %v", ErrQuotaBalanceMutationUnknown, err)
	}
	if blocked {
		_ = markQuotaBalanceGenerationUnknown(generation, message+": another unconfirmed generation prevents cache invalidation")
		return fmt.Errorf("%w: another unconfirmed generation prevents cache invalidation", ErrQuotaBalanceMutationUnknown)
	}
	for userID := range generation.Payload.UserQuota {
		if err := drainQuotaBalanceGenerationsForSubject(BatchUpdateTypeUserQuota, userID); err != nil {
			_ = markQuotaBalanceGenerationUnknown(generation, message+": dependent user drain failed: "+err.Error())
			return fmt.Errorf("%w: dependent user drain: %v", ErrQuotaBalanceMutationUnknown, err)
		}
	}
	for tokenID := range generation.Payload.TokenQuota {
		if err := drainQuotaBalanceGenerationsForSubject(BatchUpdateTypeTokenQuota, tokenID); err != nil {
			_ = markQuotaBalanceGenerationUnknown(generation, message+": dependent token drain failed: "+err.Error())
			return fmt.Errorf("%w: dependent token drain: %v", ErrQuotaBalanceMutationUnknown, err)
		}
	}
	if err := invalidateQuotaBalanceGenerationSubjects(generation, subjectLock); err != nil {
		_ = markQuotaBalanceGenerationUnknown(generation, message+": cache invalidation failed: "+err.Error())
		return fmt.Errorf("%w: cache invalidation: %v", ErrQuotaBalanceMutationUnknown, err)
	}
	if err := markQuotaBalanceGenerationUnknown(generation, message+": cache invalidated; manual resolution required"); err != nil {
		return fmt.Errorf("%w: mark durable unknown: %v", ErrQuotaBalanceMutationUnknown, err)
	}
	return fmt.Errorf("%w: journal missing after cache invalidation", ErrQuotaBalanceMutationUnknown)
}

func claimQuotaBalanceCompensation(generation *QuotaBalanceBatchDrain, workerID, message string) (*QuotaBalanceBatchDrain, bool, error) {
	stored, err := loadQuotaBalanceGenerationDetached(generation)
	if err != nil {
		return nil, false, err
	}
	now := GetDBTimestamp()
	claimable := stored.State == quotaBalanceBatchDrainPending && !stored.CacheApplied
	if stored.State == quotaBalanceBatchDrainCompensating && stored.LeaseUntil <= now {
		claimable = true
	}
	if !claimable || workerID == "" {
		return stored, false, nil
	}
	if len(message) > 4096 {
		message = message[:4096]
	}
	result := DB.Model(&QuotaBalanceBatchDrain{}).
		Where("id = ? AND state = ? AND cache_applied = ? AND lock_version = ? AND lease_until = ?",
			stored.ID, stored.State, stored.CacheApplied, stored.LockVersion, stored.LeaseUntil).
		Updates(map[string]interface{}{
			"state": quotaBalanceBatchDrainCompensating, "lease_owner": workerID,
			"lease_until": now + quotaBalanceCompensationLeaseSeconds, "lock_version": stored.LockVersion + 1,
			"attempts": stored.Attempts + 1, "last_error": message, "updated_at": now,
		})
	if result.Error != nil || result.RowsAffected != 1 {
		return stored, false, result.Error
	}
	if err := DB.First(stored, stored.ID).Error; err != nil {
		return nil, false, err
	}
	return stored, true, nil
}

func compensateQuotaBalanceGenerationCache(generation *QuotaBalanceBatchDrain, subjectLock *quotaBalanceSubjectLock) (cacheQuotaResult, error) {
	if generation == nil {
		return cacheQuotaMiss, ErrQuotaBalanceGenerationConflict
	}
	if len(generation.Payload.UserQuota) == 1 && len(generation.Payload.TokenQuota) == 0 {
		for userID, delta := range generation.Payload.UserQuota {
			return compensateUserQuotaDeltaJournaled(userID, -int64(delta), generation.GenerationKey, subjectLock.Token)
		}
	}
	if len(generation.Payload.TokenQuota) == 1 && len(generation.Payload.UserQuota) == 0 {
		for tokenID, delta := range generation.Payload.TokenQuota {
			locator := generation.Payload.TokenCacheKeys[tokenID]
			if locator == "" {
				return cacheQuotaMiss, ErrQuotaBalanceGenerationConflict
			}
			epoch, err := quotaWriterEpochForLegacyCache(DB)
			if err != nil {
				return cacheQuotaMiss, err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			result, err := common.RDB.Eval(ctx, tokenQuotaCompensationScript,
				[]string{locator, quotaWriterEpochRedisKey, quotaBalanceJournalRedisKey(generation.GenerationKey), subjectLock.Key},
				-int64(delta), tokenID, common.GetTimestamp(), epoch, subjectLock.Token).Int()
			return quotaResultFromLua(result, err)
		}
	}
	return cacheQuotaMiss, ErrQuotaBalanceGenerationConflict
}

func finishQuotaBalanceCompensation(claimed *QuotaBalanceBatchDrain, message string) error {
	if len(message) > 4096 {
		message = message[:4096]
	}
	result := DB.Model(&QuotaBalanceBatchDrain{}).
		Where("id = ? AND state = ? AND lease_owner = ? AND lock_version = ?",
			claimed.ID, quotaBalanceBatchDrainCompensating, claimed.LeaseOwner, claimed.LockVersion).
		Updates(map[string]interface{}{
			"state": quotaBalanceBatchDrainCancelled, "lease_owner": "", "lease_until": int64(0),
			"lock_version": claimed.LockVersion + 1, "last_error": message, "updated_at": GetDBTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: compensation fence lost", ErrQuotaBalanceMutationUnknown)
	}
	return nil
}

func runQuotaBalanceCompensation(claimed *QuotaBalanceBatchDrain, subjectLock *quotaBalanceSubjectLock, compensate func(string, string) (cacheQuotaResult, error), message string) error {
	journal, err := readQuotaBalanceJournalDetached(claimed.GenerationKey)
	if err != nil {
		return err
	}
	switch journal {
	case quotaBalanceJournalCompensated, quotaBalanceJournalCancelled:
		return finishQuotaBalanceCompensation(claimed, message)
	case quotaBalanceJournalAbsent:
		return safelyCancelGenerationWithMissingJournal(claimed, message+": journal missing", subjectLock)
	case quotaBalanceJournalPrepared:
		outcome, cancelErr := cancelPreparedQuotaBalanceJournal(claimed.GenerationKey)
		if cancelErr != nil {
			return cancelErr
		}
		if outcome == quotaBalanceJournalCancelled || outcome == quotaBalanceJournalCompensated {
			return finishQuotaBalanceCompensation(claimed, message)
		}
		if outcome != quotaBalanceJournalApplied {
			return fmt.Errorf("%w: compensation journal resolved as %s", ErrQuotaBalanceMutationUnknown, outcome)
		}
	case quotaBalanceJournalApplied:
	}
	var result cacheQuotaResult
	if compensate != nil {
		result, err = compensate(claimed.GenerationKey, subjectLock.Token)
	} else {
		result, err = compensateQuotaBalanceGenerationCache(claimed, subjectLock)
	}
	if err != nil || result != cacheQuotaOK {
		observed, journalErr := readQuotaBalanceJournalDetached(claimed.GenerationKey)
		if journalErr != nil || observed != quotaBalanceJournalCompensated {
			if errors.Is(err, ErrQuotaBalanceMutationUnknown) || result == cacheQuotaMiss {
				return safelyCancelGenerationWithMissingJournal(claimed, message+": compensation cache identity unavailable", subjectLock)
			}
			return fmt.Errorf("%w: compensation response result=%d error=%v journal=%s journal_error=%v",
				ErrQuotaBalanceMutationUnknown, result, err, observed, journalErr)
		}
	}
	if quotaBalanceBatchAfterCompensateHook != nil {
		if err := quotaBalanceBatchAfterCompensateHook(claimed); err != nil {
			return fmt.Errorf("%w: after compensation: %v", ErrQuotaBalanceMutationUnknown, err)
		}
	}
	return finishQuotaBalanceCompensation(claimed, message)
}

func recoverQuotaBalanceCompensations() error {
	if DB == nil || !common.RedisEnabled || common.RDB == nil {
		return ErrBatchQuotaCacheUnavailable
	}
	now := GetDBTimestamp()
	var generations []QuotaBalanceBatchDrain
	if err := DB.Where("state = ? AND lease_until <= ?", quotaBalanceBatchDrainCompensating, now).Order("id ASC").Limit(100).Find(&generations).Error; err != nil {
		return err
	}
	var recoveryErrors []error
	for index := range generations {
		kind, id, err := quotaBalanceGenerationSubject(&generations[index])
		if err != nil {
			recoveryErrors = append(recoveryErrors, err)
			continue
		}
		subjectLock, acquired, err := tryAcquireQuotaBalanceSubjectLock(kind, id)
		if err != nil {
			recoveryErrors = append(recoveryErrors, err)
			continue
		}
		if !acquired {
			continue
		}
		workerID := fmt.Sprintf("quota-balance-recovery:%d:%d", now, generations[index].ID)
		claimed, won, claimErr := claimQuotaBalanceCompensation(&generations[index], workerID, generations[index].LastError)
		if claimErr != nil {
			recoveryErrors = append(recoveryErrors, claimErr)
		} else if won {
			if err := runQuotaBalanceCompensation(claimed, subjectLock, nil, claimed.LastError); err != nil {
				recoveryErrors = append(recoveryErrors, err)
			}
		}
		releaseQuotaBalanceSubjectLock(subjectLock)
	}
	return errors.Join(recoveryErrors...)
}

func reconcileUnknownQuotaBalanceCacheResponse(generation *QuotaBalanceBatchDrain, cacheErr error, subjectLock *quotaBalanceSubjectLock) error {
	completed, err := completeQuotaBalanceGeneration(generation)
	if completed {
		return nil
	}
	if err != nil {
		return err
	}
	journal, journalErr := readQuotaBalanceJournalDetached(generation.GenerationKey)
	if journalErr != nil {
		return journalErr
	}
	switch journal {
	case quotaBalanceJournalApplied:
		return confirmAndDrainQuotaBalanceGeneration(generation)
	case quotaBalanceJournalCompensated, quotaBalanceJournalCancelled:
		if err := cancelPendingQuotaBalanceGeneration(generation, "cache mutation was compensated"); err != nil {
			return err
		}
		return cacheErr
	case quotaBalanceJournalPrepared:
		cancelled, err := cancelPreparedQuotaBalanceJournal(generation.GenerationKey)
		if err != nil {
			return err
		}
		if cancelled == quotaBalanceJournalApplied {
			return confirmAndDrainQuotaBalanceGeneration(generation)
		}
		if cancelled != quotaBalanceJournalCancelled && cancelled != quotaBalanceJournalCompensated {
			return fmt.Errorf("%w: prepared journal cancellation resolved as %s", ErrQuotaBalanceMutationUnknown, cancelled)
		}
		if err := cancelPendingQuotaBalanceGeneration(generation, "cache mutation was proven unexecuted"); err != nil {
			return err
		}
		return cacheErr
	case quotaBalanceJournalAbsent:
		if err := safelyCancelGenerationWithMissingJournal(generation, "cache journal missing; subject caches invalidated", subjectLock); err != nil {
			return err
		}
		return cacheErr
	default:
		return fmt.Errorf("%w: unresolved cache response", ErrQuotaBalanceMutationUnknown)
	}
}

func recoverQuotaBalanceBatchPreparation(generation *QuotaBalanceBatchDrain, subjectLock *quotaBalanceSubjectLock) error {
	completed, err := completeQuotaBalanceGeneration(generation)
	if completed || err != nil {
		return err
	}
	journal, err := readQuotaBalanceJournalDetached(generation.GenerationKey)
	if err != nil {
		return err
	}
	switch journal {
	case quotaBalanceJournalApplied:
		return confirmAndDrainQuotaBalanceGeneration(generation)
	case quotaBalanceJournalCompensated, quotaBalanceJournalCancelled:
		return cancelPendingQuotaBalanceGeneration(generation, "cache mutation was compensated")
	case quotaBalanceJournalPrepared:
		cancelled, cancelErr := cancelPreparedQuotaBalanceJournal(generation.GenerationKey)
		if cancelErr != nil {
			return cancelErr
		}
		if cancelled == quotaBalanceJournalApplied {
			return confirmAndDrainQuotaBalanceGeneration(generation)
		}
		if cancelled == quotaBalanceJournalCancelled || cancelled == quotaBalanceJournalCompensated {
			return cancelPendingQuotaBalanceGeneration(generation, "cache mutation was proven unexecuted")
		}
		return fmt.Errorf("%w: recovery journal cancellation resolved as %s", ErrQuotaBalanceMutationUnknown, cancelled)
	case quotaBalanceJournalAbsent:
		return safelyCancelGenerationWithMissingJournal(generation, "cache journal missing during recovery; subject caches invalidated", subjectLock)
	default:
		return ErrQuotaBalanceMutationUnknown
	}
}

func recoverQuotaBalanceBatchPreparations() error {
	if DB == nil || !DB.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
		return nil
	}
	if !common.RedisEnabled || common.RDB == nil {
		return ErrBatchQuotaCacheUnavailable
	}
	var generations []QuotaBalanceBatchDrain
	if err := DB.Where("state = ? AND cache_applied = ?", quotaBalanceBatchDrainPending, false).Order("id ASC").Limit(100).Find(&generations).Error; err != nil {
		return err
	}
	var recoveryErrors []error
	for index := range generations {
		kind, id, err := quotaBalanceGenerationSubject(&generations[index])
		if err != nil {
			recoveryErrors = append(recoveryErrors, err)
			continue
		}
		subjectLock, acquired, err := tryAcquireQuotaBalanceSubjectLock(kind, id)
		if err != nil {
			recoveryErrors = append(recoveryErrors, err)
			continue
		}
		if !acquired {
			continue
		}
		err = recoverQuotaBalanceBatchPreparation(&generations[index], subjectLock)
		releaseQuotaBalanceSubjectLock(subjectLock)
		if err != nil {
			recoveryErrors = append(recoveryErrors, err)
		}
	}
	return errors.Join(recoveryErrors...)
}

func applyLegacyBalanceCacheMutation(kind, id int, delta int, tokenKey string, apply func(string, string) (cacheQuotaResult, error), compensate func(string, string) (cacheQuotaResult, error)) (cacheQuotaResult, error) {
	subjectLock, err := acquireQuotaBalanceSubjectLock(kind, id)
	if err != nil {
		return cacheQuotaMiss, err
	}
	defer releaseQuotaBalanceSubjectLock(subjectLock)
	payload := QuotaBalanceBatchPayload{}
	if kind == BatchUpdateTypeUserQuota {
		payload.UserQuota = map[int]int{id: delta}
	} else if kind == BatchUpdateTypeTokenQuota {
		if tokenKey == "" {
			return cacheQuotaMiss, ErrQuotaBalanceGenerationConflict
		}
		payload.TokenQuota = map[int]int{id: delta}
		payload.TokenCacheKeys = map[int]string{id: getTokenCacheKey(tokenKey)}
	} else {
		return cacheQuotaMiss, fmt.Errorf("invalid durable balance mutation kind")
	}
	generation, err := prepareQuotaBalanceBatchGeneration(payload)
	if err != nil {
		return cacheQuotaMiss, err
	}
	if err := prepareQuotaBalanceJournal(generation.GenerationKey); err != nil {
		return cacheQuotaMiss, err
	}
	result, cacheErr := apply(generation.GenerationKey, subjectLock.Token)
	if cacheErr == nil && result == cacheQuotaMiss {
		if hydrateErr := rehydrateLegacyBalanceCache(kind, id, tokenKey, generation.ID, subjectLock); hydrateErr != nil {
			cacheErr = hydrateErr
		} else {
			result, cacheErr = apply(generation.GenerationKey, subjectLock.Token)
		}
	}
	if quotaBalanceBatchCacheResponseHook != nil {
		result, cacheErr = quotaBalanceBatchCacheResponseHook(generation, result, cacheErr)
	}
	if cacheErr != nil {
		if err := reconcileUnknownQuotaBalanceCacheResponse(generation, cacheErr, subjectLock); err != nil {
			return result, err
		}
		return cacheQuotaOK, nil
	}
	if result != cacheQuotaOK {
		journal, journalErr := cancelPreparedQuotaBalanceJournal(generation.GenerationKey)
		if journalErr != nil {
			return result, journalErr
		}
		if journal == quotaBalanceJournalApplied {
			if err := confirmAndDrainQuotaBalanceGeneration(generation); err != nil {
				return result, err
			}
			return cacheQuotaOK, nil
		}
		if journal != quotaBalanceJournalCancelled && journal != quotaBalanceJournalCompensated && journal != quotaBalanceJournalAbsent {
			return result, fmt.Errorf("%w: rejected mutation journal resolved as %s", ErrQuotaBalanceMutationUnknown, journal)
		}
		if journal == quotaBalanceJournalAbsent {
			if err := safelyCancelGenerationWithMissingJournal(generation, "rejected cache mutation lost its journal", subjectLock); err != nil {
				return result, err
			}
			return result, nil
		}
		if err := cancelPendingQuotaBalanceGeneration(generation, fmt.Sprintf("cache mutation rejected: result=%d", result)); err != nil {
			return result, err
		}
		return result, nil
	}

	confirmErr := error(nil)
	if quotaBalanceBatchBeforeConfirmHook != nil {
		confirmErr = quotaBalanceBatchBeforeConfirmHook(generation)
	}
	if confirmErr == nil {
		confirmErr = finishQuotaBalanceBatchPreparation(generation, true, quotaBalanceBatchDrainPending, "")
	}
	if confirmErr == nil {
		return result, nil
	}
	completed, resolveErr := completeQuotaBalanceGeneration(generation)
	if completed {
		return result, nil
	}
	if resolveErr != nil {
		return result, resolveErr
	}
	workerID := fmt.Sprintf("quota-balance-foreground:%d:%d", time.Now().UnixNano(), generation.ID)
	claimed, won, claimErr := claimQuotaBalanceCompensation(generation, workerID, confirmErr.Error())
	if claimErr != nil {
		return result, claimErr
	}
	if !won {
		completed, resolveErr = completeQuotaBalanceGeneration(generation)
		if completed {
			return result, nil
		}
		if resolveErr != nil {
			return result, resolveErr
		}
		return result, fmt.Errorf("%w: compensation claim lost", ErrQuotaBalanceMutationUnknown)
	}
	if err := runQuotaBalanceCompensation(claimed, subjectLock, compensate, confirmErr.Error()); err != nil {
		return result, err
	}
	return result, confirmErr
}

func copyQuotaBatchStore(source map[int]int) map[int]int {
	if len(source) == 0 {
		return nil
	}
	result := make(map[int]int, len(source))
	for id, delta := range source {
		result[id] = delta
	}
	return result
}

func persistQuotaBalanceBatchGeneration() error {
	if DB == nil || !DB.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
		return nil
	}
	mode, err := currentQuotaWriterMode(DB)
	if err != nil {
		return err
	}
	if mode != QuotaWriterModeLegacy {
		return nil
	}
	batchUpdateLocks[BatchUpdateTypeUserQuota].Lock()
	batchUpdateLocks[BatchUpdateTypeTokenQuota].Lock()
	defer batchUpdateLocks[BatchUpdateTypeTokenQuota].Unlock()
	defer batchUpdateLocks[BatchUpdateTypeUserQuota].Unlock()

	if quotaBalancePendingGeneration == nil {
		userQuota := copyQuotaBatchStore(batchUpdateStores[BatchUpdateTypeUserQuota])
		tokenQuota := copyQuotaBatchStore(batchUpdateStores[BatchUpdateTypeTokenQuota])
		if len(userQuota) == 0 && len(tokenQuota) == 0 {
			return nil
		}
		state, err := GetQuotaWriterEpochState(DB)
		if err != nil {
			return err
		}
		now := GetDBTimestamp()
		tokenCacheKeys, err := quotaBalanceTokenCacheLocators(DB, tokenQuota)
		if err != nil {
			return err
		}
		payload := QuotaBalanceBatchPayload{UserQuota: userQuota, TokenQuota: tokenQuota, TokenCacheKeys: tokenCacheKeys}
		fingerprint, err := quotaBalanceBatchPayloadFingerprint(payload)
		if err != nil {
			return err
		}
		quotaBalancePendingGeneration = &QuotaBalanceBatchDrain{
			SchemaVersion: quotaBalanceBatchDrainSchemaVersion,
			GenerationKey: fmt.Sprintf("legacy-balance:%d:%d", state.Epoch, time.Now().UnixNano()),
			WriterEpoch:   state.Epoch, Payload: payload, PayloadFingerprint: fingerprint, CacheApplied: true,
			State: quotaBalanceBatchDrainPending, LockVersion: 1, CreatedAt: now, UpdatedAt: now,
		}
	}
	candidate := *quotaBalancePendingGeneration
	createErr := DB.Create(&candidate).Error
	if createErr == nil && quotaBalanceBatchPersistAfterCreateHook != nil {
		createErr = quotaBalanceBatchPersistAfterCreateHook()
	}
	var stored QuotaBalanceBatchDrain
	readErr := DB.Where("generation_key = ?", candidate.GenerationKey).First(&stored).Error
	if readErr != nil {
		if createErr != nil {
			return createErr
		}
		return readErr
	}
	if stored.SchemaVersion != candidate.SchemaVersion || stored.WriterEpoch != candidate.WriterEpoch || stored.PayloadFingerprint != candidate.PayloadFingerprint || !reflect.DeepEqual(stored.Payload, candidate.Payload) {
		return fmt.Errorf("quota balance batch generation conflicts with persisted payload")
	}
	if err := ensureQuotaBalanceBatchSubjects(DB, &stored); err != nil {
		return err
	}
	for id, delta := range candidate.Payload.UserQuota {
		remaining := batchUpdateStores[BatchUpdateTypeUserQuota][id] - delta
		if remaining == 0 {
			delete(batchUpdateStores[BatchUpdateTypeUserQuota], id)
		} else {
			batchUpdateStores[BatchUpdateTypeUserQuota][id] = remaining
		}
	}
	for id, delta := range candidate.Payload.TokenQuota {
		remaining := batchUpdateStores[BatchUpdateTypeTokenQuota][id] - delta
		if remaining == 0 {
			delete(batchUpdateStores[BatchUpdateTypeTokenQuota], id)
		} else {
			batchUpdateStores[BatchUpdateTypeTokenQuota][id] = remaining
		}
	}
	quotaBalancePendingGeneration = nil
	return nil
}

func checkedLegacyBalanceTarget(current int, delta int) (int, error) {
	current64, delta64 := int64(current), int64(delta)
	if (delta64 > 0 && current64 > quotaMutationMaxInt64-delta64) || (delta64 < 0 && current64 < -quotaMutationMaxInt64-1-delta64) {
		return 0, fmt.Errorf("legacy balance batch arithmetic overflow")
	}
	target := current64 + delta64
	if target < int64(common.MinQuota) || target > int64(common.MaxQuota) {
		return 0, fmt.Errorf("legacy balance batch result exceeds int32 boundary")
	}
	return int(target), nil
}

func applyQuotaBalanceBatchGeneration(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return applyQuotaBalanceBatchGenerationWithDB(DB.WithContext(ctx), id)
}

func applyQuotaBalanceBatchGenerationWithDB(db *gorm.DB, id int64) error {
	quotaBalanceFlushInflight.Add(1)
	defer quotaBalanceFlushInflight.Add(-1)
	generationKey := ""
	err := db.Transaction(func(tx *gorm.DB) error {
		state, err := GetQuotaWriterEpochState(tx)
		if err != nil {
			return err
		}
		if QuotaWriterMode(state.Mode) != QuotaWriterModeLegacy {
			return ErrLegacyQuotaWriterModeDisabled
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		var generation QuotaBalanceBatchDrain
		if err := lockForUpdate(tx).Where("id = ?", id).First(&generation).Error; err != nil {
			return err
		}
		if generation.State == quotaBalanceBatchDrainApplied || generation.State == quotaBalanceBatchDrainCancelled {
			return nil
		}
		if !generation.CacheApplied {
			return fmt.Errorf("quota balance generation cache mutation is not confirmed")
		}
		fingerprint, err := quotaBalanceBatchPayloadFingerprint(generation.Payload)
		if err != nil || fingerprint != generation.PayloadFingerprint {
			return fmt.Errorf("quota balance generation payload fingerprint mismatch")
		}
		generationKey = generation.GenerationKey
		if generation.WriterEpoch != state.Epoch || (generation.State != quotaBalanceBatchDrainPending && generation.State != quotaBalanceBatchDrainFailed && generation.State != quotaBalanceBatchDrainApplying) {
			return ErrQuotaWriterEpochMismatch
		}
		if generation.State != quotaBalanceBatchDrainApplying {
			result := tx.Model(&QuotaBalanceBatchDrain{}).Where("id = ? AND state IN ?", generation.ID, []string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainFailed}).
				Update("state", quotaBalanceBatchDrainApplying)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
			generation.State = quotaBalanceBatchDrainApplying
		}
		for userID, delta := range generation.Payload.UserQuota {
			var user User
			if err := lockForUpdate(tx).Where("id = ?", userID).First(&user).Error; err != nil {
				return err
			}
			target, err := checkedLegacyBalanceTarget(user.Quota, delta)
			if err != nil {
				return err
			}
			result := tx.Table("users").Where("id = ? AND quota = ? AND quota_version = ?", user.Id, user.Quota, user.QuotaVersion).
				Updates(map[string]interface{}{"quota": target, "quota_version": user.QuotaVersion + 1})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
		}
		for tokenID, delta := range generation.Payload.TokenQuota {
			var token Token
			if err := lockForUpdate(tx).Where("id = ?", tokenID).First(&token).Error; err != nil {
				return err
			}
			remain, err := checkedLegacyBalanceTarget(token.RemainQuota, delta)
			if err != nil {
				return err
			}
			used, err := checkedLegacyBalanceTarget(token.UsedQuota, -delta)
			if err != nil || used < 0 {
				return fmt.Errorf("legacy token batch used quota is invalid")
			}
			result := tx.Table("tokens").Where("id = ? AND remain_quota = ? AND used_quota = ? AND quota_version = ?", token.Id, token.RemainQuota, token.UsedQuota, token.QuotaVersion).
				Updates(map[string]interface{}{"remain_quota": remain, "used_quota": used, "accessed_time": now, "quota_version": token.QuotaVersion + 1})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrAccountQuotaMutationCASLost
			}
		}
		result := tx.Model(&QuotaBalanceBatchDrain{}).Where("id = ? AND state = ?", generation.ID, quotaBalanceBatchDrainApplying).
			Updates(map[string]interface{}{"state": quotaBalanceBatchDrainApplied, "attempts": generation.Attempts + 1, "last_error": "", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAccountQuotaMutationCASLost
		}
		return nil
	})
	if err != nil {
		message := err.Error()
		if len(message) > 4096 {
			message = message[:4096]
		}
		result := db.Model(&QuotaBalanceBatchDrain{}).
			Where("id = ? AND cache_applied = ? AND state IN ?", id, true,
				[]string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainApplying, quotaBalanceBatchDrainFailed}).
			Updates(map[string]interface{}{"state": quotaBalanceBatchDrainFailed, "last_error": message, "updated_at": GetDBTimestamp(), "attempts": gorm.Expr("attempts + ?", 1)})
		if result.Error != nil {
			common.SysLog("failed to persist quota balance generation error: " + result.Error.Error())
		}
	}
	if err == nil && common.RedisEnabled && common.RDB != nil && generationKey != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_ = common.RDB.Del(ctx, quotaBalanceJournalRedisKey(generationKey)).Err()
		cancel()
	}
	return err
}

func flushQuotaBalanceBatchGenerations() {
	if DB == nil || !DB.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
		return
	}
	if common.RedisEnabled && common.RDB != nil {
		if err := recoverQuotaBalanceCompensations(); err != nil {
			common.SysLog("failed to recover quota balance compensation: " + err.Error())
		}
		if err := recoverQuotaBalanceBatchPreparations(); err != nil {
			common.SysLog("failed to recover quota balance batch preparation: " + err.Error())
		}
	}
	var generations []QuotaBalanceBatchDrain
	if err := DB.Where("cache_applied = ? AND state IN ?", true, []string{quotaBalanceBatchDrainPending, quotaBalanceBatchDrainFailed, quotaBalanceBatchDrainApplying}).Order("id ASC").Limit(100).Find(&generations).Error; err != nil {
		common.SysLog("failed to list quota balance batch drains: " + err.Error())
		return
	}
	for i := range generations {
		if err := applyQuotaBalanceBatchGeneration(generations[i].ID); err != nil {
			common.SysLog("failed to apply quota balance batch drain: " + err.Error())
			return
		}
	}
	if _, err := cleanupQuotaBalanceBatchGenerations(DB, GetDBTimestamp()); err != nil {
		common.SysLog("failed to cleanup quota balance batch drains: " + err.Error())
	}
}

func quotaBalanceBatchDrainAudit(db *gorm.DB) (pending int64, inflight bool, err error) {
	inflight = quotaBalanceFlushInflight.Load() != 0
	if db == nil || !db.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
		return 0, inflight, nil
	}
	err = db.Model(&QuotaBalanceBatchDrain{}).Where("state NOT IN ?", []string{quotaBalanceBatchDrainApplied, quotaBalanceBatchDrainCancelled}).Count(&pending).Error
	return pending, inflight, err
}

func cleanupQuotaBalanceBatchGenerations(db *gorm.DB, now int64) (int64, error) {
	cutoff := now - quotaBalanceGenerationRetention
	var ids []int64
	if err := db.Model(&QuotaBalanceBatchDrain{}).
		Where("state IN ? AND updated_at < ?", []string{quotaBalanceBatchDrainApplied, quotaBalanceBatchDrainCancelled}, cutoff).
		Order("id ASC").Limit(quotaBalanceCleanupBatchSize).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	var deleted int64
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("generation_id IN ?", ids).Delete(&QuotaBalanceBatchSubject{}).Error; err != nil {
			return err
		}
		result := tx.Where("id IN ? AND state IN ?", ids, []string{quotaBalanceBatchDrainApplied, quotaBalanceBatchDrainCancelled}).Delete(&QuotaBalanceBatchDrain{})
		deleted = result.RowsAffected
		return result.Error
	})
	return deleted, err
}

func InitializeQuotaBalanceBatchDrainsWithDB(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if !db.Migrator().HasTable(&QuotaBalanceBatchDrain{}) || !db.Migrator().HasTable(&QuotaWorkCursor{}) {
		return nil
	}
	baseCtx := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		baseCtx = db.Statement.Context
	}
	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	defer cancel()
	deletedTotal := int64(0)
	for deletedTotal < quotaBalanceMigrationRunBudget {
		deleted, err := cleanupQuotaBalanceBatchGenerations(db.WithContext(ctx), GetDBTimestamp())
		if err != nil {
			return err
		}
		deletedTotal += deleted
		if deleted < quotaBalanceCleanupBatchSize {
			break
		}
	}
	workCursor, err := loadQuotaWorkCursor(ctx, db, quotaWorkCursorBalanceMigration)
	if err != nil {
		return err
	}
	if workCursor.Complete {
		pending, pendingErr := quotaBalanceBackfillPending(ctx, db)
		if pendingErr != nil {
			return pendingErr
		}
		if !pending {
			return nil
		}
		workCursor.Complete = false
		workCursor.LastID = 0
		if err := saveQuotaWorkCursor(ctx, db, workCursor); err != nil {
			return err
		}
	}
	db = db.WithContext(ctx)
	cursor := workCursor.LastID
	processed := int(deletedTotal)
	for processed < quotaBalanceMigrationRunBudget {
		var generations []QuotaBalanceBatchDrain
		remaining := quotaBalanceMigrationRunBudget - processed
		batchSize := min(quotaBalanceCleanupBatchSize, remaining)
		if err := db.Where("id > ?", cursor).Order("id ASC").Limit(batchSize).Find(&generations).Error; err != nil {
			return err
		}
		if quotaBalanceMigrationBatchHook != nil {
			quotaBalanceMigrationBatchHook(len(generations))
		}
		if len(generations) == 0 {
			workCursor.Complete = true
			workCursor.LastID = cursor
			return saveQuotaWorkCursor(ctx, db, workCursor)
		}
		for index := range generations {
			generation := &generations[index]
			cursor = generation.ID
			legacySchema := generation.SchemaVersion < quotaBalanceBatchDrainSchemaVersion
			updates := map[string]interface{}{}
			if legacySchema {
				updates["schema_version"] = quotaBalanceBatchDrainSchemaVersion
			}
			if generation.LockVersion < 1 {
				updates["lock_version"] = int64(1)
			}
			if !generation.CacheApplied && (generation.SubjectKind == quotaBalanceSubjectKindUnknown || generation.SubjectID == 0) {
				kind, id, subjectErr := quotaBalancePayloadSubject(generation.Payload)
				if subjectErr != nil {
					updates["state"] = quotaBalanceBatchDrainUnknown
					updates["last_error"] = "subject locator backfill failed: " + subjectErr.Error()
				} else {
					storedKind, encodeErr := quotaBalanceStoredSubjectKind(kind)
					if encodeErr != nil {
						return encodeErr
					}
					updates["subject_kind"] = storedKind
					updates["subject_id"] = id
				}
			}
			if len(generation.Payload.TokenQuota) > 0 && len(generation.Payload.TokenCacheKeys) != len(generation.Payload.TokenQuota) {
				locators, err := quotaBalanceTokenCacheLocators(db, generation.Payload.TokenQuota)
				if err != nil {
					if !legacySchema && !generation.CacheApplied {
						updates["state"] = quotaBalanceBatchDrainUnknown
						updates["last_error"] = "token cache locator backfill failed: " + err.Error()
					}
				} else {
					generation.Payload.TokenCacheKeys = locators
					updates["payload"] = generation.Payload
				}
			}
			if len(generation.PayloadFingerprint) != 64 || updates["payload"] != nil {
				fingerprint, err := quotaBalanceBatchPayloadFingerprint(generation.Payload)
				if err != nil {
					return err
				}
				updates["payload_fingerprint"] = fingerprint
				if legacySchema {
					updates["cache_applied"] = true
				}
			}
			if len(updates) > 0 {
				if err := db.Model(&QuotaBalanceBatchDrain{}).Where("id = ?", generation.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
			if err := ensureQuotaBalanceBatchSubjects(db, generation); err != nil {
				return err
			}
			processed++
		}
		workCursor.LastID = cursor
		if err := saveQuotaWorkCursor(ctx, db, workCursor); err != nil {
			return err
		}
	}
	var nextGeneration QuotaBalanceBatchDrain
	result := db.Select("id").Where("id > ?", cursor).Limit(1).Find(&nextGeneration)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		workCursor.Complete = true
		return saveQuotaWorkCursor(ctx, db, workCursor)
	}
	return ErrQuotaWorkIncomplete
}
