package model

import (
	"path/filepath"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyChannelWithoutTestOptions struct {
	Id     int `gorm:"primaryKey"`
	Type   int
	Key    string
	Name   string
	Models string
}

func (legacyChannelWithoutTestOptions) TableName() string { return "channels" }

func TestLastSuccessfulChannelTestOptionsPersistPerChannelAndReplaceOnlyThatChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})

	first := Channel{Type: constant.ChannelTypeCodex, Name: "codex", Key: "private-key", Models: "gpt-5-codex"}
	second := Channel{Type: constant.ChannelTypeCodex, Name: "other", Key: "other-private-key", Models: "gpt-5-codex"}
	require.NoError(t, db.Create(&first).Error)
	require.NoError(t, db.Create(&second).Error)

	initial, err := GetLastSuccessfulChannelTestOptions(first.Id)
	require.NoError(t, err)
	assert.Nil(t, initial)

	expected := ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai-response", Stream: true, ChannelType: constant.ChannelTypeCodex}
	require.NoError(t, SaveLastSuccessfulChannelTestOptions(first.Id, expected))
	require.NoError(t, SaveLastSuccessfulChannelTestOptions(first.Id, expected), "repeating the same successful choices is idempotent")
	loaded, err := GetLastSuccessfulChannelTestOptions(first.Id)
	require.NoError(t, err)
	assert.Equal(t, &expected, loaded)
	other, err := GetLastSuccessfulChannelTestOptions(second.Id)
	require.NoError(t, err)
	assert.Nil(t, other)

	replacement := ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "", Stream: false, ChannelType: constant.ChannelTypeCodex}
	require.NoError(t, SaveLastSuccessfulChannelTestOptions(first.Id, replacement))
	loaded, err = GetLastSuccessfulChannelTestOptions(first.Id)
	require.NoError(t, err)
	assert.Equal(t, &replacement, loaded)
	assert.Error(t, SaveLastSuccessfulChannelTestOptions(99999, expected))
}

func TestLastSuccessfulChannelTestOptionsSurviveSQLiteReopen(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "channels.db")
	db, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	channel := Channel{Type: constant.ChannelTypeCodex, Name: "reopened-codex", Key: "private-key", Models: "gpt-5-codex"}
	require.NoError(t, db.Create(&channel).Error)
	expected := ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai-response", Stream: true, ChannelType: channel.Type}
	require.NoError(t, SaveLastSuccessfulChannelTestOptions(channel.Id, expected))
	require.NoError(t, sqlDB.Close())

	reopened, err := gorm.Open(sqlite.Open(databasePath), &gorm.Config{})
	require.NoError(t, err)
	reopenedSQL, err := reopened.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopenedSQL.Close()) })
	DB = reopened
	loaded, err := GetLastSuccessfulChannelTestOptions(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, &expected, loaded)
}

func TestChannelTestOptionsMigrationPreservesExistingSQLiteChannel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&legacyChannelWithoutTestOptions{}))
	require.NoError(t, db.Create(&legacyChannelWithoutTestOptions{Id: 7, Type: constant.ChannelTypeCodex, Key: "existing-private-key", Name: "existing", Models: "gpt-5-codex"}).Error)
	assert.False(t, db.Migrator().HasColumn(&Channel{}, "last_successful_test_options"))

	require.NoError(t, db.AutoMigrate(&Channel{}))
	assert.True(t, db.Migrator().HasColumn(&Channel{}, "last_successful_test_options"))
	var channel Channel
	require.NoError(t, db.First(&channel, "id = ?", 7).Error)
	assert.Equal(t, "existing-private-key", channel.Key)
	assert.Empty(t, channel.LastSuccessfulTestOptions)
}

func TestLastSuccessfulChannelTestOptionsRejectsModelOrTypeChangedAfterTest(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		require.NoError(t, sqlDB.Close())
	})
	channel := Channel{Type: constant.ChannelTypeCodex, Name: "changing-channel", Key: "private-key", Models: "gpt-5-codex"}
	require.NoError(t, db.Create(&channel).Error)
	previous := ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai-response", Stream: true, ChannelType: channel.Type}
	require.NoError(t, SaveLastSuccessfulChannelTestOptions(channel.Id, previous))
	stale := ChannelTestOptions{Model: "gpt-5-codex", EndpointType: "openai", Stream: false, ChannelType: channel.Type}

	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Update("models", "gpt-5").Error)
	require.Error(t, SaveLastSuccessfulChannelTestOptions(channel.Id, stale))
	loaded, err := GetLastSuccessfulChannelTestOptions(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, &previous, loaded)

	require.NoError(t, db.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"models": "gpt-5-codex", "type": constant.ChannelTypeOpenAI,
	}).Error)
	require.Error(t, SaveLastSuccessfulChannelTestOptions(channel.Id, stale))
	loaded, err = GetLastSuccessfulChannelTestOptions(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, &previous, loaded)
}

func TestChannelEditFromStaleSnapshotDoesNotOverwriteSuccessfulTestOptions(t *testing.T) {
	_ = setupChannelOperationTest(t)
	channel := operationFixtureChannel("test-options-edit")
	require.NoError(t, channel.Insert())
	stale := channel

	expected := ChannelTestOptions{Model: "operation-model", EndpointType: "openai-response", Stream: true, ChannelType: channel.Type}
	require.NoError(t, SaveLastSuccessfulChannelTestOptions(channel.Id, expected))
	stale.Name = "renamed-after-test"
	require.NoError(t, stale.Update())

	loaded, err := GetLastSuccessfulChannelTestOptions(channel.Id)
	require.NoError(t, err)
	assert.Equal(t, &expected, loaded)
}
