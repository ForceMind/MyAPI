package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ForceMind/MyAPI/common"
)

const upstreamDiagnosticEnv = "MYAPI_UPSTREAM_DIAGNOSTIC_ENABLED"

// Bound extra memory per observed request/event. Oversized or incomplete JSON
// is explicitly omitted, never logged as an unredacted prefix.
const upstreamDiagnosticMaxBytes = 8 << 20

func upstreamDiagnosticEnabled() bool {
	enabled, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(upstreamDiagnosticEnv)))
	return err == nil && enabled
}

type upstreamDiagnosticObserver struct {
	writer   *fullContentFileWriter
	identity fullContentRequestIdentity
	attempts atomic.Int64
}

func (o *upstreamDiagnosticObserver) ObserveRequest(req *http.Request, channelID int) func(*http.Response) {
	attempt := o.attempts.Add(1)
	entry := newFullContentLogEntry(o.identity, "upstream_request")
	entry.ChannelID, entry.Attempt = channelID, attempt
	entry.Method, entry.Path = req.Method, req.URL.EscapedPath()
	entry.ContentType = req.Header.Get("Content-Type")
	// Never record upstream headers, URL userinfo, host, or query parameters.
	body := &upstreamDiagnosticRequestBody{
		ReadCloser: req.Body, expected: req.ContentLength, entry: entry, writer: o.writer,
	}
	if req.Body != nil {
		req.Body = body
	}
	return func(resp *http.Response) {
		body.finish()
		if resp == nil || resp.Body == nil {
			return
		}
		contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		// Codex may omit Content-Type, including for buffered Chat callers whose
		// actual upstream request was forced to stream. Use only the protocol
		// observed in the sent body; never infer it from the client-facing mode.
		if strings.TrimSpace(resp.Header.Get("Content-Type")) == "" && body.stream != nil {
			contentType = "application/json"
			if *body.stream {
				contentType = "text/event-stream"
			}
		}
		usageEntry := newFullContentLogEntry(o.identity, "upstream_usage")
		usageEntry.ChannelID, usageEntry.Attempt = channelID, attempt
		usageEntry.Method, usageEntry.Path = entry.Method, entry.Path
		usageEntry.Status, usageEntry.ContentType = resp.StatusCode, resp.Header.Get("Content-Type")
		resp.Body = &upstreamDiagnosticResponseBody{
			ReadCloser: resp.Body, entry: usageEntry, writer: o.writer,
			sse: contentType == "text/event-stream", json: contentType == "application/json",
		}
	}
}

type upstreamDiagnosticRequestBody struct {
	io.ReadCloser
	mu       sync.Mutex
	once     sync.Once
	buffer   []byte
	total    int64
	eof      bool
	expected int64
	stream   *bool
	entry    fullContentLogEntry
	writer   *fullContentFileWriter
}

func (b *upstreamDiagnosticRequestBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	b.total += int64(n)
	if n > 0 && len(b.buffer)+n <= upstreamDiagnosticMaxBytes {
		b.buffer = append(b.buffer, p[:n]...)
	}
	if err == io.EOF {
		b.eof = true
	}
	b.mu.Unlock()
	return n, err
}

func (b *upstreamDiagnosticRequestBody) Close() error {
	err := b.ReadCloser.Close()
	b.finish()
	return err
}

func (b *upstreamDiagnosticRequestBody) finish() {
	b.once.Do(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		defer func() { b.buffer = nil }()
		entry := b.entry
		entry.BodyBytes = b.total
		entry.Capture = "partial"
		entry.Encoding = "omitted_incomplete_json"
		if b.eof || b.expected >= 0 && b.total == b.expected {
			entry.Capture = "complete"
			if b.total > upstreamDiagnosticMaxBytes {
				entry.Capture, entry.Encoding = "oversized", "omitted_oversized_json"
			} else if redacted, err := redactUpstreamDiagnosticJSON(b.buffer); err == nil {
				entry.Body, entry.Encoding = string(redacted), "json"
				var protocol struct {
					Stream *bool `json:"stream"`
				}
				if common.Unmarshal(b.buffer, &protocol) == nil {
					b.stream = protocol.Stream
				}
			} else {
				entry.Capture, entry.Encoding = "invalid", "omitted_invalid_json"
			}
		}
		writeFullContentLogEntry(b.writer, entry)
	})
}

