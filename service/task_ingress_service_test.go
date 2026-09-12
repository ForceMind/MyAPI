package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relaykitdto "github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTaskIngressTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("TASK_RECOVERY_IDEMPOTENCY_SECRET", strings.Repeat("1a", 32))
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	err = db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.Task{},
		&model.TaskRecoveryIdentity{},
		&model.TaskSubmissionOperation{},
		&model.TaskSubmissionAttempt{},
		&model.TaskBillingEvent{},
		&model.TaskBillingLogOutbox{},
		&model.QuotaMutationReceipt{},
		&model.Log{},
	)
	require.NoError(t, err)
	return db
}

func createIngressUserAndToken(t *testing.T, db *gorm.DB, initialQuota int) (model.User, model.Token) {
	t.Helper()
	user := model.User{
		Username: fmt.Sprintf("user_%d", common.GetRandomInt(1000000)),
		Password: "test-fixture-password",
		Status:   common.UserStatusEnabled,
		Quota:    initialQuota,
	}
	require.NoError(t, db.Create(&user).Error)

	token := model.Token{
		UserId:      user.Id,
		Key:         fmt.Sprintf("sk-test-%d", common.GetRandomInt(1000000)),
		Status:      common.TokenStatusEnabled,
		RemainQuota: initialQuota,
		ExpiredTime: -1,
	}
	require.NoError(t, db.Create(&token).Error)
	return user, token
}

func ingressBillingContext(quota int) model.TaskBillingContext {
	return model.TaskBillingContext{
		Version: model.TaskBillingContextVersion, Complete: true,
		OriginModelName: model.TaskSubmissionOperationKindVideoCreate,
		ModelPrice:      float64(quota), ModelRatio: 1, GroupRatio: 1, PerCallBilling: true,
	}
}

func TestDetectTaskOperationKind(t *testing.T) {
	testCases := []struct {
		name         string
		method       string
		path         string
		params       map[string]string
		wantKind     string
		wantOriginID string
		wantOk       bool
	}{
		{
			name:         "remix with video_id param",
			method:       http.MethodPost,
			path:         "/v1/videos/task_orig_123/remix",
			params:       map[string]string{"video_id": "task_orig_123"},
			wantKind:     model.TaskSubmissionOperationKindVideoRemix,
			wantOriginID: "task_orig_123",
			wantOk:       true,
		},
		{
			name:         "remix with id param",
			method:       http.MethodPost,
			path:         "/v1/videos/task_orig_456/remix",
			params:       map[string]string{"id": "task_orig_456"},
			wantKind:     model.TaskSubmissionOperationKindVideoRemix,
			wantOriginID: "task_orig_456",
			wantOk:       true,
		},
		{
			name:         "remix without params extracts from path",
			method:       http.MethodPost,
			path:         "/videos/task_orig_789/remix",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoRemix,
			wantOriginID: "task_orig_789",
			wantOk:       true,
		},
		{
			name:         "video generations with v1 prefix",
			method:       http.MethodPost,
			path:         "/v1/video/generations",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoCreate,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "video generations bare path",
			method:       http.MethodPost,
			path:         "/video/generations",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoCreate,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "videos create with v1 prefix",
			method:       http.MethodPost,
			path:         "/v1/videos",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoCreate,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "videos create bare path",
			method:       http.MethodPost,
			path:         "/videos",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoCreate,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "kling text2video",
			method:       http.MethodPost,
			path:         "/kling/v1/videos/text2video",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoCreate,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "kling image2video",
			method:       http.MethodPost,
			path:         "/kling/v1/videos/image2video",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoCreate,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "jimeng post",
			method:       http.MethodPost,
			path:         "/jimeng/",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindVideoCreate,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "suno submit music",
			method:       http.MethodPost,
			path:         "/suno/submit/music",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindSunoMusic,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "suno submit lyrics",
			method:       http.MethodPost,
			path:         "/suno/submit/lyrics",
			params:       nil,
			wantKind:     model.TaskSubmissionOperationKindSunoLyrics,
			wantOriginID: "",
			wantOk:       true,
		},
		{
			name:         "get method rejected",
			method:       http.MethodGet,
			path:         "/v1/videos",
			params:       nil,
			wantKind:     "",
			wantOriginID: "",
			wantOk:       false,
		},
		{
			name:         "unrelated route rejected",
			method:       http.MethodPost,
			path:         "/v1/chat/completions",
			params:       nil,
			wantKind:     "",
			wantOriginID: "",
			wantOk:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			kind, originID, ok := DetectTaskOperationKind(tc.method, tc.path, tc.params)
			assert.Equal(t, tc.wantOk, ok)
			assert.Equal(t, tc.wantKind, kind)
			assert.Equal(t, tc.wantOriginID, originID)
		})
	}
}

