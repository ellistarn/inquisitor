package inquisitor

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func GenerateReport(w io.Writer, mod *Module, packages []*Package, functions []*Function, types_ []*Type) {
	writeOverview(w, mod, packages, functions, types_)
	writeCohesionThreshold(w, types_)
	writeCognitiveThreshold(w, functions)
	writeCBOThreshold(w, types_)
	writeArchitectureBalance(w, packages)
	writeTests(w, functions)
}

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------

func writeOverview(w io.Writer, mod *Module, packages []*Package, functions []*Function, types_ []*Type) {
	// Count tests separately from functions
	testCount := 0
	for _, f := range functions {
		if f.IsTest {
			testCount++
		}
	}
	funcCount := len(functions) - testCount

	fmt.Fprintf(w, "%s\n", mod.Path)
	fmt.Fprintf(w, "  %d packages · %d types · %d functions · %d tests · %d lines\n", len(packages), len(types_), funcCount, testCount, mod.Lines)

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
			cogs[i] = f.Cog
			cycs[i] = f.Cyc
			fanIns[i] = f.FanIn
			fanOuts[i] = f.FanOut
			lines[i] = f.Lines
		}
		fmt.Fprintf(w, "    function   cog:%d  cyc:%d  fan_in:%d  fan_out:%d  %d lines\n", median(cogs), median(cycs), median(fanIns), median(fanOuts), median(lines))
	}

	// Test medians
	{
		var testFuncs []*Function
		for _, f := range functions {
			if f.IsTest {
				testFuncs = append(testFuncs, f)
			}
		}
		if len(testFuncs) > 0 {
			cogs := make([]int, len(testFuncs))
			lines := make([]int, len(testFuncs))
			for i, f := range testFuncs {
				cogs[i] = f.Cog
				lines[i] = f.Lines
			}
			fmt.Fprintf(w, "    test       cog:%d  %d lines\n", median(cogs), median(lines))
		}
	}

	// Package listing
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  Packages:")

	sorted := make([]*Package, len(packages))
	copy(sorted, packages)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Lines > sorted[j].Lines
	})

	names := make([]string, len(sorted))
	for i, p := range sorted {
		names[i] = p.Path
	}
	for i, p := range sorted {
		pct := 0
		if mod.Lines > 0 {
			pct = p.Lines * 100 / mod.Lines
		}
		fmt.Fprintf(w, "    %-*s  %d lines (%d%%)  Ca:%d  Ce:%d  I:%.2f\n",
			maxLen(names), names[i], p.Lines, pct, p.Ca, p.Ce, p.I)
	}
}

// ---------------------------------------------------------------------------
// Threshold Candidates
// ---------------------------------------------------------------------------

func writeCohesionThreshold(w io.Writer, types_ []*Type) {
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
			label: qualifiedTypeName(t),
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

func writeCognitiveThreshold(w io.Writer, functions []*Function) {
	var candidates []*Function
	for _, f := range functions {
		if f.Cog > 15 {
			candidates = append(candidates, f)
		}
	}
	if len(candidates) == 0 {
		return
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Cog > candidates[j].Cog
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
			label: qualifiedFuncName(f),
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
		fmt.Fprintf(w, "  %-*s  cog:%-5d %d lines\n",
			labelWidth, e.label,
			e.f.Cog,
			e.f.Lines)
	}
}

func writeCBOThreshold(w io.Writer, types_ []*Type) {
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
			label: qualifiedTypeName(t),
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

// ---------------------------------------------------------------------------
// Evidence Sections
// ---------------------------------------------------------------------------

func writeArchitectureBalance(w io.Writer, packages []*Package) {
	if len(packages) == 0 {
		return
	}

	sorted := make([]*Package, len(packages))
	copy(sorted, packages)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].D > sorted[j].D
	})

	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== Architecture Balance (Martin 2002) ===\n")
	fmt.Fprintf(w, "D = |A + I - 1| measures how far a package deviates from Martin's ideal: stable packages\n")
	fmt.Fprintf(w, "should be abstract, volatile packages should be concrete. D = 0 is ideal.\n")
	fmt.Fprintf(w, "No established threshold — evaluate against the designs.\n")
	fmt.Fprintln(w)

	names := make([]string, len(sorted))
	for i, p := range sorted {
		names[i] = p.Path
	}
	nameWidth := maxLen(names)

	for i, p := range sorted {
		desc := archDescription(p)
		fmt.Fprintf(w, "  %-*s  D:%.2f  A:%.2f  I:%.2f  (%s)\n",
			nameWidth, names[i], p.D, p.A, p.I, desc)
	}
}

func archDescription(p *Package) string {
	var parts []string

	if p.I < 0.5 {
		parts = append(parts, "stable")
	} else {
		parts = append(parts, "volatile")
	}

	if p.A == 0 {
		parts = append(parts, "concrete")
	} else if p.A < 1 {
		parts = append(parts, "partially abstract")
	} else {
		parts = append(parts, "fully abstract")
	}

	if p.A == 0 {
		parts = append(parts, "zero interfaces")
	}

	return strings.Join(parts, ", ")
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func writeTests(w io.Writer, functions []*Function) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "=== Tests ===\n")
	fmt.Fprintf(w, "All test functions, grouped by package.\n")

	// Collect test functions
	var tests []*Function
	for _, f := range functions {
		if f.IsTest {
			tests = append(tests, f)
		}
	}
	if len(tests) == 0 {
		return
	}

	// Group by package, sorted by package path
	byPkg := make(map[string][]*Function)
	for _, f := range tests {
		byPkg[f.Package] = append(byPkg[f.Package], f)
	}
	pkgPaths := make([]string, 0, len(byPkg))
	for p := range byPkg {
		pkgPaths = append(pkgPaths, p)
	}
	sort.Strings(pkgPaths)

	// Sort functions alphabetically within each package
	for _, p := range pkgPaths {
		sort.Slice(byPkg[p], func(i, j int) bool {
			return byPkg[p][i].Name < byPkg[p][j].Name
		})
	}

	// Compute label width for alignment
	type entry struct {
		label string
		f     *Function
	}
	var allEntries []entry
	for _, p := range pkgPaths {
		for _, f := range byPkg[p] {
			allEntries = append(allEntries, entry{label: funcDisplayName(f), f: f})
		}
	}
	labelWidth := 0
	for _, e := range allEntries {
		if len(e.label) > labelWidth {
			labelWidth = len(e.label)
		}
	}

	// Write grouped output
	for _, p := range pkgPaths {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s\n", p)
		for _, f := range byPkg[p] {
			label := funcDisplayName(f)
			fmt.Fprintf(w, "    %-*s  cog:%-4d %d lines\n", labelWidth, label, f.Cog, f.Lines)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

// qualifiedFuncName returns "full/import/path.FuncDisplay()" format.
func qualifiedFuncName(f *Function) string {
	return f.Package + "." + funcDisplayName(f)
}

// qualifiedTypeName returns "full/import/path.TypeName" format.
func qualifiedTypeName(t *Type) string {
	return t.Package + "." + t.Name
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
		vals[i] = f.Cog
	}
	return median(vals)
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


