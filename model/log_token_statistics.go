package model

import (
	"context"
	"database/sql"
	"math/big"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// LogTokenUsageEvidence is additive observation metadata, never billing input.
// Nil means the provider did not establish the counter, including cached zero.
type LogTokenUsageEvidence struct {
	Schema          int    `json:"schema"`
	InputTokens     *int64 `json:"input_tokens"`
	OutputTokens    *int64 `json:"output_tokens"`
	CacheReadTokens *int64 `json:"cache_read_tokens"`
}

type LogTokenStatisticsFilter struct {
	UserID, Type, Channel, TokenID                              int
	Start, End                                                  int64
	Model, Username, Token, Group, RequestID, UpstreamRequestID string
	Result                                                      string
}

// Decimal strings keep aggregate counts exact beyond JavaScript's safe integer.
// Overall totals are null when any matching call record has unknown usage.
type LogTokenStatistics struct {
	Records             int64    `json:"records"`
	KnownRecords        int64    `json:"known_records"`
	UnknownRecords      int64    `json:"unknown_records"`
	InputTokens         *string  `json:"input_tokens"`
	OutputTokens        *string  `json:"output_tokens"`
	TotalTokens         *string  `json:"total_tokens"`
	KnownInputTokens    string   `json:"known_input_tokens"`
	KnownOutputTokens   string   `json:"known_output_tokens"`
	KnownTotalTokens    string   `json:"known_total_tokens"`
	CacheKnownRecords   int64    `json:"cache_known_records"`
	CacheUnknownRecords int64    `json:"cache_unknown_records"`
	CacheReadTokens     string   `json:"cache_read_tokens"`
	CacheInputTokens    string   `json:"cache_input_tokens"`
	CacheHitRate        *float64 `json:"cache_hit_rate"`
}

// GetLogTokenStatistics streams only whitelisted columns from canonical rows.
// It does not parse JSON in SQL, load request bodies, cap the result silently,
// write historical data, or depend on a particular database dialect.
func GetLogTokenStatistics(ctx context.Context, db *gorm.DB, filter LogTokenStatisticsFilter) (LogTokenStatistics, error) {
	result := LogTokenStatistics{}
	if db == nil {
		return result, gorm.ErrInvalidDB
	}
	if filter.UserID < 0 || filter.TokenID < 0 {
		return result, gorm.ErrInvalidData
	}
	query := deduplicatedLogs(db).WithContext(ctx)
	if filter.Type == LogTypeUnknown {
		query = query.Where("logs.type IN ?", []int{LogTypeConsume, LogTypeError})
	} else if filter.Type == LogTypeConsume || filter.Type == LogTypeError {
		query = query.Where("logs.type = ?", filter.Type)
	} else {
		query = query.Where("1 = 0")
	}
	query, err := applyLogResultFilter(query, filter.Result)
	if err != nil {
		return result, err
	}
	if filter.UserID > 0 {
		query = query.Where("logs.user_id = ?", filter.UserID)
	}
	if filter.TokenID > 0 {
		query = query.Where("logs.token_id = ?", filter.TokenID)
	}
	if query, err = applyExplicitLogTextFilter(query, "logs.model_name", filter.Model); err != nil {
		return result, err
	}
	if filter.UserID == 0 {
		if query, err = applyExplicitLogTextFilter(query, "logs.username", filter.Username); err != nil {
			return result, err
		}
		if filter.Channel != 0 {
			query = query.Where("logs.channel_id = ?", filter.Channel)
		}
	}
	for _, field := range []struct{ column, value string }{
		{"logs.token_name", filter.Token}, {"logs.request_id", filter.RequestID},
		{"logs.upstream_request_id", filter.UpstreamRequestID},
		{"logs." + logGroupColumnForDB(db), filter.Group},
	} {
		if field.value != "" {
			query = query.Where(field.column+" = ?", field.value)
		}
	}
	if filter.Start != 0 {
		query = query.Where("logs.created_at >= ?", filter.Start)
	}
	if filter.End != 0 {
		query = query.Where("logs.created_at <= ?", filter.End)
	}
	rows, err := query.Select("logs.type, logs.prompt_tokens, logs.completion_tokens, logs.other").Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()
	var input, output, cached, cacheInput big.Int
	for rows.Next() {
		var kind int
		var prompt, completion sql.NullInt64
		var metadata sql.NullString
		if err := rows.Scan(&kind, &prompt, &completion, &metadata); err != nil {
			return LogTokenStatistics{}, err
		}
		result.Records++
		evidence := logTokenEvidence(kind, prompt, completion, metadata.String)
		if evidence == nil {
			result.UnknownRecords++
			continue
		}
		result.KnownRecords++
		input.Add(&input, big.NewInt(*evidence.InputTokens))
		output.Add(&output, big.NewInt(*evidence.OutputTokens))
		if evidence.CacheReadTokens != nil {
			result.CacheKnownRecords++
			cached.Add(&cached, big.NewInt(*evidence.CacheReadTokens))
			cacheInput.Add(&cacheInput, big.NewInt(*evidence.InputTokens))
		}
	}
	if err := rows.Err(); err != nil {
		return LogTokenStatistics{}, err
	}
	result.KnownInputTokens, result.KnownOutputTokens = input.String(), output.String()
	result.KnownTotalTokens = new(big.Int).Add(&input, &output).String()
	if result.UnknownRecords == 0 {
		result.InputTokens, result.OutputTokens, result.TotalTokens = &result.KnownInputTokens, &result.KnownOutputTokens, &result.KnownTotalTokens
	}
	result.CacheUnknownRecords = result.Records - result.CacheKnownRecords
	result.CacheReadTokens, result.CacheInputTokens = cached.String(), cacheInput.String()
	if cacheInput.Sign() > 0 {
		rate, _ := new(big.Rat).SetFrac(&cached, &cacheInput).Float64()
		result.CacheHitRate = &rate
	}
	return result, nil
}

func logTokenEvidence(kind int, prompt, completion sql.NullInt64, metadata string) *LogTokenUsageEvidence {
	var other struct {
		UsageAccuracy string                 `json:"usage_accuracy"`
		UsageOutcome  string                 `json:"usage_outcome"`
		UsageSemantic string                 `json:"usage_semantic"`
		InputTotal    *int64                 `json:"input_tokens_total"`
		Cache         *int64                 `json:"cache_tokens"`
		Write         *int64                 `json:"cache_write_tokens"`
		Creation      *int64                 `json:"cache_creation_tokens"`
		Creation5m    *int64                 `json:"cache_creation_tokens_5m"`
		Creation1h    *int64                 `json:"cache_creation_tokens_1h"`
		Evidence      *LogTokenUsageEvidence `json:"token_usage_evidence"`
	}
	if metadata != "" && common.UnmarshalJsonStr(metadata, &other) != nil {
		return nil
	}
	if other.UsageAccuracy != "" && other.UsageAccuracy != "reported" || other.UsageOutcome != "" && other.UsageOutcome != "reported" {
		return nil
	}
	if e := other.Evidence; e != nil {
		if e.Schema != 1 || e.InputTokens == nil || e.OutputTokens == nil || *e.InputTokens < 0 || *e.OutputTokens < 0 {
			return nil
		}
		if e.CacheReadTokens != nil && (*e.CacheReadTokens < 0 || *e.CacheReadTokens > *e.InputTokens) {
			e.CacheReadTokens = nil
		}
		return e
	}
	if !prompt.Valid || !completion.Valid || prompt.Int64 < 0 || completion.Int64 < 0 || prompt.Int64 > int64(common.MaxQuota) || completion.Int64 > int64(common.MaxQuota) {
		return nil
	}
	if other.UsageAccuracy == "" && other.UsageOutcome == "" {
		return nil
	}
	input, output := prompt.Int64, completion.Int64
	if other.InputTotal != nil {
		if *other.InputTotal < prompt.Int64 {
			return nil
		}
		input = *other.InputTotal
	} else if other.UsageSemantic == "anthropic" {
		var cache, write, split int64
		for _, pair := range []struct {
			from *int64
			to   *int64
		}{{other.Cache, &cache}, {other.Write, &write}, {other.Creation5m, &split}, {other.Creation1h, &split}} {
			if pair.from != nil {
				if *pair.from < 0 || *pair.from > int64(common.MaxQuota) {
					return nil
				}
				*pair.to += *pair.from
			}
		}
		if other.Write == nil {
			write = split
			if other.Creation != nil {
				if *other.Creation < 0 || *other.Creation > int64(common.MaxQuota) {
					return nil
				}
				if *other.Creation > write {
					write = *other.Creation
				}
			}
		}
		input += cache + write
	}
	if kind != LogTypeConsume || input == 0 && output == 0 {
		return nil
	}
	e := &LogTokenUsageEvidence{Schema: 1, InputTokens: &input, OutputTokens: &output}
	// Historical cache_tokens=0 was written even when the upstream omitted it.
	if other.Cache != nil && *other.Cache > 0 && *other.Cache <= input {
		e.CacheReadTokens = other.Cache
	}
	return e
}
