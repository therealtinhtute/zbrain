# zbrain V2 — Architecture Proposal

> Derived from `AUDIT.md`. V2 exists to fix the three load-bearing gaps:
> **(C1)** the evidence gate is bypassed at retrieval, **(L)** memory can never be forgotten/corrected, **(D)** dual source of truth + no tests.

## Guiding decisions (read this first)

These four decisions resolve the audit's biggest findings and shape every section below.

1. **Files are the durable truth; SQLite is the authoritative operational store, rebuildable from files.**
   All reads/queries go through SQLite (it *is* the primary database for working with the data). But every byte in SQLite is derivable from the markdown on disk via `zbrain reindex`. Writes flow **one direction** — CLI writes the file (atomically), then updates the DB in the same operation. The agent never writes the DB and never maintains a second pointer file. This kills V1's drift (CLI read DB, agent read `projects.json`) and restores "human-readable + portable" (back up the markdown; the DB is a cache).

2. **The searchable wiki and the raw evidence are physically separate, and only the wiki is indexed.**
   `wiki/` (the four tiers) is indexed. `evidence/` (raw ingested sources) is *never* in the search collection. This is the direct fix for **C1** — unverified content can no longer reach an agent through `ask`.

3. **Lexical search moves into SQLite via FTS5 (BM25).**
   Removes the global `qmd` install (onboarding friction), unifies storage, keeps it lexical — **no vectors, no embeddings**. `qmd` and any future engine remain available behind a `RetrievalAdapter` interface.

4. **Memory has a full lifecycle with human-readable tombstones.**
   `active → superseded → archived → forgotten`. Forgetting is recoverable (tombstone + `.trash/`), never a silent hard delete. Conflicts are detected on write, not swallowed.

---

## 1. Architecture Diagram

```
┌──────────────────────────────────────────────────────────────────────────┐
│ AGENTS (Claude Code, Codex, others)                                        │
│   read: per-session context file   write: ONLY via `zbrain` CLI            │
└───────────────┬───────────────────────────────────┬──────────────────────┘
                │ ask / learn / ingest                │ (never touches DB or files directly)
                ▼                                      ▼
┌──────────────────────────────────────────────────────────────────────────┐
│ CLI / COMMAND LAYER  (src/commands)                                        │
│   thin orchestration · @clack UX · one writer · owns file+DB consistency   │
└───────────────┬───────────────────────────────────┬──────────────────────┘
                ▼                                     ▼
┌───────────────────────────────┐   ┌───────────────────────────────────────┐
│ CORE / DOMAIN  (src/core)      │   │ ADAPTERS  (src/adapters)              │
│  · note service (CRUD+lifecycle)│   │  · RetrievalAdapter (FTS5 default,    │
│  · evidence service (ingest)    │◄─►│      qmd optional)                    │
│  · retrieval service            │   │  · IngestAdapter (paste/file/url/...) │
│  · lifecycle/supersede engine   │   │  · RuntimeAdapter (claude/codex)      │
│  · lease/concurrency manager    │   │  · IndexAdapter (FTS5 writer)         │
│  pure logic; takes RuntimePaths │   └───────────────────────────────────────┘
└───────────────┬───────────────────────────────┬──────────────────────────┘
                ▼ (durable truth)                 ▼ (operational store, derived)
┌───────────────────────────────┐   ┌───────────────────────────────────────┐
│ FILESYSTEM  (~/.zbrain)        │   │ SQLITE  (~/.zbrain/zbrain.db, WAL)     │
│  workspaces/<ws>/wiki/*.md     │──►│  notes · note_fts(FTS5) · links       │
│  workspaces/<ws>/evidence/*    │   │  evidence · workspaces · projects     │
│  workspaces/<ws>/.trash/*      │   │  leases · sessions · schema_meta      │
│  (YAML frontmatter + body)     │◄──│  rebuildable via `zbrain reindex`     │
└───────────────────────────────┘   └───────────────────────────────────────┘
        ▲  source of truth / backup unit          ▲  fast queries, BM25, joins
        └───────────  reindex (files → DB) is the ONLY rebuild direction ──────┘
```

---

## 2. Domain Model

