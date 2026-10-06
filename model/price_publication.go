package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const pricePublicationStateKey = "official_price_publication_state"
const publicationModeKey = "billing_setting.billing_mode"
const publicationExpressionKey = "billing_setting.billing_expr"

var publicationRatioOptionKeys = []string{"ModelRatio", "ModelPrice", "CompletionRatio", "CacheRatio", "CreateCacheRatio", "ImageRatio", "AudioRatio", "AudioCompletionRatio"}

var ErrPricePublicationConflict = errors.New("price publication has changed")
var ErrPricePublicationLocked = errors.New("model price is locked")
var ErrPricePublicationImmutable = errors.New("price publication receipt is immutable")

// PricePublication is immutable. The before/after evidence includes the whole
// pricing generation; rollback requires an exact match, so it cannot overwrite
// intervening manual changes, including changes to unselected models.
type PricePublication struct {
	ID            string `gorm:"primaryKey;size:64" json:"id"`
	RequestDigest string `gorm:"size:64" json:"request_digest"`
	ActorID       int    `json:"actor_id"`
	Action        string `gorm:"size:16" json:"action"`
	RollbackOf    string `gorm:"size:64" json:"rollback_of,omitempty"`
	BeforeJSON    string `gorm:"size:1048576" json:"-"`
	AfterJSON     string `gorm:"size:1048576" json:"-"`
	CreatedAt     int64  `json:"created_at"`
	Revision      int64  `json:"revision"`
}

func (*PricePublication) BeforeUpdate(*gorm.DB) error { return ErrPricePublicationImmutable }
func (*PricePublication) BeforeDelete(*gorm.DB) error { return ErrPricePublicationImmutable }

type PublishedModelPrice struct {
	PublicationID    string `json:"publication_id"`
	SourceSHA256     string `json:"source_sha256"`
	ExpressionSHA256 string `json:"expression_sha256"`
	Locked           bool   `json:"locked"`
}

type PricePublicationState struct {
	Revision int64                          `json:"revision"`
	Models   map[string]PublishedModelPrice `json:"models"`
}

var pricePublicationRuntime atomic.Pointer[PricePublicationState]

// The caller holds AcquirePricingRuntimeRead while pairing this reference
// with the expression. A manual edit invalidates provenance by hash even if
// the administrator has retained the old publication history.
func PublishedModelPriceForExpression(modelName, expression string) (PublishedModelPrice, bool) {
	state := pricePublicationRuntime.Load()
	if state == nil {
		return PublishedModelPrice{}, false
	}
	price, ok := state.Models[modelName]
	if !ok || price.PublicationID == "" || price.ExpressionSHA256 != billingexpr.ExprHashString(expression) {
		return PublishedModelPrice{}, false
	}
	return price, true
}

func publishPricePublicationState(raw string) error {
	state := &PricePublicationState{}
	if raw != "" {
		if err := common.UnmarshalJsonStr(raw, state); err != nil {
			return err
		}
	}
	if state.Revision < 0 {
		return fmt.Errorf("invalid price publication revision")
	}
	for name, binding := range state.Models {
		if name == "" || len(name) > 191 {
			return fmt.Errorf("invalid published model name")
		}
		if binding.PublicationID == "" && binding.SourceSHA256 == "" && binding.ExpressionSHA256 == "" {
			continue
		}
		for _, digest := range []string{binding.PublicationID, binding.SourceSHA256, binding.ExpressionSHA256} {
			decoded, err := hex.DecodeString(digest)
			if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest {
				return fmt.Errorf("invalid price publication evidence")
			}
		}
	}
	pricePublicationRuntime.Store(state)
	return nil
}

type PricePublicationSnapshot struct {
	Modes             map[string]string     `json:"modes"`
	Expressions       map[string]string     `json:"expressions"`
	State             PricePublicationState `json:"state"`
	OtherPriceOptions map[string]string     `json:"other_price_options"`
}

type PricePublicationChange struct {
	Model        string `json:"model"`
	Expression   string `json:"expression"`
	SourceSHA256 string `json:"source_sha256"`
	Locked       bool   `json:"locked"`
}

// Expression/source pairs are server-built service inputs, never accepted
// directly as rates by the public controller.
type PricePublicationCommand struct {
	ID             string                   `json:"id"`
	ActorID        int                      `json:"actor_id"`
	ExpectedDigest string                   `json:"expected_digest"`
	Action         string                   `json:"action"`
	RollbackOf     string                   `json:"rollback_of,omitempty"`
	Changes        []PricePublicationChange `json:"changes,omitempty"`
}

func (s PricePublicationSnapshot) Digest() (string, error) {
	data, err := common.Marshal(s)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}

