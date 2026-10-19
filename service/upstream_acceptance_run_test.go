package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

type acceptanceRunTestTransport struct {
	calls atomic.Int32
	send  func(*http.Request) (*http.Response, error)
}

func (transport *acceptanceRunTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	transport.calls.Add(1)
	if transport.send != nil {
		return transport.send(req)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("synthetic response")), Request: req}, nil
}

type acceptanceRunTestBody struct {
	io.ReadCloser
	onClose func()
	once    sync.Once
}

func (body *acceptanceRunTestBody) Close() error {
	body.once.Do(body.onClose)
	return body.ReadCloser.Close()
}

func setupAcceptanceRunServiceTest(t *testing.T) (*model.Channel, *acceptanceRunTestTransport, *http.Client) {
	t.Helper()
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "true")
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/acceptance-service.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(2)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.UpstreamAcceptanceRun{}, &model.SystemTaskLock{}))
	previousDB, previousMemory, previousRedis, previousTLS := model.DB, common.MemoryCacheEnabled, common.RedisEnabled, common.TLSInsecureSkipVerify
	previousTransportPolicy := upstreamAcceptanceTestTransportAllowed
	upstreamAcceptance.Lock()
	previousOwner, previousRun := upstreamAcceptance.owner, upstreamAcceptance.run
	upstreamAcceptance.owner, upstreamAcceptance.run = "acceptance-test-owner", nil
	upstreamAcceptance.Unlock()
	model.DB, common.MemoryCacheEnabled, common.RedisEnabled, common.TLSInsecureSkipVerify = db, false, false, false
	transport := &acceptanceRunTestTransport{}
	upstreamAcceptanceTestTransportAllowed = func(candidate http.RoundTripper) bool { return candidate == transport }
	t.Cleanup(func() {
		upstreamAcceptance.Lock()
		if current := upstreamAcceptance.run; current != nil {
			current.cancel()
			for operation := range current.operations {
				operation.cancel()
			}
		}
		upstreamAcceptance.owner, upstreamAcceptance.run = previousOwner, previousRun
		upstreamAcceptance.Unlock()
		upstreamAcceptanceTestTransportAllowed = previousTransportPolicy
		model.DB, common.MemoryCacheEnabled, common.RedisEnabled, common.TLSInsecureSkipVerify = previousDB, previousMemory, previousRedis, previousTLS
		require.NoError(t, pool.Close())
	})
	baseURL := "http://127.0.0.1:1"
	key, err := common.Marshal(CodexOAuthKey{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "synthetic-account"})
	require.NoError(t, err)
	channel := &model.Channel{Type: constant.ChannelTypeCodex, Name: "synthetic-acceptance", BaseURL: &baseURL, Key: string(key)}
	require.NoError(t, db.Create(channel).Error)
	return channel, transport, &http.Client{Transport: transport}
}

