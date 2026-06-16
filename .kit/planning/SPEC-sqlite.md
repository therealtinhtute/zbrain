# SPEC: SQLite Metadata Layer + Session Memory

Status: locked
Input Type: upgrade-spec
Lane: normal
Risk Flags: data-model, migration, invariant-preservation
Affected Surfaces: src/core/, src/commands/setup.ts, src/commands/ask.ts, scripts/
Downstream: plan full
Updated At: 2026-06-16

---

## Goal

Replace zbrain's file-based metadata storage with a single global WAL-mode SQLite database at `~/.zbrain/zbrain.db`. Add session/query term memory tables as new capability. All existing CLI commands must work identically after migration — no user-visible behavior change in Phase 1.

---

## In Scope

- `projects` table: replaces `~/.zbrain/projects.json`
- `evidence_sources` table: replaces per-evidence `source.yaml` + `_index.md` (state tracker)
- `sessions` + `queries` tables: new session/term memory (Phase 2)
- One-shot migration script: seed DB from existing files
- `bun:sqlite` built-in driver (zero new dependencies)
- Single global DB: `~/.zbrain/zbrain.db`
- WAL mode enabled on DB creation
- SHA-256 integrity model preserved (raw_sha256, source_sha256 columns)

## NOT In Scope

- Moving `raw.md` content into the DB (content stays as files, searched by qmd)
- Moving knowledge wiki files (`axioms/`, `mental-models/`, `projects/`, `decisions/`)
- Removing `config.yml` (global config stays YAML — human-editable)
- Vector/embedding search
- any other zbrain feature work

---

## Schema

```sql
PRAGMA journal_mode=WAL;

-- replaces projects.json
CREATE TABLE projects (
  project_root          TEXT PRIMARY KEY,
  workspace             TEXT NOT NULL,
  context_file          TEXT NOT NULL,
  runtimes              TEXT NOT NULL DEFAULT '[]',
  secondary_workspaces  TEXT NOT NULL DEFAULT '[]',
  created_at            TEXT NOT NULL,
  updated_at            TEXT NOT NULL
);

-- replaces source.yaml + _index.md
CREATE TABLE evidence_sources (
  id                    TEXT NOT NULL,
  workspace             TEXT NOT NULL,
  source_type           TEXT NOT NULL,
  origin                TEXT NOT NULL,
  label                 TEXT NOT NULL,
  workspace_at_ingest   TEXT NOT NULL,
  ingested_at           TEXT NOT NULL,
  state                 TEXT NOT NULL,
  raw_filename          TEXT NOT NULL DEFAULT 'raw.md',
  raw_sha256            TEXT NOT NULL,
  source_sha256         TEXT NOT NULL,
  state_updated_at      TEXT NOT NULL,
  PRIMARY KEY (id, workspace)
);

-- NEW: session term memory
CREATE TABLE sessions (
  id                TEXT PRIMARY KEY,
  project_root      TEXT NOT NULL,
  workspace         TEXT NOT NULL,
  started_at        TEXT NOT NULL,
  last_activity_at  TEXT NOT NULL
);

CREATE TABLE queries (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id       TEXT NOT NULL REFERENCES sessions(id),
  query_text       TEXT NOT NULL,
  workspace        TEXT NOT NULL,
  context_file     TEXT NOT NULL,
  retrieved_count  INTEGER NOT NULL DEFAULT 0,
  queried_at       TEXT NOT NULL
);
```

---

## Invariants (unchanged)

- I-1: `raw.md` remains immutable on disk; SHA-256 verified against `raw_sha256` column
- I-2: `workspace_at_ingest` must match active workspace at every state transition
- I-3: QA gate: apply blocked if P0/P1 questions `awaiting_external` or `deferred`
- I-4: `source_sha256` = SHA-256 of all metadata fields (same fingerprint logic, stored in DB column)
- I-5: Migration is non-destructive; existing YAML files survive until removed explicitly

---

## File Impact

### New Files
| File | Purpose |
|------|---------|
| `src/core/db.ts` | DB open, schema creation, WAL mode, schema version |
| `src/core/db-projects.ts` | Project registry CRUD (replaces config.ts project functions) |
| `src/core/db-evidence.ts` | Evidence metadata CRUD (replaces YAML metadata from evidence-store.ts) |
| `src/core/db-sessions.ts` | Session upsert + query insert + recent list (Phase 2) |
| `scripts/migrate-to-db.ts` | One-shot migration: projects.json + source.yaml → DB |

### Modified Files
| File | Change |
|------|--------|
| `src/core/evidence-store.ts` | Remove YAML metadata; keep SHA-256, raw.md write, path construction |
| `src/core/evidence-ingest.ts` | Use `db-evidence.ts` instead of YAML write |
| `src/core/evidence-list.ts` | SELECT from DB instead of parsing `_index.md` |
| `src/commands/setup.ts` | Call `initDb()` during setup |
| `src/commands/ask.ts` | Write query row after retrieval (Phase 2) |
| `src/core/config.ts` | Project registry functions delegated to `db-projects.ts` |

---

## Phases

- **Phase 1 (p8-sqlite-core-db)**: DB init + projects + evidence migration. Independently shippable.
- **Phase 2 (p9-sqlite-session-memory)**: Sessions + queries tables. Additive, no behavior change to existing code.

---

## Done When

### Phase 1
- `~/.zbrain/zbrain.db` created on `zbrain setup`
- `zbrain ingest list` reads from DB, not `_index.md`
- `zbrain learn` writes evidence metadata to DB
- `projects.json` reading falls back to DB for new setups
- `scripts/migrate-to-db.ts` seeds DB from existing files successfully
- All existing tests pass

### Phase 2
- `sessions` and `queries` tables populated after `zbrain ask`
- Session ID is `{project_root}:{YYYY-MM-DD}`
- `SELECT * FROM queries WHERE project_root = ?` returns ask history

---

## Risks

| Risk | Mitigation |
|------|-----------|
| bun:sqlite unavailable in compiled binary | Smoke test: `ZBRAIN_HOME=/tmp/smoke ./dist/zbrain setup && ls /tmp/smoke/zbrain.db` |
| SHA-256 chain broken by DB migration | `verifySourceRecordIntegrity()` logic unchanged; reads from DB row instead of YAML |
| DB corrupted | `raw.md` files survive; re-run migration script to rebuild |
| Concurrent CLI calls | WAL mode + synchronous bun:sqlite = safe for single-machine CLI |

---

## Driver / Tooling

- `bun:sqlite` — built into Bun runtime, synchronous API, zero extra deps
- WAL mode: `db.exec("PRAGMA journal_mode=WAL")`
- Schema version: `PRAGMA user_version = 1` stored in DB
