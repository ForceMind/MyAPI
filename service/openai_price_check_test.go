package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/pkg/billingexpr"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOpenAIPriceCheckTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/price-check.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.OfficialPriceVersion{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.PricePublication{}))
	oldDB, oldClient := model.DB, openAIOfficialPricingClient
	model.DB = db
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		model.DB, openAIOfficialPricingClient = oldDB, oldClient
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func claimOpenAIPriceCheckTest(t *testing.T) *model.SystemTask {
	t.Helper()
	task, _, err := EnqueueSystemTaskContext(context.Background(), model.SystemTaskTypeOpenAIPriceCheck, nil)
	require.NoError(t, err)
	claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "price-check-test", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, ok)
	return claimed
}

func TestOpenAIPriceCheckDefaultsAndManualDeduplication(t *testing.T) {
	setupOpenAIPriceCheckTest(t)
	handler := OpenAIPriceCheckHandler{}
	assert.False(t, handler.Enabled())
	assert.Equal(t, 24*time.Hour, handler.Interval())
	assert.Nil(t, handler.NewPayload())
	status, err := ReadOpenAIPriceCheckStatus(context.Background(), common.GetTimestamp())
	require.NoError(t, err)
	assert.True(t, status.Stale)
	assert.Zero(t, status.LastSuccessAt)
	assert.Empty(t, status.Diff)
	assert.NotNil(t, status.Diff)
	first, created, err := EnqueueSystemTaskContext(context.Background(), handler.Type(), nil)
	require.NoError(t, err)
	assert.True(t, created)
	second, created, err := EnqueueSystemTaskContext(context.Background(), handler.Type(), nil)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.TaskID, second.TaskID)
	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.OpenAIOfficialPriceCheckEnabledOptionKey] = "true"
	common.OptionMapRWMutex.Unlock()
	assert.True(t, handler.Enabled())
}

