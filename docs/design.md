# Inquisitor

A static analysis tool for Go codebases. Analyzes the AST, computes classical software engineering
metrics, and produces a **findings report** — each finding states what was measured, what it
implies, and which code it applies to.

```
inquisitor ./path/to/packages/...
```

Inquisitor is the Inspect step of **Inspect-Audit-Simplify (IAS)** — a loop that converges on the
simplest implementation that satisfies the designs:

1. **Inspect** — Inquisitor analyzes the codebase, producing a findings report.
2. **Audit** — An agent reads the findings alongside the project's design documents and answers two
   questions: Does the code implement what the designs specify? Does the code implement only what
   the designs specify? No missing capabilities. No unnecessary complexity. Just as stated.
3. **Simplify** — The agent fixes unjustified findings. Independent findings can be fixed in
   parallel. Return to step 1.

The IAS loop is defined by the Inquisitor skill (`SKILL.md`). Inquisitor produces the findings report. The
skill produces judgment. The designs are the authority.

## Output

The output is a self-contained findings report to stdout. A reader should understand every section
without referencing this document.

### Overview

The report opens with the module identity, counts, and per-level medians establishing what "normal"
looks like in this codebase.

```
github.com/example/project
  8 packages · 62 types · 296 functions · 11906 lines

  Glossary:
    cog     cognitive complexity — reader difficulty, penalizes nesting (Campbell 2018)
    cyc     cyclomatic complexity — execution paths (McCabe 1976)
    Ca      afferent coupling — packages depending on this one (Martin 2002)
    Ce      efferent coupling — packages this one depends on (Martin 2002)
    I       instability — Ce/(Ca+Ce), 0=stable 1=volatile (Martin 2002)
    A       abstractness — interfaces / total types (Martin 2002)
    D       distance — |A+I-1|, stability-abstractness balance (Martin 2002)
    LCOM4   cohesion — connected method groups, 1=cohesive (Hitz & Montazeri 1995)
    CBO     coupling between objects — external types used (Chidamber & Kemerer 1994)
    fan_in  call sites referencing this function
    fan_out distinct functions called

  Medians:
    package    Ce:1  18 exports  740 lines
    type       LCOM4:1  CBO:0  0 methods
    function   cog:3  cyc:4  fan_in:2  14 lines

  Packages:
    pkg-a    5596 lines (47%)  Ca:1  Ce:4  I:0.80
    pkg-b    2610 lines (22%)  Ca:1  Ce:2  I:0.67
    pkg-c    1592 lines (13%)  Ca:4  Ce:0  I:0.00
    pkg-d     861 lines  (7%)  Ca:1  Ce:1  I:0.50
    pkg-e     619 lines  (5%)  Ca:2  Ce:1  I:0.33
```

The medians establish what "normal" looks like. The package listing shows what exists — size,
coupling direction, and instability. Metric-based finding sections show the relevant median for context.

### Findings

One section per test. Tests with research-backed thresholds report candidates that exceed the
threshold. Tests without established thresholds report evidence for the agent to evaluate against
the designs.

Sections only appear when findings or evidence exist.

#### Threshold Tests

These tests fire when a research-backed or definitional threshold is exceeded.

| Section                  | Threshold               | Citation                    | Implies                                           |
| ------------------------ | ----------------------- | --------------------------- | ------------------------------------------------- |
| Cohesion                 | LCOM4 > 1               | Hitz & Montazeri, 1995      | This type should be split. Method clusters shown. |
| Cognitive Complexity     | cog > 15                | Campbell, SonarSource, 2018 | This function is doing too much.                  |
| Coupling Between Objects | CBO > 5                 | Basili, Briand & Melo, 1996 | This type is entangled with too many others.      |
| Delegation               | body is single call     | Fowler, 1999 ("Middle Man") | Indirection without logic. Inline candidate.      |
| Dangling Exports         | zero cross-package refs | —                           | Unexport or remove.                               |
| Unmanaged Goroutines     | no lifecycle evidence   | —                           | No context, waitgroup, or channel management.     |
| Star Topology            | zero imports between non-hub packages      | —                  | Merge packages or introduce sibling dependencies that justify the boundaries. |
| Dependency Cycles        | cycle detected in import graph             | —                  | Circular dependencies prevent independent change.       |

#### Evidence