func readPricePublicationSnapshotTx(tx *gorm.DB, locked bool) (*PricePublicationSnapshot, error) {
	keys := []string{publicationExpressionKey, publicationModeKey, pricePublicationStateKey}
	if locked {
		// All pricing writers use the same deterministic row order. Initial
		// empty rows make first publication serializable without a missing-row
		// assumption on SQLite, MySQL or PostgreSQL.
		for _, key := range keys {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Option{Key: key, Value: "{}"}).Error; err != nil {
				return nil, err
			}
		}
		tx = lockForUpdate(tx)
	}
	// Do not create absent ratio rows: absence preserves built-in defaults.
	// All participating writers first lock the three coordination rows above.
	keys = append(keys, publicationRatioOptionKeys...)
	var rows []Option
	if err := tx.Where(map[string]interface{}{"key": keys}).Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := &PricePublicationSnapshot{Modes: map[string]string{}, Expressions: map[string]string{}, State: PricePublicationState{Models: map[string]PublishedModelPrice{}}, OtherPriceOptions: map[string]string{}}
	for _, row := range rows {
		var target any
		switch row.Key {
		case publicationModeKey:
			target = &result.Modes
		case publicationExpressionKey:
			target = &result.Expressions
		case pricePublicationStateKey:
			target = &result.State
		default:
			result.OtherPriceOptions[row.Key] = row.Value
			continue
		}
		if row.Value == "" {
			continue
		}
		if err := common.UnmarshalJsonStr(row.Value, target); err != nil {
			return nil, err
		}
	}
	if result.Modes == nil {
		result.Modes = map[string]string{}
	}
	if result.Expressions == nil {
		result.Expressions = map[string]string{}
	}
	if result.State.Models == nil {
		result.State.Models = map[string]PublishedModelPrice{}
	}
	if result.State.Revision < 0 {
		return nil, fmt.Errorf("invalid price publication revision")
	}
	return result, nil
}

func ReadPricePublicationSnapshot(ctx context.Context) (*PricePublicationSnapshot, error) {
	return readPricePublicationSnapshotTx(DB.WithContext(ctx), false)
}

// validateLockedPriceChangesTx also serializes legacy pricing writes with
// publication. It must run inside their transaction, before persisting options.
func validateLockedPriceChangesTx(tx *gorm.DB, values map[string]string) error {
	_, modeChanged := values[publicationModeKey]
	_, expressionChanged := values[publicationExpressionKey]
	otherPriceChanged := false
	for _, key := range publicationRatioOptionKeys {
		if _, ok := values[key]; ok {
			otherPriceChanged = true
		}
	}
	if !modeChanged && !expressionChanged && !otherPriceChanged {
		return nil
	}
	before, err := readPricePublicationSnapshotTx(tx, true)
	if err != nil {
		return err
	}
	modes, expressions := before.Modes, before.Expressions
	if modeChanged {
		modes = nil
		if err := common.UnmarshalJsonStr(values[publicationModeKey], &modes); err != nil {
			return err
		}
	}
	if expressionChanged {
		expressions = nil
		if err := common.UnmarshalJsonStr(values[publicationExpressionKey], &expressions); err != nil {
			return err
		}
	}
	for name, binding := range before.State.Models {
		if binding.Locked && (modes[name] != before.Modes[name] || expressions[name] != before.Expressions[name]) {
			return ErrPricePublicationLocked
		}
	}
	return nil
}