func acceptanceRunRequest(t *testing.T, ctx context.Context, channel *model.Channel, kind string) *http.Request {
	t.Helper()
	method, endpoint := http.MethodGet, channel.GetBaseURL()+"/backend-api/wham/usage"
	var body io.Reader
	switch kind {
	case model.UpstreamAcceptanceDispatchAuth, model.UpstreamAcceptanceDispatchRefresh:
		method, endpoint = http.MethodPost, "https://auth.openai.com/oauth/token"
		form := url.Values{"client_id": {codexOAuthClientID}}
		if kind == model.UpstreamAcceptanceDispatchRefresh {
			form.Set("grant_type", "refresh_token")
			form.Set("refresh_token", "synthetic-refresh")
		} else {
			form.Set("grant_type", "authorization_code")
			form.Set("code", "synthetic-code")
			form.Set("code_verifier", "synthetic-verifier")
			form.Set("redirect_uri", codexOAuthRedirectURI)
		}
		body = strings.NewReader(form.Encode())
	case model.UpstreamAcceptanceDispatchModels:
		endpoint = channel.GetBaseURL() + "/backend-api/codex/models?client_version=synthetic"
	case model.UpstreamAcceptanceDispatchGeneration:
		method, endpoint = http.MethodPost, channel.GetBaseURL()+"/backend-api/codex/responses"
		body = strings.NewReader(`{"model":"synthetic-model","input":"synthetic"}`)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	require.NoError(t, err)
	if kind == model.UpstreamAcceptanceDispatchAuth || kind == model.UpstreamAcceptanceDispatchRefresh {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req.Header.Set("Authorization", "Bearer synthetic-access")
		req.Header.Set("ChatGPT-Account-ID", "synthetic-account")
	}
	return req
}

func startAcceptanceRunServiceTest(t *testing.T, channel *model.Channel, limits UpstreamAcceptanceLimits) (*model.UpstreamAcceptanceRun, context.Context, func()) {
	t.Helper()
	limits.DurationSeconds = 60
	run, err := StartUpstreamAcceptanceRun(context.Background(), channel.Id, limits)
	require.NoError(t, err)
	ctx, finish, err := BeginUpstreamAcceptanceOperation(context.Background(), channel.Id, "synthetic-account")
	require.NoError(t, err)
	t.Cleanup(finish)
	return run, ctx, finish
}

func TestUpstreamAcceptanceServiceDisabledPreservesClientWithoutDatabase(t *testing.T) {
	t.Setenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED", "false")
	previousDB := model.DB
	model.DB = nil
	t.Cleanup(func() { model.DB = previousDB })
	transport := &acceptanceRunTestTransport{}
	client := &http.Client{Transport: transport}
	redirects := 0
	client.CheckRedirect = func(*http.Request, []*http.Request) error { redirects++; return http.ErrUseLastResponse }
	transport.send = func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"http://127.0.0.1:1/second"}}, Body: io.NopCloser(strings.NewReader("")), Request: req}, nil
	}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:1/first", nil)
	require.NoError(t, err)
	response, err := DoUpstreamAcceptanceHTTP(client, req, "ordinary-mode")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, 1, redirects)
	assert.Equal(t, int32(1), transport.calls.Load())
	assert.Same(t, transport, client.Transport)
	assert.Equal(t, http.StatusFound, response.StatusCode)
}

func TestUpstreamAcceptanceServiceScopeAndRestartFence(t *testing.T) {
	channel, transport, client := setupAcceptanceRunServiceTest(t)
	req := acceptanceRunRequest(t, context.Background(), channel, "sample")
	_, err := DoUpstreamAcceptanceHTTP(client, req, "sample")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
	_, _, err = BeginUpstreamAcceptanceOperation(context.Background(), channel.Id, "synthetic-account")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
	run, ctx, finish := startAcceptanceRunServiceTest(t, channel, UpstreamAcceptanceLimits{MaxTotal: 2, MaxSample: 2})
	_, err = DoUpstreamAcceptanceHTTP(client, req, "sample")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent, "active run alone cannot authorize an unscoped request")
	_, _, err = BeginUpstreamAcceptanceOperation(context.Background(), channel.Id+1, "synthetic-account")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
	_, _, err = BeginUpstreamAcceptanceOperation(context.Background(), channel.Id, "other-account")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
	upstreamAcceptance.Lock()
	upstreamAcceptance.owner = "new-process-owner"
	upstreamAcceptance.Unlock()
	status, err := GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.False(t, status.OwnerCurrent)
	assert.False(t, status.CanDispatch)
	_, err = DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "sample"), "sample")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
	_, err = StopUpstreamAcceptanceRun(context.Background(), run.ID)
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceDenied)
	finish()
	assert.Zero(t, transport.calls.Load())
	assert.Zero(t, status.Run.Total)
}

