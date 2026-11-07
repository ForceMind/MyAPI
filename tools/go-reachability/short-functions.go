//go:build ignore

// This analysis-only command reads tracked Go syntax without importing the
// application, type-checking packages, or executing product initialization.
package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type shortCount struct {
	Unknown bool   `xml:"unknown,attr,omitempty"`
	Value   string `xml:",chardata"`
}

func knownShortCount(value int) shortCount {
	return shortCount{Value: strconv.Itoa(value)}
}

type shortReference struct {
	File       string `xml:"file"`
	Line       int    `xml:"line"`
	Column     int    `xml:"column"`
	Kind       string `xml:"kind"`
	Test       bool   `xml:"test"`
	FileSHA256 string `xml:"file_sha256"`
}

type shortReferences struct {
	Items []shortReference `xml:"reference"`
}

type shortProtections struct {
	Reasons []string `xml:"reason"`
}

type shortFunction struct {
	File                  string           `xml:"file"`
	Line                  int              `xml:"line"`
	Column                int              `xml:"column"`
	Package               string           `xml:"package"`
	Name                  string           `xml:"name"`
	Module                string           `xml:"module"`
	BodyLines             shortCount       `xml:"body_lines"`
	FunctionLines         int              `xml:"function_lines"`
	FileSHA256            string           `xml:"file_sha256"`
	Exported              bool             `xml:"exported"`
	Method                bool             `xml:"method"`
	ProductionReferences  shortCount       `xml:"production_references"`
	TestReferences        shortCount       `xml:"test_references"`
	References            shortReferences  `xml:"references"`
	Protections           shortProtections `xml:"protections"`
	ManualReviewCandidate bool             `xml:"manual_review_candidate"`
	DeletionAuthorized    bool             `xml:"deletion_authorized"`
}

type shortFileInfo struct {
	File             string `xml:"file"`
	Package          string `xml:"package"`
	SHA256           string `xml:"sha256"`
	GitBlobSHA1      string `xml:"git_blob_sha1"`
	Test             bool   `xml:"test"`
	Generated        bool   `xml:"generated"`
	Fixture          bool   `xml:"fixture"`
	BuildConstrained bool   `xml:"build_constrained"`
}

type shortFiles struct {
	Items []shortFileInfo `xml:"file"`
}

type shortRecords struct {
	Items []shortFunction `xml:"record"`
}

type shortSummary struct {
	Files                  int `xml:"files"`
	ShortFunctions         int `xml:"short_functions"`
	ManualReviewCandidates int `xml:"manual_review_candidates"`
}

type shortScope struct {
	Input               string `xml:"input"`
	ReferenceResolution string `xml:"reference_resolution"`
	PositionBasis       string `xml:"position_basis"`
	BuildScope          string `xml:"build_scope"`
	Execution           string `xml:"execution"`
	Unsupported         string `xml:"unsupported_checks"`
}

type shortInvocation struct {
	Arguments []string `xml:"argument"`
}

type shortReport struct {
	XMLName            xml.Name        `xml:"short_functions_report"`
	SchemaVersion      int             `xml:"schema_version"`
	SourceSHA          string          `xml:"source_sha"`
	GoVersion          string          `xml:"go_version"`
	AnalysisOnly       bool            `xml:"analysis_only"`
	DeletionAuthorized bool            `xml:"deletion_authorized"`
	Scope              shortScope      `xml:"scope"`
	Invocation         shortInvocation `xml:"invocation"`
	Summary            shortSummary    `xml:"summary"`
	Files              shortFiles      `xml:"files"`
	Records            shortRecords    `xml:"records"`
}

type shortSource struct {
	info shortFileInfo
	tree *ast.File
	key  string
}

type shortDeclaration struct {
	source     *shortSource
	node       *ast.FuncDecl
	record     shortFunction
	references []shortReference
	ambiguous  bool
}

