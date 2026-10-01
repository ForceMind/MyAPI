package model

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	sqlitedriver "github.com/glebarez/go-sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ChannelQuotaIdentityQualityProviderConfirmed = "provider_confirmed"
	ChannelQuotaIdentityQualityCredentialScoped  = "credential_scoped"
	channelQuotaIdentityRegistryProtocolVersion  = 1
	channelQuotaIdentityResolveMaxAttempts       = 32
)

var (
	ErrChannelQuotaIdentityAliasConflict    = errors.New("channel quota identity aliases conflict")
	ErrChannelQuotaIdentityRegistryConflict = errors.New("channel quota identity key registry conflict")
	ErrChannelQuotaIdentityRegistryCorrupt  = errors.New("channel quota identity key registry is invalid")
)

// ChannelQuotaIdentityKeyRegistry is the singleton serialization gate and
// current active-version marker for the persistent version registry. Old
// nodes may keep using a previously registered active version, while the
// current keyring cannot silently remove registered history.
type ChannelQuotaIdentityKeyRegistry struct {
	Id              int    `json:"-" gorm:"primaryKey;autoIncrement:false;<-:create"`
	ProtocolVersion int    `json:"-" gorm:"not null;<-:create"`
	ActiveVersion   string `json:"-" gorm:"type:varchar(32);not null"`
	BootstrapNonce  string `json:"-" gorm:"type:char(43);not null;<-:create"`
	CreatedAt       int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
}

func (ChannelQuotaIdentityKeyRegistry) TableName() string {
	return "channel_quota_identity_key_registries"
}

// ChannelQuotaIdentityKeyVersion binds one canonical version identifier to a
// fixed-domain fingerprint. The secret key is never persisted.
type ChannelQuotaIdentityKeyVersion struct {
	Version     string `json:"-" gorm:"primaryKey;type:varchar(32);<-:create"`
	Fingerprint string `json:"-" gorm:"type:char(64);not null;uniqueIndex:uidx_channel_quota_identity_key_fingerprint;<-:create"`
	CreatedAt   int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
}

func (ChannelQuotaIdentityKeyVersion) TableName() string {
	return "channel_quota_identity_key_versions"
}

// ChannelQuotaIdentityAlias maps a key-versioned lookup HMAC to a random,
// opaque subject. It never stores provider account ids, credentials, tokens,
// or their raw source material.
type ChannelQuotaIdentityAlias struct {
	Id         int64  `json:"-" gorm:"primaryKey"`
	KeyVersion string `json:"-" gorm:"type:varchar(32);not null;uniqueIndex:uidx_channel_quota_identity_alias,priority:1;<-:create"`
	LookupHMAC string `json:"-" gorm:"type:char(64);not null;uniqueIndex:uidx_channel_quota_identity_alias,priority:2;<-:create"`
	SubjectRef string `json:"-" gorm:"type:char(43);not null;index:idx_channel_quota_identity_subject;<-:create"`
	CreatedAt  int64  `json:"-" gorm:"type:bigint;not null;<-:create"`
}

func (ChannelQuotaIdentityAlias) TableName() string {
	return "channel_quota_identity_aliases"
}

// ChannelQuotaResolvedIdentity is the redacted identity returned to quota
// aggregation and alerting callers.
type ChannelQuotaResolvedIdentity struct {
	SubjectRef string
	Quality    string
}

type channelQuotaIdentityLookup struct {
	version     string
	fingerprint string
	digest      string
}

