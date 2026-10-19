package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/google/uuid"
)

const upstreamAcceptanceWriteTimeout = 5 * time.Second

var (
	ErrUpstreamAcceptanceDenied = errors.New("upstream acceptance denied")
	ErrUpstreamAcceptanceUnsent = errors.New("upstream acceptance request not sent")
	upstreamAcceptance          = struct {
		sync.Mutex
		owner string
		run   *upstreamAcceptanceRuntime
	}{owner: uuid.NewString()}
)

// This opt-in controls only the documented Codex acceptance paths. It is not a
// process-wide network firewall and never enables itself through an API call.
func UpstreamAcceptanceModeEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("MYAPI_UPSTREAM_ACCEPTANCE_ENABLED")), "true")
}

type UpstreamAcceptanceLimits struct {
	DurationSeconds int64 `json:"duration_seconds"`
	MaxTotal        int64 `json:"max_total"`
	MaxAuth         int64 `json:"max_auth"`
	MaxRefresh      int64 `json:"max_refresh"`
	MaxSample       int64 `json:"max_sample"`
	MaxModels       int64 `json:"max_models"`
	MaxGeneration   int64 `json:"max_generation"`
}

type UpstreamAcceptanceStatus struct {
	Run          *model.UpstreamAcceptanceRun `json:"run"`
	OwnerCurrent bool                         `json:"owner_current"`
	Operations   int                          `json:"operations"`
	HTTPInFlight int                          `json:"http_in_flight"`
	CanDispatch  bool                         `json:"can_dispatch"`
	DrainError   bool                         `json:"drain_error"`
}

type upstreamAcceptanceRuntime struct {
	id, owner, accountRef string
	channelID             int
	ctx                   context.Context
	cancel                context.CancelFunc
	operations            map[*upstreamAcceptanceOperation]struct{}
	httpInFlight          int
	stopping, drainError  bool
}

type upstreamAcceptanceOperation struct {
	run    *upstreamAcceptanceRuntime
	ctx    context.Context
	cancel context.CancelFunc
	finish sync.Once
}

type upstreamAcceptanceOperationKey struct{}

func StartUpstreamAcceptanceRun(ctx context.Context, channelID int, limits UpstreamAcceptanceLimits) (*model.UpstreamAcceptanceRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancelWrite := context.WithTimeout(ctx, upstreamAcceptanceWriteTimeout)
	defer cancelWrite()
	if !UpstreamAcceptanceModeEnabled() || limits.DurationSeconds < 1 || limits.DurationSeconds > 1200 ||
		limits.MaxTotal < 1 || limits.MaxTotal > 1000 || limits.MaxGeneration < 0 || limits.MaxGeneration > 10 {
		return nil, ErrUpstreamAcceptanceDenied
	}
	channel, key, err := acceptanceChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if _, ok := acceptanceOrigin(channel.GetBaseURL()); !ok {
		return nil, ErrUpstreamAcceptanceDenied
	}
	upstreamAcceptance.Lock()
	defer upstreamAcceptance.Unlock()
	if current := upstreamAcceptance.run; current != nil {
		if current.ctx.Err() != nil && !current.stopping {
			current.stopping = true
			stopAcceptanceLocked(current, "deadline")
			drainAcceptanceLocked(current)
		}
		if !current.stopping || len(current.operations) != 0 || current.httpInFlight != 0 || current.drainError {
			return nil, ErrUpstreamAcceptanceDenied
		}
	}
	now := time.Now().Unix()
	run := &model.UpstreamAcceptanceRun{
		ID: uuid.NewString(), OwnerID: upstreamAcceptance.owner, ChannelID: channelID,
		AccountRef:    model.ChannelQuotaAccountRef("codex", key.AccountID),
		CreatedAtUnix: now, DeadlineAtUnix: now + limits.DurationSeconds,
		MaxTotal: limits.MaxTotal, MaxAuth: limits.MaxAuth, MaxRefresh: limits.MaxRefresh,
		MaxSample: limits.MaxSample, MaxModels: limits.MaxModels, MaxGeneration: limits.MaxGeneration,
	}
	if err := model.CreateUpstreamAcceptanceRun(ctx, run); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithDeadline(context.Background(), time.Unix(run.DeadlineAtUnix, 0))
	upstreamAcceptance.run = &upstreamAcceptanceRuntime{
		id: run.ID, owner: run.OwnerID, channelID: channelID, accountRef: run.AccountRef,
		ctx: runCtx, cancel: cancel, operations: make(map[*upstreamAcceptanceOperation]struct{}),
	}
	return run, nil
}

