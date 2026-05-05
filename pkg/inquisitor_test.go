package inquisitor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

// loadTestPackages loads Go packages from a temp directory using the same mode as the main tool.
func loadTestPackages(t *testing.T, dir string) []*packages.Package {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Tests: false,
		Dir:   dir,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("loading packages: %v", err)
	}
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			t.Fatalf("package %s: %s", pkg.PkgPath, e)
		}
	}
	return pkgs
}

// writeTempModule creates a go.mod and a single .go file in dir, returning the directory.
func writeTempModule(t *testing.T, moduleName, goSource string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+moduleName+"\ngo 1.23\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(goSource), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCognitiveComplexity(t *testing.T) {
	source := `package main

func Complex(items []int) int {
	result := 0
	for _, item := range items {           // +1 (nesting 0)
		if item > 0 {                       // +1+1 (nesting 1)
			for i := 0; i < item; i++ {     // +1+2 (nesting 2)
				switch {                     // +1+3 (nesting 3)
				case i%2 == 0:
					if i > 10 {              // +1+4 (nesting 4)
						result += i
					} else {                 // +1
						if i > 5 {           // +1+5 (nesting 5)
							result--
						}
					}
				case i%3 == 0:
					result += 2
				}
			}
		}
	}
	return result
}
`

	dir := writeTempModule(t, "test/cog", source)
	pkgs := loadTestPackages(t, dir)
	analyzedPaths := BuildAnalyzedPaths(pkgs)
	functions := AnalyzeFunctions(pkgs, analyzedPaths)

	var found *Function
	for _, f := range functions {
		if f.Name == "Complex" {
			found = f
			break
		}
	}
	if found == nil {
		t.Fatal("function Complex not found in analysis results")
	}
	if found.Cognitive <= 15 {
		t.Errorf("expected cognitive complexity > 15, got %d", found.Cognitive)
	}

	// Also verify it appears in the report's cognitive complexity section.
	types_ := AnalyzeTypes(pkgs, analyzedPaths)
	module, packages := AnalyzePackages(pkgs, functions, types_, analyzedPaths)

	var buf bytes.Buffer
	GenerateReport(&buf, module, packages, functions, types_)
	report := buf.String()

	if !strings.Contains(report, "Cognitive Complexity") {
		t.Error("report missing Cognitive Complexity section")
	}
	if !strings.Contains(report, "Complex()") {
		t.Error("report does not mention Complex() in cognitive complexity section")
	}
}

// findType returns the named type from the slice, or fails the test.
func findType(t *testing.T, types []*Type, name string) *Type {
	t.Helper()
	for _, typ := range types {
		if typ.Name == name {
			return typ
		}
	}
	t.Fatalf("type %s not found in analysis results", name)
	return nil
}

// clusterMethods returns the set of all method names across clusters.
func clusterMethods(clusters [][]string) map[string]bool {
	methods := make(map[string]bool)
	for _, cluster := range clusters {
		for _, m := range cluster {
			methods[m] = true
		}
	}
	return methods
}

func TestLCOM4(t *testing.T) {
	source := `package main

type Service struct {
	db    string
	cache string
}

func (s *Service) GetDB() string    { return s.db }
func (s *Service) SetDB(v string)   { s.db = v }
func (s *Service) GetCache() string { return s.cache }
func (s *Service) SetCache(v string) { s.cache = v }
`

	dir := writeTempModule(t, "test/lcom", source)
	pkgs := loadTestPackages(t, dir)
	analyzedPaths := BuildAnalyzedPaths(pkgs)
	types_ := AnalyzeTypes(pkgs, analyzedPaths)
	found := findType(t, types_, "Service")

	t.Run("metrics", func(t *testing.T) {
		if found.LCOM4 != 2 {
			t.Errorf("expected LCOM4 = 2, got %d", found.LCOM4)
		}
		if len(found.Clusters) != 2 {
			t.Errorf("expected 2 clusters, got %d", len(found.Clusters))
		}
		allMethods := clusterMethods(found.Clusters)
		for _, expected := range []string{"GetDB", "SetDB", "GetCache", "SetCache"} {
			if !allMethods[expected] {
				t.Errorf("method %s not found in any cluster", expected)
			}
		}
	})

	t.Run("report", func(t *testing.T) {
		functions := AnalyzeFunctions(pkgs, analyzedPaths)
		module, packages := AnalyzePackages(pkgs, functions, types_, analyzedPaths)

		var buf bytes.Buffer
		GenerateReport(&buf, module, packages, functions, types_)
		report := buf.String()

		for _, want := range []string{"Cohesion", "Service", "Group 1:", "Group 2:"} {
			if !strings.Contains(report, want) {
				t.Errorf("report missing expected string %q", want)
			}
		}
	})
}

