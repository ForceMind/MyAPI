package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/ForceMind/MyAPI/setting/ratio_setting"
	"gorm.io/gorm"
)

// Typed bulk single-writer (C09-N3a).
//
// This file implements the bounded typed bulk writer for the managed
// configuration families: one request carries a closed set of typed key/value
// items, everything is normalized and validated before any write, the database
// commit happens in a single transaction under optionMutationLock, and the
// committed values are published to the runtime exactly once per family
// (candidate + single generation store, mirroring the payment funding bulk).
//
// The legacy single-key PUT (UpdateOption) and the payment funding bulk keep
// their existing behavior; only this endpoint rejects unknown keys outright.
//
// External side effects (cache invalidations such as the billing pricing
// caches) deliberately run after the database commit and are NOT part of the
// database atomicity: they are in-memory best-effort projections, and a
// failure there is only logged. External provider operations are never
// enlisted in the transaction.

const (
	// TypedBulkValueTypeString is a raw string value (including JSON text for
	// map/object-typed family fields).
	TypedBulkValueTypeString = "string"
	// TypedBulkValueTypeNumber is a finite JSON number, bounded in magnitude.
	TypedBulkValueTypeNumber = "number"
	// TypedBulkValueTypeBoolean is a JSON true/false.
	TypedBulkValueTypeBoolean = "boolean"
	// TypedBulkValueTypeStringList is a JSON array of strings, persisted in
	// canonical JSON array form.
	TypedBulkValueTypeStringList = "string_list"
)

const (
	// MaxTypedBulkItems bounds the key set of one typed bulk request.
	MaxTypedBulkItems = 64
	// MaxTypedBulkKeyLength bounds the option key length (matches the 256
	// character key bound used by the option diagnostics reader).
	MaxTypedBulkKeyLength = 256
	// MaxTypedBulkValueLength bounds the canonical string form of one value
	// (matches the 65536 character option value bound).
	MaxTypedBulkValueLength = 65536
	// MaxTypedBulkStringListItems bounds the element count of one string_list.
	MaxTypedBulkStringListItems = 256
	// MaxTypedBulkStringListItemLength bounds one string_list element.
	MaxTypedBulkStringListItemLength = 1024
	// maxTypedBulkNumberMagnitude bounds the absolute value of number items.
	maxTypedBulkNumberMagnitude = 1e15
)

// typedBulkRevisionOptionKey is the reserved options-table row holding the
// persistent monotonic revision of the typed bulk write stream. It is never
// published to OptionMap and is not writable through any endpoint.
//
// Revision mechanism choice: the options table has no updated_at/version
// column, and adding one would not be maintained by the legacy writers, so a
// dedicated counter row is the minimal persistent implementation. The counter
// is bumped exactly once per committed typed bulk inside the same transaction
// (guarded conditional update), which makes the database the final CAS
// arbiter even across processes; optionMutationLock serializes in-process
// writers. Legacy single-key PUT and the payment funding bulk do not bump the
// counter: the revision versions the typed bulk write stream that the merged
// admin forms (C09-N3b) will CAS on, and those forms use this endpoint
// exclusively.
const typedBulkRevisionOptionKey = "typed_bulk_revision"

var (
	ErrTypedBulkUnknownKey       = errors.New("typed bulk option key is not managed")
	ErrTypedBulkDuplicateKey     = errors.New("typed bulk option key is duplicated")
	ErrTypedBulkInvalidValue     = errors.New("typed bulk option value is invalid")
	ErrTypedBulkItemCount        = errors.New("typed bulk item count out of range")
	ErrTypedBulkRevisionConflict = errors.New("typed bulk revision conflict")
	// ErrTypedBulkPublishFailed marks a post-commit runtime publication abort:
	// the database commit stands (same documented boundary as the payment
	// funding bulk) while the failed family and all later families keep their
	// previous runtime generation and OptionMap values.
	ErrTypedBulkPublishFailed = errors.New("typed bulk runtime publication failed")
)

