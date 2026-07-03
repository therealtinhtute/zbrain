# plan.md — MVP Team-Shared Agentic Memory (v2.1)

> Decisions locked 2026-07-03 (with ní): **git-per-workspace sync** · **fast-path note write** · **wire `sessions` table**.
> Base: `v2/07-oss-polish` @ b3cad1c — 78 tests pass, typecheck clean.
> Status at planning time: foundation ~80% done; the 5 delta tasks below are 0/5 started.

## Goal

Small team (2–5 người) shares memory through git. Claude Code is the primary runtime.
Taxonomy mapping: **workspaces = sharing scope** (`personal` local, `team-shared` git repo, `research` git repo hoặc local). The 4 wiki tiers (`axioms/mental-models/projects/decisions`) stay unchanged inside every workspace as the ranking axis. **No schema change to tiers.**

## Ground rules (from CLAUDE.md + V2 invariants)

- Files are truth; every write goes file-first, then DB in the same operation.
- `evidence/` is never indexed. Fast-path notes go to `wiki/` only, with conflict detection.
- All CLI UX via `@clack/prompts` (`src/commands/ui.ts` — `CommandUi`), no raw `console.log`.
- Every phase ends green: `bun run typecheck && bun test`. Regenerate `src/generated/bundled-assets.ts` after any `assets/` change.
- Each phase = one commit. Suggested messages included below.

---

## Phase 1 — `zbrain sync <workspace>`: git sync layer

**The only genuinely new subsystem.** Workspace root (`~/.zbrain/workspaces/<ws>/`) is itself a git repo.

### 1.1 New file `src/core/git-sync.ts` (core logic, testable)

```ts
export interface SyncResult {
  workspace: string;
  pulled: boolean; pushed: boolean; committed: boolean;
  reindexed: { indexed: number; removed: number };
  warnings: string[];
}
export function isGitWorkspace(paths: RuntimePaths, workspace: string): boolean; // .git exists
export function initGitWorkspace(paths, workspace, remoteUrl?: string): void;    // git init [+ remote add origin]
export function syncWorkspace(paths, db, workspace): SyncResult;
```

`syncWorkspace` flow (order matters):
1. Guard: `isGitWorkspace` else throw with hint `zbrain sync init <ws> --remote <url>`.
2. `git add -A && git commit -m "sync: <hostname> <ISO ts>"` — only if `git status --porcelain` is non-empty (`committed`).
3. If a remote exists: `git pull --rebase` (`pulled`). On rebase conflict: run `git rebase --abort`, throw a clear error telling the user to resolve manually in the workspace dir. **Never auto-resolve.**
4. If a remote exists: `git push` (`pushed`).
5. `rebuildWorkspace({paths, workspace, db})` from `src/core/indexer.ts` (`reindexed`). Full rebuild is fine at MVP scale; incremental is a later optimization.
6. No remote configured → local commit + reindex only, add warning `"no remote configured"`.

Implementation notes:
- Use `Bun.spawnSync(["git", "-C", wsRoot, ...])`; check `exitCode`, surface stderr in errors.
- Write a `.gitignore` in the workspace on `initGitWorkspace` — ignore nothing by default (`.trash/` **is** synced so forget propagates to the team; note this in the command output the first time).
- Do NOT touch `~/.zbrain/zbrain.db` or `projects/` — they are per-machine derived state and live outside the workspace dir already.

### 1.2 New file `src/commands/sync.ts`

```
zbrain sync <workspace>              # commit → pull --rebase → push → reindex
zbrain sync init <workspace> [--remote <url>]   # git init + optional remote
```
Register in `src/index.ts` (`registerSyncCommand(program)`). Clack spinner per step; print `SyncResult` summary via `ui.note`.

### 1.3 Tests `tests/sync.test.ts`

Simulate a team with a bare repo + two runtime homes, all inside `mkdtemp`:
1. `git init --bare remote.git`; create home A + home B (two fake `ZBRAIN_HOME`s via `RuntimePathOptions`), each `git clone` the bare into `workspaces/team/`.
2. A: `createNote` + `upsertNote` → `syncWorkspace(A)` → assert bare has the commit.
3. B: `syncWorkspace(B)` → assert note file exists in B's workspace AND `notes` row exists in B's DB (retrieval sees it).
4. Divergent-but-mergeable: A and B each add a *different* note → both sync → both end with both notes.
5. No-git workspace → `syncWorkspace` throws the hint error.
6. No remote → local commit + reindex, `warnings` contains `"no remote configured"`.

