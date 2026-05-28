# Plan: Multi-Workspace Retrieval Orchestration

Phase: mw2-retrieval-orchestration
Status: ready
Wave Count: 3
Execution Owner: work
Updated At: 2026-05-28

## Goal
Wire query parser and secondary resolver into the retrieval pipeline with primary-first merge and workspace provenance labels.

## Inputs
- Phase 1 outputs: `src/core/query-parser.ts`, `src/core/secondary-resolver.ts`, extended `projectPointerSchema`
- `src/core/retrieval.ts` — existing retrieval function
- `src/core/current-task.ts` — markdown generation
- `src/core/retrieval-ranking.ts` — ranking types

## Wave 1
### T7 — Extend RankedRetrievalResult with workspace field
- type: implementation
- inputs:
  - `src/core/retrieval-ranking.ts`
- touches:
  - `src/core/retrieval-ranking.ts`
- avoid:
  - changing `rankRetrievalResults()` behavior (additive field only)
- steps:
  1. Add optional `workspace?: string` field to `RankedRetrievalResult` interface
- expected outputs:
  - `RankedRetrievalResult` has `workspace` field, existing usage unaffected (field is optional)
- verification:
  - `bun test tests/retrieval/ranking.test.ts` (existing tests still pass)
- stop if:
  - existing tests fail
- escalate to:
  - plan phase

## Wave 2
### T8 — Implement retrieveMultiWorkspaceContext()
- type: implementation
- inputs:
  - T7 output (extended type)
  - `src/core/query-parser.ts` (phase 1)
  - `src/core/secondary-resolver.ts` (phase 1)
  - `src/core/retrieval.ts`
- touches:
  - `src/core/retrieval.ts`
- avoid:
  - modifying `retrieveWorkspaceContext()` (keep it untouched)
  - `src/core/qmd-adapter.ts`
- steps:
  1. Import `parseQuery` from `query-parser.ts` and `resolveSecondaryWorkspaces` from `secondary-resolver.ts`
  2. Define `MultiWorkspaceRetrievalOptions` interface: `{ primaryWorkspace: string; query: string; secondaries: SecondaryWorkspaceEntry[]; workspacesDir: string; limit?: number }`
  3. Implement `retrieveMultiWorkspaceContext(paths, options, adapter)`:
     a. Call `parseQuery(query, secondaries)` → get `cleanQuery` and `secondaryWorkspaces`
     b. Call `resolveSecondaryWorkspaces(workspacesDir, secondaryWorkspaces)` → get resolved names
     c. Query primary workspace with `adapter.searchWorkspace({ workspace: primaryWorkspace, query: cleanQuery, limit })`
     d. Rank primary results with `rankRetrievalResults()`, tag each with `workspace: primaryWorkspace`
     e. Calculate remaining slots: `totalLimit - primary.length`
     f. For each resolved secondary (in order): query with `min(entry.limit, remainingSlots / remainingSecondaries)`, rank, tag with workspace name, append to results, subtract used slots
     g. Generate markdown via updated `generateCurrentTaskMarkdown()` (T9)
     h. Write `current-task.md` and return context
  4. Export `retrieveMultiWorkspaceContext` and `MultiWorkspaceRetrievalOptions`
- expected outputs:
  - New exported function in `src/core/retrieval.ts`
  - Existing `retrieveWorkspaceContext` unchanged
- verification:
  - `bun test tests/retrieval/retrieval.integration.test.ts`
- stop if:
  - merge logic has ambiguous slot allocation for edge cases (0 remaining, 1 remaining with 3 secondaries)
- escalate to:
  - user clarification (slot allocation rules)

### T9 — Update current-task.ts for workspace provenance
- type: implementation
- inputs:
  - T7 output (workspace field on results)
  - `src/core/current-task.ts`
- touches:
  - `src/core/current-task.ts`
- avoid:
  - breaking single-workspace output format when no secondaries present
- steps:
  1. Add optional `secondaryWorkspaces?: string[]` to `CurrentTaskInput` interface
  2. In `generateCurrentTaskMarkdown()`: if `secondaryWorkspaces` is non-empty, add `Secondary Workspaces: [list]` to header
  3. Conditionally add `Workspace` column to table when any result has `workspace` set and multiple distinct workspaces present
  4. In Full Context sections: prefix heading with `[workspace]` when multiple workspaces present — e.g., `### [ttdvkh] axioms/clean-arch.md (P0)`
  5. When only one workspace in results: produce identical output to current format (no visual change)
- expected outputs:
  - Updated `generateCurrentTaskMarkdown()` with conditional workspace labels
  - Single-workspace queries produce byte-identical output to current behavior
- verification:
  - `bun test tests/retrieval/current-task.test.ts`
- stop if:
  - single-workspace output differs from current (backward compat broken)
- escalate to:
  - plan phase

## Wave 3
### T10 — Unit tests for retrieval orchestration
- type: test
- inputs:
  - T8, T9 outputs
- touches:
  - `tests/retrieval/retrieval.integration.test.ts` (extend)
  - `tests/retrieval/current-task.test.ts` (extend)
- avoid:
  - full V1-V9 integration tests (those go in phase 3)
- steps:
  1. Add test: `retrieveMultiWorkspaceContext` with no secondaries configured → identical to single-workspace
  2. Add test: `retrieveMultiWorkspaceContext` with keyword trigger → secondary workspace queried
  3. Add test: `retrieveMultiWorkspaceContext` with @tag → secondary workspace queried, tag stripped from search
  4. Add test: primary fills all slots → no secondary results included
  5. Add test: `current-task.md` with multi-workspace results has workspace column and labels
  6. Add test: `current-task.md` with single workspace has no workspace column (backward compat)
- expected outputs:
  - 6+ new test cases passing
- verification:
  - `bun test tests/retrieval/`
- stop if:
  - mock adapter calls don't match expected workspace names
- escalate to:
  - plan phase

## Risks / Watch-fors
- Slot allocation with integer division: `remaining / remainingSecondaries` could be 0 if primary fills almost all slots. Floor to 0 means skip, which is correct per spec (primary-first).
- `generateCurrentTaskMarkdown` conditional logic must be tested for both paths (with/without secondaries) in every change.
