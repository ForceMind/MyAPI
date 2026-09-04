package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
)

const (
	codexCredentialPersistenceTimeout  = 5 * time.Second
	codexCredentialLeaseDuration       = 60 * time.Second
	codexCredentialLeaseWaitTimeout    = 5 * time.Second
	codexCredentialLeasePollInterval   = 50 * time.Millisecond
	codexCredentialLeaseReleaseTimeout = 5 * time.Second
)

type codexCredentialRefreshLease struct {
	lockType string
	taskID   string
	owner    string
}

type CodexCredentialRefreshOptions struct {
	// ResetCaches is retained for caller compatibility. Credential persistence
	// now updates the affected channel cache entry directly, so a full cache
	// rebuild is neither required nor performed here.
	ResetCaches bool
	// ExpectedKey identifies the credential snapshot that produced an upstream
	// authentication failure. If another operation already replaced it before
	// this refresh acquires the local gate and distributed lease, the latest
	// persisted credential is returned without another token rotation.
	ExpectedKey *string
}

type CodexOAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	AccountID   string `json:"account_id,omitempty"`
	LastRefresh string `json:"last_refresh,omitempty"`
	Email       string `json:"email,omitempty"`
	Type        string `json:"type,omitempty"`
	Expired     string `json:"expired,omitempty"`
}

// CodexCredentialPersistenceError identifies a failure after the upstream has
// already rotated a credential, allowing background sampling to record the
// persistence failure separately from an upstream outage.
type CodexCredentialPersistenceError struct {
	err error
}

func (err *CodexCredentialPersistenceError) Error() string {
	return "codex channel credential could not be persisted"
}

func (err *CodexCredentialPersistenceError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.err
}

func parseCodexOAuthKey(raw string) (*CodexOAuthKey, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("codex channel: empty oauth key")
	}
	var key CodexOAuthKey
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("codex channel: invalid oauth key json")
	}
	return &key, nil
}

func RefreshCodexChannelCredential(ctx context.Context, channelID int, opts CodexCredentialRefreshOptions) (*CodexOAuthKey, *model.Channel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if channelID <= 0 {
		return nil, nil, errors.New("invalid Codex channel id")
	}
	var oauthKey *CodexOAuthKey
	var channel *model.Channel
	err := model.WithChannelCredentialUpdateGate(ctx, channelID, func(gateCtx context.Context) error {
		lease, err := acquireCodexCredentialRefreshLease(gateCtx, channelID)
		if err != nil {
			return err
		}
		defer lease.release()

		current, err := model.GetChannelById(channelID, true)
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("channel not found")
		}
		if current.Type != constant.ChannelTypeCodex {
			return fmt.Errorf("channel type is not Codex")
		}

		currentKey, err := parseCodexOAuthKey(strings.TrimSpace(current.Key))
		if err != nil {
			return err
		}
		if opts.ExpectedKey != nil && current.Key != *opts.ExpectedKey {
			confirmed, err := model.UpdateChannelCredentialIfUnchanged(gateCtx, current.Id, constant.ChannelTypeCodex, current.Key, current.Key)
			if err != nil {
				return err
			}
			if !confirmed {
				return errors.New("codex channel credential changed while reusing refreshed credential")
			}
			oauthKey = currentKey
			channel = current
			return nil
		}
		if strings.TrimSpace(currentKey.RefreshToken) == "" {
			return fmt.Errorf("codex channel: refresh_token is required to refresh credential")
		}

		refreshCtx, refreshCancel := context.WithTimeout(gateCtx, 10*time.Second)
		res, err := RefreshCodexOAuthTokenWithProxy(refreshCtx, currentKey.RefreshToken, current.GetSetting().Proxy)
		refreshCancel()
		if err != nil {
			return err
		}

		currentKey.AccessToken = res.AccessToken
		currentKey.RefreshToken = res.RefreshToken
		currentKey.LastRefresh = time.Now().Format(time.RFC3339)
		currentKey.Expired = res.ExpiresAt.Format(time.RFC3339)
		if strings.TrimSpace(currentKey.Type) == "" {
			currentKey.Type = "codex"
		}
		if strings.TrimSpace(currentKey.AccountID) == "" {
			if accountID, ok := ExtractCodexAccountIDFromJWT(currentKey.AccessToken); ok {
				currentKey.AccountID = accountID
			}
		}
		if strings.TrimSpace(currentKey.Email) == "" {
			if email, ok := ExtractEmailFromJWT(currentKey.AccessToken); ok {
				currentKey.Email = email
			}
		}

		encoded, err := common.Marshal(currentKey)
		if err != nil {
			return err
		}
		// A successful OAuth refresh may rotate the only usable refresh token.
		// Preserve the held gate while replacing request cancellation with a
		// short persistence budget.
		persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(gateCtx), codexCredentialPersistenceTimeout)
		defer persistCancel()
		updated, err := model.UpdateChannelCredentialIfUnchanged(persistCtx, current.Id, constant.ChannelTypeCodex, current.Key, string(encoded))
		if err != nil {
			return &CodexCredentialPersistenceError{err: err}
		}
		if !updated {
			return &CodexCredentialPersistenceError{err: errors.New("codex channel credential changed during refresh")}
		}

		current.Key = string(encoded)
		oauthKey = currentKey
		channel = current
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return oauthKey, channel, nil
}

func acquireCodexCredentialRefreshLease(ctx context.Context, channelID int) (*codexCredentialRefreshLease, error) {
	taskID, err := model.GenerateSystemTaskID()
	if err != nil {
		return nil, err
	}
	randomOwner, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return nil, err
	}
	lease := &codexCredentialRefreshLease{
		lockType: fmt.Sprintf("codex_credential_refresh:%d", channelID),
		taskID:   taskID,
		owner:    "codex_refresh_" + randomOwner,
	}
	waitCtx, cancel := context.WithTimeout(ctx, codexCredentialLeaseWaitTimeout)
	defer cancel()
	ticker := time.NewTicker(codexCredentialLeasePollInterval)
	defer ticker.Stop()
	for {
		acquired, err := model.TryAcquireNamedSystemTaskLockWithContext(
			waitCtx,
			lease.lockType,
			lease.taskID,
			lease.owner,
			time.Now().Add(codexCredentialLeaseDuration).Unix(),
		)
		if err != nil {
			return nil, err
		}
		if acquired {
			return lease, nil
		}
		select {
		case <-waitCtx.Done():
			return nil, waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func (lease *codexCredentialRefreshLease) release() {
	if lease == nil {
		return
	}
	releaseCtx, cancel := context.WithTimeout(context.Background(), codexCredentialLeaseReleaseTimeout)
	defer cancel()
	if err := model.ReleaseNamedSystemTaskLockWithContext(releaseCtx, lease.lockType, lease.taskID, lease.owner); err != nil {
		common.SysError("failed to release Codex credential refresh lease")
	}
}
