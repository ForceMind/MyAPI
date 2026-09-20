package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/bytedance/gopkg/util/gopool"
)

const (
	channelQuotaAlertDeliveryBatchSize = 50
	channelQuotaAlertDeliveryLease     = 30 * time.Second
	channelQuotaAlertDeliveryTimeout   = 5 * time.Second
	channelQuotaAlertDeliveryInterval  = 15 * time.Second
)

var (
	ErrChannelQuotaAlertDeliveryDisabled     = errors.New("channel quota alert delivery is disabled")
	ErrChannelQuotaAlertDeliveryInvalidInput = errors.New("invalid channel quota alert delivery input")
	errChannelQuotaAlertRedirectBlocked      = errors.New("channel quota alert webhook redirects are disabled")
	channelQuotaAlertDeliveryWorkerOnce      sync.Once
)

type channelQuotaAlertHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type ChannelQuotaAlertDeliverySummary struct {
	Enabled     bool `json:"enabled"`
	Claimed     int  `json:"claimed"`
	Delivered   int  `json:"delivered"`
	Retryable   int  `json:"retryable"`
	Quarantined int  `json:"quarantined"`
}

type ChannelQuotaAlertDeliveryStatus struct {
	PolicyEnabled bool   `json:"policy_enabled"`
	Configured    bool   `json:"configured"`
	EndpointHost  string `json:"endpoint_host,omitempty"`
	HTTPSOnly     bool   `json:"https_only"`
	Redirects     bool   `json:"redirects_allowed"`
	TimeoutMillis int64  `json:"timeout_ms"`
	MaxAttempts   int    `json:"max_attempts"`
}

