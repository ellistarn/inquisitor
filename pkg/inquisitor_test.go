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
	cycles := DetectCycles(packages)

	var buf bytes.Buffer
	GenerateReport(&buf, module, packages, functions, types_, cycles)
	report := buf.String()

	if !strings.Contains(report, "Cognitive Complexity") {
		t.Error("report missing Cognitive Complexity section")
	}
	if !strings.Contains(report, "Complex()") {
		t.Error("report does not mention Complex() in cognitive complexity section")
	}
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

	var found *Type
	for _, typ := range types_ {
		if typ.Name == "Service" {
			found = typ
			break
		}
	}
	if found == nil {
		t.Fatal("type Service not found in analysis results")
	}
	if found.LCOM4 != 2 {
		t.Errorf("expected LCOM4 = 2, got %d", found.LCOM4)
	}
	if len(found.Clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(found.Clusters))
	}

	// Verify the clusters contain the right methods.
	allMethods := make(map[string]bool)
	for _, cluster := range found.Clusters {
		for _, m := range cluster {
			allMethods[m] = true
		}
	}
	for _, expected := range []string{"GetDB", "SetDB", "GetCache", "SetCache"} {
		if !allMethods[expected] {
			t.Errorf("method %s not found in any cluster", expected)
		}
	}

	// Verify it appears in the cohesion report section.
	functions := AnalyzeFunctions(pkgs, analyzedPaths)
	module, packages := AnalyzePackages(pkgs, functions, types_, analyzedPaths)
	cycles := DetectCycles(packages)

	var buf bytes.Buffer
	GenerateReport(&buf, module, packages, functions, types_, cycles)
	report := buf.String()

	if !strings.Contains(report, "Cohesion") {
		t.Error("report missing Cohesion section")
	}
	if !strings.Contains(report, "Service") {
		t.Error("report does not mention Service in cohesion section")
	}
	if !strings.Contains(report, "Group 1:") || !strings.Contains(report, "Group 2:") {
		t.Error("report does not show two cluster groups")
	}
}

func TestDependencyCycles(t *testing.T) {
	tests := []struct {
		name       string
		packages   []*Package
		wantCycles int
		wantInPath []string // at least one cycle should contain all of these
	}{
		{
			name: "simple two-node cycle",
			packages: []*Package{
				{Path: "a", Imports: []string{"b"}},
				{Path: "b", Imports: []string{"a"}},
			},
			wantCycles: 1,
			wantInPath: []string{"a", "b"},
		},
		{
			name: "three-node cycle",
			packages: []*Package{
				{Path: "a", Imports: []string{"b"}},
				{Path: "b", Imports: []string{"c"}},
				{Path: "c", Imports: []string{"a"}},
			},
			wantCycles: 1,
			wantInPath: []string{"a", "b", "c"},
		},
		{
			name: "no cycle",
			packages: []*Package{
				{Path: "a", Imports: []string{"b"}},
				{Path: "b", Imports: []string{"c"}},
				{Path: "c", Imports: nil},
			},
			wantCycles: 0,
		},
		{
			name:       "empty graph",
			packages:   nil,
			wantCycles: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycles := DetectCycles(tt.packages)
			if len(cycles) != tt.wantCycles {
				t.Errorf("expected %d cycles, got %d: %v", tt.wantCycles, len(cycles), cycles)
			}
			if tt.wantInPath != nil && len(cycles) > 0 {
				cycle := cycles[0]
				cycleSet := make(map[string]bool)
				for _, p := range cycle {
					cycleSet[p] = true
				}
				for _, want := range tt.wantInPath {
					if !cycleSet[want] {
						t.Errorf("expected %q in cycle, got %v", want, cycle)
					}
				}
			}
		})
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
	cycles := DetectCycles(packages)

	var buf bytes.Buffer
	GenerateReport(&buf, module, packages, functions, types_, cycles)
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
		strings.Contains(report, "=== Coupling Between Objects") ||
		strings.Contains(report, "=== Dependency Cycles")
	if !hasFinding {
		t.Error("report contains no candidate sections")
	}

	// Architecture Balance should always appear since we have packages.
	if !strings.Contains(report, "=== Architecture Balance") {
		t.Error("report missing Architecture Balance section")
	}
}
