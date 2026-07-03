# CHANGELOG

## 2.1.0 (2026-07-03)

### Added
- **`zbrain sync <workspace>`**: git-backed workspace sync — commit local changes, `pull --rebase`,
  push, reindex. `zbrain sync init <workspace> [--remote <url>]` turns a workspace into a git repo.
  Cross-machine consistency is git's job; advisory leases/optimistic locking remain the
  same-machine, multi-agent safety net (see README "Team setup").
- **`zbrain note add`**: fast-path write for already-trusted, first-party knowledge directly to
  the wiki, bypassing `learn`/`ingest` staging. Still conflict-checked and governed by the same
  supersede-not-overwrite lifecycle; reserve `learn`/`ingest` for material needing a human review
  step.
- **`zbrain workspace current`**: prints the resolved project→workspace binding as JSON. Replaces
  reading `~/.zbrain/projects.json` directly.
- **Session metadata wired end-to-end**: the `sessions` SQLite table (previously dead) is now
  written on every `zbrain ask` call and every MCP `recall` call (`touchSession`), and surfaced in
  `zbrain doctor`'s idle-session check.
- **`zbrain doctor --fix`**: GCs sessions idle 30+ days (`SESSION_IDLE_GC_DAYS`) before generating
  the doctor report.

### Changed
- **Project registry consolidated to SQLite (AC-P1-9).** `~/.zbrain/projects.json` is no longer
  written. On first run after upgrade, `initDb` imports any existing `projects.json` entries not
  already in SQLite, then renames the file to `projects.json.bak` (never deletes). All bundled
  skills/engine-rules assets now instruct agents to run `zbrain workspace current` instead of
  reading the file.

### Fixed
- **`zbrain sync init --remote <url>` on a workspace whose remote already has history** used to
  always `git init` an unrelated root, so the very first `sync` rebased onto the remote's history
  and conflicted immediately (reproduced end-to-end during this release's smoke test). It now
  fetches first and adopts the existing `main`/`master` branch when present, clearing the
  freshly-scaffolded starter files so they don't block the checkout. A second teammate can now
  join a shared workspace through the documented CLI flow (`workspace create` → `sync init
  --remote`) without a manual `git clone` workaround.

## 2.0.0 (2026-06-23)

### Breaking
- **Storage layout**: tier content moved from `<ws>/<tier>/` to `<ws>/wiki/<tier>/`. Evidence is structurally unindexable (C1 fix).
- **File-first writes**: notes are markdown files with YAML frontmatter; SQLite is a rebuildable cache. `rm zbrain.db && zbrain reindex` is lossless.
- **Retrieval engine**: FTS5 (BM25) replaces external `qmd` dependency. `qmd` remains selectable but is no longer the default.
- **Review→apply**: no longer silent-overwrites. `ingest apply` runs through `noteService` with conflict detection; overlapping writes without `supersedes` are refused.
- **CLI surface**: per-session context files replace shared `current-task.md`. `zbrain note {show,update,archive,forget,restore}`, `zbrain lease {acquire,release,list}`, `zbrain reindex`, `zbrain doctor`, `zbrain mcp serve` are new.
- **MCP server**: ships as a new agent-facing surface. `remember` writes to evidence pipeline only — does NOT auto-apply (the moat is preserved).

### Added
- **Memory lifecycle**: notes have a full state machine (`active → superseded → archived → forgotten → active`). Forgetting is recoverable via tombstone + `.trash/`.
- **Tombstones + .trash**: forget moves file to `.trash/<id>.md.bak`; restore reads `original_tier` / `original_path` from a sidecar tombstone.
- **Advisory write leases**: per-`(workspace, path)` with TTL auto-expiry.
- **Per-session context files**: `projects/<hash>/sessions/<sid>.md`; multiple agents no longer clobber each other.
- **`zbrain doctor`**: 8-check reconciliation (DB↔files, orphan evidence, stale review, broken links, expired leases, idle sessions, schema version, FTS5 sync).
- **`zbrain reindex`**: deterministic file→DB rebuild; disaster-recovery primitive.
- **Optimistic locking**: every mutation accepts `expectedSha`; mismatch throws `ShaMismatchError` instead of clobbering.
- **Tier-weighted ranking**: `BM25 × tier_weight` instead of hard tier sort. Decisions can outrank axioms on relevance.
- **`status='active'` filter** in retrieval: superseded/archived/forgotten structurally excluded.
- **V1→V2 layout migration**: idempotent, runs on every CLI boot via `assertRuntimeReady`.
- **MCP server (JSON-RPC 2.0 over stdio)**: 4 tools — `recall`, `remember`, `list_pending`, `get_note`.

### Changed
- `qmd` global install no longer required.
- `classifyByTier` uses path substring match (still works for `wiki/<tier>/`); explicit `tier` field in frontmatter planned for a future migration.
- Retrieval ranking multiplied by tier weight; relative order within a tier preserved.

### Removed
- `queries` table (V1 speculative; never consumed).
- `current-task.md` shared file (replaced by per-session files).
- `source.yaml` write path (restored as file in `evidence/sources/<id>/`).
- Dependency on global `qmd` install (FTS5 is default; qmd stays selectable).

### Migration
- **V1→V2 layout**: auto on next `zbrain` CLI invocation. Tier content moves from `<ws>/<tier>/` to `<ws>/wiki/<tier>/`; idempotent. Per-workspace failures log a warning, do not block the command.
- **V1 storage**: existing `evidence/sources/<id>/raw.md` is still readable. New notes use frontmatter; old applied-content files (no frontmatter) are tolerated by `readNote` (defaults to `status: active`).

### Test discipline
- Test suite restored: `bun test` exits 0 across 10 files, 70+ tests.
- CI: GitHub Actions matrix (macOS + Ubuntu) runs `bun test` + `bun run typecheck` on every push.

## 0.1.0 (pre-V2)

- V1 MVP. Per-workspaces isolation, evidence pipeline, qmd-backed BM25, tier rerank. Test suite later removed (chore commits `c057fe0`, `fa873e1`); restored in V2 Phase 00.
