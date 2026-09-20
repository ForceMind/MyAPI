package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// AffQuotaTransferStatusPending is the transient state of a transfer record
	// inside its transaction; a committed record is always succeeded.
	AffQuotaTransferStatusPending   = "pending"
	AffQuotaTransferStatusSucceeded = "succeeded"

	affQuotaTransferMaxRequestIdLength = 64
)

// ErrAffQuotaTransferConflict is returned when a request id was already used
// with a different quota or ended in a state that cannot be replayed.
var ErrAffQuotaTransferConflict = errors.New("aff quota transfer conflicts with an existing request")

// InviteRewardGrant is the durable, idempotent record of an inviter reward.
// The (inviter_id, invitee_id) unique index guarantees a repeated
// finishInsert/FinalizeOAuthUserCreation can never credit the inviter twice.
type InviteRewardGrant struct {
	Id        int   `json:"id" gorm:"primaryKey;autoIncrement"`
	InviterId int   `json:"inviter_id" gorm:"not null;index;uniqueIndex:uidx_invite_reward_grant_pair,priority:1"`
	InviteeId int   `json:"invitee_id" gorm:"not null;uniqueIndex:uidx_invite_reward_grant_pair,priority:2"`
	Quota     int   `json:"quota" gorm:"not null"`
	CreatedAt int64 `json:"created_at" gorm:"bigint"`
}

func (InviteRewardGrant) TableName() string {
	return "invite_reward_grants"
}

// AffQuotaTransfer is the durable operation identity of one affiliate-to-wallet
// transfer attempt. The (user_id, request_id) unique index makes client
// retries safe: a replayed request id returns the recorded outcome instead of
// moving quota twice.
type AffQuotaTransfer struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int    `json:"user_id" gorm:"not null;index;uniqueIndex:uidx_aff_quota_transfer_request,priority:1"`
	RequestId string `json:"request_id" gorm:"type:varchar(64);not null;uniqueIndex:uidx_aff_quota_transfer_request,priority:2"`
	Quota     int    `json:"quota" gorm:"not null"`
	Status    string `json:"status" gorm:"type:varchar(16);not null"`
	CreatedAt int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt int64  `json:"updated_at" gorm:"bigint"`
}

func (AffQuotaTransfer) TableName() string {
	return "aff_quota_transfers"
}

// inviteUser credits the inviter reward exactly once per inviter/invitee pair.
// The grant row and the aggregate counters commit in the same transaction, so
// a duplicate invocation inserts no grant row and applies no increment.
func inviteUser(inviterId int, inviteeId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		grant := InviteRewardGrant{
			InviterId: inviterId,
			InviteeId: inviteeId,
			Quota:     common.QuotaForInviter,
			CreatedAt: time.Now().Unix(),
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		result = tx.Model(&User{}).Where("id = ?", inviterId).Updates(map[string]interface{}{
			"aff_count":   gorm.Expr("aff_count + ?", 1),
			"aff_quota":   gorm.Expr("aff_quota + ?", common.QuotaForInviter),
			"aff_history": gorm.Expr("aff_history + ?", common.QuotaForInviter),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// insertUserWithInitialQuotaTx applies the new-user quota inside the creation
// transaction. Authoritative mode creates the user with zero quota and records
// a durable mutation receipt in the same transaction; legacy/bridge keep the
// historical direct assignment. Mode detection failure fails closed and rolls
// back the registration.
func insertUserWithInitialQuotaTx(tx *gorm.DB, user *User) error {
	mode, err := businessUserQuotaWriterMode(tx)
	if err != nil {
		return err
	}
	authoritative := mode == QuotaWriterModeAuthoritative && common.QuotaForNewUser > 0
	if authoritative {
		user.Quota = 0
	} else {
		user.Quota = common.QuotaForNewUser
	}
	user.AffCode = common.GetRandomString(4)

	// 初始化用户设置，包括默认的边栏配置
	if user.Setting == "" {
		defaultSetting := dto.UserSetting{}
		// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
		user.SetSetting(defaultSetting)
	}

	if err := mapEmailConstraintError(tx.Create(user).Error); err != nil {
		return err
	}
	if !authoritative {
		return nil
	}
	_, _, err = mutateUserQuotaAuthoritative(tx, UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            int64(common.QuotaForNewUser),
		MutationType:     "system_init",
		BusinessEventKey: fmt.Sprintf("user_init:%d", user.Id),
		ReasonCode:       "new_user_grant",
	})
	return err
}

// postCommitUserQuotaWriterMode resolves the business writer mode in a short
// read-only transaction so post-commit rewards can pick their write path.
func postCommitUserQuotaWriterMode() (QuotaWriterMode, error) {
	var mode QuotaWriterMode
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		mode, err = businessUserQuotaWriterMode(tx)
		return err
	})
	return mode, err
}

// grantInviteeQuotaReward credits the invitee registration reward after the
// creation transaction commits. Authoritative mode writes a durable receipt
// keyed by the invitee id, making repeated post-commit hooks replay-safe;
// legacy/bridge keep the historical direct increase. Mode detection failure is
// logged and skipped (fail-closed, never a silent legacy write).
func grantInviteeQuotaReward(userId int) {
	if common.QuotaForInvitee <= 0 {
		return
	}
	mode, err := postCommitUserQuotaWriterMode()
	if err != nil {
		common.SysError(fmt.Sprintf("unable to resolve quota writer mode, skipped invitee reward for user %d: %s", userId, err.Error()))
		return
	}
	if mode == QuotaWriterModeAuthoritative {
		if _, err := MutateUserQuota(DB, UserQuotaMutationInput{
			UserID:           userId,
			Delta:            int64(common.QuotaForInvitee),
			MutationType:     "invite",
			BusinessEventKey: fmt.Sprintf("invite:invitee:%d", userId),
			ReasonCode:       "invite_reward",
		}); err != nil {
			common.SysError(fmt.Sprintf("failed to credit invitee reward for user %d: %s", userId, err.Error()))
		}
	} else {
		_ = IncreaseUserQuota(userId, common.QuotaForInvitee, true)
	}
	RecordLog(userId, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(common.QuotaForInvitee)))
}

