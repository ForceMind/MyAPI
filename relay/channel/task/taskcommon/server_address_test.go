package taskcommon

import (
	"testing"

	"github.com/ForceMind/MyAPI/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

func TestBuildProxyURLPreservesRawServerAddressConcatenation(t *testing.T) {
	previousServerAddress := system_setting.GetServerAddress()
	t.Cleanup(func() { system_setting.SetServerAddress(previousServerAddress) })

	testCases := []struct {
		name          string
		serverAddress string
		want          string
	}{
		{
			name:          "trailing slash is retained",
			serverAddress: "https://dashboard.example.test/",
			want:          "https://dashboard.example.test//v1/videos/task_123/content",
		},
		{
			name:          "empty address produces relative URL",
			serverAddress: "",
			want:          "/v1/videos/task_123/content",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			system_setting.SetServerAddress(testCase.serverAddress)
			assert.Equal(t, testCase.want, BuildProxyURL("task_123"))
		})
	}
}
