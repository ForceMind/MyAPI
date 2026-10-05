package service

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	"gorm.io/gorm"
)

const (
	OpenAIPriceCheckInterval     = 24 * time.Hour
	OpenAIPriceCheckStaleAfter   = 72 * time.Hour
	openAIPriceCheckTimeout      = 15 * time.Second
	openAIPriceCheckWriteTimeout = 5 * time.Second
	maxOpenAIPriceCheckDiffRows  = 128
)

// OpenAIPriceCheckHandler reuses the scheduler's active key, DB lease and
// completion-to-start cadence. Manual checks run even while scheduling is off.
type OpenAIPriceCheckHandler struct{}

func (OpenAIPriceCheckHandler) Type() string            { return model.SystemTaskTypeOpenAIPriceCheck }
func (OpenAIPriceCheckHandler) Interval() time.Duration { return OpenAIPriceCheckInterval }
func (OpenAIPriceCheckHandler) NewPayload() any         { return nil }
func (OpenAIPriceCheckHandler) Enabled() bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[model.OpenAIOfficialPriceCheckEnabledOptionKey] == "true"
}

func (OpenAIPriceCheckHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	// The existing fixed-URL client also enforces TLS, no redirects, response
	// size and its own 15-second deadline. This context bounds the whole run,
	// including freezing the source and writing the success receipt.
	runCtx, cancel := context.WithTimeout(ctx, openAIPriceCheckTimeout)
	defer cancel()
	source, err := FetchOpenAIOfficialPricing(runCtx)
	code := "source_fetch_failed"
	if err == nil {
		code = "source_save_failed"
		err = model.CompleteOpenAIPriceCheck(runCtx, task, runnerID, func(tx *gorm.DB) (*model.OpenAIPriceCheckResult, error) {
			frozen, err := FreezeOpenAIPriceSource(runCtx, tx, source)
			if err != nil {
				return nil, err
			}
			return &model.OpenAIPriceCheckResult{CheckedAt: common.GetTimestamp(), SourceSHA256: frozen.ContentSHA256, SourceFetchedAt: frozen.FetchedAt}, nil
		}, "")
	}
	if err == nil {
		return
	}
	if errors.Is(err, model.ErrSystemTaskLockLost) {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		code = "check_timeout"
	} else if errors.Is(err, context.Canceled) {
		code = "check_canceled"
	}
	// A canceled fetch still gets a bounded terminal attempt record. Fencing
	// prevents this cleanup from modifying a replacement worker's task.
	writeCtx, writeCancel := context.WithTimeout(context.Background(), openAIPriceCheckWriteTimeout)
	defer writeCancel()
	if finishErr := model.CompleteOpenAIPriceCheck(writeCtx, task, runnerID, nil, code); finishErr != nil {
		common.SysLog("official price check failed to persist terminal state")
	}
}

type OpenAIPriceCheckDiffRow struct {
	ModelSHA256             string                           `json:"model_sha256"`
	Model                   string                           `json:"model"`
	ModelNameTruncated      bool                             `json:"model_name_truncated"`
	Change                  string                           `json:"change"`
	CurrentMode             string                           `json:"current_mode"`
	CurrentExpressionSHA256 string                           `json:"current_expression_sha256"`
	Candidate               *OpenAIPricePublicationCandidate `json:"candidate"`
	Locked                  bool                             `json:"locked"`
	Eligible                bool                             `json:"eligible"`
	PendingReview           bool                             `json:"pending_review"`
}

type OpenAIPriceCheckStatus struct {
	Enabled             bool   `json:"enabled"`
	IntervalSeconds     int64  `json:"interval_seconds"`
	StaleAfterSeconds   int64  `json:"stale_after_seconds"`
	Stale               bool   `json:"stale"`
	LastAttemptAt       int64  `json:"last_attempt_at"`
	LastAttemptStatus   string `json:"last_attempt_status"`
	LastTaskID          string `json:"last_task_id"`
	LastErrorCode       string `json:"last_error_code"`
	LastSuccessAt       int64  `json:"last_success_at"`
	NextCheckAt         int64  `json:"next_check_at"`
	SourceSHA256        string `json:"source_sha256"`
	SourceFetchedAt     int64  `json:"source_fetched_at"`
	PendingSourceSHA256 string `json:"pending_source_sha256"`
	ExpectedDigest      string `json:"expected_digest"`
	Revision            int64  `json:"revision"`
	DiffTotal           int    `json:"diff_total"`
	DiffTruncated       bool   `json:"diff_truncated"`
	DiffReviewRequired  bool   `json:"diff_review_required"`
	// Counts apply only to the returned rows. Truncation always leaves review
	// pending; it must never be presented as a complete/no-change comparison.
	DiffCounts map[string]int            `json:"diff_counts"`
	Diff       []OpenAIPriceCheckDiffRow `json:"diff"`
}

