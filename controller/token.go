package controller

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/i18n"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type tokenAutoGroupsInput struct {
	Set    bool
	Groups []string
}

func (input *tokenAutoGroupsInput) UnmarshalJSON(data []byte) error {
	input.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		input.Groups = nil
		return nil
	}
	return common.Unmarshal(data, &input.Groups)
}

type tokenRequest struct {
	model.Token
	RemainQuota         *int64               `json:"remain_quota"`
	QuotaVersion        string               `json:"quota_version"`
	MetadataFingerprint string               `json:"metadata_fingerprint"`
	RequestID           string               `json:"request_id"`
	AutoGroups          tokenAutoGroupsInput `json:"auto_groups"`
	present             map[string]bool
	remainQuotaNull     bool
}

var tokenRequestCanonicalJSONFields = []string{
	"id", "user_id", "key", "status", "name", "created_time", "accessed_time", "expired_time",
	"remain_quota", "quota_version", "metadata_fingerprint", "request_id", "unlimited_quota",
	"model_limits_enabled", "model_limits", "allow_ips", "used_quota", "group", "access_profile_id",
	"cross_group_retry", "auto_groups", "DeletedAt",
}

func (request *tokenRequest) UnmarshalJSON(data []byte) error {
	if err := common.ValidateUniqueJSONKeys(data); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := common.Unmarshal(data, &raw); err != nil {
		return err
	}
	for key := range raw {
		for _, canonical := range tokenRequestCanonicalJSONFields {
			if key == canonical {
				break
			}
			if strings.EqualFold(key, canonical) {
				return errors.New("noncanonical token request field")
			}
		}
	}
	type tokenRequestAlias tokenRequest
	var decoded tokenRequestAlias
	if err := common.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*request = tokenRequest(decoded)
	request.present = make(map[string]bool, len(raw))
	for key := range raw {
		request.present[key] = true
	}
	if value, ok := raw["remain_quota"]; ok {
		request.remainQuotaNull = strings.TrimSpace(string(value)) == "null"
	}
	return nil
}

type tokenQuotaRequestFingerprintField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type tokenQuotaRequestFingerprintPayload struct {
	Domain             string                              `json:"domain"`
	FingerprintVersion int                                 `json:"fingerprint_version"`
	UserID             int                                 `json:"user_id"`
	TokenID            int                                 `json:"token_id"`
	Fields             []tokenQuotaRequestFingerprintField `json:"fields"`
}

func tokenQuotaRequestFingerprint(request *tokenRequest, userID int) (string, error) {
	fields := make([]tokenQuotaRequestFingerprintField, 0, 15)
	appendField := func(name string, value any) {
		if request.present[name] {
			fields = append(fields, tokenQuotaRequestFingerprintField{Name: name, Value: value})
		}
	}
	appendField("name", request.Name)
	appendField("status", request.Status)
	appendField("expired_time", request.ExpiredTime)
	appendField("remain_quota", request.RemainQuota)
	appendField("unlimited_quota", request.UnlimitedQuota)
	appendField("model_limits_enabled", request.ModelLimitsEnabled)
	appendField("model_limits", request.ModelLimits)
	appendField("allow_ips", request.AllowIps)
	appendField("group", request.Group)
	appendField("access_profile_id", request.AccessProfileID)
	appendField("cross_group_retry", request.CrossGroupRetry)
	appendField("auto_groups", request.AutoGroups.Groups)
	appendField("quota_version", request.QuotaVersion)
	appendField("metadata_fingerprint", request.MetadataFingerprint)
	appendField("request_id", request.RequestID)
	payload, err := common.Marshal(tokenQuotaRequestFingerprintPayload{
		Domain: "token_quota_adjustment", FingerprintVersion: model.TokenQuotaAdjustmentFingerprintVersion,
		UserID: userID, TokenID: request.Id, Fields: fields,
	})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:]), nil
}

type tokenResponse struct {
	*model.Token
	AutoGroups []string `json:"auto_groups"`
	// AccessProfile explains the legacy Group value while the additive
	// access_profile_id field and routing compatibility remain in place.
	AccessProfile       accessProfileMetadata `json:"access_profile"`
	QuotaVersion        string                `json:"quota_version"`
	MetadataFingerprint string                `json:"metadata_fingerprint"`
}

func buildMaskedTokenResponse(token *model.Token) *tokenResponse {
	if token == nil {
		return nil
	}
	maskedToken := *token
	maskedToken.Key = token.GetMaskedKey()
	autoGroups, err := token.GetAutoGroups()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to parse auto groups for token %d: %v", token.Id, err))
		autoGroups = nil
	}
	if len(autoGroups) == 0 {
		autoGroups = nil
	}
	metadataFingerprint, err := token.QuotaMetadataFingerprint()
	if err != nil {
		common.SysError(fmt.Sprintf("failed to fingerprint token %d metadata: %v", token.Id, err))
	}
	return &tokenResponse{
		Token:               &maskedToken,
		AutoGroups:          autoGroups,
		AccessProfile:       model.ResolveAccessProfileID(token.AccessProfileID, token.Group, ""),
		QuotaVersion:        strconv.FormatInt(token.QuotaVersion, 10),
		MetadataFingerprint: metadataFingerprint,
	}
}

