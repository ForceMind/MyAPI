package model

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelModelRoutePrecedenceAndConflict(t *testing.T) {
	rules := []dto.ModelRoute{
		{PublicModel: "public-", UpstreamModel: "prefix-target", Match: "prefix", Priority: 100},
		{PublicModel: "public-chat", UpstreamModel: "exact-target", Match: "exact"},
		{PublicModel: "public-chat", UpstreamModel: "responses-target", Match: "exact", Endpoint: "/v1/responses"},
	}
	settings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: rules})
	require.NoError(t, err)
	ch := &Channel{Id: 10, Type: constant.ChannelTypeOpenAI, Models: "public-chat", OtherSettings: string(settings)}
	for _, tt := range []struct{ model, path, want, reason string }{
		{"public-chat", "/v1/responses", "responses-target", "explicit_exact"},
		{"public-chat", "/v1/chat/completions", "exact-target", "explicit_exact"},
		{"public-other", "/v1/responses", "prefix-target", "explicit_prefix"},
		{"unrelated", "/v1/responses", "unrelated", "configured_model"},
	} {
		t.Run(tt.model+tt.path, func(t *testing.T) {
			r, err := ResolveChannelModelRoute(ch, tt.model, tt.path)
			require.NoError(t, err)
			require.Equal(t, tt.want, r.UpstreamModel)
			require.Equal(t, tt.reason, r.Reason)
			require.Len(t, r.ConfigDigest, 64)
		})
	}
	ch.ModelMapping = common.GetPointer(`{"public-chat":"mid","mid":"legacy-target"}`)
	r, err := ResolveChannelModelRoute(ch, "public-chat", "/v1/responses")
	require.NoError(t, err)
	require.Equal(t, "legacy-target", r.UpstreamModel)
	require.Equal(t, "explicit_mapping", r.Reason)
	ch.ModelMapping = common.GetPointer(`{"public-chat":"mid","mid":"public-chat"}`)
	_, err = ResolveChannelModelRoute(ch, "public-chat", "/v1/responses")
	require.ErrorIs(t, err, ErrModelRouteConflict)
	rules = append(rules, dto.ModelRoute{PublicModel: "public-chat", UpstreamModel: "conflict", Match: "exact"})
	require.ErrorIs(t, ValidateChannelModelRoutes(constant.ChannelTypeOpenAI, rules), ErrModelRouteConflict)
}

func TestChannelModelRouteEndpointDoesNotFallBackToUnmappedName(t *testing.T) {
	settings, err := common.Marshal(dto.ChannelOtherSettings{ModelRoutes: []dto.ModelRoute{{PublicModel: "public", UpstreamModel: "target", Match: "exact", Endpoint: "/v1/responses"}}})
	require.NoError(t, err)
	ch := &Channel{Type: constant.ChannelTypeOpenAI, OtherSettings: string(settings)}
	_, err = ResolveChannelModelRoute(ch, "public", "/v1/chat/completions")
	require.ErrorIs(t, err, ErrModelRouteEndpoint)
	_, err = ResolveChannelModelRoute(ch, "public", "")
	require.ErrorIs(t, err, ErrModelRouteEndpoint)
	other, err := ResolveChannelModelRoute(ch, "embedding-model", "/v1/embeddings")
	require.NoError(t, err)
	require.Equal(t, "embedding-model", other.UpstreamModel, "unrelated legacy protocols retain their behavior")
	ch.Type = constant.ChannelTypeCodex
	_, err = ResolveChannelModelRoute(ch, "public", "/v1/chat/completions")
	require.ErrorIs(t, err, ErrModelRouteEndpoint)
	route, err := ResolveChannelModelRoute(ch, "public", "/v1/responses")
	require.NoError(t, err)
	require.Equal(t, "target", route.UpstreamModel)
}