// TypedBulkFieldError describes one rejected request item. Kind is one of the
// sentinel errors above; Key identifies the offending option key when
// applicable; Reason carries the underlying validator detail.
type TypedBulkFieldError struct {
	Kind   error
	Key    string
	Reason string
}

func (e *TypedBulkFieldError) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("%s: %s", e.Kind, e.Reason)
	}
	return fmt.Sprintf("%s: %s (%s)", e.Kind, e.Key, e.Reason)
}

func (e *TypedBulkFieldError) Is(target error) bool { return target == e.Kind }

// TypedBulkRevisionConflictError reports a failed expected-revision CAS check.
type TypedBulkRevisionConflictError struct {
	Expected int64
	Actual   int64
}

func (e *TypedBulkRevisionConflictError) Error() string {
	return fmt.Sprintf("%s: expected %d, current %d", ErrTypedBulkRevisionConflict, e.Expected, e.Actual)
}

func (e *TypedBulkRevisionConflictError) Is(target error) bool {
	return target == ErrTypedBulkRevisionConflict
}

// TypedBulkOption is one typed bulk request item. Value carries the raw JSON
// lexeme so the declared Type can be enforced against the actual JSON type.
type TypedBulkOption struct {
	Key   string          `json:"key"`
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

// typedBulkManagedFamilies is the closed allow-list of configuration families
// writable through the typed bulk endpoint. Excluded on purpose:
//   - performance_setting: hot-path decision family pending D14.
//   - channel_affinity_setting: channel/retry/billing decision family pending D15.
//   - payment_setting: owned by the payment funding bulk (single runtime
//     candidate + funding barrier); compliance fields are never editable here.
//   - user_funding_setting: owned by the funding state machine transitions.
var typedBulkManagedFamilies = map[string]struct{}{
	"access_profile_setting": {},
	"billing_setting":        {},
	"checkin_setting":        {},
	"claude":                 {},
	"console_setting":        {},
	"discord":                {},
	"fetch_setting":          {},
	"gemini":                 {},
	"general_setting":        {},
	"global":                 {},
	"grok":                   {},
	"group_ratio_setting":    {},
	"legal":                  {},
	"monitor_setting":        {},
	"oidc":                   {},
	"passkey":                {},
	"perf_metrics_setting":   {},
	"qwen":                   {},
	"quota_setting":          {},
	"token_setting":          {},
	"tool_price_setting":     {},
}

// typedBulkFamilyPublish is the per-family runtime publication call: one
// candidate build and one generation store per family (MapConfig families use
// their own generation publish; reflection-delegating families use the
// existing candidate Set inside config.UpdateConfigFromMap). It is a variable
// so tests can inject a mid-group publication failure.
var typedBulkFamilyPublish = config.UpdateConfigFromMap

// typedBulkPrepared holds the fully normalized and validated request.
type typedBulkPrepared struct {
	// values is the canonical key -> string form for persistence and OptionMap.
	values map[string]string
	// familyValues groups field -> value by family for validation/publish.
	familyValues map[string]map[string]string
	// sortedKeys is the deterministic applied-key list for responses/audit.
	sortedKeys []string
}

// CurrentTypedBulkRevision returns the persistent typed bulk revision (0 when
// no typed bulk has committed yet). It backs the form-loading GET endpoint.
func CurrentTypedBulkRevision() (int64, error) {
	var option Option
	err := DB.Where("key = ?", typedBulkRevisionOptionKey).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return parseTypedBulkRevision(option.Value)
}

func parseTypedBulkRevision(raw string) (int64, error) {
	revision, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || revision < 0 {
		return 0, fmt.Errorf("corrupt typed bulk revision %q", raw)
	}
	return revision, nil
}

// canonicalizeTypedBulkValue enforces the declared type against the actual
// JSON type and returns the canonical string form consumed by the existing
// per-key validators, persistence, and OptionMap.
func canonicalizeTypedBulkValue(valueType string, raw json.RawMessage) (string, error) {
	invalid := func(reason string) (string, error) {
		return "", &TypedBulkFieldError{Kind: ErrTypedBulkInvalidValue, Reason: reason}
	}
	switch valueType {
	case TypedBulkValueTypeString:
		if common.GetJsonType(raw) != "string" {
			return invalid("value must be a JSON string")
		}
		var value string
		if err := common.Unmarshal(raw, &value); err != nil {
			return invalid("value must be a JSON string")
		}
		if len(value) > MaxTypedBulkValueLength {
			return invalid(fmt.Sprintf("value exceeds %d bytes", MaxTypedBulkValueLength))
		}
		return value, nil
	case TypedBulkValueTypeBoolean:
		if common.GetJsonType(raw) != "boolean" {
			return invalid("value must be a JSON boolean")
		}
		var value bool
		if err := common.Unmarshal(raw, &value); err != nil {
			return invalid("value must be a JSON boolean")
		}
		return strconv.FormatBool(value), nil
	case TypedBulkValueTypeNumber:
		// json.Number also accepts quoted JSON strings, so enforce the raw
		// number lexeme before decoding.
		if common.GetJsonType(raw) != "number" {
			return invalid("value must be a JSON number")
		}
		var number json.Number
		if err := common.Unmarshal(raw, &number); err != nil {
			return invalid("value must be a JSON number")
		}
		text := number.String()
		if strings.ContainsAny(text, ".eE") {
			value, err := strconv.ParseFloat(text, 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > maxTypedBulkNumberMagnitude {
				return invalid("number must be finite and within ±1e15")
			}
			return strconv.FormatFloat(value, 'f', -1, 64), nil
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil || math.Abs(float64(value)) > maxTypedBulkNumberMagnitude {
			return invalid("integer must be within ±1e15")
		}
		return text, nil
	case TypedBulkValueTypeStringList:
		if common.GetJsonType(raw) != "array" {
			return invalid("value must be a JSON array of strings")
		}
		var list []string
		if err := common.Unmarshal(raw, &list); err != nil {
			return invalid("value must be a JSON array of strings")
		}
		if list == nil {
			// Persist an explicit empty list rather than JSON null.
			list = []string{}
		}
		if len(list) > MaxTypedBulkStringListItems {
			return invalid(fmt.Sprintf("string_list exceeds %d elements", MaxTypedBulkStringListItems))
		}
		for _, element := range list {
			if len(element) > MaxTypedBulkStringListItemLength {
				return invalid(fmt.Sprintf("string_list element exceeds %d bytes", MaxTypedBulkStringListItemLength))
			}
		}
		encoded, err := common.Marshal(list)
		if err != nil {
			return invalid(err.Error())
		}
		if len(encoded) > MaxTypedBulkValueLength {
			return invalid(fmt.Sprintf("value exceeds %d bytes", MaxTypedBulkValueLength))
		}
		return string(encoded), nil
	default:
		return invalid("unknown value type " + strconv.Quote(valueType))
	}
}

// normalizeTypedBulkOptions performs the whole normalize + validate phase.
// Any failure returns before any lock, database, runtime, or OptionMap write.
func normalizeTypedBulkOptions(items []TypedBulkOption) (*typedBulkPrepared, error) {
	if len(items) == 0 || len(items) > MaxTypedBulkItems {
		return nil, &TypedBulkFieldError{
			Kind:   ErrTypedBulkItemCount,
			Reason: fmt.Sprintf("items must contain between 1 and %d entries", MaxTypedBulkItems),
		}
	}
	prepared := &typedBulkPrepared{
		values:       make(map[string]string, len(items)),
		familyValues: make(map[string]map[string]string),
	}
	for _, item := range items {
		if len(item.Key) == 0 || len(item.Key) > MaxTypedBulkKeyLength {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkUnknownKey, Key: item.Key, Reason: "key length out of range"}
		}
		if _, duplicated := prepared.values[item.Key]; duplicated {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkDuplicateKey, Key: item.Key, Reason: "key appears more than once"}
		}
		family, field, found := strings.Cut(item.Key, ".")
		if !found || field == "" {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkUnknownKey, Key: item.Key, Reason: "managed option keys use the <family>.<field> form"}
		}
		if _, managed := typedBulkManagedFamilies[family]; !managed {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkUnknownKey, Key: item.Key, Reason: "configuration family is not managed by the typed bulk writer"}
		}
		registered := config.GlobalConfig.Get(family)
		if registered == nil {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkUnknownKey, Key: item.Key, Reason: "configuration family is not registered"}
		}
		exported, err := config.ConfigToMap(registered)
		if err != nil {
			return nil, err
		}
		if _, known := exported[field]; !known {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkUnknownKey, Key: item.Key, Reason: "unknown field in managed configuration family"}
		}
		value, err := canonicalizeTypedBulkValue(item.Type, item.Value)
		if err != nil {
			var fieldErr *TypedBulkFieldError
			if errors.As(err, &fieldErr) {
				fieldErr.Key = item.Key
			}
			return nil, err
		}
		value, err = normalizeOptionValue(item.Key, value)
		if err != nil {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkInvalidValue, Key: item.Key, Reason: err.Error()}
		}
		if err := validateOptionValue(item.Key, value); err != nil {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkInvalidValue, Key: item.Key, Reason: err.Error()}
		}
		prepared.values[item.Key] = value
		if prepared.familyValues[family] == nil {
			prepared.familyValues[family] = make(map[string]string)
		}
		prepared.familyValues[family][field] = value
	}
	// Whole-family validation catches cross-field constraints of one family in
	// a single pass, before anything is written.
	for family, values := range prepared.familyValues {
		registered := config.GlobalConfig.Get(family)
		if err := config.ValidateConfigFromMap(registered, values); err != nil && err != config.ErrMapConfigValidationUnsupported {
			return nil, &TypedBulkFieldError{Kind: ErrTypedBulkInvalidValue, Key: family, Reason: err.Error()}
		}
	}
	// The group ratio compatibility aliases are persisted together with the
	// canonical flat keys (same expansion as UpdateOptionsBulk), so a reload
	// never prefers a stale canonical row over the freshly written alias row.
	for _, pair := range groupRatioOptionPairs {
		if value, ok := prepared.values[pair.alias]; ok {
			prepared.values[pair.canonical] = value
		}
	}
	prepared.sortedKeys = make([]string, 0, len(prepared.values))
	for key := range prepared.values {
		prepared.sortedKeys = append(prepared.sortedKeys, key)
	}
	sort.Strings(prepared.sortedKeys)
	return prepared, nil
}

