package inquisitor

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func GenerateReport(w io.Writer, mod *Module, packages []*Package, functions []*Function, types_ []*Type, cycles [][]string) {
	pkgNames := disambiguatedNameMap(packages)
	writeOverview(w, mod, packages, functions, types_)
	writeCohesionCandidates(w, types_, pkgNames)
	writeCognitiveCandidates(w, functions, pkgNames)
	writeCBOCandidates(w, types_, pkgNames)
	writeCycleCandidates(w, cycles, packages, pkgNames)
	writeArchitectureBalance(w, packages, pkgNames)
}

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------

func writeOverview(w io.Writer, mod *Module, packages []*Package, functions []*Function, types_ []*Type) {
	fmt.Fprintf(w, "%s\n", mod.Path)
	fmt.Fprintf(w, "  %d packages · %d types · %d functions · %d lines\n", len(packages), len(types_), len(functions), mod.Lines)

	// Glossary
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Glossary:")
	fmt.Fprintln(w, "    cog     cognitive complexity (Campbell 2018) — measures reader difficulty by penalizing")
	fmt.Fprintln(w, "            nesting depth. An if inside a for costs more than two flat structures. Functions")
	fmt.Fprintln(w, "            exceeding cog:15 correlate with elevated defect rates. Decompose or flatten.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    cyc     cyclomatic complexity (McCabe 1976) — counts linearly independent execution paths.")
	fmt.Fprintln(w, "            Higher values mean more test cases needed for coverage and more states a reader")
	fmt.Fprintln(w, "            must track. Widely used as a maintenance risk indicator.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    fan_in  direct static call sites referencing this function. High fan_in means the")
	fmt.Fprintln(w, "            function is heavily reused — changes to its contract are expensive. Low fan_in")
	fmt.Fprintln(w, "            (especially fan_in:1) suggests the function may not justify its abstraction cost.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    fan_out distinct functions called from this function's body. High fan_out means the")
	fmt.Fprintln(w, "            function coordinates many others — it's a point of high cognitive load and")
	fmt.Fprintln(w, "            change sensitivity.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    LCOM4   lack of cohesion (Hitz & Montazeri 1995) — connected components in the method-")
	fmt.Fprintln(w, "            field graph. LCOM4:1 means all methods work together through shared state.")
	fmt.Fprintln(w, "            LCOM4 > 1 means the type contains independent method groups that don't interact —")
	fmt.Fprintln(w, "            a sign it should be split. Higher values correlate with defect-proneness.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    CBO     coupling between objects (Chidamber & Kemerer 1994) — distinct types from other")
	fmt.Fprintln(w, "            packages used through fields, parameters, and bodies. Basili et al. (1996) found")
	fmt.Fprintln(w, "            CBO correlates with fault-proneness; practitioners use CBO > 5 as a threshold.")
	fmt.Fprintln(w, "            Reduce by narrowing interfaces or splitting responsibilities.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    Ca      afferent coupling (Martin 2002) — packages depending on this one. High Ca means")
	fmt.Fprintln(w, "            changes here ripple outward. Stable packages (high Ca, low Ce) should change")
	fmt.Fprintln(w, "            rarely and define abstractions.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    Ce      efferent coupling (Martin 2002) — packages this one depends on. High Ce means")
	fmt.Fprintln(w, "            this package is sensitive to changes elsewhere. Volatile packages (low Ca, high Ce)")
	fmt.Fprintln(w, "            are expected to change often.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    I       instability (Martin 2002) — Ce/(Ca+Ce). 0 = maximally stable (everything depends")
	fmt.Fprintln(w, "            on it, it depends on nothing). 1 = maximally volatile (nothing depends on it, it")
	fmt.Fprintln(w, "            depends on everything). Dependencies should point toward stability.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    A       abstractness (Martin 2002) — interfaces / total types. Abstract packages define")
	fmt.Fprintln(w, "            contracts without implementation. Martin's stable-abstractions principle: stable")
	fmt.Fprintln(w, "            packages should be abstract so they can be extended without modification.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    D       distance (Martin 2002) — |A+I-1|. Measures deviation from the ideal: stable")
	fmt.Fprintln(w, "            packages should be abstract, volatile packages should be concrete. D = 0 is")
	fmt.Fprintln(w, "            ideal. High D suggests a package is either too concrete for its stability or")
	fmt.Fprintln(w, "            too abstract for its volatility.")

	// Medians
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Medians:")

	// Package medians
	{
		ces := make([]int, len(packages))
		exports := make([]int, len(packages))
		lines := make([]int, len(packages))
		for i, p := range packages {
			ces[i] = p.Ce
			exports[i] = p.ExportedSymbols
			lines[i] = p.Lines
		}
		fmt.Fprintf(w, "    package    Ce:%d  %d exports  %d lines\n", median(ces), median(exports), median(lines))
	}

	// Type medians
	{
		lcom4s := make([]int, len(types_))
		cbos := make([]int, len(types_))
		methods := make([]int, len(types_))
		for i, t := range types_ {
			lcom4s[i] = t.LCOM4
			cbos[i] = t.CBO
			methods[i] = t.Methods
		}
		fmt.Fprintf(w, "    type       LCOM4:%d  CBO:%d  %d methods\n", median(lcom4s), median(cbos), median(methods))
	}

	// Function medians
	{
		cogs := make([]int, len(functions))
		cycs := make([]int, len(functions))
		fanIns := make([]int, len(functions))
		fanOuts := make([]int, len(functions))
		lines := make([]int, len(functions))
		for i, f := range functions {
			cogs[i] = f.Cognitive
			cycs[i] = f.Cyclomatic
			fanIns[i] = f.FanIn
			fanOuts[i] = f.FanOut
			lines[i] = f.Lines
		}
		fmt.Fprintf(w, "    function   cog:%d  cyc:%d  fan_in:%d  fan_out:%d  %d lines\n", median(cogs), median(cycs), median(fanIns), median(fanOuts), median(lines))
	}

	// Package listing
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Packages:")

	sorted := make([]*Package, len(packages))
	copy(sorted, packages)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Lines > sorted[j].Lines
	})

	names := disambiguatedNames(sorted)
	for i, p := range sorted {
		pct := 0
		if mod.Lines > 0 {
			pct = p.Lines * 100 / mod.Lines
		}
		fmt.Fprintf(w, "    %-*s  %d lines (%d%%)  Ca:%d  Ce:%d  I:%.2f\n",
			maxLen(names), names[i], p.Lines, pct, p.Ca, p.Ce, p.Instability)
	}
}

