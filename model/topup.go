package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TopUp struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Amount          int64   `json:"amount"`
	Money           float64 `json:"money"`
	TradeNo         string  `json:"trade_no" gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string  `json:"payment_method" gorm:"type:varchar(50)"`
	PaymentProvider string  `json:"payment_provider" gorm:"type:varchar(50);default:''"`
	CreateTime      int64   `json:"create_time"`
	CompleteTime    int64   `json:"complete_time"`
	Status          string  `json:"status"`
	FundingEpoch    uint64  `json:"funding_epoch" gorm:"not null;default:0;index"`
}

const (
	PaymentMethodStripe       = "stripe"
	PaymentMethodCreem        = "creem"
	PaymentMethodWaffo        = "waffo"
	PaymentMethodWaffoPancake = "waffo_pancake"
	PaymentMethodBalance      = "balance"
)

const (
	PaymentProviderEpay         = "epay"
	PaymentProviderStripe       = "stripe"
	PaymentProviderCreem        = "creem"
	PaymentProviderWaffo        = "waffo"
	PaymentProviderWaffoPancake = "waffo_pancake"
	PaymentProviderBalance      = "balance"
)

var (
	ErrPaymentMethodMismatch   = errors.New("payment method mismatch")
	ErrTopUpNotFound           = errors.New("topup not found")
	ErrTopUpStatusInvalid      = errors.New("topup status invalid")
	ErrInvalidTopUpQuota       = errors.New("invalid top-up quota")
	ErrTopUpQuotaLimitExceeded = errors.New("top-up quota limit exceeded")
)

func (topUp *TopUp) Insert(expectedEpoch ...uint64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		state, err := requireUserFundingEnabledTx(tx, expectedUserFundingEpoch(expectedEpoch))
		if err != nil {
			return err
		}
		topUp.FundingEpoch = state.Epoch
		return tx.Create(topUp).Error
	})
}

func topUpQuotaMaxCurrent(creditedQuota int) (int, error) {
	if creditedQuota <= 0 || creditedQuota >= common.MaxQuota {
		return 0, ErrInvalidTopUpQuota
	}
	return common.MaxQuota - 1 - creditedQuota, nil
}

// ValidateTopUpQuotaCapacity performs the user-facing pre-payment check. The
// settlement path repeats the same invariant with an atomic conditional
// update, because the wallet balance can change after checkout creation.
func ValidateTopUpQuotaCapacity(userId int, creditedQuota int) error {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return err
	}

	var user User
	if err := DB.Select("quota").Where("id = ?", userId).First(&user).Error; err != nil {
		return err
	}
	if user.Quota > maxCurrentQuota {
		return ErrTopUpQuotaLimitExceeded
	}
	return nil
}

// creditTopUpQuota atomically enforces the int32 wallet ceiling while adding
// quota. Keeping the predicate and increment in one UPDATE prevents two
// concurrent callbacks from both passing a separate read/check.
func topUpBusinessEventKey(topUp *TopUp) (string, error) {
	if topUp == nil || strings.TrimSpace(topUp.TradeNo) == "" {
		return "", ErrTopUpNotFound
	}
	provider := strings.ToLower(strings.TrimSpace(topUp.PaymentProvider))
	if provider == "" {
		provider = "legacy:" + strings.ToLower(strings.TrimSpace(topUp.PaymentMethod))
	}
	digest := sha256.Sum256([]byte(provider + "\x00" + strings.TrimSpace(topUp.TradeNo)))
	return "topup:" + hex.EncodeToString(digest[:]), nil
}

func topUpQuotaMutationInput(topUp *TopUp, creditedQuota int) (UserQuotaMutationInput, error) {
	eventKey, err := topUpBusinessEventKey(topUp)
	if err != nil {
		return UserQuotaMutationInput{}, err
	}
	return UserQuotaMutationInput{
		UserID: topUp.UserId, Delta: int64(creditedQuota), MutationType: "topup", BusinessEventKey: eventKey,
		ReasonCode: "topup_credit", MaxQuotaExclusive: int64(common.MaxQuota),
		Metadata: map[string]interface{}{"topup_id": topUp.Id, "provider": topUp.PaymentProvider, "trade_no": topUp.TradeNo},
	}, nil
}

func replayCompletedTopUpCredit(tx *gorm.DB, topUp *TopUp, creditedQuota int) (*UserQuotaMutationReceipt, error) {
	mode, err := businessUserQuotaWriterMode(tx)
	if err != nil || mode != QuotaWriterModeAuthoritative {
		return nil, err
	}
	input, err := topUpQuotaMutationInput(topUp, creditedQuota)
	if err != nil {
		return nil, err
	}
	return replayBusinessUserQuotaMutationAuthoritative(tx, input)
}

func creditTopUpQuota(tx *gorm.DB, topUp *TopUp, creditedQuota int, updates map[string]interface{}) (*UserQuotaMutationReceipt, error) {
	maxCurrentQuota, err := topUpQuotaMaxCurrent(creditedQuota)
	if err != nil {
		return nil, err
	}
	input, err := topUpQuotaMutationInput(topUp, creditedQuota)
	if err != nil {
		return nil, err
	}
	receipt, replayed, authoritative, err := mutateBusinessUserQuotaIfAuthoritative(tx, input)
	if err != nil {
		if errors.Is(err, ErrUserQuotaUpperBoundExceeded) {
			return nil, ErrTopUpQuotaLimitExceeded
		}
		return nil, err
	}
	if authoritative {
		if !replayed && len(updates) > 0 {
			if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Updates(updates).Error; err != nil {
				return nil, err
			}
		}
		return receipt, nil
	}

	updateFields := make(map[string]interface{}, len(updates)+1)
	for key, value := range updates {
		updateFields[key] = value
	}
	updateFields["quota"] = gorm.Expr("quota + ?", creditedQuota)

	result := tx.Model(&User{}).
		Where("id = ? AND quota <= ?", topUp.UserId, maxCurrentQuota).
		Updates(updateFields)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 1 {
		return nil, nil
	}

	var count int64
	if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return nil, ErrTopUpQuotaLimitExceeded
}

func (topUp *TopUp) Update() error {
	var err error
	err = DB.Save(topUp).Error
	return err
}

func GetTopUpById(id int) *TopUp {
	var topUp *TopUp
	var err error
	err = DB.Where("id = ?", id).First(&topUp).Error
	if err != nil {
		return nil
	}
	return topUp
}

func GetTopUpByTradeNo(tradeNo string) *TopUp {
	topUp, _ := GetTopUpByTradeNoWithError(tradeNo)
	return topUp
}

// GetTopUpByTradeNoWithError distinguishes a missing order from a failed query
// so payment callbacks can request a retry after temporary database failures.
func GetTopUpByTradeNoWithError(tradeNo string) (*TopUp, error) {
	var topUp TopUp
	if err := DB.Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTopUpNotFound
		}
		return nil, fmt.Errorf("get topup order: %w", err)
	}
	return &topUp, nil
}

func UpdatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string, decision UserFundingWebhookDecision) error {
	return updatePendingTopUpStatus(tradeNo, expectedPaymentProvider, targetStatus, &decision)
}

func UpdatePendingTopUpStatusTrusted(tradeNo string, expectedPaymentProvider string, targetStatus string) error {
	return updatePendingTopUpStatus(tradeNo, expectedPaymentProvider, targetStatus, nil)
}

func RechargeEpay(tradeNo string, actualPaymentMethod string, callerIp string, decision UserFundingWebhookDecision) (bool, error) {
	return rechargeEpay(tradeNo, actualPaymentMethod, callerIp, &decision)
}

func RechargeEpayTrusted(tradeNo string, actualPaymentMethod string, callerIp string) (bool, error) {
	return rechargeEpay(tradeNo, actualPaymentMethod, callerIp, nil)
}

func Recharge(referenceId string, customerId string, callerIp string, decision UserFundingWebhookDecision) error {
	return rechargeStripe(referenceId, customerId, callerIp, &decision)
}

func RechargeTrusted(referenceId string, customerId string, callerIp string) error {
	return rechargeStripe(referenceId, customerId, callerIp, nil)
}

func RechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string, decision UserFundingWebhookDecision) error {
	return rechargeCreem(referenceId, customerEmail, customerName, callerIp, &decision)
}

func RechargeCreemTrusted(referenceId string, customerEmail string, customerName string, callerIp string) error {
	return rechargeCreem(referenceId, customerEmail, customerName, callerIp, nil)
}

func RechargeWaffo(tradeNo string, callerIp string, decision UserFundingWebhookDecision) error {
	return rechargeWaffo(tradeNo, callerIp, &decision)
}

func RechargeWaffoTrusted(tradeNo string, callerIp string) error {
	return rechargeWaffo(tradeNo, callerIp, nil)
}

func RechargeWaffoPancake(tradeNo string, decision UserFundingWebhookDecision) error {
	return rechargeWaffoPancake(tradeNo, &decision)
}

func RechargeWaffoPancakeTrusted(tradeNo string) error {
	return rechargeWaffoPancake(tradeNo, nil)
}

func updatePendingTopUpStatus(tradeNo string, expectedPaymentProvider string, targetStatus string, decision *UserFundingWebhookDecision) error {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		state, err := userFundingStateForWebhookTx(tx, decision)
		if err != nil {
			return err
		}
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if expectedPaymentProvider != "" && topUp.PaymentProvider != expectedPaymentProvider {
			return ErrPaymentMethodMismatch
		}
		if err := validateUserFundingWebhookDecisionTx(
			state, decision, UserFundingOrderTopUp,
			topUp.Id, topUp.FundingEpoch, topUp.TradeNo, topUp.PaymentProvider,
		); err != nil {
			return err
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		result := tx.Model(&TopUp{}).
			Where("id = ? AND status = ? AND payment_provider = ?", topUp.Id, common.TopUpStatusPending, topUp.PaymentProvider).
			Update("status", targetStatus)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrTopUpStatusInvalid
		}
		return nil
	})
}

// RechargeEpay 原子完成易支付订单：订单行锁、状态校验、成功更新与用户额度增加
// 在同一个事务内完成，因此同一订单的并发/重复回调（包括多实例部署下）最多充值一次。
// alreadyDone=true 表示订单此前已完成，本次为幂等重复回调。
// 进程内的 LockOrder 只是优化，正确性由本函数的数据库行锁保证。
func rechargeEpay(tradeNo string, actualPaymentMethod string, callerIp string, decision *UserFundingWebhookDecision) (alreadyDone bool, err error) {
	if tradeNo == "" {
		return false, errors.New("未提供支付单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var quotaToAdd int
	var quotaReceipt *UserQuotaMutationReceipt
	topUp := &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		state, stateErr := userFundingStateForWebhookTx(tx, decision)
		if stateErr != nil {
			return stateErr
		}
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.PaymentProvider != PaymentProviderEpay {
			return ErrPaymentMethodMismatch
		}
		if err := validateUserFundingWebhookDecisionTx(
			state, decision, UserFundingOrderTopUp,
			topUp.Id, topUp.FundingEpoch, topUp.TradeNo, topUp.PaymentProvider,
		); err != nil {
			return err
		}
		var quotaErr error
		quotaToAdd, quotaErr = common.QuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}
		if topUp.Status == common.TopUpStatusSuccess {
			quotaReceipt, err = replayCompletedTopUpCredit(tx, topUp, quotaToAdd)
			if err != nil {
				return err
			}
			alreadyDone = true
			return nil
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}
		if actualPaymentMethod != "" && topUp.PaymentMethod != actualPaymentMethod {
			topUp.PaymentMethod = actualPaymentMethod
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		quotaReceipt, err = creditTopUpQuota(tx, topUp, quotaToAdd, nil)
		return err
	})
	if err != nil {
		if !errors.Is(err, ErrTopUpNotFound) && !errors.Is(err, ErrPaymentMethodMismatch) && !errors.Is(err, ErrTopUpStatusInvalid) {
			common.SysError("epay topup failed: " + err.Error())
		}
		return false, err
	}
	if alreadyDone {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
		return true, nil
	}
	if quotaReceipt != nil {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else {
		syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, "epay topup")
	}

	common.SysLog(fmt.Sprintf("易支付充值成功 trade_no=%s user_id=%d quota_to_add=%d money=%.2f", topUp.TradeNo, topUp.UserId, quotaToAdd, topUp.Money))
	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentProviderEpay)
	return false, nil
}

func rechargeStripe(referenceId string, customerId string, callerIp string, decision *UserFundingWebhookDecision) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	var quotaReceipt *UserQuotaMutationReceipt
	var replayed bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		state, stateErr := userFundingStateForWebhookTx(tx, decision)
		if stateErr != nil {
			return stateErr
		}
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}

		if topUp.PaymentProvider != PaymentProviderStripe {
			return ErrPaymentMethodMismatch
		}
		if err := validateUserFundingWebhookDecisionTx(
			state, decision, UserFundingOrderTopUp,
			topUp.Id, topUp.FundingEpoch, topUp.TradeNo, topUp.PaymentProvider,
		); err != nil {
			return err
		}
		quota, err = common.QuotaFromDecimalStrict(
			decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}

		if topUp.Status == common.TopUpStatusSuccess {
			quotaReceipt, err = replayCompletedTopUpCredit(tx, topUp, quota)
			replayed = err == nil
			return err
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		quotaReceipt, err = creditTopUpQuota(tx, topUp, quota, map[string]interface{}{
			"stripe_customer": customerId,
		})
		return err
	})

	if err != nil {
		common.SysError("topup failed: " + err.Error())
		return fmt.Errorf("stripe topup: %w", err)
	}
	if replayed {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
		return nil
	}
	if quota == 0 {
		return nil // Successful replay: no second cache increment or log.
	}
	if quotaReceipt != nil {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else {
		syncCreditUserQuotaCache(topUp.UserId, quota, "stripe topup")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%d", logger.FormatQuota(quota), topUp.Amount), callerIp, topUp.PaymentMethod, PaymentMethodStripe)

	return nil
}

// topUpQueryWindowSeconds 限制充值记录查询的时间窗口（秒）。
const topUpQueryWindowSeconds int64 = 30 * 24 * 60 * 60

// topUpQueryCutoff 返回允许查询的最早 create_time（秒级 Unix 时间戳）。
func topUpQueryCutoff() int64 {
	return common.GetTimestamp() - topUpQueryWindowSeconds
}

func GetUserTopUps(userId int, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	cutoff := topUpQueryCutoff()

	// Get total count within transaction
	err = tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, cutoff).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated topups within same transaction
	err = tx.Where("user_id = ? AND create_time >= ?", userId, cutoff).Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// GetAllTopUps 获取全平台的充值记录（管理员使用，不限制时间窗口）
func GetAllTopUps(pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err = tx.Model(&TopUp{}).Count(&total).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return topups, total, nil
}

// searchTopUpCountHardLimit 搜索充值记录时 COUNT 的安全上限，
// 防止对超大表执行无界 COUNT 触发 DoS。
const searchTopUpCountHardLimit = 10000

// SearchUserTopUps 按订单号搜索某用户的充值记录
func SearchUserTopUps(userId int, keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{}).Where("user_id = ? AND create_time >= ?", userId, topUpQueryCutoff())
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// SearchAllTopUps 按订单号搜索全平台充值记录（管理员使用，不限制时间窗口）
func SearchAllTopUps(keyword string, pageInfo *common.PageInfo) (topups []*TopUp, total int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	query := tx.Model(&TopUp{})
	if keyword != "" {
		pattern, perr := sanitizeLikePattern(keyword)
		if perr != nil {
			tx.Rollback()
			return nil, 0, perr
		}
		query = query.Where("trade_no LIKE ? ESCAPE '!'", pattern)
	}

	if err = query.Limit(searchTopUpCountHardLimit).Count(&total).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to count search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&topups).Error; err != nil {
		tx.Rollback()
		common.SysError("failed to search topups: " + err.Error())
		return nil, 0, errors.New("搜索充值记录失败")
	}

	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}
	return topups, total, nil
}

// ManualCompleteTopUp 管理员手动完成订单并给用户充值
func ManualCompleteTopUp(tradeNo string, callerIp string) error {
	if tradeNo == "" {
		return errors.New("未提供订单号")
	}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	var userId int
	var quotaToAdd int
	var payMoney float64
	var paymentMethod string
	var quotaReceipt *UserQuotaMutationReceipt
	var replayed bool

	err := DB.Transaction(func(tx *gorm.DB) error {
		topUp := &TopUp{}
		// 行级锁，避免并发补单
		if err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error; err != nil {
			return errors.New("充值订单不存在")
		}

		// 计算应充值额度：
		// - Stripe 订单：Money 代表经分组倍率换算后的美元数量，直接 * QuotaPerUnit
		// - Creem 订单：Amount 已是最终额度，与 RechargeCreem 保持一致
		// - 其他订单（如易支付）：Amount 为美元数量，* QuotaPerUnit
		var quotaErr error
		switch topUp.PaymentProvider {
		case PaymentProviderStripe:
			quotaToAdd, quotaErr = common.QuotaFromDecimalStrict(
				decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		case PaymentProviderCreem:
			quotaToAdd, quotaErr = common.QuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))
		default:
			quotaToAdd, quotaErr = common.QuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
		}
		if quotaErr != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}
		if topUp.Status == common.TopUpStatusSuccess {
			var replayErr error
			quotaReceipt, replayErr = replayCompletedTopUpCredit(tx, topUp, quotaToAdd)
			replayed = replayErr == nil
			return replayErr
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("订单状态不是待支付，无法补单")
		}

		// 标记完成
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		// 增加用户额度（立即写库，保持一致性）
		var creditErr error
		quotaReceipt, creditErr = creditTopUpQuota(tx, topUp, quotaToAdd, nil)
		if creditErr != nil {
			return creditErr
		}

		userId = topUp.UserId
		payMoney = topUp.Money
		paymentMethod = topUp.PaymentMethod
		return nil
	})

	if err != nil {
		return err
	}
	if replayed {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
		return nil
	}

	// 事务外记录日志，避免阻塞
	if quotaToAdd == 0 {
		return nil
	}
	if quotaReceipt != nil {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else {
		syncCreditUserQuotaCache(userId, quotaToAdd, "manual topup")
	}
	RecordTopupLog(userId, fmt.Sprintf("管理员补单成功，充值金额: %v，支付金额：%f", logger.FormatQuota(quotaToAdd), payMoney), callerIp, paymentMethod, "admin")
	return nil
}
func rechargeCreem(referenceId string, customerEmail string, customerName string, callerIp string, decision *UserFundingWebhookDecision) (err error) {
	if referenceId == "" {
		return errors.New("未提供支付单号")
	}

	var quota int
	var quotaReceipt *UserQuotaMutationReceipt
	var replayed bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		state, stateErr := userFundingStateForWebhookTx(tx, decision)
		if stateErr != nil {
			return stateErr
		}
		err := lockForUpdate(tx).Where(refCol+" = ?", referenceId).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderCreem {
			return ErrPaymentMethodMismatch
		}
		if err := validateUserFundingWebhookDecisionTx(
			state, decision, UserFundingOrderTopUp,
			topUp.Id, topUp.FundingEpoch, topUp.TradeNo, topUp.PaymentProvider,
		); err != nil {
			return err
		}
		quota, err = common.QuotaFromDecimalStrict(decimal.NewFromInt(topUp.Amount))
		if err != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}

		if topUp.Status == common.TopUpStatusSuccess {
			quotaReceipt, err = replayCompletedTopUpCredit(tx, topUp, quota)
			replayed = err == nil
			return err
		}
		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		err = tx.Save(topUp).Error
		if err != nil {
			return err
		}

		// 构建更新字段，优先使用邮箱，如果邮箱为空则使用用户名
		updateFields := map[string]interface{}{}

		// 如果有客户邮箱，尝试更新用户邮箱（仅当用户邮箱为空时）
		if customerEmail != "" {
			// 先检查用户当前邮箱是否为空
			var user User
			err = tx.Where("id = ?", topUp.UserId).First(&user).Error
			if err != nil {
				return err
			}

			// 如果用户邮箱为空，则更新为支付时使用的邮箱
			if user.Email == "" {
				updateFields["email"] = customerEmail
			}
		}

		quotaReceipt, err = creditTopUpQuota(tx, topUp, quota, updateFields)
		return err
	})

	if err != nil {
		if errors.Is(err, ErrUserFundingWebhookAcknowledge) || errors.Is(err, ErrUserQuotaMutationConflict) {
			return err
		}
		common.SysError("creem topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if replayed {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
		return nil
	}
	if quota == 0 {
		return nil
	}
	if quotaReceipt != nil {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else {
		syncCreditUserQuotaCache(topUp.UserId, quota, "creem topup")
	}

	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用Creem充值成功，充值额度: %v，支付金额：%.2f", quota, topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodCreem)

	return nil
}

func rechargeWaffo(tradeNo string, callerIp string, decision *UserFundingWebhookDecision) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	var quotaReceipt *UserQuotaMutationReceipt
	var replayed bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		state, stateErr := userFundingStateForWebhookTx(tx, decision)
		if stateErr != nil {
			return stateErr
		}
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffo {
			return ErrPaymentMethodMismatch
		}
		if err := validateUserFundingWebhookDecisionTx(
			state, decision, UserFundingOrderTopUp,
			topUp.Id, topUp.FundingEpoch, topUp.TradeNo, topUp.PaymentProvider,
		); err != nil {
			return err
		}
		quotaToAdd, err = common.QuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		if topUp.Status == common.TopUpStatusSuccess {
			quotaReceipt, err = replayCompletedTopUpCredit(tx, topUp, quotaToAdd)
			replayed = err == nil
			return err
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		quotaReceipt, err = creditTopUpQuota(tx, topUp, quotaToAdd, nil)
		return err
	})

	if err != nil {
		if errors.Is(err, ErrUserFundingWebhookAcknowledge) || errors.Is(err, ErrUserQuotaMutationConflict) {
			return err
		}
		common.SysError("waffo topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if replayed {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
		return nil
	}
	if quotaReceipt != nil {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else if quotaToAdd > 0 {
		syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, "waffo topup")
	}

	if quotaToAdd > 0 {
		RecordTopupLog(topUp.UserId, fmt.Sprintf("Waffo充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, PaymentMethodWaffo)
	}

	return nil
}

func rechargeWaffoPancake(tradeNo string, decision *UserFundingWebhookDecision) (err error) {
	if tradeNo == "" {
		return errors.New("未提供支付单号")
	}

	var quotaToAdd int
	var quotaReceipt *UserQuotaMutationReceipt
	var replayed bool
	topUp := &TopUp{}

	refCol := "`trade_no`"
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		refCol = `"trade_no"`
	}

	err = DB.Transaction(func(tx *gorm.DB) error {
		state, stateErr := userFundingStateForWebhookTx(tx, decision)
		if stateErr != nil {
			return stateErr
		}
		err := lockForUpdate(tx).Where(refCol+" = ?", tradeNo).First(topUp).Error
		if err != nil {
			return errors.New("充值订单不存在")
		}

		if topUp.PaymentProvider != PaymentProviderWaffoPancake {
			return ErrPaymentMethodMismatch
		}
		if err := validateUserFundingWebhookDecisionTx(
			state, decision, UserFundingOrderTopUp,
			topUp.Id, topUp.FundingEpoch, topUp.TradeNo, topUp.PaymentProvider,
		); err != nil {
			return err
		}
		quotaToAdd, err = common.QuotaFromDecimalStrict(
			decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil || quotaToAdd <= 0 {
			return ErrInvalidTopUpQuota
		}

		if topUp.Status == common.TopUpStatusSuccess {
			quotaReceipt, err = replayCompletedTopUpCredit(tx, topUp, quotaToAdd)
			replayed = err == nil
			return err
		}

		if topUp.Status != common.TopUpStatusPending {
			return errors.New("充值订单状态错误")
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}

		quotaReceipt, err = creditTopUpQuota(tx, topUp, quotaToAdd, nil)
		return err
	})

	if err != nil {
		if errors.Is(err, ErrUserFundingWebhookAcknowledge) || errors.Is(err, ErrUserQuotaMutationConflict) {
			return err
		}
		common.SysError("waffo pancake topup failed: " + err.Error())
		return errors.New("充值失败，请稍后重试")
	}
	if replayed {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
		return nil
	}
	if quotaReceipt != nil {
		projectBusinessUserQuotaReceipt(DB, quotaReceipt)
	} else if quotaToAdd > 0 {
		syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, "waffo pancake topup")
	}

	if quotaToAdd > 0 {
		RecordLog(topUp.UserId, LogTypeTopup, fmt.Sprintf("Waffo Pancake充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(quotaToAdd), topUp.Money))
	}

	return nil
}