func TestExecuteTaskIngress_NewSubmissionAccepted(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 10000)

	var dispatchCalled int32
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		atomic.AddInt32(&dispatchCalled, 1)
		return &TaskProviderDispatchResult{
			Status:         TaskProviderDispatchStatusAccepted,
			ProviderTaskID: "upstream-video-task-42",
			TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "upstream-video-task-42", 2000, ingressBillingContext(2000)),
		}, nil
	}

	headers := http.Header{
		"Idempotency-Key": []string{"unique-idemp-key-1"},
		"Content-Type":    []string{"application/json"},
	}

	req := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		Header:         headers,
		ContentType:    "application/json",
		Body:           []byte(`{"prompt":"a peaceful lakeside","model":"sora-2"}`),
		EstimatedQuota: 2000,
		Dispatcher:     dispatcher,
		DB:             db,
	}

	result, err := ExecuteTaskIngress(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsReplay)
	assert.False(t, result.Conflict)
	assert.Empty(t, result.ErrorCode)
	assert.Empty(t, result.ErrorMsg)
	require.NotNil(t, result.Response)
	assert.NotEmpty(t, result.Response.ID)
	assert.Equal(t, dto.TaskOperationObject, result.Response.Object)
	assert.Equal(t, model.TaskSubmissionOperationKindVideoCreate, result.Response.Kind)
	assert.Equal(t, string(model.TaskSubmissionOperationStatusAccepted), result.Response.Status)
	assert.Equal(t, int32(1), atomic.LoadInt32(&dispatchCalled))

	// Ensure header was stripped
	assert.Empty(t, headers.Get("Idempotency-Key"))

	// Quota was deducted from user & token
	var updatedUser model.User
	require.NoError(t, db.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 8000, updatedUser.Quota)

	var updatedToken model.Token
	require.NoError(t, db.First(&updatedToken, token.Id).Error)
	assert.Equal(t, 8000, updatedToken.RemainQuota)
}

func TestExecuteTaskIngress_IdempotentReplay(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 10000)

	var dispatchCalled int32
	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		atomic.AddInt32(&dispatchCalled, 1)
		return &TaskProviderDispatchResult{
			Status:         TaskProviderDispatchStatusAccepted,
			ProviderTaskID: "upstream-replay-id-1",
			TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "upstream-replay-id-1", 2500, ingressBillingContext(2500)),
		}, nil
	}

	body := []byte(`{"prompt":"replay test scene","model":"sora-2"}`)
	firstHeaders := http.Header{
		"Idempotency-Key": []string{"replay-idemp-key"},
	}

	req1 := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		Header:         firstHeaders,
		ContentType:    "application/json",
		Body:           body,
		EstimatedQuota: 2500,
		Dispatcher:     dispatcher,
		DB:             db,
	}

	res1, err := ExecuteTaskIngress(context.Background(), req1)
	require.NoError(t, err)
	require.NotNil(t, res1)
	assert.False(t, res1.IsReplay)
	assert.False(t, res1.Conflict)
	assert.Equal(t, int32(1), atomic.LoadInt32(&dispatchCalled))

	// Replay second request: exact same key and body
	secondHeaders := http.Header{
		"Idempotency-Key": []string{"replay-idemp-key"},
	}

	req2 := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		Header:         secondHeaders,
		ContentType:    "application/json",
		Body:           body,
		EstimatedQuota: 2500,
		Dispatcher:     dispatcher,
		DB:             db,
	}

	res2, err := ExecuteTaskIngress(context.Background(), req2)
	require.NoError(t, err)
	require.NotNil(t, res2)
	assert.True(t, res2.IsReplay)
	assert.False(t, res2.Conflict)
	require.NotNil(t, res2.Response)
	assert.Equal(t, res1.Response.ID, res2.Response.ID)
	assert.Equal(t, res1.Response.Status, res2.Response.Status)

	// Dispatcher was NOT invoked a second time
	assert.Equal(t, int32(1), atomic.LoadInt32(&dispatchCalled))

	// Quota was NOT deducted again
	var updatedUser model.User
	require.NoError(t, db.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 7500, updatedUser.Quota)

	var updatedToken model.Token
	require.NoError(t, db.First(&updatedToken, token.Id).Error)
	assert.Equal(t, 7500, updatedToken.RemainQuota)
}

