package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type pollingBoundaryAdaptor struct {
	statusCode int
	body       []byte
	info       *relaycommon.TaskInfo
	parseErr   error
	adjust     int
}

func (a *pollingBoundaryAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *pollingBoundaryAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	status := a.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(a.body))}, nil
}
func (a *pollingBoundaryAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	if a.parseErr != nil {
		return nil, a.parseErr
	}
	if a.info == nil {
		return nil, nil
	}
	copyInfo := *a.info
	return &copyInfo, nil
}
func (a *pollingBoundaryAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return a.adjust
}

func TestDurableVideoUncertainResponsesDoNotApplyEconomics(t *testing.T) {
	for _, tc := range []struct {
		name        string
		statusCode  int
		body        string
		info        *relaycommon.TaskInfo
		parseErr    error
		disposition string
		reason      string
	}{
		{"http_401", 401, `{}`, nil, nil, model.TaskPollingDispositionManual, "video_http_401"},
		{"http_403", 403, `{}`, nil, nil, model.TaskPollingDispositionManual, "video_http_403"},
		{"http_404", 404, `{}`, nil, nil, model.TaskPollingDispositionManual, "video_http_404"},
		{"http_500", 500, `{}`, nil, nil, model.TaskPollingDispositionRetryable, "video_http_5xx"},
		{"error_envelope", 200, `{"error":{"message":"private"}}`, &relaycommon.TaskInfo{Status: string(model.TaskStatusFailure)}, nil, model.TaskPollingDispositionRetryable, "video_error_envelope"},
		{"empty_status", 200, `{"state":"pending"}`, &relaycommon.TaskInfo{}, nil, model.TaskPollingDispositionRetryable, "video_status_unknown"},
		{"unknown_status", 200, `{"state":"mystery"}`, &relaycommon.TaskInfo{Status: "MYSTERY"}, nil, model.TaskPollingDispositionRetryable, "video_status_unknown"},
		{"invalid_json", 200, `{`, nil, errors.New("invalid provider JSON"), model.TaskPollingDispositionRetryable, "video_error_envelope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBridgeTestDB(t)
			fixture := newBridgeTestFixture(t, db, tc.name, 1000, 1000, 100)
			prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
			var channel model.Channel
			require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
			adaptor := &pollingBoundaryAdaptor{statusCode: tc.statusCode, body: []byte(tc.body), info: tc.info, parseErr: tc.parseErr}
			err := updateVideoSingleTask(context.Background(), adaptor, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task})
			require.Error(t, err)
			var task model.Task
			require.NoError(t, db.First(&task, fixture.Task.ID).Error)
			assert.EqualValues(t, model.TaskStatusInProgress, task.Status)
			assert.Equal(t, 100, task.Quota)
			assert.Equal(t, tc.disposition, task.PollingDisposition)
			assert.Equal(t, tc.reason, task.PollingReasonCode)
			var observations int64
			require.NoError(t, db.Model(&model.TaskTerminalObservation{}).Where("operation_id = ?", fixture.Operation.ID).Count(&observations).Error)
			assert.Zero(t, observations)
			var user model.User
			require.NoError(t, db.First(&user, fixture.User.Id).Error)
			assert.Equal(t, 900, user.Quota)
			assert.Equal(t, 100, user.UsedQuota)
		})
	}
}

