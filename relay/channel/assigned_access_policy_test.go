package channel

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAssignedAccessHTTPDispatchWithoutFailoverBlocksRevocation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	prior, redisEnabled := model.DB, common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	model.InitColumnNamesForTest()
	t.Cleanup(func() { model.DB = prior; common.RedisEnabled = redisEnabled; _ = conn.Close() })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.AssignedAccessPolicy{}))
	user := model.User{Username: "wire-policy-owner", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "synthetic-wire-key", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	var sends atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	mapping := `{"alias":"actual"}`
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Models: "alias", Group: "default", Key: "synthetic-channel-key", BaseURL: &upstream.URL, ModelMapping: &mapping}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "alias", ChannelId: channel.Id, Enabled: true}).Error)
	row, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, "token", token.Id, 0, `{"enabled":true,"public_models":["alias"],"upstream_models":["actual"],"channel_ids":null}`)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("id", user.Id)
	c.Set("token_id", token.Id)
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	c.Set("model_route", map[string]any{"upstream_model": "actual"})
	info := &relaycommon.RelayInfo{UserId: user.Id, TokenId: token.Id, TokenKey: token.Key, OriginModelName: "alias", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id, ChannelType: channel.Type, ChannelBaseUrl: upstream.URL, ApiKey: channel.Key, UpstreamModelName: "actual"}}
	request, err := http.NewRequest(http.MethodPost, upstream.URL+"/v1/responses", bytes.NewBufferString(`{"model":"actual"}`))
	require.NoError(t, err)
	response, err := DoRequest(c, request, info)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NoError(t, response.Body.Close())
	assert.EqualValues(t, 1, sends.Load())
	for _, body := range []string{
		`{"model":"actual","model":"forbidden","Model":"actual"}`,
		`{"model":"actual","\u006dodel":"actual"}`,
		`{"model":"actual","model":"actual"}`,
	} {
		ambiguous, buildErr := http.NewRequest(http.MethodPost, upstream.URL+"/v1/responses", bytes.NewBufferString(body))
		require.NoError(t, buildErr)
		rejected, rejectErr := DoRequest(c, ambiguous, info)
		require.Error(t, rejectErr)
		assert.Nil(t, rejected)
		assert.EqualValues(t, 1, sends.Load(), "ambiguous target proof must never be sent")
	}
	_, err = model.ConfigureAssignedAccessPolicy(context.Background(), db, "token", token.Id, row.Revision, `{"enabled":false,"public_models":null,"upstream_models":null,"channel_ids":null}`)
	require.NoError(t, err)
	request, err = http.NewRequest(http.MethodPost, upstream.URL+"/v1/responses", bytes.NewBufferString(`{"model":"actual"}`))
	require.NoError(t, err)
	response, err = DoRequest(c, request, info)
	require.Error(t, err)
	assert.Nil(t, response)
	var policyErr *types.NewAPIError
	require.ErrorAs(t, err, &policyErr)
	assert.Equal(t, http.StatusForbidden, policyErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(policyErr))
	assert.EqualValues(t, 1, sends.Load(), "revoked dispatch must never reach upstream")
}

type assignedPolicyWssAdaptor struct {
	Adaptor
	url           string
	beforeHeaders func() error
}

func (a assignedPolicyWssAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}
func (a assignedPolicyWssAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return a.beforeHeaders()
}

func TestAssignedAccessWebSocketRechecksBeforeDial(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	prior := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = prior; _ = conn.Close() })
	require.NoError(t, db.AutoMigrate(&model.AssignedAccessPolicy{}))
	var dials atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { dials.Add(1); w.WriteHeader(http.StatusBadRequest) }))
	defer upstream.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	c.Set("id", 1)
	c.Set("token_id", 1)
	channel := &model.Channel{Id: 1, Type: constant.ChannelTypeOpenAI}
	require.NoError(t, service.ValidateAssignedAccessSelection(c, channel, "realtime-model"), "unassigned legacy admission is unchanged")
	adaptor := assignedPolicyWssAdaptor{url: "ws" + strings.TrimPrefix(upstream.URL, "http"), beforeHeaders: func() error {
		_, err := model.ConfigureAssignedAccessPolicy(context.Background(), db, "token", 1, 0, `{"enabled":true,"public_models":null,"upstream_models":null,"channel_ids":null}`)
		return err
	}}
	info := &relaycommon.RelayInfo{UserId: 1, TokenId: 1, OriginModelName: "realtime-model", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1, ChannelType: constant.ChannelTypeOpenAI}}
	socket, err := DoWssRequest(adaptor, c, info, nil)
	require.Error(t, err)
	assert.Nil(t, socket)
	var policyErr *types.NewAPIError
	require.ErrorAs(t, err, &policyErr)
	assert.Equal(t, http.StatusForbidden, policyErr.StatusCode)
	assert.Zero(t, dials.Load(), "new assignment must deny unsupported transport before dial")
}