// EnsureChannelQuotaIdentityKeyring explicitly validates and registers a
// deployment keyring without resolving identity material. Startup paths can
// call it before enabling quota sampling.
func EnsureChannelQuotaIdentityKeyring(ctx context.Context, db *gorm.DB, keyring common.ChannelQuotaIdentityKeyring) error {
	if db == nil || db.Dialector == nil {
		return gorm.ErrInvalidDB
	}
	if !db.Migrator().HasTable(&ChannelQuotaIdentityKeyRegistry{}) ||
		!db.Migrator().HasTable(&ChannelQuotaIdentityKeyVersion{}) ||
		!db.Migrator().HasTable(&ChannelQuotaIdentityAlias{}) ||
		!db.Migrator().HasIndex(&ChannelQuotaIdentityKeyVersion{}, "uidx_channel_quota_identity_key_fingerprint") ||
		!db.Migrator().HasIndex(&ChannelQuotaIdentityAlias{}, "uidx_channel_quota_identity_alias") {
		return ErrChannelQuotaIdentityRegistryCorrupt
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := common.ValidateChannelQuotaIdentityKeyring(keyring); err != nil {
		return err
	}
	keys := append([]common.ChannelQuotaIdentityKey{keyring.Active}, keyring.Retired...)
	lookups := make([]channelQuotaIdentityLookup, 0, len(keys))
	for _, key := range keys {
		fingerprint, err := common.ChannelQuotaIdentityKeyFingerprint(key)
		if err != nil {
			return err
		}
		lookups = append(lookups, channelQuotaIdentityLookup{version: key.Version, fingerprint: fingerprint})
	}
	sort.Slice(lookups, func(i, j int) bool { return lookups[i].version < lookups[j].version })
	return runChannelQuotaIdentityTransaction(ctx, db, func(tx *gorm.DB) error {
		return registerChannelQuotaIdentityKeyring(tx, lookups, keyring.Active.Version)
	})
}

// ResolveChannelQuotaIdentity resolves an upstream account id or credential
// to a stable opaque subject. Every key in an overlapping keyring receives an
// alias in one transaction, ordered by version and digest. A persistent gate
// serializes registry changes and first-writer subject creation across nodes.
func ResolveChannelQuotaIdentity(ctx context.Context, db *gorm.DB, keyring common.ChannelQuotaIdentityKeyring, provider, identityKind string, material []byte) (ChannelQuotaResolvedIdentity, error) {
	if db == nil || db.Dialector == nil {
		return ChannelQuotaResolvedIdentity{}, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ChannelQuotaResolvedIdentity{}, err
	}
	if err := common.ValidateChannelQuotaIdentityKeyring(keyring); err != nil {
		return ChannelQuotaResolvedIdentity{}, err
	}

	keys := append([]common.ChannelQuotaIdentityKey{keyring.Active}, keyring.Retired...)
	lookups := make([]channelQuotaIdentityLookup, 0, len(keys))
	for _, key := range keys {
		fingerprint, err := common.ChannelQuotaIdentityKeyFingerprint(key)
		if err != nil {
			return ChannelQuotaResolvedIdentity{}, err
		}
		digest, err := common.ChannelQuotaIdentityLookupHMAC(key, provider, identityKind, material)
		if err != nil {
			return ChannelQuotaResolvedIdentity{}, err
		}
		lookups = append(lookups, channelQuotaIdentityLookup{version: key.Version, fingerprint: fingerprint, digest: digest})
	}
	sort.Slice(lookups, func(i, j int) bool {
		if lookups[i].version != lookups[j].version {
			return lookups[i].version < lookups[j].version
		}
		return lookups[i].digest < lookups[j].digest
	})

	quality := ChannelQuotaIdentityQualityCredentialScoped
	if identityKind == common.ChannelQuotaIdentityKindProviderAccount {
		quality = ChannelQuotaIdentityQualityProviderConfirmed
	}
	var resolved ChannelQuotaResolvedIdentity
	err := runChannelQuotaIdentityTransaction(ctx, db, func(tx *gorm.DB) error {
		if err := registerChannelQuotaIdentityKeyring(tx, lookups, keyring.Active.Version); err != nil {
			return err
		}
		subject, err := resolveChannelQuotaIdentityAliases(tx, lookups)
		if err != nil {
			return err
		}
		resolved = ChannelQuotaResolvedIdentity{SubjectRef: subject, Quality: quality}
		return nil
	})
	if err != nil {
		return ChannelQuotaResolvedIdentity{}, err
	}
	return resolved, nil
}

// LookupChannelQuotaIdentity reads an already registered subject without
// creating an alias. Routing uses it to bind an account's current credential
// to persisted quota observations without mutating identity state per request.
func LookupChannelQuotaIdentity(ctx context.Context, db *gorm.DB, keyring common.ChannelQuotaIdentityKeyring, provider, identityKind string, material []byte) (ChannelQuotaResolvedIdentity, bool, error) {
	if db == nil || db.Dialector == nil {
		return ChannelQuotaResolvedIdentity{}, false, gorm.ErrInvalidDB
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ChannelQuotaResolvedIdentity{}, false, err
	}
	if err := common.ValidateChannelQuotaIdentityKeyring(keyring); err != nil {
		return ChannelQuotaResolvedIdentity{}, false, err
	}
	keys := append([]common.ChannelQuotaIdentityKey{keyring.Active}, keyring.Retired...)
	lookupByVersion := make(map[string]string, len(keys))
	digests := make([]string, 0, len(keys))
	for _, key := range keys {
		digest, err := common.ChannelQuotaIdentityLookupHMAC(key, provider, identityKind, material)
		if err != nil {
			return ChannelQuotaResolvedIdentity{}, false, err
		}
		lookupByVersion[key.Version] = digest
		digests = append(digests, digest)
	}
	var aliases []ChannelQuotaIdentityAlias
	if err := db.WithContext(ctx).Where("lookup_hmac IN ?", digests).Find(&aliases).Error; err != nil {
		return ChannelQuotaResolvedIdentity{}, false, err
	}
	if len(aliases) == 0 {
		return ChannelQuotaResolvedIdentity{}, false, nil
	}
	subject := ""
	for _, alias := range aliases {
		if lookupByVersion[alias.KeyVersion] != alias.LookupHMAC || !canonicalChannelQuotaIdentitySubject(alias.SubjectRef) ||
			subject != "" && subject != alias.SubjectRef {
			return ChannelQuotaResolvedIdentity{}, false, ErrChannelQuotaIdentityRegistryCorrupt
		}
		subject = alias.SubjectRef
	}
	quality := ChannelQuotaIdentityQualityCredentialScoped
	if identityKind == common.ChannelQuotaIdentityKindProviderAccount {
		quality = ChannelQuotaIdentityQualityProviderConfirmed
	}
	return ChannelQuotaResolvedIdentity{SubjectRef: subject, Quality: quality}, true, nil
}

func registerChannelQuotaIdentityKeyring(tx *gorm.DB, lookups []channelQuotaIdentityLookup, activeVersion string) error {
	now := time.Now().Unix()
	bootstrapNonce, err := newChannelQuotaIdentitySubjectRef()
	if err != nil {
		return err
	}
	gate := ChannelQuotaIdentityKeyRegistry{
		Id: 1, ProtocolVersion: channelQuotaIdentityRegistryProtocolVersion, ActiveVersion: activeVersion, BootstrapNonce: bootstrapNonce, CreatedAt: now,
	}
	gateInsert := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&gate)
	if gateInsert.Error != nil {
		return gateInsert.Error
	}
	if err := lockForUpdate(tx).Where("id = ?", 1).Take(&gate).Error; err != nil {
		return err
	}
	gateCreated := constantTimeStringEqual(gate.BootstrapNonce, bootstrapNonce)
	if gate.ProtocolVersion != channelQuotaIdentityRegistryProtocolVersion || !canonicalChannelQuotaIdentityVersion(gate.ActiveVersion) ||
		!canonicalChannelQuotaIdentitySubject(gate.BootstrapNonce) {
		return ErrChannelQuotaIdentityRegistryCorrupt
	}

	var registered []ChannelQuotaIdentityKeyVersion
	if err := lockForUpdate(tx).Order("version ASC").Find(&registered).Error; err != nil {
		return err
	}
	if (gateCreated && len(registered) != 0) || (!gateCreated && len(registered) == 0) {
		return ErrChannelQuotaIdentityRegistryCorrupt
	}
	provided := make(map[string]channelQuotaIdentityLookup, len(lookups))
	for _, lookup := range lookups {
		provided[lookup.version] = lookup
	}
	registeredByVersion := make(map[string]ChannelQuotaIdentityKeyVersion, len(registered))
	registeredFingerprints := make(map[string]struct{}, len(registered))
	for _, item := range registered {
		if !canonicalChannelQuotaIdentityVersion(item.Version) || !canonicalLowerHex(item.Fingerprint, 64) {
			return ErrChannelQuotaIdentityRegistryCorrupt
		}
		if _, exists := registeredFingerprints[item.Fingerprint]; exists {
			return ErrChannelQuotaIdentityRegistryCorrupt
		}
		registeredFingerprints[item.Fingerprint] = struct{}{}
		registeredByVersion[item.Version] = item
		if lookup, exists := provided[item.Version]; exists && !constantTimeStringEqual(item.Fingerprint, lookup.fingerprint) {
			return ErrChannelQuotaIdentityRegistryConflict
		}
	}
	if !gateCreated {
		if _, exists := registeredByVersion[gate.ActiveVersion]; !exists {
			return ErrChannelQuotaIdentityRegistryCorrupt
		}
	}

	currentConfiguration := activeVersion == gate.ActiveVersion
	if len(registered) != 0 {
		if currentConfiguration {
			for _, item := range registered {
				if _, exists := provided[item.Version]; !exists {
					return ErrChannelQuotaIdentityRegistryConflict
				}
			}
		} else if _, knownOldActive := registeredByVersion[activeVersion]; knownOldActive {
			if _, attemptsRollback := provided[gate.ActiveVersion]; attemptsRollback {
				return ErrChannelQuotaIdentityRegistryConflict
			}
			for _, lookup := range lookups {
				if _, exists := registeredByVersion[lookup.version]; !exists {
					return ErrChannelQuotaIdentityRegistryConflict
				}
			}
		} else {
			for _, item := range registered {
				if _, exists := provided[item.Version]; !exists {
					return ErrChannelQuotaIdentityRegistryConflict
				}
			}
			if _, overlaps := provided[gate.ActiveVersion]; !overlaps {
				return ErrChannelQuotaIdentityRegistryConflict
			}
		}
	}

	for _, lookup := range lookups {
		if _, exists := registeredByVersion[lookup.version]; exists {
			continue
		}
		candidate := ChannelQuotaIdentityKeyVersion{Version: lookup.version, Fingerprint: lookup.fingerprint, CreatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return err
		}
		var persisted ChannelQuotaIdentityKeyVersion
		if err := lockForUpdate(tx).Where("version = ?", lookup.version).Take(&persisted).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelQuotaIdentityRegistryConflict
			}
			return err
		}
		if !canonicalLowerHex(persisted.Fingerprint, 64) || !constantTimeStringEqual(persisted.Fingerprint, lookup.fingerprint) {
			return ErrChannelQuotaIdentityRegistryConflict
		}
		registeredByVersion[lookup.version] = persisted
	}

	if activeVersion != gate.ActiveVersion {
		if _, staleNode := registeredByVersion[activeVersion]; staleNode {
			if _, hasCurrent := provided[gate.ActiveVersion]; !hasCurrent {
				return nil
			}
		}
		result := tx.Model(&ChannelQuotaIdentityKeyRegistry{}).
			Where("id = ? AND active_version = ?", 1, gate.ActiveVersion).
			Update("active_version", activeVersion)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrChannelQuotaIdentityRegistryConflict
		}
	}
	return nil
}

