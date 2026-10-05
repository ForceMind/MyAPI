package controller

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRetrySafetyGuardsPrecedeChannelErrors(t *testing.T) {
	for _, name := range []string{"budget exhausted", "skip retry", "specific channel", "response started", "canceled"} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			remaining := 1
			err := types.NewError(errors.New("fixture"), types.ErrorCodeChannelInvalidKey)
			switch name {
			case "budget exhausted":
				remaining = 0
			case "skip retry":
				err = types.NewError(errors.New("fixture"), types.ErrorCodeChannelInvalidKey, types.ErrOptionWithSkipRetry())
			case "specific channel":
				c.Set("specific_channel_id", 1)
			case "response started":
				c.Writer.WriteString("data: fixture\n\n")
			case "canceled":
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			}
			assert.False(t, shouldRetry(c, err, remaining))
		})
	}
}