func TestDurableVideoResponseIDMismatchDoesNotApplyTerminal(t *testing.T) {
	for _, status := range []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure} {
		t.Run(string(status), func(t *testing.T) {
			db := setupBridgeTestDB(t)
			fixture := newBridgeTestFixture(t, db, "id-mismatch-"+string(status), 1000, 1000, 100)
			prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
			var channel model.Channel
			require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
			adaptor := &pollingBoundaryAdaptor{body: []byte(`{"state":"terminal"}`), info: &relaycommon.TaskInfo{TaskID: "another-task", Status: string(status), Reason: "provider.failed"}, adjust: 80}
			require.Error(t, updateVideoSingleTask(context.Background(), adaptor, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
			var task model.Task
			require.NoError(t, db.First(&task, fixture.Task.ID).Error)
			assert.EqualValues(t, model.TaskStatusInProgress, task.Status)
			assert.Equal(t, model.TaskPollingDispositionManual, task.PollingDisposition)
			assert.Equal(t, "video_response_id_mismatch", task.PollingReasonCode)
			var observations int64
			require.NoError(t, db.Model(&model.TaskTerminalObservation{}).Count(&observations).Error)
			assert.Zero(t, observations)
		})
	}
}

func TestDurablePerCallSettlementUsesImmutableReceiptQuota(t *testing.T) {
	db := setupBridgeTestDB(t)
	billingContext := model.TaskBillingContext{Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1, ModelRatio: 1, GroupRatio: 1, OriginModelName: "test-model", PerCallBilling: true}
	fixture := newBridgeTestFixture(t, db, "immutable-per-call", 1000, 1000, 100, billingContext)
	prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
	fixture.Task.Quota = 777
	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", fixture.Task.ID).Update("quota", 777).Error)
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	adaptor := &pollingBoundaryAdaptor{body: []byte(`{"state":"done"}`), info: &relaycommon.TaskInfo{TaskID: fixture.Attempt.ProviderOperationID, Status: string(model.TaskStatusSuccess)}}
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
	var task model.Task
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	assert.Equal(t, 100, task.Quota)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
}

func TestComputeTaskQuotaFromTokensDoesNotFallbackToMutableQuota(t *testing.T) {
	task := &model.Task{Quota: 999, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{Version: 99, Complete: true}}}
	quota, clamp, ok := computeTaskQuotaFromTokens(task, 100)
	assert.False(t, ok)
	assert.Zero(t, quota)
	assert.Nil(t, clamp)
}

func TestLatePollingErrorCannotOverrideAppliedTerminal(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "late-error", 1000, 1000, 100)
	prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	success := &pollingBoundaryAdaptor{body: []byte(`{"state":"done"}`), info: &relaycommon.TaskInfo{TaskID: fixture.Attempt.ProviderOperationID, Status: string(model.TaskStatusSuccess)}, adjust: 100}
	require.NoError(t, updateVideoSingleTask(context.Background(), success, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
	stale := fixture.Task
	late := &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewReader([]byte(`{}`)))}
	require.Error(t, ApplyRealtimeTaskPollingResponse(context.Background(), success, &channel, &stale, late))
	var task model.Task
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, task.Status)
	assert.Empty(t, task.PollingDisposition)
	var observation model.TaskTerminalObservation
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, model.TaskTerminalObservationApplied, observation.State)
}

func TestSunoBatchUncertaintyDisposition(t *testing.T) {
	for _, tc := range []struct {
		name        string
		items       func(string) []taskdto.SunoDataResponse
		disposition string
		reason      string
	}{
		{"pending", func(id string) []taskdto.SunoDataResponse {
			return []taskdto.SunoDataResponse{{TaskID: id, Status: string(model.TaskStatusInProgress)}}
		}, "", ""},
		{"missing", func(string) []taskdto.SunoDataResponse { return nil }, model.TaskPollingDispositionRetryable, "suno_task_missing"},
		{"unknown", func(id string) []taskdto.SunoDataResponse {
			return []taskdto.SunoDataResponse{{TaskID: id, Status: "MYSTERY"}}
		}, model.TaskPollingDispositionRetryable, "suno_status_unknown"},
		{"foreign_id", func(string) []taskdto.SunoDataResponse {
			return []taskdto.SunoDataResponse{{TaskID: "foreign", Status: string(model.TaskStatusSuccess)}}
		}, model.TaskPollingDispositionRetryable, "suno_task_missing"},
		{"duplicate", func(id string) []taskdto.SunoDataResponse {
			return []taskdto.SunoDataResponse{{TaskID: id, Status: string(model.TaskStatusInProgress)}, {TaskID: id, Status: string(model.TaskStatusInProgress)}}
		}, model.TaskPollingDispositionManual, "suno_duplicate_task_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBridgeTestDB(t)
			fixture := newBridgeTestFixture(t, db, "suno-"+tc.name, 1000, 1000, 100)
			prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatformSuno)
			baseURL := "https://suno.example.test"
			require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", fixture.Attempt.ChannelID).Updates(map[string]interface{}{"type": constant.ChannelTypeSunoAPI, "base_url": baseURL}).Error)
			adaptor := &sunoTerminalBatchAdaptor{items: tc.items(fixture.Attempt.ProviderOperationID)}
			previousFactory := GetTaskAdaptorFunc
			GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
			t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
			require.NoError(t, updateSunoTasks(context.Background(), fixture.Attempt.ChannelID, []string{fixture.Attempt.ProviderOperationID}, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
			var task model.Task
			require.NoError(t, db.First(&task, fixture.Task.ID).Error)
			assert.Equal(t, tc.disposition, task.PollingDisposition)
			assert.Equal(t, tc.reason, task.PollingReasonCode)
			var observations int64
			require.NoError(t, db.Model(&model.TaskTerminalObservation{}).Count(&observations).Error)
			assert.Zero(t, observations)
		})
	}
}

