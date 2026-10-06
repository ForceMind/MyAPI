package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

const (
	// AdminQuotaAdjustment 操作枚举：加/减/覆盖目标用户钱包额度。
	AdminQuotaAdjustmentOperationAdd      = "add"
	AdminQuotaAdjustmentOperationSubtract = "subtract"
	AdminQuotaAdjustmentOperationOverride = "override"

	// AdminQuotaAdjustmentStatusPending 是事务内的瞬时状态；已提交的记录始终为 succeeded。
	AdminQuotaAdjustmentStatusPending   = "pending"
	AdminQuotaAdjustmentStatusSucceeded = "succeeded"

	adminQuotaAdjustmentMaxRequestIdLength = 64
)

// ErrAdminQuotaAdjustmentConflict 表示 request id 已被不同参数使用，
// 或记录停在无法重放的状态。
var ErrAdminQuotaAdjustmentConflict = errors.New("admin quota adjustment conflicts with an existing request")

// AdminQuotaAdjustment 是一次管理员额度调整的持久操作身份。
// (target_user_id, request_id) 唯一索引使客户端重试安全：
// 重放已 succeeded 的请求直接返回记录结果而不重复写余额；
// 用不同参数复用同一 request id 判为冲突。
type AdminQuotaAdjustment struct {
	Id                 int    `json:"id" gorm:"primaryKey;autoIncrement"`
	AdminUserId        int    `json:"admin_user_id" gorm:"not null;index"`
	TargetUserId       int    `json:"target_user_id" gorm:"not null;index;uniqueIndex:uidx_admin_quota_adjustment_request,priority:1"`
	RequestId          string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex:uidx_admin_quota_adjustment_request,priority:2"`
	Operation          string `json:"operation" gorm:"type:varchar(16);not null"`
	Delta              int64  `json:"delta" gorm:"type:bigint;not null"`
	TargetQuota        int64  `json:"target_quota" gorm:"type:bigint;not null"`
	AppliedQuotaBefore int64  `json:"applied_quota_before" gorm:"type:bigint;not null"`
	AppliedQuotaAfter  int64  `json:"applied_quota_after" gorm:"type:bigint;not null"`
	Status             string `json:"status" gorm:"type:varchar(16);not null"`
	CreatedAt          int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt          int64  `json:"updated_at" gorm:"bigint"`
}

func (AdminQuotaAdjustment) TableName() string {
	return "admin_quota_adjustments"
}

func normalizeAdminQuotaAdjustmentInput(adminUserId int, targetUserId int, operation string, value int, requestId string) (string, string, error) {
	if adminUserId <= 0 || targetUserId <= 0 {
		return "", "", errors.New("invalid admin or target user id")
	}
	operation = strings.ToLower(strings.TrimSpace(operation))
	switch operation {
	case AdminQuotaAdjustmentOperationAdd, AdminQuotaAdjustmentOperationSubtract:
		if value <= 0 {
			return "", "", errors.New("调整额度必须大于 0！")
		}
	case AdminQuotaAdjustmentOperationOverride:
		if value < 0 {
			return "", "", errors.New("目标额度不能为负数！")
		}
	default:
		return "", "", errors.New("无效的额度调整操作！")
	}
	if value > common.MaxQuota {
		return "", "", fmt.Errorf("调整额度超出系统上限 %d！", common.MaxQuota)
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" || len(requestId) > adminQuotaAdjustmentMaxRequestIdLength {
		return "", "", errors.New("无效的调整请求标识！")
	}
	return operation, requestId, nil
}

// adminQuotaAdjustmentReplay 在事务内按 (target_user_id, request_id) 解析重放。
// 返回 (existing, handled, err)：handled 为 true 时调用方直接返回 existing。
func adminQuotaAdjustmentReplay(tx *gorm.DB, targetUserId int, requestId string, operation string, value int) (*AdminQuotaAdjustment, bool, error) {
	var existing AdminQuotaAdjustment
	findErr := tx.Where("target_user_id = ? AND request_id = ?", targetUserId, requestId).First(&existing).Error
	switch {
	case findErr == nil:
		if existing.Operation != operation {
			return nil, false, fmt.Errorf("%w: request id reused with a different operation", ErrAdminQuotaAdjustmentConflict)
		}
		sameParams := false
		if operation == AdminQuotaAdjustmentOperationOverride {
			sameParams = existing.TargetQuota == int64(value)
		} else {
			wantDelta := int64(value)
			if operation == AdminQuotaAdjustmentOperationSubtract {
				wantDelta = -wantDelta
			}
			sameParams = existing.Delta == wantDelta
		}
		if !sameParams {
			return nil, false, fmt.Errorf("%w: request id reused with different parameters", ErrAdminQuotaAdjustmentConflict)
		}
		if existing.Status == AdminQuotaAdjustmentStatusSucceeded {
			return &existing, true, nil
		}
		return nil, false, fmt.Errorf("%w: request id recorded in an unrecoverable state", ErrAdminQuotaAdjustmentConflict)
	case !errors.Is(findErr, gorm.ErrRecordNotFound):
		return nil, false, findErr
	}
	return nil, false, nil
}

