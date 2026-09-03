package ionet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerEndpointsEscapeIdentifiers(t *testing.T) {
	const deploymentID = "dep/a?b#c"
	const containerID = "ctr/a?b#c"
	const baseURL = "https://ionet.example/api"

	tests := []struct {
		name           string
		response       *HTTPResponse
		call           func(*Client) error
		expectedMethod string
		expectedPath   string
	}{
		{"list", &HTTPResponse{StatusCode: 200, Body: []byte(`{"data":{"containers":[]}}`)}, func(c *Client) error { _, err := c.ListContainers(deploymentID); return err }, "GET", "/deployment/dep%2Fa%3Fb%23c/containers"},
		{"details", &HTTPResponse{StatusCode: 200, Body: []byte(`{"id":"container"}`)}, func(c *Client) error { _, err := c.GetContainerDetails(deploymentID, containerID); return err }, "GET", "/deployment/dep%2Fa%3Fb%23c/container/ctr%2Fa%3Fb%23c"},
		{"jobs", &HTTPResponse{StatusCode: 200, Body: []byte(`{"data":{"containers":[]}}`)}, func(c *Client) error { _, err := c.GetContainerJobs(deploymentID, containerID); return err }, "GET", "/deployment/dep%2Fa%3Fb%23c/containers-jobs/ctr%2Fa%3Fb%23c"},
		{"raw logs", &HTTPResponse{StatusCode: 200, Body: []byte{}}, func(c *Client) error { _, err := c.GetContainerLogsRaw(deploymentID, containerID, nil); return err }, "GET", "/deployment/dep%2Fa%3Fb%23c/log/ctr%2Fa%3Fb%23c"},
		{"restart", &HTTPResponse{StatusCode: 204}, func(c *Client) error { return c.RestartContainer(deploymentID, containerID) }, "POST", "/deployment/dep%2Fa%3Fb%23c/container/ctr%2Fa%3Fb%23c/restart"},
		{"stop", &HTTPResponse{StatusCode: 204}, func(c *Client) error { return c.StopContainer(deploymentID, containerID) }, "POST", "/deployment/dep%2Fa%3Fb%23c/container/ctr%2Fa%3Fb%23c/stop"},
		{"execute", &HTTPResponse{StatusCode: 200, Body: []byte(`{"output":"ok"}`)}, func(c *Client) error {
			_, err := c.ExecuteInContainer(deploymentID, containerID, []string{"echo", "ok"})
			return err
		}, "POST", "/deployment/dep%2Fa%3Fb%23c/container/ctr%2Fa%3Fb%23c/exec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeHTTPClient{response: tt.response}
			api := NewClientWithConfig("key", baseURL, transport)

			require.NoError(t, tt.call(api))
			require.Len(t, transport.requests, 1)
			assert.Equal(t, tt.expectedMethod, transport.requests[0].Method)
			assert.Equal(t, baseURL+tt.expectedPath, transport.requests[0].URL)
		})
	}
}

func TestExecuteInContainerJSONResponses(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		output  string
		wantErr bool
	}{
		{"output", `{"output":"done"}`, "done", false},
		{"raw fallback without output", `{"status":"done"}`, `{"status":"done"}`, false},
		{"malformed", `{"output":`, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 200, Body: []byte(tt.body)}}
			api := NewClientWithConfig("key", "https://ionet.example/api", transport)

			output, err := api.ExecuteInContainer("dep", "container", []string{"echo", "ok"})
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorContains(t, err, "failed to parse execution result")
				require.Len(t, transport.requests, 1)
				assert.Equal(t, "POST", transport.requests[0].Method)
				assert.Equal(t, "https://ionet.example/api/deployment/dep/container/container/exec", transport.requests[0].URL)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.output, output)
		})
	}
}

func TestStreamContainerLogsDoesNotMutateOptions(t *testing.T) {
	opts := &GetLogsOptions{Cursor: "original", Follow: false, Limit: 10}
	transport := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 200, Body: []byte(`{"logs":[]}`)}}
	api := NewClientWithConfig("key", "https://ionet.example/api", transport)

	require.NoError(t, api.StreamContainerLogs("dep/a?b#c", "ctr/a?b#c", opts, func(*LogEntry) error { return nil }))
	require.Len(t, transport.requests, 1)
	assert.Equal(t, "https://ionet.example/api/deployment/dep%2Fa%3Fb%23c/log/ctr%2Fa%3Fb%23c?cursor=original&follow=true&limit=10", transport.requests[0].URL)
	assert.Equal(t, "original", opts.Cursor)
	assert.False(t, opts.Follow)
}