Kept deliberately small — the audit warned against over-engineering the periphery. The **Note** is the atomic unit of memory; we do *not* introduce a separate fine-grained "fact graph" in V2 (that's a future option, not a need).

| Entity | Meaning | Identity | Mutability |
|---|---|---|---|
| **Workspace** | Isolation boundary (one domain). | `name` (slug) | Created/renamed, never silently merged |
| **Note** | One wiki document = one memory. Has a tier, status, body, and source links. | stable `id` (ULID) in frontmatter | Mutable via supersede; content-addressed by `content_sha` |
| **Source (Evidence)** | Immutable raw material a note was derived from. | `id` (`YYYYMMDD-slug`) | Immutable after ingest (SHA-locked) |
| **Link** | Typed relation between notes: `supersedes`, `references`, `contradicts`. | `(from_id, type, to_id)` | Derived from frontmatter |
| **Lease** | Advisory write lock on a note path, for multi-agent safety. | `(workspace, path)` | TTL'd, auto-expires |
| **Session** | A single agent's retrieval context. | `session_id` | Per-agent; isolates context files |

### Note shape (frontmatter is the contract)

```yaml
---
id: 01J8Z3K7QHe...            # ULID, stable forever
tier: axioms                  # axioms | mental-models | projects | decisions
status: active                # active | superseded | archived | forgotten
created_at: 2026-06-20T...
updated_at: 2026-06-20T...
content_sha: 9f2c...          # sha256(body) — optimistic-lock + integrity
sources: [20260620-paste-x]   # evidence ids this note is derived from
supersedes: [01J8...]         # note ids this replaces (lifecycle)
superseded_by: null           # set when this note is replaced
review_by: 2027-06-20         # optional staleness signal (no auto-delete)
---
# Title
Body in plain markdown. Every claim cites a source id inline: [src:20260620-paste-x].
```

**Why notes, not facts:** a solo dev can reason about one-file-per-memory. Stable IDs + `sources`/`supersedes` links give us 80% of a knowledge graph's value (lineage, lifecycle, contradiction tracking) at 10% of the complexity, and leave the door open (§8).

---

## 3. Storage Model

### Filesystem (durable truth, human-readable, the backup unit)

```
~/.zbrain/
  config.yml                         # global config (human-readable)
  zbrain.db                          # DERIVED cache — safe to delete & rebuild
  workspaces/
    <workspace>/
      workspace.md                   # purpose + operating rules
      wiki/                          # ◄── THE ONLY INDEXED TREE
        axioms/*.md
        mental-models/*.md
        projects/*.md
        decisions/*.md
      evidence/                      # ◄── NEVER indexed (quarantine)
        sources/<id>/raw.md          # immutable
        sources/<id>/source.yaml     # immutable metadata (RESTORED in V2 — human-readable)
        reviews/<id>.md              # human-readable review record
      .trash/                        # forgotten notes (recoverable)
        <id>.md
  projects/<hash>/sessions/<sid>.md  # per-session retrieval context (multi-agent safe)
```

Two changes vs V1 worth calling out:
- **`source.yaml` is restored and actually written** (V1 dropped it; metadata leaked into the opaque DB). Evidence is now fully reconstructable from disk → "human-readable data" holds again.
- **`wiki/` is a real subtree.** Indexing targets `wiki/` only, so `evidence/` and `.trash/` are structurally unindexable. Closes **C1** by construction, not by config.

### SQLite (operational store — every column rebuildable from files)

```sql
CREATE TABLE notes (
  id            TEXT PRIMARY KEY,        -- ULID from frontmatter
  workspace     TEXT NOT NULL,
  path          TEXT NOT NULL,           -- relative to wiki/
  tier          TEXT NOT NULL,           -- explicit, NOT inferred from path
  status        TEXT NOT NULL,           -- active|superseded|archived|forgotten
  title         TEXT,
  content_sha   TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL,
  review_by     TEXT,
  UNIQUE(workspace, path)
);
CREATE VIRTUAL TABLE note_fts USING fts5(   -- lexical BM25, no embeddings
  title, body, content='', tokenize='unicode61'
);                                          -- rowid ↔ notes via a map table
CREATE TABLE note_fts_map (rowid INTEGER PRIMARY KEY, note_id TEXT);

CREATE TABLE links (from_id TEXT, type TEXT, to_id TEXT,
  PRIMARY KEY(from_id, type, to_id));

CREATE TABLE evidence (id TEXT, workspace TEXT, source_type TEXT, origin TEXT,
  label TEXT, state TEXT, raw_sha256 TEXT, source_sha256 TEXT,
  ingested_at TEXT, state_updated_at TEXT, PRIMARY KEY(id, workspace));

CREATE TABLE workspaces (name TEXT PRIMARY KEY, created_at TEXT);
CREATE TABLE projects   (project_root TEXT PRIMARY KEY, workspace TEXT,
  secondary_workspaces TEXT, runtimes TEXT, created_at TEXT, updated_at TEXT);

CREATE TABLE leases (workspace TEXT, path TEXT, holder TEXT, acquired_at TEXT,
  expires_at TEXT, PRIMARY KEY(workspace, path));

CREATE TABLE sessions (id TEXT PRIMARY KEY, project_root TEXT, workspace TEXT,
  started_at TEXT, last_activity_at TEXT);

CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT);  -- version + migration log
```

**Tier is an explicit column from frontmatter**, not a substring of the path (V1's `classifyByTier` was fragile and let `evidence/.../raw.md` rank P2). No unused tables ship — drop V1's speculative `queries` table until usage-scoring is real.

---

## 4. Memory Lifecycle Model

```
                 ingest            review/apply
  (external) ───────────► evidence ───────────► NOTE:active
                                                   │  │  │
                          new note supersedes ─────┘  │  └──── review_by passes → STALE flag (not deleted)
                                   ▼                  │
                            NOTE:superseded           │ user/agent archives
                            (kept, links to successor)▼
                                                NOTE:archived ──── forget ───► NOTE:forgotten
                                                (out of retrieval)            (moved to .trash/, tombstone)
                                                                                   │ restore
                                                                                   ▼
                                                                              NOTE:active
```

Rules:
- **Update = supersede, never silent overwrite.** Writing a corrected note creates a new note with `supersedes: [old_id]`; the old note flips to `status: superseded`, `superseded_by: <new_id>`. Both stay on disk; only the successor is retrievable. This fixes V1's last-writer-wins ([evidence-apply.ts:123] in V1).
- **Conflict detection on write.** If a new note targets a path/claim that an `active` note already covers and is *not* declared as a supersede, the CLI refuses and surfaces the conflict. No silent contradictory memory.
- **Forgetting is recoverable.** `zbrain forget <id>` writes a one-line tombstone (`# forgotten <id> at <ts> reason: ...`), moves the file to `.trash/`, removes it from the index. `zbrain restore <id>` reverses it. Hard delete is an explicit `--purge`.
- **Staleness ≠ deletion.** `review_by` in the past raises a flag in `zbrain doctor`/`list`; it never auto-removes. Humans/agents decide.
- **Optimistic locking.** Every mutation passes the `content_sha` it read; mismatch ⇒ "note changed under you, re-read" instead of clobber (multi-agent safety, §6).

---

## 5. Retrieval Pipeline

```
ask("question", session_id)
  1. resolve workspace        (DB; single source of truth — no projects.json read)
  2. parse query              → keywords + tier hints + optional @secondary tags
  3. FTS5 BM25 search         WHERE status='active' AND workspace=? (wiki only)
  4. tier-WEIGHTED score      final = bm25 * tier_weight[tier]   (NOT hard tier sort)
  5. fetch bodies for top-N
  6. write context            → projects/<hash>/sessions/<session_id>.md (atomic)
  7. return citations         note ids + paths + source ids
  8. empty?                   record gap, suggest learn/research, NEVER answer from memory
```

Two corrections vs V1:
- **Tier-weighted, not tier-absolute.** V1 sorted tier-first regardless of BM25, so a barely-relevant axiom outranked a bull's-eye decision. V2 multiplies BM25 by a tier weight (e.g. axioms 1.5, mental-models 1.3, projects 1.1, decisions 1.0) — priority *and* relevance.
- **`status='active'` filter** means superseded/archived/forgotten notes are structurally excluded. Retrieval only ever sees current, verified memory.

Auto-index guarantee: if the workspace FTS is empty/stale, `ask` triggers an incremental reindex before searching, so the **first `ask` never hard-errors** (V1's worst first-run bug).

---

## 6. Sync / Indexing Pipeline

**Principle: files → DB is the only rebuild direction. The DB is never the truth.**

- **On write (incremental):** CLI writes the `.md` atomically (temp + rename), parses frontmatter, upserts the `notes` row + `note_fts` entry + `links` in one SQLite transaction. O(1) per note — fixes V1's full-workspace reindex on every apply.
- **`zbrain reindex [--workspace]`:** walk `wiki/`, compare each file's `content_sha` to the DB row, upsert changed/new, delete rows whose files vanished. Deterministic; the DB can be `rm`'d and fully rebuilt. This is also the **disaster-recovery + migration** primitive.
- **`zbrain doctor`:** the reconciliation + health command (new, top UX ROI from audit): verifies DB↔files consistency, finds orphaned evidence (DB row, missing `raw.md`), stale `review_by`, expired leases, broken supersede links.
- **Crash safety:** because the file is the truth and written first, a crash before the DB update just means the next `reindex`/`doctor` picks it up. No orphan-with-no-recovery (V1's ISSUE-007).
- **Optional watch mode** (`zbrain watch`) for users who hand-edit markdown in an editor — fs events → incremental reindex.

---

## 7. Extension System Design

Keep it as **typed interfaces + a small registry**, not a dynamic plugin loader (a solo dev must keep it in their head). Four seams, all already implied by V1's structure:

```ts
interface RetrievalAdapter {            // default: Fts5Adapter; alt: QmdAdapter
  search(o: {workspace: string; query: string; limit: number}): RawResult[];
}
interface IndexAdapter {                // writes the search index on note change
  upsert(note: Note): void; remove(id: string): void; rebuild(ws: string): void;
}
interface IngestAdapter {               // source types: paste, file, url, clipboard...
  id: string; accept(input): boolean; toRawMarkdown(input): Promise<RawSource>;
}
interface RuntimeAdapter {              // claude | codex | future; owns init injection
  id: string; inject(paths, selection): InjectResult;
}
```

- Adapters are registered in a static `registry.ts`; config selects which is active (`retrieval.engine: fts5 | qmd`).
- A "plugin" in V2 = adding a file that implements an interface + a registry line. No runtime code loading, no sandboxing burden, no security surface (audit-aligned: don't add complexity the MVP doesn't need).
- Future dynamic plugins can layer on the same interfaces without changing callers.

---

## 8. Migration Path for Future Semantic Search

The constraints forbid embeddings/vectors **now**. V2 is built so adding them later is additive, not a rewrite:

1. **Stable note IDs already exist** (ULID in frontmatter) — the join key for any future embedding row. Adding them retroactively is the expensive part; V2 pays it upfront for free.
2. **`RetrievalAdapter` is the seam.** A future `HybridAdapter` runs FTS5 (BM25) and a vector recall, then merges/re-ranks. Existing callers (`ask`) don't change.
3. **Sidecar storage, opt-in.** Embeddings live in a new `note_vectors` table (or a sidecar `embeddings.db`) keyed by `note_id` + `content_sha`. If the local-first/no-vector stance ever relaxes, this is a single additive migration — the four wiki tables are untouched.
4. **Lazy + local.** Embeddings computed on `reindex` only when the adapter is enabled, ideally via a local model so the local-first promise survives. Default stays lexical.
5. **No lock-in either way.** Because files are truth, switching engines = `reindex` under a new adapter. Nothing is trapped in the DB.

> V2's job is to make semantic search a *toggle later*, not to build it now.

---

## 9. Risks and Tradeoffs

| Decision | Upside | Risk / cost | Mitigation |
|---|---|---|---|
| FTS5 instead of qmd default | Zero external dep, unified storage, BM25 native | Lose qmd-specific features; FTS5 tokenizer is simpler | Keep `QmdAdapter` selectable; FTS5 covers MVP retrieval |
| Files = truth, DB = derived | No drift, portable backup, trivial recovery | Every write does file I/O + DB txn; large workspaces cost more on full `reindex` | Incremental indexing on write; full reindex is rare/explicit |
| Supersede instead of overwrite | Full lineage, no lost history, no silent conflicts | `.trash/` and superseded notes accumulate | `doctor` reports growth; `--purge` for real deletion |
| Optimistic locking + leases | Lock-free reads, safe concurrent writes | Write can fail under contention; lease TTL tuning | Clear "re-read and retry" UX; short TTL + auto-expiry |
| Note as the unit (no fact graph) | Solo-dev-comprehensible | Coarser than claim-level memory | Stable IDs + links leave a clean upgrade path (§8) |
| Per-session context files | No multi-agent clobber | More small files under `projects/<hash>/sessions/` | GC sessions idle > N days in `doctor` |
| Explicit tier in frontmatter | Robust, not path-fragile | Author must set it (or ingest defaults it) | `learn`/`apply` default tier; `doctor` flags missing |

**Honest tension:** "SQLite remains the primary database" vs "files are truth." V2 resolves it by making SQLite the *authoritative operational store* (all queries go through it, it's primary for working with data) while keeping it *deterministically derived* from files. If you instead want SQLite to be the literal system of record (files exported from DB), that flips the write direction and reintroduces the human-readable/backup risk the audit flagged — I recommend against it.

---

## 10. Suggested Folder Structure

### Repository

```
src/
  index.ts                       # entry + commander wiring
  commands/                      # thin orchestration only (one writer)
    setup.ts init.ts learn.ts ingest.ts ask.ts
    note.ts                      # supersede / archive / forget / restore
    doctor.ts reindex.ts workspace.ts
  core/
    note-service.ts              # CRUD + lifecycle (supersede/forget)
    evidence-service.ts          # ingest, immutability, review
    retrieval-service.ts         # parse → search → weight → context
    lifecycle.ts                 # state machine + transitions
    conflict.ts                  # write-time conflict detection
    concurrency.ts               # leases + optimistic content_sha checks
    indexer.ts                   # incremental + full reindex (files→DB)
    frontmatter.ts               # parse/serialize note frontmatter
    runtime-paths.ts fs.ts
  adapters/
    retrieval/fts5-adapter.ts    # default
    retrieval/qmd-adapter.ts     # optional
    ingest/{paste,file,url}-adapter.ts
    runtime/{claude,codex}-adapter.ts
    registry.ts
  db/
    schema.sql migrations.ts db.ts
    notes-repo.ts evidence-repo.ts projects-repo.ts leases-repo.ts
  schemas/                       # zod: frontmatter, config, project
  generated/bundled-assets.ts
assets/                          # source of truth for runtime content (unchanged model)
  engine/ skills/ agents/ templates/ workspaces/
tests/                           # ◄── RESTORED: invariants pinned
  lifecycle.test.ts retrieval-ranking.test.ts concurrency.test.ts
  indexer-roundtrip.test.ts conflict.test.ts isolation.test.ts
scripts/ docs/ LICENSE CONTRIBUTING.md
```

### Runtime (`~/.zbrain`) — see §3.

---

## What changes vs V1 (one-glance)

| Audit finding | V1 | V2 fix |
|---|---|---|
| C1: evidence gate bypassed | whole workspace indexed | only `wiki/` indexed; `evidence/` structurally separate |
| No forgetting | `archived` unimplemented; overwrite wins | full lifecycle + supersede + recoverable forget + conflict detection |
| Dual source of truth | CLI=DB, agent=`projects.json` | one writer; DB derived from files; agent reads session file/CLI only |
| Human-readable regressed | metadata only in DB | `source.yaml` restored; files reconstruct everything |
| First-run hard error | index only built on apply | auto/incremental index; `ask` self-heals |
| Multi-agent unsafe | shared context file clobber | per-session files + leases + optimistic locking |
| qmd onboarding friction | global install required | FTS5 default; qmd optional |
| No tests | suite deleted | `tests/` pins the invariants |
| Fragile tier inference | substring of path | explicit `tier` in frontmatter |
| Dead/speculative schema | `queries`/`sessions` unused | drop until needed; `sessions` only when per-session ships |
```
