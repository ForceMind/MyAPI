package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service/authz"
)

var ErrManagementKeyInput = errors.New("invalid management key input")

var ManagementKeyScopes = []string{"channels.read", "channels.operate", "tokens.read", "logs.read", "system.read"}

type CreateManagementKeyInput struct {
	Name       string   `json:"name"`
	Scopes     []string `json:"scopes"`
	ChannelIDs []int    `json:"channel_ids"`
	TokenIDs   []int    `json:"token_ids"`
	ExpiresAt  int64    `json:"expires_at"`
}

// ManagementScopeAvailable also reads persisted channel policies, so revocation
// on a different instance cannot remain authorized by an in-process enforcer.
func ManagementScopeAvailable(user *model.UserBase, scope string) bool {
	if user == nil || user.Status != common.UserStatusEnabled || user.Role < common.RoleAdminUser {
		return false
	}
	switch scope {
	case "tokens.read", "logs.read", "system.read":
		return true
	case "channels.read", "channels.operate":
		permission := authz.ChannelRead
		if scope == "channels.operate" {
			permission = authz.ChannelOperate
		}
		if !authz.Can(user.Id, user.Role, permission) {
			return false
		}
		if user.Role >= common.RoleRootUser {
			return true
		}
		var rows []model.CasbinRule
		if err := model.DB.Where("ptype = ? AND v0 IN ? AND v1 = ? AND v2 = ?", "p", []string{authz.UserSubject(user.Id), authz.RoleSubject(authz.BuiltInRoleAdmin)}, permission.Resource, permission.Action).Find(&rows).Error; err != nil {
			return false
		}
		for _, subject := range []string{authz.UserSubject(user.Id), authz.RoleSubject(authz.BuiltInRoleAdmin)} {
			allow := false
			for _, row := range rows {
				if row.V0 != subject {
					continue
				}
				if row.V3 == authz.EffectDeny {
					return false
				}
				if row.V3 == "" || row.V3 == authz.EffectAllow {
					allow = true
				}
			}
			if allow {
				return true
			}
		}
	}
	return false
}

func CreateManagementKey(owner *model.UserBase, input CreateManagementKeyInput) (model.ManagementKeyMetadata, string, error) {
	var empty model.ManagementKeyMetadata
	input.Name = strings.TrimSpace(input.Name)
	now := time.Now().Unix()
	if input.Name == "" || len(input.Name) > 128 || len(input.Scopes) == 0 || len(input.Scopes) > len(ManagementKeyScopes) || len(input.ChannelIDs) > 1000 || len(input.TokenIDs) > 1000 {
		return empty, "", ErrManagementKeyInput
	}
	if input.ExpiresAt == 0 {
		input.ExpiresAt = now + 7*86400
	}
	if input.ExpiresAt <= now || input.ExpiresAt > now+90*86400 {
		return empty, "", ErrManagementKeyInput
	}
	seen := map[string]bool{}
	for _, scope := range input.Scopes {
		if seen[scope] || !ManagementScopeAvailable(owner, scope) {
			return empty, "", ErrManagementKeyInput
		}
		seen[scope] = true
	}
	sort.Strings(input.Scopes)
	if input.ChannelIDs == nil {
		input.ChannelIDs = []int{}
	}
	if input.TokenIDs == nil {
		input.TokenIDs = []int{}
	}
	for _, ids := range [][]int{input.ChannelIDs, input.TokenIDs} {
		seenIDs := map[int]bool{}
		for _, id := range ids {
			if id <= 0 || seenIDs[id] {
				return empty, "", ErrManagementKeyInput
			}
			seenIDs[id] = true
		}
		sort.Ints(ids)
	}
	// Token routes are owner scoped; keys cannot grant another user's token.
	if len(input.TokenIDs) > 0 {
		var count int64
		if err := model.DB.Model(&model.Token{}).Where("id IN ? AND user_id = ?", input.TokenIDs, owner.Id).Count(&count).Error; err != nil {
			return empty, "", err
		}
		if count != int64(len(input.TokenIDs)) {
			return empty, "", ErrManagementKeyInput
		}
	}
	if len(input.ChannelIDs) > 0 {
		var count int64
		if err := model.DB.Model(&model.Channel{}).Where("id IN ?", input.ChannelIDs).Count(&count).Error; err != nil {
			return empty, "", err
		}
		if count != int64(len(input.ChannelIDs)) {
			return empty, "", ErrManagementKeyInput
		}
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return empty, "", err
	}
	secret := model.ManagementKeyPrefix + hex.EncodeToString(random)
	scopes, err := common.Marshal(input.Scopes)
	if err != nil {
		return empty, "", err
	}
	channels, err := common.Marshal(input.ChannelIDs)
	if err != nil {
		return empty, "", err
	}
	tokens, err := common.Marshal(input.TokenIDs)
	if err != nil {
		return empty, "", err
	}
	key := model.ManagementKey{OwnerID: owner.Id, Name: input.Name, Prefix: secret[:len(model.ManagementKeyPrefix)+8], SecretHash: model.ManagementKeyDigest(secret), ScopesJSON: string(scopes), ChannelIDsJSON: string(channels), TokenIDsJSON: string(tokens), CreatedAt: now, ExpiresAt: input.ExpiresAt}
	if err := model.InsertManagementKey(&key); err != nil {
		return empty, "", err
	}
	metadata, err := key.Metadata()
	return metadata, secret, err
}
