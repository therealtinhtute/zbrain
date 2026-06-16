# Context: Core DB Layer

Phase: p8-sqlite-core-db
Status: ready
Spec Link: ../../SPEC-sqlite.md
Roadmap Link: ../../ROADMAP-sqlite.md
Blast Radius: high
Expected Proof: unit (temp DB), integration (migrate-to-db.ts), smoke (compiled binary)

---

## Goal

Replace all file-based metadata (projects.json, source.yaml, _index.md) with a single WAL-mode SQLite DB at `~/.zbrain/zbrain.db`. Existing CLI behavior is identical from the user's perspective.

---

## Scope Boundary

### Allowed Surfaces
- `src/core/db.ts` (new)
- `src/core/db-projects.ts` (new)
- `src/core/db-evidence.ts` (new)
- `src/core/evidence-store.ts` (shrink: keep SHA-256, raw.md write, path helpers)
- `src/core/evidence-ingest.ts` (update call sites)
- `src/core/evidence-list.ts` (replace _index.md parser with DB query)
- `src/core/config.ts` (delegate project registry to db-projects.ts)
- `src/commands/setup.ts` (add initDb() call)
- `scripts/migrate-to-db.ts` (new)
- `tests/core/db*.test.ts` (new test files)

### Forbidden Surfaces
- `assets/` — no content changes
- `src/commands/ask.ts` — session memory is Phase 2
- Knowledge wiki files (axioms/, mental-models/, projects/, decisions/)
- `raw.md` content — never moved to DB
- `config.yml` — stays YAML

---

## Spec Hooks

- Replaces: `projects.json` (projects table), `source.yaml` (evidence_sources table), `_index.md` (evidence_sources.state)
- Invariant I-1: raw.md immutable; SHA-256 verified via raw_sha256 column
- Invariant I-2: workspace_at_ingest lock preserved in evidence_sources.workspace_at_ingest
- Invariant I-4: source_sha256 fingerprint stored in DB column, computed identically to YAML version
- Invariant I-5: migration non-destructive; existing YAML files not deleted

---

## Locked Decisions

- Driver: `bun:sqlite` (built-in, zero deps)
- DB location: `~/.zbrain/zbrain.db` (single global file)
- WAL mode: enabled on `initDb()` — `PRAGMA journal_mode=WAL`
- Schema version: `PRAGMA user_version = 1` — guard in `initDb()` to detect stale schema
- `raw.md` stays on filesystem — content never enters DB
- Migration is non-destructive: old YAML/JSON files survive

---

## Assumptions

- `bun:sqlite` is available in `bun build --compile` output — must be smoke-tested before declaring phase done
- Single-process CLI means no real write concurrency — WAL is best practice, not a correctness fix
- Existing `source.yaml` files are all valid YAML with correct SHA-256 checksums before migration runs

---

## Canonical Refs

- `.kit/planning/SPEC-sqlite.md`
- `.kit/planning/ROADMAP-sqlite.md`
- `src/core/evidence-store.ts` — current source of truth for metadata logic
- `src/core/evidence-ingest.ts` — primary consumer of evidence-store
- `src/core/evidence-list.ts` — _index.md parser to be replaced
- `src/core/config.ts` — project registry to be delegated

---

## Rejected Options

- **One DB per workspace** — rejected in favor of one global DB. Easier cross-project queries (sessions), one backup target, simpler initDb() path.
- **`better-sqlite3`** — rejected; `bun:sqlite` is built-in, synchronous, no native bindings needed.
- **Keep source.yaml alongside DB** — rejected for Phase 1 full migration goal. Migration script provides the bridge.

---

## Deferred Ideas

- DB-level FTS5 search on evidence labels/origins (not needed while qmd handles retrieval)
- Vacuum/optimize on zbrain update (future maintenance)
- DB export command to regenerate YAML from DB (useful for debugging)

---

## Escalate If

- `bun:sqlite` is not embedded in compiled binary output → escalate to plan: add better-sqlite3 as fallback
- `verifySourceRecordIntegrity()` fails after migration due to fingerprint mismatch → escalate to brainstorm: re-examine SHA-256 column serialization
- Any existing test suite breaks in a way not obviously traceable to the DB switch → escalate to check
