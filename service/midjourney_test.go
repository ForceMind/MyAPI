package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeMidjourneyResponse(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		code        int
		description string
		result      string
		wantErr     bool
	}{
		{
			name:        "scalar response",
			body:        `{"code":1,"description":"submitted","result":"task-123"}`,
			code:        1,
			description: "submitted",
			result:      "task-123",
		},
		{
			name:        "upload response projects first result",
			body:        `{"code":1,"description":"uploaded","result":["https://cdn.example/one.png","https://cdn.example/two.png"]}`,
			code:        1,
			description: "uploaded",
			result:      "https://cdn.example/one.png",
		},
		{
			name:    "malformed response",
			body:    `{"code":1,"result":`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := decodeMidjourneyResponse([]byte(test.body))
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.code, response.Code)
			require.Equal(t, test.description, response.Description)
			require.Equal(t, test.result, response.Result)
		})
	}
}