func TestUpstreamAcceptanceServiceCountsFailedAttemptsAndSeparateBudgets(t *testing.T) {
	channel, transport, client := setupAcceptanceRunServiceTest(t)
	run, ctx, _ := startAcceptanceRunServiceTest(t, channel, UpstreamAcceptanceLimits{MaxTotal: 5, MaxAuth: 1, MaxRefresh: 1, MaxSample: 1, MaxModels: 1, MaxGeneration: 1})
	for _, kind := range []string{"auth", "refresh", "sample", "models", "generation"} {
		t.Run(kind, func(t *testing.T) {
			if kind == "sample" {
				transport.send = func(*http.Request) (*http.Response, error) { return nil, errors.New("synthetic upstream failure") }
			}
			response, err := DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, kind), kind)
			if kind == "sample" {
				require.Error(t, err)
				assert.NotErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
			} else {
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
			}
			transport.send = nil
			_, err = DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, kind), kind)
			assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
		})
	}
	status, err := GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(5), transport.calls.Load())
	assert.Equal(t, int64(5), status.Run.Total)
	assert.Equal(t, int64(1), status.Run.Auth)
	assert.Equal(t, int64(1), status.Run.Refresh)
	assert.Equal(t, int64(1), status.Run.Sample)
	assert.Equal(t, int64(1), status.Run.Models)
	assert.Equal(t, int64(1), status.Run.Generation)
	assert.Zero(t, status.HTTPInFlight)
	assert.False(t, status.CanDispatch)
}

func TestUpstreamAcceptanceServiceTotalBudgetCannotBorrowAnotherClass(t *testing.T) {
	channel, transport, client := setupAcceptanceRunServiceTest(t)
	run, ctx, _ := startAcceptanceRunServiceTest(t, channel, UpstreamAcceptanceLimits{MaxTotal: 1, MaxSample: 1, MaxModels: 1})
	response, err := DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "sample"), "sample")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	_, err = DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "models"), "models")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
	status, err := GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, int32(1), transport.calls.Load())
	assert.Equal(t, int64(1), status.Run.Total)
	assert.Zero(t, status.Run.Models)
}

func TestUpstreamAcceptanceServiceGenerationBodyOwnsConcurrencySlot(t *testing.T) {
	for _, release := range []string{"eof", "close"} {
		t.Run(release, func(t *testing.T) {
			channel, transport, client := setupAcceptanceRunServiceTest(t)
			run, ctx, _ := startAcceptanceRunServiceTest(t, channel, UpstreamAcceptanceLimits{MaxTotal: 2, MaxGeneration: 2})
			response, err := DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "generation"), "generation")
			require.NoError(t, err)
			t.Cleanup(func() { _ = response.Body.Close() })
			_, err = DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "generation"), "generation")
			assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
			status, err := GetUpstreamAcceptanceStatus(context.Background(), run.ID)
			require.NoError(t, err)
			assert.Equal(t, 1, status.HTTPInFlight)
			assert.Equal(t, int64(1), status.Run.GenerationInFlight)
			if release == "eof" {
				_, err = io.ReadAll(response.Body)
				require.NoError(t, err)
			} else {
				require.NoError(t, response.Body.Close())
			}
			second, err := DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "generation"), "generation")
			require.NoError(t, err)
			require.NoError(t, second.Body.Close())
			require.NoError(t, second.Body.Close())
			status, err = GetUpstreamAcceptanceStatus(context.Background(), run.ID)
			require.NoError(t, err)
			assert.Equal(t, int32(2), transport.calls.Load())
			assert.Equal(t, int64(2), status.Run.Generation)
			assert.Zero(t, status.Run.GenerationInFlight)
			assert.Zero(t, status.HTTPInFlight)
		})
	}
}

