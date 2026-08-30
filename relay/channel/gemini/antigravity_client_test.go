package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
)

func TestAntigravityClientLifecycleAndUsage(t *testing.T) {
	const apiKey = "test-key-must-not-leak"
	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		if got := r.Header.Get("x-goog-api-key"); got != apiKey {
			t.Errorf("API key header = %q, want configured key", got)
		}
		if got := r.Header.Get("Api-Revision"); got != "2026-05-20" {
			t.Errorf("Api-Revision = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == AntigravityInteractionsPath:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read create body: %v", err)
			}
			var request AntigravityLifecycleRequest
			if err := common.Unmarshal(body, &request); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			var fields map[string]any
			if err := common.Unmarshal(body, &fields); err != nil {
				t.Fatalf("decode create fields: %v", err)
			}
			if _, exists := fields["environment_id"]; exists {
				t.Errorf("request emitted response-only environment_id field: %s", body)
			}
			if request.Agent != AntigravityAgentPreview || request.Input != "run a bounded task" ||
				!request.Background || request.Environment != "env_remote_1" ||
				request.PreviousInteractionID != "interaction-prev" {
				t.Errorf("unexpected create request: %+v", request)
			}
			if request.AgentConfig == nil || request.AgentConfig.Type != "dynamic" ||
				request.AgentConfig.MaxTotalTokens != 2048 {
				t.Errorf("unexpected agent config: %+v", request.AgentConfig)
			}
			_, _ = w.Write([]byte(`{"id":"interaction-1","object":"interaction","status":"in_progress","usage":{"total_input_tokens":3,"total_output_tokens":0,"total_tokens":3}}`))
		case r.Method == http.MethodGet && r.URL.Path == AntigravityInteractionsPath+"/interaction-1":
			_, _ = w.Write([]byte(`{"id":"interaction-1","object":"interaction","status":"completed","output_text":"done","usage":{"total_input_tokens":3,"total_output_tokens":7,"total_tokens":10}}`))
		case r.Method == http.MethodPost && r.URL.Path == AntigravityInteractionsPath+"/interaction-1/cancel":
			_, _ = w.Write([]byte(`{"id":"interaction-1","object":"interaction","status":"cancelled"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewAntigravityClient(AntigravityClientConfig{
		BaseURL:     server.URL,
		APIKey:      apiKey,
		APIRevision: "2026-05-20",
		HTTPClient:  server.Client(),
	})
	if err != nil {
		t.Fatalf("NewAntigravityClient() error = %v", err)
	}
	created, err := client.Create(context.Background(), AntigravityLifecycleRequest{
		Agent:                 AntigravityAgentPreview,
		Input:                 "run a bounded task",
		Environment:           "env_remote_1",
		Background:            true,
		PreviousInteractionID: "interaction-prev",
		AgentConfig: &AntigravityLifecycleAgentConfig{
			Type:           "dynamic",
			MaxTotalTokens: 2048,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != "interaction-1" || created.Status != "in_progress" || created.Usage == nil || created.Usage.TotalTokens != 3 {
		t.Fatalf("unexpected create response: %+v", created)
	}

	polled, err := client.Poll(context.Background(), created.ID, 0, 2)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if polled.Status != "completed" || polled.OutputText != "done" || polled.Usage == nil || polled.Usage.TotalTokens != 10 {
		t.Fatalf("unexpected poll response: %+v", polled)
	}
	cancelled, err := client.Cancel(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Fatalf("unexpected cancel response: %+v", cancelled)
	}

	wantRequests := []string{
		http.MethodPost + " " + AntigravityInteractionsPath,
		http.MethodGet + " " + AntigravityInteractionsPath + "/interaction-1",
		http.MethodPost + " " + AntigravityInteractionsPath + "/interaction-1/cancel",
	}
	if strings.Join(requests, "|") != strings.Join(wantRequests, "|") {
		t.Fatalf("requests = %v, want %v", requests, wantRequests)
	}
}

func TestAntigravityClientRejectsUnsafeConfigurationAndIDs(t *testing.T) {
	if _, err := NewAntigravityClient(AntigravityClientConfig{BaseURL: "http://example.test", APIKey: "secret"}); err == nil {
		t.Fatal("expected non-HTTPS base URL to be rejected")
	}
	if _, err := NewAntigravityClient(AntigravityClientConfig{BaseURL: "https://example.test", APIKey: ""}); err == nil {
		t.Fatal("expected missing API key to be rejected")
	}
	client, err := NewAntigravityClient(AntigravityClientConfig{BaseURL: "https://example.test", APIKey: "secret"})
	if err != nil {
		t.Fatalf("NewAntigravityClient() error = %v", err)
	}
	for _, id := range []string{"../escape", "a/b", "a?b", strings.Repeat("x", 257)} {
		if _, err := client.Get(context.Background(), id); err == nil {
			t.Errorf("Get(%q) unexpectedly succeeded", id)
		}
	}
	if err := (AntigravityLifecycleRequest{
		Agent:                 AntigravityAgentPreview,
		Input:                 "hello",
		Environment:           AntigravityEnvironmentRemote,
		PreviousInteractionID: "interaction-previous",
	}).Validate(); err == nil {
		t.Fatal("expected continuation without an environment id to be rejected")
	}
	if err := (AntigravityLifecycleRequest{Agent: AntigravityAgentPreview, Input: "hello", AgentConfig: &AntigravityLifecycleAgentConfig{Type: "static"}}).Validate(); err == nil {
		t.Fatal("expected unsupported agent config type to be rejected")
	}
}

func TestAntigravityClientDoesNotExposeUpstreamBodyInError(t *testing.T) {
	const secretText = "provider-body-secret"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"` + secretText + `"}}`))
	}))
	defer server.Close()
	client, err := NewAntigravityClient(AntigravityClientConfig{BaseURL: server.URL, APIKey: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewAntigravityClient() error = %v", err)
	}
	_, err = client.Create(context.Background(), AntigravityLifecycleRequest{Agent: AntigravityAgentPreview, Input: "hello"})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	if strings.Contains(err.Error(), secretText) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked upstream or credential text: %v", err)
	}
}

