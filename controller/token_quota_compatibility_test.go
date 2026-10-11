package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/require"
)

func TestTokenMetadataPreservesWideQuota(t *testing.T) {
	oldDB, oldLogDB, oldRedis, oldBatch := model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupTokenControllerTestDB(t)
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldLogDB, oldRedis, oldBatch
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	token := seedToken(t, db, 1, "wide-metadata", "synthetic-wide-token")
	const balance int64 = 1<<32 + 17
	const used int64 = 1<<32 + 23
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": balance, "used_quota": used, "quota_version": int64(55),
	}).Error)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "metadata-edited", "expired_time": -1,
		"unlimited_quota": false, "group": "default",
	}, 1)
	UpdateToken(ctx)
	require.True(t, decodeAPIResponse(t, recorder).Success)
	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	require.Equal(t, "metadata-edited", stored.Name)
	require.EqualValues(t, balance, stored.RemainQuota)
	require.EqualValues(t, used, stored.UsedQuota)
	require.Equal(t, int64(55), stored.QuotaVersion)
}

func TestTokenQuotaVersionIsExactAndStaleEditsWriteNothing(t *testing.T) {
	oldDB, oldLogDB, oldRedis, oldBatch := model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupTokenControllerTestDB(t)
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldLogDB, oldRedis, oldBatch
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	token := seedToken(t, db, 1, "versioned-metadata", "synthetic-versioned-token")
	const version int64 = 1<<53 + 1
	const used int64 = 1<<32 + 23
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": 100, "used_quota": used, "quota_version": version,
	}).Error)
	token.QuotaVersion = version
	masked := buildMaskedTokenResponse(token)
	require.Equal(t, "9007199254740993", masked.QuotaVersion)
	require.NotEmpty(t, masked.MetadataFingerprint)
	for _, tc := range []struct {
		name, version, fingerprint string
		valid                      bool
	}{
		{"missing version", "", masked.MetadataFingerprint, false},
		{"missing fingerprint", "9007199254740993", "", false},
		{"stale version", "9007199254740992", masked.MetadataFingerprint, false},
		{"exact version", "9007199254740993", masked.MetadataFingerprint, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
				"id": token.Id, "name": "version-edited", "expired_time": -1, "group": "default",
				"unlimited_quota": false, "remain_quota": 120, "quota_version": tc.version,
				"metadata_fingerprint": tc.fingerprint, "request_id": "de305d54-75b4-431b-adb2-eb6b9e546014",
			}, 1)
			UpdateToken(ctx)
			require.Equal(t, tc.valid, decodeAPIResponse(t, recorder).Success)
			var stored model.Token
			require.NoError(t, db.First(&stored, token.Id).Error)
			require.Equal(t, used, stored.UsedQuota)
			if tc.valid {
				require.Equal(t, int64(120), stored.RemainQuota)
				require.Equal(t, version+1, stored.QuotaVersion)
				require.Equal(t, "version-edited", stored.Name)
			} else {
				require.Equal(t, int64(100), stored.RemainQuota)
				require.Equal(t, version, stored.QuotaVersion)
				require.Equal(t, "versioned-metadata", stored.Name)
			}
		})
	}
}

func TestTokenQuotaMetadataFingerprintRejectsStalePermissionRestore(t *testing.T) {
	oldDB, oldLogDB, oldRedis, oldBatch := model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupTokenControllerTestDB(t)
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldLogDB, oldRedis, oldBatch
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	token := seedToken(t, db, 1, "opened-enabled", "synthetic-metadata-conflict-token")
	opened := buildMaskedTokenResponse(token)
	require.NotEmpty(t, opened.MetadataFingerprint)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"status": common.TokenStatusDisabled,
		"name":   "disabled-concurrently",
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "opened-enabled", "status": common.TokenStatusEnabled,
		"expired_time": -1, "unlimited_quota": true, "group": "default",
		"remain_quota": token.RemainQuota, "quota_version": opened.QuotaVersion,
		"metadata_fingerprint": opened.MetadataFingerprint, "request_id": "26a8ea50-f99d-4b37-95c7-2e4de774ca70",
	}, 1)
	UpdateToken(ctx)
	require.False(t, decodeAPIResponse(t, recorder).Success)

	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	require.Equal(t, common.TokenStatusDisabled, stored.Status)
	require.Equal(t, "disabled-concurrently", stored.Name)
	require.Equal(t, token.RemainQuota, stored.RemainQuota)
	require.Equal(t, token.QuotaVersion, stored.QuotaVersion)
}