func TestExecuteTaskIngress_FreezesBillingPreferenceSelection(t *testing.T) {
	testCases := []struct {
		name              string
		preference        string
		walletQuota       int
		subscriptionQuota int64
		wantSource        string
	}{
		{name: "subscription_only_uses_subscription_when_wallet_empty", preference: "subscription_only", walletQuota: 0, subscriptionQuota: 1000, wantSource: "subscription"},
		{name: "subscription_first_prefers_subscription", preference: "subscription_first", walletQuota: 1000, subscriptionQuota: 1000, wantSource: "subscription"},
		{name: "wallet_only_uses_wallet", preference: "wallet_only", walletQuota: 1000, subscriptionQuota: 1000, wantSource: "wallet"},
		{name: "wallet_first_prefers_wallet", preference: "wallet_first", walletQuota: 1000, subscriptionQuota: 1000, wantSource: "wallet"},
		{name: "wallet_first_falls_back_when_wallet_insufficient", preference: "wallet_first", walletQuota: 50, subscriptionQuota: 1000, wantSource: "subscription"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTaskIngressTestDB(t)
			user, token := createIngressUserAndToken(t, db, tc.walletQuota)
			require.NoError(t, db.Model(&token).Updates(map[string]interface{}{"remain_quota": 1000}).Error)
			user.SetSetting(relaykitdto.UserSetting{BillingPreference: tc.preference})
			require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)
			plan := model.SubscriptionPlan{Title: "test", Enabled: true, TotalAmount: tc.subscriptionQuota, DurationUnit: "month", DurationValue: 1}
			require.NoError(t, db.Create(&plan).Error)

			subscription := model.UserSubscription{
				UserId: user.Id, PlanId: plan.Id, AmountTotal: tc.subscriptionQuota, AmountUsed: 0,
				Status: "active", StartTime: 1, EndTime: 1<<31 - 1, AllowWalletOverflow: true,
			}
			require.NoError(t, db.Create(&subscription).Error)
			billingContext := model.TaskBillingContext{
				Version: model.TaskBillingContextVersion, Complete: true, ModelPrice: 1,
				ModelRatio: 1, GroupRatio: 1, OriginModelName: "test-model", PerCallBilling: true,
			}
			dispatchCount := 0
			result, err := ExecuteTaskIngress(context.Background(), TaskIngressRequest{
				UserID: user.Id, TokenID: token.Id, ChannelID: 10, Provider: "test",
				OperationKind: model.TaskSubmissionOperationKindVideoCreate, HTTPMethod: http.MethodPost,
				Header:      http.Header{"Idempotency-Key": []string{"billing-" + tc.name}},
				ContentType: "application/json", Body: []byte(`{"prompt":"billing source"}`),
				EstimatedQuota: 100, BillingContext: billingContext, DB: db,
				Dispatcher: func(_ context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
					dispatchCount++
					return &TaskProviderDispatchResult{
						Status: TaskProviderDispatchStatusAccepted, ProviderTaskID: "billing-upstream-" + tc.name,
						TaskCandidate: acceptedTaskCandidateForTest(op, attempt, "billing-upstream-"+tc.name, 100, billingContext),
					}, nil
				},
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, 1, dispatchCount)

			operation, err := model.GetTaskSubmissionOperationByPublicID(db, result.Response.ID)
			require.NoError(t, err)
			require.NotNil(t, operation)
			assert.Equal(t, tc.preference, operation.BillingPreference)
			assert.Equal(t, tc.wantSource, operation.BillingSource)
			if tc.wantSource == "subscription" {
				assert.Equal(t, subscription.Id, operation.SubscriptionID)
			} else {
				assert.Zero(t, operation.SubscriptionID)
			}

			var receipt model.QuotaMutationReceipt
			require.NoError(t, db.Where("operation_id = ? AND mutation_type = ?", operation.ID, model.TaskBillingEventTypeReserve).First(&receipt).Error)
			assert.Equal(t, tc.preference, receipt.BillingPreference)
			assert.Equal(t, operation.BillingSource, receipt.BillingSource)
			assert.Equal(t, operation.SubscriptionID, receipt.SubscriptionID)

			var task model.Task
			require.NoError(t, db.First(&task, *operation.TaskID).Error)
			assert.Equal(t, operation.BillingPreference, task.PrivateData.BillingPreference)
			assert.Equal(t, operation.BillingSource, task.PrivateData.BillingSource)
			assert.Equal(t, operation.SubscriptionID, task.PrivateData.SubscriptionId)
			require.NotNil(t, task.PrivateData.BillingContext)
			assert.Equal(t, billingContext, *task.PrivateData.BillingContext)

			var userBeforeReplay model.User
			var tokenBeforeReplay model.Token
			var subscriptionBeforeReplay model.UserSubscription
			require.NoError(t, db.First(&userBeforeReplay, user.Id).Error)
			require.NoError(t, db.First(&tokenBeforeReplay, token.Id).Error)
			require.NoError(t, db.First(&subscriptionBeforeReplay, subscription.Id).Error)
			user.SetSetting(relaykitdto.UserSetting{BillingPreference: "wallet_only"})
			require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)

			replay, err := ExecuteTaskIngress(context.Background(), TaskIngressRequest{
				UserID: user.Id, TokenID: token.Id, ChannelID: 10, Provider: "test",
				OperationKind: model.TaskSubmissionOperationKindVideoCreate, HTTPMethod: http.MethodPost,
				Header:      http.Header{"Idempotency-Key": []string{"billing-" + tc.name}},
				ContentType: "application/json", Body: []byte(`{"prompt":"billing source"}`),
				EstimatedQuota: 100, BillingContext: billingContext, DB: db,
				Dispatcher: func(context.Context, *model.TaskSubmissionOperation, *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
					dispatchCount++
					return nil, errors.New("idempotent replay must not dispatch")
				},
			})
			require.NoError(t, err)
			require.True(t, replay.IsReplay)
			assert.Equal(t, 1, dispatchCount)

			var userAfterReplay model.User
			var tokenAfterReplay model.Token
			var subscriptionAfterReplay model.UserSubscription
			require.NoError(t, db.First(&userAfterReplay, user.Id).Error)
			require.NoError(t, db.First(&tokenAfterReplay, token.Id).Error)
			require.NoError(t, db.First(&subscriptionAfterReplay, subscription.Id).Error)
			assert.Equal(t, userBeforeReplay.Quota, userAfterReplay.Quota)
			assert.Equal(t, tokenBeforeReplay.RemainQuota, tokenAfterReplay.RemainQuota)
			assert.Equal(t, subscriptionBeforeReplay.AmountUsed, subscriptionAfterReplay.AmountUsed)
		})
	}
}

