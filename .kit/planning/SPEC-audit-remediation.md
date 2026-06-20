# SPEC: Audit Remediation — Remaining Backlog

Status: locked
Input Type: audit-remediation
Lane: normal
Risk Flags: invariant-preservation, doc-drift, schema-additive
Affected Surfaces: src/core/, src/commands/, assets/, CLAUDE.md, package.json, tests/
Downstream: plan full
Updated At: 2026-06-20

Audit source of truth: https://claude.ai/code/artifact/8d70b981-f969-460f-a2f8-522f7031379d (25-finding registry, rev 2026-06-20)

---

## Goal

Close the remaining audit findings against zbrain after the P0 batch already merged. The 6 HIGH findings are fixed and verified in code; this remediation lands the Medium/Low backlog as a single sweep cut into 6 independently-mergeable phases, with no regression to the merged HIGH fixes and no change to the documented evidence/retrieval invariants.

---

## Status of all 25 findings

### Already resolved (merged — out of scope here, verified in code)
| Issue | Where verified |
|-------|----------------|
| 001 phantom registry | `helpers.ts:217-224` writes `projects.json` |
| 002 apply reindex | `ingest.ts:151-157` wires `reindex` |
| 003 QA gate | `evidence-apply.ts:80-83` reads `answers.md` |
| 004 workspace escape | `ask.ts:27`, `qmd-adapter.ts:81` reject traversal |
| 006 review clobber | `evidence-apply.ts:79` gates on `row.state` |
| 011 pipe escape, 012 atomic write | `current-task.ts:38,116-118` |

### In scope (this remediation — 15 findings)
| Issue | Sev | Phase |
|-------|-----|-------|
| 008 ranking discards BM25 score | M | ar1 |
| 009 multi-workspace drops `@tags` / slot math / no dedup | M | ar1 |
| 022 `--limit abc` → 0 results | L | ar1 |
| 023 keyword match on raw query + substring | L | ar1 |
| 005 non-atomic ingest (orphan file on insert failure) | H* | ar2 |
| 014 checkpoint parse unguarded | M | ar2 |
| 024 `injectResourceIfMissing` throws on malformed front-matter | L | ar2 |
| 010 doc drift — 6-state documented, 4 implemented | M | ar3 |
| 013 `ingest` accepts non-existent `--workspace` | M | ar3 |
| 018 impure `new Date()` in context generation | L | ar3 |
| 016 no dedup on ingest | L | ar4 |
| 020 redundant `workspace`/`workspace_at_ingest` columns | L | ar4 |
| 021 unused deps `marked`, `vitest` | L | ar4 |
| 025 dead condition in `pathInside` | L | ar4 |
| 017 no secondary index on `evidence_sources` | L | ar5 |

\* 005 is HIGH in the registry; only the cheap orphan-prevention mitigation is in scope. Full FS+SQLite atomicity belongs to ISSUE-007.

### Deferred (explicit, with owner)
| Issue | Why deferred | Owner |
|-------|--------------|-------|
| 007 no multi-machine sync; SQLite is de-facto source of truth | 1-2 day data-model redesign; subsumes 005's full atomicity | own `/think` + new SPEC |
| 015 drop dead `sessions`/`queries` tables | Conflicts with locked `SPEC-sqlite.md` phase `p9-sqlite-session-memory`, which wires these exact tables | `p9-sqlite-session-memory` |
| 019 no memory-type taxonomy / decay | Audit says "don't add speculatively" | future, scale-gated |

---

## In Scope

- Retrieval ranking, multi-workspace slot allocation, query parsing, limit validation.
- Evidence ingest atomicity (orphan prevention), checkpoint + front-matter robustness.
- Doc-vs-code reconciliation of the evidence state machine + a test that enforces it.
- Shared validating workspace resolver across `learn` and `ingest`.
- Deterministic context generation (`nowIso` injection).
- Ingest content dedup via existing `raw_sha256`.
- Dependency + dead-code hygiene.
- Additive `evidence_sources` index.

## NOT In Scope

- ISSUE-007 sync / source-of-truth redesign (own effort).
- ISSUE-015 dropping `sessions`/`queries` (owned by p9).
- ISSUE-019 memory taxonomy.
- Any new product feature, any re-fix of the 6 merged HIGH issues.
- Moving `raw.md` / wiki content; vector search; config.yml changes.

