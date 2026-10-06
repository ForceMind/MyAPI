package setting

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/operation_setting"
)

// PaymentOptionKey is a field owned by the payment runtime. The constants use
// the existing option names so a later option writer can bridge to this kernel
// without translating payment semantics here.
type PaymentOptionKey string

const (
	PaymentOptionPayAddress                  PaymentOptionKey = "PayAddress"
	PaymentOptionCustomCallbackAddress       PaymentOptionKey = "CustomCallbackAddress"
	PaymentOptionEpayID                      PaymentOptionKey = "EpayId"
	PaymentOptionEpayKey                     PaymentOptionKey = "EpayKey"
	PaymentOptionPrice                       PaymentOptionKey = "Price"
	PaymentOptionUSDExchangeRate             PaymentOptionKey = "USDExchangeRate"
	PaymentOptionMinTopUp                    PaymentOptionKey = "MinTopUp"
	PaymentOptionPayMethods                  PaymentOptionKey = "PayMethods"
	PaymentOptionStripeAPISecret             PaymentOptionKey = "StripeApiSecret"
	PaymentOptionStripeWebhookSecret         PaymentOptionKey = "StripeWebhookSecret"
	PaymentOptionStripePriceID               PaymentOptionKey = "StripePriceId"
	PaymentOptionStripeUnitPrice             PaymentOptionKey = "StripeUnitPrice"
	PaymentOptionStripeMinTopUp              PaymentOptionKey = "StripeMinTopUp"
	PaymentOptionStripePromotionCodesEnabled PaymentOptionKey = "StripePromotionCodesEnabled"
	PaymentOptionCreemAPIKey                 PaymentOptionKey = "CreemApiKey"
	PaymentOptionCreemProducts               PaymentOptionKey = "CreemProducts"
	PaymentOptionCreemTestMode               PaymentOptionKey = "CreemTestMode"
	PaymentOptionCreemWebhookSecret          PaymentOptionKey = "CreemWebhookSecret"
	PaymentOptionWaffoEnabled                PaymentOptionKey = "WaffoEnabled"
	PaymentOptionWaffoAPIKey                 PaymentOptionKey = "WaffoApiKey"
	PaymentOptionWaffoPrivateKey             PaymentOptionKey = "WaffoPrivateKey"
	PaymentOptionWaffoPublicCert             PaymentOptionKey = "WaffoPublicCert"
	PaymentOptionWaffoSandboxPublicCert      PaymentOptionKey = "WaffoSandboxPublicCert"
	PaymentOptionWaffoSandboxAPIKey          PaymentOptionKey = "WaffoSandboxApiKey"
	PaymentOptionWaffoSandboxPrivateKey      PaymentOptionKey = "WaffoSandboxPrivateKey"
	PaymentOptionWaffoSandbox                PaymentOptionKey = "WaffoSandbox"
	PaymentOptionWaffoMerchantID             PaymentOptionKey = "WaffoMerchantId"
	PaymentOptionWaffoNotifyURL              PaymentOptionKey = "WaffoNotifyUrl"
	PaymentOptionWaffoReturnURL              PaymentOptionKey = "WaffoReturnUrl"
	PaymentOptionWaffoSubscriptionReturnURL  PaymentOptionKey = "WaffoSubscriptionReturnUrl"
	PaymentOptionWaffoCurrency               PaymentOptionKey = "WaffoCurrency"
	PaymentOptionWaffoUnitPrice              PaymentOptionKey = "WaffoUnitPrice"
	PaymentOptionWaffoMinTopUp               PaymentOptionKey = "WaffoMinTopUp"
	PaymentOptionWaffoPayMethods             PaymentOptionKey = "WaffoPayMethods"
	PaymentOptionWaffoPancakeMerchantID      PaymentOptionKey = "WaffoPancakeMerchantID"
	PaymentOptionWaffoPancakePrivateKey      PaymentOptionKey = "WaffoPancakePrivateKey"
	PaymentOptionWaffoPancakeReturnURL       PaymentOptionKey = "WaffoPancakeReturnURL"
	PaymentOptionWaffoPancakeUnitPrice       PaymentOptionKey = "WaffoPancakeUnitPrice"
	PaymentOptionWaffoPancakeMinTopUp        PaymentOptionKey = "WaffoPancakeMinTopUp"
	PaymentOptionWaffoPancakeStoreID         PaymentOptionKey = "WaffoPancakeStoreID"
	PaymentOptionWaffoPancakeProductID       PaymentOptionKey = "WaffoPancakeProductID"
	// TopupGroupRatio participates in every legacy top-up price calculation.
	// Keeping it in the payment generation prevents a later bridge from mixing
	// a new unit price with a stale group ratio. Its detailed map semantics are
	// deliberately left to the typed bulk contract in C09-N3.
	PaymentOptionTopupGroupRatio        PaymentOptionKey = "TopupGroupRatio"
	PaymentOptionAmountOptions          PaymentOptionKey = "payment_setting.amount_options"
	PaymentOptionAmountDiscount         PaymentOptionKey = "payment_setting.amount_discount"
	PaymentOptionComplianceConfirmed    PaymentOptionKey = "payment_setting.compliance_confirmed"
	PaymentOptionComplianceTermsVersion PaymentOptionKey = "payment_setting.compliance_terms_version"
	PaymentOptionComplianceConfirmedAt  PaymentOptionKey = "payment_setting.compliance_confirmed_at"
	PaymentOptionComplianceConfirmedBy  PaymentOptionKey = "payment_setting.compliance_confirmed_by"
	PaymentOptionComplianceConfirmedIP  PaymentOptionKey = "payment_setting.compliance_confirmed_ip"
	PaymentOptionUserFundingMode        PaymentOptionKey = "user_funding_setting.mode"
	PaymentOptionUserFundingEpoch       PaymentOptionKey = "user_funding_setting.epoch"
	PaymentOptionWebhookKeyringMetadata PaymentOptionKey = "payment_runtime.webhook_keyring_metadata"
)

