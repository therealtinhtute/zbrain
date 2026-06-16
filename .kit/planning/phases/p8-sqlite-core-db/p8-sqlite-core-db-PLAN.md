# Plan: Core DB Layer

Phase: p8-sqlite-core-db
Status: ready
Wave Count: 5
Execution Owner: work
Updated At: 2026-06-16

## Goal
Replace file-based metadata (projects.json, source.yaml, _index.md) with `~/.zbrain/zbrain.db`.
All existing CLI commands work identically. SHA-256 immutability model preserved.

## Inputs
- `src/core/evidence-store.ts` — current metadata logic
- `src/core/evidence-ingest.ts` — primary consumer
- `src/core/evidence-list.ts` — _index.md parser
- `src/core/config.ts` — project registry
- `src/commands/setup.ts` — first-run handler
- `bun:sqlite` (built-in, no install)

---

## Wave 1
### T1 — Implement db.ts: DB init, schema creation, WAL mode
- type: implementation
- inputs:
  - SPEC-sqlite.md schema section
  - `bun:sqlite` API (import { Database } from "bun:sqlite")
- touches:
  - `src/core/db.ts` (new file)
  - `tests/core/db.test.ts` (new file)
- avoid:
  - any command handlers
  - projects or evidence CRUD (those are T2/T3)
- steps:
  1. Create `src/core/db.ts` exporting `openDb(runtimeDir: string): Database`.
  2. `openDb` resolves path `{runtimeDir}/zbrain.db`, calls `new Database(path, { create: true })`.
  3. After open: `db.exec("PRAGMA journal_mode=WAL")`.
  4. Call `initSchema(db)` — runs CREATE TABLE IF NOT EXISTS for all 4 tables (projects, evidence_sources, sessions, queries) from SPEC schema.
  5. Set `PRAGMA user_version = 1` if not already set; throw if `user_version > 1` (future migration guard).
  6. Export `initDb(runtimeDir: string): Database` as the single public entry point.
  7. Write tests: temp dir DB opens successfully, WAL mode confirmed via `PRAGMA journal_mode`, schema tables present via `SELECT name FROM sqlite_master WHERE type='table'`.
- expected outputs:
  - `src/core/db.ts` with `openDb`, `initSchema`, `initDb` functions
  - `tests/core/db.test.ts` passing
- verification:
  - `bun test tests/core/db.test.ts`
- stop if:
  - `bun:sqlite` import fails — means bun version is too old; escalate
- escalate to:
  - plan phase (bun:sqlite unavailable)

---

## Wave 2
### T2 — Implement db-projects.ts: project registry CRUD
- type: implementation
- inputs:
  - T1 `openDb` / `initDb`
  - `src/core/config.ts` — `upsertProjectBinding`, `readProjectBinding`, `readProjectRegistry`, `writeProjectRegistry`
  - `src/schemas/config.ts` — `ProjectBinding`, `ProjectRegistry` types
- touches:
  - `src/core/db-projects.ts` (new file)
  - `tests/core/db-projects.test.ts` (new file)
