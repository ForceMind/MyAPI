package service

import (
	"context"
	"crypto/sha256"
	"net/http"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
)

type relayFailoverContextKey struct{}

// RelayFailover is request-local evidence, not a second scheduler or ledger.
// Credentials never enter logs; their digests only prevent retrying the same
// credential after a definite refusal, including after key reordering.
type RelayFailover struct {
	clientContext    context.Context
	cancel           context.CancelFunc
	Target           string
	excluded         map[int]map[[32]byte]bool
	Attempts         []RelayAttempt `json:"attempts"`
	ResponseRefused  bool
	DispatchPossible bool
}

type RelayAttempt struct {
	CooldownSeconds int    `json:"cooldown_seconds,omitempty"`
	ChannelID       int    `json:"channel_id"`
	KeyIndex        int    `json:"key_index"`
	Target          string `json:"upstream_model"`
	Outcome         string `json:"outcome"`
	Status          int    `json:"status,omitempty"`
}

func BeginRelayFailover(c *gin.Context) *RelayFailover {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return nil
	}
	if c.Request.URL.Path != "/v1/chat/completions" && c.Request.URL.Path != "/v1/responses" {
		return nil
	}
	raw, _ := c.Get("model_route")
	route, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	target, _ := route["upstream_model"].(string)
	if target == "" {
		return nil
	}
	state := &RelayFailover{clientContext: c.Request.Context(), Target: target, excluded: make(map[int]map[[32]byte]bool)}
	ctx := c.Request.Context()
	if common.RelayFailoverTimeoutSeconds > 0 && common.RelayFailoverTimeoutSeconds <= 3600 {
		ctx, state.cancel = context.WithTimeout(ctx, time.Duration(common.RelayFailoverTimeoutSeconds)*time.Second)
	}
	c.Request = c.Request.WithContext(context.WithValue(ctx, relayFailoverContextKey{}, state))
	return state
}

func RelayFailoverFromContext(ctx context.Context) *RelayFailover {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(relayFailoverContextKey{}).(*RelayFailover)
	return state
}

// RelayFailoverEligibleKeys intersects previous definite failures with the
// existing quota/window exclusions. Replacing a credential retires its old
// request-local failure; moving it to another index does not.
func RelayFailoverEligibleKeys(ctx context.Context, channel *model.Channel, excluded map[int]bool) (map[int]bool, bool) {
	state := RelayFailoverFromContext(ctx)
	if state == nil {
		return excluded, true
	}
	result := make(map[int]bool, len(excluded))
	for index, blocked := range excluded {
		result[index] = blocked
	}
	keys := []string{channel.Key}
	if channel.ChannelInfo.IsMultiKey {
		keys = channel.GetKeys()
	}
	eligible := false
	for index, key := range keys {
		if state.excluded[channel.Id][sha256.Sum256([]byte(key))] {
			result[index] = true
		}
		status, hasStatus := channel.ChannelInfo.MultiKeyStatusList[index]
		if !result[index] && key != "" && (!channel.ChannelInfo.IsMultiKey || !hasStatus || status == common.ChannelStatusEnabled) {
			eligible = true
		}
	}
	return result, eligible
}

func (state *RelayFailover) StartAttempt(c *gin.Context) {
	if state == nil {
		return
	}
	state.ResponseRefused = false
	state.Attempts = append(state.Attempts, RelayAttempt{ChannelID: c.GetInt("channel_id"), KeyIndex: common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex), Target: state.Target, Outcome: "selected"})
}

func (state *RelayFailover) FinishAttempt(c *gin.Context, outcome string, status int, exclude bool) {
	if state == nil || len(state.Attempts) == 0 {
		return
	}
	attempt := &state.Attempts[len(state.Attempts)-1]
	attempt.Outcome, attempt.Status = outcome, status
	if !exclude {
		return
	}
	if state.excluded[attempt.ChannelID] == nil {
		state.excluded[attempt.ChannelID] = make(map[[32]byte]bool)
	}
	state.excluded[attempt.ChannelID][sha256.Sum256([]byte(common.GetContextKeyString(c, constant.ContextKeyChannelKey)))] = true
}

// ObserveRelayFailoverResponse uses only the raw transport status. A 5xx,
// redirect, parse failure or missing response cannot prove no generation took
// place. Generic 429 permits bounded failover but is not an exhaustion marker.
func ObserveRelayFailoverResponse(c *gin.Context, status int) {
	state := RelayFailoverFromContext(c.Request.Context())
	if state == nil {
		return
	}
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusRequestEntityTooLarge, http.StatusRequestURITooLong, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		state.DispatchPossible = false
		state.ResponseRefused = true
	}
}

func AppendRelayFailoverAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || c.Request == nil || adminInfo == nil {
		return
	}
	state := RelayFailoverFromContext(c.Request.Context())
	if state == nil || len(state.Attempts) == 0 {
		return
	}
	// Copy the evidence at this log boundary; later controller decisions must
	// not mutate a payload already handed to the log writer.
	adminInfo["relay_attempts"] = append([]RelayAttempt(nil), state.Attempts...)
}

func (state *RelayFailover) Close() {
	if state != nil && state.cancel != nil {
		state.cancel()
	}
}
