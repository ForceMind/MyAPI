package model

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"errors"
	"math"
	"math/big"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

var ErrEditionCompatibilityUnverifiable = errors.New("cannot verify edition compatibility before migration")

type EditionOrderFacts struct {
	Total    int64 `json:"total"`
	Pending  int64 `json:"pending"`
	Terminal int64 `json:"terminal"`
	Unknown  int64 `json:"unknown"`
}

type EditionAuthenticationFacts struct {
	PasswordLoginEnabled     *bool  `json:"password_login_enabled"`
	PasswordLoginSource      string `json:"password_login_source"`
	PasskeyEnabled           *bool  `json:"passkey_enabled"`
	PasskeySource            string `json:"passkey_source"`
	EnabledUsers             int64  `json:"enabled_users"`
	EnabledRootUsers         int64  `json:"enabled_root_users"`
	PasswordCandidates       int64  `json:"password_candidates"`
	PasskeyCandidates        int64  `json:"passkey_candidates"`
	LocalCandidates          int64  `json:"local_candidates"`
	RootLocalCandidates      int64  `json:"root_local_candidates"`
	MissingLocalCandidates   int64  `json:"missing_local_candidates"`
	UnknownUsers             int64  `json:"unknown_users"`
	UnknownCredentialRecords int64  `json:"unknown_credential_records"`
}

type EditionCompatibilityFacts struct {
	SchemaVersion      int                         `json:"schema_version"`
	ReadOnly           bool                        `json:"read_only"`
	Schema             string                      `json:"schema"`
	Source             string                      `json:"source"`
	Database           string                      `json:"database"`
	Complete           bool                        `json:"complete"`
	TopUps             *EditionOrderFacts          `json:"top_ups"`
	SubscriptionOrders *EditionOrderFacts          `json:"subscription_orders"`
	Authentication     *EditionAuthenticationFacts `json:"authentication"`
	LoginContinuity    string                      `json:"login_continuity"`
}

var editionRequiredColumns = map[string][]string{
	"top_ups":             {"id", "status"},
	"subscription_orders": {"id", "status"},
	"users":               {"id", "role", "status", "password", "deleted_at"},
	"options":             {"key", "value"},
	"passkey_credentials": {"user_id", "credential_id", "public_key", "deleted_at"},
}

// InspectEditionCompatibilityPreMigration reads the same resolved primary that
// InitDB will open. It never opens the log database, migrates schema, publishes
// settings, or proves that a person can complete a login.
func InspectEditionCompatibilityPreMigration(ctx context.Context) (EditionCompatibilityFacts, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	facts := EditionCompatibilityFacts{SchemaVersion: 1, ReadOnly: true, Schema: "unavailable", Source: "configured-primary", Database: "unknown", LoginContinuity: "not-verified"}
	if ctx.Err() != nil {
		return facts, ErrEditionCompatibilityUnverifiable
	}
	target, err := resolveDatabaseTarget("SQL_DSN", false)
	if err != nil {
		return facts, ErrEditionCompatibilityUnverifiable
	}
	facts.Database = string(target.databaseType)
	switch target.databaseType {
	case common.DatabaseTypeSQLite:
		if target.dsn == ":memory:" {
			// A bare :memory: opens a distinct new database on every native connection.
			// No probe connection is opened and no existing data is claimed inspected.
			facts.Source, facts.Schema, facts.Complete = "new-anonymous-memory", "fresh", true
			return facts, nil
		}
		probe, cancel := context.WithTimeout(ctx, quotaAmountSQLiteProbeTimeout)
		defer cancel()
		err = inspectEditionSQLite(probe, target.dsn, &facts)
	case common.DatabaseTypeMySQL:
		probe, cancel := context.WithTimeout(ctx, quotaAmountSQLProbeTimeout)
		defer cancel()
		var cfg *mysqldriver.Config
		cfg, err = mysqldriver.ParseDSN(target.dsn)
		if err == nil {
			connector, connectorErr := mysqldriver.NewConnector(cfg)
			if connectorErr != nil {
				err = connectorErr
			} else {
				db := sql.OpenDB(connector)
				defer db.Close()
				err = inspectEditionRelational(probe, db, target.databaseType, &facts)
			}
		}
	case common.DatabaseTypePostgreSQL:
		probe, cancel := context.WithTimeout(ctx, quotaAmountSQLProbeTimeout)
		defer cancel()
		var cfg *pgx.ConnConfig
		cfg, err = pgx.ParseConfig(target.dsn)
		if err == nil {
			cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
			db := stdlib.OpenDB(*cfg)
			defer db.Close()
			err = inspectEditionRelational(probe, db, target.databaseType, &facts)
		}
	default:
		err = ErrEditionCompatibilityUnverifiable
	}
	if err != nil {
		facts.Complete, facts.TopUps, facts.SubscriptionOrders, facts.Authentication = false, nil, nil, nil
		return facts, ErrEditionCompatibilityUnverifiable
	}
	return facts, nil
}

