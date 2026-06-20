# ROADMAP: Audit Remediation — Remaining Backlog

## Planning Basis
- source spec: `.kit/planning/SPEC-audit-remediation.md`
- planning mode: `full`
- execution model: one sweep, six commits (one per phase), `bun run typecheck && bun test` green between phases

## Phase order rationale
ar0 unblocks clean verification; ar1–ar4 are independent code-only batches grouped by subsystem (revert-clean); ar5 (additive index) runs last as the only DB-touching step.

---

## Phase 1: ar0-green-baseline
**Goal:** Make the suite fully green by shipping the missing `workspaces/` asset group the runtime already expects.

**Deliverables:**
- `assets/workspaces/.gitkeep` (asset group exists, persists in git, bundles)
- regenerated `src/generated/bundled-assets.ts`
- `tests/assets.test.ts` passes (135 pass / 0 fail)

**Dependencies:**
- none (entry phase)

**Risks / Watch-fors:**
- forgetting `bun run generate:assets` → bundled assets stale vs `assets/`
- `shouldPreserveExisting` (`assets.ts:30`) must keep treating `workspaces/` as preserve-on-resetup

---

## Phase 2: ar1-retrieval-correctness
**Goal:** Retrieval ranks within-tier by relevance, routes explicit `@tags` reliably, dedups, and handles bad limits — ISSUE-008, 009, 022, 023.

**Deliverables:**
- BM25 score tiebreak within tier (`retrieval-ranking.ts`)
- `--limit` validation in `ask.ts`
- keyword matching on cleaned query with word boundaries + `ParsedQuery.tags` (`query-parser.ts`)
- `@tag` slot floor, slot clamp, `workspace:path` dedup (`retrieval.ts`)

**Dependencies:**
- ar0 green baseline (clean verification)

**Risks / Watch-fors:**
- `ParsedQuery` gains `tags`; downstream consumers must compile
- tag-floor allocation must not starve primary results

---

## Phase 3: ar2-evidence-robustness
**Goal:** Ingest never orphans a file; corrupt checkpoint and malformed front-matter degrade gracefully — ISSUE-005 (mitigation), 014, 024.

**Deliverables:**
- insert-before-write ordering in `evidence-ingest.ts` (rm-free orphan prevention)
- guarded `readCheckpoint` parse (`evidence-apply.ts`)
- guarded `injectResourceIfMissing` front-matter parse (`evidence-apply.ts`)

**Dependencies:**
- ar0

**Risks / Watch-fors:**
- must not regress the merged ISSUE-006 write-before-check guard
- must not regress checkpoint resume behavior (interrupted apply stays resumable)

---

## Phase 4: ar3-consistency-docs
**Goal:** Docs match code (test-enforced), `ingest` validates its workspace, context generation is deterministic — ISSUE-010, 013, 018.

**Deliverables:**
- 4-state machine in `CLAUDE.md` + bundled `evidence-index.md` + regenerated assets
- doc-vs-code test asserting bundled template states == `evidenceStates`
- shared validating `resolveWorkspaceName` in `workspace-resolver.ts`, used by `learn` + `ingest`
- optional `nowIso` param in `generateCurrentTaskMarkdown`, threaded from `retrieval.ts`

**Dependencies:**
- ar0

**Risks / Watch-fors:**
- forgetting `bun run generate:assets` after editing the template
- the doc test targets the bundled template (structured list), not `CLAUDE.md` prose

---

## Phase 5: ar4-hygiene
**Goal:** Remove dead weight, add ingest dedup — ISSUE-016, 020, 021, 025.

**Deliverables:**
- `marked` + `vitest` removed from `package.json`
- dead `pathInside` clause removed (`fs.ts`)
- clarifying comment on the equal-by-construction columns (`db-evidence.ts`)
- ingest dedup: `findEvidenceIdByRawSha` + early sha check + `IngestEvidenceResult.duplicate`

**Dependencies:**
- ar0

**Risks / Watch-fors:**
- confirm 0 imports of `marked`/`vitest` before removal (already verified)
- dedup must be workspace-scoped (same sha in another workspace is not a dup)

---

## Phase 6: ar5-schema-index
**Goal:** Add the missing secondary index on `evidence_sources` — ISSUE-017.

**Deliverables:**
- `CREATE INDEX IF NOT EXISTS idx_evidence_ws ON evidence_sources(workspace, ingested_at)` in `initSchema`
- test: index present after init

**Dependencies:**
- ar0

**Risks / Watch-fors:**
- keep it additive/idempotent — no `SCHEMA_VERSION` bump, no table drops (ISSUE-015 owned by p9)
