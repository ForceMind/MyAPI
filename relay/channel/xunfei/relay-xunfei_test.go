package xunfei

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeXunfeiResponseRejectsMalformedJSON(t *testing.T) {
	var response XunfeiChatResponse
	err := decodeXunfeiResponse([]byte(`{"header":`), &response)

	require.Error(t, err)
}

func TestDecodeXunfeiResponseUsesCommonJSONSemantics(t *testing.T) {
	var response XunfeiChatResponse
	err := decodeXunfeiResponse([]byte(`{"header":{"code":0,"status":2},"payload":{"choices":{"text":[{"content":"ok"}]}}}`), &response)

	require.NoError(t, err)
	require.Equal(t, 2, response.Header.Status)
	require.Equal(t, "ok", response.Payload.Choices.Text[0].Content)

}
