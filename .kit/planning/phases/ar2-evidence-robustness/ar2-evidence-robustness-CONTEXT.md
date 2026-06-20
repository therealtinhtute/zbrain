# Context: Evidence Robustness

Phase: ar2-evidence-robustness
Status: ready
Spec Link: ../../SPEC-audit-remediation.md
Roadmap Link: ../../ROADMAP-audit-remediation.md
Blast Radius: medium
Expected Proof: unit, integration

## Goal
Prevent orphaned ingest files and make crash-recovery / front-matter handling fail safe — ISSUE-005 (mitigation), 014, 024.

## Scope Boundary
### Allowed Surfaces
- `src/core/evidence-ingest.ts` (005)
- `src/core/evidence-apply.ts` — `readCheckpoint` (014) and `injectResourceIfMissing` (024) only
- `tests/evidence/evidence-pipeline.test.ts`

### Forbidden Surfaces
- the merged ISSUE-006 `assertValidEvidenceTransition(row.state, ...)` guard (`evidence-apply.ts:79`) — keep intact
- the merged ISSUE-003 QA-gate read (`evidence-apply.ts:80-83`) — keep intact
- full FS+SQLite atomicity / `source.yaml` redesign (ISSUE-007, deferred)

## Spec Hooks
- I-1 immutable `raw.md`; I-3 QA gate; I-5 state machine — all must still hold.
- Done-When: failed insert leaves no orphan `raw.md`; corrupt checkpoint resumes as fresh; malformed front-matter no-ops.

## Locked Decisions
- 005 = **insert-first ordering, rm-free**: build the source record, `insertEvidence` inside `db.transaction()`, then write `raw.md`. A failed insert never reaches the file write, so no orphan file is created. (A row-without-file from a later FS failure is surfaced by `verifyEvidenceIntegrity` and is strictly better than today's invisible orphan.)
- 014 = wrap `JSON.parse` in try/catch plus a shape check (`Array.isArray(completed_paths)`); on failure return a fresh checkpoint rather than throwing.
- 024 = wrap `YAML.load` in try/catch and require `fm && typeof fm === "object" && !Array.isArray(fm)`; otherwise return content unchanged.

## Assumptions
- `db.transaction()` is available on `bun:sqlite` (used nowhere yet — this introduces the first call).
- Removing nothing from the existing apply guard order; new guards are additive.

## Canonical Refs
- `src/core/evidence-ingest.ts:31-57`
- `src/core/evidence-apply.ts:40-61`
- `src/core/db-evidence.ts:21-40` (`insertEvidence`)

## Rejected Options
- Write `raw.md` first then delete on insert failure — needs a delete (conflicts with the "never `rm`" preference) and is more code than insert-first ordering.
- Wrapping file writes inside the SQLite transaction — file IO is not transactional; gives false safety.

## Deferred Ideas
- True FS+SQLite atomicity with temp-then-rename + compensation (ISSUE-007).

## Escalate If
- insert-first ordering breaks an existing ingest test invariant that cannot be updated within scope → plan phase.