func resolveChannelQuotaIdentityAliases(tx *gorm.DB, lookups []channelQuotaIdentityLookup) (string, error) {
	var subject string
	for _, lookup := range lookups {
		var alias ChannelQuotaIdentityAlias
		err := tx.Where("key_version = ? AND lookup_hmac = ?", lookup.version, lookup.digest).Take(&alias).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := validateChannelQuotaIdentityAlias(alias, lookup); err != nil {
			return "", err
		}
		if subject == "" {
			subject = alias.SubjectRef
		} else if !constantTimeStringEqual(subject, alias.SubjectRef) {
			return "", ErrChannelQuotaIdentityAliasConflict
		}
	}
	if subject == "" {
		var err error
		subject, err = newChannelQuotaIdentitySubjectRef()
		if err != nil {
			return "", err
		}
	}

	for index, lookup := range lookups {
		candidate := ChannelQuotaIdentityAlias{
			KeyVersion: lookup.version, LookupHMAC: lookup.digest, SubjectRef: subject, CreatedAt: time.Now().Unix(),
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "key_version"}, {Name: "lookup_hmac"}}, DoNothing: true,
		}).Create(&candidate).Error; err != nil {
			return "", err
		}
		var persisted ChannelQuotaIdentityAlias
		if err := lockForUpdate(tx).Where("key_version = ? AND lookup_hmac = ?", lookup.version, lookup.digest).Take(&persisted).Error; err != nil {
			return "", err
		}
		if err := validateChannelQuotaIdentityAlias(persisted, lookup); err != nil {
			return "", err
		}
		if index == 0 {
			subject = persisted.SubjectRef
		} else if !constantTimeStringEqual(subject, persisted.SubjectRef) {
			return "", ErrChannelQuotaIdentityAliasConflict
		}
	}
	return subject, nil
}

