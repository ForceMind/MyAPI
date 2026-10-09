package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ValidateUniqueJSONKeys rejects duplicate decoded object keys and excessive
// structure without changing request bytes or number spelling. Callers retain
// their own byte limits and protocol-specific case/field rules. In particular,
// this does not change the stricter CanonicalJSONObjectDigest contract.
func ValidateUniqueJSONKeys(input []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	nodes := 0
	if err := validateUniqueJSONValue(decoder, 0, &nodes); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("invalid JSON structure")
	}
	return nil
}

func validateUniqueJSONValue(decoder *json.Decoder, depth int, nodes *int) error {
	*nodes++
	if depth > 64 || *nodes > 50_000 {
		return errors.New("JSON structure exceeds limits")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("invalid JSON structure")
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	if delimiter != '{' && delimiter != '[' {
		return errors.New("invalid JSON structure")
	}
	keys := map[string]struct{}{}
	for decoder.More() {
		if delimiter == '{' {
			keyToken, err := decoder.Token()
			key, ok := keyToken.(string)
			if err != nil || !ok {
				return errors.New("invalid JSON key")
			}
			if _, found := keys[key]; found {
				return errors.New("duplicate JSON key")
			}
			keys[key] = struct{}{}
		}
		if err := validateUniqueJSONValue(decoder, depth+1, nodes); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || (delimiter == '{' && closing != json.Delim('}')) || (delimiter == '[' && closing != json.Delim(']')) {
		return errors.New("invalid JSON structure")
	}
	return nil
}

func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func UnmarshalJsonStr(data string, v any) error {
	return json.Unmarshal(StringToByteSlice(data), v)
}

func DecodeJson(reader io.Reader, v any) error {
	return json.NewDecoder(reader).Decode(v)
}

// DecodeJsonStrict decodes exactly one JSON value, rejecting unknown object
// fields and any trailing JSON value. Use it for persisted configuration where
// silently accepting additional data could hide a malformed or mistyped option.
func DecodeJsonStrict(reader io.Reader, v any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func IndentJson(data []byte) ([]byte, error) {
	var buffer bytes.Buffer
	if err := json.Indent(&buffer, data, "", "  "); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func GetJsonType(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "unknown"
	}
	firstChar := trimmed[0]
	switch firstChar {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// JsonRawMessageToString returns JSON strings as their decoded value and other JSON values as raw text.
func JsonRawMessageToString(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] != '"' {
		return string(trimmed)
	}
	var value string
	if err := Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	return value
}
