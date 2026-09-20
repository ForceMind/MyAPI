package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/setting"
	"github.com/ForceMind/MyAPI/setting/config"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func accessProfileTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := DB
	DB = db
	t.Cleanup(func() { DB = oldDB; require.NoError(t, sqlDB.Close()) })
	return db
}

func TestAccessProfileMigrationPreservesExplicitIdentities(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	for i, id := range []string{"standard", "team-custom"} {
		user := User{Id: i + 1, Username: id, AffCode: id, Group: "vip", AccountTierID: id}
		token := Token{Id: i + 1, Key: id, Group: "auto", AccessProfileID: id}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&token).Error)
	}
	// Two startup runs must not re-derive an explicit standard/custom identity.
	require.NoError(t, MigrateAccessProfileIdentifiers())
	require.NoError(t, MigrateAccessProfileIdentifiers())
	for i, id := range []string{"standard", "team-custom"} {
		var user User
		var token Token
		require.NoError(t, db.First(&user, i+1).Error)
		require.NoError(t, db.First(&token, i+1).Error)
		assert.Equal(t, id, user.AccountTierID)
		assert.Equal(t, id, token.AccessProfileID)
	}
}

func TestAccessProfileLegacyColumnIntroductionRecovers(t *testing.T) {
	for _, state := range []string{"legacy", "partial-schema", "backfill-failure"} {
		t.Run(state, func(t *testing.T) {
			db := accessProfileTestDB(t)
			require.NoError(t, db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, `group` TEXT)").Error)
			require.NoError(t, db.Exec("CREATE TABLE tokens (id INTEGER PRIMARY KEY, `group` TEXT)").Error)
			require.NoError(t, db.Exec("INSERT INTO users (id, `group`) VALUES (1, 'vip'), (2, 'default')").Error)
			require.NoError(t, db.Exec("INSERT INTO tokens (id, `group`) VALUES (1, 'auto'), (2, 'default')").Error)
			if state == "partial-schema" {
				// A previously added column can contain both a deliberate identity
				// and NULL rows. Neither a migration marker nor group can disambiguate
				// a non-empty standard value; preserve it.
				require.NoError(t, db.Exec("ALTER TABLE users ADD COLUMN account_tier_id varchar(64)").Error)
				require.NoError(t, db.Exec("UPDATE users SET account_tier_id = 'standard' WHERE id = 1").Error)
			}
			if state == "backfill-failure" {
				writeErr := errors.New("injected token identity backfill failure")
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail-identity", func(tx *gorm.DB) {
					if tx.Statement.Table == "tokens" {
						tx.AddError(writeErr)
					}
				}))
				assert.ErrorIs(t, prepareAccessProfileIdentifiers(), writeErr)
				var missing int64
				require.NoError(t, db.Table("users").Where("account_tier_id IS NULL").Count(&missing).Error)
				assert.Equal(t, int64(2), missing, "transaction must roll back earlier user backfills")
				require.NoError(t, db.Callback().Update().Remove("test:fail-identity"))
			}
			require.NoError(t, prepareAccessProfileIdentifiers())
			require.NoError(t, prepareAccessProfileIdentifiers())
			var users []User
			var tokens []Token
			require.NoError(t, db.Unscoped().Order("id").Find(&users).Error)
			require.NoError(t, db.Unscoped().Order("id").Find(&tokens).Error)
			require.Len(t, users, 2)
			require.Len(t, tokens, 2)
			wantTier := "priority"
			if state == "partial-schema" {
				wantTier = "standard"
			}
			assert.Equal(t, wantTier, users[0].AccountTierID)
			assert.Equal(t, "standard", users[1].AccountTierID)
			assert.Equal(t, "automatic", tokens[0].AccessProfileID)
			assert.Equal(t, "standard", tokens[1].AccessProfileID)
			assert.Equal(t, "vip", users[0].Group)
			assert.Equal(t, "auto", tokens[0].Group)
		})
	}
}

