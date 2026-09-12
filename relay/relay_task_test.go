package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	taskdto "github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel"
	"github.com/ForceMind/MyAPI/relay/channel/task/taskcommon"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type trackedLegacyTaskSubmitBody struct {
	reader    io.Reader
	bytesRead int
	closed    bool
}

func (body *trackedLegacyTaskSubmitBody) Read(data []byte) (int, error) {
	n, err := body.reader.Read(data)
	body.bytesRead += n
	return n, err
}

func (body *trackedLegacyTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

func TestLegacyTaskSubmitHTTPStatusErrorBoundsClosesAndRedactsBody(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	body := &trackedLegacyTaskSubmitBody{reader: strings.NewReader(
		sensitiveValue + strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1024),
	)}

	taskErr := legacyTaskSubmitHTTPStatusError(&http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       body,
	})

	require.True(t, body.closed)
	require.Equal(t, channel.MaxTaskSubmitResponseBytes+1, body.bytesRead)
	require.Equal(t, "fail_to_fetch_task", taskErr.Code)
	require.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	require.Equal(t, "task submission upstream returned an unexpected HTTP status", taskErr.Message)
	require.NotContains(t, taskErr.Message, sensitiveValue)
}

func TestLegacyTaskSubmitHTTPStatusErrorNormalizesInvalidStatus(t *testing.T) {
	taskErr := legacyTaskSubmitHTTPStatusError(&http.Response{StatusCode: 0})

	require.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	require.Equal(t, "fail_to_fetch_task", taskErr.Code)
}

type legacyTaskSubmitResponseProbe struct {
	doResponseCalled bool
}

var _ channel.TaskSubmitResponseParser = (*legacyTaskSubmitResponseProbe)(nil)

func (probe *legacyTaskSubmitResponseProbe) DoResponse(_ *gin.Context, _ *http.Response, _ *relaycommon.RelayInfo) (string, []byte, *taskdto.TaskError) {
	probe.doResponseCalled = true
	return "unexpected", []byte(`{}`), nil
}

func (*legacyTaskSubmitResponseProbe) ParseTaskSubmitResponse(channel.TaskSubmitParseInput) channel.TaskSubmitParseResult {
	return channel.TaskSubmitParseResult{}
}

func TestResolveLegacyTaskSubmitResponseSkipsMigratedParserOnNon200(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	body := &trackedLegacyTaskSubmitBody{reader: strings.NewReader(
		sensitiveValue + strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1024),
	)}
	probe := &legacyTaskSubmitResponseProbe{}

	taskID, taskData, taskErr := resolveLegacyTaskSubmitResponse(nil, probe, &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       body,
	}, nil)

	require.False(t, probe.doResponseCalled)
	require.Empty(t, taskID)
	require.Empty(t, taskData)
	require.True(t, body.closed)
	require.Equal(t, channel.MaxTaskSubmitResponseBytes+1, body.bytesRead)
	require.NotNil(t, taskErr)
	require.Equal(t, "fail_to_fetch_task", taskErr.Code)
	require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	require.False(t, taskErr.LocalError)
	require.Equal(t, "task submission upstream returned an unexpected HTTP status", taskErr.Message)
	require.NotContains(t, taskErr.Message, sensitiveValue)
}

type realtimePollingAdaptor struct {
	body    []byte
	info    *relaycommon.TaskInfo
	keys    []string
	onParse func()
}