func TestExecuteTaskIngress_ConflictRejected(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 10000)

	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		return &TaskProviderDispatchResult{
			Status:         TaskProviderDispatchStatusAccepted,
			ProviderTaskID: "conflict-test-upstream",
			TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "conflict-test-upstream", 1000, ingressBillingContext(1000)),
		}, nil
	}

	// 1. Initial request with body A
	firstHeaders := http.Header{
		"Idempotency-Key": []string{"conflict-key-1"},
	}
	req1 := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		Header:         firstHeaders,
		ContentType:    "application/json",
		Body:           []byte(`{"prompt":"body version A"}`),
		EstimatedQuota: 1000,
		Dispatcher:     dispatcher,
		DB:             db,
	}
	res1, err := ExecuteTaskIngress(context.Background(), req1)
	require.NoError(t, err)
	require.NotNil(t, res1)
	assert.False(t, res1.Conflict)

	// 2. Second request with identical key but different body B
	secondHeaders := http.Header{
		"Idempotency-Key": []string{"conflict-key-1"},
	}
	req2 := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		Header:         secondHeaders,
		ContentType:    "application/json",
		Body:           []byte(`{"prompt":"body version B (different)"}`),
		EstimatedQuota: 1000,
		Dispatcher:     dispatcher,
		DB:             db,
	}
	res2, err := ExecuteTaskIngress(context.Background(), req2)
	require.NoError(t, err)
	require.NotNil(t, res2)
	assert.True(t, res2.Conflict)
	assert.Equal(t, "idempotency_conflict", res2.ErrorCode)
	assert.Contains(t, res2.ErrorMsg, "task submission idempotency key belongs to a different request")

	// Quota was only deducted for the first request
	var updatedUser model.User
	require.NoError(t, db.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 9000, updatedUser.Quota)
}