func TestAccessProfileStartupMigratesLegacyBeforeApplyingDefaults(t *testing.T) {
	for _, startup := range []struct {
		name string
		run  func() error
	}{{"normal", migrateDB}, {"fast", migrateDBFast}} {
		t.Run(startup.name, func(t *testing.T) {
			db := accessProfileTestDB(t)
			// Use a realistic pre-identity schema, including existing unique key
			// columns which SQLite cannot add to a populated table via ALTER.
			require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
			require.NoError(t, db.Create(&User{Id: 1, Username: "legacy-startup", Password: "fixture-password", AffCode: "legacy-startup", Group: "vip"}).Error)
			require.NoError(t, db.Create(&Token{Id: 1, Key: "legacy-startup-token", Group: "auto"}).Error)
			require.NoError(t, db.Migrator().DropColumn(&User{}, "AccountTierID"))
			require.NoError(t, db.Migrator().DropColumn(&Token{}, "AccessProfileID"))
			require.NoError(t, startup.run())
			var user User
			var token Token
			require.NoError(t, db.First(&user, 1).Error)
			require.NoError(t, db.First(&token, 1).Error)
			assert.Equal(t, "priority", user.AccountTierID)
			assert.Equal(t, "automatic", token.AccessProfileID)
			require.NoError(t, db.Model(&user).Update("account_tier_id", "standard").Error)
			require.NoError(t, db.Model(&token).Update("access_profile_id", "custom").Error)
			require.NoError(t, startup.run())
			require.NoError(t, db.First(&user, 1).Error)
			require.NoError(t, db.First(&token, 1).Error)
			assert.Equal(t, "standard", user.AccountTierID)
			assert.Equal(t, "custom", token.AccessProfileID)
		})
	}
}

func TestAccessProfileLegacyGroupEditsRemainCompatible(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &UserSession{}))
	user := User{Username: "legacy-group-edit", AffCode: "legacy-edit", Group: "default", AccountTierID: "custom"}
	require.NoError(t, db.Create(&user).Error)
	edit := User{Id: user.Id, Username: user.Username, Group: "vip"}
	require.NoError(t, edit.Edit(false))
	var updated User
	require.NoError(t, db.First(&updated, user.Id).Error)
	assert.Equal(t, "vip", updated.Group)
	assert.Equal(t, "priority", updated.AccountTierID)

	token := Token{Key: "legacy-group-edit-token", Group: "default", AccessProfileID: "custom"}
	require.NoError(t, token.Insert())
	token.Group = "auto"
	token.AccessProfileID = "" // Old client does not send the new identity.
	require.NoError(t, token.Update())
	var updatedToken Token
	require.NoError(t, db.First(&updatedToken, token.Id).Error)
	assert.Equal(t, "auto", updatedToken.Group)
	assert.Equal(t, "automatic", updatedToken.AccessProfileID)
}

func TestUpdateOptionPersistsCanonicalAccessProfiles(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	original, err := common.Marshal(setting.GetAccessProfileSetting().Profiles)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	previousMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(string(original)))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})
	const key = "access_profile_setting.profiles"
	require.NoError(t, UpdateOption(key, `{" standard ":{"label":"Standard","fallback_profiles":[" priority "]}," priority ":{"label":"Priority"}}`))
	var stored Option
	require.NoError(t, db.First(&stored, Option{Key: key}).Error)
	var profiles map[string]setting.AccessProfileDefinition
	require.NoError(t, common.UnmarshalJsonStr(stored.Value, &profiles))
	assert.Contains(t, profiles, "standard")
	assert.NotContains(t, profiles, " standard ")
	assert.Equal(t, []string{"priority"}, profiles["standard"].FallbackProfiles)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, stored.Value, common.OptionMap[key])
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, stored.Value, config.GlobalConfig.ExportAllConfigs()[key])
	assert.ErrorContains(t, UpdateOption(key, `{"standard":{"label":"One"}," standard ":{"label":"Two"}}`), "unique after trimming")
	var unchanged Option
	require.NoError(t, db.First(&unchanged, Option{Key: key}).Error)
	assert.Equal(t, stored.Value, unchanged.Value)
}

func TestUpdateOptionPersistsCanonicalAccountTiers(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	original, err := common.Marshal(setting.GetAccessProfileSetting().AccountTiers)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	previousMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAccountTierDefinitionsByJSONString(string(original)))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})
	const key = "access_profile_setting.account_tiers"
	require.NoError(t, UpdateOption(key, `{" standard ":{"label":"Standard","route_groups":[" default "]}}`))
	var stored Option
	require.NoError(t, db.First(&stored, Option{Key: key}).Error)
	var tiers map[string]setting.AccountTierDefinition
	require.NoError(t, common.UnmarshalJsonStr(stored.Value, &tiers))
	assert.Contains(t, tiers, "standard")
	assert.NotContains(t, tiers, " standard ")
	assert.Equal(t, []string{"default"}, tiers["standard"].RouteGroups)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, stored.Value, common.OptionMap[key])
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, stored.Value, config.GlobalConfig.ExportAllConfigs()[key])
	assert.ErrorContains(t, UpdateOption(key, `{"standard":{"label":"One"}," standard ":{"label":"Two"}}`), "unique after trimming")
	var unchanged Option
	require.NoError(t, db.First(&unchanged, Option{Key: key}).Error)
	assert.Equal(t, stored.Value, unchanged.Value)
}

