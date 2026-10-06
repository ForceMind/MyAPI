package model

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"unicode/utf8"

	"github.com/ForceMind/MyAPI/common"
	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// MySQL TEXT is limited to 65,535 bytes. Reject larger payloads before any
// database write so every supported dialect has the same storage contract.
const MaxAssignedAccessPolicyJSONBytes = 65535

var (
	ErrAssignedAccessPolicyInvalid   = errors.New("invalid assigned access policy")
	ErrAssignedAccessPolicyConflict  = errors.New("assigned access policy revision conflict")
	ErrAssignedAccessPolicyForbidden = errors.New("assigned access policy mutation forbidden")
)

// AssignedAccessPolicy is private administrator-owned state. Removal preserves
// a tombstone and monotonically increasing revision, so an old create/update
// can never restore a removed assignment through an ABA revision reset.
// PolicyJSON is validated and normalized by the service boundary; the model
// independently enforces its syntax and portable storage size.
type AssignedAccessPolicy struct {
	ID          int    `json:"-" gorm:"primaryKey"`
	SubjectType string `json:"-" gorm:"type:varchar(16);not null;uniqueIndex:uidx_assigned_access_policy_subject,priority:1"`
	SubjectID   int    `json:"-" gorm:"not null;uniqueIndex:uidx_assigned_access_policy_subject,priority:2"`
	Revision    int64  `json:"-" gorm:"type:bigint;not null"`
	Assigned    bool   `json:"-" gorm:"not null"`
	PolicyJSON  string `json:"-" gorm:"type:text;not null"`
}

func ReadAssignedAccessPolicy(ctx context.Context, db *gorm.DB, subject string, id int) (AssignedAccessPolicy, error) {
	if db == nil {
		return AssignedAccessPolicy{}, gorm.ErrInvalidDB
	}
	if (subject != "user" && subject != "token") || id <= 0 {
		return AssignedAccessPolicy{}, ErrAssignedAccessPolicyInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var row AssignedAccessPolicy
	read := db.WithContext(ctx).Where("subject_type = ? AND subject_id = ?", subject, id).Limit(1).Find(&row)
	if read.Error != nil {
		return AssignedAccessPolicy{}, read.Error
	}
	if read.RowsAffected == 0 {
		return AssignedAccessPolicy{SubjectType: subject, SubjectID: id}, nil
	}
	if !validStoredAssignedAccessPolicy(row) {
		return AssignedAccessPolicy{}, ErrAssignedAccessPolicyInvalid
	}
	return row, nil
}

// ReadAssignedAccessPolicies reads both policy scopes in one database statement
// so a request cannot combine policies observed on opposite sides of a commit.
// Results are ordered user then token, including revision-zero unassigned rows
// for missing subjects. A zero token ID omits the token scope for session users.
func ReadAssignedAccessPolicies(ctx context.Context, db *gorm.DB, userID, tokenID int) ([]AssignedAccessPolicy, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if userID <= 0 || tokenID < 0 {
		return nil, ErrAssignedAccessPolicyInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := []AssignedAccessPolicy{{SubjectType: "user", SubjectID: userID}}
	query := db.WithContext(ctx).Where("subject_type = ? AND subject_id = ?", "user", userID)
	if tokenID > 0 {
		result = append(result, AssignedAccessPolicy{SubjectType: "token", SubjectID: tokenID})
		query = query.Or("subject_type = ? AND subject_id = ?", "token", tokenID)
	}
	var rows []AssignedAccessPolicy
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !validStoredAssignedAccessPolicy(row) {
			return nil, ErrAssignedAccessPolicyInvalid
		}
		switch {
		case row.SubjectType == "user" && row.SubjectID == userID:
			result[0] = row
		case tokenID > 0 && row.SubjectType == "token" && row.SubjectID == tokenID:
			result[1] = row
		default:
			return nil, ErrAssignedAccessPolicyInvalid
		}
	}
	return result, nil
}

// ConfigureAssignedAccessPolicy performs strict compare-and-swap. Revision zero
// may create a row only; it cannot overwrite an assignment or its tombstone.
func ConfigureAssignedAccessPolicy(ctx context.Context, db *gorm.DB, subject string, id int, expectedRevision int64, policyJSON string) (AssignedAccessPolicy, error) {
	if !validAssignedAccessPolicyJSON(policyJSON) {
		return AssignedAccessPolicy{}, ErrAssignedAccessPolicyInvalid
	}
	return mutateAssignedAccessPolicy(ctx, db, subject, id, expectedRevision, true, policyJSON)
}

func RemoveAssignedAccessPolicy(ctx context.Context, db *gorm.DB, subject string, id int, expectedRevision int64) (AssignedAccessPolicy, error) {
	return mutateAssignedAccessPolicy(ctx, db, subject, id, expectedRevision, false, "")
}

// MutateAssignedAccessPolicyAsAdmin authorizes against locked, current user
// records in the same transaction as the assignment mutation. A nil policy
// removes the assignment. The caller's cached role never grants permission.
func MutateAssignedAccessPolicyAsAdmin(ctx context.Context, db *gorm.DB, actorID int, subject string, id int, expectedRevision int64, policyJSON *string) (AssignedAccessPolicy, error) {
	if db == nil {
		return AssignedAccessPolicy{}, gorm.ErrInvalidDB
	}
	if actorID <= 0 || (subject != "user" && subject != "token") || id <= 0 || expectedRevision < 0 || expectedRevision == math.MaxInt64 {
		return AssignedAccessPolicy{}, ErrAssignedAccessPolicyInvalid
	}
	if policyJSON != nil && !validAssignedAccessPolicyJSON(*policyJSON) {
		return AssignedAccessPolicy{}, ErrAssignedAccessPolicyInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result AssignedAccessPolicy
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ownerID := id
		if subject == "token" {
			var token Token
			if err := tx.Select("id", "user_id").First(&token, id).Error; err != nil {
				return err
			}
			ownerID = token.UserId
		}
		if ownerID <= 0 {
			return ErrAssignedAccessPolicyForbidden
		}
		// Lock users before tokens, consistently ordering the two users even
		// when an administrator is the target of another administrator.
		userIDs := []int{actorID}
		if ownerID != actorID {
			userIDs = append(userIDs, ownerID)
		}
		sort.Ints(userIDs)
		var actor, owner User
		for _, userID := range userIDs {
			var user User
			err := lockForUpdate(tx).Select("id", "role", "status").First(&user, userID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) && userID == actorID {
				return ErrAssignedAccessPolicyForbidden
			}
			if err != nil {
				return err
			}
			if userID == actorID {
				actor = user
			}
			if userID == ownerID {
				owner = user
			}
		}
		if actor.Status != common.UserStatusEnabled || actor.Role < common.RoleAdminUser || actor.Role != common.RoleRootUser && actor.Role <= owner.Role {
			return ErrAssignedAccessPolicyForbidden
		}
		if subject == "token" {
			var token Token
			if err := lockForUpdate(tx).Select("id", "user_id").First(&token, id).Error; err != nil {
				return err
			}
			// The initial owner read was only for lock ordering. Ownership may
			// have changed before the locks, in which case no write is allowed.
			if token.UserId != ownerID {
				return ErrAssignedAccessPolicyForbidden
			}
		}
		var err error
		if policyJSON == nil {
			result, err = RemoveAssignedAccessPolicy(ctx, tx, subject, id, expectedRevision)
		} else {
			result, err = ConfigureAssignedAccessPolicy(ctx, tx, subject, id, expectedRevision, *policyJSON)
		}
		return err
	})
	if err != nil {
		return AssignedAccessPolicy{}, err
	}
	return result, nil
}

