package model

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAssignedAccessPolicyDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/assigned-policy.db"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&AssignedAccessPolicy{}))
	require.NoError(t, db.AutoMigrate(&AssignedAccessPolicy{}))
	return db
}

func TestAssignedAccessPolicyLifecycleKeepsStrictRevisionsAndTombstones(t *testing.T) {
	assignedAccessPolicyLifecycleContract(t, setupAssignedAccessPolicyDB(t))
}

func assignedAccessPolicyLifecycleContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	ctx := context.Background()
	initial, err := ReadAssignedAccessPolicy(ctx, db, "user", 7)
	require.NoError(t, err)
	assert.Equal(t, AssignedAccessPolicy{SubjectType: "user", SubjectID: 7}, initial)
	var count int64
	require.NoError(t, db.Model(&AssignedAccessPolicy{}).Count(&count).Error)
	assert.Zero(t, count, "an unassigned read must not persist state")
	policy := `{"enabled":true,"public_models":null,"upstream_models":[],"channel_ids":[7]}`
	created, err := ConfigureAssignedAccessPolicy(ctx, db, "user", 7, 0, policy)
	require.NoError(t, err)
	assert.EqualValues(t, 1, created.Revision)
	assert.True(t, created.Assigned)
	assert.Equal(t, policy, created.PolicyJSON)
	_, err = ConfigureAssignedAccessPolicy(ctx, db, "user", 7, 0, policy)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyConflict, "even identical create is a stale CAS")
	_, err = ConfigureAssignedAccessPolicy(ctx, db, "token", 7, 0, `{"enabled":false}`)
	require.NoError(t, err, "user and token IDs occupy independent subject scopes")
	updated, err := ConfigureAssignedAccessPolicy(ctx, db, "user", 7, 1, `{"enabled":false}`)
	require.NoError(t, err)
	assert.EqualValues(t, 2, updated.Revision)
	assert.Equal(t, created.ID, updated.ID)
	_, err = RemoveAssignedAccessPolicy(ctx, db, "user", 7, 1)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyConflict)
	removed, err := RemoveAssignedAccessPolicy(ctx, db, "user", 7, 2)
	require.NoError(t, err)
	assert.False(t, removed.Assigned)
	assert.Empty(t, removed.PolicyJSON)
	assert.EqualValues(t, 3, removed.Revision)
	_, err = ConfigureAssignedAccessPolicy(ctx, db, "user", 7, 0, policy)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyConflict)
	_, err = ConfigureAssignedAccessPolicy(ctx, db, "user", 7, 2, policy)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyConflict)
	restored, err := ConfigureAssignedAccessPolicy(ctx, db, "user", 7, 3, policy)
	require.NoError(t, err)
	assert.EqualValues(t, 4, restored.Revision)
	assert.True(t, restored.Assigned)
	read, err := ReadAssignedAccessPolicy(ctx, db, "user", 7)
	require.NoError(t, err)
	assert.Equal(t, restored, read)
	token, err := ReadAssignedAccessPolicy(ctx, db, "token", 7)
	require.NoError(t, err)
	assert.EqualValues(t, 1, token.Revision)
	assert.Equal(t, `{"enabled":false}`, token.PolicyJSON)
}

func TestAssignedAccessPolicyRemoveAbsentPreventsStaleCreation(t *testing.T) {
	db := setupAssignedAccessPolicyDB(t)
	removed, err := RemoveAssignedAccessPolicy(nil, db, "token", 11, 0)
	require.NoError(t, err)
	assert.False(t, removed.Assigned)
	assert.EqualValues(t, 1, removed.Revision)
	read, err := ReadAssignedAccessPolicy(nil, db, "token", 11)
	require.NoError(t, err)
	assert.Equal(t, removed, read)
	_, err = ConfigureAssignedAccessPolicy(nil, db, "token", 11, 0, `{}`)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyConflict)
	_, err = RemoveAssignedAccessPolicy(nil, db, "user", 12, 9)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyConflict)
}

