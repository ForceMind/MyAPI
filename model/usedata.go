package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id        int    `json:"id"`
	UserID    int    `json:"user_id" gorm:"index"`
	Username  string `json:"username" gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName string `json:"model_name" gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	UseGroup  string `json:"use_group" gorm:"index;size:64;default:''"`
	TokenID   int    `json:"token_id" gorm:"index;default:0"`
	ChannelID int    `json:"channel_id" gorm:"index;default:0"`
	NodeName  string `json:"node_name" gorm:"index;size:64;default:''"`
	TokenUsed int    `json:"token_used" gorm:"default:0"`
	Count     int    `json:"count" gorm:"default:0"`
	Quota     int    `json:"quota" gorm:"default:0"`
}

type QuotaDataLogParams struct {
	UserID    int
	Username  string
	ModelName string
	Quota     int
	CreatedAt int64
	TokenUsed int
	UseGroup  string
	TokenID   int
	ChannelID int
	NodeName  string
}

type QuotaDataGranularity string

const (
	QuotaDataGranularityMinute QuotaDataGranularity = "minute"
	QuotaDataGranularityHour   QuotaDataGranularity = "hour"
	QuotaDataGranularityDay    QuotaDataGranularity = "day"
	QuotaDataGranularityWeek   QuotaDataGranularity = "week"
)

func ParseQuotaDataGranularity(value string) (QuotaDataGranularity, bool) {
	granularity := QuotaDataGranularity(value)
	switch granularity {
	case QuotaDataGranularityMinute,
		QuotaDataGranularityHour,
		QuotaDataGranularityDay,
		QuotaDataGranularityWeek:
		return granularity, true
	default:
		return "", false
	}
}

func (granularity QuotaDataGranularity) BucketSeconds() int64 {
	switch granularity {
	case QuotaDataGranularityMinute:
		return 60
	case QuotaDataGranularityDay:
		return 86400
	case QuotaDataGranularityWeek:
		return 604800
	default:
		return 3600
	}
}

// QuotaDataBucketStart returns the UTC timestamp of the bucket boundary for a
// fixed timezone offset. The offset uses the same convention as the dashboard
// API: minutes east of UTC (for example, Asia/Shanghai is +480).
func QuotaDataBucketStart(
	timestamp int64,
	granularity QuotaDataGranularity,
	timezoneOffsetMinutes int,
) int64 {
	weekAnchorSeconds := int64(0)
	if granularity == QuotaDataGranularityWeek {
		// 1970-01-05 00:00:00 UTC was the first Monday after Unix epoch.
		weekAnchorSeconds = 4 * 86400
	}
	timezoneOffsetSeconds := int64(timezoneOffsetMinutes) * 60
	remainder := (timestamp + timezoneOffsetSeconds - weekAnchorSeconds) % granularity.BucketSeconds()
	if remainder < 0 {
		remainder += granularity.BucketSeconds()
	}
	return timestamp - remainder
}