func GetUpstreamAcceptanceStatus(ctx context.Context, id string) (*UpstreamAcceptanceStatus, error) {
	if !UpstreamAcceptanceModeEnabled() {
		return nil, ErrUpstreamAcceptanceDenied
	}
	upstreamAcceptance.Lock()
	defer upstreamAcceptance.Unlock()
	current := upstreamAcceptance.run
	if current != nil && current.id == id && current.ctx.Err() != nil {
		current.stopping = true
		stopAcceptanceLocked(current, "deadline")
		drainAcceptanceLocked(current)
	}
	return acceptanceStatusLocked(ctx, id)
}

func StopUpstreamAcceptanceRun(ctx context.Context, id string) (*UpstreamAcceptanceStatus, error) {
	if !UpstreamAcceptanceModeEnabled() {
		return nil, ErrUpstreamAcceptanceDenied
	}
	upstreamAcceptance.Lock()
	defer upstreamAcceptance.Unlock()
	current := upstreamAcceptance.run
	if current == nil || current.id != id || current.owner != upstreamAcceptance.owner {
		return nil, ErrUpstreamAcceptanceDenied
	}
	// Latch memory first even if durable storage is unavailable. A storage
	// failure must never leave this process able to send another request.
	current.stopping = true
	current.cancel()
	writeCtx, cancel := context.WithTimeout(context.Background(), upstreamAcceptanceWriteTimeout)
	_, err := model.StopUpstreamAcceptanceRun(writeCtx, id, current.owner, time.Now().Unix(), "manual")
	cancel()
	if err != nil {
		current.drainError = true
		return nil, err
	}
	drainAcceptanceLocked(current)
	return acceptanceStatusLocked(ctx, id)
}

func acceptanceStatusLocked(ctx context.Context, id string) (*UpstreamAcceptanceStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, upstreamAcceptanceWriteTimeout)
	defer cancel()
	run, err := model.GetUpstreamAcceptanceRun(ctx, id)
	if err != nil {
		return nil, err
	}
	status := &UpstreamAcceptanceStatus{Run: run}
	current := upstreamAcceptance.run
	if current == nil || current.id != id || current.owner != upstreamAcceptance.owner || run.OwnerID != current.owner {
		return status, nil // A persisted run is never automatically adopted.
	}
	status.OwnerCurrent = true
	status.Operations, status.HTTPInFlight = len(current.operations), current.httpInFlight
	status.DrainError = current.drainError
	status.CanDispatch = !current.stopping && !current.drainError && current.ctx.Err() == nil &&
		run.Status == model.UpstreamAcceptanceRunActive && run.Total < run.MaxTotal
	return status, nil
}

// BeginUpstreamAcceptanceOperation keeps safe local work (notably rotating
// credential persistence) in the drain set after the HTTP response ends.
func BeginUpstreamAcceptanceOperation(ctx context.Context, channelID int, accountID string) (context.Context, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !UpstreamAcceptanceModeEnabled() {
		return ctx, func() {}, nil
	}
	upstreamAcceptance.Lock()
	defer upstreamAcceptance.Unlock()
	current := upstreamAcceptance.run
	if current == nil || current.owner != upstreamAcceptance.owner || current.stopping || current.drainError ||
		current.ctx.Err() != nil || ctx.Err() != nil || channelID != current.channelID ||
		model.ChannelQuotaAccountRef("codex", accountID) != current.accountRef {
		return ctx, func() {}, ErrUpstreamAcceptanceDenied
	}
	if inherited, ok := ctx.Value(upstreamAcceptanceOperationKey{}).(*upstreamAcceptanceOperation); ok {
		if inherited.run != current {
			return ctx, func() {}, ErrUpstreamAcceptanceDenied
		}
		return ctx, func() {}, nil
	}
	readCtx, readCancel := context.WithTimeout(ctx, upstreamAcceptanceWriteTimeout)
	run, err := model.GetUpstreamAcceptanceRun(readCtx, current.id)
	readCancel()
	if err != nil || run.OwnerID != current.owner || run.Status != model.UpstreamAcceptanceRunActive {
		return ctx, func() {}, ErrUpstreamAcceptanceDenied
	}
	operationCtx, cancel := context.WithDeadline(ctx, time.Unix(run.DeadlineAtUnix, 0))
	stopCancel := context.AfterFunc(current.ctx, cancel)
	op := &upstreamAcceptanceOperation{run: current, cancel: cancel}
	op.ctx = context.WithValue(operationCtx, upstreamAcceptanceOperationKey{}, op)
	current.operations[op] = struct{}{}
	return op.ctx, func() {
		op.finish.Do(func() {
			stopCancel()
			cancel()
			upstreamAcceptance.Lock()
			defer upstreamAcceptance.Unlock()
			delete(current.operations, op)
			drainAcceptanceLocked(current)
		})
	}, nil
}