// projectNewUserQuotaReceipt hydrates the quota projection for the durable
// initial-quota receipt after the enclosing creation transaction commits. It
// is a no-op in legacy/bridge mode or when no initial grant was recorded; the
// deterministic business event key makes the lookup exact.
func projectNewUserQuotaReceipt(userId int) {
	mode, err := postCommitUserQuotaWriterMode()
	if err != nil || mode != QuotaWriterModeAuthoritative {
		return
	}
	receipt, err := FindUserQuotaMutationReceiptByEventKey(DB, fmt.Sprintf("user_init:%d", userId))
	if err != nil || receipt == nil {
		return
	}
	projectBusinessUserQuotaReceipt(DB, receipt)
}

// TransferAffQuotaToQuota moves affiliate reward quota into the wallet
// balance. The (user_id, request_id) pair is the durable operation identity:
// replaying a succeeded request returns success without any write, and
// reusing the request id with a different quota is a conflict.
func (user *User) TransferAffQuotaToQuota(quota int, requestId string, expectedEpoch ...uint64) error {
	// 检查quota是否小于最小额度
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为%s！", logger.LogQuota(common.QuotaFromFloat(common.QuotaPerUnit)))
	}
	requestId = strings.TrimSpace(requestId)
	if requestId == "" || len(requestId) > affQuotaTransferMaxRequestIdLength {
		return errors.New("无效的转移请求标识！")
	}

	now := time.Now().Unix()
	var quotaReceipt *UserQuotaMutationReceipt
	var authoritative bool

	// 开始数据库事务
	tx := DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback() // 确保在函数退出时事务能回滚
	if _, err := requireUserFundingEnabledTx(tx, expectedUserFundingEpoch(expectedEpoch)); err != nil {
		return err
	}

	var transfer AffQuotaTransfer
	findErr := tx.Where("user_id = ? AND request_id = ?", user.Id, requestId).First(&transfer).Error
	switch {
	case findErr == nil:
		if transfer.Quota != quota {
			return fmt.Errorf("%w: request id reused with a different quota", ErrAffQuotaTransferConflict)
		}
		if transfer.Status == AffQuotaTransferStatusSucceeded {
			return nil
		}
		return fmt.Errorf("%w: request id recorded in an unrecoverable state", ErrAffQuotaTransferConflict)
	case !errors.Is(findErr, gorm.ErrRecordNotFound):
		return findErr
	}

	transfer = AffQuotaTransfer{
		UserId:    user.Id,
		RequestId: requestId,
		Quota:     quota,
		Status:    AffQuotaTransferStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := tx.Create(&transfer).Error; err != nil {
		return err
	}

	// 加锁查询用户以确保数据一致性
	if err := lockForUpdate(tx).First(user, user.Id).Error; err != nil {
		return err
	}

	// 再次检查用户的AffQuota是否足够
	if user.AffQuota < quota {
		return errors.New("邀请额度不足！")
	}

	// 更新用户额度
	user.AffQuota -= quota
	input := UserQuotaMutationInput{
		UserID:           user.Id,
		Delta:            int64(quota),
		MutationType:     "aff_transfer",
		BusinessEventKey: fmt.Sprintf("aff_transfer:%d:%s", user.Id, requestId),
		ReasonCode:       "aff_transfer_credit",
	}
	var err error
	quotaReceipt, _, authoritative, err = mutateBusinessUserQuotaIfAuthoritative(tx, input)
	if err != nil {
		return err
	}
	user.Quota += quota
	if authoritative {
		// The authoritative kernel already moved `quota` via its own locked
		// read + CAS; persist only the affiliate side here.
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Update("aff_quota", user.AffQuota).Error; err != nil {
			return err
		}
	} else {
		// 保存用户状态
		if err := tx.Save(user).Error; err != nil {
			return err
		}
	}
	if err := tx.Model(&AffQuotaTransfer{}).Where("id = ?", transfer.Id).Updates(map[string]interface{}{
		"status":     AffQuotaTransferStatusSucceeded,
		"updated_at": now,
	}).Error; err != nil {
		return err
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		return err
	}
	if authoritative {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else {
		syncCreditUserQuotaCache(user.Id, quota, "aff_transfer")
	}
	return nil
}