func inspectEditionSQLite(ctx context.Context, dsn string, facts *EditionCompatibilityFacts) error {
	path, memory, err := sqliteDatabasePath(dsn)
	if err != nil {
		return err
	}
	if memory {
		return ErrEditionCompatibilityUnverifiable
	}
	state, err := inspectSQLiteStorage(path)
	if err != nil {
		return err
	}
	if state.fresh {
		facts.Schema, facts.Complete = "fresh", true
		return nil
	}
	var digest [32]byte
	if state.cleanWAL {
		digest, err = fileDigest(ctx, state.path)
		if err != nil {
			return err
		}
		if err = verifyCleanWALSourceMetadata(state); err != nil {
			return err
		}
	}
	db, err := sql.Open(sqlite.DriverName, sqliteReadOnlyURI(state.path, state.cleanWAL))
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	probeErr := func() error {
		defer db.Close()
		conn, e := db.Conn(ctx)
		if e != nil {
			return e
		}
		defer conn.Close()
		if _, e = conn.ExecContext(ctx, "PRAGMA query_only=ON"); e != nil {
			return e
		}
		var enabled int
		if e = conn.QueryRowContext(ctx, "PRAGMA query_only").Scan(&enabled); e != nil || enabled != 1 {
			return ErrEditionCompatibilityUnverifiable
		}
		tx, e := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if e != nil {
			return e
		}
		defer tx.Rollback()
		return inspectEditionSnapshot(ctx, tx, common.DatabaseTypeSQLite, facts)
	}()
	if err = verifySQLiteSidecars(state); err != nil {
		return err
	}
	if state.cleanWAL {
		if err = verifyCleanWALSourceMetadata(state); err != nil {
			return err
		}
		after, e := fileDigest(ctx, state.path)
		if e != nil {
			return e
		}
		if after != digest {
			return ErrEditionCompatibilityUnverifiable
		}
		if err = verifyCleanWALSourceMetadata(state); err != nil {
			return err
		}
	}
	return probeErr
}

func inspectEditionRelational(ctx context.Context, db *sql.DB, dialect common.DatabaseType, facts *EditionCompatibilityFacts) error {
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	return inspectEditionSnapshot(ctx, tx, dialect, facts)
}

