package common

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJsonRawMessageToString(t *testing.T) {
	tests := []struct {
		name string
		data json.RawMessage
		want string
	}{
		{name: "object", data: json.RawMessage(`{"city":"Paris","days":0,"strict":false}`), want: `{"city":"Paris","days":0,"strict":false}`},
		{name: "string", data: json.RawMessage(`"{\"city\":\"Paris\",\"days\":0,\"strict\":false}"`), want: `{"city":"Paris","days":0,"strict":false}`},
		{name: "null", data: json.RawMessage(`null`), want: ""},
		{name: "empty", data: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { require.Equal(t, tt.want, JsonRawMessageToString(tt.data)) })
	}
}

func TestDecodeJsonStrictRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	var decoded payload
	require.NoError(t, DecodeJsonStrict(strings.NewReader(`{"name":"ok"}`), &decoded))
	require.Equal(t, "ok", decoded.Name)
	require.Error(t, DecodeJsonStrict(strings.NewReader(`{"name":"ok","extra":true}`), &decoded))
	require.Error(t, DecodeJsonStrict(strings.NewReader(`{"name":"ok"}{"name":"second"}`), &decoded))
}

func TestStringJSONHelpersUseWrapperSemantics(t *testing.T) {
	require.Equal(t, `{"name":"ok"}`, MapToJsonStr(map[string]interface{}{"name": "ok"}))
	values, err := StrToJsonArray(`[1,"two"]`)
	require.NoError(t, err)
	require.Len(t, values, 2)
	require.True(t, IsJsonArray(`[1]`))
	require.False(t, IsJsonArray(`{"x":1}`))
	require.True(t, IsJsonObject(`{"x":1}`))
	require.False(t, IsJsonObject(`[1]`))
}