func TestAssignedAccessPolicyRejectsInvalidInputsAndCorruptStoredRows(t *testing.T) {
	db := setupAssignedAccessPolicyDB(t)
	for _, raw := range []string{"", "null", `[]`, `true`, `{"enabled":`, `{} {}`, "{\"x\":\"" + string([]byte{0xff}) + "\"}", `{"x":"` + strings.Repeat("a", MaxAssignedAccessPolicyJSONBytes) + `"}`} {
		_, err := ConfigureAssignedAccessPolicy(nil, db, "user", 7, 0, raw)
		require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
	}
	for _, test := range []struct {
		subject  string
		id       int
		revision int64
	}{
		{"user", 0, 0}, {"token", -1, 0}, {"User", 1, 0}, {"group", 1, 0},
		{"user", 7, -1}, {"user", 7, math.MaxInt64},
	} {
		_, err := ConfigureAssignedAccessPolicy(nil, db, test.subject, test.id, test.revision, `{}`)
		require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
		_, err = RemoveAssignedAccessPolicy(nil, db, test.subject, test.id, test.revision)
		require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
	}
	for index, row := range []AssignedAccessPolicy{
		{Assigned: true, Revision: 1, PolicyJSON: `{"enabled":`},
		{Assigned: true, Revision: 1, PolicyJSON: `null`},
		{Assigned: true, Revision: 0, PolicyJSON: `{}`},
		{Assigned: false, Revision: 1, PolicyJSON: `{}`},
	} {
		row.SubjectType, row.SubjectID = "user", index+1
		require.NoError(t, db.Create(&row).Error)
		_, err := ReadAssignedAccessPolicy(nil, db, row.SubjectType, row.SubjectID)
		require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
	}
	_, err := ReadAssignedAccessPolicy(nil, nil, "user", 7)
	require.ErrorIs(t, err, gorm.ErrInvalidDB)
	_, err = ReadAssignedAccessPolicy(nil, db, "user", -1)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ReadAssignedAccessPolicy(ctx, db, "user", 7)
	require.ErrorIs(t, err, context.Canceled)
}

func TestAssignedAccessPolicyConcurrentCreateAndUpdateHaveOneWinner(t *testing.T) {
	assignedAccessPolicyConcurrentCASContract(t, setupAssignedAccessPolicyDB(t))
}

func assignedAccessPolicyConcurrentCASContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, revision := range []int64{0, 1} {
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, policy := range []string{`{"enabled":true}`, `{"enabled":false}`} {
			go func(raw string) {
				<-start
				_, err := ConfigureAssignedAccessPolicy(context.Background(), db, "token", 11, revision, raw)
				results <- err
			}(policy)
		}
		close(start)
		first, second := <-results, <-results
		if first == nil {
			require.ErrorIs(t, second, ErrAssignedAccessPolicyConflict)
		} else {
			require.ErrorIs(t, first, ErrAssignedAccessPolicyConflict)
			require.NoError(t, second)
		}
	}
	row, err := ReadAssignedAccessPolicy(nil, db, "token", 11)
	require.NoError(t, err)
	assert.EqualValues(t, 2, row.Revision)
}

// Opt-in disposable services use the existing S1 fixture safety checks. No
// tables are dropped, and an already populated assignment schema is refused.
func TestAssignedAccessPolicyConfiguredDatabases(t *testing.T) {
	if os.Getenv("MYAPI_S1_DATABASE_TESTS") != "1" {
		t.Skip("disposable database tests are disabled; set MYAPI_S1_DATABASE_TESTS=1 explicitly")
	}
	for _, engine := range []struct{ name, env string }{
		{"mysql", "MYAPI_S1_MYSQL_DSN"},
		{"postgres", "MYAPI_S1_POSTGRES_DSN"},
	} {
		t.Run(engine.name, func(t *testing.T) {
			dsn := os.Getenv(engine.env)
			if dsn == "" {
				t.Skip(engine.env + " is not configured")
			}
			dialect, err := s1DatabaseDialector(engine.name, dsn)
			require.NoError(t, err)
			db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(2)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			tables, err := db.Migrator().GetTables()
			require.NoError(t, err)
			require.NotContains(t, tables, "assigned_access_policies", "refusing a populated assignment fixture schema")
			require.NoError(t, db.AutoMigrate(&AssignedAccessPolicy{}))
			require.NoError(t, db.AutoMigrate(&AssignedAccessPolicy{}))
			assignedAccessPolicyLifecycleContract(t, db)
			assignedAccessPolicyConcurrentCASContract(t, db)
			if engine.name == "mysql" {
				cfg, err := mysqldriver.ParseDSN(dsn)
				require.NoError(t, err)
				cfg.ClientFoundRows = true
				dialect, err := s1DatabaseDialector(engine.name, cfg.FormatDSN())
				require.NoError(t, err)
				foundRowsDB, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
				require.NoError(t, err)
				conn, err := foundRowsDB.DB()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, conn.Close()) })
				_, err = ConfigureAssignedAccessPolicy(nil, foundRowsDB, "user", 7, 0, `{}`)
				require.ErrorIs(t, err, ErrAssignedAccessPolicyConflict, "clientFoundRows must not authorize stale creation")
			}
		})
	}
}