func quotaDataBucketExpression(granularity QuotaDataGranularity, timezoneOffsetMinutes int) string {
	weekAnchorSeconds := int64(0)
	if granularity == QuotaDataGranularityWeek {
		// Unix epoch began on Thursday. Anchoring at the following Monday
		// keeps weekly buckets aligned to Monday in the requested timezone.
		weekAnchorSeconds = 4 * 86400
	}
	timezoneOffsetSeconds := int64(timezoneOffsetMinutes) * 60
	return fmt.Sprintf(
		"(created_at - ((((created_at + %d - %d) %% %d) + %d) %% %d))",
		timezoneOffsetSeconds,
		weekAnchorSeconds,
		granularity.BucketSeconds(),
		granularity.BucketSeconds(),
		granularity.BucketSeconds(),
	)
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog("正在更新数据看板数据...")
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(quotaData *QuotaData) {
	key := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s\x00%d\x00%d\x00%s",
		quotaData.UserID,
		quotaData.Username,
		quotaData.ModelName,
		quotaData.CreatedAt,
		quotaData.UseGroup,
		quotaData.TokenID,
		quotaData.ChannelID,
		quotaData.NodeName,
	)
	count := quotaData.Count
	quota := quotaData.Quota
	tokenUsed := quotaData.TokenUsed
	cachedQuotaData, ok := CacheQuotaData[key]
	if ok {
		cachedQuotaData.Count += count
		cachedQuotaData.Quota += quota
		cachedQuotaData.TokenUsed += tokenUsed
		quotaData = cachedQuotaData
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(params QuotaDataLogParams) {
	// Keep minute-level source data. Query endpoints aggregate these rows into
	// minute, hour, day, or week buckets as requested by the dashboard.
	createdAt := params.CreatedAt - (params.CreatedAt % 60)
	quotaData := &QuotaData{
		UserID:    params.UserID,
		Username:  params.Username,
		ModelName: params.ModelName,
		CreatedAt: createdAt,
		UseGroup:  params.UseGroup,
		TokenID:   params.TokenID,
		ChannelID: params.ChannelID,
		NodeName:  params.NodeName,
		Count:     1,
		Quota:     params.Quota,
		TokenUsed: params.TokenUsed,
	}

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(quotaData)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	size := len(CacheQuotaData)
	// 如果缓存中有数据，就保存到数据库中
	// 1. 先查询数据库中是否有数据
	// 2. 如果有数据，就更新数据
	// 3. 如果没有数据，就插入数据
	for _, quotaData := range CacheQuotaData {
		quotaDataDB := &QuotaData{}
		DB.Table("quota_data").
			Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
				quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
			First(quotaDataDB)
		if quotaDataDB.Id > 0 {
			//quotaDataDB.Count += quotaData.Count
			//quotaDataDB.Quota += quotaData.Quota
			//DB.Table("quota_data").Save(quotaDataDB)
			increaseQuotaData(quotaData)
		} else {
			DB.Table("quota_data").Create(quotaData)
		}
	}
	CacheQuotaData = make(map[string]*QuotaData)
	common.SysLog(fmt.Sprintf("保存数据看板数据成功，共保存%d条数据", size))
}

func increaseQuotaData(quotaData *QuotaData) {
	err := DB.Table("quota_data").
		Where("user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
			quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName).
		Updates(map[string]interface{}{
			"count":      gorm.Expr("count + ?", quotaData.Count),
			"quota":      gorm.Expr("quota + ?", quotaData.Quota),
			"token_used": gorm.Expr("token_used + ?", quotaData.TokenUsed),
		}).Error
	if err != nil {
		common.SysLog(fmt.Sprintf("increaseQuotaData error: %s", err))
	}
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	return GetQuotaDataByUsernameWithGranularity(
		username,
		startTime,
		endTime,
		QuotaDataGranularityHour,
		0,
	)
}

func GetQuotaDataByUsernameWithGranularity(
	username string,
	startTime int64,
	endTime int64,
	granularity QuotaDataGranularity,
	timezoneOffsetMinutes int,
) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	bucketExpression := quotaDataBucketExpression(granularity, timezoneOffsetMinutes)
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").
		Select(fmt.Sprintf("user_id, username, model_name, %s as created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used", bucketExpression)).
		Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).
		Group(fmt.Sprintf("user_id, username, model_name, %s", bucketExpression)).
		Order("created_at ASC, model_name ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	return GetQuotaDataByUserIdWithGranularity(
		userId,
		startTime,
		endTime,
		QuotaDataGranularityHour,
		0,
	)
}

func GetQuotaDataByUserIdWithGranularity(
	userId int,
	startTime int64,
	endTime int64,
	granularity QuotaDataGranularity,
	timezoneOffsetMinutes int,
) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	bucketExpression := quotaDataBucketExpression(granularity, timezoneOffsetMinutes)
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").
		Select(fmt.Sprintf("user_id, username, model_name, %s as created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used", bucketExpression)).
		Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).
		Group(fmt.Sprintf("user_id, username, model_name, %s", bucketExpression)).
		Order("created_at ASC, model_name ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	return GetQuotaDataGroupByUserWithGranularity(
		startTime,
		endTime,
		QuotaDataGranularityHour,
		0,
	)
}

func GetQuotaDataGroupByUserWithGranularity(
	startTime int64,
	endTime int64,
	granularity QuotaDataGranularity,
	timezoneOffsetMinutes int,
) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	bucketExpression := quotaDataBucketExpression(granularity, timezoneOffsetMinutes)
	err = DB.Table("quota_data").
		Select(fmt.Sprintf("username, %s as created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used", bucketExpression)).
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group(fmt.Sprintf("username, %s", bucketExpression)).
		Order("created_at ASC, username ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	return GetAllQuotaDatesWithGranularity(
		startTime,
		endTime,
		username,
		QuotaDataGranularityHour,
		0,
	)
}

func GetAllQuotaDatesWithGranularity(
	startTime int64,
	endTime int64,
	username string,
	granularity QuotaDataGranularity,
	timezoneOffsetMinutes int,
) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsernameWithGranularity(
			username,
			startTime,
			endTime,
			granularity,
			timezoneOffsetMinutes,
		)
	}
	var quotaDatas []*QuotaData
	bucketExpression := quotaDataBucketExpression(granularity, timezoneOffsetMinutes)
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").
		Select(fmt.Sprintf("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, %s as created_at", bucketExpression)).
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group(fmt.Sprintf("model_name, %s", bucketExpression)).
		Order("created_at ASC, model_name ASC").
		Find(&quotaDatas).Error
	return quotaDatas, err
}