---

## Invariants (must remain true after every phase)

- I-1: `raw.md` immutable; SHA-256 verified against `raw_sha256` (`db-evidence.ts:91`).
- I-2: `workspace_at_ingest` matches active workspace at every transition.
- I-3: QA gate blocks apply on P0/P1 `awaiting_external`/`deferred` (`evidence-state.ts:53`).
- I-4: One workspace per query; no cross-workspace escape (`qmd-adapter.ts:73-86`).
- I-5: Evidence state machine is exactly `ingested → reviewed → applied → archived` (`evidence-state.ts:1-6,32-37`). Docs must match this.

---

## Reversed / locked decisions (with evidence)

1. **Phase 0 fixes the test by shipping the missing asset group, not gutting the test.** `assets.ts:29-30` (`shouldPreserveExisting` returns true for `workspaces/`) proves the runtime already expects a `workspaces/` asset group. Create `assets/workspaces/.gitkeep` + regenerate bundled assets. Reverses an earlier "update the test" call.
2. **ISSUE-015 deferred to p9, not dropped.** `SPEC-sqlite.md:73-90,131` plans to wire `sessions`/`queries`. Phase 5 is index-only. Reverses an earlier "drop them" call.
3. **005 = orphan-prevention only** (insert row before writing `raw.md`; rm-free). Full atomicity ships with 007.
4. **020 = comment-only.** Both columns are equal by construction; the integrity hash uses `workspace_at_ingest` and the PK uses `workspace`, so neither can be dropped without breaking a guarantee. No migration.
5. **6 phases, one sweep, commit-per-phase.** Phases 0–4 are revert-clean; Phase 5 is additive/idempotent (`CREATE INDEX IF NOT EXISTS`, reversible via `DROP INDEX`).

---

## Phases

- **ar0-green-baseline** — fix the pre-existing `assets.test.ts` fail. Unblocks clean verification.
- **ar1-retrieval-correctness** — 008, 009, 022, 023.
- **ar2-evidence-robustness** — 005 (mitigation), 014, 024.
- **ar3-consistency-docs** — 010, 013, 018.
- **ar4-hygiene** — 016, 020, 021, 025.
- **ar5-schema-index** — 017.

Each phase ships green and useful alone.

---

## Done When

- `bun run typecheck` clean and `bun test` green (0 fails, including the formerly-pre-existing `assets.test.ts`) after every phase.
- Retrieval: within-tier results ordered by BM25 score desc; explicit `@tag` workspaces always receive ≥1 slot; merged results deduped by `workspace:path`; non-numeric `--limit` falls back to 8; keyword secondaries match on cleaned query with word boundaries.
- Evidence: a failed ingest insert leaves no orphaned `raw.md`; a corrupt checkpoint resumes as fresh instead of throwing; malformed front-matter no-ops instead of aborting apply.
- Docs: `CLAUDE.md` + bundled `evidence-index.md` describe exactly the 4 real states; a test asserts the bundled template's state list equals `evidenceStates`.
- `ingest --workspace <typo>` errors with "workspace does not exist" (same as `learn`).
- `generateCurrentTaskMarkdown` is deterministic given `nowIso`.
- Re-ingesting identical content surfaces the existing evidence id instead of creating a duplicate.
- `marked` and `vitest` removed from `package.json`; `pathInside` dead clause gone.
- `idx_evidence_ws` exists on `evidence_sources(workspace, ingested_at)`.

---

## Risks

| Risk | Mitigation |
|------|-----------|
| A phase edit regresses a merged HIGH fix | Per-phase `bun test`; phases share no mutable state except `db.ts` (ar5, isolated last) |
| `assets/` edits not regenerated into the binary | ar0 + ar3 must run `bun run generate:assets` and commit `src/generated/bundled-assets.ts` |
| ISSUE-009 tag-floor needs a `ParsedQuery` API change | Scoped explicitly in ar1 (add `tags: string[]`); single phase |
| Index creation on a live DB | `CREATE INDEX IF NOT EXISTS` is idempotent + reversible (`DROP INDEX`); no version bump |

---

## Tooling / Dependencies

- `bun` (test, typecheck, build), `bun run generate:assets`.
- `qmd` NOT required — tests inject fake `RetrievalAdapter` / `QmdRunner`.
- No API keys, no network, no MCP servers.