These sections report metric values for the agent to evaluate against the designs. No established
threshold exists — the agent decides what the designs justify.

| Section                 | What is reported                                               | Citation                       |
| ----------------------- | -------------------------------------------------------------- | ------------------------------ |
| Architecture Balance    | D for every package, sorted by D descending                    | Martin, 2002                   |
| Encapsulation           | Encapsulation ratio for every package, sorted ascending        | —                              |
| Single-Caller Functions | All functions with fan_in = 1                                  | Henry & Kafura, 1981 (concept) |
| Parameter Groups        | All parameter type tuples appearing in ≥ 2 function signatures | Fowler, 1999 ("Data Clump")    |

When a function qualifies for both Delegation and Single-Caller, only the Delegation finding is
shown. Delegation is the more specific signal — the function body is literally a single call, making
inline mechanical. Single-Caller is the weaker signal — low complexity and one caller suggests
inline but doesn't guarantee it.

Each section follows this structure:

```
=== [Test Name] ([Citation]) ===
[What this test measures]
[What a finding implies — what to DO about it]

  [candidate]  [evidence]  [location]
```

#### Cohesion Detail

When LCOM4 > 1, the output shows the actual method clusters — not just the count. The union-find
that computes connected components already identifies which methods belong to which cluster.

```
=== Cohesion — LCOM4 (Hitz & Montazeri 1995) ===
Connected method groups sharing a struct. LCOM4:1 = cohesive. LCOM4 > 1 = multiple
responsibilities sharing one type.
Implies: split into separate types, one per group.

  MyService (pkg-a)                              LCOM4:5  14 methods  CBO:4
    Group 1 [scope]: method1, method2, method3
    Group 2 [validation]: method4, method5, method6
    Group 3 [output]: method7, method8
    Group 4 [routing]: method9
    Group 5 [filtering]: method10
```

#### Context Annotations

The output shows the codebase median alongside each finding to provide context. A function with
cog:133 in a codebase where median cog is 3 tells a different story than cog:133 where median is 50.

```
=== Cognitive Complexity (Campbell 2018) ===
Reader difficulty. Penalizes nesting — an if inside a for inside a switch costs more
than three flat ifs. Measures the cost of holding nested context in working memory.
Threshold: cog > 15. Codebase median: 3.
Implies: decompose or simplify.

  ProcessItems()           cog:133  pkg-c     205 lines
  BuildGraph()             cog:104  pkg-e     200 lines
  HandleDeletion()         cog:78   pkg-a     174 lines
```

The codebase median appears in the section header, not as per-finding annotations.

#### Evidence Sections

Evidence sections report metric values without filtering. The agent evaluates each against the
designs.

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

=== Encapsulation ===
Fraction of a package's symbols that are hidden (unexported).
No established threshold — evaluate against the designs.

  pkg-c   encapsulation:0.00  13 exports  0 hidden  Ca:4
  pkg-d   encapsulation:0.15   7 exports  1 hidden  Ca:1
  pkg-a   encapsulation:0.35  12 exports  6 hidden  Ca:2

=== Single-Caller Functions ===
Functions with exactly one call site. Each adds a name and a jump.
Not all single-caller functions are problems — evaluate against the designs.

  12 of 62 functions (19%) have fan_in = 1
  pkg-a: helperOne, helperTwo, helperThree
  pkg-b: utilityA, utilityB

=== Parameter Groups ===
Parameter type tuples appearing in multiple function signatures. Recurring tuples
may indicate a missing struct.

  (string, string, string) in 5 functions
  (TypeA, TypeB) in 3 functions
  (context.Context, TypeC) in 2 functions
