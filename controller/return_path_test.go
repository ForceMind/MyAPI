package controller

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

func TestPaymentReturnPathUsesDefaultDashboardRoutes(t *testing.T) {
	previousAddress := system_setting.GetServerAddress()
	system_setting.SetServerAddress("https://dashboard.example.com/")
	t.Cleanup(func() { system_setting.SetServerAddress(previousAddress) })

	assert.Equal(
		t,
		"https://dashboard.example.com/wallet?pay=success",
		paymentReturnPath("/wallet?pay=success"),
	)
	assert.Equal(
		t,
		"https://dashboard.example.com/usage-logs",
		paymentReturnPath("/usage-logs"),
	)
}