func TestPollingCompositeKeySeparatesChannelsInBothInsertionOrders(t *testing.T) {
	first := &model.Task{ID: 1, TaskID: "public-durable", ChannelId: 11, PrivateData: model.TaskPrivateData{UpstreamTaskID: "same-upstream"}}
	second := &model.Task{ID: 2, TaskID: "public-legacy", ChannelId: 22, PrivateData: model.TaskPrivateData{UpstreamTaskID: "same-upstream"}}
	contexts := &TaskPollingContextSet{byPublicID: map[string]*TaskPollingContext{
		first.TaskID:  {Operation: &model.TaskSubmissionOperation{ID: 1}},
		second.TaskID: {},
	}}
	for _, tasks := range [][]*model.Task{{first, second}, {second, first}} {
		batch, duplicates := newTaskPollingBatch(tasks, contexts)
		assert.Empty(t, duplicates)
		require.Len(t, batch.entries, 2)
		assert.Same(t, first, batch.entries[taskPollingKey{ChannelID: 11, UpstreamID: "same-upstream"}].Task)
		assert.Same(t, second, batch.entries[taskPollingKey{ChannelID: 22, UpstreamID: "same-upstream"}].Task)
	}
}

func TestRunTaskPollingOnceSeparatesDurableAndLegacySameUpstreamAcrossChannels(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "mixed-same-upstream", 1000, 1000, 100)
	prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
	const legacyChannelID = 202
	legacyChannel := model.Channel{Id: legacyChannelID, Type: constant.ChannelTypeKling, Name: "legacy-channel", Key: "legacy-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&legacyChannel).Error)
	legacyTask := model.Task{TaskID: "legacy-public", Platform: constant.TaskPlatform("kling"), UserId: fixture.User.Id, ChannelId: legacyChannelID, Status: model.TaskStatusInProgress, Progress: "40%", SubmitTime: time.Now().Unix(), PrivateData: model.TaskPrivateData{UpstreamTaskID: fixture.Attempt.ProviderOperationID}}
	require.NoError(t, db.Create(&legacyTask).Error)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	adaptor := &taskPollingFetchAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 0
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
	previousLimit := constant.TaskQueryLimit
	constant.TaskQueryLimit = 100
	t.Cleanup(func() { constant.TaskQueryLimit = previousLimit })

	summary := RunTaskPollingOnce(context.Background(), nil)
	assert.Equal(t, 2, summary.UnfinishedTasks)
	assert.Equal(t, 2, adaptor.fetchCount())
	assert.Equal(t, []string{fixture.Attempt.ProviderOperationID, fixture.Attempt.ProviderOperationID}, adaptor.fetchedTaskIDs())
}

type sunoEchoPendingAdaptor struct {
	calls int
}