type tokenQuotaUpdateResponse struct {
	Success         bool                              `json:"success"`
	Data            tokenQuotaUpdateResponseData      `json:"data"`
	QuotaAdjustment *model.TokenQuotaAdjustmentResult `json:"quota_adjustment"`
	Replayed        bool                              `json:"replayed"`
}

type tokenQuotaUpdateResponseData struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Key          string `json:"key"`
	RemainQuota  int64  `json:"remain_quota"`
	UsedQuota    int64  `json:"used_quota"`
	QuotaVersion string `json:"quota_version"`
}

func decodeTokenQuotaUpdateResponse(t *testing.T, recorderBody []byte) tokenQuotaUpdateResponse {
	t.Helper()
	var response tokenQuotaUpdateResponse
	require.NoError(t, common.Unmarshal(recorderBody, &response))
	return response
}

func TestTokenQuotaRequestFingerprintBindsWritablePresenceAndExcludesSecret(t *testing.T) {
	decode := func(raw string) tokenRequest {
		var request tokenRequest
		require.NoError(t, common.Unmarshal([]byte(raw), &request))
		return request
	}
	base := decode(`{"id":7,"name":"same","remain_quota":11,"quota_version":"3","metadata_fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","request_id":"de305d54-75b4-431b-adb2-eb6b9e546014","key":"first-secret"}`)
	otherSecret := decode(`{"id":7,"name":"same","remain_quota":11,"quota_version":"3","metadata_fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","request_id":"de305d54-75b4-431b-adb2-eb6b9e546014","key":"second-secret"}`)
	absentAllowIPs := base
	presentAllowIPs := decode(`{"id":7,"name":"same","remain_quota":11,"quota_version":"3","metadata_fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","request_id":"de305d54-75b4-431b-adb2-eb6b9e546014","key":"first-secret","allow_ips":null}`)
	unknownFutureField := decode(`{"id":7,"name":"same","remain_quota":11,"quota_version":"3","metadata_fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","request_id":"de305d54-75b4-431b-adb2-eb6b9e546014","key":"first-secret","future_optional":"ignored-by-v1"}`)

	baseFingerprint, err := tokenQuotaRequestFingerprint(&base, 19)
	require.NoError(t, err)
	otherSecretFingerprint, err := tokenQuotaRequestFingerprint(&otherSecret, 19)
	require.NoError(t, err)
	absentFingerprint, err := tokenQuotaRequestFingerprint(&absentAllowIPs, 19)
	require.NoError(t, err)
	presentFingerprint, err := tokenQuotaRequestFingerprint(&presentAllowIPs, 19)
	require.NoError(t, err)
	unknownFutureFingerprint, err := tokenQuotaRequestFingerprint(&unknownFutureField, 19)
	require.NoError(t, err)
	require.Equal(t, "8593b6cb1bad42465f9d9deda5dc03b59b52b41b6b5bc4d9253efc60897e95d1", baseFingerprint)
	require.Equal(t, baseFingerprint, otherSecretFingerprint)
	require.Equal(t, baseFingerprint, absentFingerprint)
	require.Equal(t, baseFingerprint, unknownFutureFingerprint, "v1 ignores unknown fields; a future writable field requires a new fingerprint version")
	require.NotEqual(t, baseFingerprint, presentFingerprint)

	otherActorFingerprint, err := tokenQuotaRequestFingerprint(&base, 20)
	require.NoError(t, err)
	require.NotEqual(t, baseFingerprint, otherActorFingerprint)
}

