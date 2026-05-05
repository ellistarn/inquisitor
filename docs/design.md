# Inquisitor

A static analysis tool for Go codebases. Computes software engineering metrics and produces a
self-contained report showing where complexity lives.

```
inquisitor ./...
inquisitor ../other-project/pkg
inquisitor /absolute/path/to/module
inquisitor install
```

## Output

The report contains exactly these sections in this order: overview (module identity, glossary,
medians, package listing), then metric sections (cohesion, cognitive complexity, CBO,
architecture balance, tests — in that order).

The report opens with module identity, a glossary defining every abbreviation with citations and
action guidance, medians establishing what "normal" looks like, and a package listing:

```
github.com/example/project
  8 packages · 62 types · 278 functions · 18 tests · 11906 lines

  Glossary:
    cog     cognitive complexity (Campbell 2018) — measures reader difficulty by penalizing
            nesting depth. An if inside a for costs more than two flat structures. Functions
            exceeding cog:15 correlate with elevated defect rates. Decompose or flatten.

    cyc     cyclomatic complexity (McCabe 1976) — counts linearly independent execution paths.
            Higher values mean more test cases needed for coverage and more states a reader
            must track. Widely used as a maintenance risk indicator.

    fan_in  direct static call sites referencing this function. High fan_in means the
            function is heavily reused — changes to its contract are expensive. Low fan_in
            (especially fan_in:1) suggests the function may not justify its abstraction cost.

    fan_out distinct functions called from this function's body. High fan_out means the
            function coordinates many others — it's a point of high cognitive load and
            change sensitivity.

    LCOM4   lack of cohesion (Hitz & Montazeri 1995) — connected components in the method-
            field graph. LCOM4:1 means all methods work together through shared state.
            LCOM4 > 1 means the type contains independent method groups that don't interact —
            a sign it should be split. Higher values correlate with defect-proneness.

    CBO     coupling between objects (Chidamber & Kemerer 1994) — distinct types from other
            packages referenced through fields, parameters, return types, and method bodies.
            Basili et al. (1996) found CBO correlates with fault-proneness; practitioners
            use CBO > 5 as a threshold. Reduce by narrowing interfaces or splitting
            responsibilities.

    Ca      afferent coupling (Martin 2002) — packages depending on this one. High Ca means
            changes here ripple outward. Stable packages (high Ca, low Ce) should change
            rarely and define abstractions.

    Ce      efferent coupling (Martin 2002) — packages this one depends on. High Ce means
            this package is sensitive to changes elsewhere. Volatile packages (low Ca, high Ce)
            are expected to change often.

    I       instability (Martin 2002) — Ce/(Ca+Ce). 0 = maximally stable (everything depends
            on it, it depends on nothing). 1 = maximally volatile (nothing depends on it, it
            depends on everything). Dependencies should point toward stability.

    A       abstractness (Martin 2002) — interfaces / total types. Abstract packages define
            contracts without implementation. Martin's stable-abstractions principle: stable
            packages should be abstract so they can be extended without modification.

    D       distance (Martin 2002) — |A+I-1|. Measures deviation from the ideal: stable
            packages should be abstract, volatile packages should be concrete. D = 0 is
            ideal. High D suggests a package is either too concrete for its stability or
            too abstract for its volatility.

  Medians:
    package    Ce:1  18 exports  740 lines
    type       LCOM4:1  CBO:0  0 methods
    function   cog:3  cyc:4  fan_in:2  fan_out:4  14 lines
    test       cog:3  14 lines

  Packages:
    github.com/example/project/pkg/controller    5596 lines (47%)  Ca:1  Ce:4  I:0.80
    github.com/example/project/pkg/graph         2610 lines (22%)  Ca:1  Ce:2  I:0.67
    github.com/example/project/internal/util     1592 lines (13%)  Ca:4  Ce:0  I:0.00
    github.com/example/project/pkg/api            861 lines  (7%)  Ca:1  Ce:1  I:0.50
    github.com/example/project/internal/config    619 lines  (5%)  Ca:2  Ce:1  I:0.33
```

### Metric Sections

After the overview, the report shows metric sections. Each section fires when it has values to
report — sections with thresholds fire only when values exceed them, sections without thresholds
always fire. Threshold sections and Architecture Balance sort items by primary metric descending.
The Tests section groups by package (sorted by package path) with functions listed alphabetically
within each package.

