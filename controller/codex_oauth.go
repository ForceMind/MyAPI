package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/middleware"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/relay/channel/codex"
	"github.com/ForceMind/MyAPI/service"

	"github.com/gin-gonic/gin"
)

const codexOAuthFlowTTL = 10 * time.Minute

var exchangeCodexOAuthCode = service.ExchangeCodexAuthorizationCodeWithProxy

type codexOAuthCompleteRequest struct {
	Input  string             `json:"input"`
	Create *AddChannelRequest `json:"create,omitempty"`
}

type codexOAuthFlowPayload struct {
	Verifier  string `json:"verifier"`
	ChannelID int    `json:"channel_id"`
}

func parseCodexAuthorizationInput(input string) (code string, state string, err error) {
	v := strings.TrimSpace(input)
	if v == "" {
		return "", "", errors.New("empty input")
	}
	if strings.Contains(v, "code=") {
		if parsed, parseErr := url.Parse(v); parseErr == nil {
			query := parsed.Query()
			if parsedCode := strings.TrimSpace(query.Get("code")); parsedCode != "" {
				return parsedCode, strings.TrimSpace(query.Get("state")), nil
			}
		}
		queryInput := v
		if fragment := strings.IndexByte(queryInput, '#'); fragment >= 0 {
			queryInput = queryInput[:fragment]
		}
		if query, parseErr := url.ParseQuery(queryInput); parseErr == nil {
			if parsedCode := strings.TrimSpace(query.Get("code")); parsedCode != "" {
				return parsedCode, strings.TrimSpace(query.Get("state")), nil
			}
		}
	}
	if strings.Contains(v, "#") {
		parts := strings.SplitN(v, "#", 2)
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
	}
	return v, "", nil
}

func StartCodexOAuth(c *gin.Context) {
	startCodexOAuthWithChannelID(c, 0)
}

func StartCodexOAuthForChannel(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	startCodexOAuthWithChannelID(c, channelID)
}

func startCodexOAuthWithChannelID(c *gin.Context, channelID int) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "dashboard session required"})
		return
	}
	if channelID > 0 {
		ch, err := getCodexOAuthChannel(channelID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		_ = ch
	}

	flow, err := service.CreateCodexOAuthAuthorizationFlow()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payload, err := common.Marshal(codexOAuthFlowPayload{
		Verifier:  flow.Verifier,
		ChannelID: channelID,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{
		Purpose:   model.AuthFlowPurposeCodexOAuth,
		Provider:  "openai",
		UserId:    identity.UserID,
		SessionId: identity.SessionID,
		Payload:   string(payload),
		ExpiresAt: time.Now().Add(codexOAuthFlowTTL),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	authorizeURL, err := service.BuildCodexOAuthAuthorizeURL(state, flow.Challenge)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    gin.H{"authorize_url": authorizeURL},
	})
}

func CompleteCodexOAuth(c *gin.Context) {
	completeCodexOAuthWithChannelID(c, 0)
}

func CompleteCodexOAuthForChannel(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, fmt.Errorf("invalid channel id: %w", err))
		return
	}
	completeCodexOAuthWithChannelID(c, channelID)
}