func BeginUpstreamAcceptanceChannelOperation(ctx context.Context, channelID int) (context.Context, func(), error) {
	if !UpstreamAcceptanceModeEnabled() {
		if ctx == nil {
			ctx = context.Background()
		}
		return ctx, func() {}, nil
	}
	_, key, err := acceptanceChannel(ctx, channelID)
	if err != nil {
		return ctx, func() {}, err
	}
	return BeginUpstreamAcceptanceOperation(ctx, channelID, key.AccountID)
}

func acceptanceChannel(ctx context.Context, channelID int) (*model.Channel, *CodexOAuthKey, error) {
	if model.DB == nil || channelID <= 0 {
		return nil, nil, ErrUpstreamAcceptanceDenied
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, upstreamAcceptanceWriteTimeout)
	defer cancel()
	channel, err := model.GetUpstreamAcceptanceChannel(ctx, channelID)
	if err != nil || channel == nil || channel.Type != constant.ChannelTypeCodex || channel.ChannelInfo.IsMultiKey {
		return nil, nil, ErrUpstreamAcceptanceDenied
	}
	key, err := parseCodexOAuthKey(channel.Key)
	if err != nil || strings.TrimSpace(key.AccountID) == "" || strings.TrimSpace(key.AccessToken) == "" {
		return nil, nil, ErrUpstreamAcceptanceDenied
	}
	return channel, key, nil
}

func acceptanceOrigin(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || u.User != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return "", false
	}
	if u.Scheme == "https" && u.Host == "chatgpt.com" {
		return u.String(), true
	}
	if ip := net.ParseIP(u.Hostname()); u.Scheme == "http" && ip != nil && ip.IsLoopback() {
		return u.String(), true
	}
	return "", false
}

// claim is called only after HTTP transport policy is established, immediately
// before client.Do. A cancellation in this narrow interval retains a conservative
// attempt count; it never refunds a failed or ambiguous upstream attempt.
func claimUpstreamAcceptanceDispatch(req *http.Request, kind string) (context.Context, func(), error) {
	upstreamAcceptance.Lock()
	defer upstreamAcceptance.Unlock()
	ctx := req.Context()
	gateCtx, gateCancel := context.WithTimeout(ctx, upstreamAcceptanceWriteTimeout)
	defer gateCancel()
	op, ok := ctx.Value(upstreamAcceptanceOperationKey{}).(*upstreamAcceptanceOperation)
	current := upstreamAcceptance.run
	if !ok || current == nil || op.run != current || current.owner != upstreamAcceptance.owner ||
		current.stopping || current.drainError || ctx.Err() != nil || current.ctx.Err() != nil {
		return ctx, func() {}, ErrUpstreamAcceptanceDenied
	}
	if _, alive := current.operations[op]; !alive {
		return ctx, func() {}, ErrUpstreamAcceptanceDenied
	}
	channel, key, err := acceptanceChannel(gateCtx, current.channelID)
	if err != nil || model.ChannelQuotaAccountRef("codex", key.AccountID) != current.accountRef ||
		!validAcceptanceRequest(req, kind, channel, key) {
		return ctx, func() {}, ErrUpstreamAcceptanceDenied
	}
	_, err = model.ClaimUpstreamAcceptanceDispatch(gateCtx, current.id, current.owner, current.channelID, current.accountRef, kind, time.Now().Unix())
	if err != nil {
		if !errors.Is(err, model.ErrUpstreamAcceptanceDenied) {
			current.stopping, current.drainError = true, true
			current.cancel()
		}
		return ctx, func() {}, err
	}
	current.httpInFlight++
	var finish sync.Once
	return ctx, func() {
		finish.Do(func() {
			upstreamAcceptance.Lock()
			defer upstreamAcceptance.Unlock()
			if kind == model.UpstreamAcceptanceDispatchGeneration {
				writeCtx, cancel := context.WithTimeout(context.Background(), upstreamAcceptanceWriteTimeout)
				err := model.FinishUpstreamAcceptanceGeneration(writeCtx, current.id, current.owner, time.Now().Unix())
				cancel()
				if err != nil {
					current.stopping, current.drainError = true, true
					current.cancel()
				}
			}
			current.httpInFlight--
			drainAcceptanceLocked(current)
		})
	}, nil
}

