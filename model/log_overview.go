package model

import (
	"errors"

	"gorm.io/gorm"
)

const recentLogOverviewLimit = 5

// RecentLogOverviewItem is a bounded, content-free view of one log row. Raw
// request content, provider errors, tokens and log metadata never leave this
// projection.
type RecentLogOverviewItem struct {
	ID        int    `json:"id"`
	CreatedAt int64  `json:"created_at"`
	ModelName string `json:"model_name"`
	Username  string `json:"username,omitempty"`
}

type RecentLogOverview struct {
	Requests []RecentLogOverviewItem `json:"requests"`
	Errors   []RecentLogOverviewItem `json:"errors"`
}

func GetAdminRecentLogOverview(db *gorm.DB) (RecentLogOverview, error) {
	return recentLogOverview(db, 0, true)
}

func GetUserRecentLogOverview(db *gorm.DB, userID int) (RecentLogOverview, error) {
	if userID <= 0 {
		return RecentLogOverview{}, errors.New("valid user id required")
	}
	return recentLogOverview(db, userID, false)
}

func recentLogOverview(db *gorm.DB, userID int, admin bool) (RecentLogOverview, error) {
	if db == nil {
		return RecentLogOverview{}, gorm.ErrInvalidDB
	}
	result := RecentLogOverview{
		Requests: make([]RecentLogOverviewItem, 0),
		Errors:   make([]RecentLogOverviewItem, 0),
	}
	for _, target := range []struct {
		logType int
		rows    *[]RecentLogOverviewItem
	}{
		{logType: LogTypeConsume, rows: &result.Requests},
		{logType: LogTypeError, rows: &result.Errors},
	} {
		columns := "logs.id, logs.created_at, logs.model_name"
		if admin {
			columns += ", logs.username"
		}
		query := deduplicatedLogs(db).
			Select(columns).
			Where("logs.type = ?", target.logType)
		if !admin {
			query = query.Where("logs.user_id = ?", userID)
		}
		if err := query.Order("logs.created_at DESC, logs.id DESC").Limit(recentLogOverviewLimit).Find(target.rows).Error; err != nil {
			return RecentLogOverview{}, err
		}
	}
	return result, nil
}
