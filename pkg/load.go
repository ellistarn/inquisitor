package inquisitor

import (
	"fmt"
	"go/ast"
	"os"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
)

var generatedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// loadPackages loads Go packages matching the given patterns, filtering out
// generated files from syntax trees. Test files are included.
func LoadPackages(patterns []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedImports |
			packages.NeedDeps,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	// Fail hard on package errors.
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				fmt.Fprintf(os.Stderr, "%s: %s\n", pkg.PkgPath, e)
			}
			return nil, fmt.Errorf("package %s has errors", pkg.PkgPath)
		}
	}

	// With Tests: true, go/packages produces variants:
	//   "pkg" — production only
	//   "pkg [pkg.test]" — production + in-package test files
	//   "pkg_test [pkg.test]" — external test package
	// Keep the augmented variant (includes test files) when it exists.
	// Keep external test packages separately (they have a different PkgPath).
	byPath := map[string]*packages.Package{}
	for _, pkg := range pkgs {
		existing, ok := byPath[pkg.PkgPath]
		if !ok {
			byPath[pkg.PkgPath] = pkg
		} else {
			// Prefer the augmented variant (has .test] in ID)
			if strings.Contains(pkg.ID, ".test]") && !strings.Contains(existing.ID, ".test]") {
				byPath[pkg.PkgPath] = pkg
			}
		}
	}

	var result []*packages.Package
	for _, pkg := range byPath {
		// Filter syntax trees: remove generated files.
		var filtered []*ast.File
		var filteredFiles []string
		for i, f := range pkg.Syntax {
			filename := pkg.CompiledGoFiles[i]
			if isGenerated(f) {
				continue
			}
			filtered = append(filtered, f)
			filteredFiles = append(filteredFiles, filename)
		}
		if len(filtered) == 0 {
			continue // Skip packages with no analyzable source (e.g., synthetic test binaries)
		}
		pkg.Syntax = filtered
		pkg.CompiledGoFiles = filteredFiles
		result = append(result, pkg)
	}
	return result, nil
}

// isGenerated reports whether a file contains the Go generated-code marker.
// Per convention, the line must match: ^// Code generated .* DO NOT EDIT\.$
func isGenerated(f *ast.File) bool {
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if generatedRe.MatchString(c.Text) {
				return true
			}
		}
	}
	return false
}
