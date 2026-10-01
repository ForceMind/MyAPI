package service

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/google/uuid"
)

// IsCodexUsageLimitError intentionally distinguishes account exhaustion from
// a generic 429 caused by short-term request throttling.
func IsCodexUsageLimitError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	upstreamStatus := err.UpstreamStatusCode
	if upstreamStatus == 0 {
		upstreamStatus = err.StatusCode
	}
	if upstreamStatus != http.StatusTooManyRequests {
		return false
	}
	return strings.EqualFold(string(err.GetErrorCode()), model.CodexQuotaRouteLimitCode) ||
		strings.Contains(strings.ToLower(err.Error()), "the usage limit has been reached")
}

// RecordCodexUsageLimit stores only an opaque account identity and a bounded
// routing hold. A known WHAM reset governs release; when only the 429 is known,
// ordinary traffic remains held until a later healthy provider sample.
// Administrative diagnostics and WHAM sampling can still probe recovery.
func RecordCodexUsageLimit(ctx context.Context, channelID int, credential string) error {
	if model.DB == nil || channelID <= 0 {
		return ErrCodexQuotaRoutingUnavailable
	}
	var key struct {
		AccountID string `json:"account_id"`
	}
	if err := common.UnmarshalJsonStr(strings.TrimSpace(credential), &key); err != nil || strings.TrimSpace(key.AccountID) == "" {
		return ErrCodexQuotaRoutingUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	accountID := strings.TrimSpace(key.AccountID)
	legacyRef := model.ChannelQuotaAccountRef("codex", accountID)
	subjectRef := ""
	if os.Getenv(common.ChannelQuotaIdentityKeysEnv) != "" {
		keyring, err := common.LoadChannelQuotaIdentityKeyring()
		if err != nil {
			return err
		}
		identity, err := model.ResolveChannelQuotaIdentity(persistCtx, model.DB, keyring,
			"channel_type_"+strconv.Itoa(constant.ChannelTypeCodex), common.ChannelQuotaIdentityKindProviderAccount, []byte(accountID))
		if err != nil {
			return err
		}
		subjectRef = identity.SubjectRef
	}
	now, err := model.ReadDatabaseUnixTime(persistCtx, model.DB)
	if err != nil {
		return err
	}
	state, err := model.ReadCodexQuotaRouteState(persistCtx, model.DB, subjectRef, legacyRef, now)
	if err != nil {
		return err
	}
	resetAt := int64(0)
	if state.Blocked && state.ResetAt > now {
		resetAt = state.ResetAt
	}
	snapshot := model.ChannelQuotaSnapshot{
		ChannelId: channelID, ObservedAt: now, SampleID: uuid.NewString(),
		MetricType: "codex_rate_limit", WindowType: "none", Unit: "percent",
		Source: model.CodexQuotaRouteLimitSource, Status: "error", ErrorCode: model.CodexQuotaRouteLimitCode,
		ErrorMessage: "provider reported Codex usage limit", ResetAt: resetAt,
	}
	if subjectRef != "" {
		snapshot.SubjectRef = subjectRef
		snapshot.IdentityQuality = model.ChannelQuotaIdentityQualityProviderConfirmed
	} else {
		snapshot.AccountRef = legacyRef
	}
	if err := model.RecordChannelQuotaSnapshotWithContext(persistCtx, &snapshot); err != nil {
		return err
	}
	if snapshot.Id <= 0 {
		return errors.New("Codex usage-limit marker was not persisted")
	}
	return nil
}
