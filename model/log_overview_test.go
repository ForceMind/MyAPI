package model

import (
	"fmt"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRecentLogOverviewIsBoundedAndRedactsLogBodies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Log{}, &BillingLogProjectionIdentity{}))
	for index := 0; index < 7; index++ {
		require.NoError(t, db.Create(&Log{
			UserId: 1, Username: "alice", Type: LogTypeConsume,
			CreatedAt: int64(100 + index), ModelName: fmt.Sprintf("gpt-%d", index), Quota: index + 1,
			Content: "private prompt body", Other: "private metadata", TokenName: "private token",
		}).Error)
	}
	require.NoError(t, db.Create(&Log{UserId: 1, Username: "alice", Type: LogTypeError,
		CreatedAt: 108, ModelName: "failed-model", Content: "private upstream error", Other: "private error metadata",
	}).Error)
	require.NoError(t, db.Create(&Log{UserId: 2, Username: "bob", Type: LogTypeConsume,
		CreatedAt: 200, ModelName: "other-user-model", Content: "other private prompt",
	}).Error)
	require.NoError(t, db.Create(&Log{UserId: 2, Username: "bob", Type: LogTypeError,
		CreatedAt: 201, ModelName: "other-user-error", Content: "other private error",
	}).Error)

	self, err := GetUserRecentLogOverview(db, 1)
	require.NoError(t, err)
	require.Len(t, self.Requests, 5)
	assert.Positive(t, self.Requests[0].ID)
	assert.Equal(t, "gpt-6", self.Requests[0].ModelName)
	assert.Equal(t, "gpt-2", self.Requests[4].ModelName)
	require.Len(t, self.Errors, 1)
	assert.Equal(t, "failed-model", self.Errors[0].ModelName)
	assert.Empty(t, self.Requests[0].Username)
	encoded, err := common.Marshal(self)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "alice")
	assert.NotContains(t, string(encoded), "bob")
	assert.NotContains(t, string(encoded), "private")
	assert.NotContains(t, string(encoded), "other-user")
	assert.NotContains(t, string(encoded), `"quota"`)

	admin, err := GetAdminRecentLogOverview(db)
	require.NoError(t, err)
	require.Len(t, admin.Requests, 5)
	assert.Equal(t, "other-user-model", admin.Requests[0].ModelName)
	assert.Equal(t, "bob", admin.Requests[0].Username)
	require.Len(t, admin.Errors, 2)
	assert.Equal(t, "other-user-error", admin.Errors[0].ModelName)
	assert.Equal(t, "bob", admin.Errors[0].Username)
	encoded, err = common.Marshal(admin)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private")
}

func TestUserRecentLogOverviewRejectsMissingIdentity(t *testing.T) {
	_, err := GetUserRecentLogOverview(nil, 0)
	require.Error(t, err)
}