func TestOptionReloadPublishesAccountTierAndProfileRegistry(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	original := setting.GetAccessProfileSetting()
	originalProfiles, err := common.Marshal(original.Profiles)
	require.NoError(t, err)
	originalTiers, err := common.Marshal(original.AccountTiers)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	previousMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"access_profile_setting.profiles":      string(originalProfiles),
			"access_profile_setting.account_tiers": string(originalTiers),
		}))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, db.Create(&Option{Key: "access_profile_setting.profiles", Value: `{"standard":{"label":"Reloaded profile","route_groups":["group-a"]}}`}).Error)
	require.NoError(t, db.Create(&Option{Key: "access_profile_setting.account_tiers", Value: `{"standard":{"label":"Reloaded tier","model_allowlist":["gpt-5"]}}`}).Error)

	loadOptionsFromDatabase()

	registry := setting.GetAccessProfileSetting()
	assert.Equal(t, "Reloaded profile", registry.Profiles["standard"].Label)
	assert.Equal(t, []string{"group-a"}, registry.Profiles["standard"].RouteGroups)
	assert.Equal(t, "Reloaded tier", registry.AccountTiers["standard"].Label)
	assert.Equal(t, []string{"gpt-5"}, registry.AccountTiers["standard"].ModelAllowlist)
}

func TestUpdateOptionPersistenceFailureDoesNotPublish(t *testing.T) {
	for _, failure := range []string{"create-new", "save-new", "save-existing"} {
		t.Run(failure, func(t *testing.T) {
			db := accessProfileTestDB(t)
			require.NoError(t, db.AutoMigrate(&Option{}))
			const key = "access_profile_setting.profiles"
			const old = `{"standard":{"label":"Before"}}`
			const next = `{"standard":{"label":"After"}}`
			original, err := common.Marshal(setting.GetAccessProfileSetting().Profiles)
			require.NoError(t, err)
			common.OptionMapRWMutex.Lock()
			previousMap := common.OptionMap
			common.OptionMap = map[string]string{key: old}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(string(original)))
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousMap
				common.OptionMapRWMutex.Unlock()
			})
			require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(old))
			if failure == "save-existing" {
				require.NoError(t, db.Create(&Option{Key: key, Value: old}).Error)
			}
			writeErr := errors.New("injected option write failure")
			if failure == "create-new" {
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-option-create", func(tx *gorm.DB) { tx.AddError(writeErr) }))
			} else {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail-option-save", func(tx *gorm.DB) { tx.AddError(writeErr) }))
			}
			assert.ErrorIs(t, UpdateOption(key, next), writeErr)
			common.OptionMapRWMutex.RLock()
			assert.Equal(t, old, common.OptionMap[key])
			common.OptionMapRWMutex.RUnlock()
			profile, ok := setting.GetAccessProfileDefinition("standard")
			require.True(t, ok)
			assert.Equal(t, "Before", profile.Label)
			var stored Option
			err = db.First(&stored, Option{Key: key}).Error
			if failure == "save-existing" {
				require.NoError(t, err)
				assert.Equal(t, old, stored.Value)
			} else {
				assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
			}
		})
	}
}

func TestUpdateOptionAccessProfileConcurrentReadAndExport(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	original, err := common.Marshal(setting.GetAccessProfileSetting().Profiles)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	previousMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAccessProfileDefinitionsByJSONString(string(original)))
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		<-start
		assert.NoError(t, UpdateOption("access_profile_setting.profiles", `{"standard":{"label":"Updated"}}`))
	}()
	go func() {
		defer wg.Done()
		<-start
		setting.GetAccessProfileDefinition("standard")
		setting.GetAccessProfileSetting()
	}()
	go func() {
		defer wg.Done()
		<-start
		config.GlobalConfig.ExportAllConfigs()
	}()
	close(start)
	wg.Wait()
	profile, ok := setting.GetAccessProfileDefinition("standard")
	require.True(t, ok)
	assert.Equal(t, "Updated", profile.Label)
	var exported map[string]setting.AccessProfileDefinition
	require.NoError(t, common.UnmarshalJsonStr(config.GlobalConfig.ExportAllConfigs()["access_profile_setting.profiles"], &exported))
	assert.Equal(t, profile, exported["standard"])
}
