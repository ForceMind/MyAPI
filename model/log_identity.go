package model

import "gorm.io/gorm"

// LogIdentityFilter identifies saved requests independently of mutable Key names.
// UserID is mandatory for self-service callers; zero retains the admin scope.
type LogIdentityFilter struct {
	TokenID, UserID              int
	RequestID, UpstreamRequestID string
}

func applyLogIdentityFilter(query *gorm.DB, filters []LogIdentityFilter) (*gorm.DB, error) {
	if len(filters) == 0 {
		return query, nil
	}
	if len(filters) != 1 || filters[0].TokenID < 0 || filters[0].UserID < 0 {
		return nil, gorm.ErrInvalidData
	}
	filter := filters[0]
	if filter.TokenID > 0 {
		query = query.Where("logs.token_id = ?", filter.TokenID)
	}
	if filter.UserID > 0 {
		query = query.Where("logs.user_id = ?", filter.UserID)
	}
	if filter.RequestID != "" {
		query = query.Where("logs.request_id = ?", filter.RequestID)
	}
	if filter.UpstreamRequestID != "" {
		query = query.Where("logs.upstream_request_id = ?", filter.UpstreamRequestID)
	}
	return query, nil
}