func (a *realtimePollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *realtimePollingAdaptor) FetchTask(_ string, key string, _ map[string]any, _ string) (*http.Response, error) {
	a.keys = append(a.keys, key)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(a.body))}, nil
}
func (a *realtimePollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	if a.onParse != nil {
		a.onParse()
	}
	copyInfo := *a.info
	return &copyInfo, nil
}
func (*realtimePollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func setupRealtimeDurableTask(t *testing.T, channelType int, outcome model.TaskStatus) (*gorm.DB, model.TaskSubmissionOperation, model.Task) {
	t.Helper()
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("2b", 32))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.UserSubscription{}, &model.Task{}, &model.TaskRecoveryIdentity{}, &model.TaskSubmissionOperation{}, &model.TaskSubmissionAttempt{}, &model.TaskTerminalObservation{}, &model.TaskBillingEvent{}, &model.TaskBillingLogOutbox{}, &model.QuotaMutationReceipt{}, &model.Log{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLogDB })
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	user := model.User{Username: fmt.Sprintf("rt-user-%d-%s", channelType, outcome), AffCode: fmt.Sprintf("rt-aff-%d-%s", channelType, outcome), Password: "x", Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: fmt.Sprintf("rt-token-%d-%s", channelType, outcome), Status: common.TokenStatusEnabled, RemainQuota: 1000, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	channel := model.Channel{Id: 200 + channelType, Type: channelType, Name: "realtime", Key: "test-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	keyHash, err := model.HashTaskSubmissionIdempotencyKey(fmt.Sprintf("rt-key-%d-%s", channelType, outcome))
	require.NoError(t, err)
	fingerprint := model.FingerprintTaskSubmissionRequest([]byte(`{"prompt":"realtime"}`))
	intent, err := model.CreateOrLoadTaskSubmissionIntent(db, &model.TaskSubmissionOperation{UserID: user.Id, TokenID: token.Id, HTTPMethod: "POST", OperationKind: model.TaskSubmissionOperationKindVideoCreate, IdempotencyKeyHash: keyHash, RequestFingerprint: fingerprint}, &model.TaskSubmissionAttempt{AttemptNo: 1, ChannelID: channel.Id, Provider: "realtime", RequestClass: "video"})
	require.NoError(t, err)
	billingContext := model.TaskBillingContext{Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1, ModelRatio: 1, GroupRatio: 1, OriginModelName: "veo", PerCallBilling: true}
	receipt, err := model.ReserveTaskQuota(db, model.TaskQuotaReservationInput{OperationID: intent.Operation.ID, UserID: user.Id, TokenID: token.Id, ChannelID: channel.Id, ExpectedOperationVersion: intent.Operation.LockVersion, Quota: 100, EstimatedQuota: 100, ApplyStatistics: true, BillingSource: "wallet", BillingContext: billingContext})
	require.NoError(t, err)
	won, err := model.StartTaskSubmissionDispatch(db, intent.Operation.ID, model.TaskSubmissionDispatchTransition{ExpectedOperationVersion: receipt.OperationVersionAfter, ExpectedAttemptVersion: intent.Attempt.LockVersion})
	require.NoError(t, err)
	require.True(t, won)
	var operation model.TaskSubmissionOperation
	var attempt model.TaskSubmissionAttempt
	require.NoError(t, db.First(&operation, intent.Operation.ID).Error)
	require.NoError(t, db.First(&attempt, intent.Attempt.ID).Error)
	upstreamID := taskcommon.EncodeLocalTaskID(fmt.Sprintf("projects/test/locations/us/models/veo/operations/%d-%s", channelType, strings.ToLower(string(outcome))))
	task := model.Task{TaskID: operation.PublicID, Platform: constant.TaskPlatform(strconv.Itoa(channelType)), UserId: user.Id, Group: "default", ChannelId: channel.Id, Quota: 100, Action: constant.TaskActionGenerate, Status: model.TaskStatusInProgress, Progress: "50%", SubmitTime: time.Now().Unix(), PrivateData: model.TaskPrivateData{UpstreamTaskID: upstreamID, TokenId: token.Id, BillingSource: "wallet", BillingContext: &billingContext}}
	require.NoError(t, db.Create(&task).Error)
	won, err = model.TransitionTaskSubmissionAttempt(db, attempt.ID, model.TaskSubmissionAttemptTransition{From: model.TaskSubmissionAttemptStatusDispatching, To: model.TaskSubmissionAttemptStatusAccepted, ExpectedVersion: attempt.LockVersion, ProviderOperationID: upstreamID, TaskPlatform: string(task.Platform), TaskAction: task.Action})
	require.NoError(t, err)
	require.True(t, won)
	won, err = model.TransitionTaskSubmissionOperation(db, operation.ID, model.TaskSubmissionOperationTransition{From: model.TaskSubmissionOperationStatusDispatching, To: model.TaskSubmissionOperationStatusAccepted, ExpectedVersion: operation.LockVersion, TaskID: &task.ID})
	require.NoError(t, err)
	require.True(t, won)
	require.NoError(t, db.First(&operation, operation.ID).Error)
	return db, operation, task
}

func TestVideoFetchPublicEntryGeminiVertexDurableTerminal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, channelType := range []int{constant.ChannelTypeGemini, constant.ChannelTypeVertexAi} {
		for _, outcome := range []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure} {
			t.Run(fmt.Sprintf("%d_%s", channelType, outcome), func(t *testing.T) {
				db, operation, task := setupRealtimeDurableTask(t, channelType, outcome)
				responseID := task.PrivateData.UpstreamTaskID
				if channelType == constant.ChannelTypeVertexAi || outcome == model.TaskStatusFailure {
					responseID = ""
				}
				body := []byte(`{"done":true}`)
				if outcome == model.TaskStatusFailure {
					operationName, decodeErr := taskcommon.DecodeLocalTaskID(task.PrivateData.UpstreamTaskID)
					require.NoError(t, decodeErr)
					body = []byte(fmt.Sprintf(`{"name":%q,"done":true,"error":{"code":500,"message":"provider failed"}}`, operationName))
				}
				adaptor := &realtimePollingAdaptor{body: body, info: &relaycommon.TaskInfo{TaskID: responseID, Status: string(outcome), Reason: "provider.failed", Url: "https://media.example.test/video.mp4?sig=private"}}
				oldFactory := getRealtimeTaskAdaptor
				getRealtimeTaskAdaptor = func(constant.TaskPlatform) service.TaskPollingAdaptor { return adaptor }
				t.Cleanup(func() { getRealtimeTaskAdaptor = oldFactory })
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodGet, "/api/video/"+task.TaskID, nil)
				ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
				ctx.Set("id", task.UserId)
				require.Nil(t, RelayTaskFetch(ctx, relayconstant.RelayModeVideoFetchByID))
				assert.Equal(t, http.StatusOK, recorder.Code)
				var reloadedTask model.Task
				var reloadedOperation model.TaskSubmissionOperation
				require.NoError(t, db.First(&reloadedTask, task.ID).Error)
				require.NoError(t, db.First(&reloadedOperation, operation.ID).Error)
				if outcome == model.TaskStatusSuccess {
					assert.EqualValues(t, model.TaskStatusSuccess, reloadedTask.Status)
					assert.Equal(t, model.TaskSubmissionOperationStatusSucceeded, reloadedOperation.Status)
					assert.Equal(t, 100, reloadedTask.Quota)
				} else {
					assert.EqualValues(t, model.TaskStatusFailure, reloadedTask.Status)
					assert.Equal(t, model.TaskSubmissionOperationStatusFailed, reloadedOperation.Status)
					assert.Zero(t, reloadedTask.Quota)
				}
				var observation model.TaskTerminalObservation
				require.NoError(t, db.Where("operation_id = ?", operation.ID).First(&observation).Error)
				assert.Equal(t, model.TaskTerminalObservationApplied, observation.State)
				assert.Equal(t, task.PrivateData.UpstreamTaskID, observation.EvidenceID)
				assert.Equal(t, model.TaskSubmissionResolutionSourceProviderVerified, observation.ResolutionSource)
			})
		}
	}
}

