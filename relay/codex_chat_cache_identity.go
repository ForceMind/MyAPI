package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
)

// deriveCodexChatCacheIdentity is deliberately independent of channel IDs and
// credentials. Only the stable account ID enters the caller-supplied namespace.
// A history append cannot change the initial instruction/user prefix.
func deriveCodexChatCacheIdentity(request *dto.GeneralOpenAIRequest, userID, tokenID int, accountID, session string) (string, string) {
	if request == nil || userID <= 0 || tokenID <= 0 || strings.TrimSpace(accountID) == "" {
		return "", ""
	}
	seed := struct {
		Version      string                `json:"version"`
		UserID       int                   `json:"user_id"`
		TokenID      int                   `json:"token_id"`
		AccountID    string                `json:"account_id"`
		ExplicitKey  string                `json:"explicit_key,omitempty"`
		Session      string                `json:"session,omitempty"`
		Model        string                `json:"model,omitempty"`
		Instructions []dto.Message         `json:"instructions,omitempty"`
		Tools        []dto.ToolCallRequest `json:"tools,omitempty"`
		Functions    any                   `json:"functions,omitempty"`
		FirstUser    *dto.Message          `json:"first_user,omitempty"`
	}{Version: "myapi-codex-chat-v1", UserID: userID, TokenID: tokenID, AccountID: strings.TrimSpace(accountID)}
	// Explicit identities define the conversation even if a client truncates
	// its history. The explicit cache key itself is never rewritten.
	switch {
	case request.PromptCacheKey != "":
		seed.ExplicitKey = request.PromptCacheKey
	case session != "":
		seed.Session = session
	default:
		seed.Model, seed.Tools = request.Model, request.Tools
		if len(request.Functions) > 0 {
			seed.Functions = request.Functions
		}
		for i := range request.Messages {
			message := request.Messages[i]
			if message.Role == "user" {
				seed.FirstUser = &message
				break
			}
			if message.Role == "system" || message.Role == "developer" {
				seed.Instructions = append(seed.Instructions, message)
			}
		}
		if seed.FirstUser == nil || seed.Model == "" {
			return "", ""
		}
	}
	encoded, err := common.Marshal(seed)
	if err != nil {
		return "", ""
	}
	// Canonical hashing also sorts object members in raw function schemas and
	// multimodal content. Oversized/invalid optional identity seeds are skipped;
	// they never cause the generation request to fail or expose input in errors.
	digest, err := common.CanonicalJSONObjectDigest(encoded)
	if err != nil {
		return "", ""
	}
	key := request.PromptCacheKey
	if key == "" {
		key = "myapi-codex-" + hex.EncodeToString(digest[:])
	}
	sessionDigest := sha256.Sum256(append([]byte("myapi-codex-session-v1:"), digest[:]...))
	return key, hex.EncodeToString(sessionDigest[:])
}
