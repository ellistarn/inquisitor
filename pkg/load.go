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

// resolveFilesystemPattern resolves a single filesystem path pattern into a
// module-relative import path pattern. It returns the rewritten pattern and
// the module root directory.
func resolveFilesystemPattern(pat string) (resolved string, modRoot string, err error) {
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
		return "", "", fmt.Errorf("resolving path %q: %w", pat, err)
	}

	// Verify it's a directory
	info, err := os.Stat(absDir)
	if err != nil {
		return "", "", fmt.Errorf("stat %q: %w", absDir, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%q is not a directory", absDir)
	}

	// Find module root
	modRoot, err = findModuleRoot(absDir)
	if err != nil {
		return "", "", fmt.Errorf("finding module root for %q: %w", pat, err)
	}

	// Compute relative path from module root to target
	rel, err := filepath.Rel(modRoot, absDir)
	if err != nil {
		return "", "", fmt.Errorf("computing relative path: %w", err)
	}

	// Rewrite as ./relative/... pattern
	if rel == "." {
		return "./...", modRoot, nil
	}
	if suffix == "" {
		suffix = "/..."
	}
	return "./" + filepath.ToSlash(rel) + suffix, modRoot, nil
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

		rewritten, modRoot, err := resolveFilesystemPattern(pat)
		if err != nil {
			return nil, "", err
		}

		// Ensure all filesystem patterns share the same module root
		if moduleRoot == "" {
			moduleRoot = modRoot
		} else if moduleRoot != modRoot {
			return nil, "", fmt.Errorf("patterns span multiple modules: %q and %q", moduleRoot, modRoot)
		}

		resolved = append(resolved, rewritten)
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
	if err := validatePackageErrors(pkgs); err != nil {
		return nil, err
	}
	deduped := deduplicateTestVariants(pkgs)

	var result []*packages.Package
	for _, pkg := range deduped {
		filterGeneratedFiles(pkg)
		if len(pkg.Syntax) == 0 {
			continue // Skip packages with no analyzable source (e.g., synthetic test binaries)
		}
		result = append(result, pkg)
	}
	return result, nil
}

// validatePackageErrors fails hard if any package has load errors.
func validatePackageErrors(pkgs []*packages.Package) error {
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				fmt.Fprintf(os.Stderr, "%s: %s\n", pkg.PkgPath, e)
			}
			return fmt.Errorf("package %s has errors", pkg.PkgPath)
		}
	}
	return nil
}

// deduplicateTestVariants resolves test package variants produced by
// go/packages when Tests is true. It keeps the augmented variant (which
// includes in-package test files) when both the plain and augmented variants
// exist for the same PkgPath.
func deduplicateTestVariants(pkgs []*packages.Package) []*packages.Package {
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
	result := make([]*packages.Package, 0, len(byPath))
	for _, pkg := range byPath {
		result = append(result, pkg)
	}
	return result
}

// filterGeneratedFiles removes generated files from the package's Syntax and
// CompiledGoFiles slices in place.
func filterGeneratedFiles(pkg *packages.Package) {
	var filtered []*ast.File
	var filteredFiles []string
	for i, f := range pkg.Syntax {
		if isGenerated(f) {
			continue
		}
		filtered = append(filtered, f)
		filteredFiles = append(filteredFiles, pkg.CompiledGoFiles[i])
	}
	pkg.Syntax = filtered
	pkg.CompiledGoFiles = filteredFiles
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