func validAcceptanceRequest(req *http.Request, kind string, channel *model.Channel, key *CodexOAuthKey) bool {
	u := req.URL
	if u == nil || u.User != nil || u.Opaque != "" || u.RawPath != "" || u.Fragment != "" ||
		(req.Host != "" && req.Host != u.Host) || acceptanceHasHeader(req.Header, "Cookie") || acceptanceHasHeader(req.Header, "Host") {
		return false
	}
	if kind == model.UpstreamAcceptanceDispatchAuth || kind == model.UpstreamAcceptanceDispatchRefresh {
		if req.Method != http.MethodPost || u.Scheme != "https" || u.Host != "auth.openai.com" ||
			u.Path != "/oauth/token" || u.RawQuery != "" || req.GetBody == nil {
			return false
		}
		body, err := req.GetBody()
		if err != nil {
			return false
		}
		data, readErr := io.ReadAll(io.LimitReader(body, 65537))
		_ = body.Close()
		form, parseErr := url.ParseQuery(string(data))
		if readErr != nil || parseErr != nil || len(data) > 65536 || form.Get("client_id") != codexOAuthClientID {
			return false
		}
		for name, values := range form {
			if len(values) != 1 {
				return false
			}
			if name != "grant_type" && name != "client_id" &&
				(kind == model.UpstreamAcceptanceDispatchRefresh && name != "refresh_token" ||
					kind == model.UpstreamAcceptanceDispatchAuth && name != "code" && name != "code_verifier" && name != "redirect_uri") {
				return false
			}
		}
		if kind == model.UpstreamAcceptanceDispatchRefresh {
			return form.Get("grant_type") == "refresh_token" && form.Get("refresh_token") == strings.TrimSpace(key.RefreshToken) && strings.TrimSpace(key.RefreshToken) != ""
		}
		return form.Get("grant_type") == "authorization_code" && form.Get("code") != "" && form.Get("code_verifier") != "" && form.Get("redirect_uri") == codexOAuthRedirectURI
	}
	if kind == model.UpstreamAcceptanceDispatchModels && req.Method == http.MethodGet &&
		u.Scheme == "https" && u.Host == "api.github.com" && u.Path == "/repos/openai/codex/releases/latest" && u.RawQuery == "" {
		return !acceptanceHasHeader(req.Header, "Authorization") && !acceptanceHasHeader(req.Header, "ChatGPT-Account-ID")
	}
	origin, ok := acceptanceOrigin(channel.GetBaseURL())
	if !ok || u.Scheme+"://"+u.Host != origin || acceptanceHeader(req.Header, "Authorization") != "Bearer "+strings.TrimSpace(key.AccessToken) ||
		acceptanceHeader(req.Header, "ChatGPT-Account-ID") != strings.TrimSpace(key.AccountID) {
		return false
	}
	switch kind {
	case model.UpstreamAcceptanceDispatchSample:
		return req.Method == http.MethodGet && u.RawQuery == "" &&
			(u.Path == "/backend-api/wham/usage" || u.Path == "/backend-api/wham/rate-limit-reset-credits")
	case model.UpstreamAcceptanceDispatchModels:
		query, err := url.ParseQuery(u.RawQuery)
		return req.Method == http.MethodGet && u.Path == "/backend-api/codex/models" &&
			err == nil && len(query) == 1 && len(query["client_version"]) == 1 && query.Get("client_version") != ""
	case model.UpstreamAcceptanceDispatchGeneration:
		return req.Method == http.MethodPost && u.RawQuery == "" &&
			(u.Path == "/backend-api/codex/responses" || u.Path == "/backend-api/codex/responses/compact")
	default:
		return false
	}
}

func acceptanceHeader(headers http.Header, name string) string {
	var values []string
	for key, items := range headers {
		if strings.EqualFold(key, name) {
			values = append(values, items...)
		}
	}
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func acceptanceHasHeader(headers http.Header, name string) bool {
	for key := range headers {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func stopAcceptanceLocked(current *upstreamAcceptanceRuntime, reason string) {
	ctx, cancel := context.WithTimeout(context.Background(), upstreamAcceptanceWriteTimeout)
	defer cancel()
	if _, err := model.StopUpstreamAcceptanceRun(ctx, current.id, current.owner, time.Now().Unix(), reason); err != nil {
		current.drainError = true
	}
	current.cancel()
}

func drainAcceptanceLocked(current *upstreamAcceptanceRuntime) {
	if !current.stopping || current.drainError || len(current.operations) != 0 || current.httpInFlight != 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), upstreamAcceptanceWriteTimeout)
	defer cancel()
	if _, err := model.CompleteUpstreamAcceptanceStop(ctx, current.id, current.owner, time.Now().Unix()); err != nil {
		current.drainError = true
	}
}
