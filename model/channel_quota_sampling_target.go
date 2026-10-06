package model

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChannelQuotaSamplingTarget struct {
	ID                  int64  `json:"-" gorm:"primaryKey"`
	ChannelID           int    `json:"-" gorm:"not null;uniqueIndex:uidx_channel_quota_sampling_target,priority:1;index:idx_channel_quota_sampling_ready,priority:2"`
	SubjectRef          string `json:"-" gorm:"type:char(43);not null;uniqueIndex:uidx_channel_quota_sampling_target,priority:2"`
	IdentityQuality     string `json:"-" gorm:"type:varchar(24);not null"`
	ConfirmedSubjectRef string `json:"-" gorm:"type:char(43);not null;default:'';index"`
	Active              bool   `json:"-" gorm:"not null;index:idx_channel_quota_sampling_ready,priority:1"`
	LastAttemptAt       int64  `json:"-" gorm:"type:bigint;not null;default:0;index:idx_channel_quota_sampling_ready,priority:3"`
	LastResult          string `json:"-" gorm:"type:varchar(24);not null;default:''"`
	CreatedAt           int64  `json:"-" gorm:"type:bigint;not null"`
	UpdatedAt           int64  `json:"-" gorm:"type:bigint;not null"`
}

type ChannelQuotaSamplingIdentity struct {
	SubjectRef      string
	IdentityQuality string
}

type ChannelQuotaSamplingExpansion struct {
	ExpectedCursor int64
	NextCursor     int64
}

func (ChannelQuotaSamplingTarget) TableName() string { return "channel_quota_sampling_targets" }

func EnsureChannelQuotaSamplingTarget(ctx context.Context, db *gorm.DB, channelID int, identity ChannelQuotaSamplingIdentity) error {
	if db == nil || channelID <= 0 || !canonicalChannelQuotaIdentitySubject(identity.SubjectRef) ||
		(identity.IdentityQuality != ChannelQuotaIdentityQualityProviderConfirmed && identity.IdentityQuality != ChannelQuotaIdentityQualityCredentialScoped) {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now, err := taskRecoveryDBTimestamp(db.WithContext(ctx))
	if err != nil {
		return err
	}
	candidate := ChannelQuotaSamplingTarget{
		ChannelID: channelID, SubjectRef: identity.SubjectRef, IdentityQuality: identity.IdentityQuality,
		Active: true, CreatedAt: now, UpdatedAt: now,
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "channel_id"}, {Name: "subject_ref"}},
		DoUpdates: clause.Assignments(map[string]any{
			"identity_quality": identity.IdentityQuality, "active": true, "updated_at": now,
		}),
	}).Create(&candidate).Error
}