**Verify:** `bun test tests/sync.test.ts` green.
**Commit:** `feat(sync): git-backed workspace sync (commit → pull --rebase → push → reindex)`

---

## Phase 2 — Fast-path note write: `zbrain note add` + MCP `add_note`

Core primitives already exist (`createNote`, `detectConflict`, `upsertNote`) — this phase is wiring.

### 2.1 CLI `note add` in `src/commands/note.ts`

```
zbrain note add --tier <tier> --title <title> [--workspace <ws>] [--slug <slug>]
                [--body <text> | --file <path>]        # else read stdin
                [--source <evidence-id>...]
```
Flow in `runNoteAdd`:
1. Resolve workspace: `--workspace` flag → else `resolveWorkspace` (`src/core/workspace-resolver.ts`) → else error listing workspaces.
2. Validate tier with `isWikiTier`.
3. `slug = options.slug ?? slugify(title)`.
4. `detectConflict(paths, ws, tier, slug, [])` → if conflict, throw: `"<path> already exists (id: <id>). Use \`zbrain note update <id>\` to supersede it, or pass --slug."`
5. `createNote()` → `upsertNote()` (file first, then DB — both already behave this way).
6. `ui.note` with id + path + tier.

### 2.2 Fix two pre-existing defects in `note.ts` (touched file, in-scope)

- `runNoteUpdate` line 72: `detectConflict(..., options.slug ?? found.tier, ...)` passes a **tier** where a **slug** belongs → conflict check silently checks the wrong path. Fix: only run `detectConflict` when `options.slug` is provided and differs from `found.slug`; pass the actual slug.
- Delete dead stub `findNoteById` (lines 27–31, returns `{__deferred: true}`, never called).

### 2.3 MCP tool `add_note` in `src/mcp/server.ts`

Add to `MCP_TOOLS` + a `callTool` case:
```jsonc
{ "name": "add_note",
  "description": "Write a note directly to the wiki (trusted fast path). Conflict-checked; use remember for unverified external material.",
  "inputSchema": { "required": ["title", "body", "tier"],
    "properties": { "title": {...}, "body": {...},
      "tier": { "enum": ["axioms","mental-models","projects","decisions"] },
      "workspace": {...}, "slug": {...}, "sources": { "type": "array" } } } }
```
Same core flow as 2.1. On conflict return a JSON-RPC tool error whose message includes the existing note id (agent can then call `get_note` + decide to supersede). `remember` stays untouched — external material still goes through evidence.

### 2.4 Assets

Update `assets/skills/` zbrain:learn / engine rules to mention the fast path ("trusted first-party knowledge → `note add`; external sources → `learn`"). Then `bun run generate:assets`.

### 2.5 Tests

- `tests/note-add.test.ts`: happy path (file + DB row + FTS hit via `Fts5Adapter.search`), conflict rejected with existing id in message, stdin body, tier validation.
- `tests/mcp-protocol.test.ts`: extend — `add_note` writes an active wiki note (NOT evidence); `add_note` conflict returns error; note retrievable via `recall` immediately.
- Regression test for the 2.2 slug/tier fix.

**Verify:** `bun test` green.
**Commit:** `feat(note): fast-path note add (CLI + MCP add_note) + fix update conflict-check slug`

---

## Phase 3 — Wire the `sessions` table (short-lived metadata → SQLite)

Table exists in `src/core/db.ts` but has zero reads/writes today. Context **body** stays file-based (`projects/<hash>/sessions/<sid>.md`); SQLite holds the queryable metadata. This closes ní's "session/metadata → SQLite" requirement.

### 3.1 `src/core/session.ts`

```ts
export function touchSession(db: Database, s: { id: string; projectRoot: string; workspace: string }): void;
// INSERT INTO sessions ... ON CONFLICT(id) DO UPDATE SET last_activity_at=excluded.last_activity_at
export function listStaleSessions(db: Database, idleDays: number): SessionRow[];
export function deleteSession(db: Database, paths: RuntimePaths, id: string, projectRoot: string): void; // row + .md file (file removal via trash-safe unlink)
```

### 3.2 Call sites

- `src/commands/ask.ts`: after resolving workspace + session id, `touchSession(...)` before writing the context file.
- `src/mcp/server.ts` `recall`: same (MCP session id: reuse `getSessionId()` per server instance).

### 3.3 Doctor GC

