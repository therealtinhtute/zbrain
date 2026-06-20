# Plan: Hygiene

Phase: ar4-hygiene
Status: ready
Wave Count: 2
Execution Owner: cook
Updated At: 2026-06-20

## Goal
Land ISSUE-021, 025, 020 (trivial) and 016 (ingest dedup).

## Inputs
- `package.json`, `src/core/fs.ts`, `src/core/db-evidence.ts`, `src/core/evidence-ingest.ts`, `src/core/evidence-store.ts`

## Wave 1
### T1 — Remove unused dependencies (ISSUE-021)
- type: refactor
- inputs:
  - `package.json:17,21`
- touches:
  - `package.json`, `bun.lock`
- avoid:
  - removing anything imported anywhere
- steps:
  1. `bun remove marked vitest`.
- expected outputs:
  - `marked` + `vitest` gone from deps/devDeps
- verification:
  - `bun run typecheck` clean; `bun test` green; `grep -r "marked\|vitest" src tests` → no imports
- stop if:
  - any source/test imports either package (none found)
- escalate to:
  - check

### T2 — Remove dead condition in pathInside (ISSUE-025)
- type: refactor
- inputs:
  - `src/core/fs.ts:27-30`
- touches:
  - `src/core/fs.ts`
- avoid:
  - changing isolation semantics
- steps:
  1. Replace the return with `relativePath === "" || !relativePath.startsWith("..")`.
- expected outputs:
  - identical behavior, no unreachable clause
- verification:
  - existing `pathInside` / isolation tests still pass
- stop if:
  - any test depended on the redundant clause (it cannot — behavior is identical)
- escalate to:
  - check

### T3 — Document equal-by-construction columns (ISSUE-020)
- type: docs
- inputs:
  - `src/core/db-evidence.ts:21-40`
- touches:
  - `src/core/db-evidence.ts` (comment only)
- avoid:
  - dropping a column or adding a migration
- steps:
  1. Add a comment at `insertEvidence` explaining `workspace` (PK) and `workspace_at_ingest` (integrity-hash input) are intentionally equal and why neither is dropped.
- expected outputs:
  - documented intent; no behavior change
- verification:
  - `bun run typecheck` clean
- stop if:
  - none
- escalate to:
  - check

## Wave 2
### T4 — Ingest content dedup (ISSUE-016)
- type: implementation
- inputs:
  - `src/core/db-evidence.ts`, `src/core/evidence-ingest.ts`, `src/core/evidence-store.ts:54`
- touches:
  - `src/core/db-evidence.ts` (new `findEvidenceIdByRawSha`)
  - `src/core/evidence-ingest.ts` (early sha check + `duplicate` flag)
  - `tests/evidence/evidence-pipeline.test.ts`
- avoid:
  - cross-workspace dedup (must be workspace-scoped)
  - a UNIQUE schema constraint (no migration here)
- steps:
  1. Add `findEvidenceIdByRawSha(db, workspace, sha): string | null` → `SELECT id FROM evidence_sources WHERE workspace = ? AND raw_sha256 = ? LIMIT 1`.
  2. In `ingestEvidence`, compute `sha256(rawContent)` early; if `findEvidenceIdByRawSha` returns an id, return `{ evidenceId: existing, rawFile: <its raw path>, duplicate: true }` without inserting or writing.
  3. Add `duplicate?: boolean` to `IngestEvidenceResult`.
- expected outputs:
  - re-ingesting identical content in a workspace returns the existing id, `duplicate: true`, no new row/file
- verification:
  - test: ingest content X twice in one workspace → second returns first id + `duplicate`; same content in a different workspace still creates a new row
- stop if:
  - the result-shape change ripples beyond `learn.ts` + tests
- escalate to:
  - plan phase

## Risks / Watch-fors
- T4 builds on ar2's insert-first ordering — keep the transactional insert intact when adding the pre-check.
- Dedup must compare within the same workspace only (I-2 / isolation).