```

## Metrics Reference

Each metric has a formal definition and (where applicable) a citation. The output explains metrics
inline — this section is the reference for the formal foundations.

### Function Level

| Metric                      | Definition                                                                                          | Citation                    |
| --------------------------- | --------------------------------------------------------------------------------------------------- | --------------------------- |
| Cognitive complexity (cog)  | Reader difficulty with nesting penalties. Each control structure adds 1; nesting adds +1 per level. | Campbell, SonarSource, 2018 |
| Cyclomatic complexity (cyc) | Linearly independent paths. 1 + count of `if`, `case`, `for`, `&&`, `\|\|`.                         | McCabe, 1976                |
| Fan-in                      | Call sites within the analyzed codebase referencing this function.                                  | —                           |
| Fan-out                     | Distinct functions/methods called from this function's body.                                        | —                           |
| Parameters                  | Input parameter count.                                                                              | —                           |
| Lines                       | End line - start line + 1.                                                                          | —                           |

### Type Level

| Metric  | Definition                                                                                          | Citation                  |
| ------- | --------------------------------------------------------------------------------------------------- | ------------------------- |
| LCOM4   | Connected components in the method-field usage graph. Nodes = methods, edges = shared field access. | Hitz & Montazeri, 1995    |
| CBO     | Distinct external types referenced through fields, signatures, and bodies.                          | Chidamber & Kemerer, 1994 |
| Methods | Total methods (exported + unexported).                                                              | —                         |
| Fields  | Struct field count.                                                                                 | —                         |

### Package Level

| Metric                 | Definition                                                               | Citation     |
| ---------------------- | ------------------------------------------------------------------------ | ------------ |
| Afferent coupling (Ca) | Packages in the analyzed set that import this package.                   | Martin, 2002 |
| Efferent coupling (Ce) | Packages this package imports within the analyzed set.                   | Martin, 2002 |
| Instability (I)        | Ce / (Ca + Ce). 0 = stable, 1 = volatile.                                | Martin, 2002 |
| Abstractness (A)       | Interfaces / total types.                                                | Martin, 2002 |
| Distance (D)           | \|A + I - 1\|. Deviation from the ideal stability-abstractness tradeoff. | Martin, 2002 |
| Encapsulation ratio    | Unexported symbols / total symbols.                                      | —            |
| Exported symbols       | Public functions + types + methods + constants.                          | —            |
| Lines                  | Total source lines excluding tests.                                      | —            |

### Module Level

| Metric                | Definition                                         |
| --------------------- | -------------------------------------------------- |
| Packages              | Count of Go packages.                              |
| Internal dependencies | Import edges between packages in the analyzed set. |
| External dependencies | Third-party modules imported.                      |
| Lines                 | Total source lines.                                |

## Finding Selection

Inquisitor computes a complexity score internally to sort findings and evidence. Threshold tests use
the score for ordering candidates. Evidence sections use it for sort order but not for filtering —
all values are reported. The score is not shown in the output.

### Algorithm

Mean of percentile ranks across metrics, computed against all peers at the same level.

```
For every node at a given level:
  For each contributing metric:
    Rank all nodes by that metric (ascending)
    Assign percentile rank: rank / (n - 1)  →  [0.0, 1.0]
  Score = mean(percentile ranks)
```

### Contributing Metrics

| Level    | Metrics                                    | Rationale                                                      |
| -------- | ------------------------------------------ | -------------------------------------------------------------- |
| Function | cognitive, parameters, fan_out, fan_in     | Complexity, interface width, dependency reach, reuse evidence. |
| Type     | LCOM4, CBO, methods                        | Cohesion, coupling, surface area.                              |
| Package  | efferent_coupling, exported_symbols, lines | Outward dependency, API surface, scale.                        |

### Finding Thresholds

Threshold tests use thresholds from the defining research. Evidence sections have no threshold —
they report values for the agent to evaluate.

| Test                 | Threshold                                    | Basis                                                                             |
| -------------------- | -------------------------------------------- | --------------------------------------------------------------------------------- |
| Cohesion             | LCOM4 > 1                                    | Definitional — connected components in method-field graph (Hitz & Montazeri 1995) |
| Cognitive Complexity | cog > 15                                     | Author's recommendation, calibrated against McCabe's 10 (Campbell 2018)           |
| CBO                  | CBO > 5                                      | Empirical fault correlation (Basili, Briand & Melo 1996)                          |
| Delegation           | body is single call                          | Definitional — Fowler's "Middle Man" smell (1999)                                 |
| Dangling             | zero cross-package refs                      | Definitional                                                                      |
| Unmanaged Goroutines | no lifecycle evidence                        | Definitional                                                                      |
| Star Topology        | zero imports between non-hub packages        | Definitional                                                                      |
| Dependency Cycles    | cycle exists in the package import graph     | Definitional                                                                      |

Evidence sections (Architecture Balance, Encapsulation, Single-Caller, Parameter Groups) have no
threshold. The tool reports values. The agent evaluates against the designs.