type ChannelQuotaAlertDeliveryEvent struct {
	ID            int64  `json:"id"`
	EventKey      string `json:"event_key"`
	SnapshotID    int    `json:"snapshot_id"`
	ChannelID     int    `json:"channel_id"`
	Status        string `json:"status"`
	Kind          string `json:"kind"`
	State         string `json:"state"`
	AttemptCount  int    `json:"attempt_count"`
	NextAttemptAt int64  `json:"next_attempt_at,omitempty"`
	LastErrorCode string `json:"last_error_code,omitempty"`
	LastErrorAt   int64  `json:"last_error_at,omitempty"`
	DeliveredAt   *int64 `json:"delivered_at,omitempty"`
	ObservedAt    int64  `json:"observed_at"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

type ChannelQuotaAlertDeliveryWorker struct {
	workerID      string
	client        channelQuotaAlertHTTPDoer
	settings      func() (setting.ChannelQuotaAlertDeliverySettings, error)
	now           func() time.Time
	batchSize     int
	policyEnabled func() bool
}

type channelQuotaAlertWebhookPayload struct {
	Version       string  `json:"version"`
	EventID       string  `json:"event_id"`
	ChannelID     int     `json:"channel_id"`
	SnapshotID    int     `json:"snapshot_id"`
	Status        string  `json:"status"`
	Kind          string  `json:"kind"`
	ObservedAt    int64   `json:"observed_at"`
	Available     float64 `json:"available"`
	Total         float64 `json:"total"`
	Unit          string  `json:"unit"`
	Currency      string  `json:"currency,omitempty"`
	MetricType    string  `json:"metric_type"`
	WindowType    string  `json:"window_type"`
	PlanType      string  `json:"plan_type,omitempty"`
	WindowSeconds int64   `json:"window_seconds,omitempty"`
	ResetAt       int64   `json:"reset_at,omitempty"`
	Source        string  `json:"source"`
}

func NewChannelQuotaAlertDeliveryWorker(workerID string) *ChannelQuotaAlertDeliveryWorker {
	return &ChannelQuotaAlertDeliveryWorker{
		workerID:      strings.TrimSpace(workerID),
		client:        newChannelQuotaAlertWebhookHTTPClient(),
		settings:      setting.GetChannelQuotaAlertDeliverySettings,
		now:           time.Now,
		batchSize:     channelQuotaAlertDeliveryBatchSize,
		policyEnabled: channelQuotaAlertPolicyEnabled,
	}
}

func newChannelQuotaAlertWebhookHTTPClient() *http.Client {
	return newChannelQuotaAlertWebhookHTTPClientWithDialer(nil, nil)
}

func newChannelQuotaAlertWebhookHTTPClientWithDialer(resolver ssrfResolver, dialContext func(ctx context.Context, network, address string) (net.Conn, error)) *http.Client {
	strictProtection := &common.SSRFProtection{
		AllowPrivateIp:         false,
		DomainFilterMode:       false,
		IpFilterMode:           false,
		ApplyIPFilterForDomain: true,
	}
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialContext == nil {
		netDialer := &net.Dialer{Timeout: channelQuotaAlertDeliveryTimeout, KeepAlive: 30 * time.Second}
		dialContext = netDialer.DialContext
	}
	protectedDialer := &protectedFetchDialer{
		resolver:    resolver,
		dialContext: dialContext,
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return strictProtection, true, nil
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           protectedDialer.DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   channelQuotaAlertDeliveryTimeout,
		ResponseHeaderTimeout: channelQuotaAlertDeliveryTimeout,
		ExpectContinueTimeout: time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   channelQuotaAlertDeliveryTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errChannelQuotaAlertRedirectBlocked
		},
	}
}

func GetChannelQuotaAlertDeliveryStatus() (ChannelQuotaAlertDeliveryStatus, error) {
	settingsValue, err := setting.GetChannelQuotaAlertDeliverySettings()
	status := ChannelQuotaAlertDeliveryStatus{
		PolicyEnabled: channelQuotaAlertPolicyEnabled(),
		HTTPSOnly:     true,
		Redirects:     false,
		TimeoutMillis: channelQuotaAlertDeliveryTimeout.Milliseconds(),
		MaxAttempts:   model.ChannelQuotaAlertMaxAttempts(),
	}
	if err != nil {
		return status, err
	}
	status.Configured = settingsValue.Configured()
	if status.Configured {
		status.EndpointHost = settingsValue.EndpointHost()
	}
	return status, nil
}

func channelQuotaAlertPolicyEnabled() bool {
	common.OptionMapRWMutex.Lock()
	enabled := common.ChannelQuotaAlertEnabled
	common.OptionMapRWMutex.Unlock()
	return enabled
}

// UpdateChannelQuotaAlertDeliverySettings persists URL and signing secret as
// one hidden option. Empty values disable delivery and clear both fields.
func UpdateChannelQuotaAlertDeliverySettings(webhookURL, webhookSecret string) (ChannelQuotaAlertDeliveryStatus, error) {
	settingsValue := setting.ChannelQuotaAlertDeliverySettings{
		WebhookURL:    strings.TrimSpace(webhookURL),
		WebhookSecret: webhookSecret,
	}
	if settingsValue.WebhookURL == "" && settingsValue.WebhookSecret == "" {
		// Explicit clear is valid and keeps already-created events pending.
	} else if err := setting.ValidateChannelQuotaAlertDeliverySettings(settingsValue); err != nil {
		return ChannelQuotaAlertDeliveryStatus{}, fmt.Errorf("%w: %v", ErrChannelQuotaAlertDeliveryInvalidInput, err)
	}
	raw, err := setting.MarshalChannelQuotaAlertDeliverySettings(settingsValue)
	if err != nil {
		return ChannelQuotaAlertDeliveryStatus{}, fmt.Errorf("%w: %v", ErrChannelQuotaAlertDeliveryInvalidInput, err)
	}
	if err := model.UpdateOption(setting.ChannelQuotaAlertDeliveryOptionKey, raw); err != nil {
		return ChannelQuotaAlertDeliveryStatus{}, err
	}
	return GetChannelQuotaAlertDeliveryStatus()
}

func ListChannelQuotaAlertDeliveryEvents(ctx context.Context, filter model.ChannelQuotaAlertEventFilter, offset, limit int) ([]ChannelQuotaAlertDeliveryEvent, int64, error) {
	events, total, err := model.ListChannelQuotaAlertEvents(ctx, filter, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ChannelQuotaAlertDeliveryEvent, 0, len(events))
	for _, event := range events {
		items = append(items, ChannelQuotaAlertDeliveryEvent{
			ID: event.ID, EventKey: event.EventKey, SnapshotID: event.SnapshotID, ChannelID: event.ChannelID,
			Status: event.Status, Kind: event.Kind, State: event.State, AttemptCount: event.AttemptCount,
			NextAttemptAt: event.NextAttemptAt, LastErrorCode: event.LastErrorCode, LastErrorAt: event.LastErrorAt,
			DeliveredAt: event.DeliveredAt, ObservedAt: event.ObservedAt, CreatedAt: event.CreatedAt, UpdatedAt: event.UpdatedAt,
		})
	}
	return items, total, nil
}

func RunChannelQuotaAlertDeliveryPass(ctx context.Context, workerID string) (ChannelQuotaAlertDeliverySummary, error) {
	return NewChannelQuotaAlertDeliveryWorker(workerID).RunOnce(ctx)
}

func (worker *ChannelQuotaAlertDeliveryWorker) RunOnce(ctx context.Context) (ChannelQuotaAlertDeliverySummary, error) {
	summary := ChannelQuotaAlertDeliverySummary{}
	if worker == nil || strings.TrimSpace(worker.workerID) == "" || worker.client == nil || worker.settings == nil || worker.now == nil || worker.policyEnabled == nil || worker.batchSize <= 0 || worker.batchSize > 100 {
		return summary, ErrChannelQuotaAlertDeliveryInvalidInput
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !worker.policyEnabled() {
		return summary, nil
	}
	settingsValue, err := worker.settings()
	if err != nil {
		return summary, fmt.Errorf("%w: %v", ErrChannelQuotaAlertDeliveryDisabled, err)
	}
	if !settingsValue.Configured() {
		return summary, nil
	}
	if err := setting.ValidateChannelQuotaAlertDeliverySettings(settingsValue); err != nil {
		return summary, fmt.Errorf("%w: %v", ErrChannelQuotaAlertDeliveryDisabled, err)
	}
	summary.Enabled = true
	// Claim exactly one event immediately before its HTTP request. A batch lease
	// cannot safely cover a sequential run: at the five-second request timeout,
	// a 50-item batch would let later events expire before this worker starts
	// sending them. Per-event claims preserve the bounded lease while a second
	// worker can only take work this worker has not begun.
	for range worker.batchSize {
		if err := ctx.Err(); err != nil {
			return summary, err
		}
		now := worker.now().Unix()
		events, err := model.ClaimChannelQuotaAlertEvents(ctx, worker.workerID, now, int64(channelQuotaAlertDeliveryLease/time.Second), 1)
		if err != nil {
			return summary, err
		}
		if len(events) == 0 {
			break
		}
		event := events[0]
		summary.Claimed++
		errorCode := worker.deliver(ctx, settingsValue, event)
		transitionAt := worker.now().Unix()
		if errorCode == "" {
			won, markErr := model.MarkChannelQuotaAlertDelivered(ctx, event, worker.workerID, transitionAt)
			if markErr != nil {
				return summary, markErr
			}
			if !won {
				return summary, fmt.Errorf("channel quota alert delivery claim lost: event=%d", event.ID)
			}
			summary.Delivered++
			continue
		}
		won, markErr := model.MarkChannelQuotaAlertFailed(ctx, event, worker.workerID, errorCode, transitionAt)
		if markErr != nil {
			return summary, markErr
		}
		if !won {
			return summary, fmt.Errorf("channel quota alert failure claim lost: event=%d", event.ID)
		}
		if model.ChannelQuotaAlertFailureQuarantines(event) {
			summary.Quarantined++
		} else {
			summary.Retryable++
		}
	}
	return summary, nil
}

func (worker *ChannelQuotaAlertDeliveryWorker) deliver(ctx context.Context, settingsValue setting.ChannelQuotaAlertDeliverySettings, event model.ChannelQuotaAlertEvent) string {
	snapshot, err := model.GetChannelQuotaAlertDeliverySnapshot(ctx, event)
	if err != nil || snapshot.Total == nil {
		return "source_unavailable"
	}
	payload := channelQuotaAlertWebhookPayload{
		Version: "channel-quota-alert-v1", EventID: event.EventKey, ChannelID: event.ChannelID,
		SnapshotID: event.SnapshotID, Status: event.Status, Kind: event.Kind, ObservedAt: event.ObservedAt,
		Available: snapshot.Available, Total: *snapshot.Total, Unit: snapshot.Unit, Currency: snapshot.Currency,
		MetricType: snapshot.MetricType, WindowType: snapshot.WindowType, PlanType: snapshot.PlanType,
		WindowSeconds: snapshot.WindowSeconds, ResetAt: snapshot.ResetAt, Source: snapshot.Source,
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return "payload_invalid"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settingsValue.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return "request_invalid"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MyAPI-Channel-Quota-Alert/1.0")
	req.Header.Set("X-MyAPI-Event-ID", event.EventKey)
	req.Header.Set("X-MyAPI-Signature", channelQuotaAlertSignature(settingsValue.WebhookSecret, body))
	response, err := worker.client.Do(req)
	if err != nil {
		return channelQuotaAlertDeliveryErrorCode(err)
	}
	if response == nil {
		return "empty_response"
	}
	if response.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return ""
	}
	if response.StatusCode < 100 || response.StatusCode > 999 {
		return "http_invalid"
	}
	return "http_" + strconv.Itoa(response.StatusCode)
}

func channelQuotaAlertSignature(secret string, body []byte) string {
	digest := hmac.New(sha256.New, []byte(secret))
	_, _ = digest.Write(body)
	return "sha256=" + hex.EncodeToString(digest.Sum(nil))
}

func channelQuotaAlertDeliveryErrorCode(err error) string {
	switch {
	case errors.Is(err, errChannelQuotaAlertRedirectBlocked):
		return "redirect_blocked"
	case errors.Is(err, context.DeadlineExceeded):
		return "request_timeout"
	case errors.Is(err, context.Canceled):
		return "request_canceled"
	default:
		var urlErr *url.Error
		if errors.As(err, &urlErr) && errors.Is(urlErr.Err, errChannelQuotaAlertRedirectBlocked) {
			return "redirect_blocked"
		}
		return "network_error"
	}
}

// StartChannelQuotaAlertDeliveryWorker starts the master process loop. Claims
// remain database-CAS protected if more than one master is briefly active.
func StartChannelQuotaAlertDeliveryWorker() {
	channelQuotaAlertDeliveryWorkerOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		workerID := "quota-alert-" + common.NodeName + "-" + common.GetRandomString(8)
		worker := NewChannelQuotaAlertDeliveryWorker(workerID)
		gopool.Go(func() {
			run := func() {
				summary, err := worker.RunOnce(context.Background())
				if err != nil && !errors.Is(err, ErrChannelQuotaAlertDeliveryDisabled) {
					logger.LogWarn(context.Background(), fmt.Sprintf("channel quota alert delivery pass failed: %v", err))
					return
				}
				if summary.Delivered > 0 || summary.Retryable > 0 || summary.Quarantined > 0 {
					logger.LogInfo(context.Background(), fmt.Sprintf("channel quota alert delivery pass: claimed=%d delivered=%d retryable=%d quarantined=%d", summary.Claimed, summary.Delivered, summary.Retryable, summary.Quarantined))
				}
			}
			run()
			ticker := time.NewTicker(channelQuotaAlertDeliveryInterval)
			defer ticker.Stop()
			for range ticker.C {
				run()
			}
		})
	})
}