func buildMaskedTokenResponses(tokens []*model.Token) []*tokenResponse {
	maskedTokens := make([]*tokenResponse, 0, len(tokens))
	for _, token := range tokens {
		maskedTokens = append(maskedTokens, buildMaskedTokenResponse(token))
	}
	return maskedTokens
}

func getTokenRequestUserGroup(c *gin.Context) (string, error) {
	if userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup); userGroup != "" {
		return userGroup, nil
	}
	if userGroup := c.GetString("group"); userGroup != "" {
		return userGroup, nil
	}
	return model.GetUserGroup(c.GetInt("id"), false)
}

func setTokenAutoGroups(c *gin.Context, token *model.Token, groups []string) bool {
	if len(groups) == 0 {
		if err := token.SetAutoGroups(nil); err != nil {
			common.ApiError(c, err)
			return false
		}
		return true
	}

	maxCount := setting.GetMaxTokenAutoGroups()
	if len(groups) > maxCount {
		common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsTooMany, map[string]any{"Max": maxCount})
		return false
	}

	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		if _, ok := seen[group]; ok {
			common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsDuplicate, map[string]any{"Group": group})
			return false
		}
		seen[group] = struct{}{}
		if !service.IsUserSelectableGroup(userGroup, group) {
			common.ApiErrorI18n(c, i18n.MsgTokenAutoGroupsInvalid, map[string]any{"Group": group})
			return false
		}
	}

	if err := token.SetAutoGroups(groups); err != nil {
		common.ApiError(c, err)
		return false
	}
	return true
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(userId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(buildMaskedTokenResponses(tokens))
	common.ApiSuccess(c, pageInfo)
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, buildMaskedTokenResponse(token))
}

func GetTokenAutoGroups(c *gin.Context) {
	userGroup, err := getTokenRequestUserGroup(c)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"groups":    service.GetUserAutoGroup(userGroup),
		"max_count": setting.GetMaxTokenAutoGroups(),
	})
}

func GetTokenKey(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"key": token.GetFullKey(),
	})
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		common.SysError("failed to get token by key: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	if _, err := common.CheckedQuotaBalance(token.RemainQuota, 0); err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": common.MaxQuotaBalance})
		return
	}
	if _, err := common.CheckedQuotaUsed(token.UsedQuota, 0); err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": common.MaxQuotaBalance})
		return
	}
	var totalGranted any
	totalGrantedRepresentable := false
	if total, totalErr := common.CheckedQuotaBalance(token.RemainQuota, token.UsedQuota); totalErr == nil {
		totalGranted = total
		totalGrantedRepresentable = true
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":                      "token_usage",
			"name":                        token.Name,
			"total_granted":               totalGranted,
			"total_granted_representable": totalGrantedRepresentable,
			"total_used":                  token.UsedQuota,
			"total_available":             token.RemainQuota,
			"unlimited_quota":             token.UnlimitedQuota,
			"model_limits":                token.GetModelLimitsMap(),
			"model_limits_enabled":        token.ModelLimitsEnabled,
			"expires_at":                  expiredAt,
		},
	})
}

