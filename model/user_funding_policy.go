package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
	"gorm.io/gorm"
)

const UserFundingStateOptionKey = "user_funding_state"

var (
	ErrUserFundingUnavailable          = errors.New("user funding unavailable")
	ErrUserFundingRuntimeStale         = errors.New("user funding runtime snapshot is stale")
	ErrUserFundingWebhookAcknowledge   = errors.New("user funding webhook acknowledged without settlement")
	ErrUserFundingStateEpochExhausted  = errors.New("user funding state epoch exhausted")
	ErrUserFundingInvalidPersistedMode = errors.New("invalid persisted user funding mode")
)

type UserFundingStateSnapshot struct {
	Mode                           operation_setting.UserFundingMode `json:"mode"`
	Epoch                          uint64                            `json:"epoch"`
	RetirementCutoffEpoch          uint64                            `json:"retirement_cutoff_epoch,omitempty"`
	RetirementTopUpIDCutoff        int                               `json:"retirement_topup_id_cutoff,omitempty"`
	RetirementSubscriptionIDCutoff int                               `json:"retirement_subscription_id_cutoff,omitempty"`
	Valid                          bool                              `json:"-"`
}

type UserFundingRequestSnapshot struct {
	State UserFundingStateSnapshot
	Ready bool
}

func failClosedUserFundingState(epoch uint64) UserFundingStateSnapshot {
	return UserFundingStateSnapshot{
		Mode:  operation_setting.UserFundingModeDisabled,
		Epoch: epoch,
		Valid: false,
	}
}

func normalizeUserFundingState(state UserFundingStateSnapshot) UserFundingStateSnapshot {
	mode, err := operation_setting.NormalizeUserFundingMode(state.Mode)
	if err != nil {
		return failClosedUserFundingState(state.Epoch)
	}
	state.Mode = mode
	if state.RetirementTopUpIDCutoff < 0 || state.RetirementSubscriptionIDCutoff < 0 {
		return failClosedUserFundingState(state.Epoch)
	}
	switch mode {
	case operation_setting.UserFundingModeEnabled, operation_setting.UserFundingModeDisabled:
		if state.RetirementCutoffEpoch != 0 || state.RetirementTopUpIDCutoff != 0 || state.RetirementSubscriptionIDCutoff != 0 {
			return failClosedUserFundingState(state.Epoch)
		}
	case operation_setting.UserFundingModeRetirement:
		if state.RetirementCutoffEpoch == 0 || state.RetirementCutoffEpoch > state.Epoch {
			return failClosedUserFundingState(state.Epoch)
		}
	}
	state.Valid = true
	return state
}