var (
	ErrPaymentRuntimeUnknownOption      = errors.New("payment runtime: unknown option")
	ErrPaymentRuntimeDuplicateOption    = errors.New("payment runtime: duplicate option")
	ErrPaymentRuntimeInvalidValue       = errors.New("payment runtime: invalid value")
	ErrPaymentRuntimeNullLikeValue      = errors.New("payment runtime: null-like value")
	ErrPaymentRuntimeValueType          = errors.New("payment runtime: unexpected value type")
	ErrPaymentRuntimeForeignSnapshot    = errors.New("payment runtime: foreign snapshot")
	ErrPaymentRuntimeRevisionExhausted  = errors.New("payment runtime: revision exhausted")
	ErrPaymentRuntimeConflict           = errors.New("payment runtime: publish conflict")
	ErrPaymentRuntimeCandidateAborted   = errors.New("payment runtime: candidate aborted")
	ErrPaymentRuntimeCandidateFinalized = errors.New("payment runtime: candidate finalized")
)

// PaymentValueKind describes the closed value union accepted by the runtime.
type PaymentValueKind uint8

const (
	PaymentValueInvalid PaymentValueKind = iota
	PaymentValueString
	PaymentValueInteger
	PaymentValueNumber
	PaymentValueBoolean
	PaymentValueJSON
	PaymentValueKeyringMetadata
)

func (kind PaymentValueKind) String() string {
	switch kind {
	case PaymentValueString:
		return "string"
	case PaymentValueInteger:
		return "integer"
	case PaymentValueNumber:
		return "number"
	case PaymentValueBoolean:
		return "boolean"
	case PaymentValueJSON:
		return "json"
	case PaymentValueKeyringMetadata:
		return "keyring-metadata"
	default:
		return "invalid"
	}
}

func (kind PaymentValueKind) GoString() string { return kind.String() }

// PaymentProvider identifies the provider associated with non-secret keyring
// metadata. It deliberately cannot carry signing or API key material.
type PaymentProvider string

const (
	PaymentProviderEpay         PaymentProvider = "epay"
	PaymentProviderStripe       PaymentProvider = "stripe"
	PaymentProviderCreem        PaymentProvider = "creem"
	PaymentProviderWaffo        PaymentProvider = "waffo"
	PaymentProviderWaffoPancake PaymentProvider = "waffo_pancake"
)