func shortRoot(root string) (string, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root ||
		!utf8.ValidString(root) || strings.IndexFunc(root, unicode.IsControl) >= 0 {
		return "", errors.New("root must be an explicit clean absolute directory")
	}
	actual, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || actual != root || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("root is not a real non-symbolic directory")
	}
	return root, nil
}

func readShortSource(root, relative string) ([]byte, error) {
	if relative == "" || filepath.IsAbs(relative) || filepath.ToSlash(filepath.Clean(relative)) != relative ||
		relative == ".." || strings.HasPrefix(relative, "../") || !strings.HasSuffix(relative, ".go") ||
		!utf8.ValidString(relative) || strings.IndexFunc(relative, unicode.IsControl) >= 0 {
		return nil, errors.New("invalid tracked Go path")
	}
	current := root
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, fmt.Errorf("missing tracked source %q: %w", relative, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || index < len(parts)-1 && !info.IsDir() {
			return nil, fmt.Errorf("symbolic or non-directory source path %q", relative)
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("source %q is not a regular file", relative)
		}
	}
	return os.ReadFile(current)
}

func shortBuildConstrained(relative string, tree *ast.File) bool {
	for _, group := range tree.Comments {
		for _, comment := range group.List {
			if comment.Pos() < tree.Package &&
				(strings.HasPrefix(comment.Text, "//go:build ") || strings.HasPrefix(comment.Text, "// +build ")) {
				return true
			}
		}
	}
	for _, imported := range tree.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err == nil && path == "C" {
			return true
		}
	}
	// All files are parsed; these suffixes are only conservative protection,
	// never a claim that a particular target was loaded.
	suffixes := " aix android darwin dragonfly freebsd hurd illumos ios js linux netbsd openbsd plan9 solaris wasip1 windows " +
		"386 amd64 amd64p32 arm arm64 loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm "
	base := strings.TrimSuffix(filepath.Base(relative), ".go")
	base = strings.TrimSuffix(base, "_test")
	pieces := strings.Split(base, "_")
	return len(pieces) > 1 && strings.Contains(suffixes, " "+pieces[len(pieces)-1]+" ")
}

func shortFixture(relative string) bool {
	if strings.Contains(filepath.Base(relative), "fixture") {
		return true
	}
	for _, part := range strings.Split(relative, "/") {
		if part == "testdata" || part == "fixture" || part == "fixtures" {
			return true
		}
	}
	return false
}