- avoid:
  - `src/core/config.ts` (not modified yet — that's T6)
  - evidence tables
- steps:
  1. Create `src/core/db-projects.ts`.
  2. Implement `upsertProject(db: Database, binding: ProjectBinding): void` — INSERT OR REPLACE into `projects`, serializing `runtimes` and `secondary_workspaces` as JSON strings.
  3. Implement `readProject(db: Database, projectRoot: string): ProjectBinding | null` — SELECT + parse JSON columns.
  4. Implement `listProjects(db: Database): ProjectBinding[]` — SELECT all, parse JSON columns.
  5. Implement `readProjectRegistry(db: Database): ProjectRegistry` — wraps listProjects into `{ projects: [...] }`.
  6. Parse `runtimes` and `secondary_workspaces` columns through existing Zod schemas on read.
  7. Write tests: upsert + read roundtrip, missing project returns null, JSON column survives roundtrip.
- expected outputs:
  - `src/core/db-projects.ts` with 4 functions
  - `tests/core/db-projects.test.ts` passing
- verification:
  - `bun test tests/core/db-projects.test.ts`
- stop if:
  - Zod parse rejects existing project binding shape — fix schema, not DB
- escalate to:
  - plan phase

### T3 — Implement db-evidence.ts: evidence metadata CRUD
- type: implementation
- inputs:
  - T1 `openDb`
  - `src/core/evidence-store.ts` — `EvidenceSourceRecord`, `buildSourceRecord`, `serializeSourceRecord`, `fingerprintSourceRecord`, `sha256`
  - `src/core/evidence-state.ts` — `EvidenceState` type, `assertValidEvidenceTransition`
- touches:
  - `src/core/db-evidence.ts` (new file)
  - `tests/core/db-evidence.test.ts` (new file)
- avoid:
  - `raw.md` file operations (those stay in evidence-store.ts)
  - _index.md file (replaced, not extended)
- steps:
  1. Create `src/core/db-evidence.ts`.
  2. Implement `insertEvidence(db: Database, record: EvidenceSourceRecord): void` — INSERT into `evidence_sources`. Throw if id+workspace already exists.
  3. Implement `readEvidence(db: Database, workspace: string, id: string): EvidenceSourceRecord | null` — SELECT by PRIMARY KEY.
  4. Implement `listEvidence(db: Database, workspace: string): EvidenceSourceRecord[]` — SELECT all for workspace, ordered by ingested_at ASC.
  5. Implement `updateEvidenceState(db: Database, workspace: string, id: string, state: EvidenceState, updatedAt: string): void` — UPDATE state + state_updated_at. Call `assertValidEvidenceTransition(currentState, state)` before UPDATE.
  6. Implement `verifyEvidenceIntegrity(db: Database, workspace: string, id: string, rawContent: string): void` — reads row, recomputes sha256(rawContent) and fingerprintSourceRecord, compares to columns. Same logic as `verifySourceRecordIntegrity` in evidence-store.ts.
  7. Write tests: insert + read roundtrip, state transition valid/invalid, integrity verify pass/fail, list returns correct workspace items only.
- expected outputs:
  - `src/core/db-evidence.ts` with 6 functions
  - `tests/core/db-evidence.test.ts` passing
- verification:
  - `bun test tests/core/db-evidence.test.ts`
- stop if:
  - fingerprint mismatch after roundtrip — SHA-256 serialization differs from YAML version; fix before proceeding
- escalate to:
  - brainstorm refine

---

## Wave 3
### T4 — Shrink evidence-store.ts and update evidence-ingest.ts + evidence-list.ts
- type: refactor + implementation
- inputs:
  - T2 `db-projects.ts`
  - T3 `db-evidence.ts`
  - Current `src/core/evidence-store.ts`
  - Current `src/core/evidence-ingest.ts`
  - Current `src/core/evidence-list.ts`
- touches:
  - `src/core/evidence-store.ts` (remove YAML/MD metadata functions)
  - `src/core/evidence-ingest.ts` (use db-evidence.ts)
  - `src/core/evidence-list.ts` (replace _index.md parse with DB query)
  - `tests/core/evidence*.test.ts` (update to inject DB)
- avoid:
  - `src/commands/` except through function signatures
  - `raw.md` write logic (stays in evidence-store.ts)
  - knowledge wiki files
- steps:
  1. From `evidence-store.ts`, REMOVE: `serializeSourceRecord`, `parseSourceRecord`, `initializeEvidenceIndex`, `updateEvidenceIndex`, `fingerprintSourceRecord` (now in db-evidence.ts). KEEP: `sha256`, `buildSourceRecord`, `evidenceLocations`, `ensureEvidenceDirectories`, `listEvidenceIds`, `createEvidenceId`, `assertWorkspaceTarget`, `verifiedFactsMarkdown`.
  2. In `evidence-ingest.ts`: replace `writeTextFile(locations.sourceFile, serializeSourceRecord(record))` and `updateEvidenceIndex(...)` with `insertEvidence(db, record)`. Add `db: Database` parameter to `ingestEvidence()`.
  3. In `evidence-list.ts`: replace the _index.md regex parser with `listEvidence(db, workspace)`. Map DB rows to `EvidenceListItem`. Add `db: Database` parameter to `listEvidenceItems()`.
  4. Update all tests that use `ingestEvidence` or `listEvidenceItems` to open a temp DB and pass it in.
  5. Verify no test reads or writes `_index.md` directly after this step.
- expected outputs:
  - `evidence-store.ts` has no YAML/MD metadata functions
  - `evidence-ingest.ts` writes to DB, not YAML
  - `evidence-list.ts` queries DB, not file
  - Tests pass with injected temp DB
- verification:
  - `bun test tests/core/`
- stop if:
  - Removing a function from evidence-store.ts breaks more than 5 call sites not in scope — stop, list them, escalate
- escalate to:
  - plan phase

### T5 — Update config.ts: delegate project registry to db-projects.ts
- type: refactor
- inputs:
  - T2 `db-projects.ts`
  - Current `src/core/config.ts`
- touches:
  - `src/core/config.ts`
  - `tests/core/config*.test.ts`
- avoid:
  - Global config (config.yml) — only project registry functions change
  - `src/schemas/config.ts` — schemas unchanged
- steps:
  1. In `config.ts`, replace `readProjectRegistry`, `writeProjectRegistry`, `upsertProjectBinding`, `readProjectBinding` implementations with thin wrappers that call `db-projects.ts` equivalents, accepting `db: Database` as first param.
  2. Keep `readGlobalConfig`, `writeGlobalConfig`, `parseGlobalConfig` unchanged (YAML stays).
  3. Keep `parseProjectPointer`, `readProjectPointer` for legacy `.claude/zbrain.json` compat.
  4. Update tests: inject temp DB, verify project read/write roundtrip, verify global config functions unchanged.
- expected outputs:
  - `config.ts` project registry delegated to DB
  - Global config (YAML) functions untouched
  - Tests pass
- verification:
  - `bun test tests/core/config*.test.ts`
- stop if:
  - Any command imports config.ts project functions in a way that cannot accept db parameter — map these first, escalate if >3
- escalate to:
  - plan phase

---

## Wave 4
### T6 — Update setup.ts: call initDb() on first run
- type: implementation
- inputs:
  - T1 `initDb`
  - Current `src/commands/setup.ts`
- touches:
  - `src/commands/setup.ts`
- avoid:
  - init.ts, workspace.ts, other commands
- steps:
  1. Import `initDb` from `src/core/db.ts`.
  2. In `runSetup()`, after `assertRuntimeReady` / after runtime directory is created, call `initDb(context.paths.runtimeDir)`.
  3. Add spinner step: "Initializing database" → `initDb()` → "Database ready".
  4. Idempotent: `CREATE TABLE IF NOT EXISTS` means re-running setup is safe.
- expected outputs:
  - `setup.ts` calls `initDb` during setup flow
  - Re-running setup does not reset existing DB
- verification:
  - `ZBRAIN_HOME=/tmp/zbrain-smoke bun run src/index.ts setup && ls /tmp/zbrain-smoke/zbrain.db`
- stop if:
  - `initDb` throws during setup — investigate bun:sqlite compile embedding
- escalate to:
  - plan phase (bun:sqlite not embedded)

---

## Wave 5
### T7 — Write migration script: projects.json + source.yaml → DB
- type: migration
- inputs:
  - T1 `initDb`
  - T2 `db-projects.ts` (upsertProject)
  - T3 `db-evidence.ts` (insertEvidence)
  - Existing `~/.zbrain/projects.json`
  - Existing `~/.zbrain/workspaces/*/evidence/sources/*/source.yaml`
  - Existing `~/.zbrain/workspaces/*/evidence/_index.md` (state + state_updated_at)
- touches:
  - `scripts/migrate-to-db.ts` (new file)
- avoid:
  - deleting any existing YAML/JSON files
  - modifying any evidence content files (raw.md)
- steps:
  1. Parse runtime dir from `ZBRAIN_HOME` env or `~/.zbrain` default.
  2. Call `initDb(runtimeDir)` — creates DB if not exists.
  3. Read `projects.json` → parse each binding → call `upsertProject(db, binding)`. Log count.
  4. Walk `workspaces/*/evidence/sources/*/source.yaml`:
     - Parse source record YAML
     - Look up current state from `_index.md` regex parse (same pattern as old evidence-list.ts)
     - Call `insertEvidence(db, record)` with state from _index.md. Skip if already exists (INSERT OR IGNORE).
  5. Print summary: `Migrated: N projects, M evidence items. DB: {path}`.
  6. Print: `Old files NOT deleted. Run 'zbrain db verify' to confirm integrity (future).`
- expected outputs:
  - `scripts/migrate-to-db.ts` runnable with `bun scripts/migrate-to-db.ts`
  - DB populated from existing runtime files
- verification:
  - `bun scripts/migrate-to-db.ts`
  - `bun run src/index.ts ingest list` shows same items as before migration
- stop if:
  - Any source.yaml has SHA-256 mismatch before migration — log warning, skip that item, continue
- escalate to:
  - user clarification (corrupt evidence files)

### T8 — Smoke test compiled binary
- type: integration
- inputs:
  - T1–T7 all complete
  - `bun run build` producing `dist/zbrain`
- touches:
  - `dist/zbrain` (read-only)
- avoid:
  - modifying any source files
- steps:
  1. `bun run build`
  2. `ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup`
  3. `ls /tmp/zbrain-smoke/zbrain.db` — must exist
  4. `ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain workspace create testws`
  5. `ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain learn --workspace testws --label "smoke test" --rawContent "hello world"`
  6. `ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain ingest list` — must show item from DB
- expected outputs:
  - Compiled binary creates DB, writes evidence row, lists from DB
- verification:
  - All 6 steps above execute without error
- stop if:
  - Step 3 fails (zbrain.db not created) — bun:sqlite not embedded; escalate immediately
- escalate to:
  - plan phase (switch to better-sqlite3)

---

## Risks / Watch-fors

- `bun:sqlite` embed: T8 smoke test is the only real proof — do not skip it
- SHA-256 fingerprint: T3 `verifyEvidenceIntegrity` must produce identical result to old `verifySourceRecordIntegrity`; add an explicit cross-check test with a known source.yaml fixture
- `_index.md` regex: migration script (T7) reuses the old regex parser — validate it against actual _index.md files before calling it "done"
- Do not delete YAML files: T7 is non-destructive by design; any cleanup is a separate future task
