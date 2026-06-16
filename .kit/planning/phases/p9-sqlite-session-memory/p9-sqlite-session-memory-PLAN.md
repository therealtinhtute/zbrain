# Plan: Session Term Memory

Phase: p9-sqlite-session-memory
Status: blocked (requires p8-sqlite-core-db complete)
Wave Count: 2
Execution Owner: work
Updated At: 2026-06-16

## Goal
Populate `sessions` and `queries` tables after every `zbrain ask` call.
No change to existing command behavior — DB write is additive and non-fatal.

## Inputs
- Phase 1 complete: `src/core/db.ts` with `initDb` and `sessions`/`queries` tables
- `src/commands/ask.ts` — current retrieval call site
- `src/core/retrieval.ts` — `RetrievalContext` return type with `results` array

---

## Wave 1
### T1 — Implement db-sessions.ts: session upsert + query insert + list recent
- type: implementation
- inputs:
  - `src/core/db.ts` — `Database` type (bun:sqlite)
  - SPEC-sqlite.md sessions + queries schema
- touches:
  - `src/core/db-sessions.ts` (new file)
  - `tests/core/db-sessions.test.ts` (new file)
- avoid:
  - `src/commands/` — no command integration yet (that's T2)
  - `sessions` table schema changes — schema is locked in Phase 1
- steps:
  1. Create `src/core/db-sessions.ts`.
  2. Implement `upsertSession(db: Database, projectRoot: string, workspace: string, nowIso: string): string`:
     - Compute session ID: `{projectRoot}:{nowIso.slice(0, 10)}`
     - INSERT OR IGNORE into `sessions` (started_at = nowIso if new)
     - UPDATE `last_activity_at = nowIso` always
     - Return session ID string
  3. Implement `insertQuery(db: Database, sessionId: string, opts: { queryText: string; workspace: string; contextFile: string; retrievedCount: number; queriedAt: string }): void`:
     - INSERT into `queries` with all fields
  4. Implement `listRecentQueries(db: Database, projectRoot: string, limit?: number): Array<{ queryText: string; workspace: string; queriedAt: string }>`:
     - JOIN sessions ON session_id, WHERE sessions.project_root = ?, ORDER BY queried_at DESC, LIMIT (limit ?? 20)
  5. Write tests with temp DB:
     - `upsertSession` creates row on first call, updates `last_activity_at` on second
     - `insertQuery` creates row, readable via `listRecentQueries`
     - `listRecentQueries` returns rows for correct project_root only
     - Session ID format verified: `{projectRoot}:{YYYY-MM-DD}`
- expected outputs:
  - `src/core/db-sessions.ts` with 3 functions
  - `tests/core/db-sessions.test.ts` all passing
- verification:
  - `bun test tests/core/db-sessions.test.ts`
- stop if:
  - REFERENCES sessions(id) constraint causes INSERT failure — check upsertSession is called before insertQuery
- escalate to:
  - plan phase

---

## Wave 2
### T2 — Update ask.ts: write session + query row after retrieval
- type: implementation
- inputs:
  - T1 `upsertSession`, `insertQuery`
  - `src/commands/ask.ts` — current implementation
  - `src/core/retrieval.ts` — `RetrievalContext` return type
- touches:
  - `src/commands/ask.ts`
- avoid:
  - retrieval logic (retrieveWorkspaceContext, retrieveMultiWorkspaceContext)
  - any change to ask.ts output or user-visible behavior
  - other commands
- steps:
  1. In `runAsk()` (or equivalent), after successful `retrieveWorkspaceContext` / `retrieveMultiWorkspaceContext` call:
     - Open DB: `const db = openDb(context.paths.runtimeDir)`
     - Call `upsertSession(db, context.paths.cwd, workspace, nowIso)` → get `sessionId`
     - Call `insertQuery(db, sessionId, { queryText: query, workspace, contextFile: result.filePath, retrievedCount: result.results.length, queriedAt: nowIso })`
  2. Wrap both DB calls in try/catch — on any DB error: `console.warn("[zbrain] session memory write failed:", err.message)` — do NOT throw, do NOT surface to user.
  3. `nowIso` = `new Date().toISOString()` at start of `runAsk()`.
- expected outputs:
  - `ask.ts` writes session + query row after every retrieval
  - DB error does not fail the ask command
  - No user-visible output change
- verification:
  - Manual: `bun run src/index.ts ask "test query"` then open DB and run `SELECT * FROM queries`
  - `bun test tests/` — full test suite still passes
- stop if:
  - `ask.ts` does not expose `RuntimePaths` or query result — read the file first before implementing
- escalate to:
  - plan phase (if ask.ts structure has drifted from expectation)

---

## Risks / Watch-fors
- T2 must call `upsertSession` before `insertQuery` — FK constraint on `session_id`
- Non-fatal wrapper is critical: a DB write failure must NEVER break the ask flow for the user
- `context.paths.cwd` is the project root used as `project_root` in sessions — verify it resolves the same as `projects.project_root` from Phase 1
