package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
)

const (
	codexOAuthClientID     = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexOAuthAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	codexOAuthTokenURL     = "https://auth.openai.com/oauth/token"
	codexOAuthRedirectURI  = "http://localhost:1455/auth/callback"
	codexOAuthScope        = "openid profile email offline_access"
	codexJWTClaimPath      = "https://api.openai.com/auth"
	defaultHTTPTimeout     = 20 * time.Second
	// Representation bound for seconds -> nanoseconds, not a provider policy.
	maxCodexOAuthLifetimeSeconds = int64((1<<63 - 1) / time.Second)
)

var (
	ErrCodexCredentialSubjectUnknown  = errors.New("codex credential account subject is unknown")
	ErrCodexCredentialSubjectMismatch = errors.New("codex credential account subject does not match the channel")
)

type CodexOAuthTokenResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type CodexOAuthAuthorizationFlow struct {
	State        string
	Verifier     string
	Challenge    string
	AuthorizeURL string
}

// validateCodexTLSCredentialReplacement accepts the new account claim only
// because the caller received the token from the fixed provider TLS endpoint.
// The existing subject still requires persisted provider-confirmed evidence.
func validateCodexTLSCredentialReplacement(ctx context.Context, channelID int, existingRaw string, replacement *CodexOAuthKey) error {
	existingSubject, err := trustedCodexCredentialSubject(ctx, channelID, existingRaw)
	if err != nil {
		return ErrCodexCredentialSubjectUnknown
	}
	replacementAccount, ok := ExtractCodexAccountIDFromJWT(replacement.AccessToken)
	if !ok {
		return ErrCodexCredentialSubjectUnknown
	}
	if accountID := strings.TrimSpace(replacement.AccountID); accountID != "" && accountID != replacementAccount {
		return ErrCodexCredentialSubjectMismatch
	}
	replacementSubject, err := lookupCodexProviderAccountSubject(ctx, replacementAccount)
	if err != nil {
		return ErrCodexCredentialSubjectUnknown
	}
	if existingSubject != replacementSubject {
		return ErrCodexCredentialSubjectMismatch
	}
	replacement.AccountID = replacementAccount
	return nil
}

// validateCodexImportedCredentialReplacement accepts a local credential only
// when that exact credential already has independent provider-confirmed quota
// evidence. JWT claims and account_id fields in a local file are not trusted.
func validateCodexImportedCredentialReplacement(ctx context.Context, channelID int, existingRaw string, replacement *CodexOAuthKey) error {
	existingSubject, err := trustedCodexCredentialSubject(ctx, channelID, existingRaw)
	if err != nil {
		return ErrCodexCredentialSubjectUnknown
	}
	encoded, err := common.Marshal(replacement)
	if err != nil {
		return ErrCodexCredentialSubjectUnknown
	}
	replacementSubject, err := trustedCodexCredentialSubject(ctx, 0, string(encoded))
	if err != nil {
		return ErrCodexCredentialSubjectUnknown
	}
	if existingSubject != replacementSubject {
		return ErrCodexCredentialSubjectMismatch
	}
	return nil
}

func validateTrustedCodexCredentialContinuity(ctx context.Context, channelID int, expectedRaw, currentRaw string) error {
	if strings.TrimSpace(expectedRaw) == "" {
		return ErrCodexCredentialSubjectUnknown
	}
	expectedSubject, err := trustedCodexCredentialSubject(ctx, channelID, expectedRaw)
	if err != nil {
		return ErrCodexCredentialSubjectUnknown
	}
	currentSubject, err := trustedCodexCredentialSubject(ctx, channelID, currentRaw)
	if err != nil {
		return ErrCodexCredentialSubjectUnknown
	}
	if expectedSubject != currentSubject {
		return ErrCodexCredentialSubjectMismatch
	}
	return nil
}

func ReplaceCodexChannelCredentialFromOAuth(ctx context.Context, channelID int, expectedRaw string, replacement *CodexOAuthKey) (bool, error) {
	return replaceCodexChannelCredential(ctx, channelID, expectedRaw, replacement, true)
}

