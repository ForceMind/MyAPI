package common

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"sort"
	"unicode/utf8"
)

const (
	canonicalJSONMaxBytes = 1 << 20
	canonicalJSONMaxDepth = 64
	canonicalJSONMaxNodes = 50_000
)

var ErrCanonicalJSONObject = errors.New("canonical JSON object is invalid or exceeds limits")

const (
	canonicalJSONNull byte = iota
	canonicalJSONFalse
	canonicalJSONTrue
	canonicalJSONNumber
	canonicalJSONString
	canonicalJSONArray
	canonicalJSONObject
)

type canonicalJSONParser struct {
	decoder *json.Decoder
	nodes   int
}

type canonicalJSONObjectEntry struct {
	key    string
	digest [sha256.Size]byte
}

// CanonicalJSONObjectDigest returns only a Merkle SHA-256 digest of one
// strictly parsed top-level JSON object. Object members are ordered by their
// decoded UTF-8 key bytes, array order is retained, strings are bound by their
// decoded value, and numbers retain their original valid JSON spelling. It
// deliberately performs no Unicode normalization.
//
// The input is bounded to 1 MiB, 64 nested values, and 50,000 value nodes.
// Duplicate decoded object keys, a BOM, invalid UTF-8, a second root value, and
// trailing non-whitespace data are rejected. No error contains input data and
// this API never materializes or returns a canonical request body.
func CanonicalJSONObjectDigest(input []byte) ([sha256.Size]byte, error) {
	if len(input) == 0 || len(input) > canonicalJSONMaxBytes || !utf8.Valid(input) ||
		bytes.HasPrefix(input, []byte{0xef, 0xbb, 0xbf}) || !validCanonicalJSONSurrogateEscapes(input) {
		return [sha256.Size]byte{}, ErrCanonicalJSONObject
	}

	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	parser := canonicalJSONParser{decoder: decoder}
	digest, kind, err := parser.readValue(1)
	if err != nil || kind != canonicalJSONObject {
		return [sha256.Size]byte{}, ErrCanonicalJSONObject
	}
	if _, err := decoder.Token(); err != io.EOF {
		return [sha256.Size]byte{}, ErrCanonicalJSONObject
	}
	return digest, nil
}

// encoding/json replaces an isolated UTF-16 surrogate escape with U+FFFD.
// Reject it before decoding so distinct invalid escape spellings cannot alias
// a literal replacement character (or each other) in a security identity.
func validCanonicalJSONSurrogateEscapes(input []byte) bool {
	inString := false
	for offset := 0; offset < len(input); {
		if !inString {
			if input[offset] == '"' {
				inString = true
			}
			offset++
			continue
		}
		switch input[offset] {
		case '"':
			inString = false
			offset++
		case '\\':
			if offset+1 >= len(input) {
				return false
			}
			if input[offset+1] != 'u' {
				offset += 2
				continue
			}
			first, ok := canonicalJSONHexQuad(input, offset+2)
			if !ok {
				return false
			}
			switch {
			case first >= 0xd800 && first <= 0xdbff:
				if offset+12 > len(input) || input[offset+6] != '\\' || input[offset+7] != 'u' {
					return false
				}
				second, ok := canonicalJSONHexQuad(input, offset+8)
				if !ok || second < 0xdc00 || second > 0xdfff {
					return false
				}
				offset += 12
			case first >= 0xdc00 && first <= 0xdfff:
				return false
			default:
				offset += 6
			}
		default:
			offset++
		}
	}
	return true
}

func canonicalJSONHexQuad(input []byte, offset int) (uint16, bool) {
	if offset+4 > len(input) {
		return 0, false
	}
	var value uint16
	for _, character := range input[offset : offset+4] {
		value <<= 4
		switch {
		case character >= '0' && character <= '9':
			value += uint16(character - '0')
		case character >= 'a' && character <= 'f':
			value += uint16(character-'a') + 10
		case character >= 'A' && character <= 'F':
			value += uint16(character-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func (parser *canonicalJSONParser) readValue(depth int) ([sha256.Size]byte, byte, error) {
	if depth > canonicalJSONMaxDepth || parser.nodes >= canonicalJSONMaxNodes {
		return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
	}
	parser.nodes++

	token, err := parser.decoder.Token()
	if err != nil {
		return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
	}
	switch value := token.(type) {
	case nil:
		return sha256.Sum256([]byte{canonicalJSONNull}), canonicalJSONNull, nil
	case bool:
		if value {
			return sha256.Sum256([]byte{canonicalJSONTrue}), canonicalJSONTrue, nil
		}
		return sha256.Sum256([]byte{canonicalJSONFalse}), canonicalJSONFalse, nil
	case json.Number:
		return canonicalJSONScalarDigest(canonicalJSONNumber, []byte(value.String())), canonicalJSONNumber, nil
	case string:
		return canonicalJSONScalarDigest(canonicalJSONString, []byte(value)), canonicalJSONString, nil
	case json.Delim:
		switch value {
		case '[':
			return parser.readArray(depth)
		case '{':
			return parser.readObject(depth)
		default:
			return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
		}
	default:
		return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
	}
}

func (parser *canonicalJSONParser) readArray(depth int) ([sha256.Size]byte, byte, error) {
	digests := make([][sha256.Size]byte, 0)
	for parser.decoder.More() {
		digest, _, err := parser.readValue(depth + 1)
		if err != nil {
			return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
		}
		digests = append(digests, digest)
	}
	closing, err := parser.decoder.Token()
	if err != nil || closing != json.Delim(']') {
		return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
	}

	hasher := sha256.New()
	hasher.Write([]byte{canonicalJSONArray})
	writeCanonicalJSONLength(hasher, len(digests))
	for _, digest := range digests {
		hasher.Write(digest[:])
	}
	return canonicalJSONHash(hasher), canonicalJSONArray, nil
}

func (parser *canonicalJSONParser) readObject(depth int) ([sha256.Size]byte, byte, error) {
	entries := make([]canonicalJSONObjectEntry, 0)
	keys := make(map[string]struct{})
	for parser.decoder.More() {
		keyToken, err := parser.decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
		}
		if _, duplicate := keys[key]; duplicate {
			return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
		}
		keys[key] = struct{}{}
		digest, _, err := parser.readValue(depth + 1)
		if err != nil {
			return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
		}
		entries = append(entries, canonicalJSONObjectEntry{key: key, digest: digest})
	}
	closing, err := parser.decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return [sha256.Size]byte{}, 0, ErrCanonicalJSONObject
	}

	sort.Slice(entries, func(first, second int) bool {
		return entries[first].key < entries[second].key
	})
	hasher := sha256.New()
	hasher.Write([]byte{canonicalJSONObject})
	writeCanonicalJSONLength(hasher, len(entries))
	for _, entry := range entries {
		key := []byte(entry.key)
		writeCanonicalJSONLength(hasher, len(key))
		hasher.Write(key)
		hasher.Write(entry.digest[:])
	}
	return canonicalJSONHash(hasher), canonicalJSONObject, nil
}

func canonicalJSONScalarDigest(kind byte, value []byte) [sha256.Size]byte {
	hasher := sha256.New()
	hasher.Write([]byte{kind})
	writeCanonicalJSONLength(hasher, len(value))
	hasher.Write(value)
	return canonicalJSONHash(hasher)
}

func writeCanonicalJSONLength(hasher hash.Hash, length int) {
	var frame [8]byte
	binary.BigEndian.PutUint64(frame[:], uint64(length))
	hasher.Write(frame[:])
}

func canonicalJSONHash(hasher hash.Hash) [sha256.Size]byte {
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest
}
