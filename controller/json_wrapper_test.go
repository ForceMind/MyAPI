package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVertexArrayKeysPreservesJSONValues(t *testing.T) {
	keys, err := getVertexArrayKeys(`[
		"  string-key  ",
		"   ",
		{"b":2,"a":1},
		[1,false],
		0,
		false,
		null
	]`)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"string-key",
		`{"a":1,"b":2}`,
		`[1,false]`,
		"0",
		"false",
		"null",
	}, keys)
}

func TestGetVertexArrayKeysRejectsInvalidOrEmptyInput(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		input   string
		wantNil bool
		wantErr string
	}{
		{name: "absent", input: "", wantNil: true},
		{name: "malformed", input: `[`, wantErr: "JsonArray"},
		{name: "object", input: `{}`, wantErr: "JsonArray"},
		{name: "empty array", input: `[]`, wantErr: "keys 不能为空"},
		{name: "null array", input: `null`, wantErr: "keys 不能为空"},
		{name: "blank strings", input: `["  ",""]`, wantErr: "keys 不能为空"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			keys, err := getVertexArrayKeys(testCase.input)
			if testCase.wantErr == "" {
				require.NoError(t, err)
				if testCase.wantNil {
					assert.Nil(t, keys)
				}
				return
			}
			require.ErrorContains(t, err, testCase.wantErr)
			assert.Nil(t, keys)
		})
	}
}

func TestGetAndDecodePreservesHTTPAndDecoderContract(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		status     int
		body       string
		transport  error
		wantValue  int
		wantErr    string
		wantClosed bool
	}{
		{
			name: "unknown fields and trailing value remain compatible", status: http.StatusOK,
			body: `{"value":7,"unknown":true}{"value":99}`, wantValue: 7, wantClosed: true,
		},
		{name: "malformed JSON", status: http.StatusOK, body: `{"value":`, wantErr: "unexpected EOF", wantClosed: true},
		{name: "non-200", status: http.StatusBadGateway, body: `{"value":9}`, wantErr: "non-200 status", wantClosed: true},
		{name: "transport error", transport: errors.New("synthetic transport failure"), wantErr: "synthetic transport failure"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var method string
			var requestURL string
			var body *uptimeJSONBody
			client := &http.Client{Transport: uptimeJSONRoundTripper(func(request *http.Request) (*http.Response, error) {
				method = request.Method
				requestURL = request.URL.String()
				if testCase.transport != nil {
					return nil, testCase.transport
				}
				body = &uptimeJSONBody{Reader: strings.NewReader(testCase.body)}
				return &http.Response{StatusCode: testCase.status, Header: make(http.Header), Body: body, Request: request}, nil
			})}
			var destination struct {
				Value int `json:"value"`
			}

			err := getAndDecode(context.Background(), client, "https://uptime-fixture.invalid/status", &destination)
			assert.Equal(t, http.MethodGet, method)
			assert.Equal(t, "https://uptime-fixture.invalid/status", requestURL)
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, testCase.wantValue, destination.Value)
			if testCase.wantClosed {
				require.NotNil(t, body)
				assert.True(t, body.closed)
			} else {
				assert.Nil(t, body)
			}
		})
	}
}

type uptimeJSONRoundTripper func(*http.Request) (*http.Response, error)

func (transport uptimeJSONRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

type uptimeJSONBody struct {
	io.Reader
	closed bool
}

func (body *uptimeJSONBody) Close() error {
	body.closed = true
	return nil
}