// PaymentKeyMetadata contains only key lifecycle metadata. KeyID must be an
// opaque, non-secret identifier; actual verification material belongs to the
// later bounded keyring implementation, not this N2a kernel.
type PaymentKeyMetadata struct {
	KeyID           string          `json:"key_id"`
	Provider        PaymentProvider `json:"provider"`
	Revision        uint64          `json:"revision"`
	ActivatedAtUnix int64           `json:"activated_at_unix"`
	RetiredAtUnix   int64           `json:"retired_at_unix,omitempty"`
	ExpiresAtUnix   int64           `json:"expires_at_unix,omitempty"`
}

func (metadata PaymentKeyMetadata) String() string {
	return "PaymentKeyMetadata{redacted}"
}

func (metadata PaymentKeyMetadata) GoString() string { return metadata.String() }

// PaymentValue is a typed value object. Its fields are private so construction
// and all collection access pass through copying and validation boundaries.
type PaymentValue struct {
	kind        PaymentValueKind
	sensitive   bool
	stringValue string
	intValue    int64
	numberValue float64
	boolValue   bool
	jsonValue   []byte
	keyring     []PaymentKeyMetadata
}

func NewPaymentStringValue(value string) (PaymentValue, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "null") || strings.EqualFold(trimmed, `"null"`) {
		return PaymentValue{}, ErrPaymentRuntimeNullLikeValue
	}
	return PaymentValue{kind: PaymentValueString, stringValue: value}, nil
}

func NewPaymentIntegerValue(value int64) PaymentValue {
	return PaymentValue{kind: PaymentValueInteger, intValue: value}
}

func NewPaymentNumberValue(value float64) (PaymentValue, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return PaymentValue{}, ErrPaymentRuntimeInvalidValue
	}
	return PaymentValue{kind: PaymentValueNumber, numberValue: value}, nil
}

func NewPaymentBooleanValue(value bool) PaymentValue {
	return PaymentValue{kind: PaymentValueBoolean, boolValue: value}
}

// NewPaymentJSONValue accepts only an object or array. common.Marshal both
// validates the supplied structure and detaches it from caller-owned maps and
// slices; the encoded bytes are never exposed without another copy.
func NewPaymentJSONValue(value any) (PaymentValue, error) {
	if value == nil {
		return PaymentValue{}, ErrPaymentRuntimeNullLikeValue
	}
	encoded, err := common.Marshal(value)
	if err != nil {
		return PaymentValue{}, ErrPaymentRuntimeInvalidValue
	}
	jsonType := common.GetJsonType(encoded)
	if jsonType != "array" && jsonType != "object" {
		if jsonType == "null" {
			return PaymentValue{}, ErrPaymentRuntimeNullLikeValue
		}
		return PaymentValue{}, ErrPaymentRuntimeInvalidValue
	}
	return PaymentValue{kind: PaymentValueJSON, jsonValue: append([]byte(nil), encoded...)}, nil
}

func NewPaymentKeyringMetadataValue(metadata []PaymentKeyMetadata) (PaymentValue, error) {
	if metadata == nil {
		return PaymentValue{}, ErrPaymentRuntimeNullLikeValue
	}
	seen := make(map[string]struct{}, len(metadata))
	for _, entry := range metadata {
		if !validPaymentProvider(entry.Provider) || isNullLikePaymentText(entry.KeyID) || entry.Revision == 0 || entry.ActivatedAtUnix <= 0 {
			return PaymentValue{}, ErrPaymentRuntimeInvalidValue
		}
		if entry.RetiredAtUnix != 0 && entry.RetiredAtUnix < entry.ActivatedAtUnix {
			return PaymentValue{}, ErrPaymentRuntimeInvalidValue
		}
		if entry.ExpiresAtUnix != 0 && entry.ExpiresAtUnix < entry.ActivatedAtUnix {
			return PaymentValue{}, ErrPaymentRuntimeInvalidValue
		}
		identity := string(entry.Provider) + "\x00" + entry.KeyID
		if _, exists := seen[identity]; exists {
			return PaymentValue{}, ErrPaymentRuntimeDuplicateOption
		}
		seen[identity] = struct{}{}
	}
	return PaymentValue{
		kind:    PaymentValueKeyringMetadata,
		keyring: append([]PaymentKeyMetadata(nil), metadata...),
	}, nil
}

