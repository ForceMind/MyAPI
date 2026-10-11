package controller

import (
	"net/http"
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
				"metadata_fingerprint": tc.fingerprint,
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
		"metadata_fingerprint": opened.MetadataFingerprint,
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