func inspectEditionSnapshot(ctx context.Context, tx *sql.Tx, dialect common.DatabaseType, facts *EditionCompatibilityFacts) error {
	var catalog string
	switch dialect {
	case common.DatabaseTypeSQLite:
		catalog = "SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'"
	case common.DatabaseTypeMySQL:
		catalog = "SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.tables WHERE TABLE_SCHEMA = DATABASE()"
	case common.DatabaseTypePostgreSQL:
		catalog = "SELECT c.relname, c.relkind::text FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=ANY(current_schemas(false)) AND pg_catalog.pg_table_is_visible(c.oid) AND c.relkind IN ('r','p','v','m','f')"
	default:
		return ErrEditionCompatibilityUnverifiable
	}
	tables, err := relationalQuotaAmountTables(ctx, tx, catalog)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		facts.Schema, facts.Complete = "fresh", true
		return nil
	}
	for table, required := range editionRequiredColumns {
		if !quotaAmountBaseTable(dialect, tables[table]) {
			facts.Schema = "partial"
			return ErrEditionCompatibilityUnverifiable
		}
		columns, e := editionColumns(ctx, tx, dialect, table)
		if e != nil {
			return e
		}
		for _, column := range required {
			physical, ok := columns[column]
			if !ok || physical.hidden {
				facts.Schema = "partial"
				return ErrEditionCompatibilityUnverifiable
			}
		}
	}
	if dialect == common.DatabaseTypeMySQL {
		// READ ONLY + REPEATABLE READ does not make MyISAM (or an unknown
		// engine) transactional. Every table contributing facts must be InnoDB.
		rows, queryErr := tx.QueryContext(ctx, "SELECT TABLE_NAME,ENGINE FROM information_schema.tables WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('top_ups','subscription_orders','users','options','passkey_credentials')")
		if queryErr != nil {
			return queryErr
		}
		seen := make(map[string]bool)
		for rows.Next() {
			var table string
			var engine sql.NullString
			if queryErr = rows.Scan(&table, &engine); queryErr != nil {
				rows.Close()
				return queryErr
			}
			table = strings.ToLower(table)
			if _, required := editionRequiredColumns[table]; !required || seen[table] || !engine.Valid || !strings.EqualFold(engine.String, "InnoDB") {
				rows.Close()
				return ErrEditionCompatibilityUnverifiable
			}
			seen[table] = true
		}
		if queryErr = rows.Err(); queryErr != nil {
			rows.Close()
			return queryErr
		}
		if queryErr = rows.Close(); queryErr != nil {
			return queryErr
		}
		if len(seen) != len(editionRequiredColumns) {
			return ErrEditionCompatibilityUnverifiable
		}
	}
	facts.Schema = "existing"
	topups, err := inspectEditionOrders(ctx, tx, dialect, "top_ups")
	if err != nil {
		return err
	}
	subscriptions, err := inspectEditionOrders(ctx, tx, dialect, "subscription_orders")
	if err != nil {
		return err
	}
	auth, err := inspectEditionAuthentication(ctx, tx, dialect)
	if err != nil {
		return err
	}
	facts.Schema, facts.Complete = "existing", true
	facts.TopUps, facts.SubscriptionOrders, facts.Authentication = &topups, &subscriptions, &auth
	return nil
}

func editionColumns(ctx context.Context, tx *sql.Tx, dialect common.DatabaseType, table string) (map[string]quotaAmountPhysicalColumn, error) {
	if dialect == common.DatabaseTypeSQLite {
		return sqliteQuotaAmountColumns(ctx, tx, table)
	}
	var query string
	if dialect == common.DatabaseTypeMySQL {
		query = "SELECT TABLE_NAME,COLUMN_NAME,DATA_TYPE,COLUMN_TYPE FROM information_schema.columns WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='" + table + "'"
	} else {
		query = "SELECT c.relname,a.attname,t.typname,t.typname FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace JOIN pg_catalog.pg_attribute a ON a.attrelid=c.oid JOIN pg_catalog.pg_type t ON t.oid=a.atttypid WHERE n.nspname=ANY(current_schemas(false)) AND pg_catalog.pg_table_is_visible(c.oid) AND a.attnum>0 AND NOT a.attisdropped AND c.relname='" + table + "'"
	}
	columns, err := relationalQuotaAmountColumns(ctx, tx, query)
	return columns[table], err
}

func inspectEditionOrders(ctx context.Context, tx *sql.Tx, dialect common.DatabaseType, table string) (EditionOrderFacts, error) {
	var facts EditionOrderFacts
	rows, err := tx.QueryContext(ctx, "SELECT "+quotaExecutionQuote(dialect, "status")+" FROM "+quotaExecutionQuote(dialect, table))
	if err != nil {
		return facts, err
	}
	defer rows.Close()
	for rows.Next() {
		var status sql.NullString
		if err = rows.Scan(&status); err != nil {
			return facts, err
		}
		facts.Total++
		switch {
		case !status.Valid:
			facts.Unknown++
		case status.String == common.TopUpStatusPending:
			facts.Pending++
		case status.String == common.TopUpStatusSuccess || status.String == common.TopUpStatusFailed || status.String == common.TopUpStatusExpired:
			facts.Terminal++
		default:
			facts.Unknown++
		}
	}
	return facts, rows.Err()
}

type editionLoginCandidate struct{ root, password, passkey, unknown bool }

