package cachex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type codecNestedValue struct {
	Count int  `json:"count"`
	Ready bool `json:"ready"`
}

type codecValue struct {
	Name   string             `json:"name"`
	Nested codecNestedValue   `json:"nested"`
	Items  []codecNestedValue `json:"items"`
}

type mutatingJSONValue string

func (v *mutatingJSONValue) UnmarshalJSON(data []byte) error {
	data[0] = 'x'
	*v = "decoded"
	return nil
}

func TestJSONCodecRoundTrip(t *testing.T) {
	codec := JSONCodec[codecValue]{}
	want := codecValue{
		Name: "example",
		Nested: codecNestedValue{
			Count: 0,
			Ready: false,
		},
		Items: []codecNestedValue{{Count: 3, Ready: true}},
	}

	encoded, err := codec.Encode(want)
	require.NoError(t, err)

	got, err := codec.Decode(encoded)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestJSONCodecDecodeRejectsEmptyAndWhitespace(t *testing.T) {
	codec := JSONCodec[codecValue]{}

	for _, input := range []string{"", " \t\n "} {
		got, err := codec.Decode(input)
		require.Error(t, err)
		assert.Equal(t, codecValue{}, got)
	}
}

func TestJSONCodecDecodeRejectsInvalidValues(t *testing.T) {
	codec := JSONCodec[codecValue]{}

	for _, input := range []string{
		`{"name":`,
		`[]`,
		`{"name":"first"} {"name":"second"}`,
	} {
		got, err := codec.Decode(input)
		require.Error(t, err)
		assert.Equal(t, codecValue{}, got)
	}
}

func TestJSONCodecDecodeAcceptsTrailingWhitespace(t *testing.T) {
	codec := JSONCodec[codecValue]{}

	got, err := codec.Decode("{\"name\":\"example\"}\n\t ")
	require.NoError(t, err)
	assert.Equal(t, codecValue{Name: "example"}, got)
}

func TestJSONCodecEncodeReturnsEmptyStringForUnencodableValue(t *testing.T) {
	codec := JSONCodec[func()]{}

	encoded, err := codec.Encode(func() {})
	require.Error(t, err)
	assert.Empty(t, encoded)
}

func TestJSONCodecDecodeDoesNotAliasInputStringForCustomUnmarshaler(t *testing.T) {
	codec := JSONCodec[mutatingJSONValue]{}
	input := string([]byte(`"original"`))
	original := input

	decoded, err := codec.Decode(input)
	require.NoError(t, err)
	assert.Equal(t, mutatingJSONValue("decoded"), decoded)
	assert.Equal(t, original, input)
}