// ---------------------------------------------------------------------------
// Threshold Candidates
// ---------------------------------------------------------------------------

func writeCohesionCandidates(w io.Writer, types_ []*Type, pkgNames map[string]string) {
	var candidates []*Type
	for _, t := range types_ {
		if t.LCOM4 > 1 {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LCOM4 > candidates[j].LCOM4
	})

	medLCOM4 := medianTypeLCOM4(types_)

	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== Cohesion — LCOM4 (Hitz & Montazeri 1995) ===\n")
	fmt.Fprintf(w, "Connected method groups sharing a struct. LCOM4:1 = cohesive. LCOM4 > 1 = multiple\n")
	fmt.Fprintf(w, "responsibilities sharing one type.\n")
	fmt.Fprintf(w, "Threshold: LCOM4 > 1. Codebase median: %d.\n", medLCOM4)
	fmt.Fprintf(w, "Implies: split into separate types, one per group.\n")
	fmt.Fprintln(w)

	// Compute column widths
	type entry struct {
		label string
		t     *Type
	}
	entries := make([]entry, len(candidates))
	for i, t := range candidates {
		entries[i] = entry{
			label: fmt.Sprintf("%s (%s)", t.Name, pkgNames[t.Package]),
			t:     t,
		}
	}
	labelWidth := 0
	for _, e := range entries {
		if len(e.label) > labelWidth {
			labelWidth = len(e.label)
		}
	}

	for _, e := range entries {
		fmt.Fprintf(w, "  %-*s  LCOM4:%d  %d methods  CBO:%d\n",
			labelWidth, e.label, e.t.LCOM4, e.t.Methods, e.t.CBO)
		for ci, cluster := range e.t.Clusters {
			fmt.Fprintf(w, "    Group %d: %s\n", ci+1, strings.Join(cluster, ", "))
		}
	}
}