// writeTempMultiPackageModule creates a go.mod and multiple .go files organized by package subdirectories.
// files is a map of relative path (e.g. "a/a.go") to source content.
func writeTempMultiPackageModule(t *testing.T, moduleName string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+moduleName+"\ngo 1.23\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runPipeline executes the full analysis pipeline on packages loaded from dir,
// returning the generated report text.
func runPipeline(t *testing.T, dir string) string {
	t.Helper()
	pkgs := loadTestPackages(t, dir)
	for _, pkg := range pkgs {
		filterGeneratedFiles(pkg)
	}
	analyzedPaths := BuildAnalyzedPaths(pkgs)
	functions := AnalyzeFunctions(pkgs, analyzedPaths)
	types_ := AnalyzeTypes(pkgs, analyzedPaths)
	module, packages := AnalyzePackages(pkgs, functions, types_, analyzedPaths)
	var buf bytes.Buffer
	GenerateReport(&buf, module, packages, functions, types_)
	return buf.String()
}

func TestFanInFanOut(t *testing.T) {
	// 5 functions: A calls B and C, B calls C. D is isolated, E is recursive.
	// Medians over 5 functions: fan_in values [0,0,0,1,2] → median=0, fan_out values [0,0,0,1,2] → median=0
	source := `package main

func A() { B(); C() }
func B() { C() }
func C() {}
func D() {}
func E() { E() }
`
	dir := writeTempModule(t, "test/fanio", source)
	report := runPipeline(t, dir)

	// The report must contain the module path and medians line with fan_in/fan_out.
	if !strings.Contains(report, "test/fanio") {
		t.Error("report missing module path 'test/fanio'")
	}
	if !strings.Contains(report, "fan_in:") {
		t.Error("report missing fan_in in medians")
	}
	if !strings.Contains(report, "fan_out:") {
		t.Error("report missing fan_out in medians")
	}
	// With 5 functions sorted: fan_in [0,0,0,1,2] median=0; fan_out [0,0,0,1,2] median=0
	if !strings.Contains(report, "fan_in:0") {
		t.Errorf("expected median fan_in:0 in report")
	}
	if !strings.Contains(report, "fan_out:0") {
		t.Errorf("expected median fan_out:0 in report")
	}
	// Verify the function count
	if !strings.Contains(report, "5 functions") {
		t.Error("report should list 5 functions")
	}
}

func TestCBOComputation(t *testing.T) {
	// Create a type with enough external type references (> 5) to trigger the CBO threshold section.
	source := `package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type BigService struct {
	file   *os.File
	w      io.Writer
	reader *bufio.Reader
	client *http.Client
	sb     *strings.Builder
}

func (s *BigService) Print(f fmt.Stringer) {
	fmt.Fprintln(s.w, f)
}

func (s *BigService) Open(name string) {
	s.file, _ = os.Open(name)
}

func (s *BigService) Read() {
	s.reader.ReadLine()
}

func (s *BigService) Do() {
	s.client.Get("http://example.com")
}

func (s *BigService) Build() {
	s.sb.WriteString("x")
}
`
	dir := writeTempModule(t, "test/cbo", source)
	report := runPipeline(t, dir)

	// CBO > 5 triggers the threshold section
	if !strings.Contains(report, "=== Coupling Between Objects") {
		t.Error("report missing CBO threshold section — expected BigService to trigger CBO > 5")
	}
	if !strings.Contains(report, "BigService") {
		t.Error("report CBO section should list BigService")
	}
	// The CBO value should appear in the listing
	if !strings.Contains(report, "CBO:") {
		t.Error("report CBO section should show CBO values")
	}
}

func TestPackageMetrics(t *testing.T) {
	// Package "a" imports "b". So b has Ca:1, a has Ce:1.
	// b: I = 0/(1+0) = 0.00, a: I = 1/(0+1) = 1.00
	files := map[string]string{
		"a/a.go": `package a

import "test/pkgmetrics/b"

func UseB() string { return b.Hello() }
`,
		"b/b.go": `package b

func Hello() string { return "hello" }
`,
	}
	dir := writeTempMultiPackageModule(t, "test/pkgmetrics", files)
	report := runPipeline(t, dir)

	// The Packages listing should show both packages with correct coupling values
	if !strings.Contains(report, "Ca:1") {
		t.Error("report should show Ca:1 for package b")
	}
	if !strings.Contains(report, "Ce:1") {
		t.Error("report should show Ce:1 for package a")
	}
	if !strings.Contains(report, "I:0.00") {
		t.Error("report should show I:0.00 for package b (maximally stable)")
	}
	if !strings.Contains(report, "I:1.00") {
		t.Error("report should show I:1.00 for package a (maximally volatile)")
	}
	// Both packages should be listed
	if !strings.Contains(report, "2 packages") {
		t.Error("report should list 2 packages")
	}
}

func TestInterfaceCallsExcludedFromFanOut(t *testing.T) {
	// UseGreeter only calls an interface method — fan_out should be 0.
	// With only 1 function and fan_out:0, the median should be fan_out:0.
	source := `package main

type Greeter interface {
	Greet() string
}

func UseGreeter(g Greeter) string {
	return g.Greet()
}
`
	dir := writeTempModule(t, "test/iface", source)
	report := runPipeline(t, dir)

	// The report should be produced successfully with 1 function
	if !strings.Contains(report, "1 functions") {
		t.Error("report should list 1 function")
	}
	// Median fan_out should be 0 (interface calls excluded)
	if !strings.Contains(report, "fan_out:0") {
		t.Error("expected median fan_out:0 — interface calls should not count as fan_out")
	}
}

func TestGeneratedFilesExcluded(t *testing.T) {
	// real.go has ~3 lines of function body, generated.go has ~3 lines but should be excluded.
	// The report's line count should reflect only the non-generated file.
	files := map[string]string{
		"real.go": `package main

func RealFunc() string { return "real" }
`,
		"generated.go": `// Code generated by tool; DO NOT EDIT.

package main

func GeneratedFunc() string { return "generated" }
`,
	}
	dir := writeTempMultiPackageModule(t, "test/genfiles", files)
	report := runPipeline(t, dir)

	// The report should mention the module
	if !strings.Contains(report, "test/genfiles") {
		t.Error("report missing module path")
	}
	// Only RealFunc should be analyzed (1 function)
	if !strings.Contains(report, "1 functions") {
		t.Error("expected 1 function — generated file should be excluded")
	}
	// GeneratedFunc should NOT appear anywhere in the report
	if strings.Contains(report, "GeneratedFunc") {
		t.Error("GeneratedFunc should be excluded from report (generated file)")
	}
}

func TestSelfAnalysis(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping self-analysis in short mode")
	}

	// Load the inquisitor's own packages from the current directory.
	pkgs, err := LoadPackages([]string{"./..."})
	if err != nil {
		t.Fatalf("loading packages: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages loaded")
	}

	analyzedPaths := BuildAnalyzedPaths(pkgs)
	functions := AnalyzeFunctions(pkgs, analyzedPaths)
	types_ := AnalyzeTypes(pkgs, analyzedPaths)
	module, packages := AnalyzePackages(pkgs, functions, types_, analyzedPaths)

	var buf bytes.Buffer
	GenerateReport(&buf, module, packages, functions, types_)
	report := buf.String()

	if len(report) == 0 {
		t.Fatal("report is empty")
	}

	// The overview should mention the module path.
	if !strings.Contains(report, "github.com/ellistarn/inquisitor") {
		t.Error("report does not contain module path 'github.com/ellistarn/inquisitor'")
	}

	// We should have analyzed some functions and types.
	if len(functions) == 0 {
		t.Error("no functions found in self-analysis")
	}
	if len(types_) == 0 {
		t.Error("no types found in self-analysis")
	}

	// At least one candidate section should appear (the tool has known cognitive complexity violations).
	hasFinding := strings.Contains(report, "=== Cognitive Complexity") ||
		strings.Contains(report, "=== Cohesion") ||
		strings.Contains(report, "=== Coupling Between Objects")
	if !hasFinding {
		t.Error("report contains no candidate sections")
	}

	// Architecture Balance should always appear since we have packages.
	if !strings.Contains(report, "=== Architecture Balance") {
		t.Error("report missing Architecture Balance section")
	}
}
