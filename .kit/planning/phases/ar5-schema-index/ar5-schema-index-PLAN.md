# Plan: Schema Index

Phase: ar5-schema-index
Status: ready
Wave Count: 1
Execution Owner: cook
Updated At: 2026-06-20

## Goal
Land ISSUE-017: add an additive, idempotent secondary index on `evidence_sources(workspace, ingested_at)`.

## Inputs
- `src/core/db.ts` (`initSchema`)
- `src/core/db-evidence.ts:57-61` (the `listEvidence` query the index supports)

## Wave 1
### T1 — Add idx_evidence_ws (ISSUE-017)
- type: migration
- inputs:
  - `src/core/db.ts:21-80`
- touches:
  - `src/core/db.ts`
  - `tests/core/` (db schema test)
- avoid:
  - dropping `sessions`/`queries` (p9 owns — ISSUE-015)
  - bumping `SCHEMA_VERSION`
  - enabling foreign keys
- steps:
  1. Append `CREATE INDEX IF NOT EXISTS idx_evidence_ws ON evidence_sources(workspace, ingested_at);` to the `db.exec` block in `initSchema`.
- expected outputs:
  - index present after `initDb`; existing DBs gain it on next open
- verification:
  - test: after `initDb`, `PRAGMA index_list(evidence_sources)` (or `sqlite_master` query) includes `idx_evidence_ws`
  - `bun test` green; `bun run typecheck` clean
- stop if:
  - an index of the same name already exists with a different definition
- escalate to:
  - check

## Risks / Watch-fors
- Keep it idempotent — re-running `initDb` must not error.
- Do not let this phase drift into the deferred ISSUE-015 table drops.