func TestUpstreamAcceptanceServiceStopCancelsBodyAndWaitsForLogicalDrain(t *testing.T) {
	channel, transport, client := setupAcceptanceRunServiceTest(t)
	run, ctx, finish := startAcceptanceRunServiceTest(t, channel, UpstreamAcceptanceLimits{MaxTotal: 2, MaxGeneration: 2})
	closed := make(chan struct{})
	transport.send = func(req *http.Request) (*http.Response, error) {
		body := &acceptanceRunTestBody{ReadCloser: io.NopCloser(strings.NewReader("pending stream")), onClose: func() { close(closed) }}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	}
	response, err := DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "generation"), "generation")
	require.NoError(t, err)
	status, err := StopUpstreamAcceptanceRun(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAcceptanceRunStopping, status.Run.Status)
	assert.Equal(t, 1, status.Operations)
	assert.False(t, status.CanDispatch)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not close the in-flight response body")
	}
	// Close joins the cancellation callback's completion deterministically.
	require.NoError(t, response.Body.Close())
	_, err = DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "generation"), "generation")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
	status, err = GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAcceptanceRunStopping, status.Run.Status)
	assert.Zero(t, status.HTTPInFlight)
	finish()
	finish()
	status, err = GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAcceptanceRunStopped, status.Run.Status)
	assert.Zero(t, status.Operations)
	assert.False(t, status.DrainError)
	assert.Equal(t, int32(1), transport.calls.Load())
}

func TestUpstreamAcceptanceServiceRejectsChangedCredentialAndMalformedRequests(t *testing.T) {
	channel, transport, client := setupAcceptanceRunServiceTest(t)
	run, ctx, _ := startAcceptanceRunServiceTest(t, channel, UpstreamAcceptanceLimits{MaxTotal: 10, MaxRefresh: 2, MaxSample: 2, MaxModels: 2})
	for _, test := range []struct {
		name, kind string
		change     func(*http.Request)
	}{
		{"wrong_access", "sample", func(r *http.Request) { r.Header.Set("Authorization", "Bearer other-access") }},
		{"duplicate_access", "sample", func(r *http.Request) { r.Header.Add("Authorization", "Bearer synthetic-access") }},
		{"mixed_case_duplicate_access", "sample", func(r *http.Request) { r.Header["authorization"] = []string{"Bearer synthetic-access"} }},
		{"wrong_account", "sample", func(r *http.Request) { r.Header.Set("ChatGPT-Account-ID", "other-account") }},
		{"cookie", "sample", func(r *http.Request) { r.Header.Set("Cookie", "synthetic=value") }},
		{"noncanonical_cookie", "sample", func(r *http.Request) { r.Header["cookie"] = []string{"synthetic=value"} }},
		{"unknown_path", "sample", func(r *http.Request) { r.URL.Path = "/backend-api/other" }},
		{"escaped_path", "sample", func(r *http.Request) { r.URL.RawPath = "/backend-api/wham/%75sage" }},
		{"different_host", "sample", func(r *http.Request) { r.URL.Host = "127.0.0.2:1" }},
		{"extra_query", "sample", func(r *http.Request) { r.URL.RawQuery = "extra=true" }},
		{"duplicate_version", "models", func(r *http.Request) { r.URL.RawQuery = "client_version=a&client_version=b" }},
		{"invalid_query", "models", func(r *http.Request) { r.URL.RawQuery = "client_version=a&bad=%zz" }},
		{"wrong_refresh", "refresh", func(r *http.Request) {
			body := "client_id=" + url.QueryEscape(codexOAuthClientID) + "&grant_type=refresh_token&refresh_token=other-refresh"
			r.Body = io.NopCloser(strings.NewReader(body))
			r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }
		}},
		{"duplicate_refresh", "refresh", func(r *http.Request) {
			body := "client_id=" + url.QueryEscape(codexOAuthClientID) + "&grant_type=refresh_token&refresh_token=synthetic-refresh&refresh_token=other-refresh"
			r.Body = io.NopCloser(strings.NewReader(body))
			r.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(body)), nil }
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := acceptanceRunRequest(t, ctx, channel, test.kind)
			test.change(req)
			response, err := DoUpstreamAcceptanceHTTP(client, req, test.kind)
			if response != nil {
				_ = response.Body.Close()
			}
			assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
			assert.Zero(t, transport.calls.Load())
		})
	}
	changed, err := common.Marshal(CodexOAuthKey{AccessToken: "synthetic-access", RefreshToken: "synthetic-refresh", AccountID: "different-account"})
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("key", string(changed)).Error)
	_, err = DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "sample"), "sample")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
	status, err := GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Zero(t, status.Run.Total)
	assert.Zero(t, transport.calls.Load())
}

