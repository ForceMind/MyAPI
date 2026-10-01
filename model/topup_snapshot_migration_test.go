package model

import (
	"github.com/ForceMind/MyAPI/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"math"
	"testing"
)

type legacySnapshotlessTopUp struct {
	Id              int `gorm:"primaryKey"`
	UserId          int
	Amount          int64
	Money           float64
	TradeNo         string
	PaymentProvider string
	Status          string
}

func (legacySnapshotlessTopUp) TableName() string { return "top_ups" }

func TestTopUpSnapshotMigrationDoesNotInventHistoricalUnit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacySnapshotlessTopUp{}))
	legacy := legacySnapshotlessTopUp{UserId: 1, Amount: 2, Money: 3, TradeNo: "before-snapshot-schema", PaymentProvider: PaymentProviderEpay, Status: common.TopUpStatusPending}
	require.NoError(t, db.Create(&legacy).Error)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	var migrated TopUp
	require.NoError(t, db.First(&migrated, legacy.Id).Error)
	assert.Empty(t, migrated.QuotaPerUnitSnapshot)
	assert.Empty(t, migrated.WaffoPancakeStoreID)
	assert.Empty(t, migrated.WaffoPancakeProductID)
	assert.Empty(t, migrated.WaffoPancakeCurrency)
	assert.EqualValues(t, 2, migrated.Amount)
	assert.Equal(t, 3.0, migrated.Money)
	assert.Equal(t, common.TopUpStatusPending, migrated.Status)
	_, err = creditedQuotaForTopUp(&migrated)
	assert.ErrorIs(t, err, ErrTopUpQuotaUnitUnresolved)
}

func TestTopUpCreditPreservesProviderUnitsAndRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		provider string
		amount   int64
		money    float64
		want     int
	}{
		{PaymentProviderStripe, 2, 12.34, 1234},
		{PaymentProviderCreem, 321, 3.21, 321},
		{PaymentProviderEpay, 2, 2, 200},
		{PaymentProviderWaffo, 2, 2, 200},
		{PaymentProviderWaffoPancake, 2, 2, 200},
	} {
		quota, err := creditedQuotaForTopUp(&TopUp{PaymentProvider: tc.provider, Amount: tc.amount, Money: tc.money, QuotaPerUnitSnapshot: "100"})
		require.NoError(t, err)
		assert.Equal(t, tc.want, quota)
	}
	for _, money := range []float64{math.NaN(), math.Inf(1), -1, 0} {
		_, err := creditedQuotaForTopUp(&TopUp{PaymentProvider: PaymentProviderStripe, Money: money, QuotaPerUnitSnapshot: "100"})
		assert.ErrorIs(t, err, ErrInvalidTopUpQuota)
	}
	_, err := creditedQuotaForTopUp(&TopUp{PaymentProvider: PaymentProviderEpay, Amount: math.MaxInt64, QuotaPerUnitSnapshot: "100"})
	assert.ErrorIs(t, err, ErrInvalidTopUpQuota)
}
