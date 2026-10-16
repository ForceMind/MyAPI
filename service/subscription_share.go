package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
)

// PrepareSubscriptionShareDispatch records one possibly-sent attempt, rather
// than manufacturing a per-request native debit. It runs at the final HTTP
// boundary; a later transport error must not erase its approximate weight.
func PrepareSubscriptionShareDispatch(ctx context.Context, client *http.Client, request *http.Request, info *relaycommon.RelayInfo) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if info == nil || info.ChannelMeta == nil {
		return "", model.ErrSubscriptionShareUnavailable
	}
	if info.IsChannelTest {
		// Administrator channel diagnostics have no local Key allocation.
		// Their account activity remains an attribution uncertainty.
		return "", nil
	}
	bound, err := model.HasSubscriptionShareBinding(ctx, model.DB, info.TokenId)
	if err != nil {
		return "", err
	}
	if info.ChannelType != constant.ChannelTypeCodex {
		if bound {
			return "", model.ErrSubscriptionShareDenied
		}
		return "", nil
	}
	hasPools, err := model.HasSubscriptionSharePools(ctx, model.DB)
	if err != nil || !hasPools && !bound {
		return "", err
	}
	var credential struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	}
	if common.UnmarshalJsonStr(info.ApiKey, &credential) != nil || strings.TrimSpace(credential.AccountID) == "" || strings.TrimSpace(credential.AccessToken) == "" {
		return "", model.ErrSubscriptionShareUnavailable
	}
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	if err != nil {
		return "", model.ErrSubscriptionShareUnavailable
	}
	provider := "channel_type_" + strconv.Itoa(constant.ChannelTypeCodex)
	account, found, err := model.LookupChannelQuotaIdentity(ctx, model.DB, keyring, provider, common.ChannelQuotaIdentityKindProviderAccount, []byte(strings.TrimSpace(credential.AccountID)))
	if err != nil {
		return "", model.ErrSubscriptionShareUnavailable
	}
	if !found && !bound {
		// A managed pool can only reference a registered provider account.
		// An unrelated, unregistered account retains ordinary relay behavior.
		return "", nil
	}
	if !found || account.Quality != model.ChannelQuotaIdentityQualityProviderConfirmed {
		return "", model.ErrSubscriptionShareUnavailable
	}
	required, err := model.SubscriptionShareDispatchRequired(ctx, model.DB, info.TokenId, account.SubjectRef)
	if err != nil || !required {
		return "", err
	}
	if client == nil || client.Jar != nil || common.TLSInsecureSkipVerify || !verifiedUpstreamTransport(client.Transport, "chatgpt.com") || request == nil || request.URL == nil || request.Method != http.MethodPost || request.URL.Scheme != "https" || request.URL.Host != "chatgpt.com" || request.URL.User != nil || request.URL.RawQuery != "" || request.Host != "" && request.Host != "chatgpt.com" || request.Header.Get("Cookie") != "" || len(info.HeadersOverride) != 0 || len(info.ParamOverride) != 0 {
		return "", model.ErrSubscriptionShareUnavailable
	}
	switch request.URL.EscapedPath() {
	case "/backend-api/codex/responses", "/backend-api/codex/responses/compact", "/backend-api/codex/alpha/search":
	default:
		return "", model.ErrSubscriptionShareDenied
	}
	if len(request.Header.Values("Authorization")) != 1 || len(request.Header.Values("ChatGPT-Account-ID")) != 1 || request.Header.Get("Authorization") != "Bearer "+strings.TrimSpace(credential.AccessToken) || request.Header.Get("ChatGPT-Account-ID") != strings.TrimSpace(credential.AccountID) {
		return "", model.ErrSubscriptionShareUnavailable
	}
	identity, found, err := model.LookupChannelQuotaIdentity(ctx, model.DB, keyring, provider, common.ChannelQuotaIdentityKindCredential, []byte(info.ApiKey))
	if err != nil {
		return "", model.ErrSubscriptionShareUnavailable
	}
	if !found {
		// Refreshing an existing OAuth credential does not change its account
		// pool or require a new login. Register only its opaque local identity.
		identity, err = model.ResolveChannelQuotaIdentity(ctx, model.DB, keyring, provider, common.ChannelQuotaIdentityKindCredential, []byte(info.ApiKey))
		if err != nil {
			return "", model.ErrSubscriptionShareUnavailable
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	now, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s:%s", info.RequestId, info.RetryIndex, info.ChannelId, account.SubjectRef, request.URL.EscapedPath())))
	attemptID := hex.EncodeToString(digest[:])
	if err := model.AdmitSubscriptionShareAttempt(ctx, model.DB, info.TokenId, info.UserId, account.SubjectRef, info.ChannelId, identity.SubjectRef, info.RequestId, attemptID, now); err != nil {
		return "", err
	}
	// A persisted dispatching attempt includes cancellation or transport
	// ambiguity after this point. Disable automatic body replay for this send.
	request.GetBody = nil
	return attemptID, nil
}

// Only a caller that has not invoked the transport may exclude an attempt.
func CancelSubscriptionShareUnsent(ctx context.Context, attemptID string) error {
	if attemptID == "" {
		return nil
	}
	operation, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	now, err := model.ReadDatabaseUnixTime(operation, model.DB)
	if err != nil {
		return err
	}
	return model.MarkSubscriptionShareAttemptNotSent(operation, model.DB, attemptID, now)
}

// Reuse the existing account sampler's schedule. This step only projects
// persisted observations; it never polls or refreshes upstream credentials.
func RefreshSubscriptionSharePools(ctx context.Context) error {
	now, err := model.ReadDatabaseUnixTime(ctx, model.DB)
	if err != nil {
		return err
	}
	last := ""
	for {
		var subjects []string
		if err := model.DB.WithContext(ctx).Model(&model.SubscriptionSharePool{}).Where("enabled = ? AND subject_ref > ?", true, last).Distinct("subject_ref").Order("subject_ref ASC").Limit(1000).Pluck("subject_ref", &subjects).Error; err != nil {
			return err
		}
		for _, subject := range subjects {
			if err := model.RefreshSubscriptionShareProjection(ctx, model.DB, subject, now); err != nil {
				return err
			}
		}
		if len(subjects) < 1000 {
			return nil
		}
		last = subjects[len(subjects)-1]
	}
}

func SubscriptionShareErrorDetails(err error) (int, string, string) {
	switch {
	case errors.Is(err, model.ErrSubscriptionShareInvalid):
		return http.StatusBadRequest, "subscription_share_invalid", i18n.MsgSubscriptionShareInvalid
	case errors.Is(err, model.ErrSubscriptionShareConflict):
		return http.StatusConflict, "subscription_share_conflict", i18n.MsgSubscriptionShareConflict
	case errors.Is(err, model.ErrSubscriptionShareDeleteDenied):
		return http.StatusConflict, "subscription_share_delete_denied", i18n.MsgSubscriptionShareDeleteDenied
	case errors.Is(err, model.ErrSubscriptionShareDenied):
		return http.StatusForbidden, "subscription_share_denied", i18n.MsgSubscriptionShareDenied
	case errors.Is(err, model.ErrSubscriptionShareExceeded):
		return http.StatusForbidden, "subscription_share_exceeded", i18n.MsgSubscriptionShareExceeded
	case errors.Is(err, model.ErrSubscriptionShareAwaitingSample):
		return http.StatusTooManyRequests, "subscription_share_awaiting_sample", i18n.MsgSubscriptionShareAwaitingSample
	default:
		return http.StatusServiceUnavailable, "subscription_share_unavailable", i18n.MsgSubscriptionShareUnavailable
	}
}

func SubscriptionShareRelayError(c *gin.Context, err error) *types.NewAPIError {
	status, code, message := SubscriptionShareErrorDetails(err)
	return types.NewErrorWithStatusCode(errors.New(common.TranslateMessage(c, message)), types.ErrorCode(code), status, types.ErrOptionWithSkipRetry())
}
