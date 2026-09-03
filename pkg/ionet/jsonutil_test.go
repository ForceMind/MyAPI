package ionet

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type flexibleTimePayload struct {
	CreatedAt time.Time `json:"created_at"`
	Message   string    `json:"message"`
	Nested    struct {
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"nested"`
}

func TestDecodeWithFlexibleTimes(t *testing.T) {
	t.Parallel()

	utcTime := time.Date(2026, 9, 4, 12, 34, 56, 123456000, time.UTC)
	offsetTime := time.Date(2026, 9, 4, 12, 34, 56, 0, time.FixedZone("+08", 8*60*60))

	tests := []struct {
		name            string
		decode          func([]byte, *flexibleTimePayload) error
		body            string
		createdAt       time.Time
		createdAtOffset int
		updatedAt       time.Time
		updatedAtOffset int
		message         string
	}{
		{
			name: "direct payload normalizes timezone-free times recursively and ignores unknown fields",
			decode: func(data []byte, target *flexibleTimePayload) error {
				return decodeWithFlexibleTimes(data, target)
			},
			body:            `{"created_at":"2026-09-04T12:34:56.123456","message":"plain text","nested":{"updated_at":"2026-09-04T12:34:56"},"unknown":true}`,
			createdAt:       utcTime,
			createdAtOffset: 0,
			updatedAt:       time.Date(2026, 9, 4, 12, 34, 56, 0, time.UTC),
			updatedAtOffset: 0,
			message:         "plain text",
		},
		{
			name: "wrapped payload preserves timezone-bearing times",
			decode: func(data []byte, target *flexibleTimePayload) error {
				return decodeDataWithFlexibleTimes(data, target)
			},
			body:            `{"data":{"created_at":"2026-09-04T12:34:56+08:00","message":"not a time","nested":{"updated_at":"2026-09-04T12:34:56Z"}},"unknown":"ignored"}`,
			createdAt:       offsetTime,
			createdAtOffset: 8 * 60 * 60,
			updatedAt:       time.Date(2026, 9, 4, 12, 34, 56, 0, time.UTC),
			updatedAtOffset: 0,
			message:         "not a time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var actual flexibleTimePayload
			require.NoError(t, tt.decode([]byte(tt.body), &actual))
			require.True(t, tt.createdAt.Equal(actual.CreatedAt))
			_, createdAtOffset := actual.CreatedAt.Zone()
			require.Equal(t, tt.createdAtOffset, createdAtOffset)
			require.True(t, tt.updatedAt.Equal(actual.Nested.UpdatedAt))
			_, updatedAtOffset := actual.Nested.UpdatedAt.Zone()
			require.Equal(t, tt.updatedAtOffset, updatedAtOffset)
			require.Equal(t, tt.message, actual.Message)
		})
	}
}

func TestDecodeFlexibleTimesRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		decode func([]byte, *flexibleTimePayload) error
		body   string
	}{
		{
			name: "malformed JSON",
			decode: func(data []byte, target *flexibleTimePayload) error {
				return decodeWithFlexibleTimes(data, target)
			},
			body: `{"created_at":`,
		},
		{
			name: "invalid field type",
			decode: func(data []byte, target *flexibleTimePayload) error {
				return decodeWithFlexibleTimes(data, target)
			},
			body: `{"created_at":123,"nested":{"updated_at":"2026-09-04T12:34:56Z"}}`,
		},
		{
			name: "trailing second JSON value",
			decode: func(data []byte, target *flexibleTimePayload) error {
				return decodeDataWithFlexibleTimes(data, target)
			},
			body: `{"data":{"created_at":"2026-09-04T12:34:56Z","nested":{"updated_at":"2026-09-04T12:34:56Z"}}} {}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var actual flexibleTimePayload
			require.Error(t, tt.decode([]byte(tt.body), &actual))
		})
	}
}

func TestDecodeDataRejectsTrailingJSONValue(t *testing.T) {
	t.Parallel()

	var actual flexibleTimePayload
	err := decodeData([]byte(`{"data":{"message":"plain text"}} {}`), &actual)
	require.Error(t, err)
}

func TestDecodeWithFlexibleTimesNormalizesArrayElements(t *testing.T) {
	t.Parallel()

	type event struct {
		CreatedAt time.Time `json:"created_at"`
	}
	var events []event
	require.NoError(t, decodeDataWithFlexibleTimes(
		[]byte(`{"data":[{"created_at":"2026-09-04T12:34:56"},{"created_at":"2026-09-04T12:34:56+08:00"}]}`),
		&events,
	))
	require.Len(t, events, 2)
	require.True(t, events[0].CreatedAt.Equal(time.Date(2026, 9, 4, 12, 34, 56, 0, time.UTC)))
	_, firstOffset := events[0].CreatedAt.Zone()
	require.Zero(t, firstOffset)
	require.True(t, events[1].CreatedAt.Equal(time.Date(2026, 9, 4, 4, 34, 56, 0, time.UTC)))
	_, secondOffset := events[1].CreatedAt.Zone()
	require.Equal(t, 8*60*60, secondOffset)
}
