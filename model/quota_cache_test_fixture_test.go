package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Fixture seeding uses the same ownership checks as real quota recovery. Tests
// that exercise delayed readers or metadata refresh still use unowned writes.
func seedTokenQuotaCacheForTest(t *testing.T, token Token) {
	t.Helper()
	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeTokenQuota, token.Id)
	require.NoError(t, err)
	defer releaseQuotaBalanceSubjectLock(lock)
	_, err = cacheInitTokenWithQuotaBalanceOwner(token, lock)
	require.NoError(t, err)
}

func seedUserQuotaCacheForTest(t *testing.T, user *UserBase) {
	t.Helper()
	lock, err := acquireQuotaBalanceSubjectLock(BatchUpdateTypeUserQuota, user.Id)
	require.NoError(t, err)
	defer releaseQuotaBalanceSubjectLock(lock)
	require.NoError(t, writeUserCacheWithQuotaBalanceOwner(user, true, lock))
}