func validPaymentProvider(provider PaymentProvider) bool {
	switch provider {
	case PaymentProviderEpay, PaymentProviderStripe, PaymentProviderCreem, PaymentProviderWaffo, PaymentProviderWaffoPancake:
		return true
	default:
		return false
	}
}

func isNullLikePaymentText(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || strings.EqualFold(trimmed, "null") || strings.EqualFold(trimmed, `"null"`)
}

func (value PaymentValue) Kind() PaymentValueKind { return value.kind }

func (value PaymentValue) Sensitive() bool { return value.sensitive }

func (value PaymentValue) StringValue() (string, bool) {
	return value.stringValue, value.kind == PaymentValueString
}

func (value PaymentValue) IntegerValue() (int64, bool) {
	return value.intValue, value.kind == PaymentValueInteger
}

func (value PaymentValue) NumberValue() (float64, bool) {
	return value.numberValue, value.kind == PaymentValueNumber
}

func (value PaymentValue) BooleanValue() (bool, bool) {
	return value.boolValue, value.kind == PaymentValueBoolean
}

func (value PaymentValue) JSONValue() ([]byte, bool) {
	if value.kind != PaymentValueJSON {
		return nil, false
	}
	return append([]byte(nil), value.jsonValue...), true
}

func (value PaymentValue) DecodeJSON(destination any) error {
	if value.kind != PaymentValueJSON || destination == nil {
		return ErrPaymentRuntimeValueType
	}
	if err := common.Unmarshal(value.jsonValue, destination); err != nil {
		return ErrPaymentRuntimeInvalidValue
	}
	return nil
}

func (value PaymentValue) KeyringMetadata() ([]PaymentKeyMetadata, bool) {
	if value.kind != PaymentValueKeyringMetadata {
		return nil, false
	}
	return append([]PaymentKeyMetadata(nil), value.keyring...), true
}

func (value PaymentValue) String() string {
	if value.sensitive {
		return "PaymentValue{kind:" + value.kind.String() + ",sensitive:true,value:redacted}"
	}
	return "PaymentValue{kind:" + value.kind.String() + ",value:redacted}"
}

func (value PaymentValue) GoString() string { return value.String() }

func (value PaymentValue) clone() PaymentValue {
	value.jsonValue = append([]byte(nil), value.jsonValue...)
	value.keyring = append([]PaymentKeyMetadata(nil), value.keyring...)
	return value
}

// PaymentInitialValue is a pure-value seed entry for a new, independent
// runtime. Duplicate and unknown keys are rejected.
type PaymentInitialValue struct {
	Key   PaymentOptionKey
	Value PaymentValue
}

func (initial PaymentInitialValue) String() string   { return "PaymentInitialValue{value:redacted}" }
func (initial PaymentInitialValue) GoString() string { return initial.String() }

type PaymentMutationAction uint8

const (
	PaymentMutationInvalid PaymentMutationAction = iota
	PaymentMutationKeep
	PaymentMutationSet
	PaymentMutationClear
)

func (action PaymentMutationAction) String() string {
	switch action {
	case PaymentMutationKeep:
		return "keep"
	case PaymentMutationSet:
		return "set"
	case PaymentMutationClear:
		return "clear"
	default:
		return "invalid"
	}
}

func (action PaymentMutationAction) GoString() string { return action.String() }

// PaymentMutation makes keep, set, and clear explicit. Keep and Clear require
// a zero PaymentValue; Set requires a constructed, schema-compatible value.
type PaymentMutation struct {
	Key    PaymentOptionKey
	Action PaymentMutationAction
	Value  PaymentValue
}

func (mutation PaymentMutation) String() string {
	return "PaymentMutation{action:" + mutation.Action.String() + ",value:redacted}"
}

func (mutation PaymentMutation) GoString() string { return mutation.String() }

type paymentValueConstraint uint8

const (
	paymentValueUnconstrained paymentValueConstraint = iota
	paymentValuePositiveInteger
	paymentValuePositiveNumber
	paymentValueUserFundingMode
)

type paymentOptionSpec struct {
	kind       PaymentValueKind
	sensitive  bool
	constraint paymentValueConstraint
}

