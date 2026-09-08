package relay

import (
	"io"
	"net/http"
	"strings"
	"testing"

	taskdto "github.com/ForceMind/MyAPI/dto"
	"github.com/ForceMind/MyAPI/relay/channel"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type trackedLegacyTaskSubmitBody struct {
	reader    io.Reader
	bytesRead int
	closed    bool
}

func (body *trackedLegacyTaskSubmitBody) Read(data []byte) (int, error) {
	n, err := body.reader.Read(data)
	body.bytesRead += n
	return n, err
}

func (body *trackedLegacyTaskSubmitBody) Close() error {
	body.closed = true
	return nil
}

func TestLegacyTaskSubmitHTTPStatusErrorBoundsClosesAndRedactsBody(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	body := &trackedLegacyTaskSubmitBody{reader: strings.NewReader(
		sensitiveValue + strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1024),
	)}

	taskErr := legacyTaskSubmitHTTPStatusError(&http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       body,
	})

	require.True(t, body.closed)
	require.Equal(t, channel.MaxTaskSubmitResponseBytes+1, body.bytesRead)
	require.Equal(t, "fail_to_fetch_task", taskErr.Code)
	require.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	require.Equal(t, "task submission upstream returned an unexpected HTTP status", taskErr.Message)
	require.NotContains(t, taskErr.Message, sensitiveValue)
}

func TestLegacyTaskSubmitHTTPStatusErrorNormalizesInvalidStatus(t *testing.T) {
	taskErr := legacyTaskSubmitHTTPStatusError(&http.Response{StatusCode: 0})

	require.Equal(t, http.StatusBadGateway, taskErr.StatusCode)
	require.Equal(t, "fail_to_fetch_task", taskErr.Code)
}

type legacyTaskSubmitResponseProbe struct {
	doResponseCalled bool
}

var _ channel.TaskSubmitResponseParser = (*legacyTaskSubmitResponseProbe)(nil)

func (probe *legacyTaskSubmitResponseProbe) DoResponse(_ *gin.Context, _ *http.Response, _ *relaycommon.RelayInfo) (string, []byte, *taskdto.TaskError) {
	probe.doResponseCalled = true
	return "unexpected", []byte(`{}`), nil
}

func (*legacyTaskSubmitResponseProbe) ParseTaskSubmitResponse(channel.TaskSubmitParseInput) channel.TaskSubmitParseResult {
	return channel.TaskSubmitParseResult{}
}

func TestResolveLegacyTaskSubmitResponseSkipsMigratedParserOnNon200(t *testing.T) {
	const sensitiveValue = "sk-sensitive-upstream-value"
	body := &trackedLegacyTaskSubmitBody{reader: strings.NewReader(
		sensitiveValue + strings.Repeat("x", channel.MaxTaskSubmitResponseBytes+1024),
	)}
	probe := &legacyTaskSubmitResponseProbe{}

	taskID, taskData, taskErr := resolveLegacyTaskSubmitResponse(nil, probe, &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       body,
	}, nil)

	require.False(t, probe.doResponseCalled)
	require.Empty(t, taskID)
	require.Empty(t, taskData)
	require.True(t, body.closed)
	require.Equal(t, channel.MaxTaskSubmitResponseBytes+1, body.bytesRead)
	require.NotNil(t, taskErr)
	require.Equal(t, "fail_to_fetch_task", taskErr.Code)
	require.Equal(t, http.StatusServiceUnavailable, taskErr.StatusCode)
	require.False(t, taskErr.LocalError)
	require.Equal(t, "task submission upstream returned an unexpected HTTP status", taskErr.Message)
	require.NotContains(t, taskErr.Message, sensitiveValue)
}
