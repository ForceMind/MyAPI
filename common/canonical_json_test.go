package common

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalJSONObjectDigestKnownAnswer(t *testing.T) {
	digest, err := CanonicalJSONObjectDigest([]byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "69c29c051218dbea38795b1cdfd816855c312a9cbc0b1daaa79bb9a0b895361a", hex.EncodeToString(digest[:]))
}

func TestCanonicalJSONObjectDigestPreservesRequiredJSONSemantics(t *testing.T) {
	baseline, err := CanonicalJSONObjectDigest([]byte(`{"b":[true,null,"\u00e9"],"a":{"n":1}}`))
	require.NoError(t, err)
	equivalent, err := CanonicalJSONObjectDigest([]byte(" { \"a\" : {\"n\":1}, \"b\": [ true, null, \"é\" ] } \n"))
	require.NoError(t, err)
	assert.Equal(t, baseline, equivalent, "member order, whitespace, and equivalent string escapes are not semantic differences")

	for _, different := range []string{
		`{"b":[null,true,"\u00e9"],"a":{"n":1}}`,
		`{"b":[true,null,"\u00e9"],"a":{"n":1.0}}`,
		`{"b":[true,null,"e\u0301"],"a":{"n":1}}`,
	} {
		digest, err := CanonicalJSONObjectDigest([]byte(different))
		require.NoError(t, err)
		assert.NotEqual(t, baseline, digest)
	}
}

func TestCanonicalJSONObjectDigestRejectsDuplicateDecodedKeys(t *testing.T) {
	for _, input := range []string{
		`{"key":1,"key":2}`,
		`{"key":1,"\u006bey":2}`,
		`{"outer":{"key":1,"\u006bey":2}}`,
	} {
		_, err := CanonicalJSONObjectDigest([]byte(input))
		assert.ErrorIs(t, err, ErrCanonicalJSONObject)
		assert.Equal(t, ErrCanonicalJSONObject.Error(), err.Error())
	}
}

func TestCanonicalJSONObjectDigestRejectsIsolatedSurrogatesWithoutAliasingReplacementCharacters(t *testing.T) {
	for _, input := range []string{
		`{"\ud800":1}`,
		`{"\udc00":1}`,
		`{"value":"\ud801"}`,
		`{"value":"\udfff"}`,
		`{"value":"\ud800x"}`,
		`{"value":"\ud800\u0041"}`,
	} {
		_, err := CanonicalJSONObjectDigest([]byte(input))
		require.ErrorIs(t, err, ErrCanonicalJSONObject)
		assert.Equal(t, ErrCanonicalJSONObject.Error(), err.Error())
	}

	escapedPair, err := CanonicalJSONObjectDigest([]byte(`{"\ud800\udc00":"\ud83d\ude00"}`))
	require.NoError(t, err)
	literalPair, err := CanonicalJSONObjectDigest([]byte("{\"𐀀\":\"😀\"}"))
	require.NoError(t, err)
	assert.Equal(t, escapedPair, literalPair)

	replacement, err := CanonicalJSONObjectDigest([]byte(`{"value":"�"}`))
	require.NoError(t, err)
	assert.NotEqual(t, [32]byte{}, replacement)
}

func TestCanonicalJSONObjectDigestRejectsInvalidRootsAndEncodingWithoutReflection(t *testing.T) {
	canary := "do-not-reflect-this-secret"
	inputs := [][]byte{
		nil,
		[]byte(`[]`),
		[]byte(`null`),
		[]byte(`{"ok":true} {"second":true}`),
		[]byte(`{"ok":true} ` + canary),
		append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"ok":true}`)...),
		append([]byte(`{"value":"`), 0xff, '"', '}'),
	}
	for _, input := range inputs {
		_, err := CanonicalJSONObjectDigest(input)
		require.ErrorIs(t, err, ErrCanonicalJSONObject)
		assert.Equal(t, ErrCanonicalJSONObject.Error(), err.Error())
		assert.NotContains(t, err.Error(), canary)
	}
}

func TestCanonicalJSONObjectDigestEnforcesExactResourceBoundaries(t *testing.T) {
	exactBytes := []byte(`{"v":"` + strings.Repeat("a", canonicalJSONMaxBytes-len(`{"v":""}`)) + `"}`)
	require.Len(t, exactBytes, canonicalJSONMaxBytes)
	_, err := CanonicalJSONObjectDigest(exactBytes)
	require.NoError(t, err)
	tooManyBytes := append(exactBytes[:len(exactBytes)-2], 'a', '"', '}')
	require.Len(t, tooManyBytes, canonicalJSONMaxBytes+1)
	_, err = CanonicalJSONObjectDigest(tooManyBytes)
	assert.ErrorIs(t, err, ErrCanonicalJSONObject)

	exactDepth := `{"value":` + strings.Repeat(`[`, canonicalJSONMaxDepth-2) + `null` + strings.Repeat(`]`, canonicalJSONMaxDepth-2) + `}`
	_, err = CanonicalJSONObjectDigest([]byte(exactDepth))
	require.NoError(t, err)
	tooDeep := `{"value":` + strings.Repeat(`[`, canonicalJSONMaxDepth-1) + `null` + strings.Repeat(`]`, canonicalJSONMaxDepth-1) + `}`
	_, err = CanonicalJSONObjectDigest([]byte(tooDeep))
	assert.ErrorIs(t, err, ErrCanonicalJSONObject)

	exactNodes := canonicalJSONObjectWithNullArray(canonicalJSONMaxNodes - 2)
	_, err = CanonicalJSONObjectDigest([]byte(exactNodes))
	require.NoError(t, err)
	tooManyNodes := canonicalJSONObjectWithNullArray(canonicalJSONMaxNodes - 1)
	_, err = CanonicalJSONObjectDigest([]byte(tooManyNodes))
	assert.ErrorIs(t, err, ErrCanonicalJSONObject)
}

func TestCanonicalJSONObjectDigestRetainsNumberLexemesAndContainerTypes(t *testing.T) {
	seen := make(map[[32]byte]string)
	for _, number := range []string{"0", "-0", "1", "1.0", "1e0", "1E+0"} {
		digest, err := CanonicalJSONObjectDigest([]byte(`{"number":` + number + `}`))
		require.NoError(t, err)
		if prior, duplicate := seen[digest]; duplicate {
			t.Fatalf("number spellings %q and %q produced one digest", prior, number)
		}
		seen[digest] = number
	}

	arrayDigest, err := CanonicalJSONObjectDigest([]byte(`{"value":[]}`))
	require.NoError(t, err)
	objectDigest, err := CanonicalJSONObjectDigest([]byte(`{"value":{}}`))
	require.NoError(t, err)
	assert.NotEqual(t, arrayDigest, objectDigest)
}

func TestCanonicalJSONObjectDigestSortsDecodedUTF8KeyBytes(t *testing.T) {
	first, err := CanonicalJSONObjectDigest([]byte(`{"中":4,"é":3,"z":2,"é":1}`))
	require.NoError(t, err)
	second, err := CanonicalJSONObjectDigest([]byte(`{"e\u0301":1,"z":2,"\u00e9":3,"\u4e2d":4}`))
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, "4ed0e588beb7dbf46ae18a392333a07aaf048b48dd3bdd3de3f91af95e5d070a", hex.EncodeToString(first[:]))
}

func canonicalJSONObjectWithNullArray(elements int) string {
	if elements == 0 {
		return `{"value":[]}`
	}
	return `{"value":[` + strings.Repeat(`null,`, elements-1) + `null]}`
}
