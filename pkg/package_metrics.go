package inquisitor

import (
	"go/token"
	"go/types"
	"math"
	"sort"

	"golang.org/x/tools/go/packages"
)

// buildAnalyzedPaths returns the set of all package paths from the loaded packages.
func BuildAnalyzedPaths(pkgs []*packages.Package) map[string]bool {
	paths := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		paths[pkg.PkgPath] = true
	}
	return paths
}

// analyzePackages computes package-level and module-level metrics for all loaded packages.
func AnalyzePackages(pkgs []*packages.Package, functions []*Function, types_ []*Type, analyzedPaths map[string]bool) (*Module, []*Package) {
	// Index functions and types by package path.
	funcsByPkg := make(map[string][]*Function)
	for _, f := range functions {
		funcsByPkg[f.Package] = append(funcsByPkg[f.Package], f)
	}
	typesByPkg := make(map[string][]*Type)
	for _, t := range types_ {
		typesByPkg[t.Package] = append(typesByPkg[t.Package], t)
	}

	// Build analyzed packages.
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

		// Abstractness and ExportedSymbols from the type-checker scope.
		if pkg.Types != nil {
			scope := pkg.Types.Scope()
			var totalNamed, ifaces int
			for _, name := range scope.Names() {
				obj := scope.Lookup(name)
				if token.IsExported(name) {
					p.ExportedSymbols++
				}
				if tn, ok := obj.(*types.TypeName); ok {
					totalNamed++
					if _, isIface := tn.Type().Underlying().(*types.Interface); isIface {
						ifaces++
					}
				}
			}
			if totalNamed > 0 {
				p.Abstractness = float64(ifaces) / float64(totalNamed)
			}
		}

		// Lines — sum across filtered syntax files.
		for _, f := range pkg.Syntax {
			start := pkg.Fset.Position(f.Pos())
			end := pkg.Fset.Position(f.End())
			p.Lines += end.Line - start.Line + 1
		}

		pkgMap[p.Path] = p
		analyzed = append(analyzed, p)
	}

	// Ca — count how many other packages import each package.
	for _, p := range analyzed {
		for _, imp := range p.Imports {
			if target, ok := pkgMap[imp]; ok {
				target.Ca++
			}
		}
	}

	// Instability and Distance.
	for _, p := range analyzed {
		total := p.Ca + p.Ce
		if total > 0 {
			p.Instability = float64(p.Ce) / float64(total)
		}
		p.Distance = math.Abs(p.Abstractness + p.Instability - 1)
	}

	// Module.
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

	return mod, analyzed
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

// detectCycles finds dependency cycles in the package import graph using DFS.
// Each cycle is a slice of package paths. Cycles are sorted by length, then alphabetically.
func DetectCycles(packages []*Package) [][]string {
	// Build adjacency list.
	adj := make(map[string][]string)
	for _, p := range packages {
		adj[p.Path] = p.Imports
	}

	const (
		unvisited  = 0
		inProgress = 1
		done       = 2
	)
	state := make(map[string]int)
	parent := make(map[string]string) // for path reconstruction
	var cycles [][]string
	seen := make(map[string]bool) // deduplicate cycles by canonical form

	var dfs func(node string)
	dfs = func(node string) {
		state[node] = inProgress
		for _, next := range adj[node] {
			switch state[next] {
			case unvisited:
				parent[next] = node
				dfs(next)
			case inProgress:
				// Back edge found — extract cycle.
				cycle := extractCycle(parent, node, next)
				key := canonicalCycleKey(cycle)
				if !seen[key] {
					seen[key] = true
					cycles = append(cycles, cycle)
				}
			}
		}
		state[node] = done
	}

	// Collect and sort nodes for deterministic output.
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	for _, n := range nodes {
		if state[n] == unvisited {
			dfs(n)
		}
	}

	// Sort cycles: by length, then lexicographically.
	sort.Slice(cycles, func(i, j int) bool {
		if len(cycles[i]) != len(cycles[j]) {
			return len(cycles[i]) < len(cycles[j])
		}
		for k := 0; k < len(cycles[i]) && k < len(cycles[j]); k++ {
			if cycles[i][k] != cycles[j][k] {
				return cycles[i][k] < cycles[j][k]
			}
		}
		return false
	})

	return cycles
}

// extractCycle traces back from current to target through the parent map to build the cycle path.
func extractCycle(parent map[string]string, current, target string) []string {
	var path []string
	visited := make(map[string]bool)
	node := current
	for node != target {
		if visited[node] {
			break // safety: should never happen, but prevents infinite loop
		}
		visited[node] = true
		path = append(path, node)
		node = parent[node]
	}
	path = append(path, target)
	// Reverse so cycle reads in traversal order.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

// canonicalCycleKey produces a canonical string for a cycle so rotations are deduplicated.
func canonicalCycleKey(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	// Find the lexicographically smallest rotation.
	minIdx := 0
	for i := 1; i < len(cycle); i++ {
		if cycle[i] < cycle[minIdx] {
			minIdx = i
		}
	}
	var b []byte
	for i := 0; i < len(cycle); i++ {
		if i > 0 {
			b = append(b, ' ')
		}
		b = append(b, cycle[(minIdx+i)%len(cycle)]...)
	}
	return string(b)
}