func TestExecuteTaskIngress_ContentTypes(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 30000)

	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		return &TaskProviderDispatchResult{
			Status:         TaskProviderDispatchStatusAccepted,
			ProviderTaskID: "ct-upstream-id",
			TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "ct-upstream-id", 1000, ingressBillingContext(1000)),
		}, nil
	}

	t.Run("JSON content type", func(t *testing.T) {
		req := TaskIngressRequest{
			UserID:         user.Id,
			TokenID:        token.Id,
			ChannelID:      10,
			Provider:       "openai",
			OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
			HTTPMethod:     http.MethodPost,
			Header:         http.Header{"Idempotency-Key": {"ct-json-key"}},
			ContentType:    "application/json; charset=utf-8",
			Body:           []byte(`{"prompt":"json test"}`),
			EstimatedQuota: 1000,
			Dispatcher:     dispatcher,
			DB:             db,
		}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.False(t, res.Conflict)
		assert.Equal(t, "accepted", res.Response.Status)
	})

	t.Run("Form urlencoded content type", func(t *testing.T) {
		req := TaskIngressRequest{
			UserID:         user.Id,
			TokenID:        token.Id,
			ChannelID:      10,
			Provider:       "openai",
			OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
			HTTPMethod:     http.MethodPost,
			Header:         http.Header{"Idempotency-Key": {"ct-form-key"}},
			ContentType:    "application/x-www-form-urlencoded",
			Body:           []byte("model=fixture&prompt=form+test"),
			EstimatedQuota: 1000,
			Dispatcher:     dispatcher,
			DB:             db,
		}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.False(t, res.Conflict)
		assert.Equal(t, "accepted", res.Response.Status)
	})

	t.Run("Multipart content type", func(t *testing.T) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		require.NoError(t, writer.SetBoundary("test-boundary-12345"))

		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="prompt"`)
		header.Set("Content-Type", "text/plain; charset=utf-8")
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write([]byte("multipart video test"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		req := TaskIngressRequest{
			UserID:         user.Id,
			TokenID:        token.Id,
			ChannelID:      10,
			Provider:       "openai",
			OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
			HTTPMethod:     http.MethodPost,
			Header:         http.Header{"Idempotency-Key": {"ct-multipart-key"}},
			ContentType:    "multipart/form-data; boundary=test-boundary-12345",
			Body:           buf.Bytes(),
			EstimatedQuota: 1000,
			Dispatcher:     dispatcher,
			DB:             db,
		}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, res)
		assert.False(t, res.Conflict)
		assert.Equal(t, "accepted", res.Response.Status)
	})

	t.Run("Unsupported content type", func(t *testing.T) {
		req := TaskIngressRequest{
			UserID:         user.Id,
			TokenID:        token.Id,
			ChannelID:      10,
			Provider:       "openai",
			OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
			HTTPMethod:     http.MethodPost,
			Header:         http.Header{"Idempotency-Key": {"ct-invalid-key"}},
			ContentType:    "text/plain",
			Body:           []byte("plain text"),
			EstimatedQuota: 1000,
			Dispatcher:     dispatcher,
			DB:             db,
		}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTaskSubmissionFingerprintContentType)
		assert.Nil(t, res)
	})
}

func TestExecuteTaskIngress_InvalidHeaders(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 10000)

	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		return &TaskProviderDispatchResult{Status: TaskProviderDispatchStatusAccepted}, nil
	}

	baseReq := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		ContentType:    "application/json",
		Body:           []byte(`{"prompt":"test"}`),
		EstimatedQuota: 1000,
		Dispatcher:     dispatcher,
		DB:             db,
	}

	t.Run("missing idempotency key", func(t *testing.T) {
		req := baseReq
		req.Header = http.Header{}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyMissing)
		assert.Nil(t, res)
	})

	t.Run("alias header rejected", func(t *testing.T) {
		req := baseReq
		req.Header = http.Header{
			"X-Idempotency-Key": []string{"alias-key"},
		}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyAlias)
		assert.Nil(t, res)
	})

	t.Run("multiple idempotency keys", func(t *testing.T) {
		req := baseReq
		req.Header = http.Header{
			"Idempotency-Key": []string{"key-1", "key-2"},
		}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyMultiple)
		assert.Nil(t, res)
	})

	t.Run("invalid character in key", func(t *testing.T) {
		req := baseReq
		req.Header = http.Header{
			"Idempotency-Key": []string{"invalid key with spaces"},
		}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTaskSubmissionProtocolIdempotencyInvalid)
		assert.Nil(t, res)
	})

	t.Run("invalid HTTP method", func(t *testing.T) {
		req := baseReq
		req.HTTPMethod = http.MethodGet
		req.Header = http.Header{"Idempotency-Key": []string{"valid-key"}}
		res, err := ExecuteTaskIngress(context.Background(), req)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrTaskSubmissionProtocolMethod)
		assert.Nil(t, res)
	})
}

func TestExecuteTaskIngress_DispatchRejected(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 10000)

	dispatcher := func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
		return &TaskProviderDispatchResult{
			Status:       TaskProviderDispatchStatusRejected,
			ErrorCode:    "model_rate_limited",
			ErrorMessage: "upstream rejected request",
		}, nil
	}

	req := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		Header:         http.Header{"Idempotency-Key": {"reject-test-key"}},
		ContentType:    "application/json",
		Body:           []byte(`{"prompt":"test"}`),
		EstimatedQuota: 2000,
		Dispatcher:     dispatcher,
		DB:             db,
	}

	res, err := ExecuteTaskIngress(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.IsReplay)
	assert.False(t, res.Conflict)
	assert.Equal(t, "model_rate_limited", res.ErrorCode)
	assert.Equal(t, "upstream rejected request", res.ErrorMsg)
	require.NotNil(t, res.Response)
	assert.Equal(t, string(model.TaskSubmissionOperationStatusRejected), res.Response.Status)

	// Quota was fully refunded upon rejection
	var updatedUser model.User
	require.NoError(t, db.First(&updatedUser, user.Id).Error)
	assert.Equal(t, 10000, updatedUser.Quota)

	var updatedToken model.Token
	require.NoError(t, db.First(&updatedToken, token.Id).Error)
	assert.Equal(t, 10000, updatedToken.RemainQuota)
}

func TestTaskIngressService_ExecuteWrapper(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 10000)

	service := NewTaskIngressService(db)
	req := TaskIngressRequest{
		UserID:         user.Id,
		TokenID:        token.Id,
		ChannelID:      10,
		Provider:       "openai",
		OperationKind:  model.TaskSubmissionOperationKindVideoCreate,
		HTTPMethod:     http.MethodPost,
		Header:         http.Header{"Idempotency-Key": {"wrapper-key"}},
		ContentType:    "application/json",
		Body:           []byte(`{"prompt":"test"}`),
		EstimatedQuota: 500,
		Dispatcher: func(ctx context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			return &TaskProviderDispatchResult{
				Status:         TaskProviderDispatchStatusAccepted,
				ProviderTaskID: "wrapper-task-123",
				TaskCandidate:  acceptedTaskCandidateForTest(op, attempt, "wrapper-task-123", 500, ingressBillingContext(500)),
			}, nil
		},
	}

	res, err := service.Execute(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "accepted", res.Response.Status)
}

func TestExecuteTaskIngress_InitialQuotaClampRejectsBeforeIntentAndDispatch(t *testing.T) {
	db := setupTaskIngressTestDB(t)
	user, token := createIngressUserAndToken(t, db, 1000)
	dispatchCount := 0
	clamp := &common.QuotaClamp{Op: "QuotaFromFloat", Kind: common.QuotaClampOverflow, Original: 1e30, Clamped: common.MaxQuota}

	result, err := ExecuteTaskIngress(context.Background(), TaskIngressRequest{
		UserID: user.Id, TokenID: token.Id, ChannelID: 10, Provider: "test",
		OperationKind: model.TaskSubmissionOperationKindVideoCreate, HTTPMethod: http.MethodPost,
		Header: http.Header{"Idempotency-Key": []string{"clamp-before-intent"}}, ContentType: "application/json",
		Body: []byte(`{"prompt":"oversized"}`), EstimatedQuota: common.MaxQuota, InitialQuotaClamp: clamp,
		Dispatcher: func(context.Context, *model.TaskSubmissionOperation, *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
			dispatchCount++
			return nil, nil
		}, DB: db,
	})
	require.ErrorIs(t, err, clamp)
	assert.Nil(t, result)
	assert.Zero(t, dispatchCount)

	var operationCount, receiptCount int64
	require.NoError(t, db.Model(&model.TaskSubmissionOperation{}).Count(&operationCount).Error)
	require.NoError(t, db.Model(&model.QuotaMutationReceipt{}).Count(&receiptCount).Error)
	assert.Zero(t, operationCount)
	assert.Zero(t, receiptCount)
}

func TestExecuteTaskIngress_SubscriptionZeroQuotaDistinguishesFreeModel(t *testing.T) {
	for _, tc := range []struct {
		name         string
		free         bool
		wantReserved int64
	}{
		{name: "nonfree_truncation_reserves_one", free: false, wantReserved: 1},
		{name: "true_free_reserves_zero", free: true, wantReserved: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupTaskIngressTestDB(t)
			user, token := createIngressUserAndToken(t, db, 0)
			require.NoError(t, db.Model(&token).Updates(map[string]interface{}{"remain_quota": 1}).Error)
			user.SetSetting(relaykitdto.UserSetting{BillingPreference: "subscription_only"})
			require.NoError(t, db.Model(&user).Update("setting", user.Setting).Error)
			plan := model.SubscriptionPlan{Title: "zero", Enabled: true, TotalAmount: 10, DurationUnit: "month", DurationValue: 1}
			require.NoError(t, db.Create(&plan).Error)
			sub := model.UserSubscription{UserId: user.Id, PlanId: plan.Id, AmountTotal: 10, Status: "active", EndTime: 1<<31 - 1, AllowWalletOverflow: true}
			require.NoError(t, db.Create(&sub).Error)
			billingContext := ingressBillingContext(0)
			result, err := ExecuteTaskIngress(context.Background(), TaskIngressRequest{
				UserID: user.Id, TokenID: token.Id, ChannelID: 10, Provider: "test", OperationKind: model.TaskSubmissionOperationKindVideoCreate,
				HTTPMethod: http.MethodPost, Header: http.Header{"Idempotency-Key": []string{"zero-" + tc.name}}, ContentType: "application/json",
				Body: []byte(`{"prompt":"zero"}`), EstimatedQuota: 0, FreeModel: tc.free, BillingContext: billingContext, DB: db,
				Dispatcher: func(_ context.Context, op *model.TaskSubmissionOperation, attempt *model.TaskSubmissionAttempt) (*TaskProviderDispatchResult, error) {
					return &TaskProviderDispatchResult{Status: TaskProviderDispatchStatusAccepted, ProviderTaskID: "zero-upstream", TaskCandidate: acceptedTaskCandidateForTest(op, attempt, "zero-upstream", int(tc.wantReserved), billingContext)}, nil
				},
			})
			require.NoError(t, err)
			op, err := model.GetTaskSubmissionOperationByPublicID(db, result.Response.ID)
			require.NoError(t, err)
			var receipt model.QuotaMutationReceipt
			require.NoError(t, db.Where("operation_id = ? AND mutation_type = ?", op.ID, model.TaskBillingEventTypeReserve).First(&receipt).Error)
			assert.Equal(t, tc.wantReserved, receipt.Quota)
			assert.Equal(t, int64(0), receipt.EstimatedQuota)
			assert.Equal(t, tc.free, receipt.FreeModel)
			var updatedToken model.Token
			var updatedSub model.UserSubscription
			require.NoError(t, db.First(&updatedToken, token.Id).Error)
			require.NoError(t, db.First(&updatedSub, sub.Id).Error)
			assert.Equal(t, 1-int(tc.wantReserved), updatedToken.RemainQuota)
			assert.Equal(t, tc.wantReserved, updatedSub.AmountUsed)
			if !tc.free {
				oldDB := model.DB
				model.DB = db
				t.Cleanup(func() { model.DB = oldDB })
				var task model.Task
				require.NoError(t, db.First(&task, *op.TaskID).Error)
				assert.Zero(t, task.Quota)
				settleTaskBillingOnComplete(context.Background(), &mockAdaptor{adjustReturn: 0}, &task, &relaycommon.TaskInfo{Status: model.TaskStatusSuccess})
				require.NoError(t, db.First(&updatedToken, token.Id).Error)
				require.NoError(t, db.First(&updatedSub, sub.Id).Error)
				require.NoError(t, db.First(&task, task.ID).Error)
				assert.Equal(t, 1, updatedToken.RemainQuota)
				assert.Zero(t, updatedSub.AmountUsed)
				assert.Zero(t, task.Quota)
			}
		})
	}
}
