package model

import (
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// runB2TaskOperationQueryDatabaseContract runs in the protected disposable
// MySQL/PostgreSQL fixture after all production migration entries. It keeps
// the owner-query contract portable and independent from request middleware
// or Redis state.
func runB2TaskOperationQueryDatabaseContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	publicID, err := GenerateTaskSubmissionOperationPublicID()
	require.NoError(t, err)
	suffix := strings.TrimPrefix(publicID, "task_")
	allowIPs := "198.51.100.0/24\n2001:db8::/32"
	owner := &User{
		Username: "b2q" + suffix[:17],
		Password: "fixture-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusDisabled,
		AffCode:  "b2q" + suffix[:29],
	}
	require.NoError(t, db.Create(owner).Error)
	ownerToken := &Token{
		UserId:   owner.Id,
		Key:      "b2q-owner-" + suffix,
		Status:   common.TokenStatusExhausted,
		AllowIps: &allowIPs,
	}
	require.NoError(t, db.Create(ownerToken).Error)
	siblingToken := &Token{UserId: owner.Id, Key: "b2q-sibling-" + suffix, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(siblingToken).Error)
	deletedToken := &Token{UserId: owner.Id, Key: "b2q-deleted-" + suffix, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(deletedToken).Error)
	require.NoError(t, db.Delete(deletedToken).Error)

	identity, err := ReadTaskOperationAPIIdentity(ownerToken.Key)
	require.NoError(t, err)
	require.NotNil(t, identity)
	assert.Equal(t, ownerToken.Id, identity.TokenID)
	assert.Equal(t, owner.Id, identity.UserID)
	assert.Equal(t, common.TokenStatusExhausted, identity.TokenStatus)
	assert.Equal(t, common.UserStatusDisabled, identity.UserStatus)
	require.NotNil(t, identity.AllowIPs)
	assert.Equal(t, allowIPs, *identity.AllowIPs)

	deletedIdentity, err := ReadTaskOperationAPIIdentity(deletedToken.Key)
	require.NoError(t, err)
	assert.Nil(t, deletedIdentity)

	deletedUser := &User{
		Username: "b2s" + suffix[:17],
		Password: "fixture-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  "b2s" + suffix[:29],
	}
	require.NoError(t, db.Create(deletedUser).Error)
	activeDeletedUserToken := &Token{UserId: deletedUser.Id, Key: "b2q-deleted-user-" + suffix, Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(activeDeletedUserToken).Error)
	require.NoError(t, db.Delete(deletedUser).Error)
	deletedUserIdentity, err := ReadTaskOperationAPIIdentity(activeDeletedUserToken.Key)
	require.NoError(t, err)
	assert.Nil(t, deletedUserIdentity)

	createdAt, dispatchedAt, resolvedAt := int64(1_700_000_001), int64(1_700_000_002), int64(1_700_000_003)
	require.NoError(t, db.Exec(
		"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, lock_version, created_at, updated_at, dispatch_started_at, resolved_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		publicID, owner.Id, ownerToken.Id, "POST", TaskSubmissionOperationKindVideoCreate,
		strings.Repeat("a", taskRecoveryDigestLength), strings.Repeat("b", taskRecoveryDigestLength),
		TaskSubmissionOperationStatusSucceeded, int64(1), createdAt, resolvedAt, dispatchedAt, resolvedAt,
	).Error)

	sessionView, err := ReadTaskOperationForOwner(publicID, owner.Id, nil)
	require.NoError(t, err)
	require.NotNil(t, sessionView)
	assert.Equal(t, publicID, sessionView.PublicID)
	assert.Equal(t, TaskSubmissionOperationKindVideoCreate, sessionView.OperationKind)
	assert.Equal(t, TaskSubmissionOperationStatusSucceeded, sessionView.Status)

	ownerTokenID := ownerToken.Id
	tokenView, err := ReadTaskOperationForOwner(publicID, owner.Id, &ownerTokenID)
	require.NoError(t, err)
	require.NotNil(t, tokenView)
	assert.Equal(t, publicID, tokenView.PublicID)

	siblingTokenID := siblingToken.Id
	wrongTokenView, err := ReadTaskOperationForOwner(publicID, owner.Id, &siblingTokenID)
	require.NoError(t, err)
	assert.Nil(t, wrongTokenView)

	otherOwner := &User{
		Username: "b2r" + suffix[:17],
		Password: "fixture-password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		AffCode:  "b2r" + suffix[:29],
	}
	require.NoError(t, db.Create(otherOwner).Error)
	wrongSessionView, err := ReadTaskOperationForOwner(publicID, otherOwner.Id, nil)
	require.NoError(t, err)
	assert.Nil(t, wrongSessionView)

	unknownPublicID, err := GenerateTaskSubmissionOperationPublicID()
	require.NoError(t, err)
	unknown, err := ReadTaskOperationForOwner(unknownPublicID, owner.Id, nil)
	require.NoError(t, err)
	assert.Nil(t, unknown)

	invalidPublicID, err := GenerateTaskSubmissionOperationPublicID()
	require.NoError(t, err)
	require.NoError(t, db.Exec(
		"INSERT INTO task_submission_operations (public_id, user_id, token_id, http_method, operation_kind, idempotency_key_hash, request_fingerprint, status, lock_version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		invalidPublicID, owner.Id, ownerToken.Id, "POST", TaskSubmissionOperationKindVideoCreate,
		strings.Repeat("c", taskRecoveryDigestLength), strings.Repeat("d", taskRecoveryDigestLength),
		"invalid-public-status", int64(1), createdAt, resolvedAt,
	).Error)
	invalid, err := ReadTaskOperationForOwner(invalidPublicID, owner.Id, nil)
	assert.ErrorIs(t, err, ErrTaskOperationQueryUnavailable)
	assert.Nil(t, invalid)
}