Add a 9th check in `src/core/doctor.ts`: sessions with `last_activity_at` older than 30 days (constant `SESSION_IDLE_GC_DAYS`). Report them; `zbrain doctor --fix` deletes row + context file. Also flag orphans (row without file / file without row) — file-without-row is expected pre-migration, so `--fix` backfills rows from file mtime instead of deleting.

### 3.4 Tests

- `tests/concurrency.test.ts` or new `tests/session-db.test.ts`: `touchSession` inserts then bumps `last_activity_at`; ask writes a session row (extend `tests/end-to-end.test.ts`).
- `tests/doctor.test.ts`: stale session detected; `--fix` removes row + file; fresh session untouched.

**Verify:** `bun test` green.
**Commit:** `feat(session): wire sessions table (touch on ask/recall) + doctor GC`

---

## Phase 4 — Close AC-P1-9: kill the `projects.json` mirror

The last dual-source-of-truth remnant (V1's original disease).

### 4.1 Find every consumer first

`grep -rn "projects.json" assets/ src/ tests/` — engine rules + skills instruct agents to read it (see `helpers.ts:223` comment). List them before touching anything.

### 4.2 Replacement read path for agents

Add `zbrain workspace current --json` (in `src/commands/workspace.ts`): prints the resolved binding for `cwd` — `{ project_root, workspace, secondary_workspaces, context_file }` — straight from SQLite via `db-projects.ts`. Plain stdout JSON (this one is machine-read; keep clack for human paths only).

### 4.3 Cut over

1. Update every asset (engine rules, skills) that says "read `~/.zbrain/projects.json`" → "run `zbrain workspace current --json`". Regenerate bundled assets.
2. Delete the mirror write in `initProject` (`src/commands/helpers.ts:223-230`).
3. One-time migration in `runSetup`/`assertRuntimeReady`: if `projects.json` exists → import any bindings missing from the DB → rename to `projects.json.bak` (never delete; Hard Rule).
4. Remove `projectRegistryFile` from `RuntimePaths` once nothing references it (typecheck will find stragglers).

### 4.4 Tests

- `init` on a fresh home creates **no** `projects.json`.
- Migration: seed a legacy `projects.json` → setup imports it into SQLite → `.bak` exists.
- `workspace current --json` returns the binding after `init`.

**Verify:** `bun test` green + `grep -rn "projects.json" src/ assets/` returns only the migration code + `.bak` handling.
**Commit:** `fix(registry): single source of truth — drop projects.json mirror (AC-P1-9)`

---

## Phase 5 — Team onboarding docs + release

### 5.1 README

New section **"Team setup"**:
```
# teammate #2
git clone <team-repo> ~/.zbrain/workspaces/team-shared   # or: zbrain sync init team-shared --remote <url>
zbrain setup
cd <project> && zbrain init                                # pick team-shared (or personal + team-shared as secondary)
zbrain sync team-shared                                    # pull + reindex
```
Document explicitly:
- Leases/optimistic locking protect **multi-agent on one machine only**; cross-machine consistency is git's job (supersede model keeps merges rare).
- `personal` workspace = never synced; `team-shared`/`research` = git repos.
- Daily loop: `zbrain sync` at session start/end (hoặc hook vào Claude Code SessionStart hook — optional, ghi ví dụ).

### 5.2 Wrap-up

- Update `CHANGELOG.md` (v2.1.0), `AC-AUDIT.md` (flip AC-P1-9 to ✅).
- Full gate: `bun run typecheck && bun test && bun run build`, then binary smoke:
  `ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup && ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain sync init team --remote <tmp bare>` + one full A→B sync round-trip by hand.

**Commit:** `docs(team): onboarding + changelog for v2.1 team MVP`

---

## Out of scope (unchanged from todo.md)

Server/API sync · vectors/embeddings · fact graph · dynamic plugins · AC-P2-4 dead-code sweep · AC-P3-2 ULID migration · watch mode.

## Success criteria (MVP done =)

1. Two simulated machines share one workspace through a bare git repo; each sees the other's notes after `zbrain sync` (pinned by `tests/sync.test.ts`).
2. Agent writes a trusted note via MCP `add_note` with no review step, but conflicts/supersede are still enforced; external material still gates through evidence.
3. `ask`/`recall` leave queryable session rows in SQLite; `doctor --fix` GCs idle ones.
4. `grep projects.json` finds no live read/write path — SQLite is the only registry.
5. `bun run typecheck` + `bun test` + `bun run build` all green; binary smoke passes.
