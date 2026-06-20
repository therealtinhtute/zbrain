# Plan: Evidence Robustness

Phase: ar2-evidence-robustness
Status: ready
Wave Count: 2
Execution Owner: cook
Updated At: 2026-06-20

## Goal
Land ISSUE-005 (orphan prevention), 014 (checkpoint guard), 024 (front-matter guard) without regressing merged ISSUE-003/006 guards.

## Inputs
- `src/core/evidence-ingest.ts`, `src/core/evidence-apply.ts`, `src/core/db-evidence.ts`
- existing evidence pipeline tests (incl. "apply resumes after interruption")

## Wave 1
### T1 — Insert-before-write ordering in ingest (ISSUE-005)
- type: implementation
- inputs:
  - `src/core/evidence-ingest.ts:31-57`
  - `src/core/db-evidence.ts:21-40`
- touches:
  - `src/core/evidence-ingest.ts`
  - `tests/evidence/evidence-pipeline.test.ts`
- avoid:
  - deleting any file (no `rm`); deferred full atomicity (007)
- steps:
  1. Build the source record before any disk write.
  2. Run `insertEvidence` inside `db.transaction(() => ...)()`.
  3. Only after the insert commits, `ensureEvidenceDirectories` + write `raw.md`.
- expected outputs:
  - a failing insert leaves no `raw.md` on disk
- verification:
  - test: force `insertEvidence` to throw (e.g. duplicate PK) → assert `raw.md` does not exist and `listEvidenceIds` is unchanged
- stop if:
  - reordering breaks ID generation that depends on a pre-written file (it does not — IDs use `listEvidenceIds`)
- escalate to:
  - plan phase

### T2 — Guard checkpoint + front-matter parsing (ISSUE-014, 024)
- type: implementation
- inputs:
  - `src/core/evidence-apply.ts:40-61`
- touches:
  - `src/core/evidence-apply.ts`
  - `tests/evidence/evidence-pipeline.test.ts`
- avoid:
  - the merged guards at `evidence-apply.ts:79-83` (keep order + behavior)
- steps:
  1. `readCheckpoint`: wrap `JSON.parse` in try/catch; if it throws or `completed_paths` is not an array, return the fresh checkpoint object.
  2. `injectResourceIfMissing`: wrap `YAML.load` in try/catch; if it throws or `fm` is not a plain object (`!fm || typeof fm !== "object" || Array.isArray(fm)`), return content unchanged.
- expected outputs:
  - corrupt checkpoint → fresh start (no throw); malformed front-matter → content passes through unmodified
- verification:
  - test: truncated `checkpoint.json` → apply resumes from scratch, no `SyntaxError`
  - test: a `.md` mutation whose front-matter is a YAML scalar/list → apply completes, file written unchanged
  - regression: existing "apply resumes after interruption" test still passes
- stop if:
  - guarding changes the success-path output bytes of a well-formed front-matter file
- escalate to:
  - check

## Wave 2
### T3 — Full evidence pipeline regression sweep
- type: test
- inputs:
  - T1, T2 changes
- touches:
  - `tests/evidence/evidence-pipeline.test.ts`
- avoid:
  - adding new pipeline features
- steps:
  1. Run the full evidence suite to confirm ISSUE-003 (QA gate) and ISSUE-006 (write-before-check) behavior is intact.
- expected outputs:
  - green evidence suite with new guards active
- verification:
  - `bun test tests/evidence/` → pass; `bun run typecheck` clean
- stop if:
  - any merged HIGH guard regresses
- escalate to:
  - plan phase

## Risks / Watch-fors
- Introducing the first `db.transaction()` call — confirm the bun:sqlite transaction wrapper commits/rolls back as expected in tests.
- Do not alter the apply guard ordering established by the merged ISSUE-006 fix.
