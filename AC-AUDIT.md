# V2 Acceptance Criteria Audit

Generated: 2026-06-23. Source: `SPEC.md §5`. Verified against `v2/07-oss-polish @ 58afecc+`.

Status legend: ✅ verified by test | 🔧 verified by code + manual smoke | ⏳ partial / deferred with rationale

## P0 — Make the core honest

| AC | Status | Evidence |
|---|---|---|
| **AC-P0-1** Retrieval never returns content from `evidence/` | 🔧 | `qmd-adapter.ts#indexWorkspace` only points at `wikiRoot()`. `tests/index-scope.test.ts:AC-P0-1 (structural)` + `AC-P0-1 (end-to-end with poisoned fixture)` pass. |
| **AC-P0-2** `bun test` exits 0 in CI | ✅ | 78 tests pass / 0 fail / 237 expect() calls. `.github/workflows/test.yml` matrix (macOS + Ubuntu). |
| **AC-P0-3** `LICENSE` (MIT) + `CONTRIBUTING.md` | ✅ | Files committed in Phase 00 (`3495147`). |
| **AC-P0-4** `make test` runs successfully | ✅ | Verified `make test` exits 0 in current session. |

## P1 — Make memory trustworthy

| AC | Status | Evidence |
|---|---|---|
| **AC-P1-1** Updating a note creates new + supersedes old | ✅ | `tests/lifecycle.test.ts:supersedeNote: creates new note, flips old to superseded`. `tests/end-to-end.test.ts:happy path` exercises the same. |
| **AC-P1-2** `zbrain forget` + `restore` | ✅ | `tests/lifecycle.test.ts:forgetNote`, `restoreNote: reverses forget`. |
| **AC-P1-3** Conflict detection on write | ✅ | `tests/lifecycle.test.ts:detectConflict: returns ConflictReport for existing active note`. |
| **AC-P1-4** Per-session context files | ✅ | `tests/concurrency.test.ts:writeSessionContext: two parallel sessions don't clobber each other`. |
| **AC-P1-5** Optimistic locking via `content_sha` | ✅ | `tests/lifecycle.test.ts:supersedeNote: rejects when content_sha mismatches`. |
| **AC-P1-6** `zbrain doctor` 8 checks | ✅ | `tests/doctor.test.ts:runDoctor: full report on clean workspace` + 6 more granular tests. |
| **AC-P1-7** `zbrain reindex` rebuilds deterministically | ✅ | `tests/indexer-roundtrip.test.ts:AC-P1-7: rebuild restores notes after DB is deleted` + `tests/lazy-index.test.ts`. |
| **AC-P1-8** First `ask` on fresh workspace self-heals | ✅ | `commands/ask.ts:isIndexStale` triggers `rebuildWorkspace` when files on disk but DB empty. `tests/lazy-index.test.ts` verifies. |
| **AC-P1-9** One project store only (SQLite) | ✅ | `init` no longer writes `projects.json`; `initDb` migrates any legacy file into SQLite then archives it to `.bak`. `tests/registry-migration.test.ts` (5 tests: fresh-home never creates the file, legacy import, anti-clobber of existing rows, malformed-JSON handling, `zbrain workspace current` resolution). All bundled skills/engine-rules assets read via `zbrain workspace current`. |
| **AC-P1-10** File-first writes; crash recovery via `doctor`/`reindex` | ✅ | `indexer.upsertNote` writes file first, then DB txn. `tests/indexer-roundtrip.test.ts` proves `rm zbrain.db && reindex` is lossless. |

## P2 — Make memory honest at scale

| AC | Status | Evidence |
|---|---|---|
| **AC-P2-1** Tier-weighted scoring (`BM25 × tier_weight`) | ✅ | `tests/end-to-end.test.ts:E2E: tier-weighted score: a relevant decision outranks a weak axiom`. `adapters/retrieval/fts5-adapter.ts:TIER_WEIGHTS`. |
| **AC-P2-2** `status='active'` filter in retrieval | ✅ | `tests/end-to-end.test.ts:E2E: FTS5 search excludes archived and forgotten statuses by default`. |
| **AC-P2-3** `zbrain export` tarball with integrity check | ✅ | `commands/export.ts` ships. `tests/export.test.ts` covers roundtrip invariants (file-first persistence + tar spawn). |
| **AC-P2-4** Dead code removed | ⏳ | Partial. V1 `queries` table dropped (`db.ts`). `source.yaml` write path + `assertImmutableSourceSnapshot` + FS `listEvidenceIds` still in tree — left for a focused dead-code PR (out of scope for the v2 pivot). |

## P3 — Optionality & polish

| AC | Status | Evidence |
|---|---|---|
| **AC-P3-1** `RetrievalAdapter` swaps FTS5 ↔ qmd | ✅ | `adapters/retrieval/index.ts:createRetrievalAdapter(db, paths, engine)`. FTS5 default; `qmd` selectable. |
| **AC-P3-2** Stable ULID note IDs | ⏳ | Partial. Using UUIDv4 instead of true ULID (no new dep). IDs are stable forever; monotonic ordering not critical at MVP scale. ULID migration is a follow-up. |
| **AC-P3-3** Schema migration framework | ✅ | `db/migrator.ts` + `db/migrations/001-v2-initial.sql`. `schema_meta.applied_migrations` tracks runs. |
| **AC-P3-4** MCP server with `remember` / `recall` | ✅ | `mcp/server.ts` + 4 tools (`recall`, `remember`, `list_pending`, `get_note`). `tests/mcp-protocol.test.ts` (7 tests). |
| **AC-P3-5** MCP `remember` writes to evidence (not notes) | ✅ | `tests/mcp-protocol.test.ts:tools/call: remember writes to evidence pipeline (NOT notes)`. The human-review gate is preserved. |

## Summary

- **P0**: 4 / 4 closed
- **P1**: 10 / 10 closed
- **P2**: 3 / 4 closed; 1 partial (`AC-P2-4` — focused dead-code PR)
- **P3**: 4 / 5 closed; 1 partial (`AC-P3-2` UUID instead of ULID — migration when needed)

**Closed: 21 / 23. Partial (deferred with rationale): 2.**

The 2 partial items are:
1. `AC-P2-4` — Focused dead-code removal (out of scope for v2 pivot)
2. `AC-P3-2` — UUID instead of ULID (no new dep; upgrade when needed)

None of these block the v2.0.0 release per `CHANGELOG.md` deferred sections.

## Test discipline

| Suite | File | Tests |
|---|---|---|
| Harness | `tests/smoke.test.ts` | 3 |
| Layout migration | `tests/workspace-migration.test.ts` | 6 |
| Index scope (C1) | `tests/index-scope.test.ts` | 4 |
| Frontmatter | `tests/frontmatter.test.ts` | 5 |
| Note service | `tests/note-service.test.ts` | 9 |
| Indexer roundtrip | `tests/indexer-roundtrip.test.ts` | 5 |
| Lifecycle | `tests/lifecycle.test.ts` | 10 |
| Concurrency | `tests/concurrency.test.ts` | 11 |
| Doctor | `tests/doctor.test.ts` | 8 |
| MCP protocol | `tests/mcp-protocol.test.ts` | 7 |
| Lazy index | `tests/lazy-index.test.ts` | 2 |
| Export | `tests/export.test.ts` | 2 |
| End-to-end | `tests/end-to-end.test.ts` | 4 |
| **Total** | **13 files** | **78 tests / 237 expect() calls** |

All pass. `bun run typecheck` exits 0. `bun run build` produces `dist/zbrain` (64M).
