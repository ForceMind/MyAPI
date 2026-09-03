package vertex

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeAccessTokenResponseRejectsUnsafeFailures(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		response   *http.Response
		wantError  string
		secretText []string
	}{
		{
			name:      "nil response",
			wantError: "invalid access token response",
		},
		{
			name:      "nil body",
			response:  &http.Response{StatusCode: http.StatusOK},
			wantError: "invalid access token response",
		},
		{
			name: "reader error",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(vertexErrorReader{}),
			},
			wantError:  "invalid access token response",
			secretText: []string{"SYNTHETIC_READER_SECRET"},
		},
		{
			name: "non-success HTTP status",
			response: &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body: io.NopCloser(strings.NewReader(`{
					"access_token":"SYNTHETIC_STATUS_TOKEN",
					"error":"SYNTHETIC_STATUS_ERROR",
					"error_description":"RAW_STATUS_DESCRIPTION"
				}`)),
			},
			wantError: "access token endpoint returned HTTP status 401",
			secretText: []string{
				"SYNTHETIC_STATUS_TOKEN",
				"SYNTHETIC_STATUS_ERROR",
				"RAW_STATUS_DESCRIPTION",
			},
		},
		{
			name: "provider error",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"error":"SYNTHETIC_PROVIDER_ERROR",
					"error_description":"RAW_PROVIDER_DESCRIPTION"
				}`)),
			},
			wantError:  "access token provider returned an error",
			secretText: []string{"SYNTHETIC_PROVIDER_ERROR", "RAW_PROVIDER_DESCRIPTION"},
		},
		{
			name: "provider error wins over token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"access_token":"SYNTHETIC_CONFLICT_TOKEN",
					"error":{"message":"SYNTHETIC_CONFLICT_ERROR"}
				}`)),
			},
			wantError:  "access token provider returned an error",
			secretText: []string{"SYNTHETIC_CONFLICT_TOKEN", "SYNTHETIC_CONFLICT_ERROR"},
		},
		{
			name: "null provider error is still an error field",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"SYNTHETIC_NULL_ERROR_TOKEN","error":null}`)),
			},
			wantError:  "access token provider returned an error",
			secretText: []string{"SYNTHETIC_NULL_ERROR_TOKEN"},
		},
		{
			name: "malformed JSON",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"SYNTHETIC_MALFORMED_TOKEN"`)),
			},
			wantError:  "invalid access token response",
			secretText: []string{"SYNTHETIC_MALFORMED_TOKEN"},
		},
		{
			name: "missing token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"unknown":"SYNTHETIC_MISSING_SECRET"}`)),
			},
			wantError:  "access token response missing valid access token",
			secretText: []string{"SYNTHETIC_MISSING_SECRET"},
		},
		{
			name: "null token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":null,"unknown":"SYNTHETIC_NULL_SECRET"}`)),
			},
			wantError:  "access token response missing valid access token",
			secretText: []string{"SYNTHETIC_NULL_SECRET"},
		},
		{
			name: "object token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":{"raw":"SYNTHETIC_WRONG_TYPE_SECRET"}}`)),
			},
			wantError:  "access token response missing valid access token",
			secretText: []string{"SYNTHETIC_WRONG_TYPE_SECRET"},
		},
		{
			name: "number token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":42}`)),
			},
			wantError: "access token response missing valid access token",
		},
		{
			name: "boolean token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":true}`)),
			},
			wantError: "access token response missing valid access token",
		},
		{
			name: "array token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":["SYNTHETIC_ARRAY_TOKEN"]}`)),
			},
			wantError:  "access token response missing valid access token",
			secretText: []string{"SYNTHETIC_ARRAY_TOKEN"},
		},
		{
			name: "empty token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"","unknown":"SYNTHETIC_EMPTY_SECRET"}`)),
			},
			wantError:  "access token response missing valid access token",
			secretText: []string{"SYNTHETIC_EMPTY_SECRET"},
		},
		{
			name: "whitespace token",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"access_token":" \t\n ","unknown":"SYNTHETIC_SPACE_SECRET"}`)),
			},
			wantError:  "access token response missing valid access token",
			secretText: []string{"SYNTHETIC_SPACE_SECRET"},
		},
		{
			name: "top-level wrong type",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`["SYNTHETIC_RAW_ARRAY"]`)),
			},
			wantError:  "invalid access token response",
			secretText: []string{"SYNTHETIC_RAW_ARRAY"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			token, err := decodeAccessTokenResponse(testCase.response)
			assert.Empty(t, token)
			require.EqualError(t, err, testCase.wantError)
			for _, secret := range testCase.secretText {
				assert.NotContains(t, err.Error(), secret)
			}
			assert.NotContains(t, err.Error(), "access_token\"")
			assert.NotContains(t, err.Error(), "error_description")
			assert.NotContains(t, err.Error(), "{")
			assert.NotContains(t, err.Error(), "}")
		})
	}
}

func TestDecodeAccessTokenResponseAcceptsCompatibleSuccessPayloads(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		body      string
		wantToken string
	}{
		{
			name:      "unknown fields",
			body:      `{"access_token":"successful-token","expires_in":3600,"unknown":{"nested":true}}`,
			wantToken: "successful-token",
		},
		{
			name:      "trailing JSON value remains compatible",
			body:      `{"access_token":"first-token"}{"raw":"ignored-second-value"}`,
			wantToken: "first-token",
		},
		{
			name:      "non-whitespace token returned unchanged",
			body:      `{"access_token":"  token-with-original-spacing  "}`,
			wantToken: "  token-with-original-spacing  ",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := &vertexTestReadCloser{Reader: strings.NewReader(testCase.body)}
			response := &http.Response{StatusCode: http.StatusOK, Body: body}

			token, err := decodeAccessTokenResponse(response)
			require.NoError(t, err)
			assert.Equal(t, testCase.wantToken, token)
			assert.False(t, body.closed, "the caller owns response body closure")
			require.NoError(t, body.Close())
			assert.True(t, body.closed)
		})
	}
}

type vertexErrorReader struct{}

func (vertexErrorReader) Read([]byte) (int, error) {
	return 0, errors.New("SYNTHETIC_READER_SECRET")
}

type vertexTestReadCloser struct {
	io.Reader
	closed bool
}

func (r *vertexTestReadCloser) Close() error {
	r.closed = true
	return nil
}
