# Inquisitor

This skill defines the **Inspect-Audit-Simplify (IAS)** loop — a cycle that converges on the simplest implementation that satisfies the designs.

## 1. Inspect

Run Inquisitor:

```bash
cd .skills/inquisitor && go run . ../../path/to/packages/...
```

The output is self-contained. Each section explains what it measures and what findings imply.

## 2. Audit

Your role is **design advocate**. The designs are the authority. The code is on trial.

Two questions drive the audit:

1. **Does the code implement what the designs specify?** Missing capabilities are findings.
2. **Does the code implement only what the designs specify?** Unnecessary complexity is findings.

#### Read the designs first

Read ALL design documents before reading the tool output. The designs are the lens — you need the mental model before you see the evidence.

#### Build the concept map

Map designs to code using the package overview at the top of the output. For each design document, identify which packages implement it. For each finding, identify which design section describes the behavior.

Output the map as a table. Findings with no design backing and design concepts with no corresponding code are both audit findings.

#### Evaluate threshold findings

The tool's threshold tests have research-backed thresholds. Each candidate in these sections has already been flagged by the tool as exceeding a known limit. For each:

- Does the design justify the exception? Cite the specific design text.
- If no design text justifies it, verdict is UNJUSTIFIED.

#### Evaluate evidence

The tool's evidence sections report values without filtering — no established threshold exists. For each:

- Do the reported values align with the designs? A package with encapsulation:0.00 may be fine (pure data types) or a problem (should be hiding internals). The design decides.
- Compare the values to the design's structure. If the design describes 3 concepts and the code has 8 packages, the mismatch is a finding regardless of encapsulation ratios.

#### Produce verdicts

For every finding or evidence item worth reporting, produce:

```
### [node name] — [VERDICT]

**Finding**: [exact line from the tool output]
**Design section**: [document name] § [section heading], or "None"
**Design says**: "[exact quote, max 2 sentences]" or "Not in any design document."
**The gap**: [why the code exceeds what the design requires. Cite evidence and design language. No hedging.]
```

#### Verdicts

Exactly one of three.

- **UNJUSTIFIED**: The code is more complex than the design requires.
- **MISSING FROM DESIGN**: The code implements behavior no design document describes. Either the design needs updating or the code needs removing.
- **JUSTIFIED**: A specific design requirement demands this. Cite it. Brief.

#### Ordering

1. UNJUSTIFIED — worst first
2. MISSING FROM DESIGN
3. JUSTIFIED — brief

#### Constraints

You must not:
- Say "complex but necessary" without citing design text.
- Say "handles edge cases" without naming them and checking whether they appear in the design.
- Say "could potentially be simplified" — commit to a verdict.
- Assume the code is correct. The design is correct.

You must:
- Start every verdict from a line in the tool output.
- Quote the design when the code should match it.
- Quote the design's absence when the code has no design backing.
- When a type has LCOM4 > 1, name the clusters and what separate types they suggest.
- When a function exceeds cog > 15, identify which decisions in the function body the design does not describe.

## 3. Simplify

Fix unjustified findings. Independent findings can be fixed in parallel.

Return to step 1. The loop converges when every finding is justified by the designs.
