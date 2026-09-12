package model

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/types"

	"github.com/gin-gonic/gin"
	mysqlDriver "github.com/go-sql-driver/mysql"

	"gorm.io/gorm"
)

func applyExplicitLogTextFilter(tx *gorm.DB, column string, value string) (*gorm.DB, error) {
	if value == "" {
		return tx, nil
	}
	if strings.Contains(value, "%") {
		condition, pattern, err := buildLogLikeCondition(column, value)
		if err != nil {
			return nil, err
		}
		return tx.Where(condition, pattern), nil
	}
	return tx.Where(column+" = ?", value), nil
}

func buildLogLikeCondition(column string, value string) (string, string, error) {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		pattern, err := sanitizeClickHouseLikePattern(value)
		if err != nil {
			return "", "", err
		}
		return column + " LIKE ?", pattern, nil
	}

	pattern, err := sanitizeLikePattern(value)
	if err != nil {
		return "", "", err
	}
	return column + " LIKE ? ESCAPE '!'", pattern, nil
}

func sanitizeClickHouseLikePattern(input string) (string, error) {
	input = strings.ReplaceAll(input, `\`, `\\`)
	input = strings.ReplaceAll(input, `_`, `\_`)

	if err := validateLikePattern(input); err != nil {
		return "", err
	}
	return input, nil
}

type Log struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:2;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:1;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	// BillingEventID is the stable at-least-once projection key emitted by the
	// main-database billing outbox. It is deliberately non-unique because a
	// log sink may replay delivery; query/export aggregation must deduplicate it.
	BillingEventID          string `json:"billing_event_id,omitempty" gorm:"type:varchar(64);index:idx_logs_billing_event_id;default:''"`
	BillingProjectionDigest string `json:"-" gorm:"column:billing_projection_digest;type:varchar(65);default:''"`
	LogRowKey               string `json:"-" gorm:"column:log_row_key;type:varchar(64);default:''"`
	Other                   string `json:"other"`
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
	LogTypeLogin   = 7
)

const (
	billingProjectionDigestVersion = "1"
	billingProjectionCanonicalV1   = 1
	clickHouseCanonicalProjection  = "logs_billing_canonical"
	clickHouseIdentityTable        = "billing_log_projection_identities"
	logCreateMarker                = "myapi:controlled_log_create"
	logCreateGuardCallback         = "myapi:guard_log_create"

	LogProjectionBackfillTaskKey       = "log_projection_backfill"
	LogProjectionBackfillSchemaVersion = 1

	LogProjectionBackfillPhaseRelationalIdentity = "relational_identity"
	LogProjectionBackfillPhaseRelationalIndexes  = "relational_indexes"
	LogProjectionBackfillPhaseClickHouseIdentity = "clickhouse_identity"
	LogProjectionBackfillPhaseClickHouseMaterial = "clickhouse_materialize"
	LogProjectionBackfillPhaseCompleted          = "completed"

	LogProjectionBackfillStatusPending             = "pending"
	LogProjectionBackfillStatusRunning             = "running"
	LogProjectionBackfillStatusFailed              = "failed"
	LogProjectionBackfillStatusManualReview        = "manual_review"
	LogProjectionBackfillStatusMaintenanceRequired = "maintenance_required"
	LogProjectionBackfillStatusCompleted           = "completed"

	LogProjectionMaterializeStatusPending   = "pending"
	LogProjectionMaterializeStatusStarting  = "starting"
	LogProjectionMaterializeStatusRunning   = "running"
	LogProjectionMaterializeStatusCompleted = "completed"
	LogProjectionMaterializeStatusFailed    = "failed"
)

var (
	ErrBillingProjectionConflict        = errors.New("billing log projection conflict")
	ErrBillingProjectionDigestMismatch  = errors.New("billing log projection digest mismatch")
	ErrLogProjectionMaintenanceRequired = errors.New("log projection maintenance required")
	logStorageColumnNames               = []string{
		"id", "user_id", "created_at", "type", "content", "username",
		"token_name", "model_name", "quota", "prompt_tokens", "completion_tokens",
		"use_time", "is_stream", "channel_id", "token_id", "group", "ip",
		"request_id", "upstream_request_id", "billing_event_id",
		"billing_projection_digest", "log_row_key", "other",
	}
)

// LogProjectionBackfillState is the single durable control row for the
// production log-projection migration. It lives in the primary database;
// historical log reads, writes, and DDL always run against the log database.
type LogProjectionBackfillState struct {
	TaskKey                string `json:"task_key" gorm:"type:varchar(64);primaryKey"`
	SchemaVersion          int    `json:"schema_version" gorm:"not null"`
	Phase                  string `json:"phase" gorm:"type:varchar(64);not null"`
	LastID                 int    `json:"last_id" gorm:"not null"`
	LastEventID            string `json:"last_event_id" gorm:"type:varchar(64);not null"`
	Status                 string `json:"status" gorm:"type:varchar(32);not null;index"`
	LastError              string `json:"last_error" gorm:"type:text"`
	MaterializeMutationID  string `json:"materialize_mutation_id" gorm:"type:varchar(128);not null"`
	MaterializeStatus      string `json:"materialize_status" gorm:"type:varchar(32);not null"`
	MaterializeGeneration  int64  `json:"materialize_generation" gorm:"bigint;not null"`
	MaterializeRequestedAt int64  `json:"materialize_requested_at" gorm:"bigint;not null"`
	QuarantinedEvents      int64  `json:"quarantined_events" gorm:"bigint;not null"`
	LockVersion            int64  `json:"lock_version" gorm:"bigint;not null;default:1"`
	UpdatedAt              int64  `json:"updated_at" gorm:"bigint;not null;index"`
	CompletedAt            int64  `json:"completed_at" gorm:"bigint;not null"`

	// Complete is an in-memory batch result retained for the lower-level
	// checkpoint helper. Durable completion is represented by Status/Phase.
	Complete bool `json:"-" gorm:"-"`
}

func (state *LogProjectionBackfillState) BeforeCreate(_ *gorm.DB) error {
	if state.TaskKey == "" {
		state.TaskKey = LogProjectionBackfillTaskKey
	}
	if state.SchemaVersion == 0 {
		state.SchemaVersion = LogProjectionBackfillSchemaVersion
	}
	if state.Status == "" {
		state.Status = LogProjectionBackfillStatusPending
	}
	if state.MaterializeStatus == "" {
		state.MaterializeStatus = LogProjectionMaterializeStatusPending
	}
	if state.LockVersion == 0 {
		state.LockVersion = 1
	}
	if state.UpdatedAt == 0 {
		state.UpdatedAt = common.GetTimestamp()
	}
	return nil
}

type BillingLogProjectionIdentity struct {
	BillingEventID   string `gorm:"column:billing_event_id;type:varchar(64);primaryKey"`
	Digest           string `gorm:"column:digest;type:varchar(65);not null"`
	CanonicalVersion int    `gorm:"column:canonical_version;not null"`
	Status           string `gorm:"column:status;type:varchar(32);not null;index"`
	Reason           string `gorm:"column:reason;type:text"`
	UpdatedAt        int64  `gorm:"column:updated_at;bigint;not null"`
}

func (BillingLogProjectionIdentity) TableName() string {
	return clickHouseIdentityTable
}

const (
	BillingLogProjectionIdentityStatusCanonical   = "canonical"
	BillingLogProjectionIdentityStatusQuarantined = "quarantined"
)

type ClickHouseProjectionMutation struct {
	MutationID       string    `gorm:"column:mutation_id"`
	Command          string    `gorm:"column:command"`
	CreateTime       time.Time `gorm:"column:create_time"`
	IsDone           uint8     `gorm:"column:is_done"`
	LatestFailReason string    `gorm:"column:latest_fail_reason"`
}

func ensureLogRequestId(log *Log) {
	if log != nil && log.RequestId == "" {
		log.RequestId = common.NewRequestId()
	}
}

func appendLogDigestString(payload []byte, value string) []byte {
	payload = binary.BigEndian.AppendUint64(payload, uint64(len(value)))
	return append(payload, value...)
}

func appendLogDigestInt(payload []byte, value int64) []byte {
	return binary.BigEndian.AppendUint64(payload, uint64(value))
}

// ComputeBillingProjectionDigest returns the versioned digest of the complete
// immutable log projection. Database identity and physical-delivery fields are
// intentionally excluded: Id and LogRowKey differ between replayed rows.
func ComputeBillingProjectionDigest(log *Log) string {
	if log == nil {
		return ""
	}
	payload := make([]byte, 0, 256+len(log.Content)+len(log.Other))
	payload = appendLogDigestString(payload, billingProjectionDigestVersion)
	payload = appendLogDigestString(payload, log.BillingEventID)
	payload = appendLogDigestInt(payload, int64(log.UserId))
	payload = appendLogDigestInt(payload, log.CreatedAt)
	payload = appendLogDigestInt(payload, int64(log.Type))
	payload = appendLogDigestString(payload, log.Content)
	payload = appendLogDigestString(payload, log.Username)
	payload = appendLogDigestString(payload, log.TokenName)
	payload = appendLogDigestString(payload, log.ModelName)
	payload = appendLogDigestInt(payload, int64(log.Quota))
	payload = appendLogDigestInt(payload, int64(log.PromptTokens))
	payload = appendLogDigestInt(payload, int64(log.CompletionTokens))
	payload = appendLogDigestInt(payload, int64(log.UseTime))
	if log.IsStream {
		payload = append(payload, 1)
	} else {
		payload = append(payload, 0)
	}
	payload = appendLogDigestInt(payload, int64(log.ChannelId))
	payload = appendLogDigestInt(payload, int64(log.TokenId))
	payload = appendLogDigestString(payload, log.Group)
	payload = appendLogDigestString(payload, log.Ip)
	payload = appendLogDigestString(payload, log.RequestId)
	payload = appendLogDigestString(payload, log.UpstreamRequestId)
	payload = appendLogDigestString(payload, log.Other)
	digest := sha256.Sum256(payload)
	return billingProjectionDigestVersion + hex.EncodeToString(digest[:])
}

// PrepareLogProjectionIdentity assigns the stable physical row key and
// computes or validates the immutable billing projection digest.
func legacyLogRowKey(log *Log) string {
	if log == nil {
		return ""
	}
	payload := binary.BigEndian.AppendUint64(nil, uint64(log.Id))
	payload = appendLogDigestString(payload, ComputeBillingProjectionDigest(log))
	digest := sha256.Sum256(payload)
	return "l1" + hex.EncodeToString(digest[:])[:62]
}

func PrepareLogProjectionIdentity(log *Log) error {
	if log == nil {
		return nil
	}
	if log.LogRowKey == "" {
		log.LogRowKey = common.NewRequestId()
	}
	if log.BillingEventID == "" {
		if log.BillingProjectionDigest != "" {
			return fmt.Errorf("%w: empty billing event id", ErrBillingProjectionDigestMismatch)
		}
		return nil
	}
	expected := ComputeBillingProjectionDigest(log)
	if log.BillingProjectionDigest == "" {
		log.BillingProjectionDigest = expected
		return nil
	}
	if log.BillingProjectionDigest != expected {
		return fmt.Errorf("%w: event %s", ErrBillingProjectionDigestMismatch, log.BillingEventID)
	}
	return nil
}

func ValidateBillingProjectionCompatibility(candidate *Log, existing []*Log) error {
	if candidate == nil || candidate.BillingEventID == "" {
		return nil
	}
	if err := PrepareLogProjectionIdentity(candidate); err != nil {
		return err
	}
	for _, stored := range existing {
		if stored == nil || stored.BillingEventID != candidate.BillingEventID {
			continue
		}
		actualStoredDigest := ComputeBillingProjectionDigest(stored)
		if stored.BillingProjectionDigest != "" && stored.BillingProjectionDigest != actualStoredDigest {
			return fmt.Errorf("%w: stored event %s", ErrBillingProjectionDigestMismatch, candidate.BillingEventID)
		}
		if actualStoredDigest != candidate.BillingProjectionDigest {
			return fmt.Errorf(
				"%w: event %s has digests %s and %s",
				ErrBillingProjectionConflict,
				candidate.BillingEventID,
				actualStoredDigest,
				candidate.BillingProjectionDigest,
			)
		}
	}
	return nil
}

func registerLogCreateGuard(db *gorm.DB) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	callbacks := db.Callback().Create()
	if callbacks.Get(logCreateGuardCallback) != nil {
		return nil
	}
	return callbacks.Before("gorm:before_create").Register(logCreateGuardCallback, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Table != "logs" {
			return
		}
		allowed, ok := tx.Get(logCreateMarker)
		if !ok || allowed != true {
			tx.AddError(errors.New("logs writes must use CreateLog or CreateLogs"))
		}
	})
}

func CreateLog(db *gorm.DB, logRecord *Log) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if err := PrepareLogProjectionIdentity(logRecord); err != nil {
		return err
	}
	return db.Set(logCreateMarker, true).Create(logRecord).Error
}

func CreateLogs(db *gorm.DB, logs []*Log) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	eventDigests := map[string]string{}
	rowKeys := map[string]struct{}{}
	for _, logRecord := range logs {
		if logRecord == nil {
			return errors.New("nil log record")
		}
		if err := PrepareLogProjectionIdentity(logRecord); err != nil {
			return err
		}
		if _, exists := rowKeys[logRecord.LogRowKey]; exists {
			return errors.New("duplicate log row key in batch")
		}
		rowKeys[logRecord.LogRowKey] = struct{}{}
		if logRecord.BillingEventID == "" {
			continue
		}
		if digest, exists := eventDigests[logRecord.BillingEventID]; exists && digest != logRecord.BillingProjectionDigest {
			return fmt.Errorf("%w: event %s", ErrBillingProjectionConflict, logRecord.BillingEventID)
		}
		eventDigests[logRecord.BillingEventID] = logRecord.BillingProjectionDigest
	}
	return db.Set(logCreateMarker, true).Create(&logs).Error
}

func (log *Log) BeforeCreate(tx *gorm.DB) error {
	if err := PrepareLogProjectionIdentity(log); err != nil {
		return err
	}
	if log == nil || log.BillingEventID == "" || tx == nil || tx.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return nil
	}
	var existing []*Log
	if err := tx.Session(&gorm.Session{NewDB: true}).
		Where("billing_event_id = ?", log.BillingEventID).
		Find(&existing).Error; err != nil {
		return err
	}
	if err := ValidateBillingProjectionCompatibility(log, existing); err != nil {
		common.SysError(err.Error())
		return err
	}
	return nil
}

func createLog(log *Log) error {
	ensureLogRequestId(log)
	return CreateLog(LOG_DB, log)
}

func logListOrderSQL(prefix string, groupColumn string, dialect string) string {
	parts := []string{
		prefix + "created_at desc",
		prefix + "request_id desc",
		"CASE WHEN " + prefix + "log_row_key = '' THEN 1 ELSE 0 END",
		prefix + "log_row_key desc",
	}
	for _, expression := range logCanonicalSortExpressions(prefix, groupColumn, dialect) {
		parts = append(parts, expression+" desc")
	}
	return strings.Join(parts, ", ")
}

func clickHouseLogOrder(prefix string) string {
	return logListOrderSQL(prefix, "`group`", string(common.DatabaseTypeClickHouse))
}

func logColumnSQL(alias string, column string, groupColumn string) string {
	if column == "group" {
		return alias + groupColumn
	}
	return alias + column
}

func logStorageColumnsSQL(alias string, groupColumn string) string {
	columns := make([]string, 0, len(logStorageColumnNames))
	for _, column := range logStorageColumnNames {
		columns = append(columns, logColumnSQL(alias, column, groupColumn))
	}
	return strings.Join(columns, ", ")
}

func logCanonicalSortExpressions(alias string, groupColumn string, dialect string) []string {
	plainText := func(column string) string {
		return "COALESCE(" + logColumnSQL(alias, column, groupColumn) + ", '')"
	}
	text := func(column string) string {
		expression := plainText(column)
		switch dialect {
		case string(common.DatabaseTypeSQLite):
			return "CAST(" + expression + " AS BLOB)"
		case string(common.DatabaseTypeMySQL):
			return "BINARY " + expression
		case string(common.DatabaseTypePostgreSQL):
			return "convert_to(" + expression + ", 'UTF8')"
		default:
			return expression
		}
	}
	number := func(column string) string {
		return "COALESCE(" + logColumnSQL(alias, column, groupColumn) + ", 0)"
	}
	return []string{
		number("user_id"), number("created_at"), number("type"),
		text("content"), text("username"), text("token_name"), text("model_name"),
		number("quota"), number("prompt_tokens"), number("completion_tokens"), number("use_time"),
		"CASE WHEN " + logColumnSQL(alias, "is_stream", groupColumn) + " THEN 1 ELSE 0 END",
		number("channel_id"), number("token_id"), text("group"), text("ip"),
		text("request_id"), text("upstream_request_id"), text("other"),
		text("log_row_key"), number("id"),
	}
}

func logCanonicalSortTupleSQL(alias string, groupColumn string, dialect string, clickHouse bool) string {
	expressions := strings.Join(logCanonicalSortExpressions(alias, groupColumn, dialect), ", ")
	if clickHouse {
		return "tuple(" + expressions + ")"
	}
	return "(" + expressions + ")"
}

func logGroupColumnForDB(db *gorm.DB) string {
	if db != nil && db.Dialector != nil && db.Dialector.Name() == string(common.DatabaseTypePostgreSQL) {
		return `"group"`
	}
	return "`group`"
}

func deduplicatedLogs(db *gorm.DB) *gorm.DB {
	if db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return db.Table("(?) AS logs", clickHouseDeduplicatedLogs(db))
	}

	dialect := db.Dialector.Name()
	groupColumn := logGroupColumnForDB(db)
	canonicalPredicate := fmt.Sprintf(
		"NOT EXISTS (SELECT 1 FROM logs AS earlier WHERE earlier.billing_event_id = logs.billing_event_id AND earlier.billing_event_id <> '' AND %s < %s)",
		logCanonicalSortTupleSQL("earlier.", groupColumn, dialect, false),
		logCanonicalSortTupleSQL("logs.", groupColumn, dialect, false),
	)
	quarantinePredicate := "NOT EXISTS (SELECT 1 FROM " + clickHouseIdentityTable + " AS projection_identity WHERE projection_identity.billing_event_id = logs.billing_event_id AND projection_identity.status = '" + BillingLogProjectionIdentityStatusQuarantined + "')"
	return db.Table("logs").Where("logs.billing_event_id = '' OR (" + quarantinePredicate + " AND " + canonicalPredicate + ")")
}

func clickHouseCanonicalAggregateSelectSQL() string {
	columns := logStorageColumnsSQL("", "`group`")
	return fmt.Sprintf(
		"SELECT billing_event_id, argMin(tuple(%s), %s) AS canonical",
		columns,
		logCanonicalSortTupleSQL("", "`group`", string(common.DatabaseTypeClickHouse), true),
	)
}

func clickHouseCanonicalAggregateSQL() string {
	return clickHouseCanonicalAggregateSelectSQL() + " FROM logs GROUP BY billing_event_id"
}

func clickHouseCanonicalProjectionDefinition() string {
	return fmt.Sprintf(
		"PROJECTION %s (%s GROUP BY billing_event_id)",
		clickHouseCanonicalProjection,
		clickHouseCanonicalAggregateSelectSQL(),
	)
}

func clickHouseDeduplicatedLogs(db *gorm.DB) *gorm.DB {
	emptyColumns := logStorageColumnsSQL("", "`group`")
	canonicalColumns := make([]string, 0, len(logStorageColumnNames))
	for index, column := range logStorageColumnNames {
		alias := column
		if column == "group" {
			alias = "`group`"
		}
		canonicalColumns = append(canonicalColumns, fmt.Sprintf("tupleElement(canonical, %d) AS %s", index+1, alias))
	}
	query := fmt.Sprintf(`
SELECT %s
FROM logs
WHERE billing_event_id = ''
UNION ALL
SELECT %s
FROM (%s) AS canonical_billing_logs
WHERE billing_event_id <> '' AND billing_event_id NOT IN (SELECT billing_event_id FROM %s WHERE status = '%s')`,
		emptyColumns,
		strings.Join(canonicalColumns, ", "),
		clickHouseCanonicalAggregateSQL(),
		clickHouseIdentityTable,
		BillingLogProjectionIdentityStatusQuarantined,
	)
	return db.Session(&gorm.Session{NewDB: true}).Raw(query)
}

func assignDisplayLogIds(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].Id = startIdx + i + 1
	}
}

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			// Remove operation-audit details (operator/route info), admin-only.
			delete(otherMap, "audit_info")
			// delete(otherMap, "reject_reason")
			// delete(otherMap, "stream_status")
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
	}
	assignDisplayLogIds(logs, startIdx)
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = deduplicatedLogs(LOG_DB).
		Where("logs.token_id = ?", tokenId).
		Order(logListOrderSQL("logs.", logGroupColumnForDB(LOG_DB), LOG_DB.Dialector.Name())).
		Limit(common.MaxRecentItems).
		Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info，
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// buildOpField 构建语言无关的操作描述（写入 Other.op）。
// 前端依据 action(稳定操作标识) + params(结构化参数) 在渲染期用 i18n 本地化展示，
// 因此不在数据库中存储自然语言句子。
func buildOpField(action string, params map[string]interface{}) map[string]interface{} {
	op := map[string]interface{}{
		"action": action,
	}
	if len(params) > 0 {
		op["params"] = params
	}
	return op
}

// RecordLoginLog 记录用户登录成功的审计日志（type=LogTypeLogin）。
// username 由调用方传入（登录流程已持有用户对象），避免额外的数据库查询。
// content 为英文兜底文本（用于导出）；action+params 供前端本地化渲染。
// extra 可携带 login_method、user_agent 等附加信息（普通用户可见）。
func RecordLoginLog(userId int, username string, content string, ip string, action string, params map[string]interface{}, extra map[string]interface{}) {
	other := map[string]interface{}{}
	for k, v := range extra {
		other[k] = v
	}
	other["op"] = buildOpField(action, params)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeLogin,
		Content:   content,
		Ip:        ip,
		Other:     common.MapToJsonStr(other),
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record login log: " + err.Error())
	}
}

// RecordOperationAuditLog 记录管理/高危操作审计日志（type=LogTypeManage）。
// logUserId 为日志归属者，管理审计日志应归属实际操作者；目标资源/用户放入
// action params。username 内部按 logUserId 查询。content 为英文兜底文本（供导出使用）。
// action+params 写入 Other.op，供前端本地化渲染（普通用户可见，不含敏感信息）。
// adminInfo 存放操作者身份（写入 Other.admin_info，普通用户查询时剥离）；
// auditInfo 存放路由/方法/结果等中间件兜底信息（写入 Other.audit_info，普通用户查询时剥离）。
func RecordOperationAuditLog(logUserId int, content string, ip string, action string, params map[string]interface{}, adminInfo map[string]interface{}, auditInfo map[string]interface{}) {
	username, _ := GetUsernameById(logUserId, false)
	other := map[string]interface{}{
		"op": buildOpField(action, params),
	}
	if len(adminInfo) > 0 {
		other["admin_info"] = adminInfo
	}
	if len(auditInfo) > 0 {
		other["audit_info"] = auditInfo
	}
	log := &Log{
		UserId:    logUserId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeManage,
		Content:   content,
		Ip:        ip,
		Other:     common.MapToJsonStr(other),
	}
	if err := createLog(log); err != nil {
		common.SysLog("failed to record operation audit log: " + err.Error())
	}
}

func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, common.LocalLogPreview(content)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := createLog(log)
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) {
	if !common.LogConsumeEnabled {
		return
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	createdAt := common.GetTimestamp()
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := false
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		if settingMap.RecordIpLog {
			needRecordIp = true
		}
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        createdAt,
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := createLog(log)
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
	if common.DataExportEnabled {
		LogQuotaData(QuotaDataLogParams{
			UserID:    userId,
			Username:  username,
			ModelName: params.ModelName,
			Quota:     params.Quota,
			CreatedAt: createdAt,
			TokenUsed: params.PromptTokens + params.CompletionTokens,
			UseGroup:  params.Group,
			TokenID:   params.TokenId,
			ChannelID: params.ChannelId,
			NodeName:  common.NodeName,
		})
	}
}

type RecordTaskBillingLogParams struct {
	UserId    int
	LogType   int
	Content   string
	ChannelId int
	ModelName string
	Quota     int
	TokenId   int
	Group     string
	Other     map[string]interface{}
	NodeName  string // 任务发起节点；为空时回退当前节点
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	createdAt := common.GetTimestamp()
	log := &Log{
		UserId:    params.UserId,
		Username:  username,
		CreatedAt: createdAt,
		Type:      params.LogType,
		Content:   params.Content,
		TokenName: tokenName,
		ModelName: params.ModelName,
		Quota:     params.Quota,
		ChannelId: params.ChannelId,
		TokenId:   params.TokenId,
		Group:     params.Group,
		Other:     common.MapToJsonStr(params.Other),
	}
	err := createLog(log)
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
	if params.LogType == LogTypeConsume && common.DataExportEnabled {
		nodeName := params.NodeName
		if nodeName == "" {
			nodeName = common.NodeName
		}
		LogQuotaData(QuotaDataLogParams{
			UserID:    params.UserId,
			Username:  username,
			ModelName: params.ModelName,
			Quota:     params.Quota,
			CreatedAt: createdAt,
			UseGroup:  params.Group,
			TokenID:   params.TokenId,
			ChannelID: params.ChannelId,
			NodeName:  nodeName,
		})
	}
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	tx := deduplicatedLogs(LOG_DB)
	if logType != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupColumnForDB(LOG_DB)+" = ?", group)
	}
	err = tx.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order(logListOrderSQL("logs.", logGroupColumnForDB(LOG_DB), LOG_DB.Dialector.Name())).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		assignDisplayLogIds(logs, startIdx)
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string) (logs []*Log, total int64, err error) {
	tx := deduplicatedLogs(LOG_DB).Where("logs.user_id = ?", userId)
	if logType != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", logType)
	}

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupColumnForDB(LOG_DB)+" = ?", group)
	}
	err = tx.Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order(logListOrderSQL("logs.", logGroupColumnForDB(LOG_DB), LOG_DB.Dialector.Name())).Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type Stat struct {
	Quota int `json:"quota"`
	Rpm   int `json:"rpm"`
	Tpm   int `json:"tpm"`
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string) (stat Stat, err error) {
	tx := deduplicatedLogs(LOG_DB).Select("COALESCE(sum(quota), 0) quota")

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := deduplicatedLogs(LOG_DB).Select("count(*) rpm, COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0) tpm")

	if tx, err = applyExplicitLogTextFilter(tx, "username", username); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "username", username); err != nil {
		return stat, err
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "model_name", modelName); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "model_name", modelName); err != nil {
		return stat, err
	}
	if channel != 0 {
		tx = tx.Where("channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where(logGroupColumnForDB(LOG_DB)+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where(logGroupColumnForDB(LOG_DB)+" = ?", group)
	}

	tx = tx.Where("type = ?", LogTypeConsume)
	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	if err := tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	return stat, nil
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := deduplicatedLogs(LOG_DB).Select("COALESCE(sum(prompt_tokens), 0) + COALESCE(sum(completion_tokens), 0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func initialLogProjectionBackfillPhase(logDB *gorm.DB) string {
	if logDB != nil && logDB.Dialector != nil && logDB.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return LogProjectionBackfillPhaseClickHouseIdentity
	}
	return LogProjectionBackfillPhaseRelationalIdentity
}

func GetOrCreateLogProjectionBackfillState(ctx context.Context, mainDB *gorm.DB, logDB *gorm.DB) (*LogProjectionBackfillState, error) {
	if mainDB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var state LogProjectionBackfillState
	err := mainDB.WithContext(ctx).Where("task_key = ?", LogProjectionBackfillTaskKey).First(&state).Error
	if err == nil {
		return &state, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	state = LogProjectionBackfillState{
		TaskKey:           LogProjectionBackfillTaskKey,
		SchemaVersion:     LogProjectionBackfillSchemaVersion,
		Phase:             initialLogProjectionBackfillPhase(logDB),
		Status:            LogProjectionBackfillStatusPending,
		MaterializeStatus: LogProjectionMaterializeStatusPending,
		UpdatedAt:         common.GetTimestamp(),
	}
	if err := mainDB.WithContext(ctx).Create(&state).Error; err != nil {
		if loadErr := mainDB.WithContext(ctx).Where("task_key = ?", LogProjectionBackfillTaskKey).First(&state).Error; loadErr == nil {
			return &state, nil
		}
		return nil, err
	}
	return &state, nil
}

func GetLogProjectionBackfillState(ctx context.Context, mainDB *gorm.DB) (*LogProjectionBackfillState, error) {
	if mainDB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var state LogProjectionBackfillState
	if err := mainDB.WithContext(ctx).Where("task_key = ?", LogProjectionBackfillTaskKey).First(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

func SaveLogProjectionBackfillStateFenced(ctx context.Context, mainDB *gorm.DB, state *LogProjectionBackfillState, taskID string, runnerID string, fenceToken int64, expectedVersion int64) error {
	if mainDB == nil || state == nil || taskID == "" || runnerID == "" || fenceToken <= 0 || expectedVersion <= 0 {
		return gorm.ErrInvalidDB
	}
	state.TaskKey = LogProjectionBackfillTaskKey
	state.UpdatedAt = common.GetTimestamp()
	nextVersion := expectedVersion + 1
	result := mainDB.WithContext(ctx).Model(&LogProjectionBackfillState{}).
		Where("task_key = ? AND lock_version = ?", LogProjectionBackfillTaskKey, expectedVersion).
		Where("EXISTS (SELECT 1 FROM system_task_locks WHERE system_task_locks.type = ? AND system_task_locks.task_id = ? AND system_task_locks.locked_by = ? AND system_task_locks.fence_token = ? AND system_task_locks.locked_until >= ?)", SystemTaskTypeLogProjectionBackfill, taskID, runnerID, fenceToken, common.GetTimestamp()).
		Updates(map[string]interface{}{
			"schema_version":           state.SchemaVersion,
			"phase":                    state.Phase,
			"last_id":                  state.LastID,
			"last_event_id":            state.LastEventID,
			"status":                   state.Status,
			"last_error":               state.LastError,
			"materialize_mutation_id":  state.MaterializeMutationID,
			"materialize_status":       state.MaterializeStatus,
			"materialize_generation":   state.MaterializeGeneration,
			"materialize_requested_at": state.MaterializeRequestedAt,
			"quarantined_events":       state.QuarantinedEvents,
			"lock_version":             nextVersion,
			"updated_at":               state.UpdatedAt,
			"completed_at":             state.CompletedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrSystemTaskLockLost
	}
	state.LockVersion = nextVersion
	return nil
}

func SetLogProjectionMaintenanceRequired(ctx context.Context, mainDB *gorm.DB, logDB *gorm.DB, reason string) error {
	if mainDB == nil {
		return gorm.ErrInvalidDB
	}
	_, err := GetOrCreateLogProjectionBackfillState(ctx, mainDB, logDB)
	if err != nil {
		return err
	}
	return mainDB.WithContext(ctx).Model(&LogProjectionBackfillState{}).
		Where("task_key = ?", LogProjectionBackfillTaskKey).
		Updates(map[string]interface{}{
			"status":       LogProjectionBackfillStatusMaintenanceRequired,
			"last_error":   reason,
			"lock_version": gorm.Expr("lock_version + ?", 1),
			"updated_at":   common.GetTimestamp(),
		}).Error
}

func IsLogProjectionBackfillTerminal(state *LogProjectionBackfillState) bool {
	return state != nil && (state.Status == LogProjectionBackfillStatusCompleted || state.Status == LogProjectionBackfillStatusManualReview || state.Status == LogProjectionBackfillStatusMaintenanceRequired)
}

func upsertRelationalProjectionIdentity(ctx context.Context, db *gorm.DB, identity BillingLogProjectionIdentity) error {
	var existing BillingLogProjectionIdentity
	err := db.WithContext(ctx).Where("billing_event_id = ?", identity.BillingEventID).First(&existing).Error
	if err == nil {
		if existing.Status == BillingLogProjectionIdentityStatusQuarantined {
			return nil
		}
		return db.WithContext(ctx).Model(&BillingLogProjectionIdentity{}).
			Where("billing_event_id = ?", identity.BillingEventID).
			Updates(map[string]interface{}{
				"digest":            identity.Digest,
				"canonical_version": identity.CanonicalVersion,
				"status":            identity.Status,
				"reason":            identity.Reason,
				"updated_at":        identity.UpdatedAt,
			}).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.WithContext(ctx).Create(&identity).Error
}

func QuarantineBillingProjectionEvent(ctx context.Context, db *gorm.DB, eventID string, reason string) error {
	if db == nil || db.Dialector == nil || strings.TrimSpace(eventID) == "" {
		return gorm.ErrInvalidDB
	}
	if strings.TrimSpace(reason) == "" {
		reason = ErrBillingProjectionConflict.Error()
	}
	if db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return quarantineClickHouseProjectionEvent(ctx, db, eventID, reason)
	}
	return upsertRelationalProjectionIdentity(ctx, db, BillingLogProjectionIdentity{
		BillingEventID: eventID, CanonicalVersion: billingProjectionCanonicalV1,
		Status: BillingLogProjectionIdentityStatusQuarantined, Reason: reason, UpdatedAt: common.GetTimestamp(),
	})
}

func reconcileRelationalProjectionEvent(ctx context.Context, db *gorm.DB, eventID string) (bool, error) {
	var rows []*Log
	if err := db.WithContext(ctx).Where("billing_event_id = ?", eventID).Order("id").Find(&rows).Error; err != nil {
		return false, err
	}
	digests := map[string]struct{}{}
	reason := ""
	for _, row := range rows {
		computed := ComputeBillingProjectionDigest(row)
		if row.BillingProjectionDigest != "" && row.BillingProjectionDigest != computed {
			reason = fmt.Sprintf("stored digest %s does not match computed digest %s", row.BillingProjectionDigest, computed)
		}
		digests[computed] = struct{}{}
	}
	if len(digests) != 1 || reason != "" {
		if reason == "" {
			reason = fmt.Sprintf("historical event has %d immutable projections", len(digests))
		}
		return true, upsertRelationalProjectionIdentity(ctx, db, BillingLogProjectionIdentity{
			BillingEventID:   eventID,
			CanonicalVersion: billingProjectionCanonicalV1,
			Status:           BillingLogProjectionIdentityStatusQuarantined,
			Reason:           reason,
			UpdatedAt:        common.GetTimestamp(),
		})
	}
	var digest string
	for value := range digests {
		digest = value
	}
	if err := upsertRelationalProjectionIdentity(ctx, db, BillingLogProjectionIdentity{
		BillingEventID:   eventID,
		Digest:           digest,
		CanonicalVersion: billingProjectionCanonicalV1,
		Status:           BillingLogProjectionIdentityStatusCanonical,
		UpdatedAt:        common.GetTimestamp(),
	}); err != nil {
		return false, err
	}
	return false, nil
}

func countQuarantinedProjectionEvents(ctx context.Context, db *gorm.DB) (int64, error) {
	var count int64
	query := db.WithContext(ctx).Table(clickHouseIdentityTable).
		Where("status = ?", BillingLogProjectionIdentityStatusQuarantined)
	if db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		if err := query.Distinct("billing_event_id").Count(&count).Error; err != nil {
			return 0, err
		}
		return count, nil
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func BackfillLogProjectionIdentityBatch(ctx context.Context, db *gorm.DB, state LogProjectionBackfillState, limit int) (LogProjectionBackfillState, int, error) {
	if db == nil {
		return state, 0, gorm.ErrInvalidDB
	}
	if db.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		return state, 0, errors.New("ClickHouse log identity backfill requires an event checkpoint")
	}
	if limit <= 0 {
		limit = 100
	}
	var rows []*Log
	if err := db.WithContext(ctx).Where("id > ?", state.LastID).Order("id").Limit(limit).Find(&rows).Error; err != nil {
		return state, 0, err
	}
	updated := 0
	for _, row := range rows {
		updates := map[string]interface{}{}
		if row.LogRowKey == "" {
			updates["log_row_key"] = legacyLogRowKey(row)
		}
		if row.BillingEventID != "" {
			quarantined, err := reconcileRelationalProjectionEvent(ctx, db, row.BillingEventID)
			if err != nil {
				return state, updated, err
			}
			if !quarantined && row.BillingProjectionDigest == "" {
				updates["billing_projection_digest"] = ComputeBillingProjectionDigest(row)
			}
		}
		if len(updates) > 0 {
			if err := db.WithContext(ctx).Model(&Log{}).Where("id = ?", row.Id).Updates(updates).Error; err != nil {
				return state, updated, err
			}
			updated++
		}
		state.LastID = row.Id
	}
	quarantined, err := countQuarantinedProjectionEvents(ctx, db)
	if err != nil {
		return state, updated, err
	}
	state.QuarantinedEvents = quarantined
	state.Complete = len(rows) < limit
	return state, updated, nil
}

type LogProjectionIndexSpec struct {
	Name    string
	Columns string
}

const LogProjectionSQLiteAutoIndexMaxRows int64 = 10000

var logProjectionIndexSpecs = []LogProjectionIndexSpec{
	{Name: "idx_logs_billing_canonical", Columns: "billing_event_id, billing_projection_digest, id"},
	{Name: "idx_logs_row_key", Columns: "log_row_key"},
}

func logProjectionIndexSQL(dialect string, spec LogProjectionIndexSpec) (string, bool) {
	switch dialect {
	case string(common.DatabaseTypePostgreSQL):
		return fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON logs (%s)", spec.Name, spec.Columns), true
	case string(common.DatabaseTypeMySQL):
		return fmt.Sprintf("ALTER TABLE logs ADD INDEX %s (%s), ALGORITHM=INPLACE, LOCK=NONE", spec.Name, spec.Columns), true
	case string(common.DatabaseTypeSQLite):
		return fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON logs (%s)", spec.Name, spec.Columns), true
	default:
		return "", false
	}
}

type logProjectionIndexState struct {
	Exists  bool
	Valid   bool
	Ready   bool
	Columns string
}

const postgresLogProjectionIndexInspectionSQL = `
SELECT i.indisvalid, i.indisready, string_agg(a.attname, ', ' ORDER BY ord.ordinality) AS columns
FROM pg_class AS tbl
JOIN pg_namespace AS ns ON ns.oid = tbl.relnamespace
JOIN pg_index AS i ON i.indrelid = tbl.oid
JOIN pg_class AS idx ON idx.oid = i.indexrelid
JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS ord(attnum, ordinality) ON true
JOIN pg_attribute AS a ON a.attrelid = tbl.oid AND a.attnum = ord.attnum
WHERE ns.nspname = current_schema() AND tbl.relname = 'logs' AND idx.relname = ?
GROUP BY i.indisvalid, i.indisready`

func inspectLogProjectionIndex(ctx context.Context, db *gorm.DB, spec LogProjectionIndexSpec) (logProjectionIndexState, error) {
	state := logProjectionIndexState{}
	switch db.Dialector.Name() {
	case string(common.DatabaseTypePostgreSQL):
		var row struct {
			Valid   bool   `gorm:"column:indisvalid"`
			Ready   bool   `gorm:"column:indisready"`
			Columns string `gorm:"column:columns"`
		}
		err := db.WithContext(ctx).Raw(postgresLogProjectionIndexInspectionSQL, spec.Name).Scan(&row).Error
		if err != nil {
			return state, err
		}
		state.Exists = row.Columns != ""
		state.Valid = row.Valid
		state.Ready = row.Ready
		state.Columns = row.Columns
		return state, nil
	case string(common.DatabaseTypeMySQL):
		var columns string
		err := db.WithContext(ctx).Raw(`
SELECT COALESCE(GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ', '), '')
FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = 'logs' AND index_name = ?`, spec.Name).Scan(&columns).Error
		if err != nil {
			return state, err
		}
		state.Exists = columns != ""
		state.Valid = state.Exists
		state.Ready = state.Exists
		state.Columns = columns
		return state, nil
	case string(common.DatabaseTypeSQLite):
		if !db.Migrator().HasIndex(&Log{}, spec.Name) {
			return state, nil
		}
		var rows []struct {
			Name string `gorm:"column:name"`
		}
		if err := db.WithContext(ctx).Raw("PRAGMA index_info('" + spec.Name + "')").Scan(&rows).Error; err != nil {
			return state, err
		}
		columns := make([]string, 0, len(rows))
		for _, row := range rows {
			columns = append(columns, row.Name)
		}
		state.Exists = true
		state.Valid = true
		state.Ready = true
		state.Columns = strings.Join(columns, ", ")
		return state, nil
	default:
		return state, fmt.Errorf("unsupported log database dialect %q", db.Dialector.Name())
	}
}

func classifyMySQLLogProjectionDDLError(err error) bool {
	if errors.Is(err, driver.ErrBadConn) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1061, 1064, 1071, 1072, 1142, 1143, 1227, 1235, 1709, 1846, 1847:
			return true
		default:
			return false
		}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "algorithm=inplace is not supported") ||
		strings.Contains(message, "lock=none is not supported") ||
		strings.Contains(message, "duplicate key name") ||
		strings.Contains(message, "conflicting definition") ||
		strings.Contains(message, "syntax error")
}

func executeLogProjectionIndexDDL(ctx context.Context, db *gorm.DB, statement string) error {
	switch db.Dialector.Name() {
	case string(common.DatabaseTypeMySQL):
		return db.WithContext(ctx).Connection(func(connection *gorm.DB) (runErr error) {
			var originalLockWait int64
			if err := connection.Raw("SELECT @@SESSION.lock_wait_timeout").Scan(&originalLockWait).Error; err != nil {
				return err
			}
			var originalInnoDBLockWait int64
			if err := connection.Raw("SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&originalInnoDBLockWait).Error; err != nil {
				return err
			}
			defer func() {
				restoreErr := errors.Join(
					connection.Exec("SET SESSION lock_wait_timeout = ?", originalLockWait).Error,
					connection.Exec("SET SESSION innodb_lock_wait_timeout = ?", originalInnoDBLockWait).Error,
				)
				runErr = errors.Join(runErr, restoreErr)
			}()
			if err := connection.Exec("SET SESSION lock_wait_timeout = 5").Error; err != nil {
				return err
			}
			if err := connection.Exec("SET SESSION innodb_lock_wait_timeout = 5").Error; err != nil {
				return err
			}
			return connection.Session(&gorm.Session{SkipDefaultTransaction: true}).Exec(statement).Error
		})
	case string(common.DatabaseTypePostgreSQL):
		return db.WithContext(ctx).Connection(func(connection *gorm.DB) (runErr error) {
			var originalLockTimeout string
			if err := connection.Raw("SHOW lock_timeout").Scan(&originalLockTimeout).Error; err != nil {
				return err
			}
			defer func() {
				restoreErr := connection.Raw("SELECT set_config('lock_timeout', ?, false)", originalLockTimeout).Scan(new(string)).Error
				runErr = errors.Join(runErr, restoreErr)
			}()
			if err := connection.Exec("SET lock_timeout = '5s'").Error; err != nil {
				return err
			}
			return connection.Session(&gorm.Session{SkipDefaultTransaction: true}).Exec(statement).Error
		})
	default:
		return db.WithContext(ctx).Session(&gorm.Session{NewDB: true, SkipDefaultTransaction: true}).Exec(statement).Error
	}
}

func CreateNextLogProjectionIndex(ctx context.Context, db *gorm.DB) (bool, bool, error) {
	if db == nil || db.Dialector == nil {
		return false, false, gorm.ErrInvalidDB
	}
	for _, spec := range logProjectionIndexSpecs {
		state, err := inspectLogProjectionIndex(ctx, db, spec)
		if err != nil {
			manual := db.Dialector.Name() == string(common.DatabaseTypeMySQL) && classifyMySQLLogProjectionDDLError(err)
			return false, manual, fmt.Errorf("inspect log projection index %s: %w", spec.Name, err)
		}
		if state.Exists && state.Valid && state.Ready && state.Columns == spec.Columns {
			continue
		}
		if !state.Exists && db.Dialector.Name() == string(common.DatabaseTypeSQLite) {
			var rows int64
			if err := db.WithContext(ctx).Table("logs").Count(&rows).Error; err != nil {
				return false, false, err
			}
			if rows > LogProjectionSQLiteAutoIndexMaxRows {
				return false, true, fmt.Errorf("%w: SQLite logs table has %d rows, automatic threshold is %d", ErrLogProjectionMaintenanceRequired, rows, LogProjectionSQLiteAutoIndexMaxRows)
			}
		}
		if state.Exists {
			if db.Dialector.Name() != string(common.DatabaseTypePostgreSQL) {
				return false, true, fmt.Errorf("log projection index %s has incompatible definition %q", spec.Name, state.Columns)
			}
			if err := executeLogProjectionIndexDDL(ctx, db, "DROP INDEX CONCURRENTLY IF EXISTS "+spec.Name); err != nil {
				return false, false, fmt.Errorf("drop invalid log projection index %s: %w", spec.Name, err)
			}
		}
		statement, supported := logProjectionIndexSQL(db.Dialector.Name(), spec)
		if !supported {
			return false, true, fmt.Errorf("unsupported log database dialect %q", db.Dialector.Name())
		}
		if err := executeLogProjectionIndexDDL(ctx, db, statement); err != nil {
			manual := db.Dialector.Name() == string(common.DatabaseTypeMySQL) && classifyMySQLLogProjectionDDLError(err)
			return false, manual, fmt.Errorf("create log projection index %s: %w", spec.Name, err)
		}
		verified, err := inspectLogProjectionIndex(ctx, db, spec)
		if err != nil {
			return false, false, fmt.Errorf("verify log projection index %s: %w", spec.Name, err)
		}
		if !verified.Exists || !verified.Valid || !verified.Ready || verified.Columns != spec.Columns {
			return false, true, fmt.Errorf("log projection index %s failed verification: valid=%t ready=%t columns=%q", spec.Name, verified.Valid, verified.Ready, verified.Columns)
		}
		return false, false, nil
	}
	return true, false, nil
}

func validateBillingLogProjectionIdentities(eventID string, digest string, identities []BillingLogProjectionIdentity) error {
	if eventID == "" || digest == "" {
		return fmt.Errorf("%w: incomplete ClickHouse projection identity", ErrBillingProjectionDigestMismatch)
	}
	for _, identity := range identities {
		if identity.Status == BillingLogProjectionIdentityStatusQuarantined {
			return fmt.Errorf("%w: event %s is quarantined: %s", ErrBillingProjectionConflict, eventID, identity.Reason)
		}
		if identity.BillingEventID != eventID || identity.CanonicalVersion != billingProjectionCanonicalV1 || identity.Digest != digest {
			return fmt.Errorf("%w: event %s has digest %s and %s", ErrBillingProjectionConflict, eventID, identity.Digest, digest)
		}
	}
	return nil
}

func loadClickHouseBillingProjectionIdentities(ctx context.Context, db *gorm.DB, eventID string) ([]BillingLogProjectionIdentity, error) {
	var identities []BillingLogProjectionIdentity
	err := db.WithContext(ctx).Table(clickHouseIdentityTable).
		Select("billing_event_id, digest, canonical_version, status, reason, max(updated_at) AS updated_at").
		Where("billing_event_id = ?", eventID).
		Group("billing_event_id, digest, canonical_version, status, reason").
		Order("status desc, digest, canonical_version").
		Limit(3).
		Scan(&identities).Error
	return identities, err
}

func insertClickHouseProjectionIdentity(ctx context.Context, db *gorm.DB, identity BillingLogProjectionIdentity) error {
	return db.WithContext(ctx).Table(clickHouseIdentityTable).Create(&identity).Error
}

func quarantineClickHouseProjectionEvent(ctx context.Context, db *gorm.DB, eventID string, reason string) error {
	identities, err := loadClickHouseBillingProjectionIdentities(ctx, db, eventID)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if identity.Status == BillingLogProjectionIdentityStatusQuarantined {
			return nil
		}
	}
	return insertClickHouseProjectionIdentity(ctx, db, BillingLogProjectionIdentity{
		BillingEventID:   eventID,
		CanonicalVersion: billingProjectionCanonicalV1,
		Status:           BillingLogProjectionIdentityStatusQuarantined,
		Reason:           reason,
		UpdatedAt:        time.Now().UnixMilli(),
	})
}

func EnsureClickHouseBillingProjectionIdentity(ctx context.Context, db *gorm.DB, eventID string, digest string) error {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != string(common.DatabaseTypeClickHouse) {
		return errors.New("ClickHouse projection identity requires a ClickHouse log database")
	}
	identities, err := loadClickHouseBillingProjectionIdentities(ctx, db, eventID)
	if err != nil {
		return err
	}
	if len(identities) > 0 {
		if err := validateBillingLogProjectionIdentities(eventID, digest, identities); err != nil {
			if errors.Is(err, ErrBillingProjectionConflict) {
				if quarantineErr := quarantineClickHouseProjectionEvent(ctx, db, eventID, err.Error()); quarantineErr != nil {
					return errors.Join(err, quarantineErr)
				}
			}
			return err
		}
		return nil
	}
	identity := BillingLogProjectionIdentity{
		BillingEventID:   eventID,
		Digest:           digest,
		CanonicalVersion: billingProjectionCanonicalV1,
		Status:           BillingLogProjectionIdentityStatusCanonical,
		UpdatedAt:        time.Now().UnixMilli(),
	}
	if err := insertClickHouseProjectionIdentity(ctx, db, identity); err != nil {
		return err
	}
	identities, err = loadClickHouseBillingProjectionIdentities(ctx, db, eventID)
	if err != nil {
		return err
	}
	if len(identities) == 0 {
		return errors.New("ClickHouse projection identity insert was not observable")
	}
	if err := validateBillingLogProjectionIdentities(eventID, digest, identities); err != nil {
		if errors.Is(err, ErrBillingProjectionConflict) {
			if quarantineErr := quarantineClickHouseProjectionEvent(ctx, db, eventID, err.Error()); quarantineErr != nil {
				return errors.Join(err, quarantineErr)
			}
		}
		return err
	}
	return nil
}

func clickHouseBackfillEventIDsSQL() string {
	return "SELECT billing_event_id FROM logs WHERE billing_event_id > ? AND billing_event_id <> '' GROUP BY billing_event_id ORDER BY billing_event_id LIMIT ?"
}

func BackfillClickHouseProjectionIdentityBatch(ctx context.Context, db *gorm.DB, state LogProjectionBackfillState, limit int) (LogProjectionBackfillState, int, error) {
	if db == nil || db.Dialector == nil || db.Dialector.Name() != string(common.DatabaseTypeClickHouse) {
		return state, 0, errors.New("ClickHouse projection identity backfill requires ClickHouse")
	}
	if limit <= 0 {
		limit = 100
	}
	var eventIDs []string
	if err := db.WithContext(ctx).Raw(clickHouseBackfillEventIDsSQL(), state.LastEventID, limit).Scan(&eventIDs).Error; err != nil {
		return state, 0, err
	}
	if len(eventIDs) == 0 {
		state.Complete = true
		return state, 0, nil
	}
	var rows []*Log
	if err := db.WithContext(ctx).Where("billing_event_id IN ?", eventIDs).Order("billing_event_id, id").Find(&rows).Error; err != nil {
		return state, 0, err
	}
	rowsByEvent := make(map[string][]*Log, len(eventIDs))
	for _, row := range rows {
		rowsByEvent[row.BillingEventID] = append(rowsByEvent[row.BillingEventID], row)
	}
	processed := 0
	for _, eventID := range eventIDs {
		eventRows := rowsByEvent[eventID]
		if len(eventRows) == 0 {
			return state, processed, fmt.Errorf("ClickHouse backfill event %s disappeared during scan", eventID)
		}
		digests := map[string]struct{}{}
		reason := ""
		for _, row := range eventRows {
			computed := ComputeBillingProjectionDigest(row)
			if row.BillingProjectionDigest != "" && row.BillingProjectionDigest != computed {
				reason = fmt.Sprintf("stored digest %s does not match computed digest %s", row.BillingProjectionDigest, computed)
			}
			digests[computed] = struct{}{}
		}
		if len(digests) != 1 || reason != "" {
			if reason == "" {
				reason = fmt.Sprintf("historical event has %d immutable projections", len(digests))
			}
			if err := quarantineClickHouseProjectionEvent(ctx, db, eventID, reason); err != nil {
				return state, processed, err
			}
			state.LastEventID = eventID
			processed++
			continue
		}
		var digest string
		for value := range digests {
			digest = value
		}
		if err := EnsureClickHouseBillingProjectionIdentity(ctx, db, eventID, digest); err != nil {
			return state, processed, err
		}
		state.LastEventID = eventID
		processed++
	}
	quarantined, err := countQuarantinedProjectionEvents(ctx, db)
	if err != nil {
		return state, processed, err
	}
	state.QuarantinedEvents = quarantined
	state.Complete = len(eventIDs) < limit
	return state, processed, nil
}

func clickHouseProjectionMutationCommand() string {
	return "MATERIALIZE PROJECTION " + clickHouseCanonicalProjection
}

func FindClickHouseCanonicalProjectionMutations(ctx context.Context, db *gorm.DB, requestedAtMillis int64) ([]ClickHouseProjectionMutation, error) {
	if requestedAtMillis <= 0 {
		return nil, errors.New("ClickHouse projection materialization request time is required")
	}
	var mutations []ClickHouseProjectionMutation
	err := db.WithContext(ctx).Raw(
		"SELECT mutation_id, command, create_time, is_done, latest_fail_reason FROM system.mutations WHERE database = currentDatabase() AND table = ? AND positionCaseInsensitive(command, ?) > 0 AND toUnixTimestamp64Milli(create_time) >= ? ORDER BY create_time, mutation_id",
		"logs", clickHouseProjectionMutationCommand(), requestedAtMillis-1000,
	).Scan(&mutations).Error
	return mutations, err
}

func StartClickHouseCanonicalProjectionMaterialize(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Exec(
		"ALTER TABLE logs MATERIALIZE PROJECTION " + clickHouseCanonicalProjection + " SETTINGS mutations_sync = 0",
	).Error
}

func GetClickHouseCanonicalProjectionMutation(ctx context.Context, db *gorm.DB, mutationID string) (*ClickHouseProjectionMutation, error) {
	if mutationID == "" {
		return nil, errors.New("ClickHouse projection mutation id is required")
	}
	var mutation ClickHouseProjectionMutation
	err := db.WithContext(ctx).Raw(
		"SELECT mutation_id, command, create_time, is_done, latest_fail_reason FROM system.mutations WHERE database = currentDatabase() AND table = ? AND mutation_id = ? LIMIT 1",
		"logs", mutationID,
	).Scan(&mutation).Error
	if err != nil {
		return nil, err
	}
	if mutation.MutationID == "" {
		return nil, nil
	}
	return &mutation, nil
}
func expiredCanonicalBillingEvents(db *gorm.DB, targetTimestamp int64) *gorm.DB {
	return deduplicatedLogs(db).
		Select("logs.billing_event_id").
		Where("logs.billing_event_id <> ? AND logs.created_at < ?", "", targetTimestamp)
}

func logSafeLocatorPredicate(dialect string, prefix string) string {
	predicate := prefix + "billing_event_id <> '' OR " + prefix + "log_row_key <> ''"
	if dialect != string(common.DatabaseTypeClickHouse) {
		predicate += " OR " + prefix + "id > 0"
	}
	return "(" + predicate + ")"
}

func deletablePhysicalLogs(db *gorm.DB, targetTimestamp int64) *gorm.DB {
	emptyEventClause := "logs.billing_event_id = ? AND logs.created_at < ? AND " + logSafeLocatorPredicate(db.Dialector.Name(), "logs.")
	return db.Table("logs").Where(
		"("+emptyEventClause+") OR logs.billing_event_id IN (?)",
		"", targetTimestamp,
		expiredCanonicalBillingEvents(db.Session(&gorm.Session{NewDB: true}), targetTimestamp),
	)
}

func CountOldLog(ctx context.Context, targetTimestamp int64) (int64, error) {
	var total int64
	if err := deletablePhysicalLogs(LOG_DB.WithContext(ctx), targetTimestamp).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

type logCleanupCandidate struct {
	ID             int    `gorm:"column:id"`
	BillingEventID string `gorm:"column:billing_event_id"`
	LogRowKey      string `gorm:"column:log_row_key"`
}

func logCleanupPredicate(eventIDs []string, rowKeys []string) (string, []interface{}) {
	return logCleanupPredicateWithLegacyIDs(eventIDs, rowKeys, nil)
}

func logCleanupPredicateWithLegacyIDs(eventIDs []string, rowKeys []string, legacyIDs []int) (string, []interface{}) {
	clauses := make([]string, 0, 3)
	args := make([]interface{}, 0, 3)
	if len(eventIDs) > 0 {
		clauses = append(clauses, "billing_event_id IN ?")
		args = append(args, eventIDs)
	}
	if len(rowKeys) > 0 {
		clauses = append(clauses, "(billing_event_id = '' AND log_row_key IN ?)")
		args = append(args, rowKeys)
	}
	if len(legacyIDs) > 0 {
		clauses = append(clauses, "(billing_event_id = '' AND log_row_key = '' AND id IN ?)")
		args = append(args, legacyIDs)
	}
	return strings.Join(clauses, " OR "), args
}

type LogCleanupBatchResult struct {
	Deleted int64
	Skipped int64
	Errors  []string
}

func CountUnsafeOldLogs(ctx context.Context, targetTimestamp int64) (int64, error) {
	if LOG_DB == nil || LOG_DB.Dialector == nil {
		return 0, gorm.ErrInvalidDB
	}
	query := LOG_DB.WithContext(ctx).Table("logs").Where("created_at < ?", targetTimestamp).
		Where("NOT " + logSafeLocatorPredicate(LOG_DB.Dialector.Name(), ""))
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func DeleteOldLogBatchDetailed(ctx context.Context, targetTimestamp int64, limit int) (LogCleanupBatchResult, error) {
	result := LogCleanupBatchResult{}
	if limit <= 0 {
		limit = 100
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	var candidates []logCleanupCandidate
	query := deduplicatedLogs(LOG_DB.WithContext(ctx)).
		Select("logs.id, logs.billing_event_id, logs.log_row_key").
		Where("logs.created_at < ?", targetTimestamp).
		Where(logSafeLocatorPredicate(LOG_DB.Dialector.Name(), "logs.")).
		Order("logs.created_at, logs.request_id, logs.log_row_key, logs.id")
	if err := query.
		Limit(limit).
		Scan(&candidates).Error; err != nil {
		return result, err
	}
	if len(candidates) == 0 {
		return result, nil
	}

	eventIDs := make([]string, 0, len(candidates))
	rowKeys := make([]string, 0, len(candidates))
	legacyIDs := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.BillingEventID != "" {
			eventIDs = append(eventIDs, candidate.BillingEventID)
			continue
		}
		if candidate.LogRowKey == "" {
			if LOG_DB.Dialector.Name() != string(common.DatabaseTypeClickHouse) && candidate.ID > 0 {
				legacyIDs = append(legacyIDs, candidate.ID)
				continue
			}
			result.Skipped++
			result.Errors = append(result.Errors, fmt.Sprintf("log row cannot be safely located: id=%d", candidate.ID))
			continue
		}
		rowKeys = append(rowKeys, candidate.LogRowKey)
	}
	predicate, args := logCleanupPredicateWithLegacyIDs(eventIDs, rowKeys, legacyIDs)
	if predicate == "" {
		return result, nil
	}

	var physicalRows int64
	if err := LOG_DB.WithContext(ctx).Table("logs").Where(predicate, args...).Count(&physicalRows).Error; err != nil {
		return result, err
	}
	if physicalRows == 0 {
		return result, nil
	}

	if LOG_DB.Dialector.Name() == string(common.DatabaseTypeClickHouse) {
		if err := LOG_DB.WithContext(ctx).Exec(
			"ALTER TABLE logs DELETE WHERE "+predicate+" SETTINGS mutations_sync = 1",
			args...,
		).Error; err != nil {
			return result, err
		}
		result.Deleted = physicalRows
		return result, nil
	}

	deleteResult := LOG_DB.WithContext(ctx).Where(predicate, args...).Delete(&Log{})
	if deleteResult.Error != nil {
		return result, deleteResult.Error
	}
	result.Deleted = deleteResult.RowsAffected
	return result, nil
}

func DeleteOldLogBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	result, err := DeleteOldLogBatchDetailed(ctx, targetTimestamp, limit)
	return result.Deleted, err
}
