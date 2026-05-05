package inquisitor

import (
	"go/token"
	"go/types"
	"math"
	"sort"

	"golang.org/x/tools/go/packages"
)

// buildAnalyzedPaths returns the set of all package paths from the loaded packages.
func buildAnalyzedPaths(pkgs []*packages.Package) map[string]bool {
	paths := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		paths[pkg.PkgPath] = true
	}
	return paths
}

// Analyze runs the full analysis pipeline on the loaded packages, returning
// module, package, function, and type results.
func Analyze(pkgs []*packages.Package) (*Module, []*Package, []*Function, []*Type) {
	analyzedPaths := buildAnalyzedPaths(pkgs)
	functions := analyzeFunctions(pkgs, analyzedPaths)
	types := analyzeTypes(pkgs, analyzedPaths)
	mod, packages := analyzePackages(pkgs, functions, types, analyzedPaths)
	return mod, packages, functions, types
}

// analyzePackages computes package-level and module-level metrics for all loaded packages.
func analyzePackages(pkgs []*packages.Package, functions []*Function, types_ []*Type, analyzedPaths map[string]bool) (*Module, []*Package) {
	funcsByPkg, typesByPkg := indexByPackage(functions, types_)
	pkgMap, analyzed := buildPackages(pkgs, funcsByPkg, typesByPkg, analyzedPaths)
	computeAfferentCoupling(analyzed, pkgMap)
	computeStabilityMetrics(analyzed)
	mod := assembleModule(pkgs, analyzed)
	return mod, analyzed
}

// indexByPackage groups functions and types by their package path.
func indexByPackage(functions []*Function, types_ []*Type) (map[string][]*Function, map[string][]*Type) {
	funcsByPkg := make(map[string][]*Function)
	for _, f := range functions {
		funcsByPkg[f.Package] = append(funcsByPkg[f.Package], f)
	}
	typesByPkg := make(map[string][]*Type)
	for _, t := range types_ {
		typesByPkg[t.Package] = append(typesByPkg[t.Package], t)
	}
	return funcsByPkg, typesByPkg
}

// buildPackages constructs Package structs with efferent coupling (Ce),
// exported symbols, abstractness, and line counts.
func buildPackages(pkgs []*packages.Package, funcsByPkg map[string][]*Function, typesByPkg map[string][]*Type, analyzedPaths map[string]bool) (map[string]*Package, []*Package) {
	pkgMap := make(map[string]*Package)
	var analyzed []*Package
	for _, pkg := range pkgs {
		if !analyzedPaths[pkg.PkgPath] {
			continue
		}
		p := &Package{
			Name:      pkg.Name,
			Path:      pkg.PkgPath,
			Functions: funcsByPkg[pkg.PkgPath],
			Types:     typesByPkg[pkg.PkgPath],
		}

		// Imports — only internal (within analyzed set).
		for impPath := range pkg.Imports {
			if analyzedPaths[impPath] {
				p.Imports = append(p.Imports, impPath)
			}
		}
		sort.Strings(p.Imports)

		// Ce = number of internal imports.
		p.Ce = len(p.Imports)

		countExports(pkg, p)
		computeAbstractness(pkg, p)
		countLines(pkg, p)

		pkgMap[p.Path] = p
		analyzed = append(analyzed, p)
	}
	return pkgMap, analyzed
}

// countExports counts the number of exported symbols in the package scope.
func countExports(pkg *packages.Package, p *Package) {
	if pkg.Types == nil {
		return
	}
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		if token.IsExported(name) {
			p.ExportedSymbols++
		}
	}
}

// computeAbstractness computes the ratio of interfaces to total named types (Martin's A metric).
func computeAbstractness(pkg *packages.Package, p *Package) {
	if pkg.Types == nil {
		return
	}
	scope := pkg.Types.Scope()
	var totalNamed, ifaces int
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if tn, ok := obj.(*types.TypeName); ok {
			totalNamed++
			if _, isIface := tn.Type().Underlying().(*types.Interface); isIface {
				ifaces++
			}
		}
	}
	if totalNamed > 0 {
		p.A = float64(ifaces) / float64(totalNamed)
	}
}

// countLines sums lines across filtered syntax files for the package.
func countLines(pkg *packages.Package, p *Package) {
	for _, f := range pkg.Syntax {
		start := pkg.Fset.Position(f.Pos())
		end := pkg.Fset.Position(f.End())
		p.Lines += end.Line - start.Line + 1
	}
}

// computeAfferentCoupling counts how many other analyzed packages import each package (Ca).
func computeAfferentCoupling(analyzed []*Package, pkgMap map[string]*Package) {
	for _, p := range analyzed {
		for _, imp := range p.Imports {
			if target, ok := pkgMap[imp]; ok {
				target.Ca++
			}
		}
	}
}

// computeStabilityMetrics derives I and D from Ca, Ce, and A.
func computeStabilityMetrics(analyzed []*Package) {
	for _, p := range analyzed {
		total := p.Ca + p.Ce
		if total > 0 {
			p.I = float64(p.Ce) / float64(total)
		}
		p.D = math.Abs(p.A + p.I - 1)
	}
}

// assembleModule builds the Module aggregate from analyzed packages.
func assembleModule(pkgs []*packages.Package, analyzed []*Package) *Module {
	mod := &Module{
		Packages: analyzed,
	}
	if len(pkgs) > 0 && pkgs[0].Module != nil {
		mod.Path = pkgs[0].Module.Path
	} else if len(analyzed) > 0 {
		mod.Path = commonPrefix(analyzed)
	}
	for _, p := range analyzed {
		mod.Lines += p.Lines
	}
	return mod
}

// commonPrefix derives a module path from package paths by finding their longest common prefix.
func commonPrefix(pkgs []*Package) string {
	if len(pkgs) == 0 {
		return ""
	}
	prefix := pkgs[0].Path
	for _, p := range pkgs[1:] {
		for !hasPathPrefix(p.Path, prefix) {
			idx := lastSlash(prefix)
			if idx < 0 {
				return prefix
			}
			prefix = prefix[:idx]
		}
	}
	return prefix
}

func hasPathPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	if path[:len(prefix)] != prefix {
		return false
	}
	return len(path) == len(prefix) || path[len(prefix)] == '/'
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}
