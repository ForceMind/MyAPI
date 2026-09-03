package ionet

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHardwareJSONResponses(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		call         func(*Client) error
		expectedPath string
		wantErr      bool
	}{
		{"hardware valid", `{"id":1,"name":"A100"}`, func(c *Client) error { _, err := c.GetHardwareType(1); return err }, "/hardware/types/1", false},
		{"hardware malformed", `{`, func(c *Client) error { _, err := c.GetHardwareType(1); return err }, "/hardware/types/1", true},
		{"location valid", `{"id":2,"name":"Shanghai"}`, func(c *Client) error { _, err := c.GetLocation(2); return err }, "/locations/2", false},
		{"location malformed", `{`, func(c *Client) error { _, err := c.GetLocation(2); return err }, "/locations/2", true},
		{"availability false", `{"location_id":2,"available":false}`, func(c *Client) error { _, err := c.GetLocationAvailability(2); return err }, "/locations/2/availability", false},
		{"availability malformed", `{`, func(c *Client) error { _, err := c.GetLocationAvailability(2); return err }, "/locations/2/availability", true},
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
			assert.Equal(t, "GET", transport.requests[0].Method)
			assert.Equal(t, baseURL+tt.expectedPath, transport.requests[0].URL)
		})
	}
}

func TestHardwareLocationAndAvailabilityRejectEmptyResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		call func(*Client) error
	}{
		{"hardware null", `null`, func(c *Client) error { _, err := c.GetHardwareType(1); return err }},
		{"hardware object", `{}`, func(c *Client) error { _, err := c.GetHardwareType(1); return err }},
		{"hardware empty", ``, func(c *Client) error { _, err := c.GetHardwareType(1); return err }},
		{"hardware missing id", `{"name":"A100"}`, func(c *Client) error { _, err := c.GetHardwareType(1); return err }},
		{"location null", `null`, func(c *Client) error { _, err := c.GetLocation(1); return err }},
		{"location object", `{}`, func(c *Client) error { _, err := c.GetLocation(1); return err }},
		{"location empty", ``, func(c *Client) error { _, err := c.GetLocation(1); return err }},
		{"location missing id", `{"name":"Shanghai"}`, func(c *Client) error { _, err := c.GetLocation(1); return err }},
		{"availability null", `null`, func(c *Client) error { _, err := c.GetLocationAvailability(1); return err }},
		{"availability object", `{}`, func(c *Client) error { _, err := c.GetLocationAvailability(1); return err }},
		{"availability empty", ``, func(c *Client) error { _, err := c.GetLocationAvailability(1); return err }},
		{"availability missing id", `{"available":true}`, func(c *Client) error { _, err := c.GetLocationAvailability(1); return err }},
		{"availability null bool", `{"location_id":1,"available":null}`, func(c *Client) error { _, err := c.GetLocationAvailability(1); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &fakeHTTPClient{response: &HTTPResponse{StatusCode: 200, Body: []byte(tt.body)}}
			api := NewClientWithConfig("key", "https://ionet.example/api", transport)
			require.Error(t, tt.call(api))
		})
	}
}
