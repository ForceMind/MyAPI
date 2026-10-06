package service

import (
	"errors"

	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
)

const UserFundingUnavailableCode = "user_funding_unavailable"

type UserFundingCapabilities struct {
	Mode                        operation_setting.UserFundingMode `json:"mode"`
	Epoch                       uint64                            `json:"epoch"`
	Ready                       bool                              `json:"ready"`
	CanTopUp                    bool                              `json:"can_top_up"`
	CanRedeem                   bool                              `json:"can_redeem"`
	CanTransferAffiliateRewards bool                              `json:"can_transfer_affiliate_rewards"`
	CanPurchaseSubscription     bool                              `json:"can_purchase_subscription"`
	CanViewFundingHistory       bool                              `json:"can_view_funding_history"`
}

type UserFundingSnapshot struct {
	State        model.UserFundingStateSnapshot
	Capabilities UserFundingCapabilities
}

func CurrentUserFundingSnapshot() UserFundingSnapshot {
	requestSnapshot, err := model.GetUserFundingRequestSnapshot()
	if err != nil {
		requestSnapshot = model.UserFundingRequestSnapshot{
			State: model.UserFundingStateSnapshot{
				Mode: operation_setting.UserFundingModeDisabled,
			},
		}
	}
	state := requestSnapshot.State
	enabled := requestSnapshot.Ready && state.Mode == operation_setting.UserFundingModeEnabled
	return UserFundingSnapshot{
		State: state,
		Capabilities: UserFundingCapabilities{
			Mode:                        state.Mode,
			Epoch:                       state.Epoch,
			Ready:                       requestSnapshot.Ready,
			CanTopUp:                    enabled,
			CanRedeem:                   enabled,
			CanTransferAffiliateRewards: enabled,
			CanPurchaseSubscription:     enabled,
			CanViewFundingHistory:       true,
		},
	}
}

func CurrentUserFundingCapabilities() UserFundingCapabilities {
	return CurrentUserFundingSnapshot().Capabilities
}

func UserFundingMutationsAvailable() bool {
	return CurrentUserFundingSnapshot().Capabilities.CanTopUp
}

type UserFundingWebhookDecision = model.UserFundingWebhookDecision
type UserFundingWebhookAction = model.UserFundingWebhookAction
type UserFundingOrderKind = model.UserFundingOrderKind

const (
	UserFundingWebhookAcknowledge = model.UserFundingWebhookAcknowledge
	UserFundingWebhookSettle      = model.UserFundingWebhookSettle
	UserFundingOrderUnknown       = model.UserFundingOrderUnknown
	UserFundingOrderTopUp         = model.UserFundingOrderTopUp
	UserFundingOrderSubscription  = model.UserFundingOrderSubscription
)

func DecideUserFundingWebhook(tradeNo, provider string) (UserFundingWebhookDecision, error) {
	return model.DecideUserFundingWebhook(tradeNo, provider)
}

func IsUserFundingWebhookAcknowledge(err error) bool {
	return errors.Is(err, model.ErrUserFundingWebhookAcknowledge)
}
