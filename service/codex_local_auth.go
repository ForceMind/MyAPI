package service

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ForceMind/MyAPI/common"
)

const codexLocalAuthMaxBytes int64 = 256 << 10

const codexRuntimeEnvironmentVariable = "MYAPI_RUNTIME_ENV"

type CodexLocalAuthState string

const (
	CodexLocalAuthReady                 CodexLocalAuthState = "ready"
	CodexLocalAuthContainerUnavailable  CodexLocalAuthState = "container_host_unavailable"
	CodexLocalAuthHomeUnavailable       CodexLocalAuthState = "home_unavailable"
	CodexLocalAuthNotFound              CodexLocalAuthState = "not_found"
	CodexLocalAuthUnreadable            CodexLocalAuthState = "unreadable"
	CodexLocalAuthUnsafeFile            CodexLocalAuthState = "unsafe_file"
	CodexLocalAuthTooLarge              CodexLocalAuthState = "too_large"
	CodexLocalAuthChangedDuringRead     CodexLocalAuthState = "changed_during_read"
	CodexLocalAuthInvalidJSON           CodexLocalAuthState = "invalid_json"
	CodexLocalAuthUnsupportedAuthMethod CodexLocalAuthState = "unsupported_auth_method"
	CodexLocalAuthIncompleteCredential  CodexLocalAuthState = "incomplete_credential"
)

const (
	CodexLocalEnvironmentNative    = "native"
	CodexLocalEnvironmentContainer = "container"
)

// CodexLocalAuthInspection is safe to return from an API. It deliberately
// contains neither the credential nor the resolved auth.json path.
type CodexLocalAuthInspection struct {
	State                 CodexLocalAuthState `json:"state"`
	Platform              string              `json:"platform"`
	Environment           string              `json:"environment"`
	CodexInstalled        bool                `json:"codex_installed"`
	CLIVersion            string              `json:"cli_version,omitempty"`
	AuthFileExists        bool                `json:"auth_file_exists"`
	AuthReadable          bool                `json:"auth_readable"`
	LoggedIn              bool                `json:"logged_in"`
	AutoImportAvailable   bool                `json:"auto_import_available"`
	ManualImportAvailable bool                `json:"manual_import_available"`
	AccountHint           string              `json:"account_hint,omitempty"`
	EmailHint             string              `json:"email_hint,omitempty"`
	LastRefresh           string              `json:"last_refresh,omitempty"`
	CanRefresh            bool                `json:"can_refresh"`
}

// codexLocalAuthInspector allows the operating-system boundary to be injected
// in tests. It is deliberately package-private so production callers cannot
// supply a filesystem path: the inspected location always comes from this
// process's CODEX_HOME or operating-system home directory.
type codexLocalAuthInspector struct {
	Platform    string
	Container   bool
	Getenv      func(string) string
	UserHomeDir func() (string, error)
	LookPath    func(string) (string, error)
	Lstat       func(string) (os.FileInfo, error)
	Open        func(string) (*os.File, error)
}

func newCodexLocalAuthInspector(inContainer bool) codexLocalAuthInspector {
	return codexLocalAuthInspector{
		Platform:    runtime.GOOS,
		Container:   inContainer || strings.EqualFold(strings.TrimSpace(os.Getenv(codexRuntimeEnvironmentVariable)), CodexLocalEnvironmentContainer),
		Getenv:      os.Getenv,
		UserHomeDir: os.UserHomeDir,
		LookPath:    exec.LookPath,
		Lstat:       os.Lstat,
		Open:        os.Open,
	}
}

// InspectCodexLocalAuth reads the current process user's Codex auth file. The
// returned OAuthKey is for trusted server-side use only and must not be exposed
// in the status response.
func InspectCodexLocalAuth(inContainer bool) (CodexLocalAuthInspection, *CodexOAuthKey) {
	return newCodexLocalAuthInspector(inContainer).Inspect()
}

