package common

import (
	"net/http/httptest"
	"testing"

	appcommon "github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayPreservesStoredUserQuota(t *testing.T) {
	for _, input := range []any{int(100), int64(1<<32 + 17)} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		appcommon.SetContextKey(c, constant.ContextKeyUserQuota, input)
		info := genBaseRelayInfo(c, nil)
		var expected int64
		switch value := input.(type) {
		case int:
			expected = int64(value)
		case int64:
			expected = value
		}
		require.EqualValues(t, expected, info.UserQuota)
	}
}