func ReplaceCodexChannelCredentialFromImport(ctx context.Context, channelID int, expectedRaw string, replacement *CodexOAuthKey) (bool, error) {
	return replaceCodexChannelCredential(ctx, channelID, expectedRaw, replacement, false)
}

func replaceCodexChannelCredential(ctx context.Context, channelID int, expectedRaw string, replacement *CodexOAuthKey, trustedOAuthResponse bool) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if channelID <= 0 || strings.TrimSpace(expectedRaw) == "" || replacement == nil {
		return false, errors.New("invalid Codex credential replacement")
	}
	var updated bool
	err := model.WithChannelCredentialUpdateGate(ctx, channelID, func(gateCtx context.Context) error {
		channel, err := model.GetChannelById(channelID, true)
		if err != nil {
			return err
		}
		if channel.Type != constant.ChannelTypeCodex || channel.ChannelInfo.IsMultiKey {
			return errors.New("Codex credential replacement requires a single-key Codex channel")
		}
		if channel.Key != expectedRaw {
			return nil
		}
		if trustedOAuthResponse {
			err = validateCodexTLSCredentialReplacement(gateCtx, channelID, channel.Key, replacement)
		} else {
			err = validateCodexImportedCredentialReplacement(gateCtx, channelID, channel.Key, replacement)
		}
		if err != nil {
			return err
		}
		encoded, err := common.Marshal(replacement)
		if err != nil {
			return err
		}
		oldSubject, newSubject, confirmedSubject, err := codexCredentialSubjectTransition(gateCtx, channelID, channel.Key, string(encoded))
		if err != nil {
			return err
		}
		updated, err = model.UpdateChannelCredentialAndSubjectIfUnchanged(
			gateCtx, channelID, constant.ChannelTypeCodex, channel.Key, string(encoded), oldSubject, newSubject, confirmedSubject,
		)
		if errors.Is(err, model.ErrChannelCredentialSubjectUnknown) {
			return ErrCodexCredentialSubjectUnknown
		}
		if errors.Is(err, model.ErrChannelCredentialSubjectConflict) {
			return ErrCodexCredentialSubjectMismatch
		}
		return err
	})
	return updated, err
}

func codexCredentialSubjectTransition(ctx context.Context, channelID int, oldCredential, newCredential string) (string, string, string, error) {
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	if err != nil {
		return "", "", "", err
	}
	provider := "channel_type_" + fmt.Sprintf("%d", constant.ChannelTypeCodex)
	oldIdentity, found, err := model.LookupChannelQuotaIdentity(ctx, model.DB, keyring, provider, common.ChannelQuotaIdentityKindCredential, []byte(oldCredential))
	if err != nil || !found || oldIdentity.Quality != model.ChannelQuotaIdentityQualityCredentialScoped {
		return "", "", "", ErrCodexCredentialSubjectUnknown
	}
	confirmedSubject, err := model.LookupTrustedChannelCredentialSubject(ctx, channelID, oldIdentity.SubjectRef)
	if err != nil {
		return "", "", "", ErrCodexCredentialSubjectUnknown
	}
	newIdentity, err := model.ResolveChannelQuotaIdentity(ctx, model.DB, keyring, provider, common.ChannelQuotaIdentityKindCredential, []byte(newCredential))
	if err != nil || newIdentity.Quality != model.ChannelQuotaIdentityQualityCredentialScoped {
		return "", "", "", ErrCodexCredentialSubjectUnknown
	}
	return oldIdentity.SubjectRef, newIdentity.SubjectRef, confirmedSubject, nil
}

func replaceCodexCredentialElement(channelKey string, multiKey bool, expected, replacement string) (string, error) {
	if !multiKey {
		if channelKey != expected {
			return "", errors.New("codex credential changed before refresh")
		}
		return replacement, nil
	}
	trimmed := strings.TrimSpace(channelKey)
	if strings.HasPrefix(trimmed, "[") {
		var credentials []json.RawMessage
		if err := common.Unmarshal([]byte(trimmed), &credentials); err != nil {
			return "", err
		}
		matches := 0
		for index := range credentials {
			if string(credentials[index]) == expected {
				credentials[index] = json.RawMessage(replacement)
				matches++
			}
		}
		if matches != 1 {
			return "", errors.New("codex credential target is not unique")
		}
		encoded, err := common.Marshal(credentials)
		return string(encoded), err
	}
	credentials := strings.Split(strings.Trim(channelKey, "\n"), "\n")
	matches := 0
	for index := range credentials {
		if credentials[index] == expected {
			credentials[index] = replacement
			matches++
		}
	}
	if matches != 1 {
		return "", errors.New("codex credential target is not unique")
	}
	return strings.Join(credentials, "\n"), nil
}