// AdminAdjustUserQuota 应用一次管理员额度调整（add/subtract/override）。
// 三个管理端点共用：单事务内查重放（succeeded 同参数零写成功、异参数冲突）→
// lockForUpdate 锁目标用户（同时在锁内做二次查重，使并发重复提交收敛为一次应用）→
// 建单 → add/subtract 经模式分发，override 在锁内计算 delta=target−current 后走同一分发：
//   - authoritative：receipt 内核，键 "admin-adjust:{targetUserId}:{requestId}"，
//     MutationType "admin_adjust"，ReasonCode 按操作区分；
//   - legacy：保持迁移前直写语义（增减用表达式、覆盖直设，减可透支）；
//   - bridge 及其余：fail-closed。
//
// 记录 before/after 后标记 succeeded。提交后按模式投影 receipt 或同步缓存。
// 返回 (adjustment, replayed, err)。
func AdminAdjustUserQuota(adminUserId int, targetUserId int, operation string, value int, requestId string) (*AdminQuotaAdjustment, bool, error) {
	operation, requestId, err := normalizeAdminQuotaAdjustmentInput(adminUserId, targetUserId, operation, value, requestId)
	if err != nil {
		return nil, false, err
	}

	now := common.GetTimestamp()
	var adjustment *AdminQuotaAdjustment
	var quotaReceipt *UserQuotaMutationReceipt
	authoritative := false
	replayed := false

	err = DB.Transaction(func(tx *gorm.DB) error {
		if existing, handled, replayErr := adminQuotaAdjustmentReplay(tx, targetUserId, requestId, operation, value); handled || replayErr != nil {
			adjustment = existing
			replayed = handled
			return replayErr
		}

		// 先锁目标用户行：并发重复提交在此串行，后到的请求在锁内二次查重时
		// 看到已 succeeded 的记录并按重放返回，保证恰好应用一次。
		var user User
		if err := lockForUpdate(tx).Where("id = ?", targetUserId).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("目标用户不存在！")
			}
			return err
		}
		if existing, handled, replayErr := adminQuotaAdjustmentReplay(tx, targetUserId, requestId, operation, value); handled || replayErr != nil {
			adjustment = existing
			replayed = handled
			return replayErr
		}

		delta := int64(value)
		targetQuota := int64(0)
		switch operation {
		case AdminQuotaAdjustmentOperationSubtract:
			delta = -delta
		case AdminQuotaAdjustmentOperationOverride:
			targetQuota = int64(value)
			delta = int64(value) - int64(user.Quota)
		}

		record := AdminQuotaAdjustment{
			AdminUserId:  adminUserId,
			TargetUserId: targetUserId,
			RequestId:    requestId,
			Operation:    operation,
			Delta:        delta,
			TargetQuota:  targetQuota,
			Status:       AdminQuotaAdjustmentStatusPending,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}

		mode, err := businessUserQuotaWriterMode(tx)
		if err != nil {
			return err
		}
		quotaAfter := int64(user.Quota) + delta
		switch mode {
		case QuotaWriterModeAuthoritative:
			if delta != 0 {
				receipt, _, mutateErr := mutateUserQuotaAuthoritative(tx, UserQuotaMutationInput{
					UserID:           targetUserId,
					Delta:            delta,
					MutationType:     "admin_adjust",
					BusinessEventKey: fmt.Sprintf("admin-adjust:%d:%s", targetUserId, requestId),
					ReasonCode:       "admin_adjust_" + operation,
					OperatorUserID:   adminUserId,
					Metadata:         map[string]interface{}{"operation": operation},
				})
				if mutateErr != nil {
					return mutateErr
				}
				quotaReceipt = receipt
				quotaAfter = int64(receipt.QuotaAfter)
			}
			authoritative = true
		case QuotaWriterModeLegacy:
			// 保持迁移前直写语义：add/subtract 用表达式增减（减可透支），
			// override 直设目标值；delta == 0 时余额无变化，跳过写入。
			if delta != 0 {
				updateErr := func() error {
					if operation == AdminQuotaAdjustmentOperationOverride {
						return tx.Model(&User{}).Where("id = ?", targetUserId).Update("quota", value).Error
					}
					return tx.Model(&User{}).Where("id = ?", targetUserId).
						Update("quota", gorm.Expr("quota + ?", delta)).Error
				}()
				if updateErr != nil {
					return updateErr
				}
			}
		default:
			// bridge 及其余状态：fail-closed，不做静默 legacy 直写。
			return ErrDurableQuotaWriterModeDisabled
		}

		record.AppliedQuotaBefore = int64(user.Quota)
		record.AppliedQuotaAfter = quotaAfter
		if err := tx.Model(&AdminQuotaAdjustment{}).Where("id = ?", record.Id).Updates(map[string]interface{}{
			"status":               AdminQuotaAdjustmentStatusSucceeded,
			"applied_quota_before": record.AppliedQuotaBefore,
			"applied_quota_after":  record.AppliedQuotaAfter,
			"updated_at":           now,
		}).Error; err != nil {
			return err
		}
		record.Status = AdminQuotaAdjustmentStatusSucceeded
		adjustment = &record
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if adjustment == nil {
		return nil, false, ErrAdminQuotaAdjustmentConflict
	}
	if authoritative && quotaReceipt != nil {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else if !authoritative && !replayed && adjustment.Delta != 0 {
		if err := cacheIncrUserQuota(targetUserId, adjustment.Delta); err != nil {
			common.SysLog(fmt.Sprintf("failed to sync admin quota adjustment to user quota cache: %s", err.Error()))
		}
	}
	return adjustment, replayed, nil
}