func validateChannelQuotaIdentityAlias(alias ChannelQuotaIdentityAlias, lookup channelQuotaIdentityLookup) error {
	if alias.KeyVersion != lookup.version || !canonicalLowerHex(alias.LookupHMAC, 64) ||
		!constantTimeStringEqual(alias.LookupHMAC, lookup.digest) || !canonicalChannelQuotaIdentitySubject(alias.SubjectRef) {
		return ErrChannelQuotaIdentityRegistryCorrupt
	}
	return nil
}

func canonicalChannelQuotaIdentityVersion(value string) bool {
	if value == "" || len(value) > 32 || value != strings.ToLower(value) {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || (index > 0 && (char == '.' || char == '_' || char == '-')) {
			continue
		}
		return false
	}
	return true
}

func canonicalLowerHex(value string, expectedLength int) bool {
	if len(value) != expectedLength || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded)*2 == expectedLength
}

func canonicalChannelQuotaIdentitySubject(value string) bool {
	if len(value) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == value
}

func constantTimeStringEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func newChannelQuotaIdentitySubjectRef() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func runChannelQuotaIdentityTransaction(ctx context.Context, db *gorm.DB, operation func(*gorm.DB) error) error {
	var lastErr error
	for attempt := 0; attempt < channelQuotaIdentityResolveMaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = db.WithContext(ctx).Transaction(operation)
		if lastErr == nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !channelQuotaIdentityRetryable(db, lastErr) {
			return lastErr
		}
		delay := time.Millisecond << min(attempt, 6)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func channelQuotaIdentityRetryable(db *gorm.DB, err error) bool {
	if db == nil || db.Dialector == nil || err == nil {
		return false
	}
	var sqliteErr *sqlitedriver.Error
	if errors.As(err, &sqliteErr) {
		code := sqliteErr.Code() & 0xff
		return code == 5 || code == 6
	}
	var mysqlErr *mysqldriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1205 || mysqlErr.Number == 1213
	}
	var postgresErr *pgconn.PgError
	if errors.As(err, &postgresErr) {
		return postgresErr.Code == "40001" || postgresErr.Code == "40P01"
	}
	message := strings.ToLower(err.Error())
	switch db.Dialector.Name() {
	case "sqlite":
		return strings.Contains(message, "sqlite_busy") || strings.Contains(message, "sqlite_locked") ||
			strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") ||
			strings.Contains(message, "database is deadlocked")
	case "mysql":
		return strings.Contains(message, "error 1205") || strings.Contains(message, "error 1213")
	case "postgres":
		return strings.Contains(message, "sqlstate 40001") || strings.Contains(message, "sqlstate 40p01")
	default:
		return false
	}
}