func trustedCodexCredentialSubject(ctx context.Context, channelID int, credential string) (string, error) {
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	if err != nil {
		return "", err
	}
	identity, found, err := model.LookupChannelQuotaIdentity(ctx, model.DB, keyring,
		"channel_type_"+fmt.Sprintf("%d", constant.ChannelTypeCodex), common.ChannelQuotaIdentityKindCredential, []byte(credential))
	if err != nil || !found || identity.Quality != model.ChannelQuotaIdentityQualityCredentialScoped {
		return "", ErrCodexCredentialSubjectUnknown
	}
	return model.LookupTrustedChannelCredentialSubject(ctx, channelID, identity.SubjectRef)
}

func lookupCodexProviderAccountSubject(ctx context.Context, accountID string) (string, error) {
	keyring, err := common.LoadChannelQuotaIdentityKeyring()
	if err != nil {
		return "", err
	}
	identity, found, err := model.LookupChannelQuotaIdentity(ctx, model.DB, keyring,
		"channel_type_"+fmt.Sprintf("%d", constant.ChannelTypeCodex), common.ChannelQuotaIdentityKindProviderAccount, []byte(accountID))
	if err != nil || !found || identity.Quality != model.ChannelQuotaIdentityQualityProviderConfirmed {
		return "", ErrCodexCredentialSubjectUnknown
	}
	return identity.SubjectRef, nil
}

func RefreshCodexOAuthToken(ctx context.Context, refreshToken string) (*CodexOAuthTokenResult, error) {
	return RefreshCodexOAuthTokenWithProxy(ctx, refreshToken, "")
}

func RefreshCodexOAuthTokenWithProxy(ctx context.Context, refreshToken string, proxyURL string) (*CodexOAuthTokenResult, error) {
	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	return refreshCodexOAuthToken(ctx, client, codexOAuthTokenURL, codexOAuthClientID, refreshToken)
}

