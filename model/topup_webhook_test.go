package model

import (
	"errors"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdatePendingTopUpStatusRejectsStaleRead(t *testing.T) {
	db := accessProfileTestDB(t)
	require.NoError(t, db.AutoMigrate(&TopUp{}))
	order := TopUp{TradeNo: "already-success", PaymentProvider: PaymentProviderStripe, Status: common.TopUpStatusSuccess, CompleteTime: 123, Amount: 7}
	require.NoError(t, db.Create(&order).Error)
	// Deterministically simulate a pending snapshot read before another
	// transaction's successful completion. The durable row is already success.
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("fixture:stale-pending", func(tx *gorm.DB) {
		if topUp, ok := tx.Statement.Dest.(*TopUp); ok && tx.Error == nil {
			topUp.Status = common.TopUpStatusPending
		}
	}))
	err := UpdatePendingTopUpStatus(order.TradeNo, PaymentProviderStripe, common.TopUpStatusFailed)
	require.NoError(t, db.Callback().Query().Remove("fixture:stale-pending"))
	assert.ErrorIs(t, err, ErrTopUpStatusInvalid)
	require.NoError(t, db.First(&order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.EqualValues(t, 123, order.CompleteTime)
	assert.EqualValues(t, 7, order.Amount)
}

func TestUpdatePendingTopUpStatusPreservesDatabaseFailure(t *testing.T) {
	db := accessProfileTestDB(t)
	injected := errors.New("injected read failure")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("fixture:read-failure", func(tx *gorm.DB) { tx.AddError(injected) }))
	err := UpdatePendingTopUpStatus("stripe-order", PaymentProviderStripe, common.TopUpStatusFailed)
	assert.ErrorIs(t, err, injected)
	assert.NotErrorIs(t, err, ErrTopUpNotFound)
}
