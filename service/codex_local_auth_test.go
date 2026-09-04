package service

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexLocalAuthInspectorReadyUsesAbsoluteCodexHome(t *testing.T) {
	codexHome := t.TempDir()
	accessToken := testCodexJWT(t, map[string]any{
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": "acct-from-jwt-123456"},
	})
	idToken := testCodexJWT(t, map[string]any{"email": "person@example.com"})
	writeCodexAuthFixture(t, codexHome, map[string]any{
		"tokens": map[string]any{
			"id_token":      idToken,
			"access_token":  accessToken,
			"refresh_token": "refresh-secret",
		},
		"last_refresh": "2026-09-04T10:00:00Z",
	})
	authPath := filepath.Join(codexHome, "auth.json")
	originalAuth, err := os.ReadFile(authPath)
	require.NoError(t, err)
	originalInfo, err := os.Stat(authPath)
	require.NoError(t, err)

	inspector := testCodexInspector(codexHome)
	inspector.LookPath = func(name string) (string, error) {
		require.Equal(t, "codex", name)
		return "/fixture/bin/codex", nil
	}
	inspector.Lstat = func(path string) (os.FileInfo, error) {
		require.Equal(t, authPath, path)
		return os.Lstat(path)
	}
	inspector.Open = func(path string) (*os.File, error) {
		require.Equal(t, authPath, path)
		return os.Open(path)
	}
	result, key := inspector.Inspect()

	require.NotNil(t, key)
	assert.Equal(t, CodexLocalAuthReady, result.State)
	assert.Equal(t, "test-platform", result.Platform)
	assert.Equal(t, CodexLocalEnvironmentNative, result.Environment)
	assert.True(t, result.CodexInstalled)
	assert.True(t, result.AuthFileExists)
	assert.True(t, result.AuthReadable)
	assert.True(t, result.LoggedIn)
	assert.True(t, result.AutoImportAvailable)
	assert.True(t, result.ManualImportAvailable)
	assert.True(t, result.CanRefresh)
	assert.Equal(t, "acct…3456", result.AccountHint)
	assert.Equal(t, "p***@example.com", result.EmailHint)
	assert.Equal(t, "2026-09-04T10:00:00Z", result.LastRefresh)
	assert.Equal(t, "acct-from-jwt-123456", key.AccountID)
	assert.Equal(t, "person@example.com", key.Email)
	assert.Equal(t, "codex", key.Type)

	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "refresh-secret")
	assert.NotContains(t, string(encoded), codexHome)
	afterAuth, err := os.ReadFile(authPath)
	require.NoError(t, err)
	afterInfo, err := os.Stat(authPath)
	require.NoError(t, err)
	assert.Equal(t, originalAuth, afterAuth)
	assert.Equal(t, originalInfo.Mode(), afterInfo.Mode())
	assert.Equal(t, originalInfo.ModTime(), afterInfo.ModTime())
}

func TestCodexLocalAuthInspectorUsesUserHomeFallback(t *testing.T) {
	userHome := t.TempDir()
	codexHome := filepath.Join(userHome, ".codex")
	require.NoError(t, os.Mkdir(codexHome, 0o700))
	writeCodexAuthFixture(t, codexHome, map[string]any{
		"tokens": map[string]any{
			"access_token":  "access",
			"refresh_token": "refresh",
			"account_id":    "account-123456",
		},
	})
	inspector := testCodexInspector("")
	inspector.UserHomeDir = func() (string, error) { return userHome, nil }

	result, key := inspector.Inspect()

	assert.Equal(t, CodexLocalAuthReady, result.State)
	require.NotNil(t, key)
	assert.Equal(t, "account-123456", key.AccountID)
}

func TestCodexLocalAuthInspectorContainerShortCircuitsHostAccess(t *testing.T) {
	called := false
	inspector := codexLocalAuthInspector{
		Platform:  "linux",
		Container: true,
		Getenv: func(string) string {
			called = true
			return ""
		},
		UserHomeDir: func() (string, error) {
			called = true
			return "", errors.New("must not be called")
		},
		LookPath: func(string) (string, error) {
			called = true
			return "", errors.New("must not be called")
		},
		Lstat: func(string) (os.FileInfo, error) {
			called = true
			return nil, errors.New("must not be called")
		},
		Open: func(string) (*os.File, error) {
			called = true
			return nil, errors.New("must not be called")
		},
	}

	result, key := inspector.Inspect()

	assert.Equal(t, CodexLocalAuthContainerUnavailable, result.State)
	assert.Equal(t, CodexLocalEnvironmentContainer, result.Environment)
	assert.True(t, result.ManualImportAvailable)
	assert.False(t, called)
	assert.Nil(t, key)
}

func TestCodexLocalAuthExplicitContainerMarkerOverridesHeuristics(t *testing.T) {
	t.Setenv(codexRuntimeEnvironmentVariable, CodexLocalEnvironmentContainer)
	inspector := newCodexLocalAuthInspector(false)
	called := false
	inspector.Getenv = func(string) string {
		called = true
		return t.TempDir()
	}
	inspector.Lstat = func(string) (os.FileInfo, error) {
		called = true
		return nil, errors.New("must not inspect host credentials")
	}

	result, key := inspector.Inspect()

	assert.Equal(t, CodexLocalAuthContainerUnavailable, result.State)
	assert.Equal(t, CodexLocalEnvironmentContainer, result.Environment)
	assert.False(t, result.AutoImportAvailable)
	assert.False(t, called)
	assert.Nil(t, key)
}