// readTypedBulkRevisionTx reads the revision row inside the transaction,
// taking a FOR UPDATE row lock on databases that support it.
func readTypedBulkRevisionTx(tx *gorm.DB) (current int64, exists bool, err error) {
	var option Option
	query := lockForUpdate(tx.Where("key = ?", typedBulkRevisionOptionKey))
	err = query.First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	current, err = parseTypedBulkRevision(option.Value)
	if err != nil {
		return 0, false, err
	}
	return current, true, nil
}

// bumpTypedBulkRevisionTx advances the persistent revision exactly once. The
// guarded conditional update makes the database the final CAS arbiter even if
// a writer outside optionMutationLock (another process) races the commit.
func bumpTypedBulkRevisionTx(tx *gorm.DB, current int64, exists bool) (int64, error) {
	if current == math.MaxInt64 {
		return 0, errors.New("typed bulk revision counter exhausted")
	}
	next := current + 1
	nextRaw := strconv.FormatInt(next, 10)
	if !exists {
		if err := tx.Create(&Option{Key: typedBulkRevisionOptionKey, Value: nextRaw}).Error; err != nil {
			return 0, err
		}
		return next, nil
	}
	result := tx.Model(&Option{}).
		Where("key = ?", typedBulkRevisionOptionKey).
		Where("value = ?", strconv.FormatInt(current, 10)).
		Update("value", nextRaw)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, &TypedBulkRevisionConflictError{Expected: current, Actual: -1}
	}
	return next, nil
}