// collectShortFunctions accepts an explicit tracked-file list. Git identity and
// dirty-tree validation belong to the CLI boundary, not synthetic fixtures.
func collectShortFunctions(root string, tracked []string) (shortReport, error) {
	report := shortReport{
		SchemaVersion: 1, GoVersion: runtime.Version(), AnalysisOnly: true,
		Scope: shortScope{
			Input:               "git-tracked Go files only",
			ReferenceResolution: "same-directory/package bare identifiers; no type resolution; method counts unknown",
			PositionBasis:       "physical source lines and UTF-8 byte columns; line directives do not relocate evidence",
			BuildScope:          "all tracked syntax; build constraints are not evaluated and constrained sources are protected",
			Execution:           "no typecheck, application imports, product init, provider requests, or source writes",
			Unsupported:         "dynamic or out-of-repository references; external consumers; target/build-tag reachability; safe inlining or deletion",
		},
	}
	root, err := shortRoot(root)
	if err != nil {
		return report, err
	}
	if len(tracked) == 0 {
		return report, errors.New("missing tracked Go source input")
	}
	files := append([]string(nil), tracked...)
	sort.Strings(files)
	set := token.NewFileSet()
	var sources []*shortSource
	var declarations []*shortDeclaration
	groups := make(map[string][]*shortDeclaration)
	definitions := make(map[*ast.Ident]bool)
	for index, relative := range files {
		if index > 0 && files[index-1] == relative {
			return report, errors.New("duplicate tracked source input")
		}
		data, err := readShortSource(root, relative)
		if err != nil {
			return report, err
		}
		tree, err := parser.ParseFile(set, relative, data, parser.ParseComments)
		if err != nil {
			return report, fmt.Errorf("cannot parse tracked source %q: %w", relative, err)
		}
		hash := sha256.Sum256(data)
		blob := sha1.New()
		fmt.Fprintf(blob, "blob %d\x00", len(data))
		blob.Write(data)
		source := &shortSource{
			info: shortFileInfo{
				File: relative, Package: tree.Name.Name, SHA256: hex.EncodeToString(hash[:]), GitBlobSHA1: hex.EncodeToString(blob.Sum(nil)),
				Test: strings.HasSuffix(relative, "_test.go"), Generated: ast.IsGenerated(tree),
				Fixture: shortFixture(relative), BuildConstrained: shortBuildConstrained(relative, tree),
			},
			tree: tree, key: filepath.ToSlash(filepath.Dir(relative)) + "\x00" + tree.Name.Name,
		}
		sources = append(sources, source)
		report.Files.Items = append(report.Files.Items, source.info)
		for _, node := range tree.Decls {
			function, ok := node.(*ast.FuncDecl)
			if !ok {
				continue
			}
			definitions[function.Name] = true
			start := set.PositionFor(function.Name.Pos(), false)
			end := set.PositionFor(function.End(), false)
			body := shortCount{Unknown: true}
			if function.Body != nil {
				body = knownShortCount(end.Line - set.PositionFor(function.Body.Pos(), false).Line + 1)
			}
			module := "root"
			if strings.HasPrefix(relative, "relaykit/") {
				module = "relaykit"
			}
			declaration := &shortDeclaration{
				source: source, node: function,
				record: shortFunction{
					File: relative, Line: start.Line, Column: start.Column, Package: tree.Name.Name,
					Name: function.Name.Name, Module: module, BodyLines: body,
					FunctionLines: end.Line - set.PositionFor(function.Pos(), false).Line + 1,
					FileSHA256:    source.info.SHA256, Exported: ast.IsExported(function.Name.Name),
					Method: function.Recv != nil,
				},
			}
			declarations = append(declarations, declaration)
			if function.Recv == nil {
				key := source.key + "\x00" + function.Name.Name
				groups[key] = append(groups[key], declaration)
			}
		}
	}
	for _, source := range sources {
		var parents []ast.Node
		ast.Inspect(source.tree, func(node ast.Node) bool {
			if node == nil {
				parents = parents[:len(parents)-1]
				return true
			}
			if id, ok := node.(*ast.Ident); ok && !definitions[id] {
				matches := groups[source.key+"\x00"+id.Name]
				if len(matches) > 0 && len(parents) > 0 {
					parent := parents[len(parents)-1]
					selector, selected := parent.(*ast.SelectorExpr)
					if !(selected && selector.Sel == id) {
						if id.Obj != nil && id.Obj.Kind != ast.Fun {
							for _, declaration := range matches {
								declaration.ambiguous = true
							}
						} else {
							kind := "function-value"
							if call, ok := parent.(*ast.CallExpr); ok && call.Fun == id {
								kind = "direct-call"
								if len(parents) > 1 {
									switch parents[len(parents)-2].(type) {
									case *ast.GoStmt:
										kind = "goroutine-call"
									case *ast.DeferStmt:
										kind = "deferred-call"
									}
								}
							}
							position := set.PositionFor(id.Pos(), false)
							reference := shortReference{File: source.info.File, Line: position.Line,
								Column: position.Column, Kind: kind, Test: source.info.Test, FileSHA256: source.info.SHA256}
							for _, declaration := range matches {
								declaration.references = append(declaration.references, reference)
							}
						}
					}
				}
			}
			parents = append(parents, node)
			return true
		})
		// linkname bypasses ordinary Go visibility. Conservatively protect all
		// declarations matching its target name, even across package directories.
		for _, group := range source.tree.Comments {
			for _, comment := range group.List {
				words := strings.Fields(comment.Text)
				if len(words) < 2 || words[0] != "//go:linkname" {
					continue
				}
				position := set.PositionFor(comment.Pos(), false)
				for _, declaration := range declarations {
					local := declaration.source.key == source.key && declaration.record.Name == words[1]
					target := len(words) >= 3 && strings.HasSuffix(words[2], "."+declaration.record.Name)
					if local || target {
						declaration.references = append(declaration.references, shortReference{
							File: source.info.File, Line: position.Line, Column: position.Column,
							Kind: "link-directive", Test: source.info.Test, FileSHA256: source.info.SHA256,
						})
					}
				}
			}
		}
	}
	for _, declaration := range declarations {
		record := declaration.record
		// Include only physically short declarations, including protected APIs
		// and bodyless declarations, so the protection decisions remain visible.
		if record.FunctionLines > 15 {
			continue
		}
		protect := func(reason string) { record.Protections.Reasons = append(record.Protections.Reasons, reason) }
		if record.Exported {
			protect("exported-api")
		}
		if record.Method {
			protect("method-or-hook")
		}
		if record.Name == "init" {
			protect("init")
		}
		if record.Name == "_" {
			protect("blank-identifier")
		}
		if declaration.source.info.Generated {
			protect("generated")
		}
		if declaration.source.info.Test {
			protect("test-source")
		}
		if declaration.source.info.Fixture {
			protect("fixture-source")
		}
		if declaration.source.info.BuildConstrained {
			protect("build-constrained-source")
		}
		if record.Module == "relaykit" {
			protect("relaykit-public-boundary")
		}
		if declaration.node.Body == nil {
			protect("missing-body")
		}
		if declaration.node.Doc != nil {
			for _, comment := range declaration.node.Doc.List {
				if strings.HasPrefix(comment.Text, "//go:") || strings.HasPrefix(comment.Text, "//export ") {
					protect("compiler-or-export-directive")
					break
				}
			}
		}
		if declaration.ambiguous {
			protect("shadowed-or-ambiguous-name")
		}
		if !record.Method && len(groups[declaration.source.key+"\x00"+record.Name]) != 1 {
			protect("duplicate-definition")
		}
		sort.Slice(declaration.references, func(i, j int) bool {
			a, b := declaration.references[i], declaration.references[j]
			if a.File != b.File {
				return a.File < b.File
			}
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			if a.Column != b.Column {
				return a.Column < b.Column
			}
			return a.Kind < b.Kind
		})
		record.References.Items = declaration.references
		production, tests := 0, 0
		nonDirect := false
		for _, reference := range declaration.references {
			if reference.Test {
				tests++
			} else {
				production++
			}
			nonDirect = nonDirect || reference.Kind != "direct-call"
		}
		if record.Method {
			record.ProductionReferences = shortCount{Unknown: true}
			record.TestReferences = shortCount{Unknown: true}
		} else {
			record.ProductionReferences = knownShortCount(production)
			record.TestReferences = knownShortCount(tests)
			if production != 1 {
				protect("not-single-production-reference")
			}
			if tests != 0 {
				protect("test-reference")
			}
			if nonDirect {
				protect("non-direct-reference")
			}
		}
		record.ManualReviewCandidate = len(record.Protections.Reasons) == 0
		report.Records.Items = append(report.Records.Items, record)
		if record.ManualReviewCandidate {
			report.Summary.ManualReviewCandidates++
		}
	}
	sort.Slice(report.Records.Items, func(i, j int) bool {
		a, b := report.Records.Items[i], report.Records.Items[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Name < b.Name
	})
	report.Summary.Files = len(report.Files.Items)
	report.Summary.ShortFunctions = len(report.Records.Items)
	return report, nil
}

var shortSHA = regexp.MustCompile("^[a-f0-9]{40}$")

func parseShortTracked(raw string) ([]string, map[string]string, error) {
	if raw == "" || !strings.HasSuffix(raw, "\x00") {
		return nil, nil, errors.New("missing tracked Go source list")
	}
	var files []string
	blobs := make(map[string]string)
	for _, entry := range strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00") {
		header, filename, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(header)
		if !ok || filename == "" || len(fields) != 3 || fields[2] != "0" ||
			!shortSHA.MatchString(fields[1]) || (fields[0] != "100644" && fields[0] != "100755") {
			return nil, nil, errors.New("invalid tracked source identity")
		}
		if _, exists := blobs[filename]; exists {
			return nil, nil, errors.New("duplicate tracked source identity")
		}
		files = append(files, filename)
		blobs[filename] = fields[1]
	}
	return files, blobs, nil
}

