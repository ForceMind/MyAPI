package billing_setting

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/samber/lo"
)

const (
	BillingModeRatio      = "ratio"
	BillingModeTieredExpr = "tiered_expr"
	BillingModeField      = "billing_mode"
	BillingExprField      = "billing_expr"
)

// BillingSetting is managed by config.GlobalConfig.Register.
// DB keys: billing_setting.billing_mode, billing_setting.billing_expr
type BillingSetting struct {
	BillingMode map[string]string `json:"billing_mode"`
	BillingExpr map[string]string `json:"billing_expr"`
}

var defaultBillingSetting = BillingSetting{
	BillingMode: make(map[string]string),
	BillingExpr: make(map[string]string),
}

// BillingSnapshot exposes one immutable billing configuration generation to
// callers that must pair a model's mode and expression consistently.
type BillingSnapshot struct {
	billingMode map[string]string
	billingExpr map[string]string
}

func cloneBillingSettingMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneBillingSetting(setting BillingSetting) BillingSetting {
	return BillingSetting{
		BillingMode: cloneBillingSettingMap(setting.BillingMode),
		BillingExpr: cloneBillingSettingMap(setting.BillingExpr),
	}
}

func (s BillingSnapshot) GetBillingMode(model string) string {
	if mode, ok := s.billingMode[model]; ok {
		return mode
	}
	return BillingModeRatio
}

func (s BillingSnapshot) GetBillingExpr(model string) (string, bool) {
	expr, ok := s.billingExpr[model]
	return expr, ok
}

func (s BillingSnapshot) GetBillingModeCopy() map[string]string {
	return cloneBillingSettingMap(s.billingMode)
}

func (s BillingSnapshot) GetBillingExprCopy() map[string]string {
	return cloneBillingSettingMap(s.billingExpr)
}

type billingSettingGeneration struct {
	setting BillingSetting
}

// managedBillingSetting owns the synchronized runtime generation registered
// with the generic config manager. The two maps publish together but callers
// must not infer cross-key atomic persistence from that in-process property.
type managedBillingSetting struct {
	writeMutex sync.Mutex
	current    atomic.Pointer[billingSettingGeneration]
}

func newManagedBillingSetting(initial BillingSetting) *managedBillingSetting {
	state := &managedBillingSetting{}
	state.current.Store(&billingSettingGeneration{setting: cloneBillingSetting(initial)})
	return state
}

func (s *managedBillingSetting) snapshot() BillingSnapshot {
	if s != nil {
		if current := s.current.Load(); current != nil {
			return BillingSnapshot{
				billingMode: current.setting.BillingMode,
				billingExpr: current.setting.BillingExpr,
			}
		}
	}
	return BillingSnapshot{
		billingMode: defaultBillingSetting.BillingMode,
		billingExpr: defaultBillingSetting.BillingExpr,
	}
}

func (s *managedBillingSetting) candidate(values map[string]string) (BillingSetting, error) {
	snapshot := s.snapshot()
	candidate := BillingSetting{
		BillingMode: snapshot.billingMode,
		BillingExpr: snapshot.billingExpr,
	}
	if err := config.UpdateConfigFromMap(&candidate, values); err != nil {
		return BillingSetting{}, err
	}
	return candidate, nil
}

func (s *managedBillingSetting) ExportConfigMap() (map[string]string, error) {
	snapshot := s.snapshot()
	billingMode, err := common.Marshal(snapshot.billingMode)
	if err != nil {
		return nil, err
	}
	billingExpr, err := common.Marshal(snapshot.billingExpr)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"billing_mode": string(billingMode),
		"billing_expr": string(billingExpr),
	}, nil
}

func (s *managedBillingSetting) ValidateConfigMap(values map[string]string) error {
	_, err := s.candidate(values)
	return err
}

func (s *managedBillingSetting) UpdateConfigMap(values map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate, err := s.candidate(values)
	if err != nil {
		return err
	}
	s.current.Store(&billingSettingGeneration{setting: cloneBillingSetting(candidate)})
	return nil
}

var billingSettingState = newManagedBillingSetting(defaultBillingSetting)

var _ config.ValidatingMapConfig = (*managedBillingSetting)(nil)

func init() {
	config.GlobalConfig.Register("billing_setting", billingSettingState)
}

// ---------------------------------------------------------------------------
// Read accessors (hot path, must be fast)
// ---------------------------------------------------------------------------

func GetBillingMode(model string) string {
	return GetBillingSnapshot().GetBillingMode(model)
}

func GetBillingExpr(model string) (string, bool) {
	return GetBillingSnapshot().GetBillingExpr(model)
}

func GetBillingModeCopy() map[string]string {
	return GetBillingSnapshot().GetBillingModeCopy()
}

func GetBillingExprCopy() map[string]string {
	return GetBillingSnapshot().GetBillingExprCopy()
}

func GetBillingSnapshot() BillingSnapshot {
	return billingSettingState.snapshot()
}

func GetPricingSyncData(base map[string]any) map[string]any {
	snapshot := GetBillingSnapshot()
	extra := make(map[string]any, 2)
	if modes := snapshot.GetBillingModeCopy(); len(modes) > 0 {
		extra[BillingModeField] = modes
	}
	if exprs := snapshot.GetBillingExprCopy(); len(exprs) > 0 {
		extra[BillingExprField] = exprs
	}
	return lo.Assign(base, extra)
}

// ---------------------------------------------------------------------------
// Smoke test (called externally for validation before save)
// ---------------------------------------------------------------------------

func SmokeTestExpr(exprStr string) error {
	return smokeTestExpr(exprStr)
}

func smokeTestExpr(exprStr string) error {
	vectors := []billingexpr.TokenParams{
		{P: 0, C: 0, Len: 0},
		{P: 1000, C: 1000, Len: 1000},
		{P: 100000, C: 100000, Len: 100000},
		{P: 1000000, C: 1000000, Len: 1000000},
	}
	requests := []billingexpr.RequestInput{
		{},
		{
			Headers: map[string]string{
				"anthropic-beta": "fast-mode-2026-02-01",
			},
			Body: []byte(`{"service_tier":"fast","stream_options":{"include_usage":true},"messages":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21]}`),
		},
	}

	for _, v := range vectors {
		for _, request := range requests {
			result, _, err := billingexpr.RunExprWithRequest(exprStr, v, request)
			if err != nil {
				return fmt.Errorf("vector {p=%g, c=%g}: run failed: %w", v.P, v.C, err)
			}
			if result < 0 {
				return fmt.Errorf("vector {p=%g, c=%g}: result %f < 0", v.P, v.C, result)
			}
		}
	}
	return nil
}
