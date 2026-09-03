package zhipu

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZhipuStreamRejectsMissingUsage(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"empty", ""},
		{"content without metadata", "data:hello\n"},
		{"metadata without usage", "meta:{\"request_id\":\"test\"}\n"},
		{"null usage", "meta:{\"usage\":null}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, recorder := zhipuTestContext()
			usage, apiErr := zhipuStreamHandler(c, zhipuTestInfo(), &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)),
			})
			require.NotNil(t, apiErr, "an incomplete stream must not be successful")
			assert.Nil(t, usage)
			assert.NotContains(t, recorder.Body.String(), "[DONE]")
		})
	}
}

func TestZhipuStreamPreservesContentAndFinalUsage(t *testing.T) {
	c, recorder := zhipuTestContext()
	body := "data:first\ndata: second\nmeta:{\"request_id\":\"test\",\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n"
	usage, apiErr := zhipuStreamHandler(c, zhipuTestInfo(), &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 7, usage.TotalTokens)
	frames := strings.Split(strings.TrimSpace(recorder.Body.String()), "\n\n")
	require.Len(t, frames, 4)
	var text strings.Builder
	for index, frame := range frames[:3] {
		require.True(t, strings.HasPrefix(frame, "data: "))
		var event dto.ChatCompletionsStreamResponse
		require.NoError(t, common.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event))
		require.Len(t, event.Choices, 1)
		text.WriteString(event.Choices[0].Delta.GetContentString())
		if index == 2 {
			require.NotNil(t, event.Choices[0].FinishReason)
			assert.Equal(t, "stop", *event.Choices[0].FinishReason)
			assert.Equal(t, "test", event.Id)
		}
	}
	assert.Equal(t, "first second", text.String())
	assert.Equal(t, "data: [DONE]", frames[3])
}

func TestZhipuInvalidCredentialsAreRejectedWithoutDisclosure(t *testing.T) {
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	oldWriter, oldErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = &logs, &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = oldWriter, oldErrorWriter
		common.LogWriterMu.Unlock()
	})
	for _, key := range []string{"fixture-secret", "fixture.secret.extra", ".fixture-secret", "fixture-id.", " fixture.secret"} {
		c, _ := zhipuTestContext()
		header := make(http.Header)
		err := (&Adaptor{}).SetupRequestHeader(c, &header, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: key}})
		assert.Error(t, err)
		assert.True(t, header.Get("Authorization") == "", "invalid credentials must not produce an authorization header")
		if err != nil {
			assert.NotContains(t, err.Error(), key)
		}
		assert.NotContains(t, logs.String(), key)
	}
}

func TestZhipuValidCredentialsPreserveSignedAuthorization(t *testing.T) {
	const key = "fixture-id.fixture-signing-secret"
	t.Cleanup(func() { zhipuTokens.Delete(key) })
	c, _ := zhipuTestContext()
	header := make(http.Header)
	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: key}}))
	token, err := jwt.Parse(header.Get("Authorization"), func(token *jwt.Token) (any, error) {
		return []byte("fixture-signing-secret"), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation())
	require.NoError(t, err)
	require.True(t, token.Valid)
	assert.Equal(t, "SIGN", token.Header["sign_type"])
	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "fixture-id", claims["api_key"])
	assert.Greater(t, claims["exp"].(float64), claims["timestamp"].(float64))
	cached, err := getZhipuToken(key)
	require.NoError(t, err)
	assert.True(t, cached == header.Get("Authorization"), "unexpired authorization should be reused")
}

type zhipuReadError struct{}

func (zhipuReadError) Read([]byte) (int, error) { return 0, errors.New("fixture read failure") }

func TestZhipuStreamReadErrorsNeverCompleteSuccessfully(t *testing.T) {
	for _, prefix := range []string{
		"",
		"meta:{not-json}\n",
		"meta:{\"usage\":{\"prompt_tokens\":5,\"completion_tokens\":2,\"total_tokens\":7}}\n",
	} {
		c, recorder := zhipuTestContext()
		reader := io.MultiReader(strings.NewReader(prefix), zhipuReadError{})
		usage, apiErr := zhipuStreamHandler(c, zhipuTestInfo(), &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader)})
		require.NotNil(t, apiErr)
		assert.Nil(t, usage)
		assert.NotContains(t, recorder.Body.String(), "[DONE]")
	}
}

type zhipuReadStartedBody struct {
	*io.PipeReader
	started chan struct{}
	once    sync.Once
}

func (b *zhipuReadStartedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	return b.PipeReader.Read(p)
}

type zhipuStreamResult struct {
	usage *dto.Usage
	err   *types.NewAPIError
}

func TestZhipuStreamCancellationClosesUpstreamAndReturns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	body := &zhipuReadStartedBody{PipeReader: reader, started: make(chan struct{})}
	c, recorder := zhipuTestContext()
	c.Request = c.Request.WithContext(ctx)
	done := make(chan zhipuStreamResult, 1)
	go func() {
		usage, apiErr := zhipuStreamHandler(c, zhipuTestInfo(), &http.Response{StatusCode: http.StatusOK, Body: body})
		done <- zhipuStreamResult{usage, apiErr}
	}()
	select {
	case <-body.started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream read did not start")
	}
	cancel()
	select {
	case result := <-done:
		require.NotNil(t, result.err)
		assert.Nil(t, result.usage)
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not release the upstream reader and handler")
	}
	_, err := writer.Write([]byte("data:late\n"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
	assert.Empty(t, recorder.Body.String())
}

type zhipuFailedWriter struct{ gin.ResponseWriter }

func (*zhipuFailedWriter) Write([]byte) (int, error) {
	return 0, errors.New("fixture downstream disconnect")
}

func (*zhipuFailedWriter) WriteString(string) (int, error) {
	return 0, errors.New("fixture downstream disconnect")
}

func TestZhipuStreamWriteFailureReleasesProducer(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	c, _ := zhipuTestContext()
	c.Writer = &zhipuFailedWriter{ResponseWriter: c.Writer}
	done := make(chan zhipuStreamResult, 1)
	go func() {
		usage, apiErr := zhipuStreamHandler(c, zhipuTestInfo(), &http.Response{StatusCode: http.StatusOK, Body: reader})
		done <- zhipuStreamResult{usage, apiErr}
	}()
	// The reader can consume this batch in one read. The producer may then
	// wait on a subsequent event send when the first client write fails.
	_, err := writer.Write([]byte("data:first\ndata:second\ndata:third\n"))
	require.NoError(t, err)
	select {
	case result := <-done:
		require.NotNil(t, result.err)
		assert.Nil(t, result.usage)
	case <-time.After(3 * time.Second):
		t.Fatal("downstream failure did not release the producer")
	}
	_, err = writer.Write([]byte("data:late\n"))
	require.ErrorIs(t, err, io.ErrClosedPipe)
}