func validateShortIdentity(expected, actual, dirty string) error {
	if !shortSHA.MatchString(expected) || actual != expected {
		return errors.New("source SHA must be a full lowercase SHA matching actual Git HEAD")
	}
	if dirty != "" {
		return errors.New("tracked working tree is dirty; no inventory was produced")
	}
	return nil
}

func validateShortBlobs(files []shortFileInfo, blobs map[string]string) error {
	if len(files) != len(blobs) {
		return errors.New("tracked source identity set is incomplete")
	}
	for _, file := range files {
		if !shortSHA.MatchString(file.GitBlobSHA1) || file.GitBlobSHA1 != blobs[file.File] {
			return fmt.Errorf("tracked source bytes differ from Git identity: %q", file.File)
		}
	}
	return nil
}

func shortGit(ctx context.Context, root string, args ...string) (string, error) {
	arguments := append([]string{"-c", "core.fileMode=false", "-C", root}, args...)
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	data, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read-only Git %s failed: %w", args[0], err)
	}
	return string(data), nil
}

func runShortFunctions(args []string, output, diagnostics io.Writer) error {
	flags := flag.NewFlagSet("short-functions", flag.ContinueOnError)
	flags.SetOutput(diagnostics)
	root := flags.String("root", "", "exact repository root")
	expected := flags.String("source-sha", "", "full checkout SHA")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *root == "" || !shortSHA.MatchString(*expected) {
		return errors.New("usage: short-functions --root ABSOLUTE_ROOT --source-sha FULL_SHA")
	}
	actualRoot, err := shortRoot(*root)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	top, err := shortGit(ctx, actualRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if strings.TrimSpace(top) != actualRoot {
		return errors.New("root is not the Git worktree root")
	}
	head, err := shortGit(ctx, actualRoot, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	dirty, err := shortGit(ctx, actualRoot, "status", "--porcelain=v1", "--untracked-files=no", "-z")
	if err != nil {
		return err
	}
	if err := validateShortIdentity(*expected, strings.TrimSpace(head), dirty); err != nil {
		return err
	}
	tracked, err := shortGit(ctx, actualRoot, "ls-files", "--stage", "-z", "--", "*.go")
	if err != nil {
		return err
	}
	files, blobs, err := parseShortTracked(tracked)
	if err != nil {
		return err
	}
	report, err := collectShortFunctions(actualRoot, files)
	if err != nil {
		return err
	}
	if err := validateShortBlobs(report.Files.Items, blobs); err != nil {
		return err
	}
	finalHead, err := shortGit(ctx, actualRoot, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	finalDirty, err := shortGit(ctx, actualRoot, "status", "--porcelain=v1", "--untracked-files=no", "-z")
	if err != nil {
		return err
	}
	if err := validateShortIdentity(*expected, strings.TrimSpace(finalHead), finalDirty); err != nil {
		return err
	}
	finalTracked, err := shortGit(ctx, actualRoot, "ls-files", "--stage", "-z", "--", "*.go")
	if err != nil {
		return err
	}
	if finalTracked != tracked {
		return errors.New("tracked source set changed during collection")
	}
	report.SourceSHA = *expected
	report.Invocation.Arguments = append([]string(nil), args...)
	encoder := xml.NewEncoder(output)
	encoder.Indent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	return encoder.Flush()
}

func main() {
	if err := runShortFunctions(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "Short-function inventory rejected:", err)
		os.Exit(1)
	}
}