func ReadOpenAIPriceCheckStatus(ctx context.Context, now int64) (*OpenAIPriceCheckStatus, error) {
	status := &OpenAIPriceCheckStatus{
		Enabled: (OpenAIPriceCheckHandler{}).Enabled(), IntervalSeconds: int64(OpenAIPriceCheckInterval / time.Second),
		StaleAfterSeconds: int64(OpenAIPriceCheckStaleAfter / time.Second), Stale: true,
		DiffCounts: map[string]int{"addition": 0, "change": 0, "removal": 0, "unqualified": 0, "unchanged": 0},
		Diff:       make([]OpenAIPriceCheckDiffRow, 0),
	}
	latest, successful, err := model.ReadOpenAIPriceCheckTasks(ctx)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		status.LastAttemptAt, status.LastAttemptStatus, status.LastTaskID = latest.CreatedAt, string(latest.Status), latest.TaskID
		switch latest.Error {
		case "source_fetch_failed", "source_save_failed", "check_timeout", "check_canceled":
			status.LastErrorCode = latest.Error
		case "task lease expired":
			status.LastErrorCode = "lease_lost"
		default:
			if latest.Status == model.SystemTaskStatusFailed {
				status.LastErrorCode = "check_failed"
			}
		}
		if status.Enabled && (latest.Status == model.SystemTaskStatusSucceeded || latest.Status == model.SystemTaskStatusFailed) {
			status.NextCheckAt = latest.UpdatedAt + status.IntervalSeconds
		}
	}
	current, err := model.ReadPricePublicationSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	status.ExpectedDigest, err = current.Digest()
	if err != nil {
		return nil, err
	}
	status.Revision = current.State.Revision
	if successful == nil {
		return status, nil
	}
	var result model.OpenAIPriceCheckResult
	if len(successful.Result) > 4096 || common.UnmarshalJsonStr(successful.Result, &result) != nil || result.CheckedAt <= 0 || result.SourceFetchedAt <= 0 || result.CheckedAt > now {
		return nil, gorm.ErrInvalidData
	}
	source, err := LoadFrozenOpenAIPriceSource(ctx, model.DB, result.SourceSHA256)
	if err != nil {
		return nil, err
	}
	status.LastSuccessAt, status.SourceSHA256, status.SourceFetchedAt = result.CheckedAt, source.ContentSHA256, source.FetchedAt
	status.Stale = now-result.CheckedAt >= status.StaleAfterSeconds
	if err := buildOpenAIPriceCheckDiff(ctx, status, source, current); err != nil {
		return nil, err
	}
	return status, nil
}

// Compare against one live generation, using the same server-only candidate
// builder as publication preview. The returned digest forces later publication
// to recheck that generation. A source disappearance never removes a tariff.
func buildOpenAIPriceCheckDiff(ctx context.Context, status *OpenAIPriceCheckStatus, source *OpenAIOfficialPriceSnapshot, current *model.PricePublicationSnapshot) error {
	sourceModels := make(map[string]bool, len(source.Models))
	names := make([]string, 0, len(source.Models)+len(current.State.Models))
	for _, price := range source.Models {
		sourceModels[price.Model] = true
		names = append(names, price.Model)
	}
	for name, binding := range current.State.Models {
		if !sourceModels[name] && binding.SourceSHA256 != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	status.DiffTotal = len(names)
	if len(names) > maxOpenAIPriceCheckDiffRows {
		names = names[:maxOpenAIPriceCheckDiffRows]
		status.DiffTruncated, status.DiffReviewRequired = true, true
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		row := OpenAIPriceCheckDiffRow{Model: name, ModelSHA256: hex.EncodeToString(common.Sha256Raw([]byte(name))), CurrentMode: current.Modes[name], Locked: current.State.Models[name].Locked}
		if row.CurrentMode == "" {
			row.CurrentMode = "ratio"
		}
		if expression := current.Expressions[name]; expression != "" {
			row.CurrentExpressionSHA256 = billingexpr.ExprHashString(expression)
		}
		if !sourceModels[name] {
			row.Change = "removal"
		} else {
			candidate, err := BuildOpenAIPricePublicationCandidate(source, name)
			row.Candidate, row.Eligible = candidate, err == nil && !row.Locked
			switch {
			case err != nil:
				row.Change = "unqualified"
			case row.CurrentMode == "tiered_expr" && row.CurrentExpressionSHA256 == candidate.ExpressionSHA256 &&
				current.State.Models[name].PublicationID != "" && current.State.Models[name].SourceSHA256 != "" &&
				current.State.Models[name].ExpressionSHA256 == candidate.ExpressionSHA256:
				row.Change = "unchanged"
			case current.Expressions[name] == "" && current.State.Models[name].SourceSHA256 == "":
				row.Change = "addition"
			default:
				row.Change = "change"
			}
		}
		if len(row.Model) > 191 {
			row.Model, row.ModelNameTruncated = row.Model[:191], true
		}
		row.PendingReview = row.Change != "unchanged"
		status.DiffCounts[row.Change]++
		status.DiffReviewRequired = status.DiffReviewRequired || row.PendingReview
		status.Diff = append(status.Diff, row)
	}
	if status.DiffReviewRequired {
		status.PendingSourceSHA256 = source.ContentSHA256
	}
	return ctx.Err()
}
