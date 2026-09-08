package operation_setting

import (
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var defaultPaymentSetting = PaymentSetting{
	AmountOptions:  []int{10, 20, 50, 100, 200, 500},
	AmountDiscount: map[int]float64{},
}

// paymentSettingGeneration is immutable after publication. Amount options
// and discounts are collections, and the compliance fields must be read from
// one generation so eligibility never observes a partially updated setting.
type paymentSettingGeneration struct {
	setting PaymentSetting
}

// managedPaymentSetting owns the synchronized runtime snapshot registered
// with the generic config manager.
type managedPaymentSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[paymentSettingGeneration]
}

func clonePaymentSetting(setting PaymentSetting) PaymentSetting {
	clone := PaymentSetting{
		ComplianceConfirmed:    setting.ComplianceConfirmed,
		ComplianceTermsVersion: setting.ComplianceTermsVersion,
		ComplianceConfirmedAt:  setting.ComplianceConfirmedAt,
		ComplianceConfirmedBy:  setting.ComplianceConfirmedBy,
		ComplianceConfirmedIP:  setting.ComplianceConfirmedIP,
	}
	if setting.AmountOptions != nil {
		clone.AmountOptions = append([]int{}, setting.AmountOptions...)
	}
	if setting.AmountDiscount != nil {
		clone.AmountDiscount = make(map[int]float64, len(setting.AmountDiscount))
		for amount, discount := range setting.AmountDiscount {
			clone.AmountDiscount[amount] = discount
		}
	}
	return clone
}

func newManagedPaymentSetting(initial PaymentSetting) *managedPaymentSetting {
	state := &managedPaymentSetting{}
	state.current.Store(&paymentSettingGeneration{setting: clonePaymentSetting(initial)})
	return state
}

func (s *managedPaymentSetting) snapshot() PaymentSetting {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return clonePaymentSetting(current.setting)
		}
	}
	return clonePaymentSetting(defaultPaymentSetting)
}

func (s *managedPaymentSetting) candidate(values map[string]string) (PaymentSetting, error) {
	candidate := s.snapshot()
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return PaymentSetting{}, err
	}
	return candidate, nil
}

func (s *managedPaymentSetting) ExportConfigMap() (map[string]string, error) {
	setting := s.snapshot()
	return config.ConfigToMap(&setting)
}

func (s *managedPaymentSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedPaymentSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&paymentSettingGeneration{setting: clonePaymentSetting(candidate)})
	return nil
}

var paymentSettingState = newManagedPaymentSetting(defaultPaymentSetting)

var _ config.ValidatingMapConfig = (*managedPaymentSetting)(nil)

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", paymentSettingState)
}

func GetPaymentSetting() *PaymentSetting {
	setting := paymentSettingState.snapshot()
	return &setting
}

func IsPaymentComplianceConfirmed() bool {
	setting := paymentSettingState.snapshot()
	return setting.ComplianceConfirmed &&
		setting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