func TestAntigravityClientPollHonorsContext(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"interaction-1","status":"in_progress"}`))
	}))
	defer server.Close()
	client, err := NewAntigravityClient(AntigravityClientConfig{BaseURL: server.URL, APIKey: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewAntigravityClient() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Poll(ctx, "interaction-1", time.Second, 2); err == nil {
		t.Fatal("expected cancelled context to stop polling")
	}
}

func TestAntigravityClientPollStopsAtRequiresAction(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"id":"interaction-1","status":"requires_action"}`))
	}))
	defer server.Close()
	client, err := NewAntigravityClient(AntigravityClientConfig{BaseURL: server.URL, APIKey: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewAntigravityClient() error = %v", err)
	}
	interaction, err := client.Poll(nil, "interaction-1", time.Hour, 10)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if interaction.Status != "requires_action" || requests != 1 {
		t.Fatalf("Poll() returned %+v after %d requests, want one requires_action response", interaction, requests)
	}
}

func TestAntigravityClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"interaction-1","status":"completed","output_text":"`)
		_, _ = io.WriteString(w, strings.Repeat("x", AntigravityMaxResponseBytes))
		_, _ = io.WriteString(w, `"}`)
	}))
	defer server.Close()
	client, err := NewAntigravityClient(AntigravityClientConfig{BaseURL: server.URL, APIKey: "secret", HTTPClient: server.Client()})
	if err != nil {
		t.Fatalf("NewAntigravityClient() error = %v", err)
	}
	if _, err := client.Get(context.Background(), "interaction-1"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Get() error = %v, want oversized response error", err)
	}
}
