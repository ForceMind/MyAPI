package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

var ErrModelRouteConflict = errors.New("model_route_conflict")
var ErrModelRouteInvalid = errors.New("model_route_invalid")
var ErrModelRouteEndpoint = errors.New("model_route_endpoint_unsupported")

// ChannelModelRoute records only non-secret routing configuration. The digest
// binds the selected mapping; it is not proof of provider identity or pricing.
type ChannelModelRoute struct {
	RequestedModel string `json:"requested_model"`
	UpstreamModel  string `json:"upstream_model"`
	Endpoint       string `json:"endpoint"`
	Reason         string `json:"reason"`
	ChannelID      int    `json:"channel_id"`
	ConfigDigest   string `json:"config_digest"`
}

func IsBasicModelRouteChannel(channelType int) bool {
	return channelType == constant.ChannelTypeOpenAI || channelType == constant.ChannelTypeCodex
}

func ValidateChannelModelRoutes(channelType int, routes []dto.ModelRoute) error {
	if len(routes) == 0 {
		return nil
	}
	if !IsBasicModelRouteChannel(channelType) || len(routes) > 256 {
		return ErrModelRouteInvalid
	}
	for i, route := range routes {
		if route.PublicModel == "" || route.UpstreamModel == "" || len(route.PublicModel) > 255 || len(route.UpstreamModel) > 255 ||
			strings.TrimSpace(route.PublicModel) != route.PublicModel || strings.TrimSpace(route.UpstreamModel) != route.UpstreamModel ||
			strings.ContainsAny(route.PublicModel+route.UpstreamModel, "\r\n\t,") || (route.Match != "exact" && route.Match != "prefix") {
			return ErrModelRouteInvalid
		}
		if route.Endpoint != "" && route.Endpoint != "/v1/chat/completions" && route.Endpoint != "/v1/responses" {
			return ErrModelRouteEndpoint
		}
		if channelType == constant.ChannelTypeCodex && route.Endpoint == "/v1/chat/completions" {
			return ErrModelRouteEndpoint
		}
		for _, previous := range routes[:i] {
			if previous.PublicModel == route.PublicModel && previous.Match == route.Match && previous.Endpoint == route.Endpoint && previous.Priority == route.Priority && previous.UpstreamModel != route.UpstreamModel {
				return ErrModelRouteConflict
			}
		}
	}
	return nil
}

// ResolveChannelModelRoute is shared by candidate selection, preview and the
// final selected-channel dispatch. Existing exact ModelMapping wins over rules.
// Callers must separately enforce enabled Models, groups and caller permissions.
func ResolveChannelModelRoute(channel *Channel, requested, endpoint string) (ChannelModelRoute, error) {
	if channel == nil {
		return ChannelModelRoute{}, ErrModelRouteInvalid
	}
	result := ChannelModelRoute{RequestedModel: requested, UpstreamModel: requested, Endpoint: endpoint, Reason: "configured_model", ChannelID: channel.Id}
	var settings dto.ChannelOtherSettings
	if channel.OtherSettings != "" {
		if err := common.UnmarshalJsonStr(channel.OtherSettings, &settings); err != nil {
			return result, ErrModelRouteInvalid
		}
	}
	if err := ValidateChannelModelRoutes(channel.Type, settings.ModelRoutes); err != nil {
		return result, err
	}
	var mapping map[string]string
	if raw := channel.GetModelMapping(); raw != "" {
		if err := common.UnmarshalJsonStr(raw, &mapping); err != nil {
			return result, ErrModelRouteInvalid
		}
	}
	if target := mapping[requested]; target != "" {
		visited := map[string]bool{requested: true}
		current := requested
		for mapping[current] != "" && mapping[current] != current {
			next := mapping[current]
			if visited[next] {
				return result, ErrModelRouteConflict
			}
			visited[next] = true
			current = next
		}
		result.UpstreamModel = current
		result.Reason = "explicit_mapping"
	} else {
		best := -1
		publicMatch := false
		bestRank := [3]int{}
		for i, rule := range settings.ModelRoutes {
			matches := rule.Match == "exact" && requested == rule.PublicModel || rule.Match == "prefix" && strings.HasPrefix(requested, rule.PublicModel)
			if matches {
				if endpoint != "/v1/chat/completions" && endpoint != "/v1/responses" || channel.Type == constant.ChannelTypeCodex && endpoint != "/v1/responses" {
					return result, ErrModelRouteEndpoint
				}
				publicMatch = true
			}
			if !matches || rule.Endpoint != "" && rule.Endpoint != endpoint {
				continue
			}
			rank := [3]int{0, 0, len(rule.PublicModel)}
			if rule.Match == "exact" {
				rank[0] = 1
			}
			if rule.Endpoint != "" {
				rank[1] = 1
			}
			comparison := 0
			for j := 0; j < len(rank); j++ {
				if rank[j] > bestRank[j] {
					comparison = 1
					break
				}
				if rank[j] < bestRank[j] {
					comparison = -1
					break
				}
			}
			if best < 0 || comparison > 0 || comparison == 0 && rule.Priority > settings.ModelRoutes[best].Priority {
				best = i
				bestRank = rank
				continue
			}
			if comparison == 0 && rule.Priority == settings.ModelRoutes[best].Priority && rule.UpstreamModel != settings.ModelRoutes[best].UpstreamModel {
				return result, ErrModelRouteConflict
			}
		}
		if publicMatch && best < 0 {
			return result, ErrModelRouteEndpoint
		}
		if best >= 0 {
			result.UpstreamModel = settings.ModelRoutes[best].UpstreamModel
			result.Reason = "explicit_" + settings.ModelRoutes[best].Match
		}
	}
	if strings.TrimSpace(result.UpstreamModel) == "" || len(result.UpstreamModel) > 255 {
		return result, ErrModelRouteInvalid
	}
	payload, err := common.Marshal(struct {
		Type    int
		Models  string
		Mapping string
		Routes  []dto.ModelRoute
	}{channel.Type, channel.Models, channel.GetModelMapping(), settings.ModelRoutes})
	if err != nil {
		return result, err
	}
	digest := sha256.Sum256(payload)
	result.ConfigDigest = hex.EncodeToString(digest[:])
	return result, nil
}

// ChannelRoutingConfigDigest is a concurrency token for the non-secret routing
// fields edited together. Credentials, observations and accounting are excluded.
func ChannelRoutingConfigDigest(channel *Channel) string {
	payload, _ := common.Marshal(struct {
		Type                             int
		Models, Group, Mapping, Settings string
		Priority                         int64
		Weight                           int
	}{
		channel.Type, channel.Models, channel.Group, channel.GetModelMapping(), channel.OtherSettings, channel.GetPriority(), channel.GetWeight()})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

var ErrModelRouteConfigChanged = errors.New("model_route_config_changed")