func (*sunoEchoPendingAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *sunoEchoPendingAdaptor) FetchTask(_ string, _ string, body map[string]any, _ string) (*http.Response, error) {
	a.calls++
	ids, _ := body["ids"].([]string)
	items := make([]taskdto.SunoDataResponse, 0, len(ids))
	for _, id := range ids {
		items = append(items, taskdto.SunoDataResponse{TaskID: id, Status: string(model.TaskStatusInProgress)})
	}
	encoded, err := common.Marshal(taskdto.TaskResponse[[]taskdto.SunoDataResponse]{Code: taskdto.TaskSuccessCode, Data: items})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(encoded))}, nil
}
func (*sunoEchoPendingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}
func (*sunoEchoPendingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestRunTaskPollingOnceSeparatesSunoChannelsWithSameUpstreamID(t *testing.T) {
	db := setupBridgeTestDB(t)
	const upstreamID = "same-suno-upstream"
	for _, channelID := range []int{301, 302} {
		baseURL := "https://suno.example.test"
		channel := model.Channel{Id: channelID, Type: constant.ChannelTypeSunoAPI, Name: fmt.Sprintf("suno-%d", channelID), Key: "key", BaseURL: &baseURL, Status: common.ChannelStatusEnabled}
		require.NoError(t, db.Create(&channel).Error)
		task := model.Task{TaskID: fmt.Sprintf("suno-public-%d", channelID), Platform: constant.TaskPlatformSuno, UserId: 1, ChannelId: channelID, Status: model.TaskStatusInProgress, Progress: "30%", SubmitTime: time.Now().Unix(), PrivateData: model.TaskPrivateData{UpstreamTaskID: upstreamID}}
		require.NoError(t, db.Create(&task).Error)
	}
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })
	adaptor := &sunoEchoPendingAdaptor{}
	previousFactory := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { GetTaskAdaptorFunc = previousFactory })
	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 0
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
	previousLimit := constant.TaskQueryLimit
	constant.TaskQueryLimit = 100
	t.Cleanup(func() { constant.TaskQueryLimit = previousLimit })

	summary := RunTaskPollingOnce(context.Background(), nil)
	assert.Equal(t, 2, summary.UnfinishedTasks)
	assert.Equal(t, 2, adaptor.calls)
	var tasks []model.Task
	require.NoError(t, db.Where("platform = ?", constant.TaskPlatformSuno).Order("channel_id").Find(&tasks).Error)
	require.Len(t, tasks, 2)
	assert.Equal(t, 301, tasks[0].ChannelId)
	assert.Equal(t, 302, tasks[1].ChannelId)
	assert.EqualValues(t, model.TaskStatusInProgress, tasks[0].Status)
	assert.EqualValues(t, model.TaskStatusInProgress, tasks[1].Status)
}

func TestLegacyVideoSafetyBoundaryPreventsUnverifiedTerminal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		body       string
		info       *relaycommon.TaskInfo
	}{
		{"http_500", http.StatusInternalServerError, `{"status":"FAILURE"}`, &relaycommon.TaskInfo{TaskID: "legacy-upstream", Status: string(model.TaskStatusFailure), Reason: "provider.failed"}},
		{"error_envelope", http.StatusOK, `{"id":"request-id","error":{"message":"generic failure"}}`, &relaycommon.TaskInfo{TaskID: "legacy-upstream", Status: string(model.TaskStatusFailure), Reason: "provider.failed"}},
		{"id_mismatch", http.StatusOK, `{"status":"FAILURE"}`, &relaycommon.TaskInfo{TaskID: "different-upstream", Status: string(model.TaskStatusFailure), Reason: "provider.failed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBridgeTestDB(t)
			task := model.Task{TaskID: "legacy-public", Platform: constant.TaskPlatform("kling"), UserId: 1, ChannelId: 101, Quota: 100, Status: model.TaskStatusInProgress, Progress: "50%", PrivateData: model.TaskPrivateData{UpstreamTaskID: "legacy-upstream"}}
			require.NoError(t, db.Create(&task).Error)
			var channel model.Channel
			require.NoError(t, db.First(&channel, 101).Error)
			adaptor := &pollingBoundaryAdaptor{statusCode: tc.statusCode, body: []byte(tc.body), info: tc.info}
			require.Error(t, updateVideoSingleTask(context.Background(), adaptor, &channel, "legacy-upstream", map[string]*model.Task{"legacy-upstream": &task}))
			var reloaded model.Task
			require.NoError(t, db.First(&reloaded, task.ID).Error)
			assert.EqualValues(t, model.TaskStatusInProgress, reloaded.Status)
			assert.Equal(t, 100, reloaded.Quota)
			assert.Empty(t, reloaded.PollingDisposition)
		})
	}
}