func TestUpstreamAcceptanceServiceStorageFailureLatchesStop(t *testing.T) {
	channel, transport, client := setupAcceptanceRunServiceTest(t)
	run, ctx, _ := startAcceptanceRunServiceTest(t, channel, UpstreamAcceptanceLimits{MaxTotal: 2, MaxSample: 2})
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("test:acceptance_write_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "upstream_acceptance_runs" {
			tx.AddError(errors.New("synthetic storage failure"))
		}
	}))
	_, err := DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "sample"), "sample")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
	require.NoError(t, model.DB.Callback().Update().Remove("test:acceptance_write_failure"))
	status, err := GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.True(t, status.DrainError)
	assert.False(t, status.CanDispatch)
	assert.Zero(t, status.Run.Total)
	_, err = DoUpstreamAcceptanceHTTP(client, acceptanceRunRequest(t, ctx, channel, "sample"), "sample")
	assert.ErrorIs(t, err, ErrUpstreamAcceptanceUnsent)
	assert.Zero(t, transport.calls.Load())
}

func TestUpstreamAcceptanceServiceRefreshStopRetainsRotatedCredential(t *testing.T) {
	channel, transport, _ := setupAcceptanceRunServiceTest(t)
	establishCodexCredentialSubjectEvidence(t, model.DB, channel.Id, channel.Key, "synthetic-account")
	run, err := StartUpstreamAcceptanceRun(context.Background(), channel.Id, UpstreamAcceptanceLimits{DurationSeconds: 60, MaxTotal: 1, MaxRefresh: 1})
	require.NoError(t, err)
	client, err := GetHttpClientWithProxy("")
	require.NoError(t, err)
	previousTransport := client.Transport
	client.Transport = transport
	t.Cleanup(func() { client.Transport = previousTransport })
	accessToken := codexCredentialTestJWT(t, "synthetic-account")
	payload, err := common.Marshal(map[string]any{"access_token": accessToken, "refresh_token": "synthetic-rotated-refresh", "expires_in": 3600})
	require.NoError(t, err)
	var stopStatus *UpstreamAcceptanceStatus
	var stopErr error
	transport.send = func(req *http.Request) (*http.Response, error) {
		body := &acceptanceRunTestBody{ReadCloser: io.NopCloser(strings.NewReader(string(payload))), onClose: func() {
			// The OAuth helper closes its body after decoding a successful
			// response, before RefreshCodexChannelCredential persists rotation.
			stopStatus, stopErr = StopUpstreamAcceptanceRun(context.Background(), run.ID)
		}}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	}
	refreshed, _, err := RefreshCodexChannelCredential(context.Background(), channel.Id, CodexCredentialRefreshOptions{ExpectedKey: &channel.Key})
	require.NoError(t, err)
	require.NoError(t, stopErr)
	require.NotNil(t, stopStatus)
	assert.Equal(t, model.UpstreamAcceptanceRunStopping, stopStatus.Run.Status)
	assert.Equal(t, 1, stopStatus.Operations)
	require.NotNil(t, refreshed)
	assert.Equal(t, "synthetic-rotated-refresh", refreshed.RefreshToken)
	var saved model.Channel
	require.NoError(t, model.DB.First(&saved, channel.Id).Error)
	stored, err := parseCodexOAuthKey(saved.Key)
	require.NoError(t, err)
	assert.Equal(t, "synthetic-rotated-refresh", stored.RefreshToken)
	status, err := GetUpstreamAcceptanceStatus(context.Background(), run.ID)
	require.NoError(t, err)
	assert.Equal(t, model.UpstreamAcceptanceRunStopped, status.Run.Status)
	assert.Equal(t, int64(1), status.Run.Refresh)
	assert.Equal(t, int64(1), status.Run.Total)
	assert.Zero(t, status.Operations)
	assert.Zero(t, status.HTTPInFlight)
	assert.False(t, status.DrainError)
	assert.Equal(t, int32(1), transport.calls.Load())
}
