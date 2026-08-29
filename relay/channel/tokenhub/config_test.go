package tokenhub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/dto"
)

func TestNormalizeBaseURL(t *testing.T) {
	got, err := NormalizeBaseURL(" https://tokenhub.example.test/v1/ ")
	if err != nil || got != "https://tokenhub.example.test/v1" {
		t.Fatalf("NormalizeBaseURL() = %q, %v", got, err)
	}
}

func TestNormalizeBaseURLRejectsEmbeddedCredentials(t *testing.T) {
	if _, err := NormalizeBaseURL("https://user:secret@example.test"); err == nil {
		t.Fatal("expected credentials in base URL to be rejected")
	}
}

func TestConfigValidateProtocol(t *testing.T) {
	if err := (Config{BaseURL: "https://tokenhub.example.test", Protocol: ProtocolAnthropic}).Validate(); err != nil {
		t.Fatalf("expected protocol to validate: %v", err)
	}
	if err := (Config{BaseURL: "https://tokenhub.example.test", Protocol: "grpc"}).Validate(); err == nil {
		t.Fatal("expected unsupported protocol to fail")
	}
}

func TestFromSettingsNormalizesNonSecretOptions(t *testing.T) {
	config, err := FromSettings(&dto.TokenHubSettings{
		Enabled:             true,
		BaseURL:             " https://gateway.example.test/ ",
		Protocol:            "ANTHROPIC",
		ModelListPath:       "/catalog/models",
		AllowModelDiscovery: true,
		ModelAliases:        map[string]string{"  fast  ": "model-a"},
	})
	if err != nil {
		t.Fatalf("FromSettings() error = %v", err)
	}
	if config.BaseURL != "https://gateway.example.test" || config.Protocol != ProtocolAnthropic {
		t.Fatalf("unexpected config: %+v", config)
	}
	if config.ModelAliases["fast"] != "model-a" {
		t.Fatalf("expected trimmed model alias, got %#v", config.ModelAliases)
	}
	if config.ResolveModel("fast") != "model-a" || config.ResolveModel("other") != "other" {
		t.Fatalf("unexpected model alias resolution")
	}
}

func TestConfigRejectsUnsafePaths(t *testing.T) {
	config := Config{BaseURL: "https://gateway.example.test", ModelListPath: "/v1/models?token=secret"}
	if err := config.Validate(); err == nil {
		t.Fatal("expected query parameters in model path to be rejected")
	}
}

func TestClientModelsUsesProtocolSpecificAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-token-placeholder" {
			t.Errorf("expected anthropic auth header")
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected bearer header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"model-a","owned_by":"hub"},{"id":""}]}`))
	}))
	defer server.Close()
	models, err := NewClient(
		Config{BaseURL: server.URL, Protocol: ProtocolAnthropic, AllowModelDiscovery: true},
		"test-token-placeholder", nil,
	).Models(context.Background())
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if len(models) != 1 || models[0].ID != "model-a" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestClientHealthDoesNotExposeCredentialInFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	snapshot := NewClient(
		Config{BaseURL: server.URL, Protocol: ProtocolOpenAI},
		"test-token-placeholder", nil,
	).Health(context.Background())
	if snapshot.Healthy || snapshot.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected health snapshot: %+v", snapshot)
	}
	if snapshot.FailureCause != "HTTP 401" {
		t.Fatalf("unexpected failure cause: %q", snapshot.FailureCause)
	}
}
