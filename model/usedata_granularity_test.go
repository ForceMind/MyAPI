package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetAllQuotaDatesWithGranularityPreservesTotalsAcrossBuckets(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&[]QuotaData{
		{ModelName: "gpt-a", CreatedAt: 61, Count: 1, Quota: 10, TokenUsed: 2},
		{ModelName: "gpt-a", CreatedAt: 119, Count: 2, Quota: 20, TokenUsed: 3},
		{ModelName: "gpt-a", CreatedAt: 120, Count: 3, Quota: 30, TokenUsed: 4},
		{ModelName: "gpt-b", CreatedAt: 121, Count: 4, Quota: 40, TokenUsed: 5},
	}).Error)

	minuteRows, err := GetAllQuotaDatesWithGranularity(
		1,
		200,
		"",
		QuotaDataGranularityMinute,
		0,
	)
	require.NoError(t, err)
	require.Len(t, minuteRows, 3)
	require.Equal(t, int64(60), minuteRows[0].CreatedAt)
	require.Equal(t, 30, minuteRows[0].Quota)
	require.Equal(t, 3, minuteRows[0].Count)

	hourRows, err := GetAllQuotaDatesWithGranularity(
		1,
		200,
		"",
		QuotaDataGranularityHour,
		0,
	)
	require.NoError(t, err)
	require.Len(t, hourRows, 2)
	require.Equal(t, "gpt-a", hourRows[0].ModelName)
	require.Equal(t, 60, hourRows[0].Quota)
	require.Equal(t, 6, hourRows[0].Count)
	require.Equal(t, 9, hourRows[0].TokenUsed)
	require.Equal(t, "gpt-b", hourRows[1].ModelName)
	require.Equal(t, 40, hourRows[1].Quota)
}

func TestGetAllQuotaDatesWithGranularityAlignsDayToRequestedTimezone(t *testing.T) {
	truncateTables(t)
	firstLocalDay := time.Date(2026, 8, 27, 16, 30, 0, 0, time.UTC).Unix()
	sameLocalDay := time.Date(2026, 8, 28, 15, 30, 0, 0, time.UTC).Unix()
	nextLocalDay := time.Date(2026, 8, 28, 16, 30, 0, 0, time.UTC).Unix()
	require.NoError(t, DB.Create(&[]QuotaData{
		{ModelName: "gpt-a", CreatedAt: firstLocalDay, Count: 1, Quota: 10},
		{ModelName: "gpt-a", CreatedAt: sameLocalDay, Count: 2, Quota: 20},
		{ModelName: "gpt-a", CreatedAt: nextLocalDay, Count: 3, Quota: 30},
	}).Error)

	rows, err := GetAllQuotaDatesWithGranularity(
		firstLocalDay,
		nextLocalDay,
		"",
		QuotaDataGranularityDay,
		480,
	)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(
		t,
		time.Date(2026, 8, 27, 16, 0, 0, 0, time.UTC).Unix(),
		rows[0].CreatedAt,
	)
	require.Equal(t, 30, rows[0].Quota)
	require.Equal(t, 3, rows[0].Count)
	require.Equal(t, 30, rows[1].Quota)
	require.Equal(t, 3, rows[1].Count)
}

func TestGetAllQuotaDatesWithGranularityStartsWeekOnLocalMonday(t *testing.T) {
	truncateTables(t)
	tuesday := time.Date(2026, 8, 25, 4, 0, 0, 0, time.UTC).Unix()
	require.NoError(t, DB.Create(&QuotaData{
		ModelName: "gpt-a",
		CreatedAt: tuesday,
		Count:     1,
		Quota:     10,
	}).Error)

	rows, err := GetAllQuotaDatesWithGranularity(
		tuesday-86400,
		tuesday+86400,
		"",
		QuotaDataGranularityWeek,
		480,
	)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(
		t,
		time.Date(2026, 8, 23, 16, 0, 0, 0, time.UTC).Unix(),
		rows[0].CreatedAt,
	)
}

func TestQuotaDataBucketStartUsesFixedOffsetAndMondayAnchor(t *testing.T) {
	timestamp := time.Date(2026, 8, 25, 12, 34, 56, 0, time.UTC).Unix()

	require.Equal(
		t,
		time.Date(2026, 8, 24, 16, 0, 0, 0, time.UTC).Unix(),
		QuotaDataBucketStart(timestamp, QuotaDataGranularityDay, 480),
	)
	require.Equal(
		t,
		time.Date(2026, 8, 23, 16, 0, 0, 0, time.UTC).Unix(),
		QuotaDataBucketStart(timestamp, QuotaDataGranularityWeek, 480),
	)
}