func (inspector codexLocalAuthInspector) Inspect() (CodexLocalAuthInspection, *CodexOAuthKey) {
	result := CodexLocalAuthInspection{
		Platform:              inspector.Platform,
		Environment:           CodexLocalEnvironmentNative,
		ManualImportAvailable: true,
	}
	if inspector.Container {
		result.State = CodexLocalAuthContainerUnavailable
		result.Environment = CodexLocalEnvironmentContainer
		return result, nil
	}

	if inspector.LookPath != nil {
		_, err := inspector.LookPath("codex")
		result.CodexInstalled = err == nil
	}

	if inspector.Getenv == nil || inspector.UserHomeDir == nil || inspector.Lstat == nil || inspector.Open == nil {
		result.State = CodexLocalAuthHomeUnavailable
		return result, nil
	}

	codexHome := inspector.Getenv("CODEX_HOME")
	if codexHome != "" {
		if !filepath.IsAbs(codexHome) {
			result.State = CodexLocalAuthHomeUnavailable
			return result, nil
		}
	} else {
		var err error
		codexHome, err = inspector.UserHomeDir()
		if err != nil || codexHome == "" || !filepath.IsAbs(codexHome) {
			result.State = CodexLocalAuthHomeUnavailable
			return result, nil
		}
		codexHome = filepath.Join(codexHome, ".codex")
	}

	authPath := filepath.Join(codexHome, "auth.json")
	before, err := inspector.Lstat(authPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.State = CodexLocalAuthNotFound
		} else {
			result.State = CodexLocalAuthUnreadable
		}
		return result, nil
	}
	result.AuthFileExists = true
	if !before.Mode().IsRegular() {
		result.State = CodexLocalAuthUnsafeFile
		return result, nil
	}
	if before.Size() < 0 {
		result.State = CodexLocalAuthUnsafeFile
		return result, nil
	}
	if before.Size() > codexLocalAuthMaxBytes {
		result.State = CodexLocalAuthTooLarge
		return result, nil
	}

	file, err := inspector.Open(authPath)
	if err != nil {
		result.State = CodexLocalAuthUnreadable
		return result, nil
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		result.State = CodexLocalAuthUnreadable
		return result, nil
	}
	if !opened.Mode().IsRegular() {
		result.State = CodexLocalAuthUnsafeFile
		return result, nil
	}
	if opened.Size() > codexLocalAuthMaxBytes {
		result.State = CodexLocalAuthTooLarge
		return result, nil
	}
	if opened.Size() < 0 || !os.SameFile(before, opened) || opened.Size() != before.Size() || !opened.ModTime().Equal(before.ModTime()) {
		result.State = CodexLocalAuthChangedDuringRead
		return result, nil
	}

	data, err := io.ReadAll(io.LimitReader(file, codexLocalAuthMaxBytes+1))
	if err != nil {
		result.State = CodexLocalAuthUnreadable
		return result, nil
	}
	if int64(len(data)) > codexLocalAuthMaxBytes {
		result.State = CodexLocalAuthTooLarge
		return result, nil
	}

	afterOpen, err := file.Stat()
	if err != nil {
		result.State = CodexLocalAuthUnreadable
		return result, nil
	}
	afterPath, err := inspector.Lstat(authPath)
	if err != nil || !afterPath.Mode().IsRegular() || !os.SameFile(opened, afterOpen) || !os.SameFile(opened, afterPath) ||
		afterOpen.Size() != opened.Size() || !afterOpen.ModTime().Equal(opened.ModTime()) || int64(len(data)) != opened.Size() {
		result.State = CodexLocalAuthChangedDuringRead
		return result, nil
	}
	result.AuthReadable = true

	var authFile struct {
		APIKey      string `json:"OPENAI_API_KEY"`
		AuthMode    string `json:"auth_mode"`
		LastRefresh string `json:"last_refresh"`
		Tokens      struct {
			IDToken      string `json:"id_token"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			AccountID    string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := common.Unmarshal(data, &authFile); err != nil {
		result.State = CodexLocalAuthInvalidJSON
		return result, nil
	}

	authMode := strings.ToLower(strings.TrimSpace(authFile.AuthMode))
	if (authMode != "" && authMode != "chatgpt" && authMode != "oauth") ||
		(strings.TrimSpace(authFile.APIKey) != "" && strings.TrimSpace(authFile.Tokens.AccessToken) == "") {
		result.State = CodexLocalAuthUnsupportedAuthMethod
		return result, nil
	}

	key := &CodexOAuthKey{
		IDToken:      strings.TrimSpace(authFile.Tokens.IDToken),
		AccessToken:  strings.TrimSpace(authFile.Tokens.AccessToken),
		RefreshToken: strings.TrimSpace(authFile.Tokens.RefreshToken),
		AccountID:    strings.TrimSpace(authFile.Tokens.AccountID),
		LastRefresh:  strings.TrimSpace(authFile.LastRefresh),
		Type:         "codex",
	}
	if key.AccountID == "" {
		if accountID, ok := ExtractCodexAccountIDFromJWT(key.AccessToken); ok {
			key.AccountID = accountID
		} else if accountID, ok := ExtractCodexAccountIDFromJWT(key.IDToken); ok {
			key.AccountID = accountID
		}
	}
	if email, ok := ExtractEmailFromJWT(key.IDToken); ok {
		key.Email = email
	} else if email, ok := ExtractEmailFromJWT(key.AccessToken); ok {
		key.Email = email
	}

	result.AccountHint = maskCodexAccountHint(key.AccountID)
	result.EmailHint = maskCodexEmailHint(key.Email)
	if lastRefresh, err := time.Parse(time.RFC3339Nano, key.LastRefresh); err == nil {
		result.LastRefresh = lastRefresh.Format(time.RFC3339Nano)
	}
	result.CanRefresh = key.RefreshToken != ""
	if key.AccessToken == "" || key.RefreshToken == "" || key.AccountID == "" {
		result.State = CodexLocalAuthIncompleteCredential
		return result, nil
	}

	result.State = CodexLocalAuthReady
	result.LoggedIn = true
	result.AutoImportAvailable = true
	return result, key
}

func maskCodexAccountHint(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) == 0 {
		return ""
	}
	if len(runes) <= 8 {
		return string(runes[0]) + "…" + string(runes[len(runes)-1])
	}
	return string(runes[:4]) + "…" + string(runes[len(runes)-4:])
}

func maskCodexEmailHint(value string) string {
	value = strings.TrimSpace(value)
	at := strings.LastIndex(value, "@")
	if at <= 0 || at == len(value)-1 {
		return ""
	}
	local := []rune(value[:at])
	return string(local[0]) + "***@" + value[at+1:]
}
