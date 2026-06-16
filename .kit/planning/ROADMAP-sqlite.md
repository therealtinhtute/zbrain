# ROADMAP: SQLite Metadata Layer + Session Memory

## Planning Basis
- source spec: `.kit/planning/SPEC-sqlite.md`
- planning mode: `full`
- recommended entry phase: `p8-sqlite-core-db`
- execution mode: sequential (Phase 2 depends on Phase 1 DB layer)

---

## Phase 1: p8-sqlite-core-db — Core DB Layer

**Goal:** Replace all file-based metadata storage with SQLite. Every existing CLI command continues to work identically. `raw.md` immutability and SHA-256 integrity model preserved.

**Deliverables:**
- `src/core/db.ts` — DB open, WAL mode, schema creation, schema version guard
- `src/core/db-projects.ts` — project registry CRUD replacing `projects.json` I/O
- `src/core/db-evidence.ts` — evidence metadata CRUD replacing `source.yaml` + `_index.md` I/O
- `scripts/migrate-to-db.ts` — non-destructive one-shot migration from existing files
- `evidence-store.ts` shrunk to: SHA-256, `raw.md` write, path construction only
- `evidence-ingest.ts`, `evidence-list.ts`, `config.ts` updated to use DB modules
- `setup.ts` calls `initDb()` during first-run
- Tests for all new DB modules (temp DB in `mkdtempSync`)
- Smoke test: `ZBRAIN_HOME=/tmp/smoke ./dist/zbrain setup && ls /tmp/smoke/zbrain.db`

**Dependencies:**
- Current codebase (`src/core/evidence-store.ts`, `src/core/config.ts`, `src/commands/setup.ts`)
- `bun:sqlite` (built into Bun runtime — no install required)

**Risks / Watch-fors:**
- `bun:sqlite` must be available in `bun build --compile` output — verify with smoke test before declaring done
- `verifySourceRecordIntegrity()` reads DB row instead of YAML — logic is identical, test it explicitly
- `_index.md` regex parser in `evidence-list.ts` must be fully replaced, not just supplemented

---

## Phase 2: p9-sqlite-session-memory — Session Term Memory

**Goal:** Add `sessions` and `queries` tables populated by `zbrain ask`. Enables recall of past retrieval history per project per day. No change to existing command behavior.

**Deliverables:**
- `src/core/db-sessions.ts` — session upsert, query insert, list recent queries
- `src/commands/ask.ts` updated — writes query row after every successful retrieval
- Tests for `db-sessions.ts` (temp DB)
- Manual acceptance: `SELECT * FROM queries` returns rows after `zbrain ask`

**Dependencies:**
- Phase 1 (p8-sqlite-core-db) — DB must exist and `initDb()` must initialize all tables

**Risks / Watch-fors:**
- Session ID is `{project_root}:{YYYY-MM-DD}` — verify it survives special chars in paths
- `ask.ts` writes to DB after retrieval; must not block or throw on DB error (non-fatal)