var paymentOptionSpecs = map[PaymentOptionKey]paymentOptionSpec{
	PaymentOptionPayAddress:                  {kind: PaymentValueString},
	PaymentOptionCustomCallbackAddress:       {kind: PaymentValueString},
	PaymentOptionEpayID:                      {kind: PaymentValueString},
	PaymentOptionEpayKey:                     {kind: PaymentValueString, sensitive: true},
	PaymentOptionPrice:                       {kind: PaymentValueNumber, constraint: paymentValuePositiveNumber},
	PaymentOptionUSDExchangeRate:             {kind: PaymentValueNumber, constraint: paymentValuePositiveNumber},
	PaymentOptionMinTopUp:                    {kind: PaymentValueInteger, constraint: paymentValuePositiveInteger},
	PaymentOptionPayMethods:                  {kind: PaymentValueJSON},
	PaymentOptionStripeAPISecret:             {kind: PaymentValueString, sensitive: true},
	PaymentOptionStripeWebhookSecret:         {kind: PaymentValueString, sensitive: true},
	PaymentOptionStripePriceID:               {kind: PaymentValueString},
	PaymentOptionStripeUnitPrice:             {kind: PaymentValueNumber, constraint: paymentValuePositiveNumber},
	PaymentOptionStripeMinTopUp:              {kind: PaymentValueInteger, constraint: paymentValuePositiveInteger},
	PaymentOptionStripePromotionCodesEnabled: {kind: PaymentValueBoolean},
	PaymentOptionCreemAPIKey:                 {kind: PaymentValueString, sensitive: true},
	PaymentOptionCreemProducts:               {kind: PaymentValueJSON},
	PaymentOptionCreemTestMode:               {kind: PaymentValueBoolean},
	PaymentOptionCreemWebhookSecret:          {kind: PaymentValueString, sensitive: true},
	PaymentOptionWaffoEnabled:                {kind: PaymentValueBoolean},
	PaymentOptionWaffoAPIKey:                 {kind: PaymentValueString, sensitive: true},
	PaymentOptionWaffoPrivateKey:             {kind: PaymentValueString, sensitive: true},
	PaymentOptionWaffoPublicCert:             {kind: PaymentValueString},
	PaymentOptionWaffoSandboxPublicCert:      {kind: PaymentValueString},
	PaymentOptionWaffoSandboxAPIKey:          {kind: PaymentValueString, sensitive: true},
	PaymentOptionWaffoSandboxPrivateKey:      {kind: PaymentValueString, sensitive: true},
	PaymentOptionWaffoSandbox:                {kind: PaymentValueBoolean},
	PaymentOptionWaffoMerchantID:             {kind: PaymentValueString},
	PaymentOptionWaffoNotifyURL:              {kind: PaymentValueString},
	PaymentOptionWaffoReturnURL:              {kind: PaymentValueString},
	PaymentOptionWaffoSubscriptionReturnURL:  {kind: PaymentValueString},
	PaymentOptionWaffoCurrency:               {kind: PaymentValueString},
	PaymentOptionWaffoUnitPrice:              {kind: PaymentValueNumber, constraint: paymentValuePositiveNumber},
	PaymentOptionWaffoMinTopUp:               {kind: PaymentValueInteger, constraint: paymentValuePositiveInteger},
	PaymentOptionWaffoPayMethods:             {kind: PaymentValueJSON},
	PaymentOptionWaffoPancakeMerchantID:      {kind: PaymentValueString},
	PaymentOptionWaffoPancakePrivateKey:      {kind: PaymentValueString, sensitive: true},
	PaymentOptionWaffoPancakeReturnURL:       {kind: PaymentValueString},
	PaymentOptionWaffoPancakeUnitPrice:       {kind: PaymentValueNumber, constraint: paymentValuePositiveNumber},
	PaymentOptionWaffoPancakeMinTopUp:        {kind: PaymentValueInteger, constraint: paymentValuePositiveInteger},
	PaymentOptionWaffoPancakeStoreID:         {kind: PaymentValueString},
	PaymentOptionWaffoPancakeProductID:       {kind: PaymentValueString},
	PaymentOptionTopupGroupRatio:             {kind: PaymentValueJSON},
	PaymentOptionAmountOptions:               {kind: PaymentValueJSON},
	PaymentOptionAmountDiscount:              {kind: PaymentValueJSON},
	PaymentOptionComplianceConfirmed:         {kind: PaymentValueBoolean},
	PaymentOptionComplianceTermsVersion:      {kind: PaymentValueString},
	PaymentOptionComplianceConfirmedAt:       {kind: PaymentValueInteger},
	PaymentOptionComplianceConfirmedBy:       {kind: PaymentValueInteger},
	PaymentOptionComplianceConfirmedIP:       {kind: PaymentValueString, sensitive: true},
	PaymentOptionUserFundingMode:             {kind: PaymentValueString, constraint: paymentValueUserFundingMode},
	PaymentOptionUserFundingEpoch:            {kind: PaymentValueInteger},
	PaymentOptionWebhookKeyringMetadata:      {kind: PaymentValueKeyringMetadata},
}

