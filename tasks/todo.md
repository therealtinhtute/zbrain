# ISSUE-003 — Make the QA gate real (de-self-certify apply)

## Problem
`runIngestApply` (`src/commands/ingest.ts:125`) hardcodes
`questions: [{ id: "q-1", severity: "P0", status: "answered" }]` and feeds it to
`validateQAGate`. The gate therefore validates a constant, never real data — so the
documented invariant "block apply if any P0/P1 question is `awaiting_external`/`deferred`"
**can never fire**. The review stage never persists question status; apply never reads it.

The intended design is already half-scaffolded but unwired:
- `evidence-store.ts` defines `qaAnswersFile = qa/{id}/answers.md` — **never written or read**.
- `assets/templates/evidence-qa-answers.md` defines the `| question_id | severity | status | answer |` table.

## Fix (Option B — gate reads persisted review data, foot-gun removed)
Persist QA questions at **review**, read them at **apply**. `applyEvidence` no longer
accepts caller-supplied `questions` — it reads `answers.md` itself, so no caller can fabricate.

### Success criteria
- Review records each question's `id/severity/status` to `qa/{id}/answers.md`.
- Apply reads `answers.md` and gates on it; the hardcoded literal is gone.
- End-to-end (CLI): `review --status awaiting_external` then `apply` **throws**; `review` (answered) then `apply` **succeeds**.
- `bun run typecheck` clean; full suite green (except the pre-existing `tests/assets.test.ts` fail).

## STATUS: DONE (uncommitted)
- typecheck clean; full suite **132 pass / 1 fail** (the 1 fail is the pre-existing `tests/assets.test.ts`, unrelated).
- New CLI regression test passes: `review --status awaiting_external` → `apply` **rejects** with "QA gate blocked"; the wiki file is never written.
- ISSUE-003 file set (vs review): `evidence-state.ts`, `evidence-store.ts`, `evidence-review.ts`, `evidence-apply.ts`, `commands/ingest.ts` + 3 test files.

## Steps
1. **`src/core/evidence-store.ts`** — add `qaAnswersMarkdown(questions)` writer + `parseQaAnswers(md)` reader (table round-trip; gate only needs id/severity/status, answer cell pipe-escaped). → verify: unit round-trip.
2. **`src/core/evidence-review.ts`** — `reviewEvidence` accepts optional `questions: EvidenceQuestion[]`; if omitted, derive one answered question per distinct `fact.questionId`. Write `answers.md`. → verify: review writes file.
3. **`src/core/evidence-apply.ts`** — remove `questions` from `ApplyEvidenceOptions`; read+parse `answers.md`, call `validateQAGate` on real data (missing file → empty → no block). → verify: blocked-QA test.
4. **`src/commands/ingest.ts`** — `runIngestReview` gains `--severity` (default P0) / `--status` (default answered); fact optional when status ≠ answered. `runIngestApply` drops the hardcoded `questions`. → verify: CLI loop test.
5. **Tests** — update 4 `applyEvidence` call sites (evidence-pipeline ×3 + acceptance ×1) to drop `questions:`; migrate the "rejects blocked QA" case to persist the blocker at review. Add CLI regression test in `commands.integration.test.ts` proving review→apply gate fires.
6. **Verify** — `bun run typecheck` + `bun test`; confirm new test fails before the fix logic and passes after.

## Out of scope
- Multi-question interactive review UX, answer-content storage beyond the table, ISSUE-005/006 atomicity.
- No `assets/` change (template already bundled) → no asset regen.

## Notes / risks
- Backward compat: old evidence without `answers.md` → empty questions → apply allowed (same as today). Review now always writes the file.
- `validateQAGate` itself is unchanged and already unit-tested.