func TestLoadTaskPollingContextsSchemaProbeFailClosed(t *testing.T) {
	t.Run("partial_schema", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskSubmissionOperation{}))
		_, err = LoadTaskPollingContexts(db, []*model.Task{{TaskID: "task_partial"}})
		require.ErrorContains(t, err, "schema is incomplete")
	})
	t.Run("probe_error", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate(&model.Task{}))
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
		_, err = LoadTaskPollingContexts(db, []*model.Task{{TaskID: "task_probe_error"}})
		require.ErrorContains(t, err, "inspect task polling schema")
	})
}

func TestCompatibleVideoProjectionPreservesTimesDataAndPrivateSignedURL(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "compatible-projection", 1000, 1000, 100)
	prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
	startTime := time.Now().Add(-2 * time.Minute).Unix()
	finishTime := startTime + 60
	signedURL := "https://media.example.test/compatible.mp4?signature=private&expires=99"
	providerData, err := common.Marshal(map[string]interface{}{"state": "complete", "prompt": "private prompt", "asset_id": "asset-1"})
	require.NoError(t, err)
	compatibleBody, err := common.Marshal(taskdto.TaskResponse[model.Task]{
		Code: taskdto.TaskSuccessCode,
		Data: model.Task{TaskID: fixture.Attempt.ProviderOperationID, Status: model.TaskStatusSuccess, Progress: "100%", StartTime: startTime, FinishTime: finishTime, FailReason: signedURL, Data: providerData},
	})
	require.NoError(t, err)
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	adaptor := &pollingBoundaryAdaptor{body: compatibleBody, info: &relaycommon.TaskInfo{}, adjust: 100}
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
	var task model.Task
	var observation model.TaskTerminalObservation
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	require.NoError(t, db.Where("operation_id = ?", fixture.Operation.ID).First(&observation).Error)
	assert.Equal(t, startTime, task.StartTime)
	assert.Equal(t, finishTime, task.FinishTime)
	assert.Contains(t, string(task.Data), `"asset_id":"asset-1"`)
	assert.NotContains(t, string(task.Data), "private prompt")
	assert.NotContains(t, string(task.Data), `"code"`)
	assert.Equal(t, signedURL, task.PrivateData.ResultURL)
	assert.Equal(t, "https://media.example.test/compatible.mp4", observation.ResultURL)
	assert.Equal(t, signedURL, observation.OperationalResultURL)
}

func TestCompatibleVideoProjectionRejectsInvalidTimes(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "compatible-invalid-times", 1000, 1000, 100)
	prepareDurablePollingFixture(t, db, &fixture, constant.TaskPlatform("kling"))
	body, err := common.Marshal(taskdto.TaskResponse[model.Task]{Code: taskdto.TaskSuccessCode, Data: model.Task{TaskID: fixture.Attempt.ProviderOperationID, Status: model.TaskStatusSuccess, StartTime: 20, FinishTime: 10}})
	require.NoError(t, err)
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	adaptor := &pollingBoundaryAdaptor{body: body}
	require.Error(t, updateVideoSingleTask(context.Background(), adaptor, &channel, fixture.Attempt.ProviderOperationID, map[string]*model.Task{fixture.Attempt.ProviderOperationID: &fixture.Task}))
	var task model.Task
	require.NoError(t, db.First(&task, fixture.Task.ID).Error)
	assert.EqualValues(t, model.TaskStatusInProgress, task.Status)
	assert.Equal(t, model.TaskPollingDispositionRetryable, task.PollingDisposition)
	assert.Equal(t, "video_task_schema_invalid", task.PollingReasonCode)
}