func validatedPaymentValue(key PaymentOptionKey, value PaymentValue) (PaymentValue, error) {
	spec, ok := paymentOptionSpecs[key]
	if !ok {
		return PaymentValue{}, ErrPaymentRuntimeUnknownOption
	}
	if value.kind == PaymentValueInvalid {
		return PaymentValue{}, ErrPaymentRuntimeNullLikeValue
	}
	if value.kind != spec.kind {
		return PaymentValue{}, ErrPaymentRuntimeValueType
	}
	switch spec.constraint {
	case paymentValuePositiveInteger:
		if value.intValue <= 0 {
			return PaymentValue{}, ErrPaymentRuntimeInvalidValue
		}
	case paymentValuePositiveNumber:
		if value.numberValue <= 0 || math.IsNaN(value.numberValue) || math.IsInf(value.numberValue, 0) {
			return PaymentValue{}, ErrPaymentRuntimeInvalidValue
		}
	case paymentValueUserFundingMode:
		switch value.stringValue {
		case string(operation_setting.UserFundingModeEnabled), string(operation_setting.UserFundingModeRetirement), string(operation_setting.UserFundingModeDisabled):
		default:
			return PaymentValue{}, ErrPaymentRuntimeInvalidValue
		}
	}
	value = value.clone()
	value.sensitive = spec.sensitive
	return value, nil
}

type paymentGeneration struct {
	revision         uint64
	values           map[PaymentOptionKey]PaymentValue
	canonicalOptions map[string]string
}

func buildPaymentGeneration(revision uint64, values map[PaymentOptionKey]PaymentValue) (*paymentGeneration, error) {
	ownedValues := make(map[PaymentOptionKey]PaymentValue, len(values))
	canonical := make(map[string]string, len(values))
	for key, value := range values {
		owned := value.clone()
		encoded, err := canonicalPaymentValue(owned)
		if err != nil {
			return nil, err
		}
		ownedValues[key] = owned
		canonical[string(key)] = encoded
	}
	return &paymentGeneration{revision: revision, values: ownedValues, canonicalOptions: canonical}, nil
}

func canonicalPaymentValue(value PaymentValue) (string, error) {
	switch value.kind {
	case PaymentValueString:
		return value.stringValue, nil
	case PaymentValueInteger:
		return strconv.FormatInt(value.intValue, 10), nil
	case PaymentValueNumber:
		return strconv.FormatFloat(value.numberValue, 'f', -1, 64), nil
	case PaymentValueBoolean:
		return strconv.FormatBool(value.boolValue), nil
	case PaymentValueJSON:
		return string(value.jsonValue), nil
	case PaymentValueKeyringMetadata:
		encoded, err := common.Marshal(value.keyring)
		if err != nil {
			return "", ErrPaymentRuntimeInvalidValue
		}
		return string(encoded), nil
	default:
		return "", ErrPaymentRuntimeNullLikeValue
	}
}

// PaymentRuntime owns one atomically published immutable generation. It has no
// dependency on option persistence, provider SDKs, or the legacy globals.
type PaymentRuntime struct {
	current atomic.Pointer[paymentGeneration]
}