func TestOpenAIPriceCheckSuccessRefreshesSameHashWithoutPublishing(t *testing.T) {
	db := setupOpenAIPriceCheckTest(t)
	ctx := context.Background()
	document := publicationDocumentFixture()
	version, err := model.StoreOfficialPriceVersion(ctx, db, document, 100)
	require.NoError(t, err)
	openAIOfficialPricingClient = &http.Client{Transport: officialPriceRoundTripper(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, openAIOfficialPricingURL, req.URL.String())
		assert.Empty(t, req.Header.Get("Authorization"))
		_, ok := req.Context().Deadline()
		assert.True(t, ok)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(document))}, nil
	})}
	before, err := model.ReadPricePublicationSnapshot(ctx)
	require.NoError(t, err)
	first := claimOpenAIPriceCheckTest(t)
	(OpenAIPriceCheckHandler{}).Run(ctx, first, "price-check-test")
	finished, err := model.GetSystemTaskByTaskID(first.TaskID)
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
	oldResult := model.OpenAIPriceCheckResult{CheckedAt: common.GetTimestamp() - 30, SourceSHA256: version.ContentSHA256, SourceFetchedAt: 100}
	oldJSON, err := common.Marshal(oldResult)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.SystemTask{}).Where("task_id = ?", first.TaskID).Update("result", string(oldJSON)).Error)
	second := claimOpenAIPriceCheckTest(t)
	(OpenAIPriceCheckHandler{}).Run(ctx, second, "price-check-test")
	status, err := ReadOpenAIPriceCheckStatus(ctx, common.GetTimestamp())
	require.NoError(t, err)
	assert.False(t, status.Enabled, "manual success must not enable scheduling")
	assert.False(t, status.Stale)
	assert.Greater(t, status.LastSuccessAt, oldResult.CheckedAt)
	assert.EqualValues(t, 100, status.SourceFetchedAt)
	assert.Equal(t, version.ContentSHA256, status.SourceSHA256)
	assert.Equal(t, version.ContentSHA256, status.PendingSourceSHA256)
	assert.Equal(t, "succeeded", status.LastAttemptStatus)
	require.Len(t, status.Diff, 1)
	assert.Equal(t, "addition", status.Diff[0].Change)
	assert.True(t, status.Diff[0].PendingReview)
	assert.True(t, status.Diff[0].Eligible)
	var count int64
	require.NoError(t, db.Model(&model.OfficialPriceVersion{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&model.PricePublication{}).Count(&count).Error)
	assert.Zero(t, count)
	after, err := model.ReadPricePublicationSnapshot(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	fresh, err := ReadOpenAIPriceCheckStatus(ctx, status.LastSuccessAt+72*3600-1)
	require.NoError(t, err)
	assert.False(t, fresh.Stale)
	stale, err := ReadOpenAIPriceCheckStatus(ctx, status.LastSuccessAt+72*3600)
	require.NoError(t, err)
	assert.True(t, stale.Stale)
}

func TestOpenAIPriceCheckFailureKeepsGoodEvidenceAndLocks(t *testing.T) {
	for _, failure := range []string{"http", "malformed", "oversize", "deadline", "canceled", "lost-lease"} {
		t.Run(failure, func(t *testing.T) {
			db := setupOpenAIPriceCheckTest(t)
			ctx := context.Background()
			version, err := model.StoreOfficialPriceVersion(ctx, db, publicationDocumentFixture(), 100)
			require.NoError(t, err)
			good := claimOpenAIPriceCheckTest(t)
			require.NoError(t, model.CompleteOpenAIPriceCheck(ctx, good, "price-check-test", func(*gorm.DB) (*model.OpenAIPriceCheckResult, error) {
				return &model.OpenAIPriceCheckResult{CheckedAt: common.GetTimestamp() - 10, SourceSHA256: version.ContentSHA256, SourceFetchedAt: 100}, nil
			}, ""))
			for key, value := range map[string]string{
				"billing_setting.billing_mode":     `{"fixture-model":"tiered_expr"}`,
				"billing_setting.billing_expr":     `{"fixture-model":"p * 9 + c * 12"}`,
				"official_price_publication_state": `{"revision":7,"models":{"fixture-model":{"locked":true}}}`,
			} {
				require.NoError(t, db.Create(&model.Option{Key: key, Value: value}).Error)
			}
			before, err := model.ReadPricePublicationSnapshot(ctx)
			require.NoError(t, err)
			task := claimOpenAIPriceCheckTest(t)
			openAIOfficialPricingClient = &http.Client{Transport: officialPriceRoundTripper(func(req *http.Request) (*http.Response, error) {
				code, body := http.StatusOK, publicationDocumentFixture()+"\nnew source\n"
				switch failure {
				case "http":
					code = http.StatusServiceUnavailable
				case "malformed":
					body = "unsupported pricing format"
				case "oversize":
					body = strings.Repeat("x", model.MaxOfficialPriceDocumentBytes+1)
				case "deadline":
					return nil, context.DeadlineExceeded
				case "canceled":
					return nil, req.Context().Err()
				case "lost-lease":
					require.NoError(t, db.Model(&model.SystemTaskLock{}).Where("task_id = ?", task.TaskID).Update("fence_token", task.FenceToken+1).Error)
				}
				return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			runCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			(OpenAIPriceCheckHandler{}).Run(runCtx, task, "price-check-test")
			latest, goodAfter, err := model.ReadOpenAIPriceCheckTasks(ctx)
			require.NoError(t, err)
			assert.Equal(t, good.TaskID, goodAfter.TaskID)
			if failure == "lost-lease" {
				assert.Equal(t, model.SystemTaskStatusRunning, latest.Status)
			} else {
				assert.Equal(t, model.SystemTaskStatusFailed, latest.Status)
			}
			var count int64
			require.NoError(t, db.Model(&model.OfficialPriceVersion{}).Count(&count).Error)
			assert.EqualValues(t, 1, count)
			after, err := model.ReadPricePublicationSnapshot(ctx)
			require.NoError(t, err)
			assert.Equal(t, before, after, "failed check must not change effective price or administrator locks")
			status, err := ReadOpenAIPriceCheckStatus(ctx, common.GetTimestamp())
			require.NoError(t, err)
			assert.Equal(t, version.ContentSHA256, status.SourceSHA256)
			assert.False(t, status.Stale, "a failed attempt does not erase recent good evidence")
		})
	}
}

func TestOpenAIPriceCheckDiffRetainsChangesRemovalsAndUnqualifiedRows(t *testing.T) {
	document := officialPriceFixtureHeader
	for _, name := range []string{"added", "changed", "same", "unqualified"} {
		row := strings.ReplaceAll(strings.ReplaceAll(officialPriceFixtureRow, " (<272K context length)", ""), "fixture-model", name)
		if name == "unqualified" {
			row = strings.Replace(row, "$0.10", "-", 1)
		}
		document += row
	}
	document += "\nShort context: ≤272K input tokens. Long context: >272K input tokens.\n"
	source := publicationSourceFixture(document)
	var err error
	source.Models, err = parseOpenAIStandardTextPrices([]byte(document))
	require.NoError(t, err)
	candidate, err := BuildOpenAIPricePublicationCandidate(source, "same")
	require.NoError(t, err)
	current := &model.PricePublicationSnapshot{
		Modes:       map[string]string{"same": "tiered_expr", "changed": "tiered_expr", "removed": "tiered_expr"},
		Expressions: map[string]string{"same": candidate.Expression, "changed": "p * 20", "removed": "p * 3"},
		State:       model.PricePublicationState{Models: map[string]model.PublishedModelPrice{"changed": {Locked: true}, "removed": {SourceSHA256: strings.Repeat("a", 64)}, "same": {SourceSHA256: strings.Repeat("a", 64), PublicationID: strings.Repeat("b", 64), ExpressionSHA256: candidate.ExpressionSHA256}}},
	}
	status := &OpenAIPriceCheckStatus{DiffCounts: map[string]int{}}
	require.NoError(t, buildOpenAIPriceCheckDiff(context.Background(), status, source, current))
	assert.Equal(t, map[string]int{"addition": 1, "change": 1, "unchanged": 1, "unqualified": 1, "removal": 1}, status.DiffCounts)
	require.Len(t, status.Diff, 5)
	assert.Equal(t, "changed", status.Diff[1].Model)
	assert.True(t, status.Diff[1].Locked)
	assert.False(t, status.Diff[1].Eligible)
	assert.True(t, status.Diff[1].PendingReview)
	assert.Equal(t, billingexpr.ExprHashString("p * 20"), status.Diff[1].CurrentExpressionSHA256)
	assert.Nil(t, status.Diff[2].Candidate, "removed model has no replacement price")
	assert.False(t, status.Diff[3].PendingReview, "identical expression has no pending change")
	assert.False(t, status.Diff[4].Eligible)
	assert.True(t, status.DiffReviewRequired)
	assert.Equal(t, source.ContentSHA256, status.PendingSourceSHA256)
}

func TestOpenAIPriceCheckDiffIsBoundedAndMarksIncompleteReview(t *testing.T) {
	source := publicationSourceFixture(publicationDocumentFixture())
	current := &model.PricePublicationSnapshot{State: model.PricePublicationState{Models: map[string]model.PublishedModelPrice{}}}
	for i := 0; i < 140; i++ {
		current.State.Models[fmt.Sprintf("removed-%03d", i)] = model.PublishedModelPrice{SourceSHA256: strings.Repeat("a", 64)}
	}
	status := &OpenAIPriceCheckStatus{DiffCounts: map[string]int{}}
	require.NoError(t, buildOpenAIPriceCheckDiff(context.Background(), status, source, current))
	assert.Len(t, status.Diff, 128)
	assert.Equal(t, 140, status.DiffTotal)
	assert.True(t, status.DiffTruncated)
	assert.True(t, status.DiffReviewRequired)
	assert.Equal(t, source.ContentSHA256, status.PendingSourceSHA256)
}

func TestOpenAIPriceCheckMatchingManualTariffStillNeedsPublicationReview(t *testing.T) {
	source := publicationSourceFixture(publicationDocumentFixture())
	var err error
	source.Models, err = parseOpenAIStandardTextPrices([]byte(source.sourceDocument))
	require.NoError(t, err)
	candidate, err := BuildOpenAIPricePublicationCandidate(source, "fixture-model")
	require.NoError(t, err)
	current := &model.PricePublicationSnapshot{Modes: map[string]string{"fixture-model": "tiered_expr"}, Expressions: map[string]string{"fixture-model": candidate.Expression}}
	status := &OpenAIPriceCheckStatus{DiffCounts: map[string]int{}}
	require.NoError(t, buildOpenAIPriceCheckDiff(context.Background(), status, source, current))
	require.Len(t, status.Diff, 1)
	assert.Equal(t, "change", status.Diff[0].Change)
	assert.True(t, status.Diff[0].PendingReview)
	assert.True(t, status.DiffReviewRequired, "an equal manual tariff is not an official publication")
}

func TestOpenAIPriceCheckSchedulerUsesDailyCadenceAndSharedActiveKey(t *testing.T) {
	db := setupOpenAIPriceCheckTest(t)
	handler := OpenAIPriceCheckHandler{}
	withSystemTaskRegistry(t, handler)
	runSystemTaskScheduler()
	assert.Zero(t, countSystemTasks(t, handler.Type()), "fresh installations have no periodic checks")
	common.OptionMapRWMutex.Lock()
	common.OptionMap[model.OpenAIOfficialPriceCheckEnabledOptionKey] = "true"
	common.OptionMapRWMutex.Unlock()
	runSystemTaskScheduler()
	assert.EqualValues(t, 1, countSystemTasks(t, handler.Type()))
	task := claimOpenAIPriceCheckTest(t)
	runSystemTaskScheduler()
	assert.EqualValues(t, 1, countSystemTasks(t, handler.Type()), "manual enqueue and running scheduled task share one active key")
	require.NoError(t, model.CompleteOpenAIPriceCheck(context.Background(), task, "price-check-test", nil, "source_fetch_failed"))
	runSystemTaskScheduler()
	assert.EqualValues(t, 1, countSystemTasks(t, handler.Type()), "failed checks must not hot-loop")
	require.NoError(t, db.Model(&model.SystemTask{}).Where("task_id = ?", task.TaskID).Update("updated_at", common.GetTimestamp()-86401).Error)
	runSystemTaskScheduler()
	assert.EqualValues(t, 2, countSystemTasks(t, handler.Type()))
	runSystemTaskScheduler()
	assert.EqualValues(t, 2, countSystemTasks(t, handler.Type()), "overdue active check prevents overlap")
}

func TestOpenAIPriceCheckTruncatedNamesKeepDistinctIdentities(t *testing.T) {
	prefix := strings.Repeat("m", 191)
	document := officialPriceFixtureHeader
	for _, name := range []string{prefix + "a", prefix + "b"} {
		document += strings.ReplaceAll(strings.ReplaceAll(officialPriceFixtureRow, " (<272K context length)", ""), "fixture-model", name)
	}
	document += "\nShort context: ≤272K input tokens. Long context: >272K input tokens.\n"
	source := publicationSourceFixture(document)
	var err error
	source.Models, err = parseOpenAIStandardTextPrices([]byte(document))
	require.NoError(t, err)
	status := &OpenAIPriceCheckStatus{DiffCounts: map[string]int{}}
	require.NoError(t, buildOpenAIPriceCheckDiff(context.Background(), status, source, &model.PricePublicationSnapshot{}))
	require.Len(t, status.Diff, 2)
	assert.Equal(t, status.Diff[0].Model, status.Diff[1].Model)
	assert.NotEqual(t, status.Diff[0].ModelSHA256, status.Diff[1].ModelSHA256)
	for _, row := range status.Diff {
		assert.True(t, row.ModelNameTruncated)
		assert.Equal(t, "unqualified", row.Change)
		assert.True(t, row.PendingReview)
		assert.False(t, row.Eligible)
	}
}