// This reader observes exactly the bytes consumed by existing response
// handlers. It does not pre-read, replace usage, retry, or log response text.
type upstreamDiagnosticResponseBody struct {
	io.ReadCloser
	mu           sync.Mutex
	entry        fullContentLogEntry
	writer       *fullContentFileWriter
	sse          bool
	json         bool
	buffer       []byte
	data         []byte
	droppingLine bool
	droppedEvent bool
	recorded     bool
	finished     bool
	done         bool
	sequence     int64
}

func (b *upstreamDiagnosticResponseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	if !b.finished {
		if b.sse {
			b.observeSSE(p[:n])
		} else if b.json {
			if len(b.buffer)+n <= upstreamDiagnosticMaxBytes && !b.droppedEvent {
				b.buffer = append(b.buffer, p[:n]...)
			} else {
				b.buffer, b.droppedEvent = nil, true
			}
		}
		if err != nil {
			b.finish(err == io.EOF)
		}
	}
	b.mu.Unlock()
	return n, err
}

func (b *upstreamDiagnosticResponseBody) Close() error {
	err := b.ReadCloser.Close()
	b.mu.Lock()
	if !b.finished {
		b.finish(false)
	}
	b.mu.Unlock()
	return err
}

func (b *upstreamDiagnosticResponseBody) observeSSE(p []byte) {
	for _, ch := range p {
		if ch != '\n' {
			if len(b.buffer) < upstreamDiagnosticMaxBytes && !b.droppingLine {
				b.buffer = append(b.buffer, ch)
			} else {
				b.buffer, b.droppingLine, b.droppedEvent = nil, true, true
			}
			continue
		}
		b.observeSSELine()
	}
}

func (b *upstreamDiagnosticResponseBody) observeSSELine() {
	line := bytes.TrimSuffix(b.buffer, []byte{'\r'})
	if !b.droppingLine {
		if len(line) == 0 {
			if b.droppedEvent {
				b.recordUnobserved("oversized")
			} else if len(b.data) > 0 {
				b.recordUsage(b.data)
			}
			b.data, b.droppedEvent = nil, false
		} else if bytes.HasPrefix(line, []byte("data:")) && !b.droppedEvent {
			value := bytes.TrimPrefix(line[5:], []byte{' '})
			if len(b.data)+len(value)+1 <= upstreamDiagnosticMaxBytes {
				if len(b.data) > 0 {
					b.data = append(b.data, '\n')
				}
				b.data = append(b.data, value...)
			} else {
				b.data, b.droppedEvent = nil, true
			}
		}
	}
	b.buffer, b.droppingLine = nil, false
}

func (b *upstreamDiagnosticResponseBody) finish(complete bool) {
	b.finished = true
	complete = complete || b.sse && b.done
	if complete && b.sse {
		if len(b.buffer) > 0 || b.droppingLine {
			b.observeSSELine()
		}
		if b.droppedEvent {
			b.recordUnobserved("oversized")
		} else if len(b.data) > 0 {
			b.recordUsage(b.data)
		}
	} else if complete && b.json && !b.droppedEvent {
		b.recordUsage(b.buffer)
	}
	if !complete || !b.recorded {
		state := "unobserved"
		if !complete {
			state = "partial"
		} else if b.droppedEvent {
			state = "oversized"
		} else if !b.sse && !b.json {
			state = "unsupported_content_type"
		}
		b.recordUnobserved(state)
	}
	b.buffer, b.data = nil, nil
}

