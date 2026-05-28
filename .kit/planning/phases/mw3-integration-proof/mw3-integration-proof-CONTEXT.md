# Context: Integration Proof

Phase: mw3-integration-proof
Status: ready
Spec Link: ../../multi-workspace-context.md
Roadmap Link: ../../ROADMAP-multi-workspace.md
Blast Radius: low
Expected Proof: integration

## Goal
Prove all 9 validation scenarios (V1–V9) from the spec pass as integration tests, and confirm zero regressions in the existing test suite.

## Scope Boundary
### Allowed Surfaces
- `tests/retrieval/multi-workspace.integration.test.ts` (new file)
- Existing test files (run only, not modify)

### Forbidden Surfaces
- All `src/` files — no implementation changes in this phase
- Any evidence pipeline files
- Any CLI command files

## Spec Hooks
- All validation expectations V1–V9
- R6 (backward compatible)
- I-8 (evidence pipeline unchanged)

## Locked Decisions
- Integration tests use injected `RetrievalAdapter` (same pattern as existing `retrieval.integration.test.ts`) — no real qmd dependency
- Each V scenario is one dedicated test case with descriptive name
- Temp directories for `workspacesDir` with real directory creation to test `resolveSecondaryWorkspaces`

## Assumptions
- Phase 1 and Phase 2 are complete and their unit tests pass
- Existing test suite is green before this phase starts

## Canonical Refs
- `tests/retrieval/retrieval.integration.test.ts` — pattern reference
- `.kit/planning/multi-workspace-context.md` — V1–V9 scenarios

## Rejected Options
- **E2E tests with real qmd**: adds external dependency to test suite. Injected adapter is sufficient for proving orchestration logic.
- **Snapshot tests for current-task.md**: brittle on formatting changes. Explicit assertions on content are more maintainable.

## Deferred Ideas
- E2E tests with real qmd binary (if/when CI has qmd available)
- Performance benchmarks for multi-workspace retrieval

## Escalate If
- A validation scenario reveals ambiguity in the spec (e.g., V7 proportional allocation produces unexpected results)
- Existing tests break due to phase 2 changes (indicates backward compat issue → revisit phase 2)
