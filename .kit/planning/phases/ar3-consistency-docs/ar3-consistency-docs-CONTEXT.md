# Context: Consistency & Docs

Phase: ar3-consistency-docs
Status: ready
Spec Link: ../../SPEC-audit-remediation.md
Roadmap Link: ../../ROADMAP-audit-remediation.md
Blast Radius: medium
Expected Proof: unit, integration

## Goal
Make docs match the real 4-state machine (test-enforced), validate `ingest`'s workspace, and make context generation deterministic — ISSUE-010, 013, 018.

## Scope Boundary
### Allowed Surfaces
- `assets/templates/evidence-index.md` (010)
- `CLAUDE.md` (010 — project doc)
- `src/generated/bundled-assets.ts` (regenerated)
- `src/core/workspace-resolver.ts` (013 — add validating resolver)
- `src/commands/ingest.ts`, `src/commands/learn.ts` (013 — use shared resolver)
- `src/core/current-task.ts`, `src/core/retrieval.ts` (018 — `nowIso` threading)
- `tests/` (doc-vs-code test, ingest validation test, determinism test)

### Forbidden Surfaces
- the evidence state machine itself (`evidence-state.ts` is the source of truth — docs follow it, not vice versa)
- ranking/allocation (owned by ar1)

## Spec Hooks
- I-5: state machine is exactly `ingested → reviewed → applied → archived`.
- Done-When: bundled template states == `evidenceStates`; `ingest --workspace typo` errors like `learn`; deterministic markdown given `nowIso`.

## Locked Decisions
- 010: rewrite the template + `CLAUDE.md` "Evidence Pipeline" section to the 4 real states; the **automated test targets the bundled `evidence-index.md` state list** (structured `` - `state` `` lines), not `CLAUDE.md` prose.
- 013: add `resolveWorkspaceName(paths, workspace?)` to `workspace-resolver.ts` that validates `existsSync(join(workspacesDir, name))`; `learn` and `ingest` both call it. Removes `ingest.ts`'s non-validating local copy.
- 018: add optional `nowIso?: string` to `generateCurrentTaskMarkdown`; default to `new Date().toISOString()` only when absent; thread an injectable value from `retrieve*Context`.

## Assumptions
- The stale 6-state text lives in exactly two files (confirmed by grep): `assets/templates/evidence-index.md:9-15` and `CLAUDE.md`.
- `evidenceStates` is exported from `evidence-state.ts:1-6` and importable in tests.

## Canonical Refs
- `src/core/evidence-state.ts:1-6`
- `assets/templates/evidence-index.md:9-15`
- `src/commands/learn.ts:31-41` (the validating resolver to mirror)
- `src/commands/ingest.ts:42-47` (the non-validating one to replace)
- `src/core/current-task.ts:35-45`

## Rejected Options
- A doc test that parses `CLAUDE.md` prose — brittle; scope the test to the bundled template instead.
- Removing the timestamp line entirely — loses the human-useful "Generated:" line; injecting `nowIso` preserves it while enabling determinism.

## Deferred Ideas
- Auto-generating the entire CLAUDE.md pipeline section from code (beyond scope).

## Escalate If
- `evidenceStates` cannot be imported into the test environment without a refactor → plan phase.