func TestTokenQuotaRetryReturnsOriginalReceiptAndCurrentMaskedToken(t *testing.T) {
	oldDB, oldLogDB, oldRedis, oldBatch := model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupTokenControllerTestDB(t)
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldLogDB, oldRedis, oldBatch
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	token := seedToken(t, db, 1, "opened", "synthetic-replayed-token")
	opened := buildMaskedTokenResponse(token)
	body := map[string]any{
		"id": token.Id, "name": "first-write", "status": common.TokenStatusEnabled,
		"expired_time": -1, "unlimited_quota": true, "group": "default", "remain_quota": 80,
		"quota_version": opened.QuotaVersion, "metadata_fingerprint": opened.MetadataFingerprint,
		"request_id": "a8098c1a-f86e-11da-bd1a-00112444be1e",
	}
	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)
	first := decodeTokenQuotaUpdateResponse(t, recorder.Body.Bytes())
	require.True(t, first.Success)
	require.False(t, first.Replayed)
	require.NotNil(t, first.QuotaAdjustment)
	require.Equal(t, int64(100), first.QuotaAdjustment.RemainQuotaBefore)
	require.Equal(t, int64(80), first.QuotaAdjustment.RemainQuotaAfter)
	require.Equal(t, "0", first.QuotaAdjustment.QuotaVersionBefore)
	require.Equal(t, "1", first.QuotaAdjustment.QuotaVersionAfter)
	staleNewRequest := make(map[string]any, len(body))
	for key, value := range body {
		staleNewRequest[key] = value
	}
	staleNewRequest["request_id"] = "9b2f4292-d4d1-4ee9-a5d7-0d9b471d87c5"
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", staleNewRequest, 1)
	UpdateToken(ctx)
	require.False(t, decodeAPIResponse(t, recorder).Success)

	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"name": "later-current", "status": common.TokenStatusDisabled, "remain_quota": int64(65),
		"used_quota": int64(35), "quota_version": int64(2),
	}).Error)
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)
	retry := decodeTokenQuotaUpdateResponse(t, recorder.Body.Bytes())
	require.True(t, retry.Success)
	require.True(t, retry.Replayed)
	require.Equal(t, first.QuotaAdjustment, retry.QuotaAdjustment)
	require.Equal(t, "later-current", retry.Data.Name)
	require.Equal(t, int64(65), retry.Data.RemainQuota)
	require.Equal(t, int64(35), retry.Data.UsedQuota)
	require.Equal(t, "2", retry.Data.QuotaVersion)
	require.Equal(t, model.MaskTokenKey("synthetic-replayed-token"), retry.Data.Key)
	var receiptCount int64
	require.NoError(t, db.Model(&model.TokenQuotaAdjustment{}).Count(&receiptCount).Error)
	require.Equal(t, int64(1), receiptCount)

	changed := make(map[string]any, len(body))
	for key, value := range body {
		changed[key] = value
	}
	changed["name"] = "different-payload"
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", changed, 1)
	UpdateToken(ctx)
	require.False(t, decodeAPIResponse(t, recorder).Success)
	extended := make(map[string]any, len(body)+1)
	for key, value := range body {
		extended[key] = value
	}
	extended["allow_ips"] = "127.0.0.1"
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", extended, 1)
	UpdateToken(ctx)
	require.False(t, decodeAPIResponse(t, recorder).Success)

	other := seedToken(t, db, 1, "other", "synthetic-other-replayed-token")
	changed["id"] = other.Id
	ctx, recorder = newAuthenticatedContext(t, http.MethodPut, "/api/token/", changed, 1)
	UpdateToken(ctx)
	require.False(t, decodeAPIResponse(t, recorder).Success)
}

func TestTokenQuotaCommandRequiresExplicitCanonicalIdentity(t *testing.T) {
	oldDB, oldLogDB, oldRedis, oldBatch := model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupTokenControllerTestDB(t)
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldLogDB, oldRedis, oldBatch
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	token := seedToken(t, db, 1, "identity", "synthetic-identity-token")
	opened := buildMaskedTokenResponse(token)
	base := map[string]any{
		"id": token.Id, "name": token.Name, "status": token.Status, "expired_time": -1,
		"unlimited_quota": true, "group": "default", "remain_quota": token.RemainQuota,
		"quota_version": opened.QuotaVersion, "metadata_fingerprint": opened.MetadataFingerprint,
		"request_id": "16fd2706-8baf-433b-82eb-8c7fada847da",
	}
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		valid  bool
	}{
		{"same target is an explicit command", func(map[string]any) {}, true},
		{"missing request id", func(body map[string]any) { delete(body, "request_id") }, false},
		{"uppercase request id", func(body map[string]any) { body["request_id"] = "16FD2706-8BAF-433B-82EB-8C7FADA847DA" }, false},
		{"null remain quota", func(body map[string]any) { body["remain_quota"] = nil }, false},
		{"identity without remain quota", func(body map[string]any) { delete(body, "remain_quota") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := make(map[string]any, len(base))
			for key, value := range base {
				body[key] = value
			}
			body["request_id"] = "16fd2706-8baf-433b-82eb-" + map[bool]string{true: "8c7fada847da", false: "8c7fada847db"}[tc.valid]
			tc.mutate(body)
			ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
			UpdateToken(ctx)
			require.Equal(t, tc.valid, decodeAPIResponse(t, recorder).Success)
		})
	}
	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	require.Equal(t, int64(1), stored.QuotaVersion, "only the explicit same-target command may advance the version")
}