func SyncChannelQuotaSamplingTargets(ctx context.Context, db *gorm.DB, channelID int, expectedChannelKey string, complete bool, identities []ChannelQuotaSamplingIdentity, expansion ...ChannelQuotaSamplingExpansion) ([]ChannelQuotaSamplingTarget, error) {
	if db == nil || channelID <= 0 {
		return nil, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(expansion) > 1 || (len(expansion) == 1 && (expansion[0].ExpectedCursor < 0 || expansion[0].NextCursor < 0)) {
		return nil, gorm.ErrInvalidData
	}
	seen := make(map[string]ChannelQuotaSamplingIdentity, len(identities))
	for _, identity := range identities {
		identity.SubjectRef = strings.TrimSpace(identity.SubjectRef)
		identity.IdentityQuality = strings.TrimSpace(identity.IdentityQuality)
		if !canonicalChannelQuotaIdentitySubject(identity.SubjectRef) ||
			(identity.IdentityQuality != ChannelQuotaIdentityQualityProviderConfirmed && identity.IdentityQuality != ChannelQuotaIdentityQualityCredentialScoped) {
			return nil, ErrChannelQuotaIdentityRegistryCorrupt
		}
		seen[identity.SubjectRef] = identity
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	operation := func(tx *gorm.DB) error {
		if err := lockChannelQuotaSnapshotBatch(tx, channelID); err != nil {
			return err
		}
		var channel Channel
		if err := tx.Select("id", "key", "quota_sampling_cursor").Where("id = ?", channelID).First(&channel).Error; err != nil {
			return err
		}
		if channel.Key != expectedChannelKey {
			return errors.New("channel credentials changed during quota target sync")
		}
		if len(expansion) == 1 {
			advance := expansion[0]
			if channel.QuotaSamplingCursor != advance.ExpectedCursor {
				return errors.New("channel quota expansion changed during target sync")
			}
			updated := tx.Model(&Channel{}).Where("id = ? AND quota_sampling_cursor = ?", channelID, advance.ExpectedCursor).
				Where(map[string]any{"key": expectedChannelKey}).Update("quota_sampling_cursor", advance.NextCursor)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return errors.New("channel quota expansion changed during target sync")
			}
		}
		now, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			identity := seen[ref]
			candidate := ChannelQuotaSamplingTarget{ChannelID: channelID, SubjectRef: ref, IdentityQuality: identity.IdentityQuality, Active: true, CreatedAt: now, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "channel_id"}, {Name: "subject_ref"}},
				DoUpdates: clause.Assignments(map[string]any{"identity_quality": identity.IdentityQuality, "active": true, "updated_at": now}),
			}).Create(&candidate).Error; err != nil {
				return err
			}
		}
		query := tx.Model(&ChannelQuotaSamplingTarget{}).Where("channel_id = ? AND active = ?", channelID, true)
		if len(refs) > 0 {
			query = query.Where("subject_ref NOT IN ?", refs)
		}
		return query.Updates(map[string]any{"active": false, "updated_at": now}).Error
	}
	var err error
	for attempt := 0; attempt < channelQuotaSnapshotBatchTransactionAttempts; attempt++ {
		err = db.WithContext(ctx).Transaction(operation)
		if err == nil || !channelQuotaSnapshotBatchRetryable(db, err) {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
		}
	}
	if err != nil {
		return nil, err
	}
	if !complete && len(refs) == 0 {
		return []ChannelQuotaSamplingTarget{}, nil
	}
	var targets []ChannelQuotaSamplingTarget
	query := db.WithContext(ctx).Where("channel_id = ? AND active = ?", channelID, true)
	if !complete {
		query = query.Where("subject_ref IN ?", refs)
	}
	err = query.Order("last_attempt_at ASC, subject_ref ASC").Find(&targets).Error
	return targets, err
}

func MarkChannelQuotaSamplingTargetAttempt(ctx context.Context, db *gorm.DB, channelID int, subjectRef, result, confirmedSubjectRef string) error {
	if db == nil || channelID <= 0 || !canonicalChannelQuotaIdentitySubject(subjectRef) {
		return gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result = strings.TrimSpace(result)
	confirmedSubjectRef = strings.TrimSpace(confirmedSubjectRef)
	if result == "" || len(result) > 24 || confirmedSubjectRef != "" && !canonicalChannelQuotaIdentitySubject(confirmedSubjectRef) {
		return errors.New("invalid quota sampling target result")
	}
	operation := func(tx *gorm.DB) error {
		if err := lockChannelQuotaSnapshotBatch(tx, channelID); err != nil {
			return err
		}
		var target ChannelQuotaSamplingTarget
		if err := tx.Select("id", "confirmed_subject_ref").
			Where("channel_id = ? AND subject_ref = ? AND active = ?", channelID, subjectRef, true).
			First(&target).Error; err != nil {
			return err
		}
		databaseNow, err := taskRecoveryDBTimestamp(tx)
		if err != nil {
			return err
		}
		groupSubjectRef := confirmedSubjectRef
		if groupSubjectRef == "" {
			groupSubjectRef = target.ConfirmedSubjectRef
		}
		updates := map[string]any{"last_attempt_at": databaseNow, "last_result": result, "updated_at": databaseNow}
		if confirmedSubjectRef != "" {
			updates["confirmed_subject_ref"] = confirmedSubjectRef
		}
		updated := tx.Model(&ChannelQuotaSamplingTarget{}).
			Where("id = ? AND active = ?", target.ID, true).
			Updates(updates)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if groupSubjectRef == "" {
			return nil
		}
		return tx.Model(&ChannelQuotaSamplingTarget{}).
			Where("channel_id = ? AND active = ? AND confirmed_subject_ref = ? AND id <> ?", channelID, true, groupSubjectRef, target.ID).
			Updates(map[string]any{"last_attempt_at": databaseNow, "updated_at": databaseNow}).Error
	}
	var err error
	for attempt := 0; attempt < channelQuotaSnapshotBatchTransactionAttempts; attempt++ {
		err = db.WithContext(ctx).Transaction(operation)
		if err == nil || !channelQuotaSnapshotBatchRetryable(db, err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
		}
	}
	return err
}