// NewPaymentRuntime constructs an independent runtime solely from explicit
// values. It performs no external reads or writes.
func NewPaymentRuntime(initial []PaymentInitialValue) (*PaymentRuntime, error) {
	values := make(map[PaymentOptionKey]PaymentValue, len(initial))
	seen := make(map[PaymentOptionKey]struct{}, len(initial))
	for _, entry := range initial {
		if _, exists := seen[entry.Key]; exists {
			return nil, ErrPaymentRuntimeDuplicateOption
		}
		seen[entry.Key] = struct{}{}
		value, err := validatedPaymentValue(entry.Key, entry.Value)
		if err != nil {
			return nil, err
		}
		values[entry.Key] = value
	}
	generation, err := buildPaymentGeneration(1, values)
	if err != nil {
		return nil, err
	}
	runtime := &PaymentRuntime{}
	runtime.current.Store(generation)
	return runtime, nil
}

func (runtime *PaymentRuntime) String() string {
	if runtime == nil {
		return "PaymentRuntime{nil}"
	}
	return "PaymentRuntime{values:redacted}"
}

func (runtime *PaymentRuntime) GoString() string { return runtime.String() }

// PaymentSnapshot pins exactly one generation. Collection accessors always
// return detached copies.
type PaymentSnapshot struct {
	runtime    *PaymentRuntime
	generation *paymentGeneration
}

func (runtime *PaymentRuntime) Current() PaymentSnapshot {
	if runtime == nil {
		return PaymentSnapshot{}
	}
	return PaymentSnapshot{runtime: runtime, generation: runtime.current.Load()}
}

func (snapshot PaymentSnapshot) Revision() uint64 {
	if snapshot.generation == nil {
		return 0
	}
	return snapshot.generation.revision
}

func (snapshot PaymentSnapshot) Value(key PaymentOptionKey) (PaymentValue, bool) {
	if snapshot.generation == nil {
		return PaymentValue{}, false
	}
	value, ok := snapshot.generation.values[key]
	return value.clone(), ok
}

type PaymentOptionValues map[PaymentOptionKey]PaymentValue

func (values PaymentOptionValues) String() string {
	return "PaymentOptionValues{count:" + strconv.Itoa(len(values)) + ",values:redacted}"
}

func (values PaymentOptionValues) GoString() string { return values.String() }

func (snapshot PaymentSnapshot) Values() PaymentOptionValues {
	if snapshot.generation == nil {
		return PaymentOptionValues{}
	}
	values := make(PaymentOptionValues, len(snapshot.generation.values))
	for key, value := range snapshot.generation.values {
		values[key] = value.clone()
	}
	return values
}

type PaymentCanonicalOptions map[string]string

func (options PaymentCanonicalOptions) String() string {
	return "PaymentCanonicalOptions{count:" + strconv.Itoa(len(options)) + ",values:redacted}"
}

func (options PaymentCanonicalOptions) GoString() string { return options.String() }

func (snapshot PaymentSnapshot) CanonicalOptions() PaymentCanonicalOptions {
	if snapshot.generation == nil {
		return PaymentCanonicalOptions{}
	}
	options := make(PaymentCanonicalOptions, len(snapshot.generation.canonicalOptions))
	for key, value := range snapshot.generation.canonicalOptions {
		options[key] = value
	}
	return options
}

func (snapshot PaymentSnapshot) String() string {
	return "PaymentSnapshot{revision:" + strconv.FormatUint(snapshot.Revision(), 10) + ",values:redacted}"
}

func (snapshot PaymentSnapshot) GoString() string { return snapshot.String() }

const (
	paymentCandidatePending uint32 = iota
	paymentCandidatePublishing
	paymentCandidatePublished
	paymentCandidateAborted
	paymentCandidateConflicted
)

// PaymentCandidate is fully validated and detached before Publish. Publish has
// exactly one runtime side effect: a single atomic generation CAS.
type PaymentCandidate struct {
	runtime *PaymentRuntime
	base    *paymentGeneration
	next    *paymentGeneration
	state   atomic.Uint32
}

