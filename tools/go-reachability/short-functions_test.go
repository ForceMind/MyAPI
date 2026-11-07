//go:build ignore

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shortFixtureSources(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	var tracked []string
	for relative, contents := range files {
		filename := filepath.Join(root, filepath.FromSlash(relative))
		require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o700))
		require.NoError(t, os.WriteFile(filename, []byte(contents), 0o640))
		tracked = append(tracked, relative)
	}
	return root, tracked
}

func shortRecord(t *testing.T, report shortReport, file, name string) shortFunction {
	t.Helper()
	for _, record := range report.Records.Items {
		if record.File == file && record.Name == name {
			return record
		}
	}
	t.Fatalf("missing source record %s:%s", file, name)
	return shortFunction{}
}

func TestShortFunctionsSingleDirectCallBindsEvidenceAndReadsOnly(t *testing.T) {
	contents := "package sample\nfunc helper() {}\nfunc caller() { helper() }\n"
	root, tracked := shortFixtureSources(t, map[string]string{"sample.go": contents})
	report, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	record := shortRecord(t, report, "sample.go", "helper")
	assert.True(t, record.ManualReviewCandidate)
	assert.False(t, record.DeletionAuthorized)
	assert.Equal(t, "1", record.ProductionReferences.Value)
	assert.Equal(t, "0", record.TestReferences.Value)
	assert.Equal(t, "1", record.BodyLines.Value)
	assert.Equal(t, 1, record.FunctionLines)
	require.Len(t, record.References.Items, 1)
	assert.Equal(t, "direct-call", record.References.Items[0].Kind)
	assert.Equal(t, 3, record.References.Items[0].Line)
	assert.Equal(t, 17, record.References.Items[0].Column)
	assert.Empty(t, record.Protections.Reasons)
	hash := sha256.Sum256([]byte(contents))
	assert.Equal(t, hex.EncodeToString(hash[:]), record.FileSHA256)
	assert.Equal(t, record.FileSHA256, record.References.Items[0].FileSHA256)
	actual, err := os.ReadFile(filepath.Join(root, "sample.go"))
	require.NoError(t, err)
	assert.Equal(t, contents, string(actual))
	info, err := os.Stat(filepath.Join(root, "sample.go"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	assert.True(t, report.AnalysisOnly)
	assert.False(t, report.DeletionAuthorized)
	assert.Equal(t, 1, report.Summary.ManualReviewCandidates)
}

func TestShortFunctionsCallbackTestAndNonlocalReferencesStayProtected(t *testing.T) {
	for _, fixture := range []struct {
		name, caller, testSource, reason, kind string
	}{
		{name: "callback", caller: "var callback = helper\n", reason: "non-direct-reference", kind: "function-value"},
		{name: "direct test", caller: "func caller() { helper() }\n", testSource: "package sample\nfunc proof() { helper() }\n", reason: "test-reference", kind: "direct-call"},
		{name: "deferred call", caller: "func caller() { defer helper() }\n", reason: "non-direct-reference", kind: "deferred-call"},
		{name: "goroutine call", caller: "func caller() { go helper() }\n", reason: "non-direct-reference", kind: "goroutine-call"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			files := map[string]string{"sample.go": "package sample\nfunc helper() {}\n" + fixture.caller}
			if fixture.testSource != "" {
				files["sample_test.go"] = fixture.testSource
			}
			root, tracked := shortFixtureSources(t, files)
			report, err := collectShortFunctions(root, tracked)
			require.NoError(t, err)
			record := shortRecord(t, report, "sample.go", "helper")
			assert.False(t, record.ManualReviewCandidate)
			assert.Contains(t, record.Protections.Reasons, fixture.reason)
			assert.Equal(t, fixture.kind, record.References.Items[0].Kind)
		})
	}
	t.Run("different directory package is not a second reference", func(t *testing.T) {
		root, tracked := shortFixtureSources(t, map[string]string{
			"sample.go":      "package sample\nfunc helper() {}\nfunc caller() { helper() }\n",
			"other/other.go": "package sample\nfunc helper() {}\n",
		})
		report, err := collectShortFunctions(root, tracked)
		require.NoError(t, err)
		assert.True(t, shortRecord(t, report, "sample.go", "helper").ManualReviewCandidate)
		assert.False(t, shortRecord(t, report, "other/other.go", "helper").ManualReviewCandidate)
	})
}

func TestShortFunctionsDuplicateBuildVariantsAndShadowedNamesAreNotGuessed(t *testing.T) {
	root, tracked := shortFixtureSources(t, map[string]string{
		"helper_linux.go":   "package sample\nfunc helper() {}\n",
		"helper_windows.go": "package sample\nfunc helper() {}\n",
		"caller.go":         "package sample\nfunc caller() { helper() }\n",
	})
	report, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	for _, file := range []string{"helper_linux.go", "helper_windows.go"} {
		record := shortRecord(t, report, file, "helper")
		assert.False(t, record.ManualReviewCandidate)
		assert.Contains(t, record.Protections.Reasons, "duplicate-definition")
		assert.Contains(t, record.Protections.Reasons, "build-constrained-source")
	}
	t.Run("local variable name is not counted as a package function", func(t *testing.T) {
		root, tracked := shortFixtureSources(t, map[string]string{
			"sample.go": "package sample\nfunc helper() {}\nfunc caller() { helper() }\nfunc other() { helper := 1; _ = helper }\n",
		})
		report, err := collectShortFunctions(root, tracked)
		require.NoError(t, err)
		record := shortRecord(t, report, "sample.go", "helper")
		assert.Equal(t, "1", record.ProductionReferences.Value)
		assert.Contains(t, record.Protections.Reasons, "shadowed-or-ambiguous-name")
		assert.False(t, record.ManualReviewCandidate)
	})
}

func TestShortFunctionsProtectAPIsHooksInitGeneratedFixturesAndRelaykit(t *testing.T) {
	root, tracked := shortFixtureSources(t, map[string]string{
		"sample.go":           "package sample\nfunc Public() {}\nfunc Ωmega() {}\nfunc init() {}\ntype row struct{}\nfunc (row) BeforeCreate() {}\nfunc caller() { Public(); Ωmega(); generatedHelper() }\n",
		"generated.go":        "// Code generated by fixture. DO NOT EDIT.\npackage sample\nfunc generatedHelper() {}\n",
		"proof_test.go":       "package sample\nfunc testHelper() {}\nfunc proof() { testHelper() }\n",
		"testdata/fixture.go": "package fixture\nfunc helper() {}\nfunc caller() { helper() }\n",
		"relaykit/api.go":     "package relaykit\nfunc helper() {}\nfunc caller() { helper() }\n",
	})
	report, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	for _, expected := range []struct{ file, name, reason string }{
		{"sample.go", "Public", "exported-api"},
		{"sample.go", "Ωmega", "exported-api"},
		{"sample.go", "init", "init"},
		{"sample.go", "BeforeCreate", "method-or-hook"},
		{"generated.go", "generatedHelper", "generated"},
		{"proof_test.go", "testHelper", "test-source"},
		{"testdata/fixture.go", "helper", "fixture-source"},
		{"relaykit/api.go", "helper", "relaykit-public-boundary"},
	} {
		record := shortRecord(t, report, expected.file, expected.name)
		assert.Contains(t, record.Protections.Reasons, expected.reason)
		assert.False(t, record.ManualReviewCandidate)
	}
	method := shortRecord(t, report, "sample.go", "BeforeCreate")
	assert.True(t, method.ProductionReferences.Unknown)
	assert.Empty(t, method.ProductionReferences.Value)
	assert.True(t, method.TestReferences.Unknown)
	assert.Empty(t, method.TestReferences.Value)
}

func TestShortFunctionsLinkDirectiveAndPhysicalLineEvidence(t *testing.T) {
	root, tracked := shortFixtureSources(t, map[string]string{
		"sample.go":     "package sample\n//line imaginary/outside.go:900\nfunc helper() {}\nfunc caller() { helper() }\n",
		"other/link.go": "package other\n//go:linkname alias example.invalid/sample.helper\nfunc alias()\n",
	})
	report, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	record := shortRecord(t, report, "sample.go", "helper")
	assert.Equal(t, 3, record.Line)
	assert.Equal(t, "sample.go", record.File)
	assert.False(t, record.ManualReviewCandidate)
	assert.Contains(t, record.Protections.Reasons, "non-direct-reference")
	require.Len(t, record.References.Items, 2)
	assert.Equal(t, "link-directive", record.References.Items[0].Kind)
}

func TestShortFunctionsMissingMalformedAndUnsafeSourcesCannotBecomeZero(t *testing.T) {
	root, _ := shortFixtureSources(t, map[string]string{"valid.go": "package sample\n"})
	_, err := collectShortFunctions(root, nil)
	require.Error(t, err)
	for _, paths := range [][]string{{"missing.go"}, {"../outside.go"}, {filepath.Join(root, "valid.go")}, {"valid.go", "valid.go"}} {
		_, err := collectShortFunctions(root, paths)
		require.Error(t, err)
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "broken.go"), []byte("package sample\nfunc broken("), 0o600))
	_, err = collectShortFunctions(root, []string{"broken.go"})
	require.Error(t, err)
	outside := filepath.Join(t.TempDir(), "outside.go")
	require.NoError(t, os.WriteFile(outside, []byte("package sample\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked.go")))
	_, err = collectShortFunctions(root, []string{"linked.go"})
	require.Error(t, err)
	require.NoError(t, os.Symlink(filepath.Dir(outside), filepath.Join(root, "linked-directory")))
	_, err = collectShortFunctions(root, []string{"linked-directory/outside.go"})
	require.Error(t, err)
}

