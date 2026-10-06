package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelOperationCreate = "channel.create"
	ChannelOperationCopy   = "channel.copy"
)

var (
	ErrChannelOperationConflict   = errors.New("channel operation key conflicts with a different request")
	ErrChannelOperationKeyInvalid = errors.New("channel operation key is invalid")
	channelOperationKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)
)

// ChannelRoutingOperation stores only completed operations. The row, channel
// mutation, ability mutation, result, and routing epoch commit atomically.
type ChannelRoutingOperation struct {
	OperationType string `json:"operation_type" gorm:"type:varchar(32);primaryKey"`
	OperationKey  string `json:"operation_key" gorm:"type:varchar(128);primaryKey"`
	Fingerprint   string `json:"fingerprint" gorm:"type:char(64);not null"`
	Result        string `json:"result" gorm:"type:text;not null"`
	CreatedAt     int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt     int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (ChannelRoutingOperation) TableName() string { return "channel_routing_operations" }

type ChannelOperationResult struct {
	IDs         []int    `json:"ids"`
	Names       []string `json:"names,omitempty"`
	SourceProxy string   `json:"source_proxy,omitempty"`
	Epoch       int64    `json:"epoch"`
}

func NormalizeChannelOperationKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	if !channelOperationKeyPattern.MatchString(key) {
		return "", ErrChannelOperationKeyInvalid
	}
	return key, nil
}

func FingerprintChannelOperation(value any) (string, error) {
	payload, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func executeChannelOperation(
	ctx context.Context,
	operationType string,
	operationKey string,
	fingerprint string,
	mutate func(tx *gorm.DB) (ChannelOperationResult, error),
) (result ChannelOperationResult, replayed bool, err error) {
	if DB == nil {
		return ChannelOperationResult{}, false, gorm.ErrInvalidDB
	}
	operationKey, err = NormalizeChannelOperationKey(operationKey)
	if err != nil {
		return ChannelOperationResult{}, false, err
	}
	if fingerprint == "" {
		return ChannelOperationResult{}, false, errors.New("channel operation fingerprint is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ensureChannelRoutingSchema(DB); err != nil {
		return ChannelOperationResult{}, false, err
	}

	for attempt := 0; attempt < 5; attempt++ {
		result = ChannelOperationResult{}
		replayed = false
		err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			tx = markChannelRoutingManagedTransaction(tx)
			if operationKey != "" {
				now := getDBTimestampTx(tx)
				candidate := ChannelRoutingOperation{
					OperationType: operationType,
					OperationKey:  operationKey,
					Fingerprint:   fingerprint,
					Result:        "",
					CreatedAt:     now,
					UpdatedAt:     now,
				}
				insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate)
				if insert.Error != nil {
					return insert.Error
				}
				if insert.RowsAffected == 0 {
					var existing ChannelRoutingOperation
					if err := lockForUpdate(tx).Where(
						"operation_type = ? AND operation_key = ?",
						operationType,
						operationKey,
					).First(&existing).Error; err != nil {
						return err
					}
					if existing.Fingerprint != fingerprint {
						return ErrChannelOperationConflict
					}
					if strings.TrimSpace(existing.Result) == "" {
						return errors.New("completed channel operation has no result")
					}
					if err := common.UnmarshalJsonStr(existing.Result, &result); err != nil {
						return err
					}
					replayed = true
					return nil
				}
			}

			var mutationErr error
			result, mutationErr = mutate(tx)
			if mutationErr != nil {
				return mutationErr
			}
			result.Epoch, mutationErr = advanceChannelRoutingEpoch(tx)
			if mutationErr != nil {
				return mutationErr
			}
			if operationKey == "" {
				return nil
			}
			resultJSON, mutationErr := common.Marshal(result)
			if mutationErr != nil {
				return mutationErr
			}
			update := tx.Model(&ChannelRoutingOperation{}).
				Where("operation_type = ? AND operation_key = ? AND fingerprint = ?", operationType, operationKey, fingerprint).
				Updates(map[string]any{
					"result":     string(resultJSON),
					"updated_at": getDBTimestampTx(tx),
				})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return errors.New("channel operation result was not persisted")
			}
			return nil
		})
		if err == nil {
			observeCommittedChannelRoutingEpoch(result.Epoch)
			return result, replayed, nil
		}
		if DB.Dialector.Name() != "sqlite" || !isRetryableSQLiteWriteError(err) || attempt == 4 {
			return ChannelOperationResult{}, false, err
		}
		select {
		case <-ctx.Done():
			return ChannelOperationResult{}, false, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 5 * time.Millisecond):
		}
	}
	return ChannelOperationResult{}, false, err
}

func BatchInsertChannelsWithOperation(
	ctx context.Context,
	channels []Channel,
	operationKey string,
	fingerprint string,
) (ChannelOperationResult, bool, error) {
	if len(channels) == 0 {
		return ChannelOperationResult{IDs: []int{}}, false, nil
	}
	return executeChannelOperation(ctx, ChannelOperationCreate, operationKey, fingerprint, func(tx *gorm.DB) (ChannelOperationResult, error) {
		attemptChannels := make([]Channel, len(channels))
		copy(attemptChannels, channels)
		for i := range attemptChannels {
			attemptChannels[i].Id = 0
			if attemptChannels[i].CreatedTime == 0 {
				attemptChannels[i].CreatedTime = common.GetTimestamp()
			}
		}
		for _, chunk := range lo.Chunk(attemptChannels, 50) {
			if err := tx.Create(&chunk).Error; err != nil {
				return ChannelOperationResult{}, err
			}
			for i := range chunk {
				if err := chunk[i].AddAbilities(tx); err != nil {
					return ChannelOperationResult{}, err
				}
			}
		}
		result := ChannelOperationResult{
			IDs:   make([]int, 0, len(attemptChannels)),
			Names: make([]string, 0, len(attemptChannels)),
		}
		for i := range attemptChannels {
			result.IDs = append(result.IDs, attemptChannels[i].Id)
			result.Names = append(result.Names, attemptChannels[i].Name)
		}
		return result, nil
	})
}

func CopyChannelWithOperation(
	ctx context.Context,
	sourceID int,
	suffix string,
	resetBalance bool,
	operationKey string,
	fingerprint string,
) (ChannelOperationResult, bool, error) {
	return executeChannelOperation(ctx, ChannelOperationCopy, operationKey, fingerprint, func(tx *gorm.DB) (ChannelOperationResult, error) {
		var origin Channel
		if err := lockForUpdate(tx).First(&origin, "id = ?", sourceID).Error; err != nil {
			return ChannelOperationResult{}, err
		}
		clone := origin
		clone.Id = 0
		clone.CreatedTime = common.GetTimestamp()
		clone.Name = origin.Name + suffix
		clone.TestTime = 0
		clone.ResponseTime = 0
		if resetBalance {
			clone.Balance = 0
			clone.UsedQuota = 0
		}
		if err := clone.ValidateSettings(); err != nil {
			return ChannelOperationResult{}, fmt.Errorf("invalid cloned channel settings: %w", err)
		}
		if err := tx.Create(&clone).Error; err != nil {
			return ChannelOperationResult{}, err
		}
		if err := clone.AddAbilities(tx); err != nil {
			return ChannelOperationResult{}, err
		}
		return ChannelOperationResult{
			IDs:         []int{clone.Id},
			Names:       []string{clone.Name},
			SourceProxy: origin.GetSetting().Proxy,
		}, nil
	})
}