| Section | Metric | Fires when | Citation |
|---|---|---|---|
| Cohesion | LCOM4 | > 1 | Hitz & Montazeri, 1995 |
| Cognitive Complexity | cog | > 15 | Campbell, SonarSource, 2018 |
| Coupling Between Objects | CBO | > 5 | Practitioner convention; Basili, Briand & Melo 1996 |
| Architecture Balance | D | always | Martin, 2002 |
| Tests | — | always | — |

```
=== Cognitive Complexity (Campbell 2018) ===
Reader difficulty. Penalizes nesting — an if inside a for inside a switch costs more
than three flat ifs. Measures the cost of holding nested context in working memory.
Threshold: cog > 15. Codebase median: 3.
Implies: decompose or simplify.

  github.com/example/project/pkg.ProcessItems()           cog:133  205 lines
  github.com/example/project/pkg.BuildGraph()             cog:104  200 lines
```

```
=== Cohesion — LCOM4 (Hitz & Montazeri 1995) ===
Connected method groups sharing a struct. LCOM4:1 = cohesive. LCOM4 > 1 = multiple
responsibilities sharing one type.
Implies: split into separate types, one per group.

  github.com/example/project/pkg.MyService                LCOM4:5  14 methods  CBO:4
    Group 1: method1, method2, method3
    Group 2: method4, method5, method6
    Group 3: method7, method8
    Group 4: method9
    Group 5: method10
```

```
=== Coupling Between Objects — CBO (Chidamber & Kemerer 1994) ===
Distinct types from other packages referenced through fields, parameters, return types,
and method bodies. Measures how entangled a type is with its environment.
Threshold: CBO > 5. Codebase median: 2.
Implies: narrow interfaces or split responsibilities.

  github.com/example/project/pkg.RequestHandler           CBO:12  LCOM4:3  8 methods
  github.com/example/project/pkg.EventBus                 CBO:9   LCOM4:1  5 methods
```

```
=== Architecture Balance (Martin 2002) ===
D = |A + I - 1| measures how far a package deviates from Martin's ideal: stable packages
should be abstract, volatile packages should be concrete. D = 0 is ideal.
No established threshold — evaluate against the designs.

  github.com/example/project/internal/util      D:1.00  A:0.00  I:0.00  (stable, concrete, zero interfaces)
  github.com/example/project/internal/config    D:0.67  A:0.00  I:0.33
  github.com/example/project/pkg/api            D:0.50  A:0.00  I:0.50
  github.com/example/project/pkg/graph          D:0.33  A:0.00  I:0.67  (volatile, concrete)
  github.com/example/project/pkg/controller     D:0.20  A:0.00  I:0.80  (volatile, concrete)
```

```
=== Tests ===
All test functions, grouped by package.

  github.com/example/project/pkg/controller
    TestReconcileCreate()            cog:5   32 lines
    TestReconcileUpdate()            cog:3   24 lines
    TestReconcileDelete()            cog:4   28 lines

  github.com/example/project/pkg/graph
    TestBuildAcyclic()               cog:8   45 lines
    TestCycleDetection()             cog:6   38 lines

  github.com/example/project/pkg/api
    TestHandleRequest()              cog:12  56 lines
    TestValidateInput()              cog:3   18 lines
```

## Scope

- **Default argument**: `./...` when no package patterns are given.
- **Test files**: Included. Prefers in-package test variant (`package foo` over `package foo_test`).
- **Test classification**: Functions named `Test*` in `_test.go` files. `Benchmark*` and `Fuzz*` are not classified as tests — they exercise the system with random or load-driven inputs that don't map to specific design concepts.
- **Test count**: Tests are counted separately from functions in the overview.
- **Tests section empty state**: When no test functions exist, the section still fires with the header and no entries.
- **Threshold sections**: Test functions still appear in threshold sections (cog > 15, LCOM4 > 1, CBO > 5) alongside production code. A test with cog:40 is Excess regardless of where it's listed — the threshold section is the canonical place to surface Excess.
- **Generated code**: Files with a `// Code generated` header are excluded.
- **Output**: stdout.
- **Parse errors**: If any package fails to load, the tool exits non-zero.
- **Interface method calls**: Excluded from fan_out (cannot be statically resolved).
- **Self-calls**: Excluded from fan_in and fan_out.
- **fan_in scope**: Only counts callers within the analyzed package set.
- **LCOM4**: Computed only for struct types with at least one method.
- **Types with zero methods**: Excluded from the cohesion threshold section.
- **CBO**: Distinct types from other packages referenced through fields, parameters, return types, and method bodies.
- **Install**: `inquisitor install` writes SKILL.md to `~/.agents/skills/inquisition/SKILL.md`, creates directories as needed, overwrites existing.
