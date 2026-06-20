# Implementation Notes — ISSUE-006 (validate real state before evidence writes)

> Running log of decisions, changes, and tradeoffs **not** spelled out in the approved plan
> (`tasks/todo.md`). For ní to skim before review.

Branch: `audit/p0-quick-wins` · base commit before this work: `c11ff4d` (ISSUE-003).

---

## Decisions not in the spec

### D1 — Notes file: markdown, in `tasks/`
Chose `tasks/implementation-notes.md` (markdown, not HTML) — git-friendly, lives next to
`todo.md`, no rendering step. The goal allowed either.

### D2 — Where to place the apply-side state guard
Plan said "add an early guard among the existing asserts." Exact placement chosen:
**right after `assertWorkspaceLock`, before the QA-gate read.** Rationale: fail-fast on an
illegal state is cheaper than `existsSync` + reading + parsing `answers.md`, and it keeps the
guard adjacent to the other identity/lock asserts. Trade-off: a not-yet-reviewed item now
fails on the *state* check rather than (incidentally) the gate — clearer error, same outcome.

---

## Changes made
- `src/core/evidence-review.ts:42` — `assertValidEvidenceTransition("ingested","reviewed")` → `assertValidEvidenceTransition(row.state as EvidenceState, "reviewed")`; added `type EvidenceState` to the existing import.
- `src/core/evidence-apply.ts:79` — added `assertValidEvidenceTransition(row.state as EvidenceState, "applied")` right after `assertWorkspaceLock`; added `assertValidEvidenceTransition` + `type EvidenceState` to the import.
- `tests/evidence/evidence-pipeline.test.ts` — two new tests (both tagged ISSUE-006):
  1. re-review of a `reviewed` item throws `Invalid evidence transition` and `verified-facts.md` keeps "First fact." (no clobber).
  2. apply on a never-reviewed (`ingested`) item throws and the wiki file is never written.

## Tradeoffs / things to know
- **T1 — Behavior change: re-review is now a hard error, not a silent overwrite.** The state
  machine (`transitionMap`) only allows `reviewed → applied`, so `reviewed → reviewed` was always
  meant to be illegal. Before, the hardcoded literal let a re-review *silently clobber*
  `verified-facts.md`/`answers.md`; now it throws `Invalid evidence transition: reviewed -> reviewed`.
  If you ever want "amend a review" as a real feature, that's a separate change (add a self-loop
  or an explicit re-open transition) — out of scope here.
- **T2 — Double validation is intentional, not redundant-by-accident.** The new early guard
  protects the *file writes*; `updateEvidenceState` (`db-evidence.ts:74`) still re-checks against
  a fresh row read to protect the *DB row*. In this single-threaded CLI they always agree; keeping
  both is cheap defense-in-depth.
- **T3 — `row.state as EvidenceState` cast.** The DB column is plain `TEXT`. The cast is safe
  because rows are only ever written via `insertEvidence(state:"ingested")` and validated
  transitions, so the value is always one of the four states. No runtime validation added — if the
  DB were hand-edited to an unknown state, `transitionMap[from]` would be `undefined` and `.includes`
  would throw a TypeError instead of a clean message. Judged not worth guarding for a personal tool;
  flag if zbrain ever gains external/multi-writer DB access (overlaps ISSUE-007).
- **T4 — Apply guard placement** (see D2): before the QA-gate read. Net effect — a not-yet-reviewed
  apply now fails with `Invalid evidence transition` rather than slipping past an empty gate and
  failing later. Clearer error, and nothing is written.
- **No scope creep:** did not touch the parallel "write-before-check" shapes that aren't state
  transitions (e.g. checkpoint/manifest ordering in apply) — those belong to ISSUE-005 (atomicity).

## Verification
- `bun run typecheck` → clean.
- `bun test` → **134 pass / 1 fail**; the single fail is the pre-existing `tests/assets.test.ts`
  (`assets/workspaces/` missing), unrelated to this work and present before the session.
- **Regression proof:** stashed only the two source guards (kept the new tests) → both tests
  **failed** (`0 pass / 2 fail`); the apply test showed `existsSync(...) === true`, i.e. the wiki
  file *was* written before the throw — exactly the bug. Restored guards → both pass.