// RefreshCodexChannelCredentialElement rotates one expected credential without
// replacing a channel's other keys. The credential gate and distributed lease
// serialize refresh-token use; stale replacement is rejected without overwriting
// other credentials. Reordering preserves the matching credential's identity.
func RefreshCodexChannelCredentialElement(ctx context.Context, channelID int, expectedCredential, proxyURL string) (*CodexOAuthKey, error) {
	if channelID <= 0 || strings.TrimSpace(expectedCredential) == "" {
		return nil, errors.New("invalid Codex channel credential target")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, finish, err := BeginUpstreamAcceptanceChannelOperation(ctx, channelID)
	if err != nil {
		return nil, err
	}
	defer finish()
	var refreshed *CodexOAuthKey
	err = model.WithChannelCredentialUpdateGate(ctx, channelID, func(gateCtx context.Context) error {
		lease, err := acquireCodexCredentialRefreshLease(gateCtx, channelID)
		if err != nil {
			return err
		}
		defer lease.release()
		channel, err := model.GetChannelById(channelID, true)
		if err != nil {
			return err
		}
		if channel.Type != constant.ChannelTypeCodex {
			return errors.New("channel type is not Codex")
		}
		matches := 0
		credentials := []string{channel.Key}
		if channel.ChannelInfo.IsMultiKey {
			credentials = channel.GetKeys()
		}
		for _, candidate := range credentials {
			if candidate == expectedCredential {
				matches++
			}
		}
		if matches == 0 {
			return errors.New("codex credential changed before refresh")
		}
		if matches != 1 {
			return errors.New("duplicate Codex credential target")
		}
		key, err := parseCodexOAuthKey(expectedCredential)
		if err != nil {
			return err
		}
		if _, err := trustedCodexCredentialSubject(gateCtx, channelID, expectedCredential); err != nil {
			return ErrCodexCredentialSubjectUnknown
		}
		if strings.TrimSpace(key.RefreshToken) == "" {
			return errors.New("codex channel: refresh_token is required to refresh credential")
		}
		refreshCtx, cancel := context.WithTimeout(gateCtx, 10*time.Second)
		result, err := RefreshCodexOAuthTokenWithProxy(refreshCtx, key.RefreshToken, proxyURL)
		cancel()
		if err != nil {
			return err
		}
		// The provider may rotate the only usable refresh token before the
		// request context is canceled. Finish validation and persistence with a
		// bounded context that keeps the channel gate lease.
		persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(gateCtx), codexCredentialPersistenceTimeout)
		defer persistCancel()
		replacement := &CodexOAuthKey{AccessToken: result.AccessToken}
		if err := validateCodexTLSCredentialReplacement(persistCtx, channelID, expectedCredential, replacement); err != nil {
			// The upstream may already have rotated the only usable refresh token.
			// Do not replay it or persist a credential for another/unknown account.
			return &CodexCredentialPersistenceError{err: err}
		}
		key.AccessToken, key.RefreshToken = result.AccessToken, result.RefreshToken
		key.AccountID = replacement.AccountID
		key.LastRefresh = time.Now().Format(time.RFC3339)
		key.Expired = result.ExpiresAt.Format(time.RFC3339)
		encoded, err := common.Marshal(key)
		if err != nil {
			return err
		}
		newChannelKey, err := replaceCodexCredentialElement(channel.Key, channel.ChannelInfo.IsMultiKey, expectedCredential, string(encoded))
		if err != nil {
			return &CodexCredentialPersistenceError{err: err}
		}
		oldSubject, newSubject, confirmedSubject, err := codexCredentialSubjectTransition(persistCtx, channelID, expectedCredential, string(encoded))
		if err != nil {
			return &CodexCredentialPersistenceError{err: err}
		}
		updated, err := model.UpdateChannelCredentialAndSubjectIfUnchanged(
			persistCtx, channelID, constant.ChannelTypeCodex, channel.Key, newChannelKey, oldSubject, newSubject, confirmedSubject,
		)
		if err != nil {
			return &CodexCredentialPersistenceError{err: err}
		}
		if !updated {
			return &CodexCredentialPersistenceError{err: errors.New("codex credential changed during refresh")}
		}
		refreshed = key
		return nil
	})
	return refreshed, err
}

func ExchangeCodexAuthorizationCode(ctx context.Context, code string, verifier string) (*CodexOAuthTokenResult, error) {
	return ExchangeCodexAuthorizationCodeWithProxy(ctx, code, verifier, "")
}

func ExchangeCodexAuthorizationCodeWithProxy(ctx context.Context, code string, verifier string, proxyURL string) (*CodexOAuthTokenResult, error) {
	client, err := getCodexOAuthHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}
	return exchangeCodexAuthorizationCode(ctx, client, codexOAuthTokenURL, codexOAuthClientID, code, verifier, codexOAuthRedirectURI)
}

func CreateCodexOAuthAuthorizationFlow() (*CodexOAuthAuthorizationFlow, error) {
	state, err := createStateHex(16)
	if err != nil {
		return nil, err
	}
	verifier, challenge, err := generatePKCEPair()
	if err != nil {
		return nil, err
	}
	authorizeURL, err := buildCodexAuthorizeURL(state, challenge)
	if err != nil {
		return nil, err
	}
	return &CodexOAuthAuthorizationFlow{
		State:        state,
		Verifier:     verifier,
		Challenge:    challenge,
		AuthorizeURL: authorizeURL,
	}, nil
}

// BuildCodexOAuthAuthorizeURL builds the browser URL for a caller-owned state
// token. This lets the dashboard bind PKCE state to its server-side AuthFlow
// record instead of storing the verifier in a browser cookie.
func BuildCodexOAuthAuthorizeURL(state string, challenge string) (string, error) {
	if strings.TrimSpace(state) == "" || strings.TrimSpace(challenge) == "" {
		return "", errors.New("codex oauth state and challenge are required")
	}
	return buildCodexAuthorizeURL(state, challenge)
}