func TestShortFunctionsXMLPreservesEmptySuccessAndUnknownCounts(t *testing.T) {
	root, tracked := shortFixtureSources(t, map[string]string{"empty.go": "package sample\n"})
	report, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	data, err := xml.Marshal(report)
	require.NoError(t, err)
	var empty struct {
		XMLName xml.Name
		Files   *shortFiles   `xml:"files"`
		Records *shortRecords `xml:"records"`
	}
	require.NoError(t, xml.Unmarshal(data, &empty))
	assert.Equal(t, "short_functions_report", empty.XMLName.Local)
	require.NotNil(t, empty.Files)
	require.NotNil(t, empty.Records)
	assert.Empty(t, empty.Records.Items)
	assert.Equal(t, 1, report.Summary.Files)
	assert.Zero(t, report.Summary.ShortFunctions)

	root, tracked = shortFixtureSources(t, map[string]string{"method.go": "package sample\ntype row struct{}\nfunc (row) hook() {}\n"})
	report, err = collectShortFunctions(root, tracked)
	require.NoError(t, err)
	data, err = xml.Marshal(report)
	require.NoError(t, err)
	var decoded shortReport
	require.NoError(t, xml.Unmarshal(data, &decoded))
	method := shortRecord(t, decoded, "method.go", "hook")
	assert.True(t, decoded.AnalysisOnly)
	assert.False(t, decoded.DeletionAuthorized)
	assert.True(t, method.ProductionReferences.Unknown)
	assert.Empty(t, method.ProductionReferences.Value)
	assert.False(t, method.ManualReviewCandidate)
	assert.False(t, method.DeletionAuthorized)
}

