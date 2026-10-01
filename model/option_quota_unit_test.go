package model

import (
	"strconv"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuotaPerUnitOptionRejectsUnusableOrOverflowingUnit(t *testing.T) {
	for _, value := range []string{
		"", "not-a-number", "NaN", "+Inf", "-Inf", "0", "-1", "0.5",
		strconv.Itoa(common.MaxQuota/10 + 1),
		strconv.Itoa(common.MaxQuota),
	} {
		t.Run(value, func(t *testing.T) {
			require.Error(t, validateOptionValue("QuotaPerUnit", value))
		})
	}
	for _, value := range []string{"1", "1.5", "500000", strconv.Itoa(common.MaxQuota / 10)} {
		t.Run(value, func(t *testing.T) {
			require.NoError(t, validateOptionValue("QuotaPerUnit", value))
		})
	}
}

func TestQuotaPerUnitInvalidWriteLeavesDatabaseAndRuntimeUnchanged(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = previousUnit })

	for _, value := range []string{"NaN", "0", strconv.Itoa(common.MaxQuota/10 + 1)} {
		require.Error(t, UpdateOption("QuotaPerUnit", value))
	}
	assert.Zero(t, countOptions(t, db))
	assert.Equal(t, previousUnit, common.QuotaPerUnit)
}