func setupRealtimeLegacyTask(t *testing.T, privateKey string) (*gorm.DB, model.Channel, model.Task, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = oldDB })
	channel := model.Channel{Id: 501, Type: constant.ChannelTypeGemini, Name: "rotated", Key: "rotated-channel-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	operationName := "models/veo/operations/key-test"
	upstreamID := taskcommon.EncodeLocalTaskID(operationName)
	task := model.Task{TaskID: "legacy-realtime-public", Platform: constant.TaskPlatform(strconv.Itoa(channel.Type)), UserId: 9, ChannelId: channel.Id, Status: model.TaskStatusInProgress, Progress: "50%", Action: constant.TaskActionGenerate, PrivateData: model.TaskPrivateData{UpstreamTaskID: upstreamID, Key: privateKey}}
	require.NoError(t, db.Create(&task).Error)
	return db, channel, task, operationName
}

func fetchRealtimeTaskForTest(t *testing.T, task model.Task) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/video/"+task.TaskID, nil)
	ctx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	ctx.Set("id", task.UserId)
	require.Nil(t, RelayTaskFetch(ctx, relayconstant.RelayModeVideoFetchByID))
	return recorder
}

func TestRealtimeFetchPrefersTaskPrivateKeyBeforeRotatedChannelKey(t *testing.T) {
	db, _, task, operationName := setupRealtimeLegacyTask(t, "submission-key")
	adaptor := &realtimePollingAdaptor{body: []byte(fmt.Sprintf(`{"name":%q,"done":false}`, operationName)), info: &relaycommon.TaskInfo{TaskID: task.PrivateData.UpstreamTaskID, Status: string(model.TaskStatusInProgress), Progress: "60%"}}
	oldFactory := getRealtimeTaskAdaptor
	getRealtimeTaskAdaptor = func(constant.TaskPlatform) service.TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { getRealtimeTaskAdaptor = oldFactory })

	fetchRealtimeTaskForTest(t, task)
	require.Equal(t, []string{"submission-key"}, adaptor.keys)
	task.PrivateData.Key = ""
	require.NoError(t, db.Model(&model.Task{}).Where("id = ?", task.ID).Update("private_data", task.PrivateData).Error)
	fetchRealtimeTaskForTest(t, task)
	require.Equal(t, []string{"submission-key", "rotated-channel-key"}, adaptor.keys)
}

func TestRealtimePendingCASLossReturnsConcurrentTerminalProjection(t *testing.T) {
	db, _, task, operationName := setupRealtimeLegacyTask(t, "submission-key")
	adaptor := &realtimePollingAdaptor{body: []byte(fmt.Sprintf(`{"name":%q,"done":false}`, operationName)), info: &relaycommon.TaskInfo{TaskID: task.PrivateData.UpstreamTaskID, Status: string(model.TaskStatusInProgress), Progress: "60%"}}
	adaptor.onParse = func() {
		require.NoError(t, db.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]interface{}{"status": model.TaskStatusSuccess, "progress": "100%", "finish_time": time.Now().Unix()}).Error)
		adaptor.onParse = nil
	}
	oldFactory := getRealtimeTaskAdaptor
	getRealtimeTaskAdaptor = func(constant.TaskPlatform) service.TaskPollingAdaptor { return adaptor }
	t.Cleanup(func() { getRealtimeTaskAdaptor = oldFactory })

	recorder := fetchRealtimeTaskForTest(t, task)
	assert.Contains(t, recorder.Body.String(), `"status":"succeeded"`)
	var reloaded model.Task
	require.NoError(t, db.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
}