type upstreamDiagnosticUsage struct {
	EventType   string          `json:"event_type"`
	UsageSource string          `json:"usage_source,omitempty"`
	UsageState  string          `json:"usage_state"`
	Usage       json.RawMessage `json:"usage,omitempty"`
}

func (b *upstreamDiagnosticResponseBody) recordUsage(raw []byte) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
		b.done = true
		return
	}
	var event map[string]json.RawMessage
	if common.Unmarshal(raw, &event) != nil || event == nil {
		b.recordUnobserved("invalid_json")
		return
	}
	var eventType string
	_ = common.Unmarshal(event["type"], &eventType)
	terminal := false
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete":
		terminal = true
	case "response.created", "response.in_progress", "response.usage", "usage":
	default:
		eventType = "other"
	}
	type usageField struct {
		source  string
		value   json.RawMessage
		present bool
	}
	fields := make([]usageField, 0, 2)
	usage, present := event["usage"]
	if present {
		fields = append(fields, usageField{source: "usage", value: usage, present: true})
	}
	if response, ok := event["response"]; ok {
		var object map[string]json.RawMessage
		if common.Unmarshal(response, &object) == nil {
			if nested, ok := object["usage"]; ok || !present {
				fields = append(fields, usageField{source: "response.usage", value: nested, present: ok})
			}
		}
	}
	if len(fields) == 0 {
		fields = append(fields, usageField{source: "usage"})
	}
	for _, field := range fields {
		if b.sse && !field.present && !terminal {
			continue
		}
		record := upstreamDiagnosticUsage{EventType: eventType, UsageSource: field.source, UsageState: "missing"}
		if field.present {
			record.UsageState = "present"
			if bytes.Equal(bytes.TrimSpace(field.value), []byte("null")) {
				record.UsageState = "null"
			}
			redacted, err := redactUpstreamDiagnosticJSON(field.value)
			if err != nil {
				b.recordUnobserved("invalid_usage_json")
				continue
			}
			record.Usage = redacted
		}
		encoded, err := common.Marshal(record)
		if err != nil {
			b.recordUnobserved("encoding_failed")
			continue
		}
		entry := b.entry
		b.sequence++
		entry.Sequence, entry.Timestamp = b.sequence, time.Now().UTC().Format(time.RFC3339Nano)
		entry.Body, entry.Encoding, entry.Capture = string(encoded), "json", "complete"
		entry.BodyBytes = int64(len(raw))
		writeFullContentLogEntry(b.writer, entry)
		b.recorded = true
	}
}

func (b *upstreamDiagnosticResponseBody) recordUnobserved(state string) {
	entry := b.entry
	b.sequence++
	entry.Sequence, entry.Timestamp = b.sequence, time.Now().UTC().Format(time.RFC3339Nano)
	entry.Capture, entry.Encoding = state, "omitted_unobserved_usage"
	writeFullContentLogEntry(b.writer, entry)
	b.recorded = true
}

// RawMessage preserves number lexemes and missing/null fields. Decoding into
// interface{} here would round large counts through float64 before logging.
func redactUpstreamDiagnosticJSON(raw []byte) ([]byte, error) {
	var validated json.RawMessage
	if err := common.Unmarshal(raw, &validated); err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(validated)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var object map[string]json.RawMessage
		if err := common.Unmarshal(trimmed, &object); err != nil {
			return nil, err
		}
		for key, child := range object {
			if isFullContentLogSecretKey(key) {
				object[key] = json.RawMessage(`"[REDACTED]"`)
				continue
			}
			value, err := redactUpstreamDiagnosticJSON(child)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		return common.Marshal(object)
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var values []json.RawMessage
		if err := common.Unmarshal(trimmed, &values); err != nil {
			return nil, err
		}
		for i, child := range values {
			value, err := redactUpstreamDiagnosticJSON(child)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return common.Marshal(values)
	}
	return trimmed, nil
}
