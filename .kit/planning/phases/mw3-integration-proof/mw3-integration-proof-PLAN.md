# Plan: Integration Proof

Phase: mw3-integration-proof
Status: ready
Wave Count: 2
Execution Owner: work
Updated At: 2026-05-28

## Goal
Prove all V1–V9 validation scenarios and confirm zero regressions.

## Inputs
- Phase 1 + Phase 2 outputs (all new modules and their unit tests passing)
- `.kit/planning/multi-workspace-context.md` — V1–V9 definitions
- `tests/retrieval/retrieval.integration.test.ts` — pattern reference

## Wave 1
### T11 — V1–V9 integration test suite
- type: test
- inputs:
  - `src/core/retrieval.ts` (retrieveMultiWorkspaceContext)
  - `src/core/query-parser.ts`
  - `src/core/secondary-resolver.ts`
  - `src/schemas/config.ts`
- touches:
  - `tests/retrieval/multi-workspace.integration.test.ts` (new file)
- avoid:
  - modifying any `src/` files
  - modifying existing test files
- steps:
  1. Create `tests/retrieval/multi-workspace.integration.test.ts`
  2. Setup: helper to create temp `workspacesDir` with named subdirectories
  3. V1 test: query with no keywords, no tags, no secondary config → single workspace, identical output
  4. V2 test: query hits keyword "file-storage" → primary + up to 3 framework-core results, total ≤ 8
  5. V3 test: query contains `@research` → tag stripped, research queried, results merged
  6. V4 test: keyword AND @tag for same workspace → queried once, not twice (assert adapter call count)
  7. V5 test: secondary workspace doesn't exist on disk → warning, skip, retrieval continues
  8. V6 test: primary returns 8 results → no secondary results (0 remaining slots)
  9. V7 test: primary returns 3, two secondaries triggered → remaining 5 slots split per limits
  10. V8 test: evidence pipeline functions → assert they don't accept or use secondary workspace config (type-level check or import verification)
  11. V9 test: zbrain.json without `secondary_workspaces` → works exactly as current behavior
- expected outputs:
  - 9 test cases, all passing
- verification:
  - `bun test tests/retrieval/multi-workspace.integration.test.ts`
- stop if:
  - any V scenario fails → indicates phase 2 bug or spec ambiguity
- escalate to:
  - plan phase (if spec ambiguity), phase 2 review (if implementation bug)

## Wave 2
### T12 — Full regression check
- type: test
- inputs:
  - all test files
- touches:
  - nothing (run only)
- avoid:
  - modifying any files
- steps:
  1. Run `bun test` (full suite)
  2. Verify all existing tests pass
  3. Verify no type errors: `bun run tsc --noEmit` (if configured) or `bunx tsc --noEmit`
- expected outputs:
  - full test suite green
  - no type errors
- verification:
  - `bun test` exit code 0
  - `bunx tsc --noEmit` exit code 0 (if tsconfig exists)
- stop if:
  - any existing test fails → phase 2 introduced a regression
- escalate to:
  - phase 2 review

## Risks / Watch-fors
- V7 proportional allocation: if limit math produces fractional slots, floor to integer. Test should assert exact counts.
- V4 deduplication: mock adapter must track call count per workspace to prove single invocation.
- V8 is a structural assertion — evidence pipeline code should not import or reference secondary workspace types. A grep-based check is acceptable.