// UpdateOptionsTypedBulk is the typed bulk single writer: full normalize +
// validate (any failure writes nothing), one database transaction guarded by
// optionMutationLock with the expected-revision CAS check, then one runtime
// publication per family after the commit.
//
// Publication abort semantics: if a family publication fails after the
// database commit, the whole group aborts, the failure is recorded through
// SysError, the failed family and every later family keep their previous
// runtime generation and OptionMap values, and ErrTypedBulkPublishFailed is
// returned. Families published before the failure keep their new generation;
// this post-commit boundary matches the payment funding bulk contract and is
// never presented as a cross-family atomic runtime commit.
func UpdateOptionsTypedBulk(items []TypedBulkOption, expectedRevision *int64) (int64, []string, error) {
	if expectedRevision != nil && *expectedRevision < 0 {
		return 0, nil, &TypedBulkFieldError{Kind: ErrTypedBulkInvalidValue, Key: "expected_revision", Reason: "expected_revision must not be negative"}
	}
	prepared, err := normalizeTypedBulkOptions(items)
	if err != nil {
		return 0, nil, err
	}
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()

	var newRevision int64
	err = DB.Transaction(func(tx *gorm.DB) error {
		current, exists, err := readTypedBulkRevisionTx(tx)
		if err != nil {
			return err
		}
		if expectedRevision != nil && *expectedRevision != current {
			return &TypedBulkRevisionConflictError{Expected: *expectedRevision, Actual: current}
		}
		if err := persistOptionValuesTx(tx, prepared.values); err != nil {
			return err
		}
		newRevision, err = bumpTypedBulkRevisionTx(tx, current, exists)
		return err
	})
	if err != nil {
		return 0, nil, err
	}

	families := make([]string, 0, len(prepared.familyValues))
	for family := range prepared.familyValues {
		families = append(families, family)
	}
	sort.Strings(families)
	for _, family := range families {
		registered := config.GlobalConfig.Get(family)
		if err := typedBulkFamilyPublish(registered, prepared.familyValues[family]); err != nil {
			common.SysError(fmt.Sprintf("typed bulk: aborted group publication at family %s; that family and later families stay on the previous runtime generation: %v", family, err))
			return 0, nil, fmt.Errorf("%w: %s: %w", ErrTypedBulkPublishFailed, family, err)
		}
		publishTypedBulkFamilyOptionMap(family, prepared.familyValues[family], prepared.values)
		if family == "billing_setting" {
			// External side effect: pricing/ratio cache invalidation runs after
			// the commit and is not part of the database atomicity. These are
			// idempotent in-memory projections; there is nothing to roll back.
			InvalidatePricingCache()
			ratio_setting.InvalidateExposedDataCache()
		}
	}
	return newRevision, prepared.sortedKeys, nil
}

// publishTypedBulkFamilyOptionMap writes the committed values of one
// successfully published family into OptionMap. The group ratio compatibility
// aliases are written in the same locked section so readers never observe two
// meanings for one setting.
func publishTypedBulkFamilyOptionMap(family string, familyValues map[string]string, values map[string]string) {
	common.OptionMapRWMutex.Lock()
	defer common.OptionMapRWMutex.Unlock()
	for field := range familyValues {
		key := family + "." + field
		value := values[key]
		common.OptionMap[key] = value
		if family == "group_ratio_setting" {
			switch field {
			case "group_ratio":
				common.OptionMap[groupRatioOptionKey] = value
			case "group_group_ratio":
				common.OptionMap[groupGroupRatioOptionKey] = value
			}
		}
	}
}