func TestAssignedAccessPolicyExistingCASUsesPortableLockedTransaction(t *testing.T) {
	for _, engine := range []string{"mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			sqlDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			var dialect gorm.Dialector = mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true})
			if engine == "postgres" {
				dialect = postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(dialect, &gorm.Config{DisableAutomaticPing: true})
			require.NoError(t, err)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT .*assigned_access_policies.*subject_type.*subject_id.*FOR UPDATE").
				WillReturnRows(sqlmock.NewRows([]string{"id", "subject_type", "subject_id", "revision", "assigned", "policy_json"}).AddRow(77, "user", 7, 1, true, `{}`))
			mock.ExpectExec("UPDATE .*assigned_access_policies.*WHERE id = .* AND revision = .*").
				WithArgs(false, "", int64(2), 77, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
			row, err := RemoveAssignedAccessPolicy(nil, db, "user", 7, 1)
			require.NoError(t, err)
			assert.EqualValues(t, 2, row.Revision)
			assert.False(t, row.Assigned)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestAssignedAccessPolicyCreateMapsOnlyUniqueConflicts(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{"gorm duplicate", gorm.ErrDuplicatedKey, ErrAssignedAccessPolicyConflict},
		{"mysql duplicate", &mysqldriver.MySQLError{Number: 1062}, ErrAssignedAccessPolicyConflict},
		{"postgres duplicate", &pgconn.PgError{Code: "23505"}, ErrAssignedAccessPolicyConflict},
		{"mysql unrelated", &mysqldriver.MySQLError{Number: 1146}, nil},
		{"postgres unrelated", &pgconn.PgError{Code: "42P01"}, nil},
		{"connection failure", errors.New("connection failed"), nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupAssignedAccessPolicyDB(t)
			require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:assigned-policy-error", func(tx *gorm.DB) { tx.AddError(test.err) }))
			_, err := ConfigureAssignedAccessPolicy(nil, db, "user", 7, 0, `{}`)
			want := test.want
			if want == nil {
				want = test.err
			}
			require.ErrorIs(t, err, want)
		})
	}
}

func setupAssignedAccessPolicyAdminDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupAssignedAccessPolicyDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}))
	for _, user := range []User{
		{Id: 1, Username: "policy-root", AffCode: "policy-root", Role: common.RoleRootUser, Status: common.UserStatusEnabled},
		{Id: 2, Username: "policy-admin", AffCode: "policy-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
		{Id: 3, Username: "policy-owner", AffCode: "policy-owner", Role: common.RoleCommonUser, Status: common.UserStatusEnabled},
		{Id: 4, Username: "policy-peer", AffCode: "policy-peer", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
	} {
		require.NoError(t, db.Create(&user).Error)
	}
	require.NoError(t, db.Create(&Token{Id: 11, UserId: 3, Key: "assigned-policy-token", Status: common.TokenStatusEnabled}).Error)
	return db
}

func TestAssignedAccessPolicyAdminMutationRechecksDemotionAndRevocation(t *testing.T) {
	db := setupAssignedAccessPolicyAdminDB(t)
	policy := `{"enabled":true}`
	created, err := MutateAssignedAccessPolicyAsAdmin(nil, db, 2, "user", 3, 0, &policy)
	require.NoError(t, err)
	var staleActor User
	require.NoError(t, db.First(&staleActor, 2).Error)
	require.Equal(t, common.RoleAdminUser, staleActor.Role)
	require.NoError(t, db.Model(&User{}).Where("id = ?", 2).Update("role", common.RoleCommonUser).Error)
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, staleActor.Id, "user", 3, created.Revision, nil)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyForbidden, "a previously authorized request cannot use a stale administrator role")
	unchanged, err := ReadAssignedAccessPolicy(nil, db, "user", 3)
	require.NoError(t, err)
	assert.Equal(t, created, unchanged)
	require.NoError(t, db.Model(&User{}).Where("id = ?", 2).Updates(map[string]any{"role": common.RoleAdminUser, "status": common.UserStatusDisabled}).Error)
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, 2, "user", 3, created.Revision, &policy)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyForbidden)
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, 99, "user", 3, created.Revision, &policy)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyForbidden)
	removed, err := MutateAssignedAccessPolicyAsAdmin(nil, db, 1, "user", 3, created.Revision, nil)
	require.NoError(t, err)
	assert.False(t, removed.Assigned)
	assert.EqualValues(t, 2, removed.Revision)
}

