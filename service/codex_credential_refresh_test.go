package service

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type codexCredentialRefreshRoundTripper func(*http.Request) (*http.Response, error)

func (transport codexCredentialRefreshRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestRefreshCodexChannelCredentialSerializesConcurrentRotationAndReusesWinner(t *testing.T) {
	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SystemTaskLock{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	channel := &model.Channel{
		Type: constant.ChannelTypeCodex,
		Name: "concurrent-refresh-fixture",
		Key:  `{"access_token":"old-access","refresh_token":"old-refresh","account_id":"account-123456"}`,
	}
	require.NoError(t, db.Create(channel).Error)
	expectedKey := channel.Key

	client, err := GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	firstRequestStarted := make(chan struct{})
	releaseFirstRequest := make(chan struct{})
	var requestCount atomic.Int32
	var leaseVisible atomic.Bool
	var leaseUntil atomic.Int64
	client.Transport = codexCredentialRefreshRoundTripper(func(request *http.Request) (*http.Response, error) {
		var lock model.SystemTaskLock
		if err := db.Where("type = ?", "codex_credential_refresh:"+strconv.Itoa(channel.Id)).First(&lock).Error; err == nil {
			leaseVisible.Store(true)
			leaseUntil.Store(lock.LockedUntil)
		}
		if requestCount.Add(1) == 1 {
			close(firstRequestStarted)
			<-releaseFirstRequest
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"winner-access","refresh_token":"winner-refresh","expires_in":3600}`)),
			Request:    request,
		}, nil
	})

	type refreshResult struct {
		key *CodexOAuthKey
		err error
	}
	results := make(chan refreshResult, 2)
	refresh := func() {
		key, _, err := RefreshCodexChannelCredential(context.Background(), channel.Id, CodexCredentialRefreshOptions{ExpectedKey: &expectedKey})
		results <- refreshResult{key: key, err: err}
	}
	go refresh()
	<-firstRequestStarted
	go refresh()
	close(releaseFirstRequest)

	for range 2 {
		result := <-results
		require.NoError(t, result.err)
		require.NotNil(t, result.key)
		assert.Equal(t, "winner-access", result.key.AccessToken)
		assert.Equal(t, "winner-refresh", result.key.RefreshToken)
	}
	assert.Equal(t, int32(1), requestCount.Load(), "only the gate winner may rotate the upstream refresh token")
	assert.True(t, leaseVisible.Load(), "the distributed lease must exist before the upstream refresh starts")
	assert.GreaterOrEqual(t, leaseUntil.Load(), time.Now().Add(15*time.Second).Unix())
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	storedCredential, err := parseCodexOAuthKey(stored.Key)
	require.NoError(t, err)
	assert.Equal(t, "winner-refresh", storedCredential.RefreshToken)
	var lockCount int64
	require.NoError(t, db.Model(&model.SystemTaskLock{}).Count(&lockCount).Error)
	assert.Zero(t, lockCount)
}

func TestRefreshCodexChannelCredentialPersistsRotatedTokenAfterRequestCancellation(t *testing.T) {
	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SystemTaskLock{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	channel := &model.Channel{
		Type: constant.ChannelTypeCodex,
		Name: "refresh-cancellation-fixture",
		Key:  `{"access_token":"old-access","refresh_token":"old-refresh","account_id":"account-123456"}`,
	}
	require.NoError(t, db.Create(channel).Error)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:inspect_codex_persistence_context", func(tx *gorm.DB) {
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
		assert.NoError(t, tx.Statement.Context.Err())
		deadline, present := tx.Statement.Context.Deadline()
		assert.True(t, present)
		assert.LessOrEqual(t, time.Until(deadline), codexCredentialPersistenceTimeout)
	}))

	client, err := GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	client.Transport = codexCredentialRefreshRoundTripper(func(request *http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`)),
			Request:    request,
		}, nil
	})

	refreshed, _, err := RefreshCodexChannelCredential(ctx, channel.Id, CodexCredentialRefreshOptions{ResetCaches: true})

	require.NoError(t, err)
	require.NotNil(t, refreshed)
	assert.Equal(t, "new-refresh", refreshed.RefreshToken)
	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	storedCredential, err := parseCodexOAuthKey(stored.Key)
	require.NoError(t, err)
	assert.Equal(t, "new-access", storedCredential.AccessToken)
	assert.Equal(t, "new-refresh", storedCredential.RefreshToken)
	var lockCount int64
	require.NoError(t, db.Model(&model.SystemTaskLock{}).Count(&lockCount).Error)
	assert.Zero(t, lockCount, "release must use an independent context after request cancellation")
}