func completeCodexOAuthWithChannelID(c *gin.Context, channelID int) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "dashboard session required"})
		return
	}
	var req codexOAuthCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if channelID == 0 {
		if req.Create == nil || req.Create.Mode != "single" || req.Create.Channel == nil ||
			req.Create.Channel.Id != 0 || req.Create.Channel.Type != constant.ChannelTypeCodex ||
			req.Create.Channel.ChannelInfo.IsMultiKey || strings.TrimSpace(req.Create.Channel.Key) != "" ||
			req.Create.BatchAddSetKeyPrefix2Name {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgCodexOAuthInvalidCreate)})
			return
		}
		if err := req.Create.Channel.ValidateSettings(); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgCodexOAuthInvalidSettings)})
			return
		}
		if err := validateChannelModelNames(req.Create.Channel); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgCodexOAuthModelNameTooLong)})
			return
		}
	} else if req.Create != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": common.TranslateMessage(c, i18n.MsgCodexOAuthInvalidCreate)})
		return
	}
	code, state, err := parseCodexAuthorizationInput(req.Input)
	if err != nil || code == "" || state == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid Codex callback URL"})
		return
	}

	channelProxy := ""
	var targetChannel *model.Channel
	if channelID > 0 {
		ch, err := getCodexOAuthChannel(channelID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
		targetChannel = ch
		channelProxy = ch.GetSetting().Proxy
	} else {
		channelProxy = req.Create.Channel.GetSetting().Proxy
	}

	flowMatch := model.AuthFlowMatch{
		Purpose:   model.AuthFlowPurposeCodexOAuth,
		Provider:  "openai",
		UserId:    identity.UserID,
		SessionId: identity.SessionID,
	}
	pendingFlow, err := model.GetAuthFlow(state, flowMatch)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex login session expired; start again"})
		return
	}
	var flowPayload codexOAuthFlowPayload
	if err := common.Unmarshal([]byte(pendingFlow.Payload), &flowPayload); err != nil || strings.TrimSpace(flowPayload.Verifier) == "" {
		common.SysError("invalid Codex OAuth flow payload")
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex login session is invalid; start again"})
		return
	}
	if flowPayload.ChannelID != channelID {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex login session does not match this channel"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	tokenRes, err := exchangeCodexOAuthCode(ctx, code, flowPayload.Verifier, channelProxy)
	if err != nil {
		common.SysError("failed to exchange codex authorization code: " + err.Error())
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex authorization failed; start again"})
		return
	}
	if _, err := model.ConsumeAuthFlow(state, flowMatch); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex login session expired; start again"})
		return
	}

	accountID, ok := service.ExtractCodexAccountIDFromJWT(tokenRes.AccessToken)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Codex credential does not contain an account ID"})
		return
	}
	email, _ := service.ExtractEmailFromJWT(tokenRes.AccessToken)
	key := codex.OAuthKey{
		AccessToken:  tokenRes.AccessToken,
		RefreshToken: tokenRes.RefreshToken,
		AccountID:    accountID,
		LastRefresh:  time.Now().Format(time.RFC3339),
		Expired:      tokenRes.ExpiresAt.Format(time.RFC3339),
		Email:        email,
		Type:         "codex",
	}
	encoded, err := common.Marshal(key)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := gin.H{
		"account_id":   accountID,
		"email":        email,
		"expires_at":   key.Expired,
		"last_refresh": key.LastRefresh,
	}
	if channelID > 0 {
		updated, updateErr := model.UpdateChannelCredentialIfUnchanged(
			c.Request.Context(),
			channelID,
			constant.ChannelTypeCodex,
			targetChannel.Key,
			string(encoded),
		)
		if updateErr != nil {
			common.ApiError(c, updateErr)
			return
		}
		if !updated {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Codex channel credential changed; start again"})
			return
		}
		service.ResetProxyClientCache()
		data["channel_id"] = channelID
		c.JSON(http.StatusOK, gin.H{"success": true, "message": "saved", "data": data})
		return
	}

	channel := req.Create.Channel
	channel.Key = string(encoded)
	channel.CreatedTime = common.GetTimestamp()
	if err := validateChannel(channel, true); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	if err := model.InsertChannelWithAbilities(c.Request.Context(), channel); err != nil {
		common.ApiError(c, err)
		return
	}
	model.InitChannelCache()
	recordManageAudit(c, "channel.codex_oauth.create", map[string]interface{}{"channel_id": channel.Id})
	data["channel_id"] = channel.Id
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "saved", "data": data})
}

func getCodexOAuthChannel(channelID int) (*model.Channel, error) {
	ch, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, err
	}
	if ch == nil {
		return nil, errors.New("channel not found")
	}
	if ch.Type != constant.ChannelTypeCodex {
		return nil, errors.New("channel type is not Codex")
	}
	return ch, nil
}