func TestLegacyVideoExplicitProviderTerminalStillApplies(t *testing.T) {
	db := setupBridgeTestDB(t)
	task := model.Task{TaskID: "legacy-terminal-public", Platform: constant.TaskPlatform("kling"), UserId: 1, ChannelId: 101, Status: model.TaskStatusInProgress, Progress: "50%", PrivateData: model.TaskPrivateData{UpstreamTaskID: "legacy-terminal-upstream"}}
	require.NoError(t, db.Create(&task).Error)
	var channel model.Channel
	require.NoError(t, db.First(&channel, 101).Error)
	adaptor := &pollingBoundaryAdaptor{body: []byte(`{"status":"FAILURE"}`), info: &relaycommon.TaskInfo{TaskID: "legacy-terminal-upstream", Status: string(model.TaskStatusFailure), Reason: "provider.failed"}}
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, &channel, "legacy-terminal-upstream", map[string]*model.Task{"legacy-terminal-upstream": &task}))
	var reloaded model.Task
	require.NoError(t, db.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	assert.Equal(t, "100%", reloaded.Progress)
}

func TestLegacyKlingTerminalWithoutResponseTaskIDFailsClosed(t *testing.T) {
	db := setupBridgeTestDB(t)
	fixture := newBridgeTestFixture(t, db, "legacy-kling-missing-response-id", 1000, 1000, 100)
	legacyTask := model.Task{
		TaskID: "legacy-kling-public", Platform: constant.TaskPlatform("kling"), UserId: fixture.User.Id,
		ChannelId: fixture.Attempt.ChannelID, Quota: 50, Status: model.TaskStatusInProgress, Progress: "50%",
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "legacy-kling-upstream", TokenId: fixture.Token.Id, BillingSource: "wallet"},
	}
	require.NoError(t, db.Create(&legacyTask).Error)
	var channel model.Channel
	require.NoError(t, db.First(&channel, fixture.Attempt.ChannelID).Error)
	channel.Type = constant.ChannelTypeKling
	adaptor := &pollingBoundaryAdaptor{
		body: []byte(`{"status":"FAILURE"}`),
		info: &relaycommon.TaskInfo{Status: string(model.TaskStatusFailure), Reason: "provider.failed"},
	}
	err := updateVideoSingleTask(context.Background(), adaptor, &channel, legacyTask.PrivateData.UpstreamTaskID, map[string]*model.Task{legacyTask.PrivateData.UpstreamTaskID: &legacyTask})
	require.ErrorContains(t, err, "omitted task id")

	var reloadedTask model.Task
	var reloadedOperation model.TaskSubmissionOperation
	var reloadedUser model.User
	require.NoError(t, db.First(&reloadedTask, legacyTask.ID).Error)
	require.NoError(t, db.First(&reloadedOperation, fixture.Operation.ID).Error)
	require.NoError(t, db.First(&reloadedUser, fixture.User.Id).Error)
	assert.EqualValues(t, model.TaskStatusInProgress, reloadedTask.Status)
	assert.Equal(t, "50%", reloadedTask.Progress)
	assert.Equal(t, 50, reloadedTask.Quota)
	assert.Equal(t, model.TaskSubmissionOperationStatusAccepted, reloadedOperation.Status)
	assert.Equal(t, 900, reloadedUser.Quota)
	assert.Equal(t, 100, reloadedUser.UsedQuota)
	var observationCount int64
	require.NoError(t, db.Model(&model.TaskTerminalObservation{}).Where("operation_id = ?", fixture.Operation.ID).Count(&observationCount).Error)
	assert.Zero(t, observationCount)
}
