# Context: Session Term Memory

Phase: p9-sqlite-session-memory
Status: blocked (requires p8-sqlite-core-db)
Spec Link: ../../SPEC-sqlite.md
Roadmap Link: ../../ROADMAP-sqlite.md
Blast Radius: low
Expected Proof: unit (temp DB), manual acceptance (SELECT rows after zbrain ask)

---

## Goal

Populate `sessions` and `queries` tables after every `zbrain ask` call. Enables per-project retrieval history: "what did I ask today in this workspace?"

---

## Scope Boundary

### Allowed Surfaces
- `src/core/db-sessions.ts` (new file)
- `src/commands/ask.ts` (add non-fatal DB write after retrieval)
- `tests/core/db-sessions.test.ts` (new file)

### Forbidden Surfaces
- All other commands (learn, ingest, setup, init, workspace, update)
- `src/core/db.ts` — schema already includes sessions + queries tables from Phase 1
- Knowledge wiki files (axioms/, mental-models/, projects/, decisions/)
- Retrieval logic (`src/core/retrieval.ts`, `src/core/retrieval-ranking.ts`)

---

## Spec Hooks

- `sessions.id` = `{project_root}:{YYYY-MM-DD}` — one session per project per day
- `queries.session_id` REFERENCES sessions(id)
- `queries.context_file` = path to `current-task.md` written by retrieval
- `queries.retrieved_count` = number of results from retrieval
- DB write in ask.ts must be non-fatal: if DB is unavailable, ask still completes

---

## Locked Decisions

- Session granularity: per project per day (not per CLI invocation, not per hour)
- Session ID: `{project_root}:{YYYY-MM-DD}` — deterministic, no UUID
- Non-fatal write: ask.ts wraps DB call in try/catch; failure logs warning, does not throw
- No embedding / no vector recall — this is timestamp + text storage only

---

## Assumptions

- Phase 1 (p8) DB is initialized and `sessions` + `queries` tables exist before Phase 2 runs
- `ask.ts` has access to `RuntimePaths` (already the case via `createCommandContext`)
- `retrieved_count` is available from the `RetrievalContext.results.length` return value

---

## Canonical Refs

- `.kit/planning/SPEC-sqlite.md`
- `.kit/planning/ROADMAP-sqlite.md`
- `src/commands/ask.ts` — current retrieval call site
- `src/core/retrieval.ts` — returns `RetrievalContext` with `results` array
- `src/core/db.ts` (Phase 1) — `initDb` / `openDb`

---

## Rejected Options

- **UUID session ID** — rejected; `{project_root}:{date}` is deterministic and human-readable without a UUID generator
- **Session per CLI invocation** — rejected; too granular, makes recall noisy
- **Embeddings for semantic recall** — explicitly out of scope; plain text + timestamp is sufficient for "what did I ask recently"

---

## Deferred Ideas

- `zbrain history` CLI command to display recent queries (future)
- FTS5 search on `query_text` for "did I ask about X before?" (future)
- Session summary generation from queries (future)

---

## Escalate If

- `ask.ts` imports or architecture has changed since Phase 1 in a way that makes DB injection non-trivial → escalate to plan refresh
- DB write in ask.ts causes any observable latency or error surfaced to user → make it async fire-and-forget