func refreshCodexOAuthToken(
	ctx context.Context,
	client *http.Client,
	tokenURL string,
	clientID string,
	refreshToken string,
) (*CodexOAuthTokenResult, error) {
	rt := strings.TrimSpace(refreshToken)
	if rt == "" {
		return nil, errors.New("empty refresh_token")
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", rt)
	form.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := DoUpstreamAcceptanceHTTP(client, req, model.UpstreamAcceptanceDispatchRefresh)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}

	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("codex oauth refresh failed: status=%d", resp.StatusCode)
	}

	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" || payload.ExpiresIn <= 0 || payload.ExpiresIn > maxCodexOAuthLifetimeSeconds {
		return nil, errors.New("codex oauth refresh response missing or invalid fields")
	}

	return &CodexOAuthTokenResult{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

func exchangeCodexAuthorizationCode(
	ctx context.Context,
	client *http.Client,
	tokenURL string,
	clientID string,
	code string,
	verifier string,
	redirectURI string,
) (*CodexOAuthTokenResult, error) {
	c := strings.TrimSpace(code)
	v := strings.TrimSpace(verifier)
	if c == "" {
		return nil, errors.New("empty authorization code")
	}
	if v == "" {
		return nil, errors.New("empty code_verifier")
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("code", c)
	form.Set("code_verifier", v)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := DoUpstreamAcceptanceHTTP(client, req, model.UpstreamAcceptanceDispatchAuth)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := common.DecodeJson(resp.Body, &payload); err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("codex oauth code exchange failed: status=%d", resp.StatusCode)
	}
	if strings.TrimSpace(payload.AccessToken) == "" || strings.TrimSpace(payload.RefreshToken) == "" || payload.ExpiresIn <= 0 || payload.ExpiresIn > maxCodexOAuthLifetimeSeconds {
		return nil, errors.New("codex oauth token response missing or invalid fields")
	}
	return &CodexOAuthTokenResult{
		AccessToken:  strings.TrimSpace(payload.AccessToken),
		RefreshToken: strings.TrimSpace(payload.RefreshToken),
		ExpiresAt:    time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}, nil
}

func getCodexOAuthHTTPClient(proxyURL string) (*http.Client, error) {
	baseClient, err := GetHttpClientWithProxy(strings.TrimSpace(proxyURL))
	if err != nil {
		return nil, err
	}
	if baseClient == nil {
		baseClient = &http.Client{}
	}
	clientCopy := *baseClient
	clientCopy.Timeout = defaultHTTPTimeout
	// Token POST bodies contain authorization codes, PKCE verifiers or refresh
	// credentials. The fixed token endpoint must not replay them at a Location,
	// including same-origin redirects. Keep the shared relay policy unchanged.
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &clientCopy, nil
}

func buildCodexAuthorizeURL(state string, challenge string) (string, error) {
	u, err := url.Parse(codexOAuthAuthorizeURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", codexOAuthClientID)
	q.Set("redirect_uri", codexOAuthRedirectURI)
	q.Set("scope", codexOAuthScope)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("id_token_add_organizations", "true")
	q.Set("codex_cli_simplified_flow", "true")
	q.Set("originator", "codex_cli_rs")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func createStateHex(nBytes int) (string, error) {
	if nBytes <= 0 {
		return "", errors.New("invalid state bytes length")
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func generatePKCEPair() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge, nil
}

func ExtractCodexAccountIDFromJWT(token string) (string, bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", false
	}
	raw, ok := claims[codexJWTClaimPath]
	if !ok {
		return "", false
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return "", false
	}
	v, ok := obj["chatgpt_account_id"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func ExtractEmailFromJWT(token string) (string, bool) {
	claims, ok := decodeJWTClaims(token)
	if !ok {
		return "", false
	}
	v, ok := claims["email"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	return s, true
}

func decodeJWTClaims(token string) (map[string]any, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, false
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims map[string]any
	if err := common.Unmarshal(payloadRaw, &claims); err != nil {
		return nil, false
	}
	return claims, true
}