func validAssignedAccessPolicyJSON(value string) bool {
	if len(value) == 0 || len(value) > MaxAssignedAccessPolicyJSONBytes || !utf8.ValidString(value) {
		return false
	}
	if _, err := common.CanonicalJSONObjectDigest([]byte(value)); err != nil {
		return false
	}
	var object map[string]json.RawMessage
	return common.UnmarshalJsonStr(value, &object) == nil && object != nil
}

func validStoredAssignedAccessPolicy(row AssignedAccessPolicy) bool {
	if (row.SubjectType != "user" && row.SubjectType != "token") || row.SubjectID <= 0 || row.Revision <= 0 {
		return false
	}
	if row.Assigned {
		return validAssignedAccessPolicyJSON(row.PolicyJSON)
	}
	return row.PolicyJSON == ""
}

func mutateAssignedAccessPolicy(ctx context.Context, db *gorm.DB, subject string, id int, expectedRevision int64, assigned bool, policyJSON string) (AssignedAccessPolicy, error) {
	if db == nil {
		return AssignedAccessPolicy{}, gorm.ErrInvalidDB
	}
	if (subject != "user" && subject != "token") || id <= 0 || expectedRevision < 0 || expectedRevision == math.MaxInt64 {
		return AssignedAccessPolicy{}, ErrAssignedAccessPolicyInvalid
	}
	if ctx == nil {
		ctx = context.Background()
	}
	row := AssignedAccessPolicy{
		SubjectType: subject,
		SubjectID:   id,
		Revision:    expectedRevision + 1,
		Assigned:    assigned,
		PolicyJSON:  policyJSON,
	}
	if expectedRevision == 0 {
		// A plain INSERT makes duplicate creation unambiguous, including MySQL
		// clientFoundRows mode, where a no-op upsert can report an affected row.
		err := db.WithContext(ctx).Create(&row).Error
		var mysqlErr *mysql.MySQLError
		var pgErr *pgconn.PgError
		var sqliteErr *sqlitedriver.Error
		if errors.Is(err, gorm.ErrDuplicatedKey) ||
			errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 ||
			errors.As(err, &pgErr) && pgErr.Code == "23505" ||
			errors.As(err, &sqliteErr) && (sqliteErr.Code() == 1555 || sqliteErr.Code() == 2067) {
			return AssignedAccessPolicy{}, ErrAssignedAccessPolicyConflict
		}
		if err != nil {
			return AssignedAccessPolicy{}, err
		}
		return row, nil
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing AssignedAccessPolicy
		err := lockForUpdate(tx).Where("subject_type = ? AND subject_id = ?", subject, id).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAssignedAccessPolicyConflict
		}
		if err != nil {
			return err
		}
		if existing.Revision != expectedRevision {
			return ErrAssignedAccessPolicyConflict
		}
		row.ID = existing.ID
		update := tx.Model(&AssignedAccessPolicy{}).
			Where("id = ? AND revision = ?", existing.ID, expectedRevision).
			Updates(map[string]any{"revision": row.Revision, "assigned": assigned, "policy_json": policyJSON})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return ErrAssignedAccessPolicyConflict
		}
		return nil
	})
	if err != nil {
		return AssignedAccessPolicy{}, err
	}
	return row, nil
}