func encodeUserFundingState(state UserFundingStateSnapshot) (string, error) {
	state.Valid = false
	encoded, err := common.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func decodeUserFundingState(raw string) UserFundingStateSnapshot {
	var state UserFundingStateSnapshot
	if err := common.UnmarshalJsonStr(raw, &state); err != nil {
		return failClosedUserFundingState(0)
	}
	return normalizeUserFundingState(state)
}

func validateUserFundingStateProjectionTx(tx *gorm.DB, state UserFundingStateSnapshot) (UserFundingStateSnapshot, error) {
	var modeOption Option
	if err := optionKeyQuery(tx, operation_setting.UserFundingModeOptionKey).First(&modeOption).Error; err == nil {
		mode, normalizeErr := operation_setting.NormalizeUserFundingMode(operation_setting.UserFundingMode(modeOption.Value))
		if normalizeErr != nil || mode != state.Mode {
			return failClosedUserFundingState(state.Epoch), nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return failClosedUserFundingState(state.Epoch), err
	}

	var epochOption Option
	if err := optionKeyQuery(tx, operation_setting.UserFundingEpochOptionKey).First(&epochOption).Error; err == nil {
		epoch, parseErr := strconv.ParseUint(epochOption.Value, 10, 64)
		if parseErr != nil || epoch != state.Epoch {
			return failClosedUserFundingState(state.Epoch), nil
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return failClosedUserFundingState(state.Epoch), err
	}
	return state, nil
}

func legacyUserFundingStateTx(tx *gorm.DB) (UserFundingStateSnapshot, error) {
	state := UserFundingStateSnapshot{
		Mode:  operation_setting.UserFundingModeEnabled,
		Epoch: 0,
		Valid: true,
	}
	// Older installations may have persisted only SelfUseModeEnabled. A
	// missing funding-state row must not turn a self-use deployment back on.
	var selfUseOption Option
	selfUseErr := optionKeyQuery(tx, "SelfUseModeEnabled").First(&selfUseOption).Error
	if selfUseErr != nil && !errors.Is(selfUseErr, gorm.ErrRecordNotFound) {
		return failClosedUserFundingState(0), selfUseErr
	}
	if selfUseErr == nil {
		selfUseModeEnabled, parseErr := strconv.ParseBool(strings.TrimSpace(selfUseOption.Value))
		if parseErr != nil {
			return failClosedUserFundingState(0), nil
		}
		if selfUseModeEnabled {
			state.Mode = operation_setting.UserFundingModeDisabled
		}
	}
	var modeOption Option
	modeErr := optionKeyQuery(tx, operation_setting.UserFundingModeOptionKey).First(&modeOption).Error
	if modeErr != nil && !errors.Is(modeErr, gorm.ErrRecordNotFound) {
		return failClosedUserFundingState(0), modeErr
	}
	if modeErr == nil {
		mode, err := operation_setting.NormalizeUserFundingMode(operation_setting.UserFundingMode(modeOption.Value))
		if err != nil {
			return failClosedUserFundingState(0), nil
		}
		state.Mode = mode
	}

	var epochOption Option
	epochErr := optionKeyQuery(tx, operation_setting.UserFundingEpochOptionKey).First(&epochOption).Error
	if epochErr != nil && !errors.Is(epochErr, gorm.ErrRecordNotFound) {
		return failClosedUserFundingState(0), epochErr
	}
	if epochErr == nil {
		epoch, err := strconv.ParseUint(epochOption.Value, 10, 64)
		if err != nil {
			return failClosedUserFundingState(0), nil
		}
		state.Epoch = epoch
	}
	return normalizeUserFundingState(state), nil
}

func readUserFundingStateTx(tx *gorm.DB, lockAndEnsure bool) (UserFundingStateSnapshot, *Option, error) {
	if tx == nil {
		return failClosedUserFundingState(0), nil, gorm.ErrInvalidDB
	}
	var option Option
	query := optionKeyQuery(tx, UserFundingStateOptionKey)
	if lockAndEnsure {
		query = lockForUpdate(query)
	}
	err := query.First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state, legacyErr := legacyUserFundingStateTx(tx)
		if legacyErr != nil {
			return failClosedUserFundingState(0), nil, legacyErr
		}
		if !lockAndEnsure {
			return state, nil, nil
		}
		raw, encodeErr := encodeUserFundingState(state)
		if encodeErr != nil {
			return failClosedUserFundingState(state.Epoch), nil, encodeErr
		}
		option = Option{Key: UserFundingStateOptionKey, Value: raw}
		if createErr := tx.Create(&option).Error; createErr != nil {
			return failClosedUserFundingState(state.Epoch), nil, createErr
		}
		return state, &option, nil
	}
	if err != nil {
		return failClosedUserFundingState(0), nil, err
	}
	if lockAndEnsure {
		// SQLite has no SELECT FOR UPDATE. A no-op UPDATE obtains the writer
		// reservation while MySQL/PostgreSQL retain their row lock.
		if err := optionKeyQuery(tx.Model(&Option{}), UserFundingStateOptionKey).
			UpdateColumn("value", gorm.Expr("value")).Error; err != nil {
			return failClosedUserFundingState(0), nil, err
		}
	}
	state, projectionErr := validateUserFundingStateProjectionTx(tx, decodeUserFundingState(option.Value))
	return state, &option, projectionErr
}

func GetUserFundingStateSnapshot() (UserFundingStateSnapshot, error) {
	var state UserFundingStateSnapshot
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		state, _, err = readUserFundingStateTx(tx, true)
		return err
	})
	if err != nil {
		return failClosedUserFundingState(0), err
	}
	return state, nil
}

func GetUserFundingRequestSnapshot() (UserFundingRequestSnapshot, error) {
	state, err := GetUserFundingStateSnapshot()
	if err != nil {
		return UserFundingRequestSnapshot{State: failClosedUserFundingState(0)}, err
	}
	local := operation_setting.GetUserFundingSetting()
	ready := state.Valid && local.Mode == state.Mode && local.Epoch == state.Epoch
	return UserFundingRequestSnapshot{State: state, Ready: ready}, nil
}

func expectedUserFundingEpoch(provided []uint64) uint64 {
	if len(provided) > 0 {
		return provided[0]
	}
	return operation_setting.GetUserFundingSetting().Epoch
}

