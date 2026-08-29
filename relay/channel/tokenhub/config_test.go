package tokenhub

import "testing"

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
