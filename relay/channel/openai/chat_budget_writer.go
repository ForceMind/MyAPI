package openai

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// Some legacy SSE helpers discard Write errors. Observe the existing writer
// only for strict Chat so no failed downstream frame can release its reserve.
// Keep Gin's remaining ResponseWriter behavior and ordinary formatting intact.
type strictChatWriteObserver struct {
	gin.ResponseWriter
	failed atomic.Bool
}

func (w *strictChatWriteObserver) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if err != nil || n != len(data) {
		w.failed.Store(true)
	}
	return n, err
}

func (w *strictChatWriteObserver) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	if err != nil || n != len(data) {
		w.failed.Store(true)
	}
	return n, err
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