func requireUserFundingEnabledTx(tx *gorm.DB, expectedEpoch uint64) (UserFundingStateSnapshot, error) {
	state, _, err := readUserFundingStateTx(tx, true)
	if err != nil {
		return state, err
	}
	if !state.Valid || state.Mode != operation_setting.UserFundingModeEnabled || state.Epoch != expectedEpoch {
		return state, ErrUserFundingUnavailable
	}
	return state, nil
}

func latestFundingOrderIDTx[T any](tx *gorm.DB) (int, error) {
	var row struct {
		Id int
	}
	err := tx.Model(new(T)).Select("id").Order("id desc").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return row.Id, err
}

func nextUserFundingStateFromCurrentTx(tx *gorm.DB, current UserFundingStateSnapshot, targetMode operation_setting.UserFundingMode) (UserFundingStateSnapshot, error) {
	mode, err := operation_setting.NormalizeUserFundingMode(targetMode)
	if err != nil {
		return failClosedUserFundingState(0), err
	}
	if current.Epoch == ^uint64(0) {
		return current, ErrUserFundingStateEpochExhausted
	}
	next := current
	next.Mode = mode
	next.Epoch++
	next.Valid = true

	switch mode {
	case operation_setting.UserFundingModeEnabled, operation_setting.UserFundingModeDisabled:
		next.RetirementCutoffEpoch = 0
		next.RetirementTopUpIDCutoff = 0
		next.RetirementSubscriptionIDCutoff = 0
	case operation_setting.UserFundingModeRetirement:
		if current.Mode == operation_setting.UserFundingModeEnabled {
			next.RetirementCutoffEpoch = current.Epoch
			next.RetirementTopUpIDCutoff, err = latestFundingOrderIDTx[TopUp](tx)
			if err != nil {
				return current, err
			}
			next.RetirementSubscriptionIDCutoff, err = latestFundingOrderIDTx[SubscriptionOrder](tx)
			if err != nil {
				return current, err
			}
		}
	}
	return next, nil
}

func saveUserFundingStateTx(tx *gorm.DB, option *Option, state UserFundingStateSnapshot) error {
	if tx == nil || option == nil {
		return gorm.ErrInvalidData
	}
	raw, err := encodeUserFundingState(state)
	if err != nil {
		return err
	}
	if err := optionKeyQuery(tx.Model(&Option{}), UserFundingStateOptionKey).Update("value", raw).Error; err != nil {
		return err
	}
	for key, value := range map[string]string{
		operation_setting.UserFundingModeOptionKey:  string(state.Mode),
		operation_setting.UserFundingEpochOptionKey: strconv.FormatUint(state.Epoch, 10),
	} {
		stored := Option{Key: key}
		if err := optionKeyQuery(tx, key).FirstOrCreate(&stored, Option{Key: key}).Error; err != nil {
			return err
		}
		if err := optionKeyQuery(tx.Model(&Option{}), key).Update("value", value).Error; err != nil {
			return err
		}
	}
	return nil
}

func InitializeUserFundingStateTx(tx *gorm.DB, mode operation_setting.UserFundingMode) (UserFundingStateSnapshot, error) {
	normalized, err := operation_setting.NormalizeUserFundingMode(mode)
	if err != nil {
		return failClosedUserFundingState(0), err
	}
	state := UserFundingStateSnapshot{Mode: normalized, Epoch: 1, Valid: true}
	raw, err := encodeUserFundingState(state)
	if err != nil {
		return state, err
	}
	for key, value := range map[string]string{
		UserFundingStateOptionKey:                   raw,
		operation_setting.UserFundingModeOptionKey:  string(state.Mode),
		operation_setting.UserFundingEpochOptionKey: strconv.FormatUint(state.Epoch, 10),
	} {
		stored := Option{Key: key, Value: value}
		if err := optionKeyQuery(tx, key).Assign("value", value).FirstOrCreate(&stored).Error; err != nil {
			return state, err
		}
	}
	return state, nil
}

func PublishUserFundingState(state UserFundingStateSnapshot) error {
	if !state.Valid {
		state = failClosedUserFundingState(state.Epoch)
	}
	return operation_setting.PublishUserFundingSnapshot(state.Mode, state.Epoch)
}

type UserFundingOrderKind uint8

const (
	UserFundingOrderUnknown UserFundingOrderKind = iota
	UserFundingOrderTopUp
	UserFundingOrderSubscription
)

type UserFundingWebhookAction uint8

const (
	UserFundingWebhookAcknowledge UserFundingWebhookAction = iota
	UserFundingWebhookSettle
)

type UserFundingWebhookDecision struct {
	action   UserFundingWebhookAction
	kind     UserFundingOrderKind
	mode     operation_setting.UserFundingMode
	epoch    uint64
	tradeNo  string
	provider string
}

