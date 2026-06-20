# HANDOFF

**Branch:** `audit/p0-quick-wins` — **local only, no upstream**, **3 commits ahead of `master`**. Working tree clean except `.kit/` continuity files.
**Base:** `master` = `origin/master` = `f47e71d`.
**This branch HEAD:** `01d1289 fix(evidence): validate real state before review/apply writes (ISSUE-006)`
**Continuity mode:** harness (feature branch, mid-review — commit-only-local per user choice)
**Session date:** 2026-06-20
**Audit report (persists, full 25-finding registry):** https://claude.ai/code/artifact/8d70b981-f969-460f-a2f8-522f7031379d

> Note: `.kit/workflow-state.yml` still points at a **separate, untouched track** — `p8-sqlite-core-db` planning (`.kit/planning/...`). This branch is the audit-fix track and does **not** work that plan; those pointers are preserved intentionally.

---

## START HERE

The branch is green and committed but **unmerged and unpushed**. Decide: open a PR, or continue the backlog.

```bash
git -C /Users/tinhtute/Lab/zbrain checkout audit/p0-quick-wins
git diff master                # review the 3 commits (P0 batch + ISSUE-003 + ISSUE-006)
bun run typecheck              # expect clean
bun test                       # expect 134 pass / 1 pre-existing fail (assets.test.ts)
```
**Expected:** typecheck clean; only `tests/assets.test.ts` fails (pre-existing, unrelated). Then either `gh pr create --base master` or pick the next backlog item (ISSUE-005 or ISSUE-007 — see below).

---

## Completed This Session

1. **ISSUE-003 — QA gate de-self-certified** (`c11ff4d`). Apply hardcoded `questions:[{P0,answered}]` → the gate validated a constant and could never block. Now: review persists each question's `id/severity/status` to `qa/{id}/answers.md` (the slot + `evidence-qa-answers.md` template that existed but were never wired); `applyEvidence` reads that file and gates on real data; the `questions` field is **removed** from `ApplyEvidenceOptions` so no caller can fabricate. Review CLI gained `--severity`/`--status` (validated; fact only required when `status=answered`). Files: `evidence-state.ts`, `evidence-store.ts`, `evidence-review.ts`, `evidence-apply.ts`, `commands/ingest.ts` + 3 test files.
2. **ISSUE-006 — validate real state before evidence writes** (`01d1289`). `reviewEvidence` hardcoded `assertValidEvidenceTransition("ingested","reviewed")` (ignored the row's real state) → re-reviewing an `applied` item clobbered `verified-facts.md`/`answers.md` before the late DB check threw. `applyEvidence` had the same write-before-check shape. Now both gate on `row.state` **before** any write. Files: `evidence-review.ts:42`, `evidence-apply.ts:79` + 2 regression tests.

Decisions/tradeoffs for both are logged in **`tasks/implementation-notes.md`** and plans in **`tasks/todo.md`**.

---

## Proof

- `bun run typecheck` → **clean**.
- `bun test` → **134 pass / 1 fail** (the fail is pre-existing — see Blockers).
- ISSUE-006 tests **proven as real regressions**: stashing only the two guards (keeping the tests) → both fail (apply test confirmed the wiki file was written before the throw); restored → pass.
- ISSUE-003 CLI loop test: `review --status awaiting_external` → `apply` rejects with `"QA gate blocked"`, wiki file never written.

---

## Blockers / Proof Gaps

- **PRE-EXISTING test fail (not from this work):** `tests/assets.test.ts` → "bundled asset tree" expects a top-level `workspaces/` dir, but `assets/workspaces/` does not exist. Proven pre-existing by stashing on a clean tree. Needs its own ticket: create `assets/workspaces/` seed dir **or** update the test.
- **ISSUE-003 real-path gap:** the reindex/apply path is verified only via injected fakes; the real `qmd index` spawn is non-fatal and unexercised. To confirm end-to-end, run a real `zbrain ingest apply` then `zbrain ask` against `~/.zbrain/` with `qmd` installed.
- **ISSUE-006 cast (T3):** `row.state as EvidenceState` has no runtime validation. Safe today (rows only ever hold valid states); becomes a risk if zbrain gains external/multi-writer DB access — overlaps ISSUE-007.
- **Not landed:** branch is local; no push, no PR (per user choice).

---

## Remaining Audit Backlog (prioritized — ISSUE-003 & 006 now DONE)

| Pri | Issue | What | Where |
|-----|-------|------|-------|
| **P1** | ISSUE-005 | No atomicity — **zero** `db.transaction()`. `ingestEvidence` writes `raw.md` then inserts → orphaned file on insert failure. Apply has a checkpoint (partial mitigation), ingest doesn't. **Note:** true FS+SQLite atomicity needs an ordering/compensation design, not one transaction. | `evidence-ingest.ts:37,51`; `evidence-apply.ts` |
| **P1** | ISSUE-007 | SQLite is truth for evidence state; not rebuildable from markdown → no multi-machine sync. Durable fix for the ISSUE-001 mirror-drift too. **Large** (architecture). Subsumes ISSUE-005. | `index.ts`, `db.ts:42-56` |
| P2 | ISSUE-008 | `rankRetrievalResults` sorts by tier then original index — `result.score` (BM25) is never used; within-tier relevance ignored. Small fix: secondary sort by score desc. | `retrieval-ranking.ts:42-49` |
| P2 | ISSUE-009/010 | Multi-workspace drops explicit `@tags`; `CLAUDE.md` + bundled `evidence-index.md` template document a 6-state machine, code has 4. | `retrieval.ts:74-93`; `CLAUDE.md` |
| P3 | ISSUE-013–025 | Hygiene: dead `sessions`/`queries` tables, no ingest dedup, unused deps (`marked`,`vitest`), checkpoint parse guard. | various |

Full registry (file:line, fixes, effort) is in the audit artifact above.

---

## Key Decisions This Session

- **ISSUE-003 Option B over A:** make `applyEvidence` read `answers.md` itself (remove the `questions` param) rather than just fixing the one CLI caller — removes the fabrication foot-gun at the core, not just for one caller.
- **ISSUE-006 scope: review + symmetric apply guard.** Fixed the identical write-before-check in apply too (same root cause, ~2 extra lines), not just the audit-scoped review path.
- **Behavior change accepted (T1):** re-reviewing a `reviewed` item is now a hard `Invalid evidence transition` error, not a silent overwrite. The state machine never permitted `reviewed→reviewed`; "amend a review" would be a separate feature.
- **Commit-only-local** continued from prior session: each fix committed on the branch, no push/PR, for local review.

---

## Environment

- Working directory: `/Users/tinhtute/Lab/zbrain` (macOS / darwin)
- Runtime home: `~/.zbrain/` (NOT a git repo; holds `zbrain.db` + WAL)
- Tests: `bun test` → 134 pass / 1 pre-existing fail; typecheck: `bun run typecheck` clean
- `qmd` (`npm i -g @tobilu/qmd`) required only for the real retrieval path; tests inject fakes and don't need it.
- This `HANDOFF.md` is written but **not committed** (matches prior session's pattern).
