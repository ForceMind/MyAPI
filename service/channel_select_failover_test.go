package service

import (
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayFailoverHonorsCrossGroupRetry(t *testing.T) {
	oldCooldown, oldTimeout := common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds
	common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds = 0, 0
	t.Cleanup(func() {
		common.RelayFailureCooldownSeconds, common.RelayFailoverTimeoutSeconds = oldCooldown, oldTimeout
	})
	for _, test := range []struct {
		name                        string
		crossGroupRetry, sibling    bool
		initialEmpty, affinityGroup bool
		wantChannel                 int
		wantGroup                   string
	}{
		{name: "disabled stops after current group fails", wantGroup: "default"},
		{name: "enabled can select the next group", crossGroupRetry: true, wantChannel: 7102, wantGroup: "vip"},
		{name: "disabled keeps a healthy same-group sibling", sibling: true, wantChannel: 7103, wantGroup: "default"},
		{name: "disabled still scans initially empty groups", initialEmpty: true, wantChannel: 7102, wantGroup: "vip"},
		{name: "disabled retains the actual affinity group", affinityGroup: true, wantGroup: "vip"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			common.RetryTimes = 2
			if !test.initialEmpty {
				createChannelSelectAutoGroupsChannel(t, db, 7101, "default", "public")
			}
			createChannelSelectAutoGroupsChannel(t, db, 7102, "vip", "public")
			if test.sibling {
				createChannelSelectAutoGroupsChannel(t, db, 7103, "default", "public")
				require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 7101).Update("priority", 10).Error)
				require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 7101).Update("priority", 10).Error)
			}
			require.NoError(t, model.InitChannelCache())
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
			common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, test.crossGroupRetry)
			c.Set("model_route", map[string]any{"upstream_model": "public"})
			state := BeginRelayFailover(c)
			require.NotNil(t, state)
			defer state.Close()
			param := &RetryParam{Ctx: c, TokenGroup: "auto", ModelName: "public", RequestPath: c.Request.URL.Path, Retry: common.GetPointer(0)}
			if !test.initialEmpty {
				var first *model.Channel
				if test.affinityGroup {
					// Affinity records the actual group without a selection-loop index.
					var err error
					first, err = model.CacheGetChannel(7102)
					require.NoError(t, err)
					common.SetContextKey(c, constant.ContextKeyAutoGroup, "vip")
				} else {
					var err error
					first, _, err = CacheGetRandomSatisfiedChannel(param)
					require.NoError(t, err)
					require.NotNil(t, first)
					require.Equal(t, 7101, first.Id)
				}
				c.Set("channel_id", first.Id)
				common.SetContextKey(c, constant.ContextKeyChannelKey, first.Key)
				state.StartAttempt(c)
				state.FinishAttempt(c, "retryable_refusal", 429, true)
				param.IncreaseRetry()
			}
			selected, group, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			assert.Equal(t, test.wantGroup, group)
			if test.wantChannel == 0 {
				assert.Nil(t, selected, "disabled cross-group retry must not select a different tariff group")
			} else {
				require.NotNil(t, selected)
				assert.Equal(t, test.wantChannel, selected.Id)
			}
		})
	}
}
