package ionet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDeploymentRequest() *DeploymentRequest {
	return &DeploymentRequest{
		ResourcePrivateName: "demo",
		DurationHours:       1,
		GPUsPerContainer:    1,
		HardwareID:          1,
		LocationIDs:         []int{1},
		ContainerConfig:     ContainerConfig{ReplicaCount: 1},
		RegistryConfig:      RegistryConfig{ImageURL: "example/image"},
	}
}

func TestDeploymentJSONResponses(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		call           func(*Client) error
		expectedMethod string
		expectedPath   string
		wantErr        bool
	}{
		{"deploy valid", `{"deployment_id":"created"}`, func(c *Client) error { _, err := c.DeployContainer(validDeploymentRequest()); return err }, "POST", "/deploy", false},
		{"deploy malformed", `{"deployment_id":`, func(c *Client) error { _, err := c.DeployContainer(validDeploymentRequest()); return err }, "POST", "/deploy", true},
		{"update valid", `{"deployment_id":"updated"}`, func(c *Client) error {
			_, err := c.UpdateDeployment("dep/a?b#c", &UpdateDeploymentRequest{})
			return err
		}, "PATCH", "/deployment/dep%2Fa%3Fb%23c", false},
		{"update malformed", `{"deployment_id":`, func(c *Client) error { _, err := c.UpdateDeployment("dep", &UpdateDeploymentRequest{}); return err }, "PATCH", "/deployment/dep", true},
		{"delete valid", `{"deployment_id":"deleted"}`, func(c *Client) error { _, err := c.DeleteDeployment("dep/a?b#c"); return err }, "DELETE", "/deployment/dep%2Fa%3Fb%23c", false},
		{"delete malformed", `{"deployment_id":`, func(c *Client) error { _, err := c.DeleteDeployment("dep"); return err }, "DELETE", "/deployment/dep", true},
		{"availability false", `false`, func(c *Client) error { _, err := c.CheckClusterNameAvailability("cluster"); return err }, "GET", "/clusters/check_cluster_name_availability?cluster_name=cluster", false},
		{"availability malformed", `{`, func(c *Client) error { _, err := c.CheckClusterNameAvailability("cluster"); return err }, "GET", "/clusters/check_cluster_name_availability?cluster_name=cluster", true},
		{"cluster update valid", `{"status":"ok"}`, func(c *Client) error {
			_, err := c.UpdateClusterName("cluster/a?b#c", &UpdateClusterNameRequest{Name: "new"})
			return err
		}, "PUT", "/clusters/cluster%2Fa%3Fb%23c/update-name", false},
		{"cluster update malformed", `{`, func(c *Client) error {
			_, err := c.UpdateClusterName("cluster", &UpdateClusterNameRequest{Name: "new"})
			return err
		}, "PUT", "/clusters/cluster/update-name", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 200, Body: []byte(tt.body)}}
			baseURL := "https://ionet.example/api"
			api := NewClientWithConfig("key", baseURL, transport)

			err := tt.call(api)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, transport.requests, 1)
			assert.Equal(t, tt.expectedMethod, transport.requests[0].Method)
			assert.Equal(t, baseURL+tt.expectedPath, transport.requests[0].URL)
		})
	}
}

func TestMutationResponsesRejectEmptyOrMissingDeploymentID(t *testing.T) {
	tests := []struct {
		name string
		body string
		call func(*Client) error
	}{
		{"deploy null", `null`, func(c *Client) error { _, err := c.DeployContainer(validDeploymentRequest()); return err }},
		{"deploy object", `{}`, func(c *Client) error { _, err := c.DeployContainer(validDeploymentRequest()); return err }},
		{"deploy empty", ``, func(c *Client) error { _, err := c.DeployContainer(validDeploymentRequest()); return err }},
		{"update null", `null`, func(c *Client) error { _, err := c.UpdateDeployment("dep", &UpdateDeploymentRequest{}); return err }},
		{"update object", `{}`, func(c *Client) error { _, err := c.UpdateDeployment("dep", &UpdateDeploymentRequest{}); return err }},
		{"update empty", ``, func(c *Client) error { _, err := c.UpdateDeployment("dep", &UpdateDeploymentRequest{}); return err }},
		{"update missing id", `{"status":"ok"}`, func(c *Client) error { _, err := c.UpdateDeployment("dep", &UpdateDeploymentRequest{}); return err }},
		{"delete null", `null`, func(c *Client) error { _, err := c.DeleteDeployment("dep"); return err }},
		{"delete object", `{}`, func(c *Client) error { _, err := c.DeleteDeployment("dep"); return err }},
		{"delete empty", ``, func(c *Client) error { _, err := c.DeleteDeployment("dep"); return err }},
		{"delete missing id", `{"status":"ok"}`, func(c *Client) error { _, err := c.DeleteDeployment("dep"); return err }},
		{"cluster update null", `null`, func(c *Client) error {
			_, err := c.UpdateClusterName("cluster", &UpdateClusterNameRequest{Name: "new"})
			return err
		}},
		{"cluster update object", `{}`, func(c *Client) error {
			_, err := c.UpdateClusterName("cluster", &UpdateClusterNameRequest{Name: "new"})
			return err
		}},
		{"cluster update empty", ``, func(c *Client) error {
			_, err := c.UpdateClusterName("cluster", &UpdateClusterNameRequest{Name: "new"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 200, Body: []byte(tt.body)}}
			api := NewClientWithConfig("key", "https://ionet.example/api", transport)
			require.Error(t, tt.call(api))
			require.Len(t, transport.requests, 1)
		})
	}
}

func TestDeploymentPathsEscapeIdentifiers(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		call           func(*Client) error
		expectedMethod string
		expectedPath   string
	}{
		{"get", `{"data":{}}`, func(c *Client) error { _, err := c.GetDeployment("dep/a?b#c"); return err }, "GET", "/deployment/dep%2Fa%3Fb%23c"},
		{"extend", `{"data":{}}`, func(c *Client) error {
			_, err := c.ExtendDeployment("dep/a?b#c", &ExtendDurationRequest{DurationHours: 1})
			return err
		}, "POST", "/deployment/dep%2Fa%3Fb%23c/extend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 200, Body: []byte(tt.response)}}
			baseURL := "https://ionet.example/api"
			api := NewClientWithConfig("key", baseURL, transport)

			require.NoError(t, tt.call(api))
			require.Len(t, transport.requests, 1)
			assert.Equal(t, tt.expectedMethod, transport.requests[0].Method)
			assert.Equal(t, baseURL+tt.expectedPath, transport.requests[0].URL)
		})
	}
}