func (runtime *PaymentRuntime) PrepareCandidate(base PaymentSnapshot, mutations []PaymentMutation) (*PaymentCandidate, error) {
	if runtime == nil || base.runtime != runtime || base.generation == nil {
		return nil, ErrPaymentRuntimeForeignSnapshot
	}
	if base.generation.revision == ^uint64(0) {
		return nil, ErrPaymentRuntimeRevisionExhausted
	}
	values := make(map[PaymentOptionKey]PaymentValue, len(base.generation.values))
	for key, value := range base.generation.values {
		values[key] = value.clone()
	}
	seen := make(map[PaymentOptionKey]struct{}, len(mutations))
	for _, mutation := range mutations {
		if _, known := paymentOptionSpecs[mutation.Key]; !known {
			return nil, ErrPaymentRuntimeUnknownOption
		}
		if _, exists := seen[mutation.Key]; exists {
			return nil, ErrPaymentRuntimeDuplicateOption
		}
		seen[mutation.Key] = struct{}{}
		switch mutation.Action {
		case PaymentMutationKeep:
			if mutation.Value.kind != PaymentValueInvalid {
				return nil, ErrPaymentRuntimeInvalidValue
			}
		case PaymentMutationClear:
			if mutation.Value.kind != PaymentValueInvalid {
				return nil, ErrPaymentRuntimeInvalidValue
			}
			delete(values, mutation.Key)
		case PaymentMutationSet:
			value, err := validatedPaymentValue(mutation.Key, mutation.Value)
			if err != nil {
				return nil, err
			}
			values[mutation.Key] = value
		default:
			return nil, ErrPaymentRuntimeInvalidValue
		}
	}
	next, err := buildPaymentGeneration(base.generation.revision+1, values)
	if err != nil {
		return nil, err
	}
	return &PaymentCandidate{runtime: runtime, base: base.generation, next: next}, nil
}

func (candidate *PaymentCandidate) Revision() uint64 {
	if candidate == nil || candidate.next == nil {
		return 0
	}
	return candidate.next.revision
}

func (candidate *PaymentCandidate) CanonicalOptions() PaymentCanonicalOptions {
	if candidate == nil || candidate.next == nil {
		return PaymentCanonicalOptions{}
	}
	options := make(PaymentCanonicalOptions, len(candidate.next.canonicalOptions))
	for key, value := range candidate.next.canonicalOptions {
		options[key] = value
	}
	return options
}

func (candidate *PaymentCandidate) Publish() error {
	if candidate == nil || candidate.runtime == nil || candidate.next == nil {
		return ErrPaymentRuntimeCandidateFinalized
	}
	if !candidate.state.CompareAndSwap(paymentCandidatePending, paymentCandidatePublishing) {
		if candidate.state.Load() == paymentCandidateAborted {
			return ErrPaymentRuntimeCandidateAborted
		}
		return ErrPaymentRuntimeCandidateFinalized
	}
	if !candidate.runtime.current.CompareAndSwap(candidate.base, candidate.next) {
		candidate.state.Store(paymentCandidateConflicted)
		return ErrPaymentRuntimeConflict
	}
	candidate.state.Store(paymentCandidatePublished)
	return nil
}

// Abort is idempotent. It returns true only when it transitions a pending
// candidate to aborted; an aborted candidate can never publish.
func (candidate *PaymentCandidate) Abort() bool {
	return candidate != nil && candidate.state.CompareAndSwap(paymentCandidatePending, paymentCandidateAborted)
}

func (candidate *PaymentCandidate) String() string {
	if candidate == nil {
		return "PaymentCandidate{nil}"
	}
	return "PaymentCandidate{revision:" + strconv.FormatUint(candidate.Revision(), 10) + ",values:redacted}"
}

func (candidate *PaymentCandidate) GoString() string { return candidate.String() }

var packagePaymentRuntime = func() *PaymentRuntime {
	runtime, err := NewPaymentRuntime(nil)
	if err != nil {
		panic("payment runtime: invalid empty generation")
	}
	return runtime
}()

// CurrentPaymentRuntime exposes the package runtime for future option/provider
// wiring. This file intentionally does not read or replace any legacy global.
func CurrentPaymentRuntime() PaymentSnapshot {
	return packagePaymentRuntime.Current()
}

func PreparePaymentCandidate(base PaymentSnapshot, mutations []PaymentMutation) (*PaymentCandidate, error) {
	return packagePaymentRuntime.PrepareCandidate(base, mutations)
}
