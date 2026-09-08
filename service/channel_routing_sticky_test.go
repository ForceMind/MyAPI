package service

import (
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSmartRoutingStickyTest(t *testing.T) *gorm.DB {
	t.Helper()
	setRoutingPolicyForTest(t, true, true)
	db := setupRoutingSelectorDB(t, true)
	require.NoError(t, db.AutoMigrate(&model.ChannelRoutingSession{}))
	return db
}

func newSmartRoutingStickyParam(userID, tokenID int, conversationID string) *RetryParam {
	body := `{}`
	if conversationID != "" {
		body = fmt.Sprintf(`{"conversation_id":%q}`, conversationID)
	}
	ctx := newRoutingSessionContext("/v1/chat/completions", body, userID, tokenID)
	return &RetryParam{
		Ctx: ctx, TokenGroup: "sticky-group", ModelName: "routing-model",
		RequestPath: "/v1/chat/completions", Retry: common.GetPointer(0),
	}
}

func currentSmartRoutingBinding(t *testing.T, db *gorm.DB, param *RetryParam) model.ChannelRoutingSession {
	t.Helper()
	hash := channelRoutingSessionHash(param.Ctx)
	require.NotEmpty(t, hash)
	var binding model.ChannelRoutingSession
	require.NoError(t, db.Where("key_hash = ?", hash).First(&binding).Error)
	return binding
}

func TestSmartRoutingStickyKeepsAuthenticatedBindingAcrossPriorityWeightChanges(t *testing.T) {
	db := setupSmartRoutingStickyTest(t)
	bound := addRoutingSelectorChannel(t, db, "bound", "sticky-group", 10, 1)
	preferred := addRoutingSelectorChannel(t, db, "preferred", "sticky-group", 9, 100)

	first := newSmartRoutingStickyParam(71, 81, "conversation-sticky")
	selected, group, err := SelectSmartChannelRouting(first)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, bound.Id, selected.Id)
	assert.Equal(t, "sticky-group", group)
	assert.Equal(t, bound.Id, currentSmartRoutingBinding(t, db, first).ChannelId)

	second := newSmartRoutingStickyParam(71, 81, "conversation-sticky")
	selected, _, err = SelectSmartChannelRouting(second)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, bound.Id, selected.Id, "a separate request with the same authenticated session must reuse its binding")

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", bound.Id).Updates(map[string]any{"priority": 1, "weight": 0}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", bound.Id).Updates(map[string]any{"priority": 1, "weight": 0}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", preferred.Id).Updates(map[string]any{"priority": 20, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", preferred.Id).Updates(map[string]any{"priority": 20, "weight": 1}).Error)

	afterChange := newSmartRoutingStickyParam(71, 81, "conversation-sticky")
	selected, _, err = SelectSmartChannelRouting(afterChange)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, bound.Id, selected.Id, "priority and weight changes must not migrate an active binding")
	assert.Equal(t, bound.Id, currentSmartRoutingBinding(t, db, afterChange).ChannelId)
}

func TestSmartRoutingStickySwitchesExcludedBindingBeforeLowerPriority(t *testing.T) {
	db := setupSmartRoutingStickyTest(t)
	bound := addRoutingSelectorChannel(t, db, "bound", "sticky-group", 10, 1)
	replacement := addRoutingSelectorChannel(t, db, "replacement", "sticky-group", 10, 0)
	lowerPriority := addRoutingSelectorChannel(t, db, "lower-priority", "sticky-group", 9, 100)

	retry := newSmartRoutingStickyParam(72, 82, "conversation-failure")
	selected, _, err := SelectSmartChannelRouting(retry)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, bound.Id, selected.Id)

	MarkSmartChannelRoutingFailure(retry.Ctx, bound.Id, "upstream_status_500")
	retry.ExcludeChannel(bound.Id)
	selected, _, err = SelectSmartChannelRouting(retry)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, replacement.Id, selected.Id, "the same-priority replacement must win before a lower-priority channel")
	assert.NotEqual(t, lowerPriority.Id, selected.Id)
	state := existingChannelRoutingRequestState(retry.Ctx)
	require.NotNil(t, state)
	assert.Equal(t, 1, state.SwitchCount)
	assert.Equal(t, replacement.Id, currentSmartRoutingBinding(t, db, retry).ChannelId)

	future := newSmartRoutingStickyParam(72, 82, "conversation-failure")
	selected, _, err = SelectSmartChannelRouting(future)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, replacement.Id, selected.Id, "the switched binding must be reused by later requests")
	assert.Equal(t, replacement.Id, currentSmartRoutingBinding(t, db, future).ChannelId)
}

func TestSmartRoutingStickyRequiresSessionIDAndSeparatesPrincipals(t *testing.T) {
	db := setupSmartRoutingStickyTest(t)
	channel := addRoutingSelectorChannel(t, db, "only", "sticky-group", 10, 1)

	withoutSession := newSmartRoutingStickyParam(73, 83, "")
	selected, _, err := SelectSmartChannelRouting(withoutSession)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, channel.Id, selected.Id)
	assert.Empty(t, channelRoutingSessionHash(withoutSession.Ctx))
	var bindings int64
	require.NoError(t, db.Model(&model.ChannelRoutingSession{}).Count(&bindings).Error)
	assert.Zero(t, bindings, "requests without an explicit session identity must not create a binding")

	principals := []*RetryParam{
		newSmartRoutingStickyParam(73, 83, "shared-conversation"),
		newSmartRoutingStickyParam(74, 83, "shared-conversation"),
		newSmartRoutingStickyParam(73, 84, "shared-conversation"),
	}
	hashes := make(map[string]struct{}, len(principals))
	for _, param := range principals {
		selected, _, err = SelectSmartChannelRouting(param)
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, channel.Id, selected.Id)
		hash := channelRoutingSessionHash(param.Ctx)
		hashes[hash] = struct{}{}
		assert.Equal(t, channel.Id, currentSmartRoutingBinding(t, db, param).ChannelId)
	}
	assert.Len(t, hashes, len(principals), "user and token identities must scope otherwise identical session IDs")
	require.NoError(t, db.Model(&model.ChannelRoutingSession{}).Count(&bindings).Error)
	assert.Equal(t, int64(len(principals)), bindings)
}