func TestAssignedAccessPolicyAdminMutationHonorsCurrentHierarchyAndOwnership(t *testing.T) {
	db := setupAssignedAccessPolicyAdminDB(t)
	policy := `{"enabled":true}`
	for _, target := range []int{1, 2, 4} {
		_, err := MutateAssignedAccessPolicyAsAdmin(nil, db, 2, "user", target, 0, &policy)
		require.ErrorIs(t, err, ErrAssignedAccessPolicyForbidden, "an administrator cannot mutate a root, self, or peer assignment")
	}
	_, err := MutateAssignedAccessPolicyAsAdmin(nil, db, 1, "user", 1, 0, &policy)
	require.NoError(t, err, "root may manage its own assignment")
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, 1, "user", 4, 0, &policy)
	require.NoError(t, err, "root may manage another administrator")
	created, err := MutateAssignedAccessPolicyAsAdmin(nil, db, 2, "token", 11, 0, &policy)
	require.NoError(t, err)
	require.NoError(t, db.Model(&Token{}).Where("id = ?", 11).Update("user_id", 4).Error)
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, 2, "token", 11, created.Revision, nil)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyForbidden, "token ownership must be freshly loaded")
	unchanged, err := ReadAssignedAccessPolicy(nil, db, "token", 11)
	require.NoError(t, err)
	assert.Equal(t, created, unchanged)
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, 1, "token", 11, created.Revision, nil)
	require.NoError(t, err)
	require.NoError(t, db.Model(&User{}).Where("id = ?", 3).Update("role", common.RoleRootUser).Error)
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, 2, "user", 3, 0, &policy)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyForbidden, "a promoted target must no longer be writable by a lower administrator")
}

func TestAssignedAccessPolicyAdminMutationRejectsOwnerChangeDuringAuthorization(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{DisableAutomaticPing: true})
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*tokens.*").WithArgs(11).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).AddRow(11, 2))
	// The target has a lower ID than the actor. Both user rows must be locked
	// in the same ascending order used when actor and target are reversed.
	mock.ExpectQuery("SELECT .*users.*FOR UPDATE").WithArgs(2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "status"}).AddRow(2, common.RoleCommonUser, common.UserStatusEnabled))
	mock.ExpectQuery("SELECT .*users.*FOR UPDATE").WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "status"}).AddRow(3, common.RoleAdminUser, common.UserStatusEnabled))
	mock.ExpectQuery("SELECT .*tokens.*FOR UPDATE").WithArgs(11).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id"}).AddRow(11, 4))
	mock.ExpectRollback()
	policy := `{"enabled":true}`
	_, err = MutateAssignedAccessPolicyAsAdmin(nil, db, 3, "token", 11, 0, &policy)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyForbidden)
	require.NoError(t, mock.ExpectationsWereMet(), "changed ownership must roll back without any assignment write")
}

func TestAssignedAccessPolicyPairReadUsesOneSnapshotAndPreservesMissingSubjects(t *testing.T) {
	db := setupAssignedAccessPolicyDB(t)
	user, err := ConfigureAssignedAccessPolicy(nil, db, "user", 7, 0, `{"enabled":true}`)
	require.NoError(t, err)
	token, err := ConfigureAssignedAccessPolicy(nil, db, "token", 11, 0, `{"enabled":false}`)
	require.NoError(t, err)
	_, err = ConfigureAssignedAccessPolicy(nil, db, "token", 7, 0, `{"enabled":true}`)
	require.NoError(t, err)
	queries := 0
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:assigned-policy-snapshot", func(tx *gorm.DB) {
		queries++
	}))
	rows, err := ReadAssignedAccessPolicies(nil, db, 7, 11)
	require.NoError(t, err)
	assert.Equal(t, 1, queries, "both scopes must come from one database statement")
	assert.Equal(t, []AssignedAccessPolicy{user, token}, rows)
	rows, err = ReadAssignedAccessPolicies(nil, db, 7, 0)
	require.NoError(t, err)
	assert.Equal(t, []AssignedAccessPolicy{user}, rows)
	rows, err = ReadAssignedAccessPolicies(nil, db, 9, 12)
	require.NoError(t, err)
	assert.Equal(t, []AssignedAccessPolicy{{SubjectType: "user", SubjectID: 9}, {SubjectType: "token", SubjectID: 12}}, rows)
	require.NoError(t, db.Model(&AssignedAccessPolicy{}).Where("id = ?", token.ID).Update("policy_json", "null").Error)
	rows, err = ReadAssignedAccessPolicies(nil, db, 7, 11)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
	assert.Nil(t, rows, "a corrupt scope must not return the other scope as a usable partial result")
	_, err = ReadAssignedAccessPolicies(nil, db, 0, 11)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
	_, err = ReadAssignedAccessPolicies(nil, db, 7, -1)
	require.ErrorIs(t, err, ErrAssignedAccessPolicyInvalid)
}
