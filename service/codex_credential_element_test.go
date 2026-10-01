package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRefreshCodexCredentialElementPreservesOtherAccountsAndRejectsConflicts(t *testing.T) {
	for _, scenario := range []string{"rotate one account", "single formatted credential", "request canceled after rotation", "changed before refresh", "changed during refresh", "duplicate credential"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.SystemTaskLock{}))
			previousDB, previousCache := model.DB, common.MemoryCacheEnabled
			model.DB, common.MemoryCacheEnabled = db, false
			t.Cleanup(func() { model.DB, common.MemoryCacheEnabled = previousDB, previousCache; _ = sqlDB.Close() })
			old := `{"access_token":"synthetic-old","refresh_token":"synthetic-refresh","account_id":"account-a"}`
			other := `{"access_token":"synthetic-other","refresh_token":"synthetic-other-refresh","account_id":"account-b"}`
			channel := model.Channel{Type: constant.ChannelTypeCodex, Key: "[" + old + "," + other + "]", ChannelInfo: model.ChannelInfo{IsMultiKey: true}}
			if scenario == "single formatted credential" {
				old = " " + strings.ReplaceAll(old, ",", ",\n") + " "
				channel.Key, channel.ChannelInfo.IsMultiKey = old, false
			}
			if scenario == "duplicate credential" {
				channel.Key = "[" + old + "," + old + "]"
			}
			require.NoError(t, db.Create(&channel).Error)
			if scenario == "changed before refresh" {
				require.NoError(t, db.Model(&channel).Update("key", "operator-before").Error)
			}
			client, err := GetHttpClientWithProxy("")
			require.NoError(t, err)
			previousTransport := client.Transport
			t.Cleanup(func() { client.Transport = previousTransport })
			requests := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client.Transport = codexCredentialRefreshRoundTripper(func(request *http.Request) (*http.Response, error) {
				requests++
				require.NoError(t, request.ParseForm())
				assert.Equal(t, "synthetic-refresh", request.Form.Get("refresh_token"))
				if scenario == "changed during refresh" {
					require.NoError(t, db.Model(&channel).Update("key", "operator-during").Error)
				}
				if scenario == "request canceled after rotation" {
					cancel()
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: request, Body: io.NopCloser(strings.NewReader(`{"access_token":"synthetic-new","refresh_token":"synthetic-new-refresh","expires_in":3600}`))}, nil
			})
			refreshed, err := RefreshCodexChannelCredentialElement(ctx, channel.Id, old, "")
			var persisted model.Channel
			require.NoError(t, db.First(&persisted, channel.Id).Error)
			switch scenario {
			case "rotate one account", "single formatted credential", "request canceled after rotation":
				require.NoError(t, err)
				require.NotNil(t, refreshed)
				assert.Equal(t, "account-a", refreshed.AccountID)
				assert.Equal(t, "synthetic-new", refreshed.AccessToken)
				if scenario == "single formatted credential" {
					require.Len(t, persisted.GetKeys(), 1)
				} else {
					require.Len(t, persisted.GetKeys(), 2)
					assert.Equal(t, other, persisted.GetKeys()[1])
				}
				assert.NotContains(t, persisted.Key, "synthetic-old")
				assert.Equal(t, 1, requests)
			case "changed during refresh":
				var persistenceErr *CodexCredentialPersistenceError
				require.ErrorAs(t, err, &persistenceErr)
				assert.Nil(t, refreshed)
				assert.Equal(t, "operator-during", persisted.Key)
				assert.Equal(t, 1, requests)
			default:
				require.Error(t, err)
				assert.Nil(t, refreshed)
				assert.Zero(t, requests)
				if scenario == "changed before refresh" {
					assert.Equal(t, "operator-before", persisted.Key)
				}
			}
		})
	}
}
