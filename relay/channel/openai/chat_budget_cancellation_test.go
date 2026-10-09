package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/constant"
	"github.com/ForceMind/MyAPI/model"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	relayconstant "github.com/ForceMind/MyAPI/relay/constant"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type chatCancellationWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	stage  string
}

func (w *chatCancellationWriter) Write(data []byte) (int, error) {
	if w.stage == "done-write-error" && strings.Contains(string(data), "[DONE]") {
		w.cancel()
		return 0, io.ErrClosedPipe
	}
	if strings.HasSuffix(w.Body.String(), "data: [DONE]") && string(data) == "\n\n" {
		if w.stage == "done-boundary-in-write-cancel" {
			// Cancellation after observer entry is ordered only against the
			// successful write result, not physical network delivery.
			w.cancel()
		}
		if w.stage == "done-boundary-error" {
			w.cancel()
			return 0, io.ErrClosedPipe
		}
		if w.stage == "done-boundary-short-write" {
			w.cancel()
			return 1, nil
		}
	}
	n, err := w.ResponseRecorder.Write(data)
	if w.stage == "usage-write" && strings.Contains(string(data), `"prompt_tokens":100`) ||
		w.stage == "done-prefix" && strings.Contains(string(data), "[DONE]") ||
		w.stage == "done-event" && strings.HasSuffix(w.Body.String(), "data: [DONE]\n\n") {
		w.cancel()
	}
	return n, err
}

func (w *chatCancellationWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func TestStrictChatCancellationTerminalWriteBoundary(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	for _, stage := range []string{"pre-cancelled", "usage-write", "done-prefix", "done-event", "done-boundary-in-write-cancel", "done-write-error", "done-boundary-error", "done-boundary-short-write"} {
		t.Run(stage, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			ctx, cancel := context.WithCancel(request.Context())
			defer cancel()
			writer := &chatCancellationWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, stage: stage}
			c, _ := gin.CreateTestContext(writer)
			c.Request = request.WithContext(ctx)
			if stage == "pre-cancelled" {
				cancel()
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: model.TokenBudgetOpenAIChatModel}, StrictTokenBudget: true, IsStream: true, DisablePing: true, ShouldIncludeUsage: true, RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: types.RelayFormatOpenAI}
			body := "data: " + chatQualifiedResponse(chatQualifiedUsage, true) + "\n\ndata: [DONE]\n\n"
			usage, apiErr := OaiStreamHandler(c, info, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))})
			require.Nil(t, apiErr)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.NotNil(t, usage.BillingUsage)
			// Match the service's captured-completion predicate, without using
			// the final request context to conceal pre-terminal cancellation.
			qualified := usage.BillingUsage.ChatTextEvidence != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone && info.StreamStatus.EndError == nil && !info.StreamStatus.HasErrors()
			acceptedTerminal := stage == "done-event" || stage == "done-boundary-in-write-cancel"
			assert.Equal(t, acceptedTerminal, qualified)
			if acceptedTerminal {
				assert.Contains(t, writer.Body.String(), "data: [DONE]\n\n")
			} else {
				assert.True(t, info.StreamStatus.HasErrors())
			}
		})
	}
}

func TestStrictChatWriterCancelledBeforeWriteFailsClosed(t *testing.T) {
	for _, useString := range []bool{false, true} {
		request, cancel := context.WithCancel(context.Background())
		cancel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		writer := &strictChatWriteObserver{ResponseWriter: c.Writer, requestContext: request}
		if useString {
			_, err := writer.WriteString("data: [DONE]\n\n")
			require.NoError(t, err)
		} else {
			_, err := writer.Write([]byte("data: [DONE]\n\n"))
			require.NoError(t, err)
		}
		assert.True(t, writer.failed.Load(), "even a successful buffered write cannot erase earlier cancellation")
	}
}

type chatFlushPanicWriter struct{ *httptest.ResponseRecorder }

func (w *chatFlushPanicWriter) Flush() { panic("synthetic flush failure") }
func TestStrictChatWriterFlushPanicRetainsFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(&chatFlushPanicWriter{httptest.NewRecorder()})
	writer := &strictChatWriteObserver{ResponseWriter: c.Writer}
	require.Panics(t, writer.Flush)
	assert.True(t, writer.failed.Load())
}