func (decision UserFundingWebhookDecision) Action() UserFundingWebhookAction { return decision.action }
func (decision UserFundingWebhookDecision) Kind() UserFundingOrderKind       { return decision.kind }
func (decision UserFundingWebhookDecision) Mode() operation_setting.UserFundingMode {
	return decision.mode
}
func (decision UserFundingWebhookDecision) Epoch() uint64    { return decision.epoch }
func (decision UserFundingWebhookDecision) TradeNo() string  { return decision.tradeNo }
func (decision UserFundingWebhookDecision) Provider() string { return decision.provider }
func (decision UserFundingWebhookDecision) ShouldSettle() bool {
	return decision.action == UserFundingWebhookSettle
}

func userFundingOrderEligible(state UserFundingStateSnapshot, kind UserFundingOrderKind, id int, epoch uint64) bool {
	if !state.Valid {
		return false
	}
	switch state.Mode {
	case operation_setting.UserFundingModeEnabled:
		return epoch <= state.Epoch
	case operation_setting.UserFundingModeRetirement:
		if epoch > 0 {
			return epoch <= state.RetirementCutoffEpoch
		}
		switch kind {
		case UserFundingOrderTopUp:
			return id > 0 && id <= state.RetirementTopUpIDCutoff
		case UserFundingOrderSubscription:
			return id > 0 && id <= state.RetirementSubscriptionIDCutoff
		}
	}
	return false
}

func DecideUserFundingWebhook(tradeNo, provider string) (UserFundingWebhookDecision, error) {
	decision := UserFundingWebhookDecision{
		action:   UserFundingWebhookAcknowledge,
		kind:     UserFundingOrderUnknown,
		tradeNo:  tradeNo,
		provider: provider,
	}
	if tradeNo == "" || provider == "" {
		return decision, nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		state, _, err := readUserFundingStateTx(tx, false)
		if err != nil {
			return err
		}
		decision.mode = state.Mode
		decision.epoch = state.Epoch
		if !state.Valid || state.Mode == operation_setting.UserFundingModeDisabled {
			return nil
		}
		local := operation_setting.GetUserFundingSetting()
		if local.Mode != state.Mode || local.Epoch != state.Epoch {
			return ErrUserFundingRuntimeStale
		}

		var subscription SubscriptionOrder
		subscriptionErr := tx.Where("trade_no = ?", tradeNo).First(&subscription).Error
		if subscriptionErr == nil {
			if subscription.PaymentProvider == provider && subscription.Status == common.TopUpStatusPending &&
				userFundingOrderEligible(state, UserFundingOrderSubscription, subscription.Id, subscription.FundingEpoch) {
				decision.action = UserFundingWebhookSettle
				decision.kind = UserFundingOrderSubscription
			}
			return nil
		}
		if !errors.Is(subscriptionErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("query subscription order for funding webhook: %w", subscriptionErr)
		}

		var topUp TopUp
		topUpErr := tx.Where("trade_no = ?", tradeNo).First(&topUp).Error
		if topUpErr == nil {
			if topUp.PaymentProvider == provider && topUp.Status == common.TopUpStatusPending &&
				userFundingOrderEligible(state, UserFundingOrderTopUp, topUp.Id, topUp.FundingEpoch) {
				decision.action = UserFundingWebhookSettle
				decision.kind = UserFundingOrderTopUp
			}
			return nil
		}
		if errors.Is(topUpErr, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("query top-up order for funding webhook: %w", topUpErr)
	})
	return decision, err
}

func validateUserFundingWebhookDecisionTx(
	state UserFundingStateSnapshot,
	decision *UserFundingWebhookDecision,
	kind UserFundingOrderKind,
	id int,
	orderEpoch uint64,
	tradeNo string,
	provider string,
) error {
	if decision == nil {
		return nil // explicit trusted/admin paths intentionally bypass webhook validation
	}
	if !decision.ShouldSettle() || decision.kind != kind || decision.tradeNo != tradeNo || decision.provider != provider ||
		decision.mode != state.Mode || decision.epoch != state.Epoch {
		return ErrUserFundingWebhookAcknowledge
	}
	if !userFundingOrderEligible(state, kind, id, orderEpoch) {
		return ErrUserFundingWebhookAcknowledge
	}
	return nil
}

func userFundingStateForWebhookTx(tx *gorm.DB, decision *UserFundingWebhookDecision) (UserFundingStateSnapshot, error) {
	if decision == nil {
		return UserFundingStateSnapshot{}, nil
	}
	state, _, err := readUserFundingStateTx(tx, true)
	return state, err
}