func AddToken(c *gin.Context) {
	request := tokenRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	token := request.Token
	if request.RemainQuota != nil {
		token.RemainQuota = *request.RemainQuota
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if token.RemainQuota < 0 {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
		return
	}
	if token.RemainQuota > int64(common.MaxQuota) {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": common.MaxQuota})
		return
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	if token.Group == "auto" {
		if !setTokenAutoGroups(c, &token, request.AutoGroups.Groups) {
			return
		}
	} else {
		token.CrossGroupRetry = false
		_ = token.SetAutoGroups(nil)
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:             c.GetInt("id"),
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		Group:              token.Group,
		AccessProfileID:    strings.TrimSpace(token.AccessProfileID),
		CrossGroupRetry:    token.CrossGroupRetry,
		AutoGroups:         token.AutoGroups,
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionShareDeleteDenied) {
			subscriptionShareError(c, err)
			return
		}
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	request := tokenRequest{}
	err := c.ShouldBindJSON(&request)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	token := request.Token
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	quotaIdentityPresent := request.present["request_id"] || request.present["quota_version"] || request.present["metadata_fingerprint"]
	quotaRequested := request.present["remain_quota"]
	if statusOnly != "" && (quotaRequested || quotaIdentityPresent) ||
		statusOnly == "" && !quotaRequested && quotaIdentityPresent || request.remainQuotaNull || quotaRequested && request.RemainQuota == nil {
		common.ApiErrorI18n(c, i18n.MsgTokenQuotaConflict)
		return
	}
	var expectedQuotaVersion int64
	var requestFingerprint string
	var adjustmentCommand model.TokenQuotaAdjustmentCommand
	var cleanToken *model.Token
	if quotaRequested {
		if *request.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		if *request.RemainQuota > int64(common.MaxQuota) {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": common.MaxQuota})
			return
		}
		expectedQuotaVersion, err = strconv.ParseInt(request.QuotaVersion, 10, 64)
		if err != nil || expectedQuotaVersion < 0 || strconv.FormatInt(expectedQuotaVersion, 10) != request.QuotaVersion ||
			request.MetadataFingerprint == "" || request.RequestID == "" {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaConflict)
			return
		}
		requestFingerprint, err = tokenQuotaRequestFingerprint(&request, userId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		adjustmentCommand = model.TokenQuotaAdjustmentCommand{
			UserID: userId, RequestID: request.RequestID, FingerprintVersion: model.TokenQuotaAdjustmentFingerprintVersion,
			RequestFingerprint:   requestFingerprint,
			ExpectedQuotaVersion: expectedQuotaVersion, ExpectedMetadataFingerprint: request.MetadataFingerprint,
		}
		var replayResult *model.TokenQuotaAdjustmentResult
		var replayed bool
		cleanToken, replayResult, replayed, err = model.FindTokenQuotaAdjustmentReplay(token.Id, adjustmentCommand)
		if err != nil {
			if errors.Is(err, model.ErrTokenQuotaAdjustmentConflict) || errors.Is(err, model.ErrTokenQuotaAdjustmentInvalidRequest) {
				common.ApiErrorI18n(c, i18n.MsgTokenQuotaConflict)
				return
			}
			common.ApiError(c, err)
			return
		}
		if replayed {
			c.JSON(http.StatusOK, gin.H{
				"success": true, "message": "", "data": buildMaskedTokenResponse(cleanToken),
				"quota_adjustment": replayResult, "replayed": true,
			})
			return
		}
	} else {
		cleanToken, err = model.GetTokenByIds(token.Id, userId)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			common.ApiErrorI18n(c, i18n.MsgTokenExhaustedCannotEable)
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		// If you add more fields, please also update token.Update()
		cleanToken.Name = token.Name
		cleanToken.ExpiredTime = token.ExpiredTime
		if quotaRequested {
			cleanToken.RemainQuota = *request.RemainQuota
		}
		cleanToken.UnlimitedQuota = token.UnlimitedQuota
		if request.present["model_limits_enabled"] {
			cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		}
		if request.present["model_limits"] {
			cleanToken.ModelLimits = token.ModelLimits
		}
		cleanToken.AllowIps = token.AllowIps
		if strings.TrimSpace(token.AccessProfileID) != "" {
			cleanToken.AccessProfileID = strings.TrimSpace(token.AccessProfileID)
		} else if token.Group != cleanToken.Group {
			// Legacy clients only know group. Preserve their historical behavior
			// when changing groups, while allowing explicit profile IDs to win.
			cleanToken.AccessProfileID = model.EffectiveAccessProfileID(token.Group)
		}
		cleanToken.Group = token.Group
		cleanToken.CrossGroupRetry = token.CrossGroupRetry
		if token.Group != "auto" {
			cleanToken.CrossGroupRetry = false
			_ = cleanToken.SetAutoGroups(nil)
		} else if request.AutoGroups.Set {
			if !setTokenAutoGroups(c, cleanToken, request.AutoGroups.Groups) {
				return
			}
		}
	}
	var adjustmentResult *model.TokenQuotaAdjustmentResult
	replayed := false
	if quotaRequested {
		adjustmentResult, replayed, err = cleanToken.UpdateWithQuota(adjustmentCommand)
	} else if statusOnly != "" {
		err = cleanToken.UpdateStatus()
	} else {
		err = cleanToken.UpdateMetadata(request.present["model_limits_enabled"], request.present["model_limits"])
	}
	if err != nil {
		if errors.Is(err, model.ErrTokenQuotaVersionConflict) || errors.Is(err, model.ErrTokenQuotaAdjustmentConflict) ||
			errors.Is(err, model.ErrTokenQuotaAdjustmentInvalidRequest) {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaConflict)
			return
		}
		common.ApiError(c, err)
		return
	}
	response := gin.H{
		"success": true,
		"message": "",
		"data":    buildMaskedTokenResponse(cleanToken),
	}
	if quotaRequested {
		response["quota_adjustment"] = adjustmentResult
		response["replayed"] = replayed
	}
	c.JSON(http.StatusOK, response)
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionShareDeleteDenied) {
			subscriptionShareError(c, err)
			return
		}
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}

func GetTokenKeysBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	if len(tokenBatch.Ids) > 100 {
		common.ApiErrorI18n(c, i18n.MsgBatchTooMany, map[string]any{"Max": 100})
		return
	}
	userId := c.GetInt("id")
	tokens, err := model.GetTokenKeysByIds(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	keysMap := make(map[int]string)
	for _, t := range tokens {
		keysMap[t.Id] = t.GetFullKey()
	}
	common.ApiSuccess(c, gin.H{"keys": keysMap})
}
