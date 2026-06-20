# Plan: Consistency & Docs

Phase: ar3-consistency-docs
Status: ready
Wave Count: 1
Execution Owner: cook
Updated At: 2026-06-20

## Goal
Land ISSUE-010 (doc drift + test), 013 (shared validating resolver), 018 (deterministic context). Three independent surfaces — one wave.

## Inputs
- `src/core/evidence-state.ts` (source of truth for states)
- `assets/templates/evidence-index.md`, `CLAUDE.md`
- `src/core/workspace-resolver.ts`, `src/commands/{ingest,learn}.ts`
- `src/core/current-task.ts`, `src/core/retrieval.ts`

## Wave 1
### T1 — Reconcile state-machine docs + enforce with a test (ISSUE-010)
- type: docs
- inputs:
  - `src/core/evidence-state.ts:1-6`
  - `assets/templates/evidence-index.md:9-15`, `CLAUDE.md`
- touches:
  - `assets/templates/evidence-index.md`
  - `CLAUDE.md`
  - `src/generated/bundled-assets.ts` (regenerated)
  - `tests/` (new doc-vs-code test)
- avoid:
  - editing `evidence-state.ts` to match docs (docs follow code)
- steps:
  1. Replace the 6-state list (`ingested/analyzed/qa_in_progress/qa_done/applied/archived`) in the template + `CLAUDE.md` with the 4 real states `ingested → reviewed → applied → archived`.
  2. Run `bun run generate:assets`.
  3. Add a test that reads the bundled `evidence-index.md` state list and asserts it equals `evidenceStates` imported from `evidence-state.ts`.
- expected outputs:
  - docs describe 4 states; test fails if they ever diverge again
- verification:
  - new doc-vs-code test passes; `bun test` green
- stop if:
  - the bundled template's state list cannot be parsed deterministically (then assert against a known constant list) 
- escalate to:
  - plan phase

### T2 — Shared validating workspace resolver (ISSUE-013)
- type: refactor
- inputs:
  - `src/commands/learn.ts:31-41`, `src/commands/ingest.ts:42-47`
- touches:
  - `src/core/workspace-resolver.ts`
  - `src/commands/ingest.ts`, `src/commands/learn.ts`
  - `tests/commands.integration.test.ts`
- avoid:
  - changing active-workspace fallback behavior
- steps:
  1. Add `resolveWorkspaceName(paths, workspace?)` to `workspace-resolver.ts`: if `workspace` given, validate `existsSync(join(paths.workspacesDir, workspace))` and throw `Workspace "<name>" does not exist.` otherwise; else fall back to `resolveActiveWorkspace`.
  2. Replace the local resolvers in `ingest.ts` and `learn.ts` with the shared one.
- expected outputs:
  - `ingest review --workspace typo` errors with the same message as `learn`
- verification:
  - integration test: `runIngestReview(id, { workspace: "nope" })` throws "does not exist"
- stop if:
  - a command relies on the non-validating behavior (none does)
- escalate to:
  - check

### T3 — Deterministic context generation (ISSUE-018)
- type: implementation
- inputs:
  - `src/core/current-task.ts:35-45`, `src/core/retrieval.ts:29-107`
- touches:
  - `src/core/current-task.ts`
  - `src/core/retrieval.ts`
  - `tests/retrieval/` determinism test
- avoid:
  - removing the "Generated:" line
- steps:
  1. Add optional `nowIso?: string` to `CurrentTaskInput`; use it for the "Generated:" line, defaulting to `new Date().toISOString()` only when absent.
  2. Thread an optional `nowIso` from `retrieveWorkspaceContext` / `retrieveMultiWorkspaceContext` into `generateCurrentTaskMarkdown`.
- expected outputs:
  - identical inputs + same `nowIso` → byte-identical markdown
- verification:
  - unit: two calls with the same `nowIso` produce identical output
- stop if:
  - none expected
- escalate to:
  - check

## Risks / Watch-fors
- Forgetting `bun run generate:assets` after editing the template makes the doc-vs-code test pass against stale bundled content.
- The doc test must import `evidenceStates`, not hardcode the list, or it cannot catch future drift.