func TestCodexLocalAuthInspectorRejectsUnavailableHome(t *testing.T) {
	tests := []struct {
		name         string
		codexHome    string
		home         string
		homeErr      error
		wantHomeCall bool
	}{
		{name: "relative CODEX_HOME", codexHome: "relative/path"},
		{name: "home lookup error", homeErr: errors.New("no home"), wantHomeCall: true},
		{name: "relative user home", home: "relative/home", wantHomeCall: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			homeCalled := false
			inspector := testCodexInspector(test.codexHome)
			inspector.UserHomeDir = func() (string, error) {
				homeCalled = true
				return test.home, test.homeErr
			}

			result, key := inspector.Inspect()

			assert.Equal(t, CodexLocalAuthHomeUnavailable, result.State)
			assert.Equal(t, test.wantHomeCall, homeCalled)
			assert.Nil(t, key)
		})
	}
}

func TestCodexLocalAuthInspectorFileStates(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		result, key := testCodexInspector(t.TempDir()).Inspect()
		assert.Equal(t, CodexLocalAuthNotFound, result.State)
		assert.False(t, result.AuthFileExists)
		assert.Nil(t, key)
	})

	t.Run("unreadable", func(t *testing.T) {
		codexHome := t.TempDir()
		writeCodexAuthFixture(t, codexHome, map[string]any{})
		inspector := testCodexInspector(codexHome)
		inspector.Open = func(string) (*os.File, error) { return nil, os.ErrPermission }
		result, key := inspector.Inspect()
		assert.Equal(t, CodexLocalAuthUnreadable, result.State)
		assert.True(t, result.AuthFileExists)
		assert.False(t, result.AuthReadable)
		assert.Nil(t, key)
	})

	t.Run("symlink is unsafe", func(t *testing.T) {
		codexHome := t.TempDir()
		target := filepath.Join(t.TempDir(), "target.json")
		require.NoError(t, os.WriteFile(target, []byte("{}"), 0o600))
		require.NoError(t, os.Symlink(target, filepath.Join(codexHome, "auth.json")))
		result, key := testCodexInspector(codexHome).Inspect()
		assert.Equal(t, CodexLocalAuthUnsafeFile, result.State)
		assert.Nil(t, key)
	})

	t.Run("non regular is unsafe", func(t *testing.T) {
		codexHome := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(codexHome, "auth.json"), 0o700))
		result, key := testCodexInspector(codexHome).Inspect()
		assert.Equal(t, CodexLocalAuthUnsafeFile, result.State)
		assert.Nil(t, key)
	})

	t.Run("too large", func(t *testing.T) {
		codexHome := t.TempDir()
		data := make([]byte, codexLocalAuthMaxBytes+1)
		require.NoError(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), data, 0o600))
		result, key := testCodexInspector(codexHome).Inspect()
		assert.Equal(t, CodexLocalAuthTooLarge, result.State)
		assert.Nil(t, key)
	})

	t.Run("opened file differs from lstat", func(t *testing.T) {
		codexHome := t.TempDir()
		writeCodexAuthFixture(t, codexHome, map[string]any{})
		otherPath := filepath.Join(t.TempDir(), "other.json")
		require.NoError(t, os.WriteFile(otherPath, []byte("{}"), 0o600))
		inspector := testCodexInspector(codexHome)
		inspector.Open = func(string) (*os.File, error) { return os.Open(otherPath) }
		result, key := inspector.Inspect()
		assert.Equal(t, CodexLocalAuthChangedDuringRead, result.State)
		assert.Nil(t, key)
	})
}

func TestCodexLocalAuthInspectorContentStates(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want CodexLocalAuthState
	}{
		{name: "invalid json", data: []byte("{"), want: CodexLocalAuthInvalidJSON},
		{name: "API key auth", data: []byte(`{"OPENAI_API_KEY":"fixture-api-key"}`), want: CodexLocalAuthUnsupportedAuthMethod},
		{name: "explicit unsupported mode", data: []byte(`{"auth_mode":"api_key","tokens":{"access_token":"a","refresh_token":"r","account_id":"id"}}`), want: CodexLocalAuthUnsupportedAuthMethod},
		{name: "missing refresh", data: []byte(`{"tokens":{"access_token":"a","account_id":"id"}}`), want: CodexLocalAuthIncompleteCredential},
		{name: "missing account", data: []byte(`{"tokens":{"access_token":"not-a-jwt","refresh_token":"r"}}`), want: CodexLocalAuthIncompleteCredential},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			codexHome := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), test.data, 0o600))

			result, key := testCodexInspector(codexHome).Inspect()

			assert.Equal(t, test.want, result.State)
			assert.Nil(t, key)
			assert.True(t, result.AuthReadable)
		})
	}
}

func testCodexInspector(codexHome string) codexLocalAuthInspector {
	return codexLocalAuthInspector{
		Platform: "test-platform",
		Getenv: func(name string) string {
			if name == "CODEX_HOME" {
				return codexHome
			}
			return ""
		},
		UserHomeDir: func() (string, error) { return "", errors.New("unexpected home lookup") },
		LookPath:    func(string) (string, error) { return "", errors.New("not installed") },
		Lstat:       os.Lstat,
		Open:        os.Open,
	}
}

func writeCodexAuthFixture(t *testing.T, codexHome string, value any) {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), data, 0o600))
}

func testCodexJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := common.Marshal(claims)
	require.NoError(t, err)
	return "fixture." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}
