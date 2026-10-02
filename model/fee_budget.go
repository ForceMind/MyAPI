package model

import (
	"errors"
	"regexp"

	"github.com/shopspring/decimal"
)

var ErrFeeBudgetInvalid = errors.New("invalid exact USD budget amount")
var ErrFeeBudgetExceeded = errors.New("USD budget is insufficient")
var feeUSDPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

// Amounts remain decimal strings across SQL and JSON. Never round each request
// to cents, convert through quota/float64, or depend on global division precision.
// A bounded encoding also prevents attacker-controlled decimal exponents.
func NormalizeFeeBudgetUSD(value string) (string, error) {
	if len(value) == 0 || len(value) > 128 || !feeUSDPattern.MatchString(value) {
		return "", ErrFeeBudgetInvalid
	}
	amount, err := decimal.NewFromString(value)
	if err != nil || amount.IsNegative() {
		return "", ErrFeeBudgetInvalid
	}
	normalized := amount.String()
	if len(normalized) > 128 {
		return "", ErrFeeBudgetInvalid
	}
	return normalized, nil
}

func feeBudgetDecimal(value string) (decimal.Decimal, error) {
	normalized, err := NormalizeFeeBudgetUSD(value)
	if err != nil || normalized != value {
		return decimal.Zero, ErrFeeBudgetInvalid
	}
	return decimal.NewFromString(normalized)
}

type FeeBudgetPolicyInput struct {
	Enabled  bool   `json:"enabled"`
	LimitUSD string `json:"limit_usd"`
}

func validateFeeBudgetState(budget *TokenBudget) error {
	for _, value := range []string{budget.FeeLimitUSD, budget.FeeUsedUSD, budget.FeeReservedUSD} {
		if _, err := feeBudgetDecimal(value); err != nil {
			return err
		}
	}
	return nil
}

func feeBudgetFits(limit, used, reservation string) error {
	max, err := feeBudgetDecimal(limit)
	if err != nil {
		return err
	}
	consumed, err := feeBudgetDecimal(used)
	if err != nil {
		return err
	}
	pending, err := feeBudgetDecimal(reservation)
	if err != nil {
		return err
	}
	if consumed.Add(pending).GreaterThan(max) {
		return ErrFeeBudgetExceeded
	}
	return nil
}

func feeBudgetAdd(used, actual string) (string, error) {
	left, err := feeBudgetDecimal(used)
	if err != nil {
		return "", err
	}
	right, err := feeBudgetDecimal(actual)
	if err != nil {
		return "", err
	}
	return NormalizeFeeBudgetUSD(left.Add(right).String())
}

func CheckFeeBudgetAdmission(budget *TokenBudget) error {
	if budget == nil || !budget.FeeEnabled {
		return nil
	}
	limit, err := feeBudgetDecimal(budget.FeeLimitUSD)
	if err != nil {
		return err
	}
	used, err := feeBudgetDecimal(budget.FeeUsedUSD)
	if err != nil {
		return err
	}
	if used.GreaterThanOrEqual(limit) {
		return ErrFeeBudgetExceeded
	}
	return nil
}
