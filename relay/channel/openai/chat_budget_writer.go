package openai

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// Some legacy SSE helpers discard Write errors. Observe the existing writer
// only for strict Chat so no failed downstream frame can release its reserve.
// Keep Gin's remaining ResponseWriter behavior and ordinary formatting intact.
type strictChatWriteObserver struct {
	gin.ResponseWriter
	requestContext context.Context
	failed         atomic.Bool
	donePrefix     atomic.Bool
	doneWritten    atomic.Bool
}

func (w *strictChatWriteObserver) Write(data []byte) (int, error) {
	w.observeCancellation()
	n, err := w.ResponseWriter.Write(data)
	if err != nil || n != len(data) {
		w.failed.Store(true)
	} else {
		w.observeTerminal(string(data))
	}
	return n, err
}

func (w *strictChatWriteObserver) WriteString(data string) (int, error) {
	w.observeCancellation()
	n, err := w.ResponseWriter.WriteString(data)
	if err != nil || n != len(data) {
		w.failed.Store(true)
	} else {
		w.observeTerminal(data)
	}
	return n, err
}

func (w *strictChatWriteObserver) observeCancellation() {
	if w.requestContext != nil && w.requestContext.Err() != nil {
		w.failed.Store(true)
	}
}

// CustomEvent writes the terminal payload and its SSE boundary separately.
// Keep only frame state, never response contents. Delivery is not usage proof:
// this solely preserves the existing unknown-hold boundary on interrupted
// writes while allowing cancellation after a complete terminal write.
func (w *strictChatWriteObserver) observeTerminal(data string) {
	if data == "data: [DONE]\n\n" || w.donePrefix.Load() && data == "\n\n" {
		w.doneWritten.Store(true)
	}
	w.donePrefix.Store(data == "data: [DONE]")
}

func (w *strictChatWriteObserver) Flush() {
	defer func() {
		if value := recover(); value != nil {
			w.failed.Store(true)
			panic(value)
		}
	}()
	w.ResponseWriter.Flush()
}

func (w *strictChatWriteObserver) Unwrap() http.ResponseWriter { return w.ResponseWriter }
