# Context: Multi-Workspace Retrieval Orchestration

Phase: mw2-retrieval-orchestration
Status: ready
Spec Link: ../../multi-workspace-context.md
Roadmap Link: ../../ROADMAP-multi-workspace.md
Blast Radius: medium
Expected Proof: unit

## Goal
Wire the query parser and secondary resolver into the retrieval pipeline. Implement primary-first merge and add workspace provenance labels to `current-task.md` output.

## Scope Boundary
### Allowed Surfaces
- `src/core/retrieval.ts` — add `retrieveMultiWorkspaceContext()`
- `src/core/current-task.ts` — add workspace column + section labels
- `src/core/retrieval-ranking.ts` — extend `RankedRetrievalResult` with optional `workspace` field
- `tests/retrieval/current-task.test.ts` — extend with workspace label tests
- `tests/retrieval/retrieval.integration.test.ts` — extend with multi-workspace test

### Forbidden Surfaces
- `src/core/qmd-adapter.ts` — no changes (already supports any collection name)
- `src/schemas/config.ts` — done in phase 1
- `src/core/query-parser.ts` — done in phase 1
- Any evidence pipeline files
- Any CLI command files

## Spec Hooks
- R4 (primary-first merge), R5 (source labeling), R6 (backward compatible)
- R8 (multi-workspace query flow), R9 (total limit unchanged)
- I-8 (evidence pipeline unchanged)

## Locked Decisions
- `retrieveMultiWorkspaceContext()` is a new function, not a modification of `retrieveWorkspaceContext()` — existing callers untouched
- Merge algorithm: primary results take as many slots as they fill (up to total limit). Remaining = totalLimit - primary.length. Each secondary gets min(its own limit, remaining / number of secondaries) slots, allocated in config order.
- `RankedRetrievalResult` gains optional `workspace?: string` field — undefined means primary (backward compat for existing callers)
- `current-task.md` header adds `Secondary Workspaces: [list]` when secondaries are active
- Table format gains `Workspace` column only when secondaries are present (no visual change for single-workspace queries)

## Assumptions
- `QmdAdapter.searchWorkspace()` already accepts any workspace string — no adapter changes needed
- The existing `RetrievalAdapter` interface in `retrieval.ts` is sufficient for multi-workspace use (call it N times with different workspace names)
- Total limit default (8) is sufficient for primary + secondary combined

## Canonical Refs
- `src/core/retrieval.ts:16-35` — existing `retrieveWorkspaceContext`
- `src/core/current-task.ts:32-87` — markdown generation
- `src/core/retrieval-ranking.ts:33-50` — `rankRetrievalResults`
- `src/core/query-parser.ts` (from phase 1)
- `src/core/secondary-resolver.ts` (from phase 1)

## Rejected Options
- **Modifying existing `retrieveWorkspaceContext()`**: would risk breaking existing callers and make backward compat harder to prove. New function is cleaner.
- **Single merged qmd query across collections**: qmd doesn't support multi-collection queries. Would need adapter changes. Sequential per-workspace queries are simpler.
- **Always showing workspace column**: adds noise for single-workspace projects. Conditional column is a minor complexity worth the cleaner default output.

## Deferred Ideas
- Cross-workspace tier interleaving (P0 from secondary ranks above P2 from primary)
- Parallel qmd queries (Promise.all) — qmd is sync/subprocess anyway, so no benefit now

## Escalate If
- Existing `retrieveWorkspaceContext()` callers need the workspace field (indicates broader refactor needed)
- `current-task.md` format change breaks a known downstream consumer (need to identify and update)
- Secondary queries are noticeably slow (>100ms combined) — revisit parallel execution
