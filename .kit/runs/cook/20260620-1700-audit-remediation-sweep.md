# COOK RUN

Run ID: work-20260620-1700-audit-remediation-sweep
Mode: full
Status: passed
Spec: .kit/planning/SPEC-audit-remediation.md
Roadmap: .kit/planning/ROADMAP-audit-remediation.md
Workflow State: .kit/workflow-state.yml
Phase: ar0-green-baseline → ar5-schema-index (full sweep)
Plan: .kit/planning/phases/{ar0..ar5}/*-PLAN.md
Started At: 2026-06-20 17:00

## Preflight
- scope drift: no — surfaces match SPEC Affected Surfaces (src/core/, src/commands/, assets/, CLAUDE.md, package.json, tests/)
- working tree note: branch `audit/p0-quick-wins`, 3 commits ahead of master, clean except `.kit/` continuity files
- required artifacts present: yes — SPEC (locked), ROADMAP, 6 phase CONTEXT+PLAN files
- baseline: `bun run typecheck` clean; `bun test` → 134 pass / 1 fail (pre-existing `assets.test.ts`, fixed by ar0)
- execution model: one sweep, verify typecheck+test green between phases (per ROADMAP)

## Wave / Task Log

### ar0-green-baseline — Wave 1
#### T1 — Ship workspaces asset group + regenerate bundle
- status: DONE
- changed files:
  - assets/workspaces/.gitkeep (new)
  - src/generated/bundled-assets.ts (regenerated — includes `workspaces/.gitkeep`)
- verification:
  - `bun test tests/assets.test.ts` → 2 pass / 0 fail
  - `bun test` → 135 pass / 0 fail
  - `bun run typecheck` → clean
- notes:
  - generator picks up dotfiles (`isFile()` true for `.gitkeep`); empty contents, preserved on resetup via `shouldPreserveExisting("workspaces/")`.

### ar1-retrieval-correctness — Waves 1-3
#### T1 — BM25 score tiebreak (ISSUE-008)
- status: DONE
- changed: src/core/retrieval-ranking.ts (score-desc tiebreak before `_index`), tests/retrieval/ranking.test.ts (+2)
#### T2 — Validate `--limit` (ISSUE-022)
- status: DONE
- changed: src/commands/ask.ts (sanitize to int>0 else 8), tests/commands.integration.test.ts (+1: `--limit abc` → adapter sees 8, results: 8)
#### T3 — Keyword match hardening + expose tags (ISSUE-023 + 009 prereq)
- status: DONE
- changed: src/core/query-parser.ts (word-boundary regex match on cleanQuery; `ParsedQuery.tags`), tests/core/query-parser.test.ts (+4: substring no longer matches, boundary still hits, tags exposed)
#### T4 — Multi-workspace slot floor/clamp/dedup (ISSUE-009)
- status: DONE
- changed: src/core/retrieval.ts (tagged secondary floor ≥1 even when primary saturated; ceil fair-share clamp; dedup by `workspace:path`), tests/retrieval/multi-workspace.integration.test.ts (+2: V10 tag floor, V11 dedup)
- verification:
  - `bun run typecheck` → clean
  - `bun test` → 144 pass / 0 fail (+9 from 135)
- notes:
  - dedup key `${workspace ?? primaryWorkspace}:${path}` per SPEC "Done When: deduped by workspace:path"; V6 (keyword-only, saturated primary) still skips secondary — only `@tag` secondaries get the floor.

### ar2-evidence-robustness — Waves 1-2
#### T1 — Insert-before-write ordering (ISSUE-005)
- status: DONE
- changed: src/core/evidence-ingest.ts (build record → `db.transaction(insertEvidence)` → then dirs + raw.md), tests/evidence/evidence-pipeline.test.ts (+1: forced INSERT failure leaves no orphan raw.md, listEvidenceIds unchanged)
#### T2 — Guard checkpoint + front-matter (ISSUE-014, 024)
- status: DONE
- changed: src/core/evidence-apply.ts (readCheckpoint try/catch + completed_paths array check → fresh; injectResourceIfMissing try/catch + non-plain-object guard → unchanged), tests (+2: corrupt checkpoint resumes fresh; YAML-list front-matter passes through byte-identical)
#### T3 — Evidence regression sweep
- status: DONE
- verification:
  - `bun run typecheck` → clean
  - `bun test tests/evidence/` → 9 pass / 0 fail (ISSUE-003 QA gate + ISSUE-006 write-before-check intact)
  - `bun test` → 147 pass / 0 fail (+3 from 144)
- notes:
  - first `db.transaction()` in the codebase; bun:sqlite rolls back + rethrows on callback error (verified by the orphan test). Success-path bytes for well-formed front-matter unchanged.

### ar3-consistency-docs — Wave 1
#### T1 — Reconcile state-machine docs + enforce test (ISSUE-010)
- status: DONE
- changed: assets/templates/evidence-index.md (4-state legend), CLAUDE.md (state-machine line + apply-stage line), src/generated/bundled-assets.ts (regenerated; 0 stale states), tests/docs-state-machine.test.ts (new: bundled legend == evidenceStates)
#### T2 — Shared validating workspace resolver (ISSUE-013)
- status: DONE
- changed: src/core/workspace-resolver.ts (`resolveWorkspaceName`), src/commands/learn.ts + ingest.ts (use shared; dropped now-unused imports — existsSync/join/RuntimePaths in learn, resolveActiveWorkspace/RuntimePaths in ingest), tests/commands.integration.test.ts (+1: `ingest review --workspace nope` throws "does not exist")
#### T3 — Deterministic context generation (ISSUE-018)
- status: DONE
- changed: src/core/current-task.ts (`nowIso?` on CurrentTaskInput; used for Generated line), src/core/retrieval.ts (thread `nowIso` through both retrieve fns), tests/retrieval/current-task.test.ts (+1: same nowIso → byte-identical)
- verification:
  - `bun run typecheck` → clean
  - `bun test` → 150 pass / 0 fail (+3 from 147, 24 files)
- notes:
  - doc test imports `evidenceStates` (not a hardcoded list) so future drift fails. `ingest` now validates `--workspace` identically to `learn`.

### ar4-hygiene — Waves 1-2
#### T1 — Remove unused deps (ISSUE-021)
- status: DONE
- changed: package.json + bun.lock (`bun remove marked vitest`); **trashed dead `vitest.config.ts`** (scope add: it `import`ed `vitest/config` and broke typecheck once vitest was gone — project uses bun:test, no script referenced it)
#### T2 — Remove dead pathInside clause (ISSUE-025)
- status: DONE
- changed: src/core/fs.ts (`relativePath === "" || !relativePath.startsWith("..")` — the `!startsWith("../")` clause was unreachable; behavior identical)
#### T3 — Document equal-by-construction columns (ISSUE-020)
- status: DONE
- changed: src/core/db-evidence.ts (comment at insertEvidence explaining `workspace` PK vs `workspace_at_ingest` hash input; neither droppable)
#### T4 — Ingest content dedup (ISSUE-016)
- status: DONE
- changed: src/core/db-evidence.ts (`findEvidenceIdByRawSha`), src/core/evidence-ingest.ts (early sha check → return existing id + `duplicate: true`, no insert/write), `IngestEvidenceResult.duplicate?`, tests/evidence/evidence-pipeline.test.ts (+1: re-ingest returns existing id; other workspace creates a fresh row)
- verification:
  - `bun run typecheck` → clean
  - `bun test` → 151 pass / 0 fail (+1)
  - `grep -rn "marked\|vitest" src tests` → NONE
- notes:
  - dedup is workspace-scoped (I-2). createEvidenceId is workspace-scoped too, so cross-workspace id strings can coincide; PK `(id, workspace)` keeps rows distinct (test asserts the finance row exists rather than id inequality).

### ar5-schema-index — Wave 1
#### T1 — Add idx_evidence_ws (ISSUE-017)
- status: DONE
- changed: src/core/db.ts (`CREATE INDEX IF NOT EXISTS idx_evidence_ws ON evidence_sources(workspace, ingested_at)` in initSchema), tests/core/db.test.ts (+1: index present after initDb)
- verification:
  - `bun run typecheck` → clean
  - `bun test tests/core/db.test.ts` → 7 pass (incl. existing idempotency test — `IF NOT EXISTS` re-runs clean)
  - `bun test` → 152 pass / 0 fail (+1)
- notes:
  - additive + idempotent; no SCHEMA_VERSION bump, no table drops (ISSUE-015 stays deferred to p9).

## Summary
- passed tasks: ar0 T1; ar1 T1-T4; ar2 T1-T3; ar3 T1-T3; ar4 T1-T4; ar5 T1 — **all DONE**
- blocked tasks: none
- unresolved concerns: none in scope. Scope addition: trashed dead `vitest.config.ts` (required to keep typecheck clean after `bun remove vitest`).
- final state: `bun run typecheck` clean; `bun test` **152 pass / 0 fail** (baseline was 134 pass / 1 fail → +17 net tests, formerly-failing assets.test.ts now green)
- 15 findings closed: 008, 009, 022, 023 (ar1); 005, 014, 024 (ar2); 010, 013, 018 (ar3); 016, 020, 021, 025 (ar4); 017 (ar5)
- invariants I-1..I-5 preserved; merged HIGH fixes (003/006) regression-swept green
- not committed (per session pattern — commit-only-local; no push/PR unless asked)

## Next Recommended Action
- `check` full (phase gate — running now per goal)
- then `git` (commit-per-phase or single sweep commit on `audit/p0-quick-wins`) if the gate is clean