func writeCognitiveCandidates(w io.Writer, functions []*Function, pkgNames map[string]string) {
	var candidates []*Function
	for _, f := range functions {
		if f.Cognitive > 15 {
			candidates = append(candidates, f)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Cognitive > candidates[j].Cognitive
	})

	medCog := medianFuncCognitive(functions)

	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== Cognitive Complexity (Campbell 2018) ===\n")
	fmt.Fprintf(w, "Reader difficulty. Penalizes nesting — an if inside a for inside a switch costs more\n")
	fmt.Fprintf(w, "than three flat ifs. Measures the cost of holding nested context in working memory.\n")
	fmt.Fprintf(w, "Threshold: cog > 15. Codebase median: %d.\n", medCog)
	fmt.Fprintf(w, "Implies: decompose or simplify.\n")
	fmt.Fprintln(w)

	type entry struct {
		label string
		f     *Function
	}
	entries := make([]entry, len(candidates))
	for i, f := range candidates {
		entries[i] = entry{
			label: funcDisplayName(f),
			f:     f,
		}
	}
	labelWidth := 0
	for _, e := range entries {
		if len(e.label) > labelWidth {
			labelWidth = len(e.label)
		}
	}

	for _, e := range entries {
		fmt.Fprintf(w, "  %-*s  cog:%-5d %-*s  %d lines\n",
			labelWidth, e.label,
			e.f.Cognitive,
			0, pkgNames[e.f.Package],
			e.f.Lines)
	}
}

func writeCBOCandidates(w io.Writer, types_ []*Type, pkgNames map[string]string) {
	var candidates []*Type
	for _, t := range types_ {
		if t.CBO > 5 {
			candidates = append(candidates, t)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CBO > candidates[j].CBO
	})

	medCBO := medianTypeCBO(types_)

	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== Coupling Between Objects — CBO (Chidamber & Kemerer 1994) ===\n")
	fmt.Fprintf(w, "Distinct types from other packages referenced through fields, parameters, return types,\n")
	fmt.Fprintf(w, "and method bodies. Measures how entangled a type is with its environment.\n")
	fmt.Fprintf(w, "Threshold: CBO > 5. Codebase median: %d.\n", medCBO)
	fmt.Fprintf(w, "Implies: reduce external type dependencies or split the type.\n")
	fmt.Fprintln(w)

	type entry struct {
		label string
		t     *Type
	}
	entries := make([]entry, len(candidates))
	for i, t := range candidates {
		entries[i] = entry{
			label: fmt.Sprintf("%s (%s)", t.Name, pkgNames[t.Package]),
			t:     t,
		}
	}
	labelWidth := 0
	for _, e := range entries {
		if len(e.label) > labelWidth {
			labelWidth = len(e.label)
		}
	}

	for _, e := range entries {
		fmt.Fprintf(w, "  %-*s  CBO:%-4d LCOM4:%d  %d methods\n",
			labelWidth, e.label, e.t.CBO, e.t.LCOM4, e.t.Methods)
	}
}

func writeCycleCandidates(w io.Writer, cycles [][]string, packages []*Package, pkgNames map[string]string) {
	if len(cycles) == 0 {
		return
	}

	// Sort by cycle length, then alphabetically by first element
	sorted := make([][]string, len(cycles))
	copy(sorted, cycles)
	sort.Slice(sorted, func(i, j int) bool {
		if len(sorted[i]) != len(sorted[j]) {
			return len(sorted[i]) < len(sorted[j])
		}
		// Compare alphabetically by joined short names
		a := cycleString(sorted[i])
		b := cycleString(sorted[j])
		return a < b
	})

	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== Dependency Cycles ===\n")
	fmt.Fprintf(w, "Cycles in the package import graph. Circular dependencies prevent independent change.\n")
	fmt.Fprintf(w, "Implies: break the cycle by extracting shared types into a new package or inverting a dependency.\n")
	fmt.Fprintln(w)

	for _, cycle := range sorted {
		short := make([]string, len(cycle))
		for i, p := range cycle {
			short[i] = pkgNames[p]
		}
		// Close the cycle by repeating the first element
		short = append(short, short[0])
		fmt.Fprintf(w, "  %s\n", strings.Join(short, " → "))
	}
}

// ---------------------------------------------------------------------------
// Evidence Sections
// ---------------------------------------------------------------------------

