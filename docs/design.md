# Inquisitor

A static analysis tool for Go codebases. Computes software engineering metrics and produces a
self-contained report showing where complexity lives. The skill (`SKILL.md`) uses this report to
defend the project — finding missing implementations, extra implementations, and poor architecture
by comparing where complexity IS against where designs say it SHOULD be.

```
inquisitor ./path/to/packages/...
```

## Reading the Report

The report shows where complexity lives. Three signals — compared against each other — reveal what
needs to change.

**Designs** — what the code should be. The intended architecture, in the project's design
documents. The tool doesn't know the designs — the agent brings that context.

**Names** — the code's vocabulary. Package names, type names, function names. They map code to
design concepts. If the design says "connection pool" and the code has `ResourceManager`, that's
already a signal — either the vocabulary diverges (confusion) or the concepts differ (mismatch).

**Metrics** — the code's actual structure. They reveal what role each piece plays regardless of what
it's called or what it's meant to be:

| Level | Metric signature | Role |
|-------|-----------------|------|
| Package | Ca:high Ce:low I:low | Library — stable foundation, changes rarely |
| Package | Ca:low Ce:high I:high | Leaf module — implementation, complexity lives here |
| Package | Ca:high Ce:high | Hub — everything flows through it, often a problem |
| | | |
| Type | LCOM4:1 | One job |
| Type | LCOM4:N | N independent jobs — design should name each |
| | | |
| Function | fan_out:high cog:low | Coordinator — orchestrates without deciding |
| Function | fan_out:high cog:high | Overloaded — coordinates AND computes, almost never justified |
| Function | fan_in:high | Utility — heavily reused, must be simple |
| Function | cog:high fan_out:low | Calculator — complex but self-contained |

"High" and "low" are relative to the codebase medians shown in the report. A package with Ce:4 in a
codebase where median Ce is 1 has high efferent coupling.

All three align in healthy code. Mismatches between any pair are the signal:

- **Design says X, metrics show Y** — The design describes a stable persistence layer but the package has I:0.95 (volatile). Intent doesn't match structure.
- **Design says X, names say Y** — The design describes "event routing" but the code calls it `DataProcessor`. Vocabulary drift.
- **Names and metrics align, but no design covers it** — A cohesive, well-structured package named `cache` — but no design mentions caching. Code nobody asked for.

These mismatches map to three problems:
- Design describes behavior but no complexity exists for it → **missing implementation**
- Complexity exists but no design describes it → **extra implementation**
- Complexity exists and maps to a design concept, but the structure is wrong → **poor architecture**

**Proportionality** — complexity is justified when proportional to what the design describes.
The design says "handles 10 protocol states" and the function has cog:40 → 4 per state, fine.
The design says "wraps HTTP responses" and the function has cog:40 → 40x what's needed.

**Cross-level insight** — Ca/Ce tell you the package's role. fan_in tells you which functions in
that package ARE the contract others depend on. Together: "this is a library (Ca:4) and these 2
functions (fan_in:8, fan_in:6) are its real interface — the other 18 functions are internal
machinery." This means: simplifying internals is cheap (nothing external uses them). Changing
high-fan_in functions is expensive (everything uses them). When the design says "simplify this
package," the metrics tell you where to cut safely.

**Worked example** — A type `OrderProcessor` in `pkg-a` (Ca:3, I:0.25) shows LCOM4:3, CBO:8, 12
methods. The design document says: "OrderProcessor validates, prices, and fulfills orders." Three
design-described operations → three method clusters → LCOM4:3 is explained. CBO:8 with three
integration points (validation rules, pricing engine, fulfillment service) → ~2.7 types per
integration. The metrics match the design's described scope. Verdict: justified.

## Output

The report opens with module identity, a glossary defining every abbreviation with citations and
action guidance, medians establishing what "normal" looks like, and a package listing:

```
github.com/example/project
  8 packages · 62 types · 296 functions · 11906 lines

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
            packages used through fields, parameters, and bodies. Basili et al. (1996) found
            CBO correlates with fault-proneness; practitioners use CBO > 5 as a threshold.
            Reduce by narrowing interfaces or splitting responsibilities.

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

  Packages:
    pkg-a    5596 lines (47%)  Ca:1  Ce:4  I:0.80
    pkg-b    2610 lines (22%)  Ca:1  Ce:2  I:0.67
    pkg-c    1592 lines (13%)  Ca:4  Ce:0  I:0.00
    pkg-d     861 lines  (7%)  Ca:1  Ce:1  I:0.50
    pkg-e     619 lines  (5%)  Ca:2  Ce:1  I:0.33
```

### Threshold Sections

Sections that fire when a value exceeds a research-backed or practitioner-established limit.
All sections sort items by primary metric descending.

| Section | Threshold | Citation |
|---|---|---|
| Cohesion | LCOM4 > 1 | Hitz & Montazeri, 1995 |
| Cognitive Complexity | cog > 15 | Campbell, SonarSource, 2018 |
| Coupling Between Objects | CBO > 5 | Practitioner convention; Basili, Briand & Melo 1996 |

```
=== Cognitive Complexity (Campbell 2018) ===
Reader difficulty. Penalizes nesting — an if inside a for inside a switch costs more
than three flat ifs. Measures the cost of holding nested context in working memory.
Threshold: cog > 15. Codebase median: 3.
Implies: decompose or simplify.

  ProcessItems()           cog:133  pkg-c     205 lines
  BuildGraph()             cog:104  pkg-e     200 lines
```

```
=== Cohesion — LCOM4 (Hitz & Montazeri 1995) ===
Connected method groups sharing a struct. LCOM4:1 = cohesive. LCOM4 > 1 = multiple
responsibilities sharing one type.
Implies: split into separate types, one per group.

  MyService (pkg-a)                              LCOM4:5  14 methods  CBO:4
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

  RequestHandler (pkg-a)   CBO:12  LCOM4:3  8 methods
  EventBus (pkg-b)         CBO:9   LCOM4:1  5 methods
```

### Evidence Sections

Sections that report values without filtering for the agent to evaluate against the designs.

```
=== Architecture Balance (Martin 2002) ===
D = |A + I - 1| measures how far a package deviates from Martin's ideal: stable packages
should be abstract, volatile packages should be concrete. D = 0 is ideal.
No established threshold — evaluate against the designs.

  pkg-c   D:1.00  A:0.00  I:0.00  (stable, concrete, zero interfaces)
  pkg-a   D:0.20  A:0.00  I:0.80  (volatile, concrete)
  pkg-b   D:0.33  A:0.00  I:0.67  (volatile, concrete)
  pkg-d   D:0.50  A:0.00  I:0.50
  pkg-e   D:0.67  A:0.00  I:0.33
```

## Scope

- **Generated code**: Files with a `// Code generated` header are excluded.
- **Parse errors**: If any package fails to load, the tool exits with an error.
- **Exit code**: 0 on success. Non-zero if packages fail to load.
