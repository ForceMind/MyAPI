package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ForceMind/MyAPI/common"
	"github.com/google/uuid"
)

// WHAM usage and reset-credit responses are small JSON documents.  Bound the
// amount read from an upstream (or proxy) response so a malformed/infinite
// body cannot make a quota poller allocate unbounded memory.  Read one extra
// byte to distinguish an accepted response at the limit from an oversized
// response.
const maxCodexWhamResponseBytes = 1 << 20

func readCodexWhamResponse(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCodexWhamResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxCodexWhamResponseBytes {
		return nil, fmt.Errorf("Codex WHAM response exceeds %d bytes", maxCodexWhamResponseBytes)
	}
	return body, nil
}

func FetchCodexWhamUsage(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" {
		return 0, nil, fmt.Errorf("empty baseURL")
	}
	at := strings.TrimSpace(accessToken)
	aid := strings.TrimSpace(accountID)
	if at == "" {
		return 0, nil, fmt.Errorf("empty accessToken")
	}
	if aid == "" {
		return 0, nil, fmt.Errorf("empty accountID")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bu+"/backend-api/wham/usage", nil)
	if err != nil {
		return 0, nil, err
	}
	setCodexWhamRequestHeaders(req, at, aid)

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err = readCodexWhamResponse(resp)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func FetchCodexWhamRateLimitResetCredits(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" {
		return 0, nil, fmt.Errorf("empty baseURL")
	}
	at := strings.TrimSpace(accessToken)
	aid := strings.TrimSpace(accountID)
	if at == "" {
		return 0, nil, fmt.Errorf("empty accessToken")
	}
	if aid == "" {
		return 0, nil, fmt.Errorf("empty accountID")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bu+"/backend-api/wham/rate-limit-reset-credits", nil)
	if err != nil {
		return 0, nil, err
	}
	setCodexWhamRequestHeaders(req, at, aid)

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err = readCodexWhamResponse(resp)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func ConsumeCodexWhamRateLimitResetCredit(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" {
		return 0, nil, fmt.Errorf("empty baseURL")
	}
	at := strings.TrimSpace(accessToken)
	aid := strings.TrimSpace(accountID)
	if at == "" {
		return 0, nil, fmt.Errorf("empty accessToken")
	}
	if aid == "" {
		return 0, nil, fmt.Errorf("empty accountID")
	}

	requestBody, err := common.Marshal(map[string]string{
		"redeem_request_id": uuid.NewString(),
	})
	if err != nil {
		return 0, nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		bu+"/backend-api/wham/rate-limit-reset-credits/consume",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return 0, nil, err
	}
	setCodexWhamRequestHeaders(req, at, aid)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err = readCodexWhamResponse(resp)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func setCodexWhamRequestHeaders(req *http.Request, accessToken string, accountID string) {
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("chatgpt-account-id", accountID)
	req.Header.Set("Accept", "application/json")
	if req.Header.Get("originator") == "" {
		req.Header.Set("originator", "codex_cli_rs")
	}
}