func TestTokenQuotaCommandRejectsNoncanonicalAndDuplicateJSONFields(t *testing.T) {
	oldDB, oldLogDB, oldRedis, oldBatch := model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupTokenControllerTestDB(t)
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldLogDB, oldRedis, oldBatch
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	token := seedToken(t, db, 1, "canonical-json", "synthetic-canonical-json-token")
	opened := buildMaskedTokenResponse(token)
	requestID := "886313e1-3b8a-5372-9b90-0c9aee199e5d"
	tests := []struct {
		name string
		body string
	}{
		{"noncanonical remain value", fmt.Sprintf(`{"id":%d,"name":"changed","Remain_Quota":120}`, token.Id)},
		{"noncanonical remain null", fmt.Sprintf(`{"id":%d,"name":"changed","Remain_Quota":null}`, token.Id)},
		{"noncanonical version", fmt.Sprintf(`{"id":%d,"remain_quota":120,"Quota_Version":%q,"metadata_fingerprint":%q,"request_id":%q}`, token.Id, opened.QuotaVersion, opened.MetadataFingerprint, requestID)},
		{"noncanonical metadata fingerprint", fmt.Sprintf(`{"id":%d,"remain_quota":120,"quota_version":%q,"Metadata_Fingerprint":%q,"request_id":%q}`, token.Id, opened.QuotaVersion, opened.MetadataFingerprint, requestID)},
		{"noncanonical request id", fmt.Sprintf(`{"id":%d,"remain_quota":120,"quota_version":%q,"metadata_fingerprint":%q,"Request_ID":%q}`, token.Id, opened.QuotaVersion, opened.MetadataFingerprint, requestID)},
		{"duplicate exact remain", fmt.Sprintf(`{"id":%d,"remain_quota":100,"remain_quota":120,"quota_version":%q,"metadata_fingerprint":%q,"request_id":%q}`, token.Id, opened.QuotaVersion, opened.MetadataFingerprint, requestID)},
		{"duplicate case variant", fmt.Sprintf(`{"id":%d,"remain_quota":100,"Remain_Quota":120,"quota_version":%q,"metadata_fingerprint":%q,"request_id":%q}`, token.Id, opened.QuotaVersion, opened.MetadataFingerprint, requestID)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", nil, 1)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/token/", bytes.NewBufferString(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			UpdateToken(ctx)
			require.False(t, decodeAPIResponse(t, recorder).Success)
			var stored model.Token
			require.NoError(t, db.First(&stored, token.Id).Error)
			require.Equal(t, "canonical-json", stored.Name)
			require.Equal(t, int64(100), stored.RemainQuota)
			require.Zero(t, stored.QuotaVersion)
		})
	}
}

func TestGetTokenUsageReportsUnrepresentableDerivedTotal(t *testing.T) {
	oldDB, oldLogDB, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	db := setupTokenControllerTestDB(t)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	token := seedToken(t, db, 1, "usage", "synthetic-usage-token")

	type usageResponse struct {
		Success bool `json:"success"`
		Code    bool `json:"code"`
		Data    struct {
			TotalGranted              *int64 `json:"total_granted"`
			TotalGrantedRepresentable bool   `json:"total_granted_representable"`
			TotalUsed                 int64  `json:"total_used"`
			TotalAvailable            int64  `json:"total_available"`
		} `json:"data"`
	}
	requestUsage := func() usageResponse {
		ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/usage/token/", nil, 1)
		ctx.Request.Header.Set("Authorization", "Bearer sk-"+token.Key)
		GetTokenUsage(ctx)
		var response usageResponse
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response
	}

	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": int64(100), "used_quota": int64(20),
	}).Error)
	normal := requestUsage()
	require.True(t, normal.Code)
	require.True(t, normal.Data.TotalGrantedRepresentable)
	require.NotNil(t, normal.Data.TotalGranted)
	require.Equal(t, int64(120), *normal.Data.TotalGranted)

	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{
		"remain_quota": common.MaxQuotaBalance, "used_quota": common.MaxQuotaBalance,
	}).Error)
	unrepresentable := requestUsage()
	require.True(t, unrepresentable.Code)
	require.False(t, unrepresentable.Data.TotalGrantedRepresentable)
	require.Nil(t, unrepresentable.Data.TotalGranted)
	require.Equal(t, common.MaxQuotaBalance, unrepresentable.Data.TotalAvailable)
	require.Equal(t, common.MaxQuotaBalance, unrepresentable.Data.TotalUsed)

	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Update("used_quota", common.MaxQuotaBalance+1).Error)
	invalid := requestUsage()
	require.False(t, invalid.Code)
	require.False(t, invalid.Success)
}
