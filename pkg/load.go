package inquisitor

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
)

var generatedRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// isFilesystemPath reports whether a pattern looks like a filesystem path
// rather than a Go import path.
func isFilesystemPath(pattern string) bool {
	// Strip /... suffix for directory detection
	clean := strings.TrimSuffix(pattern, "/...")
	if strings.HasPrefix(clean, ".") || strings.HasPrefix(clean, "/") {
		return true
	}
	// Check if the resolved path is an existing directory on disk
	abs, err := filepath.Abs(clean)
	if err != nil {
		return false
	}
	info, err := os.Stat(abs)
	return err == nil && info.IsDir()
}

// findModuleRoot walks up from dir looking for a go.mod file and returns the
// directory containing it.
func findModuleRoot(dir string) (string, error) {
	orig, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	cur := orig
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("no go.mod found above %s", orig)
		}
		cur = parent
	}
}

// resolvePatterns rewrites filesystem path patterns into module-relative
// import path patterns and determines the appropriate Config.Dir.
// Returns the rewritten patterns and the module root directory (empty string
// if no filesystem paths were detected).
func resolvePatterns(patterns []string) ([]string, string, error) {
	var moduleRoot string
	resolved := make([]string, 0, len(patterns))

	for _, pat := range patterns {
		if !isFilesystemPath(pat) {
			resolved = append(resolved, pat)
			continue
		}

		// Separate /... suffix
		suffix := ""
		dir := pat
		if strings.HasSuffix(dir, "/...") {
			suffix = "/..."
			dir = strings.TrimSuffix(dir, "/...")
		}

		// Resolve to absolute path
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, "", fmt.Errorf("resolving path %q: %w", pat, err)
		}

		// Verify it's a directory
		info, err := os.Stat(absDir)
		if err != nil {
			return nil, "", fmt.Errorf("stat %q: %w", absDir, err)
		}
		if !info.IsDir() {
			return nil, "", fmt.Errorf("%q is not a directory", absDir)
		}

		// Find module root
		modRoot, err := findModuleRoot(absDir)
		if err != nil {
			return nil, "", fmt.Errorf("finding module root for %q: %w", pat, err)
		}

		// Ensure all filesystem patterns share the same module root
		if moduleRoot == "" {
			moduleRoot = modRoot
		} else if moduleRoot != modRoot {
			return nil, "", fmt.Errorf("patterns span multiple modules: %q and %q", moduleRoot, modRoot)
		}

		// Compute relative path from module root to target
		rel, err := filepath.Rel(modRoot, absDir)
		if err != nil {
			return nil, "", fmt.Errorf("computing relative path: %w", err)
		}

		// Rewrite as ./relative/... pattern
		if rel == "." {
			resolved = append(resolved, "./...") // module root itself
		} else {
			// If no /... suffix was given, add it (directory with no .go files)
			if suffix == "" {
				suffix = "/..."
			}
			resolved = append(resolved, "./"+filepath.ToSlash(rel)+suffix)
		}
	}

	if moduleRoot != "" {
		for _, p := range resolved {
			if !strings.HasPrefix(p, "./") && p != "./..." {
				return nil, "", fmt.Errorf("cannot mix filesystem paths with import path patterns (got %q)", p)
			}
		}
	}

	return resolved, moduleRoot, nil
}

// LoadPackages loads Go packages matching the given patterns, filtering out
// generated files from syntax trees. Test files are included.
func LoadPackages(patterns []string) ([]*packages.Package, error) {
	resolvedPatterns, moduleRoot, err := resolvePatterns(patterns)
	if err != nil {
		return nil, fmt.Errorf("resolving patterns: %w", err)
	}

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

	if moduleRoot != "" {
		cfg.Dir = moduleRoot
	}

	pkgs, err := packages.Load(cfg, resolvedPatterns...)
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
