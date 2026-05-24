# Phase Plan: P6 — Retrieval Pipeline

Inputs: P2 (workspace resolver), P4 (subagents + /ask command)
Depends on: P2, P4, qmd installed

---

## Wave 1: qmd Retrieval Module

### Task 1.1: qmd retrieval logic
- Create src/core/qmd-retrieval.ts
- Export: `PostFilterResult` type: { path: string, score: number, tier: 'P0'|'P1'|'P2'|'P3', snippet: string }
- Export: `classifyByTier(path: string): 'P0'|'P1'|'P2'|'P3'`
  - /axioms/* → P0
  - /mental-models/* → P1
  - /projects/* → P2
  - /decisions/* → P3
  - anything else → P2 (default)
- Export: `postFilterAndRank(results: QmdResult[]): PostFilterResult[]`
  - Classify each result by path prefix
  - Sort: P0 first, then P1, P2, P3 — within tier, keep original BM25 score order
  - Return top 8 results
- **Verification**: `bun test tests/core/qmd-retrieval.test.ts` — mixed results correctly sorted, P0 always first, unknown paths default to P2
- **Touched**: src/core/qmd-retrieval.ts, tests/core/qmd-retrieval.test.ts

---

## Wave 2: Current Task Writer

### Task 2.1: current-task.md generator
- Create src/core/current-task-writer.ts
- Export: `generateCurrentTask(query: string, workspace: string, results: PostFilterResult[], fullBodies: Map<string, string>): string`
- Output format:
  ```
  # Wiki Context — {query summary}
  Generated: {ISO datetime}
  Workspace: {workspace}
  Retrieval: qmd BM25 search

  ## Search Keywords
  {query}

  ## Retrieved Docs (by priority)
  ### P0 — Axioms
  | Score | File | Preview |
  | ... |

  ### P1 — Mental Models
  | ... |

  (empty tiers omitted)

  ## Full Context
  ### {file path} (P0)
  {full body for axioms, ~400 tokens for others}

  ## Knowledge Gaps
  {tiers with 0 results listed}
  ```
- **Verification**: `bun test tests/core/current-task-writer.test.ts` — generates valid markdown, empty tiers omitted, axioms get full body
- **Touched**: src/core/current-task-writer.ts, tests/core/current-task-writer.test.ts

---

## Wave 3: qmd Config Generation

### Task 3.1: qmd config generator
- Add to src/core/asset-extractor.ts (or new file src/core/qmd-config.ts):
- Export: `generateQmdConfig(workspaces: string[]): string`
  - Reads ~/.zwiki/workspaces/ directory
  - For each workspace, generates YAML collection entry with:
    - path: absolute path to workspace
    - pattern: '**/*.md'
    - ignore: evidence/sources/, evidence/qa/, evidence/applied/, evidence/archive/
    - context descriptions per subdirectory
    - includeByDefault: false
  - Writes to ~/.zwiki/engine/qmd-config.yml
- **Verification**: `bun test tests/core/qmd-config.test.ts` — generates valid YAML, paths are absolute, ignore list correct
- **Touched**: src/core/qmd-config.ts, tests/core/qmd-config.test.ts

---

## Wave 4: Subagent Refinement

### Task 4.1: Refine wiki-qmd-selector with post-filter spec
- Update assets/agents/wiki-qmd-selector.md:
  - Add explicit post-filter algorithm (from qmd-retrieval.ts logic)
  - Add current-task.md format specification
  - Add token budget: axioms full body, others ~400 tokens
  - Add workspace-scoped search rule: "ALWAYS specify collection parameter. Omitting collection = violation."
- **Verification**: Agent instructions match qmd-retrieval.ts logic exactly
- **Touched**: assets/agents/wiki-qmd-selector.md

### Task 4.2: Integration test script
- Create tests/retrieval-e2e.md — manual test script:
  1. Ensure programming workspace has: 1 axiom, 1 mental-model, 1 project entry
  2. `/ask "programming principle"` → verify axiom ranked first
  3. Switch to finance workspace → `/ask "programming principle"` → no results from programming
  4. `/ask "nonexistent topic"` → knowledge gap reported
- **Verification**: Test covers priority ordering, workspace isolation, knowledge gaps
- **Touched**: tests/retrieval-e2e.md

---

## Stop Conditions
- qmd MCP tools unavailable → verify qmd installation + MCP config
- Post-filter produces wrong ordering → debug with explicit BM25 scores

## Escalation
- qmd search returns unexpected format → inspect qmd docs / MCP response schema
- BM25 scores too similar across tiers → accept, post-filter still works by path prefix