func inspectEditionAuthentication(ctx context.Context, tx *sql.Tx, dialect common.DatabaseType) (EditionAuthenticationFacts, error) {
	facts := EditionAuthenticationFacts{PasswordLoginSource: "startup-default", PasskeySource: "startup-default"}
	passwordDefault, passkeyDefault := common.PasswordLoginEnabled, false
	facts.PasswordLoginEnabled, facts.PasskeyEnabled = &passwordDefault, &passkeyDefault
	placeholders := "?,?"
	if dialect == common.DatabaseTypePostgreSQL {
		placeholders = "$1,$2"
	}
	rows, err := tx.QueryContext(ctx, "SELECT "+quotaExecutionQuote(dialect, "key")+","+quotaExecutionQuote(dialect, "value")+" FROM "+quotaExecutionQuote(dialect, "options")+" WHERE "+quotaExecutionQuote(dialect, "key")+" IN ("+placeholders+")", "PasswordLoginEnabled", "passkey.enabled")
	if err != nil {
		return facts, err
	}
	seen := map[string]bool{}
	for rows.Next() {
		var key, value sql.NullString
		if err = rows.Scan(&key, &value); err != nil {
			rows.Close()
			return facts, err
		}
		if !key.Valid || (key.String != "PasswordLoginEnabled" && key.String != "passkey.enabled") {
			continue
		}
		var result *bool
		if !seen[key.String] && value.Valid && (value.String == "true" || value.String == "false") {
			enabled := value.String == "true"
			result = &enabled
		}
		seen[key.String] = true
		if key.String == "PasswordLoginEnabled" {
			facts.PasswordLoginEnabled = result
			facts.PasswordLoginSource = "persisted"
			if result == nil {
				facts.PasswordLoginSource = "invalid"
			}
		} else {
			facts.PasskeyEnabled = result
			facts.PasskeySource = "persisted"
			if result == nil {
				facts.PasskeySource = "invalid"
			}
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return facts, err
	}
	if err = rows.Close(); err != nil {
		return facts, err
	}
	users := make(map[int64]*editionLoginCandidate)
	rows, err = tx.QueryContext(ctx, "SELECT id,role,status,password FROM "+quotaExecutionQuote(dialect, "users")+" WHERE deleted_at IS NULL")
	if err != nil {
		return facts, err
	}
	for rows.Next() {
		var id, role, status sql.NullInt64
		var password sql.NullString
		if err = rows.Scan(&id, &role, &status, &password); err != nil {
			rows.Close()
			return facts, err
		}
		if status.Valid && status.Int64 == int64(common.UserStatusDisabled) {
			continue
		}
		if !status.Valid || status.Int64 != int64(common.UserStatusEnabled) || !id.Valid || id.Int64 <= 0 || !role.Valid || (role.Int64 != 0 && role.Int64 != 1 && role.Int64 != 10 && role.Int64 != 100) {
			facts.UnknownUsers++
			continue
		}
		candidate := &editionLoginCandidate{root: role.Int64 == int64(common.RoleRootUser)}
		facts.EnabledUsers++
		if candidate.root {
			facts.EnabledRootUsers++
		}
		if _, duplicate := users[id.Int64]; duplicate {
			candidate.unknown = true
		}
		if !password.Valid {
			candidate.unknown = true
		} else if password.String != "" {
			if editionPasswordCandidate(password.String) {
				candidate.password = facts.PasswordLoginEnabled != nil && *facts.PasswordLoginEnabled
			} else {
				candidate.unknown = true
			}
		}
		users[id.Int64] = candidate
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return facts, err
	}
	if err = rows.Close(); err != nil {
		return facts, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT user_id,credential_id,public_key FROM "+quotaExecutionQuote(dialect, "passkey_credentials")+" WHERE deleted_at IS NULL")
	if err != nil {
		return facts, err
	}
	defer rows.Close()
	credentialSeen := make(map[int64]bool)
	for rows.Next() {
		var userID sql.NullInt64
		var credential, public sql.NullString
		if err = rows.Scan(&userID, &credential, &public); err != nil {
			return facts, err
		}
		if !userID.Valid || userID.Int64 <= 0 {
			facts.UnknownCredentialRecords++
			continue
		}
		user := users[userID.Int64]
		if user == nil {
			continue
		}
		if credentialSeen[userID.Int64] {
			user.unknown = true
			facts.UnknownCredentialRecords++
		}
		credentialSeen[userID.Int64] = true
		if !credential.Valid || !public.Valid || !editionPasskeyCandidate(credential.String, public.String) {
			user.unknown = true
			facts.UnknownCredentialRecords++
			continue
		}
		user.passkey = facts.PasskeyEnabled != nil && *facts.PasskeyEnabled
	}
	if err = rows.Err(); err != nil {
		return facts, err
	}
	for _, user := range users {
		if user.password {
			facts.PasswordCandidates++
		}
		if user.passkey {
			facts.PasskeyCandidates++
		}
		if user.password || user.passkey {
			facts.LocalCandidates++
			if user.root {
				facts.RootLocalCandidates++
			}
		} else if !user.unknown {
			facts.MissingLocalCandidates++
		}
		if user.unknown {
			facts.UnknownUsers++
		}
	}
	return facts, nil
}

func editionPasswordCandidate(hash string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "" || (parts[1] != "2" && parts[1] != "2a" && parts[1] != "2b" && parts[1] != "2y") || len(parts[2]) != 2 || len(parts[3]) != 53 {
		return false
	}
	if _, err := bcrypt.Cost([]byte(hash)); err != nil {
		return false
	}
	encoding := base64.NewEncoding("./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789").WithPadding(base64.NoPadding)
	salt, err := encoding.DecodeString(parts[3][:22])
	if err != nil || len(salt) != 16 {
		return false
	}
	checksum, err := encoding.DecodeString(parts[3][22:])
	return err == nil && len(checksum) == 23
}

func editionPasskeyCandidate(credential, encoded string) bool {
	if len(credential) > 65536 || len(encoded) > 65536 {
		return false
	}
	id, err := base64.StdEncoding.Strict().DecodeString(credential)
	if err != nil || len(id) == 0 || base64.StdEncoding.EncodeToString(id) != credential {
		return false
	}
	key, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(key) == 0 || base64.StdEncoding.EncodeToString(key) != encoded {
		return false
	}
	parsed, err := webauthncose.ParsePublicKey(key)
	if err != nil {
		return false
	}
	var public any
	switch k := parsed.(type) {
	case webauthncose.EC2PublicKeyData:
		expected := int64(0)
		switch webauthncose.COSEAlgorithmIdentifier(k.Algorithm) {
		case webauthncose.AlgES256:
			expected = int64(webauthncose.P256)
		case webauthncose.AlgES384:
			expected = int64(webauthncose.P384)
		case webauthncose.AlgES512:
			expected = int64(webauthncose.P521)
		}
		if expected == 0 || k.Curve != expected {
			return false
		}
		ec, e := k.ToECDSA()
		if e != nil {
			return false
		}
		length := (ec.Curve.Params().BitSize + 7) / 8
		if len(k.XCoord) != length || len(k.YCoord) != length || !ec.Curve.IsOnCurve(ec.X, ec.Y) {
			return false
		}
		public = ec
	case webauthncose.OKPPublicKeyData:
		if k.Algorithm != int64(webauthncose.AlgEdDSA) || len(k.XCoord) != ed25519.PublicKeySize {
			return false
		}
		public = ed25519.PublicKey(k.XCoord)
	case webauthncose.RSAPublicKeyData:
		algorithm := webauthncose.COSEAlgorithmIdentifier(k.Algorithm)
		if algorithm != webauthncose.AlgRS1 && algorithm != webauthncose.AlgRS256 && algorithm != webauthncose.AlgRS384 && algorithm != webauthncose.AlgRS512 && algorithm != webauthncose.AlgPS256 && algorithm != webauthncose.AlgPS384 && algorithm != webauthncose.AlgPS512 {
			return false
		}
		exponent := new(big.Int).SetBytes(k.Exponent)
		modulus := new(big.Int).SetBytes(k.Modulus)
		if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > math.MaxInt32 || exponent.Bit(0) == 0 || modulus.BitLen() < 1024 || modulus.Bit(0) == 0 {
			return false
		}
		public = &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}
	default:
		return false
	}
	_, err = x509.MarshalPKIXPublicKey(public)
	return err == nil
}