func ApplyPricePublication(ctx context.Context, command PricePublicationCommand) (*PricePublication, error) {
	for _, digest := range []string{command.ID, command.ExpectedDigest} {
		decoded, err := hex.DecodeString(digest)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != digest {
			return nil, fmt.Errorf("invalid publication identity or expected digest")
		}
	}
	if command.ActorID <= 0 || len(command.Changes) > 64 {
		return nil, fmt.Errorf("invalid publication actor or model count")
	}
	if command.Action != "publish" && command.Action != "lock" && command.Action != "rollback" {
		return nil, fmt.Errorf("invalid publication action")
	}
	if command.Action != "rollback" && (len(command.Changes) == 0 || command.RollbackOf != "") {
		return nil, fmt.Errorf("invalid publication changes")
	}
	if command.Action == "rollback" && (len(command.Changes) != 0 || len(command.RollbackOf) != 64) {
		return nil, fmt.Errorf("invalid rollback target")
	}
	seen := map[string]bool{}
	for _, change := range command.Changes {
		if change.Model == "" || len(change.Model) > 191 || seen[change.Model] {
			return nil, fmt.Errorf("invalid or duplicate publication model")
		}
		seen[change.Model] = true
	}
	sort.Slice(command.Changes, func(i, j int) bool { return command.Changes[i].Model < command.Changes[j].Model })
	raw, err := common.Marshal(command)
	if err != nil {
		return nil, err
	}
	requestDigest := fmt.Sprintf("%x", sha256.Sum256(raw))
	optionMutationLock.Lock()
	defer optionMutationLock.Unlock()
	var result PricePublication
	var values map[string]string
	readyToCommit := false
	err = DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		before, err := readPricePublicationSnapshotTx(tx, true)
		if err != nil {
			return err
		}
		err = tx.Where("id = ?", command.ID).Take(&result).Error
		if err == nil {
			if result.ID != command.ID || result.RequestDigest != requestDigest {
				return ErrPricePublicationConflict
			}
			// A retry returns the historical receipt, never republishes its old
			// options over a newer generation.
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		digest, err := before.Digest()
		if err != nil {
			return err
		}
		if digest != command.ExpectedDigest {
			return ErrPricePublicationConflict
		}
		beforeJSON, err := common.Marshal(before)
		if err != nil {
			return err
		}
		if len(beforeJSON) > MaxOfficialPriceDocumentBytes {
			return fmt.Errorf("price publication evidence exceeds capacity")
		}
		after := *before
		if command.Action == "rollback" {
			var target PricePublication
			if err := tx.Where("id = ?", command.RollbackOf).Take(&target).Error; err != nil {
				return err
			}
			if target.ID != command.RollbackOf || target.AfterJSON != string(beforeJSON) {
				return ErrPricePublicationConflict
			}
			after = PricePublicationSnapshot{}
			if err := common.UnmarshalJsonStr(target.BeforeJSON, &after); err != nil {
				return err
			}
		} else {
			for _, change := range command.Changes {
				binding := after.State.Models[change.Model]
				if command.Action == "lock" {
					if after.Modes[change.Model] != "tiered_expr" {
						return fmt.Errorf("price locks require expression pricing")
					}
					if change.Expression != "" || change.SourceSHA256 != "" {
						return fmt.Errorf("lock cannot change a price")
					}
					binding.Locked = change.Locked
					after.State.Models[change.Model] = binding
					continue
				}
				if binding.Locked {
					return ErrPricePublicationLocked
				}
				if _, err := GetOfficialPriceVersion(ctx, tx, change.SourceSHA256); err != nil {
					return err
				}
				if change.Expression == "" || len(change.Expression) > 8192 {
					return fmt.Errorf("invalid publication expression")
				}
				after.Modes[change.Model] = "tiered_expr"
				after.Expressions[change.Model] = change.Expression
				after.State.Models[change.Model] = PublishedModelPrice{PublicationID: command.ID, SourceSHA256: change.SourceSHA256, ExpressionSHA256: billingexpr.ExprHashString(change.Expression), Locked: change.Locked}
			}
		}
		if before.State.Revision == int64(^uint64(0)>>1) {
			return fmt.Errorf("price publication revision exhausted")
		}
		after.State.Revision = before.State.Revision + 1
		values = make(map[string]string, 3)
		for key, value := range map[string]any{publicationModeKey: after.Modes, publicationExpressionKey: after.Expressions, pricePublicationStateKey: after.State} {
			encoded, err := common.Marshal(value)
			if err != nil {
				return err
			}
			if len(encoded) > 65536 {
				return fmt.Errorf("price publication generation exceeds option capacity")
			}
			values[key] = string(encoded)
		}
		if err := validateModelPricingOptions(map[string]string{publicationModeKey: values[publicationModeKey], publicationExpressionKey: values[publicationExpressionKey]}); err != nil {
			return err
		}
		afterJSON, err := common.Marshal(after)
		if err != nil {
			return err
		}
		if len(afterJSON) > MaxOfficialPriceDocumentBytes {
			return fmt.Errorf("price publication evidence exceeds capacity")
		}
		result = PricePublication{ID: command.ID, RequestDigest: requestDigest, ActorID: command.ActorID, Action: command.Action, RollbackOf: command.RollbackOf, BeforeJSON: string(beforeJSON), AfterJSON: string(afterJSON), CreatedAt: time.Now().UTC().Unix(), Revision: after.State.Revision}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		if err := persistOptionValuesTx(tx, values); err != nil {
			return err
		}
		readyToCommit = true
		return nil
	})
	if err != nil {
		if readyToCommit {
			markPricingRuntimeUnavailable()
		}
		return nil, err
	}
	if values != nil {
		state := values[pricePublicationStateKey]
		delete(values, pricePublicationStateKey)
		if err := publishModelPricingOptions(values); err != nil {
			return &result, fmt.Errorf("price committed but runtime publication failed: %w", err)
		}
		if err := publishPricePublicationState(state); err != nil {
			markPricingRuntimeUnavailable()
			return &result, fmt.Errorf("price committed but evidence publication failed: %w", err)
		}
	}
	return &result, nil
}
