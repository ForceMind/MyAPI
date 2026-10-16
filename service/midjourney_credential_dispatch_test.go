package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type credentialDispatchRoundTripper func(*http.Request) (*http.Response, error)

func (f credentialDispatchRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestMidjourneyCredentialGenerationRecheckedBeforeSend(t *testing.T) {
	priorDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = priorDB })
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	token := model.Token{UserId: 91, Key: "midjourney-generation", CredentialVersion: 2}
	require.NoError(t, db.Create(&token).Error)

	var sends atomic.Int32
	restoreClient := SetHttpClientForTest(&http.Client{Transport: credentialDispatchRoundTripper(func(*http.Request) (*http.Response, error) {
		sends.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":1}`))}, nil
	})})
	t.Cleanup(restoreClient)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://gateway.test/mj/submit", strings.NewReader(`{"prompt":"synthetic"}`))
	common.SetContextKey(ctx, constant.ContextKeyUserId, token.UserId)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, token.Id)
	common.SetContextKey(ctx, constant.ContextKeyTokenCredentialVersion, token.CredentialVersion)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("credential_version", token.CredentialVersion+1).Error)

	response, _, err := DoMidjourneyHttpRequest(ctx, time.Second, "https://provider.example.test/mj/submit", &relaycommon.RelayInfo{
		UserId: token.UserId, TokenId: token.Id, TokenCredentialVersion: token.CredentialVersion,
		RequestId: "midjourney-stale-generation",
	})
	require.ErrorIs(t, err, model.ErrTokenCredentialVersionChanged)
	require.NotNil(t, response)
	require.Equal(t, http.StatusForbidden, response.StatusCode)
	require.Zero(t, sends.Load())
}

func TestMidjourneyAuthoritativeAdmissionExistsBeforeProviderSend(t *testing.T) {
	db := setupPostConsumeModeDB(t, model.QuotaWriterModeAuthoritative)
	require.NoError(t, db.AutoMigrate(&model.TokenCredentialDispatchAdmission{}))
	user, token := seedAuthoritativeBilling(t, db, "midjourney-dispatch-admission", 1000, 500, false)
	require.NoError(t, db.Model(token).Update("credential_version", int64(3)).Error)

	var sends atomic.Int32
	var observedOpen atomic.Bool
	restoreClient := SetHttpClientForTest(&http.Client{Transport: credentialDispatchRoundTripper(func(*http.Request) (*http.Response, error) {
		sends.Add(1)
		var count int64
		err := db.Model(&model.TokenCredentialDispatchAdmission{}).
			Where("token_id = ? AND state = ?", token.Id, model.TokenCredentialDispatchAdmissionStateOpen).
			Count(&count).Error
		observedOpen.Store(err == nil && count == 1)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":1,"result":"synthetic-mj-task"}`))}, nil
	})})
	t.Cleanup(restoreClient)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://gateway.test/mj/submit", strings.NewReader(`{"prompt":"synthetic"}`))
	info := &relaycommon.RelayInfo{
		UserId: user.Id, TokenId: token.Id, TokenCredentialVersion: 3,
		RequestId: "midjourney-dispatch-admission",
	}
	response, _, err := DoMidjourneyHttpRequest(ctx, time.Second, "https://provider.example.test/mj/submit", info)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, int32(1), sends.Load())
	require.True(t, observedOpen.Load(), "provider send must observe the committed open admission")
	require.Len(t, info.TokenCredentialDispatchAdmissionID, 64)

	require.NoError(t, MarkTokenCredentialDispatchAdmissionUnknown(context.Background(), info, "provider_outcome_unknown"))
	var admission model.TokenCredentialDispatchAdmission
	require.NoError(t, db.Where("admission_id = ?", info.TokenCredentialDispatchAdmissionID).Take(&admission).Error)
	require.Equal(t, model.TokenCredentialDispatchAdmissionStateUnknown, admission.State)
}