func TestShortFunctionsRequireExactCleanGitIdentityBeforeOutput(t *testing.T) {
	sha := strings.Repeat("a", 40)
	require.NoError(t, validateShortIdentity(sha, sha, ""))
	for _, fixture := range []struct{ expected, actual, dirty string }{
		{"", sha, ""},
		{"latest", sha, ""},
		{sha, strings.Repeat("b", 40), ""},
		{sha, sha, " M sample.go\x00"},
		{sha, sha, "M  sample.go\x00"},
	} {
		require.Error(t, validateShortIdentity(fixture.expected, fixture.actual, fixture.dirty))
	}
	require.Error(t, runShortFunctions(nil, io.Discard, io.Discard))
	require.Error(t, runShortFunctions([]string{"--source-sha", sha}, io.Discard, io.Discard))
	require.Error(t, runShortFunctions([]string{"--unknown"}, io.Discard, io.Discard))
}

func TestShortFunctionsTrackedBlobIdentityCannotHideChangedOrMissingBytes(t *testing.T) {
	root, tracked := shortFixtureSources(t, map[string]string{"sample.go": "package sample\nfunc helper() {}\n"})
	report, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	require.Len(t, report.Files.Items, 1)
	file := report.Files.Items[0]
	raw := "100644 " + file.GitBlobSHA1 + " 0\tsample.go\x00"
	paths, blobs, err := parseShortTracked(raw)
	require.NoError(t, err)
	assert.Equal(t, []string{"sample.go"}, paths)
	require.NoError(t, validateShortBlobs(report.Files.Items, blobs))
	require.NoError(t, os.WriteFile(filepath.Join(root, "sample.go"), []byte("package sample\nfunc changed() {}\n"), 0o640))
	changed, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	require.Error(t, validateShortBlobs(changed.Files.Items, blobs))
	require.Error(t, validateShortBlobs(report.Files.Items, nil))
	for _, invalid := range []string{"", "sample.go\x00", strings.Replace(raw, " 0\t", " 2\t", 1), raw + raw} {
		_, _, err := parseShortTracked(invalid)
		require.Error(t, err)
	}
}

func TestShortFunctionsCompilerExportDirectiveIsNotAnOrdinaryPrivateHelper(t *testing.T) {
	root, tracked := shortFixtureSources(t, map[string]string{
		"sample.go": "package sample\n//export helper\nfunc helper() {}\nfunc caller() { helper() }\n",
	})
	report, err := collectShortFunctions(root, tracked)
	require.NoError(t, err)
	record := shortRecord(t, report, "sample.go", "helper")
	assert.False(t, record.ManualReviewCandidate)
	assert.Contains(t, record.Protections.Reasons, "compiler-or-export-directive")
}
