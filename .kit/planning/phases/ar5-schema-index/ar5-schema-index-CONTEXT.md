# Context: Schema Index

Phase: ar5-schema-index
Status: ready
Spec Link: ../../SPEC-audit-remediation.md
Roadmap Link: ../../ROADMAP-audit-remediation.md
Blast Radius: low
Expected Proof: unit, integration

## Goal
Add the missing secondary index on `evidence_sources` — ISSUE-017. The only DB-touching phase, kept additive and idempotent.

## Scope Boundary
### Allowed Surfaces
- `src/core/db.ts` (`initSchema`)
- `tests/core/` (db schema test)

### Forbidden Surfaces
- dropping `sessions` / `queries` tables (ISSUE-015 — owned by p9-sqlite-session-memory)
- `SCHEMA_VERSION` bump (not needed for an idempotent `CREATE INDEX IF NOT EXISTS`)
- foreign-key enforcement (tied to the dead-tables decision, deferred)

## Spec Hooks
- Done-When: `idx_evidence_ws` exists on `evidence_sources(workspace, ingested_at)`.
- Reversed decision #2: ISSUE-015 deferred to p9; this phase does not touch those tables.

## Locked Decisions
- Add `CREATE INDEX IF NOT EXISTS idx_evidence_ws ON evidence_sources(workspace, ingested_at)` to the `db.exec` block in `initSchema`. Idempotent on every open; no migration logic; reversible via `DROP INDEX`.
- Keep `SCHEMA_VERSION = 1`. (If ISSUE-015 is later reinstated, a v2 bump + migration becomes warranted — out of scope now.)

## Assumptions
- The composite PK `(id, workspace)` cannot serve a leading-`workspace` filter, so the index is genuinely useful for `WHERE workspace = ? ORDER BY ingested_at` queries (`listEvidence`).
- `CREATE INDEX IF NOT EXISTS` on an existing live DB is safe and fast at personal scale.

## Canonical Refs
- `src/core/db.ts:21-80`
- `src/core/db-evidence.ts:57-61` (`listEvidence` query the index supports)
- `.kit/planning/SPEC-sqlite.md:73-90,131` (p9 ownership of sessions/queries)

## Rejected Options
- Dropping the dead tables here (ISSUE-015) — conflicts with the locked p9 phase; deferred with owner.
- A `SCHEMA_VERSION` bump — unnecessary churn for an idempotent additive index.

## Deferred Ideas
- ISSUE-015 table drops + ISSUE-017 FK enforcement — bundled into the p9 session-memory work.

## Escalate If
- the index is found to already exist under a different name, or a query plan shows it unused → check.