func writeArchitectureBalance(w io.Writer, packages []*Package, pkgNames map[string]string) {
	if len(packages) == 0 {
		return
	}

	sorted := make([]*Package, len(packages))
	copy(sorted, packages)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Distance > sorted[j].Distance
	})

	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== Architecture Balance (Martin 2002) ===\n")
	fmt.Fprintf(w, "D = |A + I - 1| measures how far a package deviates from Martin's ideal: stable packages\n")
	fmt.Fprintf(w, "should be abstract, volatile packages should be concrete. D = 0 is ideal.\n")
	fmt.Fprintf(w, "No established threshold — evaluate against the designs.\n")
	fmt.Fprintln(w)

	names := make([]string, len(sorted))
	for i, p := range sorted {
		names[i] = pkgNames[p.Path]
	}
	nameWidth := maxLen(names)

	for i, p := range sorted {
		desc := archDescription(p)
		fmt.Fprintf(w, "  %-*s  D:%.2f  A:%.2f  I:%.2f  (%s)\n",
			nameWidth, names[i], p.Distance, p.Abstractness, p.Instability, desc)
	}
}

func archDescription(p *Package) string {
	var parts []string

	if p.Instability < 0.5 {
		parts = append(parts, "stable")
	} else {
		parts = append(parts, "volatile")
	}

	if p.Abstractness == 0 {
		parts = append(parts, "concrete")
	} else if p.Abstractness < 1 {
		parts = append(parts, "partially abstract")
	} else {
		parts = append(parts, "fully abstract")
	}

	if p.Abstractness == 0 {
		parts = append(parts, "zero interfaces")
	}

	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func shortPkgName(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

func median(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := make([]int, len(values))
	copy(sorted, values)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}

func funcDisplayName(f *Function) string {
	if f.Receiver == "" {
		return f.Name + "()"
	}
	if f.PointerReceiver {
		return fmt.Sprintf("(*%s).%s()", f.Receiver, f.Name)
	}
	return fmt.Sprintf("%s.%s()", f.Receiver, f.Name)
}

func medianTypeLCOM4(types_ []*Type) int {
	vals := make([]int, len(types_))
	for i, t := range types_ {
		vals[i] = t.LCOM4
	}
	return median(vals)
}

func medianTypeCBO(types_ []*Type) int {
	vals := make([]int, len(types_))
	for i, t := range types_ {
		vals[i] = t.CBO
	}
	return median(vals)
}

func medianFuncCognitive(functions []*Function) int {
	vals := make([]int, len(functions))
	for i, f := range functions {
		vals[i] = f.Cognitive
	}
	return median(vals)
}

func cycleString(cycle []string) string {
	short := make([]string, len(cycle))
	for i, p := range cycle {
		short[i] = shortPkgName(p)
	}
	return strings.Join(short, " → ")
}

func maxLen(ss []string) int {
	m := 0
	for _, s := range ss {
		if len(s) > m {
			m = len(s)
		}
	}
	return m
}

// disambiguatedNames returns short package names for the given packages,
// using enough path components to make each name unique.
func disambiguatedNames(pkgs []*Package) []string {
	names := make([]string, len(pkgs))

	// Start with short names (1 component)
	for i, p := range pkgs {
		names[i] = shortPkgName(p.Path)
	}

	// Find duplicates and add path components until unique
	for {
		// Build map of name -> indices
		seen := map[string][]int{}
		for i, n := range names {
			seen[n] = append(seen[n], i)
		}

		resolved := true
		for _, indices := range seen {
			if len(indices) <= 1 {
				continue
			}
			resolved = false
			// Add one more path component for each duplicate
			for _, idx := range indices {
				names[idx] = addPathComponent(pkgs[idx].Path, names[idx])
			}
		}
		if resolved {
			break
		}
	}
	return names
}

// disambiguatedNameMap returns a map from package path to disambiguated short name.
func disambiguatedNameMap(pkgs []*Package) map[string]string {
	names := disambiguatedNames(pkgs)
	m := make(map[string]string, len(pkgs))
	for i, p := range pkgs {
		m[p.Path] = names[i]
	}
	return m
}

// addPathComponent prepends one more path component to the current short name.
func addPathComponent(fullPath, currentName string) string {
	// Find where currentName starts in fullPath
	suffix := "/" + currentName
	i := strings.LastIndex(fullPath, suffix)
	if i <= 0 {
		return fullPath // can't disambiguate further
	}
	// Find the component before the suffix
	prefix := fullPath[:i]
	lastSlash := strings.LastIndex(prefix, "/")
	if lastSlash < 0 {
		return fullPath
	}
	return fullPath[lastSlash+1:]
}
