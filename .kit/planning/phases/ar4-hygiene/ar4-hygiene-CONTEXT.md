# Context: Hygiene

Phase: ar4-hygiene
Status: ready
Spec Link: ../../SPEC-audit-remediation.md
Roadmap Link: ../../ROADMAP-audit-remediation.md
Blast Radius: low
Expected Proof: unit

## Goal
Remove dead weight and add ingest dedup — ISSUE-016, 020, 021, 025.

## Scope Boundary
### Allowed Surfaces
- `package.json` (021)
- `src/core/fs.ts` (025)
- `src/core/db-evidence.ts` (016 query + 020 comment)
- `src/core/evidence-ingest.ts` (016 dedup check + result flag)
- `tests/evidence/`, `tests/core/`

### Forbidden Surfaces
- schema changes / table drops (ISSUE-015 owned by p9; index is ar5)
- the merged ingest insert-ordering from ar2 — build on it, do not revert it

## Spec Hooks
- Done-When: deps removed; `pathInside` dead clause gone; re-ingest surfaces existing id; columns documented as intentionally equal.

## Locked Decisions
- 021: `bun remove marked vitest` (grep confirmed 0 imports in `src/` and `tests/`).
- 025: in `pathInside`, drop the unreachable `&& !relativePath.startsWith("../")` — any `"../"` already starts with `".."`.
- 016: add `findEvidenceIdByRawSha(db, workspace, sha)` to `db-evidence.ts`; in `ingestEvidence`, compute `sha256(rawContent)` early, query before insert, and if found return `{ evidenceId: existing, rawFile, duplicate: true }` without inserting. Dedup is workspace-scoped.
- 020: add a comment at `insertEvidence` documenting that `workspace` (PK) and `workspace_at_ingest` (integrity hash input) are intentionally equal by construction; no runtime assert (no caller can diverge them) and no column drop (each backs a different guarantee).

## Assumptions
- `bun build --compile` already tree-shakes `marked`; removal is hygiene, not weight.
- `IngestEvidenceResult` may gain an optional `duplicate?: boolean` without breaking callers (`learn.ts` reads `evidenceId`/`rawFile`).

## Canonical Refs
- `package.json:17,21`
- `src/core/fs.ts:27-30`
- `src/core/db-evidence.ts:21-40`
- `src/core/evidence-ingest.ts:31-57`
- `src/core/evidence-store.ts:54` (`sha256`)

## Rejected Options
- Adding a `UNIQUE(raw_sha256, workspace)` constraint — schema change; the SELECT-before-insert approach matches the audit's recommended fix without a migration.
- Dropping a redundant column for 020 — breaks either the PK or the integrity hash.

## Deferred Ideas
- Interactive add-vs-skip prompt on duplicate (command-layer UX, optional follow-up).

## Escalate If
- `IngestEvidenceResult` shape change ripples into more callers than `learn.ts` + tests → plan phase.
