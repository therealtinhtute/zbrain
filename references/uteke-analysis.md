---
title: "Uteke v0.11.0 — Architecture Analysis"
description: "Source-level analysis of Uteke, a Rust local-first memory engine for AI coding agents, with design decisions transferable to a Go implementation."
source_title: "Uteke"
source_url: "https://github.com/codecoradev/uteke"
source_kind: "open-source implementation and technical documentation"
source_repository: "codecoradev/uteke"
source_version: "0.11.0"
source_commit: "afb98c6"
accessed_at: "2026-08-03"
fetch_method: "git clone, full source read (~39k lines Rust, 5 crates)"
status: provisional
tags: [memory-engine, sqlite, fts5, hnsw, usearch, rrf, onnx, mcp, rust, go, local-first]
---

# Uteke v0.11.0 — Architecture Analysis

## Kết luận

Copy Uteke's retrieval shape; reject its storage topology.

Uteke keeps vectors in a separate `usearch` HNSW file next to SQLite. Roughly a
third of the hardening in the codebase exists to fight the resulting desync:
cross-process file locks, save-retry loops, `uteke repair`, and Windows I/O
workarounds. A Go implementation can collapse that into one SQLite file and
delete the entire bug class.

Their own measurements say the trade costs nothing: recall latency is flat at
~40ms from 100 to 10,000 memories, and HNSW contributes under 1ms of it. The
floor is embedding inference, not search.

## Provenance

The URL commonly cited for this project, `github.com/codecora/uteke`, returns
404 (`gh api repos/codecora/uteke` → `Not Found`). The real repository is
**`github.com/codecoradev/uteke`**, found via `gh api "search/repositories?q=uteke"`.

Analyzed at commit `afb98c6`, tag v0.11.0. Approximately 39k lines of Rust
across five crates: `uteke-core`, `uteke-cli`, `uteke-server`, `uteke-mcp`,
`docgen`.

Every claim below is tagged:

| Tag | Meaning |
|---|---|
| **[F]** | Fact — read directly in source at the cited line |
| **[I]** | Inference — reasoning, could be wrong |
| **[U]** | Undiscoverable from the repository, with a suggested way to find out |

The binary was not compiled or executed. Nothing here is runtime-verified.
Where that matters — the column-index defect in §10 especially — it is stated
explicitly.

---

## 1. Problem Framing

### Observation

**What the marketing says [F].** `README.md` — "brain for your AI."
`docs/architecture.md:1` — "Everything lives in `~/.codecora/uteke/` — fully
local, fully yours."

**What the code says [F].** Effort allocation is the honest signal. Ranked by
lines of defensive code:

1. **Cross-process contention.** `memory/vector.rs:1-13` — module doc:
   *"Cross-process safety (#543): Each VectorIndex acquires an exclusive file
   lock (via fs2) on the .usearch file during construction."* Lock held for the
   whole index lifetime (`vector.rs:74-77`, released `382-406`).
2. **Windows filesystem hostility.** `vector.rs:9-13` — *"Windows compatibility
   (#647, #684): Both `save()` and `load()` use buffer-based serialization to
   bypass usearch's C++ file I/O (`fopen`, `fread`, `mmap`) which has
   Windows-specific issues (MAX_PATH, file lock conflicts, AV interference)."*
3. **Databases arriving in broken states.** Three `ensure_*` repair functions
   plus a datetime repair run on *every* open — `schema.rs:130-166`, `241-320`,
   `171-239`. Issue refs #500, #544, #549.
4. **Embedding failures on write.** `operations.rs:16-56` — three attempts,
   200/400/800ms backoff; on final failure the memory is stored **without** an
   embedding and the log tells the user to run `uteke repair` (#621).

**Primary user [F/I].** `uteke-server/src/main.rs:185` binds `127.0.0.1` by
default [F]. `uteke-server/src/context.rs:35-38` — no token configured means
auth entirely disabled [F]. `docs/architecture.md:11` — the CLI "auto-routes to
server if running" [F]. `docs/roadmap.md` header: *"Demand-gated — we build what
people actually use"* [F].

**Constraints [F].** Offline at query time — ONNX inference is local, the only
network call is the one-time model download (`embed/engine.rs:236-374`).
Zero-config — no API key required. Privacy by locality, not cryptography: no
encryption at rest anywhere, only `0700` directories and `0600` files
(`engine.rs:74-79, 422-430`).

### Inference

The defensive-code profile is not a distributed-systems profile. Nobody writes
cross-process file locks and Windows Defender workarounds for a server product.
That list is what you get when many short-lived CLI processes hammer one file on
one laptop — `xargs -P5 uteke remember`, a Claude Code MCP session, and a
`uteke recall` in another terminal, all at once [I].

So the primary user is an **individual developer**, and the actual pain point is
not "agents forget" — it is "agents forget, and every workaround needs a server
I have to run." Uteke's differentiator is that there is nothing to run [I].

Server mode exists but is a secondary deployment shape: localhost default,
optional auth, CLI auto-routing [I]. Enterprise is not a target — no SSO, no
RBAC, no audit trail that survives (§7), no encryption.

Constraint ranking, most to least binding [I]: (1) zero-config offline,
(2) cross-platform including Windows, (3) survive concurrent processes,
(4) latency — a distant fourth, given the flat 40ms floor in §9 that nobody
optimized.

### Relevance to Go

The concurrency model is the first architectural decision, and it is
load-bearing. Two shapes:

| Shape | You inherit | You avoid |
|---|---|---|
| Process-per-invocation (Uteke) | file locks, `SQLITE_BUSY` storms, index-desync repair | daemon lifecycle, socket management |
| Daemon + thin client | daemon lifecycle, IPC | most of Uteke's defensive-code list |

Go pushes toward the daemon: goroutines, channels, `net/http`, and graceful
shutdown are all stdlib-easy, and a single long-lived writer makes SQLite
trivially safe. Uteke half-committed (`docs/architecture.md:11`) and pays for
both.

A `database/sql` warning: Go's pool opens multiple connections by default. With
SQLite that means concurrent writers and `SQLITE_BUSY` inside one process. For
process-per-invocation, set `db.SetMaxOpenConns(1)` for the writer. For a
daemon, one writer connection plus a read pool with WAL.

The `ZBRAIN_HOME` convention already matches their `UTEKE_HOME`
(`lib.rs`, `uteke_home()`), which is correct for test isolation.

### Alternative

**Full daemon commitment.** `zbrain serve` owns the store; the CLI is a thin
Unix-socket client; the MCP server is a mode of the daemon, not a separate
binary. Auto-spawn on first CLI use, idle-timeout shutdown. This deletes the
cross-process problem class rather than defending against it. Cost: a lifecycle
to manage and a socket path convention.

**Middle ground.** Keep process-per-invocation but route every write through a
single advisory-locked section, and make reads lock-free by keeping vectors in
SQLite (§3). WAL gives concurrent readers for free; Uteke forfeits that by
taking an exclusive lock even for reads.

### Open Question

**[U] Were #543, #621, #647, #684 filed by real users or written defensively?**
This changes how much hardening to pre-emptively copy. *How to find out:* those
issue numbers are in the source comments — read the threads at
`github.com/codecoradev/uteke/issues/543` and similar. A user-reported stack
trace means it is real; a self-filed "harden X" means it is speculative.

**[U] Does anyone actually run `uteke-serve`?** *How to find out:* check
release-asset download counts via `gh api repos/codecoradev/uteke/releases`, or
search issues for `uteke-serve`. Near-zero server-mode bug reports means it is
dead weight.

---

## 2. Core Mental Models

### Observation

**Memory is a typed row [F].** Not a cache (no eviction by pressure), not an
event log (§7), not primarily a graph. `memories` has 20 columns (`store.rs`,
SCHEMA constant):

```text
id, content, embedding, tags, metadata, created_at, updated_at,
namespace, access_count, last_accessed, deprecated, valid_from,
valid_until, memory_type, importance, pinned, content_type,
slug, source, source_type
```

**Nine types, auto-inferred on write [F]** (`operations.rs:108-152`):
`fact, procedure, preference, decision, context, note, insight, reference,
event`. Pattern-based classification; ambiguous input silently falls back to
`fact` (`operations.rs:145-152`).

**Importance is computed, not declared [F]** (`store.rs:274-328`,
`recompute_importance`):

```text
importance = 0.3·access_score + 0.3·recency_score
           + 0.2·connectivity + 0.2·pinned
recency_score = e^(-ln2 · days_since_access / 30)     # 30-day half-life
pinned ⇒ importance = 1.0                              # hard override
```

**Relationships are modeled three ways at once [F]:**

| # | Mechanism | Tables | Schema ver | Populated by |
|---|---|---|---|---|
| 1 | Typed memory→memory edges | `memory_edges` | v8 | auto on `remember()`: `similar_to` at cos ≥0.80, `possible_duplicate` at ≥0.92 |
| 2 | Entity knowledge graph | `graph_nodes`, `graph_edges` | v7 | entity extraction; separate ID space |
| 3 | Tags | `memories.tags` (JSON) **and** `memory_tags` (junction) | v5 | dual-write on every insert |

`schema.rs:475-478` [F]: the JSON `tags` column is retained *"for backward
compat"* while `memory_tags` is the queryable form. Both are written on every
insert.

`schema.rs:626-676` [F]: migration v8 backfills `memory_edges` from a legacy
`metadata.relationships` JSON blob — relationships were previously stuffed into
the metadata column.

**Documents are a fourth, disjoint model [F]** (`schema.rs:780-851`):
`documents` carries both adjacency (`parent_id`) and a materialized path
(`path`, `depth`, `sort_order`), depth capped at 10. `document_chunks` holds the
embedded pieces. Documents have their own FTS index, their own MCP tools, and
their own room-linkage table (`room_documents`, v15).

**Namespaces are a flat string column [F].** `namespace = None` in a recall
means search all namespaces (`operations.rs:427`, issue #448).

**Ownership philosophy [F].** Everything under `~/.codecora/uteke/`.
Export/import is JSONL. Grep across `docs/roadmap.md` and `docs/comparison.md`
for `sync|replicat|litestream|crdt` returns zero hits.

### Inference

These four models accreted; they were not designed together [I]. The version
stamps tell the story: tags (v5) → entity graph (v7) → memory edges (v8) →
documents (v11) → doc hierarchy (v12) → global docs (v13) → room-docs (v15).
Each solved a felt need; none replaced its predecessor. The v8 backfill from
`metadata.relationships` is a smoking gun — relationships lived in an untyped
JSON blob before someone promoted them to a table [I].

The tag dual-write is migration debt that never got a phase 2 [I]. The standard
sequence is write-both → backfill → read-from-new → stop-writing-old →
drop-old. They stopped at step 3, five schema versions ago.

Two graph systems coexist because neither won [I]. `graph_nodes`/`graph_edges`
models entities ("Postgres", "auth service"); `memory_edges` models memories.
That is a defensible distinction, but the evidence it is not earning its keep is
that graph reranking is opt-in via `RecallStrategy::Graph`
(`operations.rs:569-579`) rather than folded into the default path.

Nine memory types is more taxonomy than the retrieval path consumes [I].
`memory_type` participates in FTS5 (added in v14) and can be filtered on, but
nothing in the ranking pipeline weights types differently. Auto-inference
silently defaulting to `fact` means the real distribution is probably heavily
skewed — nine buckets doing the work of two or three.

### Relevance to Go

**Pick one edge model and one tag model on day one.** This is the cheapest large
win available, because it is free now and expensive later.

A unified edge table serves everything:

```sql
CREATE TABLE edges (
  src_type   TEXT NOT NULL,   -- 'memory' | 'entity' | 'doc'
  src_id     TEXT NOT NULL,
  dst_type   TEXT NOT NULL,
  dst_id     TEXT NOT NULL,
  rel        TEXT NOT NULL,   -- 'similar_to' | 'supersedes' | 'mentions' | …
  weight     REAL DEFAULT 1.0,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (src_type, src_id, dst_type, dst_id, rel)
);
```

One traversal implementation, one index strategy, one place to reason about
cascade deletes.

**Tags: junction table only, from commit one.** `tags(memory_id, tag)` with an
index on `tag`. Never a JSON column.

**A zbrain-specific warning.** The layout in `CLAUDE.md` already declares two
content spaces:

```text
wiki/     axioms/ mental-models/ projects/ decisions/
evidence/ sources/ analysis/ qa/ applied/ archive/
```

That is structurally similar to Uteke's memories-vs-documents split. Decide now
whether `wiki/` and `evidence/` share one storage model with a discriminator
column, or are genuinely separate tables. Uteke chose separate, and the cost is
visible: duplicated FTS setup, duplicated search code, duplicated MCP tools
(seven `uteke_doc_*` alongside `uteke_remember`/`uteke_recall`), and a fourth
model to keep in sync. Recommendation: one `items` table with a `kind` column,
one FTS index, one search path, one set of tools.

**Types: four or five, not nine.** `fact | decision | procedure | reference`
covers the real retrieval distinctions. Splitting later is easy; un-splitting
requires a migration.

**Steal the importance formula** (`store.rs:274-328`) — the weighted
access/recency/connectivity/pinned blend with a 30-day exponential half-life is
well-shaped and portable. But not their implementation: `recompute_importance()`
loads every memory into RAM. In Go, do it in SQL as a batched
`UPDATE ... FROM (SELECT ...)`, or incrementally on access.

### Alternative

**Drop the entity knowledge graph at v1.** It is the least-proven of the four
models (opt-in reranking, separate ID space, unvalidated entity extraction
quality). Ship memory↔memory edges only; add entities when a user asks "what do
I know about X" and tag-filtering demonstrably is not enough.

**Or invert it:** make the entity graph primary and memories the edge payload.
That is the Zep/Graphiti bet — retrieval becomes graph traversal seeded by
vector search, not vector search decorated with graph signals. Materially
different product, much more machinery, only worth it if queries are relational
("what changed about auth since the migration?") rather than lookup-shaped
("what did I decide about auth?").

### Open Question

**[U] What is the actual distribution of `memory_type` in real stores?** If 90%
are `fact`, the taxonomy is decorative and three types suffice. *How to find
out:* their own CLI answers it — `uteke stats` on any populated store, or
`SELECT memory_type, COUNT(*) FROM memories GROUP BY 1`.

**[U] Does `memory_edges` auto-linking improve recall, or just add rows?** Every
`remember()` runs a vector search and writes edges at cos ≥0.80 — write
amplification on every insert for a signal only consumed in opt-in graph mode.
*How to find out:* ablation. Run their benchmark harness (`benchmarks/`) with
graph rerank on and off against a fixed query set. If the delta is noise, skip
auto-linking in Go and reclaim insert throughput (relevant to §9's 3.6× insert
degradation).

---

## 3. Storage Architecture

### Observation

**SQLite, statically compiled [F].** `uteke-core/Cargo.toml:20`:

```toml
rusqlite = { version = "0.40", features = ["bundled"] }
```

`bundled` compiles SQLite's amalgamation into the binary — no system
`libsqlite3`, no version skew.

**Connection configuration is exactly three pragmas [F]**
(`store.rs:194-199`):

```rust
conn.execute_batch("PRAGMA journal_mode=WAL;")?;
conn.execute_batch("PRAGMA busy_timeout=5000;")?;
conn.execute_batch("PRAGMA foreign_keys=ON;")?;
```

One `Connection` per `Uteke` instance, no pool (`store.rs:179-181`).

**Notably absent [F]:** `synchronous`, `cache_size`, `temp_store`, `mmap_size`.
All left at SQLite defaults — meaning `synchronous=FULL`, an fsync per commit.

**Schema — 12 tables plus one virtual [F]** (`store.rs` SCHEMA constant,
`CURRENT_SCHEMA_VERSION = 15` at `store.rs:176`):

| Table | Introduced | Role |
|---|---|---|
| `memories` | v1 | 20-column core row |
| `schema_version` | v1 | migration ledger |
| `memories_fts` | v2 | FTS5 external-content virtual table |
| `rooms`, `room_memories` | v3 | multi-agent sharing |
| `memory_tags` | v5 | tag junction |
| `graph_nodes`, `graph_edges` | v7 | entity knowledge graph |
| `memory_edges` | v8 | typed memory↔memory |
| `timeline_events` | v9 | append-only audit |
| `documents`, `document_chunks` | v11 | document tree |
| `room_documents` | v15 | doc↔room |

**Embeddings are stored twice [F].**

*Copy 1* — a little-endian f32 BLOB in `memories.embedding`
(`store.rs:331-351`):

```rust
pub(crate) fn serialize_embedding(embedding: &[f32]) -> Vec<u8> {
    let mut blob = Vec::with_capacity(embedding.len() * 4);
    for &val in embedding { blob.extend_from_slice(&val.to_le_bytes()); }
    blob
}
```

768 dims × 4 bytes = **3,072 bytes per memory**, uncompressed, unquantized.

*Copy 2* — a usearch HNSW index in a separate file, plus a separate `.keys` TSV
sidecar mapping usearch's u64 keys to UUIDs (`vector.rs:39-54, 204-210`). Index
config:

```rust
IndexOptions {
    dimensions: dims,
    metric: MetricKind::Cos,
    quantization: ScalarKind::F32,
    ..Default::default()
}
```

**Persistence is atomic-rename [F]** (`vector.rs:188-215`):
`save_to_buffer()` → `std::fs::write(tmp)` → `rename(tmp, final)`. The `.keys`
sidecar gets the same treatment. Critically, `save()` serializes and rewrites
the entire index, and it is called per insert.

**Migration strategy [F]:**

- 15 forward-only steps, dispatcher at `schema.rs:334-393`
- each wrapped in a transaction, version stamped after success
- idempotent via `column_exists()` guards before every `ALTER`
- newer-than-supported DB is a hard error with upgrade instructions
  (`schema.rs:121-127`)
- **fresh databases stamp `CURRENT_SCHEMA_VERSION` directly**, skipping all
  migration functions
- indexes live in a separate `SCHEMA_INDEXES` constant applied after migrations,
  because indexes on migration-added columns fail on old DBs (`store.rs`, #492)
- v14 required drop + recreate + full rebuild of `memories_fts` because FTS5 has
  no `ALTER TABLE ADD COLUMN`

**Repair-on-every-open [F]** (`schema.rs:130-166`), with candid comments:

- `ensure_documents_has_children()` — #500: *"a partially-migrated DB may have
  schema_version=12 without the column"*
- `ensure_documents_fts()` — #549: migration used best-effort
  `let _ = execute(...)`, so silent failure left the table absent
- `fts5_exists()` → `init_fts5()` + `rebuild_fts5()` — #544: fresh databases
  skip all migrations, so FTS5 was never created
- `repair_datetime_timezones()` (`schema.rs:171-239`) — RFC3339 strings written
  without `+00:00` crashed `load_all()` on parse

**Concurrency [F]:**

- *In-process:* `RwLock<VectorIndex>` (#209 — concurrent readers),
  `Mutex<Option<Box<dyn Embedder>>>` around the ONNX session
- *Cross-process:* an exclusive `fs2` lock on the `.usearch` file, acquired at
  construction, held for the object's lifetime (`vector.rs:74-77, 382-406`)
- *SQLite:* WAL plus `busy_timeout=5000`
- *Write ordering in `remember_precomputed`* (`operations.rs`): acquire index
  write-lock first, then write SQLite, with the comment that orphaned index
  entries are harmless since SQLite is truth; index save retried 3×; write-lock
  explicitly dropped before `auto_link_cosine` to avoid deadlock (#442)
- *Write ordering in `update_memory`* (`operations.rs:1190-1200`): SQLite first,
  then vector index — the opposite order

### Inference

**Why SQLite [I]:** it is the only embedded store that gives ACID plus a mature
full-text engine plus zero operational surface in one file. Alternatives they
would have weighed and rejected:

| Rejected | Why [I] |
|---|---|
| Postgres + pgvector | requires a server — kills the value proposition |
| RocksDB / sled | no SQL, no FTS; you write your own query layer and secondary indexes |
| Qdrant / LanceDB embedded | vector-first, weak or absent lexical search; would still need SQLite alongside |
| Plain files + in-memory index | no ACID, no concurrent access, full rebuild on start |
| DuckDB | analytics-shaped; weaker concurrent-write story; FTS less mature |

`bundled` specifically is a distribution decision — see §8, where they lost
static linking to ONNX anyway.

**Why usearch instead of a SQLite vector extension [I]:** timing. `sqlite-vec`
only reached usable maturity in 2024; their vector index predates it (v1).
usearch gave them real HNSW with Rust bindings immediately. The decision was
probably correct when made and is now the source of most of their pain.

**The two-file split is the root cause of the codebase's largest defensive-code
cluster [I].** Trace it: separate file → no shared transaction → desync possible
→ need `uteke repair`; separate file → cross-process contention → exclusive
`fs2` lock; separate file → C++ file I/O → Windows `MAX_PATH`/AV problems →
buffer-serialization workaround; separate file → no incremental persistence →
full rewrite per insert (§9). Five distinct problem classes, one root.

**The migration system's specific failure mode is instructive [I].** Fresh DBs
stamping HEAD means any side effect living only inside a migration function is
absent on new installs. #544 (FTS5 missing on fresh DBs) is exactly that bug,
and the fix was a repair function rather than fixing the stamping — so the tax
is now paid on every open forever.

**The `let _ = execute(...)` pattern (#549) is the second-order cause [I].**
Best-effort DDL turns a loud failure into a silent one, and the version stamp
then lies.

**The inconsistent write ordering between `remember` and `update` is probably a
latent bug [I]** — not provable without a runtime test, but two paths with
opposite lock-then-write orders is how you get deadlocks and partial states
under concurrency.

**The exclusive read lock is an unflagged ceiling [I].** WAL exists precisely to
give concurrent readers. Taking an exclusive OS-level lock on the vector file
throws that away — two `uteke recall` processes serialize completely. No comment
acknowledges this.

### Relevance to Go

Driver choice determines the distribution story (§8):

| Driver | cgo | Static binary | Cross-compile | FTS5 |
|---|---|---|---|---|
| `modernc.org/sqlite` | **no** | yes | trivial | yes (verify per version) |
| `mattn/go-sqlite3` | yes | no | painful | yes via build tag |

Use `modernc.org/sqlite`. It is transpiled pure-Go SQLite — slower than cgo on
raw throughput, but the bottleneck is a 40ms embedding call (§9), so it is
unmeasurable. **Verify FTS5 in the pinned version before committing:**

```go
db, _ := sql.Open("sqlite", ":memory:")
_, err := db.Exec("CREATE VIRTUAL TABLE t USING fts5(x)")
// err must be nil
```

Pragmas — set more than they did:

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;   -- safe under WAL, big write win; they left FULL
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA temp_store   = MEMORY;
PRAGMA cache_size   = -64000;   -- 64MB
```

`synchronous=NORMAL` under WAL loses at most the last transaction on OS crash
(not on process crash) and removes an fsync from every commit. For a local
memory store that is the right trade, and it is free throughput Uteke leaves on
the table.

**Vectors in SQLite** via `sqlite-vec`
(`github.com/asg017/sqlite-vec-go-bindings`):

```sql
CREATE VIRTUAL TABLE vec_items USING vec0(
  memory_id TEXT PRIMARY KEY,
  embedding FLOAT[768]
);
```

What this buys, concretely:

- insert plus vector update in one transaction — desync becomes structurally
  impossible
- no separate file, so no `fs2` lock, no serialized readers, no Windows I/O
  workarounds
- no full-index rewrite per insert
- room and namespace filters push into SQL instead of over-fetch-then-filter
  (§6 — this is the big one)
- one backup artifact, one `VACUUM`, one integrity check

The trade: `sqlite-vec` brute-forces rather than using HNSW. At 10k × 768 dims
that is ~30MB scanned — single-digit ms, against a 40ms embedding call.
Unmeasurable in the critical path.

**Migration rules — three, all learned from their scars:**

1. **Never stamp fresh DBs at HEAD.** Create at v0, run every migration in
   order. One code path, exercised on every install. This alone eliminates #544
   and #549.
2. **No best-effort DDL.** Every statement's error propagates and rolls back the
   transaction.
3. **Use `PRAGMA user_version`** instead of a `schema_version` table — an atomic
   4-byte header field that participates in the transaction and needs no query.

```go
var v int
db.QueryRow("PRAGMA user_version").Scan(&v)
for i := v; i < len(migrations); i++ {
    tx, _ := db.Begin()
    if err := migrations[i](tx); err != nil { tx.Rollback(); return err }
    tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1))
    tx.Commit()
}
```

Keep their newer-than-supported hard-error behavior (`schema.rs:121-127`).
Silently operating on a future schema corrupts data.

**Timestamps as `INTEGER` Unix micros, not RFC3339 text.** Their
`repair_datetime_timezones()` (`schema.rs:171-239`) exists purely because text
timestamps can be written malformed and only fail at read time. Integers sort
correctly by construction, index better, and have no parse-failure class.

**Column scanning — see §10.** Never write a SELECT list twice. One constant:

```go
const memoryCols = `id, content, tags, metadata, created_at, updated_at,
  namespace, access_count, last_accessed, deprecated, valid_from,
  valid_until, memory_type, importance, pinned, content_type, slug,
  source, source_type`
```

and scan by name (`sqlx.StructScan`, or `rows.Columns()` plus a name→index map).

### Alternative

**Brute-force in pure Go, no extension at all.** Keep the f32 BLOB in
`memories.embedding`, load all vectors into a `[]float32` slab at startup, and
scan with a SIMD-friendly dot product. At 10k memories that is 30MB resident and
~2-5ms per query. Zero dependencies, zero cgo, trivially correct. Add
`sqlite-vec` or HNSW only when a measurement demands it. For v1 this is probably
the right answer — less machinery than `sqlite-vec` and identical latency at
this scale.

At the other end: if 1M+ memories are certain, a real HNSW (cgo `hnswlib`, or
`github.com/coder/hnsw` in pure Go) in a separate file — and then Uteke's entire
`vector.rs` is the field guide to what that costs. Read it before choosing it.

**Quantization:** f32 storage costs 3KB/memory before overhead. int8
quantization (scale plus zero-point per vector) cuts that 4× with typically
under 1% recall loss. Not urgent at 10k, material at 1M.

### Open Question

**[U] Does `modernc.org/sqlite` ship FTS5 in the version you would pin?**
Highest-priority unknown, since "no" invalidates the whole storage plan. *How to
find out:* the five-line spike above.

**[U] Where does brute-force actually lose to HNSW on your hardware?** *How to
find out:* generate N random normalized 768-d vectors for
N ∈ {1k, 10k, 100k, 1M}, time a full cosine scan, plot against the 40ms
embedding floor. Estimated crossover is north of 100k, but measure.

**[U] Is `sqlite-vec` production-stable enough to depend on?** Pre-1.0 at time
of writing. *How to find out:* check its release cadence and open-issue profile;
more decisively, structure the code behind a `VectorIndex` interface so swapping
brute-force ↔ `sqlite-vec` ↔ HNSW is a one-file change. Uteke's `Embedder` trait
shows they understood this pattern for embeddings — they just never applied it
to the index.

---

## 4. Embedding & Search Pipeline

### Observation

**Model [F]** (`embed/engine.rs:14-21`):

```rust
const HF_REPO: &str = "onnx-community/embeddinggemma-300m-ONNX";
const MODEL_DIMS: usize = 768;
const MAX_SEQ_LEN: usize = 2048;
```

EmbeddingGemma 300M, Q4-quantized ONNX. Weights arrive as `model_q4.onnx` plus
`model_q4.onnx_data` (**187MB**) plus `tokenizer.json`.

**Integrity is pinned in source [F]** (`engine.rs:25-38`): a `MODEL_CHECKSUMS`
table of hardcoded SHA256 per file. Mismatch deletes the file and errors
(`engine.rs:393-420`).

**Download path [F]** (`engine.rs:236-374`): streaming 64KB chunks → `.tmp` →
atomic rename; three retries; 30s connect / 300s read timeouts; progress printed
every 10%; `clean_tmp_files()` on startup; `0700` dirs / `0600` files (#134).

**Pooling [F]** (`engine.rs:163-189`): they take ONNX **`outputs[1]`** — the
model's own pre-pooled sentence embedding — then L2-normalize in Rust. No manual
mean-pooling, no attention-mask arithmetic.

**Runtime init [F]:** ORT environment created once behind a `OnceLock`, via
`ort_init::init_ort_environment()`, which detects AVX2 vs SSE4.2 at runtime and
picks the matching shared library. Session guarded by a `Mutex`.

**Pluggable backends [F]** (`embed/mod.rs`): an `Embedder` trait with
`OnnxEmbedder` (default, feature-gated), `OpenAiEmbedder`, `OllamaEmbedder`,
`FallbackEmbedder` (chain), plus a `validate_base_url()` SSRF guard.

**Init-error caching [F]** (`lib.rs`, #822): `embedder_init_error`
distinguishes permanent failures (cached forever) from transient ones (60s TTL,
then retry). Without this, a missing model would re-attempt a 187MB download on
every call.

**Write path [F]** (`operations.rs`):

1. `retry_embed()` — three attempts, 200→400→800ms (`operations.rs:16-56`); on
   total failure store without embedding and log "run `uteke repair`" (#621)
2. `check_duplicate()` (`operations.rs:214-265`) —
   `const DEDUP_THRESHOLD: f32 = 0.95`; top-5 vector search, namespace-filtered,
   skips `chunk:`-prefixed IDs; a hit returns the existing ID and skips the
   insert entirely
3. acquire index write-lock → SQLite write → index add → `save()` (3× retry)
4. drop write-lock → `auto_link_cosine()` (deadlock avoidance, #442)

**Read path — vector channel [F]** (`operations.rs:~427-490`): a retry loop that
widens the candidate pool because post-filtering can starve results:

```rust
for multiplier in [3, 9, 27] {
    let k  = (limit * multiplier).min(index_len);
    let ef = (limit * multiplier * 4).max(50);
    // search, then post-filter namespace / tags / entity / category / deprecated
    if enough_results { break }
}
```

Hot-tier memories get `+0.1` (`operations.rs:481-484`). Cosine distance to
similarity: `(1.0 - d).clamp(0.0, 1.0)` (`vector.rs`).

**A dead parameter [F]:**
`pub fn search(&self, query: &[f32], k: usize, _ef: usize)` (`vector.rs`) — `ef`
is computed by the caller and then ignored, because the Rust usearch bindings do
not expose it. So the widening loop only actually widens `k`.

**Read path — lexical channel [F]** (`memory/fts5.rs`):

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    content, tags, namespace, memory_type,
    content='memories', content_rowid='rowid'
);
```

External-content table (no duplicated text), kept in sync by `memories_fts_ai` /
`_ad` / `_au` triggers, with the `('delete', ...)` insert form external-content
tables require. Two strategies: `search_fts5()` (whole query as a quoted phrase)
and `search_fts5_tokens()` (whitespace split, ≥2 chars, max 10 tokens, joined
`OR`) — phrase first, tokens as fallback when empty. Both filter
`m.deprecated = 0` and `ORDER BY f.rank`.

**Fusion [F]** (`operations.rs:748-874`, `recall_rrf`):

```rust
const RRF_K: u32 = 60;
// 1. vector search, limit*3          ← runs first
// 2. FTS5 search,  limit*3           ← then this, SEQUENTIALLY
// 3. for each channel, by ordinal rank:
let rrf = 1.0 / (RRF_K as f64 + rank as f64 + 1.0);
// 4. normalize
let max_rrf    = 2.0 / (RRF_K as f64 + 1.0);
let normalized = (score / max_rrf).clamp(0.0, 1.0);
```

**BM25 is discarded** — bound as `_rank_val` at `operations.rs:794`. Only
ordinal position feeds RRF.

**Post-fusion signals, all opt-in [F]:**

- Jaccard token overlap: `sr.score += j * self.jaccard_weight;` then re-sort
  (`operations.rs:847-866`) — `jaccard_weight` defaults to **0.0**
- Graph rerank: only under `RecallStrategy::Graph`
- Salience/recency: applied **after** the cache read (`operations.rs:593-613`)

**`min_score` is deliberately not applied to RRF output [F]**, with a comment
explaining RRF scores are not cosine-comparable.

**Recall cache [F]** (`recall_cache.rs`): key
`(query_hash, namespace, limit, strategy)`; `max_entries: 256`;
`ttl_secs: 300`; FIFO eviction; hit/miss counters. `min_score` is intentionally
excluded from the key — the caller re-filters cached results.

### Inference

**Why EmbeddingGemma 300M Q4 [I]:** the sweet spot for CPU-only local inference
circa their v0.9 — genuinely good quality, MTEB-competitive, Apache-licensed,
~190MB quantized. The 2048-token window is generous for memory-sized text.

**Using `outputs[1]` is the smartest small decision in the file [I].** Manual
mean-pooling over `last_hidden_state` requires applying the attention mask
correctly, and getting it subtly wrong (averaging over padding) is one of the
most common embedding bugs in the wild. Taking the model's own pooled output
sidesteps it entirely.

**Checksum pinning is supply-chain defense with a UX payoff [I].** A truncated
187MB download otherwise manifests as an inscrutable ONNX parse error; the
checksum turns it into "corrupt download, retrying."

**Sequential vector-then-FTS is a Rust ergonomics artifact, not a decision [I].**
Nothing in `recall_rrf` depends on ordering. Parallelizing in Rust means `rayon`
or threads plus `Send + Sync` on the store — real friction. In Go it is four
lines.

**`jaccard_weight = 0.0` and opt-in graph rerank are their honest evaluation
[I]:** these were built, measured, found not clearly better, and shipped
disabled rather than deleted. Read that as "skip these."

**Discarding BM25 in favor of ordinal rank is correct, not a bug [I].** That is
canonical RRF (Cormack et al. 2009). Its entire purpose is to fuse rankings
whose scores are incommensurable — a BM25 score of 12.4 and a cosine of 0.83
have no shared scale. Normalizing them would be worse.

**Excluding `min_score` from the cache key is a genuinely good design detail
[I]** — cache the expensive part (embedding plus search), apply the cheap filter
per-call. Same reasoning behind applying recency boosts post-cache: it keeps
cached entries time-invariant so a 300s TTL does not freeze recency scores.

**The dead `ef` parameter means their widening loop is half-broken [I].** They
intended to widen both the candidate pool (`k`) and HNSW search effort (`ef`);
only `k` actually widens. Under heavy post-filtering, recall quality is worse
than the code implies.

**Storing without an embedding on retry-exhaustion is a deliberate
availability-over-consistency choice [I]** — better to keep the user's data
(findable via FTS5) than to lose the write. But it creates a silent two-tier
store where some memories are lexical-only, and the only recovery is a manual
`uteke repair` the user has to notice they need.

### Relevance to Go

**Run the two channels in goroutines.** Free, and Uteke does not have it:

```go
type chanResult struct {
    hits []Hit
    err  error
}
vecCh, ftsCh := make(chan chanResult, 1), make(chan chanResult, 1)

go func() { h, err := s.vectorSearch(ctx, qvec, limit*3); vecCh <- chanResult{h, err} }()
go func() { h, err := s.ftsSearch(ctx, query, limit*3);   ftsCh <- chanResult{h, err} }()

v, f := <-vecCh, <-ftsCh
```

The embedding call must precede the vector search but not the FTS search — so
you can go further and kick off FTS while embedding, hiding the entire lexical
channel inside the 40ms ONNX call. That is a real latency win Uteke's structure
forecloses.

**RRF, verbatim — it is this small:**

```go
const rrfK = 60.0

func fuse(channels ...[]Hit) []Hit {
    scores := map[string]float64{}
    for _, ch := range channels {
        for rank, hit := range ch {
            scores[hit.ID] += 1.0 / (rrfK + float64(rank) + 1.0)
        }
    }
    maxRRF := float64(len(channels)) / (rrfK + 1.0)
    // …normalize by maxRRF, sort desc, truncate
}
```

Their `max_rrf = 2.0/(k+1)` hardcodes "2 channels" — parameterize it as
`len(channels)` so adding a third does not silently break normalization.

**The embedding backend is the hardest decision.** Go's ONNX story is materially
worse than Rust's:

| Option | cgo | Static | First-run | Notes |
|---|---|---|---|---|
| HTTP → Ollama / llama.cpp | no | yes | user installs Ollama | simplest, most swappable; adds an external dep |
| `onnxruntime_go` | yes | no | 190MB model download | Uteke's position exactly — sidecar `.so`, AVX2 splits |
| Cloud API (OpenAI/Voyage) | no | yes | API key | kills offline-first |
| Pure-Go inference | no | yes | — | not viable for 300M params today |

**Copy the `Embedder` interface from commit one** — the single most valuable
structural idea in their embedding code:

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    Dims() int
    ID() string   // model identity — see below
}
```

**Add `ID()`, which Uteke lacks.** If a user switches embedding models, every
stored vector becomes meaningless — different model, different space. Stamp the
model ID on each row and refuse (or auto-reindex) on mismatch. Uteke has no such
guard; switching from ONNX to OpenAI silently corrupts recall quality with no
error. This is a bug avoidable for free.

Also copy:

- SHA256-pinned model downloads, atomic tmp+rename, `clean_tmp_files()` on start
- the permanent-vs-transient init-error cache (#822) — trivial in Go with a
  `struct{ err error; until time.Time }`
- `EmbedBatch` — Uteke has no batching, which is part of their §9 insert-
  throughput problem
- the recall cache design exactly: key on `(query, ns, limit, strategy)`, exclude
  `min_score`, apply recency after cache read. Use
  `golang.org/x/sync/singleflight` alongside it so concurrent identical queries
  share one embedding call — Uteke has no such dedup.

Skip: Jaccard boosting and graph reranking (their own defaults say so), and the
`ef` widening loop (dead in their code, irrelevant if you brute-force).

### Alternative

**A much smaller embedding model.** all-MiniLM-L6-v2 is 22M params, 384 dims,
~90MB → ~23MB quantized. That is 8× smaller storage per vector (1.5KB vs 3KB)
and roughly 5-10× faster inference — plausibly turning the 40ms floor into under
10ms, which changes the performance story entirely (§9). The quality gap on
short factual recall is real but modest.

Uteke never validated that the bigger model was worth it — their LongMemEval row
is literally `*Pending run*` (`benchmarks/RESULTS.md:20-22`). Shipping the small
model, measuring, and upgrading only if recall suffers is a high-leverage
experiment, especially since model download size is the worst part of first-run
UX (§8).

**A cross-encoder reranker** on the top-20 fused results would beat every
post-fusion signal they built (Jaccard, graph, salience) — but it doubles
inference cost and needs a second model. Not for v1.

**Query expansion** — embed the raw query and an LLM-rewritten variant, fuse
three channels. Cheap if an agent is already in the loop.

### Open Question

**[U] What does `ef` actually cost them?** Since the parameter is dead, their
HNSW runs at usearch's default `ef`, which may be too low for heavily-filtered
queries. *How to find out:* irrelevant if you brute-force. If you adopt HNSW,
benchmark recall@10 against exhaustive ground truth across `ef` values.

**[U] How often does `retry_embed` exhaust and store an embedding-less memory?**
A silent correctness hole. *How to find out:* in your own implementation, make it
a counted, surfaced metric (`zbrain stats` showing `memories_without_embedding:
N`) rather than a log line, and make repair automatic on next start.

**[U] Is 0.95 the right dedup threshold?** Too low silently discards
legitimately distinct memories — a data-loss failure mode, since
`check_duplicate` skips the insert entirely. *How to find out:* log
near-threshold pairs (0.90-0.98) rather than acting on them, review a sample,
then set the threshold. Consider making dedup a flag on the new row rather than
a silent skip — strictly safer.

---

## 5. MCP Server Design

### Observation

**Tools only [F].** Grepping `"name":` in `crates/uteke-mcp/src/lib.rs` (1741
lines) yields ~32 flat tools:

| Group | Tools |
|---|---|
| Core | `uteke_remember`, `uteke_recall`, `uteke_list`, `uteke_forget`, `uteke_search`, `uteke_stats`, `uteke_context` |
| Documents | `uteke_doc_create`, `_update`, `_get`, `_list`, `_search`, `_delete`, `_move` |
| Rooms | `uteke_room_recall`, `_memories`, `_create`, `_list`, `_delete`, `_stats`, `_summary`, `_document` |
| Tags | `uteke_tags_list`, `_rename`, `_delete` |
| Graph | `uteke_graph`, `uteke_graph_add_edge`, `uteke_graph_remove_edge` |
| Misc | `uteke_pin`, `uteke_unpin`, `uteke_dream` |

**No `resources/list`, no `resources/read`, no `prompts/list`.** MCP's other two
primitives are entirely unused.

**Transport is dual, handler is shared [F].** `uteke-mcp/src/lib.rs:60-66`
exposes `handle_jsonrpc(&uteke, &line) -> Option<String>`, consumed by both:

*stdio* (`uteke-mcp/src/main.rs:47-68`) — line-delimited JSON-RPC:

```rust
for line in stdin.lock().lines() {
    let line = line?;
    if let Some(response) = uteke_mcp::handle_jsonrpc(&uteke, &line) {
        let _ = writeln!(stdout, "{response}");
        let _ = stdout.flush();
    }
}
```

*HTTP* — `POST /mcp` on `uteke-serve`, protocol version `2025-06-18`
(Streamable HTTP).

**Notifications handled correctly [F]** (`lib.rs:83-116`): `id: None` means
return `None`, so no response is written, per JSON-RPC 2.0 §4.1.

**Error handling is two codes [F]** (`lib.rs:107`): `-32700` (parse error) and
`-32603` (internal error) for everything else — validation failures, not-found,
lock contention, embedding errors all collapse into "internal error" with a
string message. No `-32602` (invalid params), no `-32601` (method not found)
distinction, and no use of MCP's `isError: true` tool-result channel.

**No retry logic at the MCP layer at all [F].** Retries live deeper
(`retry_embed`, index-save retry). A failed tool call is simply a failed tool
call.

**Auth [F]:**

- *stdio:* none. Correct — the process boundary is the security boundary.
- *HTTP* (`uteke-server/src/context.rs:32-75`, `main.rs:263-265`): optional
  `Authorization: Bearer`; `--auth-token` (full access) and `--read-only-token`
  (GET only, #409); tokens SHA256-hashed at startup so only the incoming token is
  hashed per request; `GET /health` exempt; CORS origins configurable; default
  bind `127.0.0.1`.

**Isolation between agents: none [F].** `namespace` is a request parameter any
caller may set to any value. Room `author` is a self-declared string, never
verified. There is no notion of agent identity anywhere in the codebase.

**A documented past bug [F]** — comment in `uteke-mcp/src/main.rs`: hardcoding
`~/.uteke` instead of resolving `uteke_home()` *"silently opened an empty second
store and never saw anything the CLI had written."*

**Content type shape [F]** (`lib.rs:47-52`): `McpContent` uses
`#[serde(tag = "role")]` while also carrying a `type` field — a confused
encoding of MCP's content union.

### Inference

**Tools-only is a defensible v1 but leaves value on the table [I].** MCP
resources are addressable, cacheable, client-listable content — exactly what
documents are. Seven `uteke_doc_*` tools reimplement, badly, what
`resources/list` plus `resources/read` provide natively. Likely they built tools
first (best-supported primitive early on) and never revisited.

**32 tools is over budget [I].** Every tool's name, description, and JSON schema
goes into the calling model's context on every request. At ~100-150 tokens each
that is ~4k tokens of overhead before the user says anything — and it degrades
tool-selection accuracy, because the model must discriminate among eight
near-synonymous `uteke_room_*` entries. A real cost, not a stylistic complaint.

**Collapsing all errors to `-32603` actively hurts agent behavior [I].** An agent
receiving "internal error" has one recovery strategy: retry or give up. An agent
receiving "validation failed: tag exceeds 64 chars" can fix the call. Since the
consumer is an LLM — the most adaptable error-handler ever built — throwing away
error structure discards the transport's main advantage.

**The absent auth is correct for stdio and a real gap for HTTP [I].** Once bound
beyond localhost, "any caller can claim any namespace and any author" means
namespaces are organizational, not a boundary. Fine if documented as such,
dangerous if users assume otherwise. No such documentation was found.

**The `~/.uteke` hardcoding bug is the classic multi-binary failure [I]** —
three binaries must agree on the home directory, and one did not. It failed
silently and confusingly (empty store, no error).

### Relevance to Go

**Use `github.com/modelcontextprotocol/go-sdk`.** Uteke hand-rolled JSON-RPC and
their `McpContent` shape shows the cost. The SDK gives correct content unions,
notification semantics, capability negotiation, and protocol-version handling
for free.

**Map errors properly:**

| Situation | JSON-RPC / MCP response |
|---|---|
| malformed JSON | `-32700` |
| unknown method | `-32601` |
| bad/missing args | `-32602` with the offending field named |
| domain failure (not found, validation) | `isError: true` in the tool result, human-readable text |
| genuine internal fault | `-32603` |

The `isError` distinction matters: protocol errors say "your call was
malformed"; `isError` results say "your call was fine, the operation failed" —
and the agent's recovery differs.

**Keep the tool surface tight.** zbrain's existing set — `remember`, `recall`,
`add_note`, `get_note`, `list_pending` — is right-sized. Resist growth. For
document operations, prefer:

```text
resources/list  →  zbrain://wiki/axioms/…, zbrain://evidence/sources/…
resources/read  →  content by URI
```

over `zbrain_doc_get` / `_list` / `_search`. Clients cache resources; they
cannot cache tool calls.

**Share one handler across transports** — their
`handle_jsonrpc(&uteke, &line) -> Option<String>` shape is right. In Go:

```go
type Handler struct{ store *Store }
func (h *Handler) Handle(ctx context.Context, req *Request) *Response
```

then wrap it in a stdio loop and an `http.HandlerFunc`. One implementation, two
transports, no drift.

**Resolve `ZBRAIN_HOME` in exactly one place** and have every entry point call
it. Their `~/.uteke` bug is a one-line mistake with a silent, baffling symptom.
Better still: on startup, log the resolved store path at INFO — one line that
turns a 30-minute mystery into an instant diagnosis.

**Contexts everywhere.** Every tool handler takes `ctx context.Context` and
threads it into `db.QueryContext` and the embedder. Uteke has no cancellation
story at all — a slow embedding call cannot be aborted.

If HTTP is added: copy their pre-hashed-token pattern (`main.rs:263-265`) and
their read-only second token (#409). Use `crypto/subtle.ConstantTimeCompare` for
the comparison.

### Alternative

**A single `zbrain_memory` tool with an `action` parameter** instead of 32 flat
tools:

```json
{"action": "recall", "query": "...", "namespace": "...", "limit": 10}
```

Cuts context overhead ~30×. Trade-off: no per-action JSON schemas, so the model
gets less validation help and argument errors rise. A middle path is best —
five to seven top-level tools grouped by noun (`memory`, `document`, `room`)
with an action field each. Keeps schemas meaningful, keeps the surface small.

**Resources for read, tools for write** is the cleanest split and matches MCP's
design intent: `resources/*` for anything addressable and cacheable, `tools/*`
for anything that mutates or requires computation (like `recall`, which is not
addressable because the query is the address).

### Open Question

**[U] Does tool count measurably degrade agent accuracy at 32?** Widely
believed, rarely measured. *How to find out:* register the tool set at 5 tools
and at 30 (padded with plausible decoys), run 20 realistic requests, count
correct tool selections.

**[U] Do MCP clients (Claude Code, Cursor) support resources well today?**
Determines whether the resources recommendation is real or theoretical. *How to
find out:* register one trivial resource and check whether the client lists and
reads it.

---

## 6. Multi-Agent Memory (Rooms)

### Observation

**Data model [F]** (`store.rs:135-153`):

```text
rooms(id, name, description, created_at, …)
room_memories(room_id, memory_id, author, role, joined_at)
room_documents(room_id, document_id, …)              -- schema v15
```

A pure junction over existing memories — rooms own nothing.

**Writing to a room is a dual-write [F]** (`rooms.rs:37-59`,
`remember_in_room`): the memory is stored in the writing agent's own namespace,
and a `room_memories` link row is inserted. There is no room-owned storage.

**Shared vs private is emergent, not enforced [F]:**

- memory with no `room_memories` row is private by convention
- memory with a link row is shared
- but `namespace = None` in a recall searches all namespaces
  (`operations.rs:427`, #448), so "private" is not a boundary — it is a default
  filter

**Semantic room recall is over-fetch-then-post-filter [F]** (`rooms.rs:77-125`,
`recall_room_semantic`):

```rust
let fetch_limit = (limit * 5).min(200).max(limit);
// 1. recall_hybrid() across ALL namespaces at fetch_limit
// 2. drop every result whose id ∉ room membership set
// 3. re-sort, truncate to limit
```

The `.min(200)` cap carries issue ref #546.

**Consistency [F]:** one SQLite file, WAL, `busy_timeout=5000`. Writes serialize
at the SQLite level. Strong consistency, trivially — because there is nothing to
be inconsistent with.

**Conflict handling [F]:** none. No vector clocks, no version columns, no
optimistic-concurrency check, no merge. `update_memory`
(`operations.rs:1132-1204`) does a read-then-write with no version guard — two
concurrent updates to the same memory are last-write-wins with no detection.

**Discovery [F]:** `list_rooms()` / `uteke_room_list`. A poll.

**Subscription [F]:** none. No push, no watch, no long-poll, no SSE, no
`sqlite3_update_hook`. An agent learns of another agent's write only by querying
again.

**Scale [F]:** `memory/rooms.rs` is 1917 lines; 8 of the ~32 MCP tools are room
tools.

### Inference

**The dual-write is a deliberate ownership choice and a good one [I]:** memories
stay in their author's namespace, so deleting a room does not destroy content,
and an agent's memories remain coherent independent of room membership. Rooms
are a view, not a container. Copy this.

**The over-fetch-then-filter is a workaround for the two-file storage split
[I]**, and the clearest illustration in the codebase of why that split hurts.
Room membership lives in SQLite; the vector index lives elsewhere and knows
nothing about rooms. So "nearest neighbors within this room" is inexpressible —
you can only take global neighbors and hope enough belong.

**This has a real, silent failure mode [I]:** for a room whose memories are a
small or semantically-atypical slice of the store, the global top-200 may
contain very few members. Recall then returns 3 results when 10 were asked for,
with no error and no indication that the cap bound. The `.min(200)` (#546) is a
bound on the damage, not a fix. Expect degradation as store size grows relative
to room size — the direction real usage goes.

**"No conflict handling" is defensible here and dangerous later [I].** Single
machine, single file, serialized writes — CRDTs genuinely are not needed. But
the absence of a version column means optimistic concurrency cannot be added
later without a migration, and lost updates cannot be detected today. A
`version INTEGER` column costs 8 bytes and buys
`UPDATE … WHERE id=? AND version=?`.

**Polling-only discovery is the biggest missed opportunity in the feature [I].**
The entire premise of rooms is agents coordinating. Coordination without
notification means every agent polls on a timer or misses updates — so the
feature works well for "agent B reads what agent A wrote yesterday" and poorly
for anything live.

**1917 lines and 8 MCP tools for a feature with no notification mechanism
suggests it was built ahead of demand [I]** — which sits oddly against their own
"demand-gated" roadmap statement.

### Relevance to Go

**Copy the junction-table model verbatim.** Memories owned by namespaces, rooms
as a many-to-many view, author recorded on the link.

**Push the room filter into SQL — the payoff for §3's single-store decision.**
With vectors in SQLite:

```sql
SELECT m.id, m.content, vec_distance_cosine(v.embedding, ?) AS dist
FROM   vec_items v
JOIN   memories m       ON m.id = v.memory_id
JOIN   room_memories rm ON rm.memory_id = m.id
WHERE  rm.room_id = ? AND m.deprecated = 0
ORDER  BY dist
LIMIT  ?;
```

Exact top-K within the room, always. No over-fetch, no `.min(200)`, no silent
under-return, no #546. The correctness improvement is total, and it falls out of
the storage decision rather than requiring new code.

**Add a `version INTEGER` column now.** Eight bytes, and it converts
last-write-wins into detectable conflict:

```go
res, _ := tx.ExecContext(ctx,
    `UPDATE memories SET content=?, version=version+1, updated_at=?
     WHERE id=? AND version=?`, content, now, id, expectedVersion)
if n, _ := res.RowsAffected(); n == 0 {
    return ErrConflict   // someone else wrote; caller re-reads and retries
}
```

Free today, impossible to retrofit cheaply.

**Go makes real subscriptions easy — where you can beat them outright.** With a
daemon (§1):

```go
type Broker struct {
    mu   sync.RWMutex
    subs map[roomID][]chan Event
}
// fan out on commit; MCP clients receive notifications/resources/updated
```

Or, without a daemon, SQLite's update hook (exposed by `modernc.org/sqlite`)
plus a small poll-on-change loop. MCP has a notification channel —
`notifications/resources/list_changed` — and using it turns rooms from a polling
API into live shared memory.

**But: defer rooms entirely for v1.** 1917 lines and 8 tools for a feature whose
necessity is unproven. zbrain's `workspace` concept already gives the isolation
axis. Rooms are the sharing axis — build them when two agents actually need to
share, and let that case shape the design.

### Alternative

**Rooms as a namespace list rather than a junction table.** A room is
`{name, namespaces: [...]}`; room recall is recall with `namespace IN (...)`.
Far simpler — no link rows, no dual-write, no membership sync — and it fits SQL
naturally. Trade-off: no per-memory sharing granularity (you share a whole
namespace or nothing). For most real multi-agent setups that is the right
granularity, at a tenth of the code.

**Or: skip rooms; use tags.** A shared memory is one tagged `shared:project-x`.
Recall filters on the tag. Zero new tables, zero new tools. The loss is
authorship metadata and room-level stats — both addable later.

**Multi-machine rooms are not a rooms feature — they are a sync feature**, and
Uteke offers no prior art at all (§10). That would mean CRDTs or a central
server, and a different product.

### Open Question

**[U] How often does the `.min(200)` cap actually bind, and how badly?** The
difference between "minor inefficiency" and "silently broken feature." *How to
find out:* instrument `recall_room_semantic` to log
`(fetch_limit, results_after_filter, limit)` and run it against a store with 10k
memories and a 50-memory room.

**[U] Do multiple agents genuinely write concurrently in practice?** Determines
whether conflict detection is theoretical. *How to find out:* instrument an
alpha — count same-memory writes within a 1s window.

**[U] Is anyone using rooms at all?** *How to find out:* search their issue
tracker for `uteke_room_*` bug reports. Near-zero traffic on a 1917-line feature
is a strong signal to defer.

---

## 7. Time-Travel Queries

### Observation

This is the section where the docs and the code diverge most.

**The mechanism is a filter over current rows [F]**
(`operations.rs:1258-1312`). Given `--at <RFC3339>`:

```text
created_at  <= point_in_time
AND (valid_until IS NULL OR valid_until >  point_in_time)
AND (valid_from  IS NULL OR valid_from  <= point_in_time)
```

Applied in Rust, in memory, after recall returns
(`operations.rs:1298-1312`) — not pushed into SQL. Surfaced as `--at` on `list`
and `recall`.

**It is not event sourcing, not snapshots, not bitemporal [F].** There is no
history table, no shadow table, no snapshot mechanism, no `system_time`
dimension. The three columns (`created_at`, `valid_from`, `valid_until`) plus
`deprecated` are the entire apparatus.

**`timeline_events` is a much thinner thing than its docs claim [F].**

Schema v9 (`schema.rs:678-683`), with the comment *"No data backfill — timeline
tracking starts from this version forward."* Table:
`(id, memory_id, event_type, event_data, created_at)`. Six declared event types
(`timeline.rs:25-38`): `Created, Updated, Recalled, Consolidated, Tagged,
Forgot`.

The module doc (`timeline.rs:10-11`) says: *"Timeline events are emitted
automatically by the memory lifecycle hooks (`remember_precomputed`, recall
access tracking, consolidate, forget)."*

**That is not true.** Grepping every emission site across all crates:

```text
crates/uteke-core/src/operations.rs:318   → TimelineEventType::Created
crates/uteke-core/src/consolidate.rs:116  → TimelineEventType::Consolidated
```

Two call sites. `Updated`, `Recalled`, `Tagged`, and `Forgot` are declared,
round-trip-tested (`timeline.rs:224-237`), documented as emitted, and never
written by any code path. `update_memory` (`operations.rs:1132-1204`) emits
nothing.

**Content history does not exist [F].** `update_memory` calls
`store.update_fields(...)` (`operations.rs:1191-1200`), an in-place `UPDATE`.
The prior content is overwritten and unrecoverable. `event_data` on timeline
rows is an optional JSON payload, and the only two emitters pass `None` and
consolidation metadata respectively — no before/after content.

**Storage overhead [F]:** effectively zero for `--at` (three existing columns,
no history rows). `timeline_events` gains roughly one row per memory created
plus one per consolidation — negligible, and far less than its design implies.

**Indexing [F]:** `idx_timeline_created` exists (`store.rs:87`). But timestamps
are RFC3339 TEXT, so all range comparisons are lexicographic string comparisons
— correct only because RFC3339-UTC happens to sort correctly.
`repair_datetime_timezones()` (`schema.rs:171-239`) exists precisely because
some writer produced strings without the `+00:00` suffix, breaking parsing in
`load_all()`.

**What the docs promise [F]** (`docs/time-travel.md`): *"audit trail — trace how
knowledge evolved"* and *"compare memory state between two points in time."*

**What it delivers [F]:** which memories existed and were valid at time T. Never
what they contained.

**Machinery that exists but is unused for history [F]:** a `deprecate()` path
(`store.rs`, `test_deprecate`) and a `supersedes` edge type declared in the v8
migration (`schema.rs:627`). Both are exactly what a supersession-based history
would need. Neither is wired into `update_memory`.

### Inference

**This is the cheapest implementation that satisfies the headline feature [I].**
`valid_from` / `valid_until` / `deprecated` already existed for TTL and
deprecation semantics. Adding `--at` reused them for free — zero schema change,
zero storage cost, one afternoon of work.

**The gap between `docs/time-travel.md` and the implementation is the most
significant honesty problem in the repo [I].** "Trace how knowledge evolved" is
precisely what the design cannot do. A user who edits a memory and later runs
`--at` gets the current content with a historical existence filter — silently,
with no warning that they are looking at a rewritten past.

**The unused event types tell a clear story [I]:** the timeline was designed as
a full audit log, the enum and storage were built, and the emission hooks for
four of six types were never written. Then the doc comment described the intent
rather than the state. This is what a feature abandoned at 40% looks like.

**They had the pieces for real history and did not connect them [I].**
`deprecate()` plus `supersedes` plus `valid_until` is a complete supersession
design. Wiring `update_memory` to insert-new, deprecate-old, and link would have
delivered the documented feature.

**Storing timestamps as text is the root of a whole class of trouble [I].** More
bytes, slower comparison, correct sorting only by RFC3339 luck, and a
malformed-write failure mode that only manifests at read time — exactly what
`repair_datetime_timezones` cleans up after.

### Relevance to Go

Decide which product you are building, explicitly, before writing code:

| Approach | Storage cost | Query complexity | Gives you |
|---|---|---|---|
| Validity filter (Uteke) | zero | trivial | which memories existed at T |
| **Soft-update / supersession** | ~1 extra row per edit | low | **full content history** |
| Full event sourcing | high, unbounded | high (replay/projection) | complete reconstruction, audit |

**Recommendation: soft-update.** A small delta from their design with a large
capability gain.

Never `UPDATE` content in place. On edit:

```go
func (s *Store) Update(ctx context.Context, id, newContent string) (string, error) {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return "", err }
    defer tx.Rollback()

    now := time.Now().UnixMicro()
    newID := uuid.NewString()

    // 1. close the old row's validity interval
    if _, err := tx.ExecContext(ctx,
        `UPDATE memories SET valid_until = ?, deprecated = 1 WHERE id = ?`,
        now, id); err != nil { return "", err }

    // 2. insert the successor
    if _, err := tx.ExecContext(ctx,
        `INSERT INTO memories (id, content, valid_from, created_at, …)
         VALUES (?, ?, ?, ?, …)`,
        newID, newContent, now, now); err != nil { return "", err }

    // 3. link them
    if _, err := tx.ExecContext(ctx,
        `INSERT INTO edges (src_type, src_id, dst_type, dst_id, rel, created_at)
         VALUES ('memory', ?, 'memory', ?, 'supersedes', ?)`,
        newID, id, now); err != nil { return "", err }

    return newID, tx.Commit()
}
```

Now the same `--at` filter returns genuinely correct historical content,
`supersedes` gives a version chain, and "compare state between two points"
becomes a real query rather than a doc claim.

Cost: edited rows are retained. A memory edited 10 times keeps 11 rows. Bound it
if needed — prune superseded rows older than N days, keeping the chain head.

**Push the temporal filter into SQL, not application code.** Uteke filters in
Rust after fetching (`operations.rs:1298-1312`), so the vector search returns
rows that are then thrown away — wasting recall budget exactly like the room
filter (§6):

```sql
WHERE created_at <= ?
  AND (valid_until IS NULL OR valid_until >  ?)
  AND (valid_from  IS NULL OR valid_from  <= ?)
```

**Timestamps as `INTEGER` Unix micros.** Correct sorting by construction,
tighter indexes, faster comparison, no parse-failure class, no
`repair_datetime_timezones` equivalent. Format at the presentation edge only.

**If you build a timeline table, emit every declared event type or do not
declare it.** Their enum-without-emitters is a trap for anyone reading the code.
Write a test asserting each declared type has at least one emission site.

Index for the queries you will actually run:

```sql
CREATE INDEX idx_mem_temporal ON memories(valid_from, valid_until) WHERE deprecated = 0;
CREATE INDEX idx_mem_created  ON memories(created_at);
CREATE INDEX idx_edges_super  ON edges(dst_id, rel) WHERE rel = 'supersedes';
```

Partial indexes matter — most queries filter `deprecated = 0`, and under
soft-update the deprecated set grows.

### Alternative

**Full event sourcing** — an append-only `events` table as the source of truth,
with `memories` as a materialized projection. Perfect audit, replay, and "what
did the store look like at T" for everything. Cost: every read path goes through
a projection, every schema change is a re-projection, storage grows unboundedly.
Over-engineered for a local memory store, but right if compliance/audit becomes
a requirement.

**Snapshots** — periodic full copies of the DB (`VACUUM INTO` is one line).
Simple, gives true point-in-time state, costs O(db_size) per snapshot. A
reasonable complement to soft-update: soft-update for fine-grained content
history, daily snapshots for disaster recovery.

**Content-addressed storage** — store content blobs keyed by hash, with
`memories` pointing at a hash. Edits create a new blob; identical content dedups
automatically. Elegant and gives free dedup, but adds indirection to every read.

### Open Question

**[U] Do users actually want content history, or just "what did I know then"?**
Very different implementations, and the second is nearly free. *How to find out:*
ask directly. Uteke's docs claim the first and implement the second, and
apparently nobody complained loudly enough to close the gap — weak evidence that
existence-filtering suffices, but their users self-selected for a tool that never
offered content history.

**[U] What is the realistic edit rate on agent-written memories?** Drives the
storage cost of soft-update. If agents overwhelmingly append rather than edit,
soft-update costs nearly nothing. *How to find out:* instrument an alpha — count
`Update` vs `Create` calls.

**[U] Is `docs/time-travel.md` knowingly aspirational or unnoticed drift?**
Matters for how much to trust their other docs. *How to find out:* check
`git log docs/time-travel.md` against the timeline feature commits.

---

## 8. Binary Distribution & Operations

### Observation

**"Single binary" does not survive contact with the release workflow [F].**

`.github/workflows/release.yml:196-221` — each release tarball contains:

- `uteke` (CLI)
- `uteke-serve` (HTTP server)
- `uteke-mcp` (MCP stdio server)
- the ONNX Runtime shared library as a sidecar

Three binaries plus a dynamic library, not one binary.

**ONNX Runtime is explicitly dynamic, with the reasoning in-source [F]**
(`uteke-core/Cargo.toml:29-33`):

```toml
# NOTE: load-dynamic requires api-18+ to avoid pykeio/ort#547 (vitis EP compile error).
# download-binaries is intentionally removed — ORT lib is shipped as a sidecar instead.
ort = { version = "2.0.0-rc.12", default-features = false,
        features = ["load-dynamic", "ndarray", "api-18"], optional = true }
```

**Build matrix — 5 artifacts, 4 platforms [F]** (`release.yml:284-303, 369-371`):

| Target | Note |
|---|---|
| `x86_64-unknown-linux-gnu` | AVX2 |
| `x86_64-unknown-linux-gnu` | legacy SSE4.2 build |
| `aarch64-unknown-linux-gnu` | |
| `aarch64-apple-darwin` | |
| `x86_64-pc-windows-msvc` | |

No musl. No static linking. All Linux builds are glibc-linked. CPU capability is
detected at runtime to select the right ORT library (`embed/ort_init.rs`).

Plus a runtime download [F]: ~190MB of model weights fetched from HuggingFace on
first embedding call (`embed/engine.rs:236-374`).

**Release profile is properly tuned [F]** (root `Cargo.toml`):

```toml
[workspace.package]
version = "0.11.0"
edition = "2024"
rust-version = "1.85"

[workspace.lints.rust]
unsafe_code = "forbid"

[profile.release]
opt-level = 3
lto = true
codegen-units = 1
strip = true
panic = "abort"
```

No UPX.

**Install [F]** (`install.sh`): OS/arch detection → download tarball → verify
SHA256 against a published `checksums-sha256.txt` → extract. It rejects tar
entries matching `^/|(^|/)\.\.(/|$)` before extraction (`install.sh:132`) — a
real path-traversal guard.

**First run [F]:** prints `Downloading embedding model (first run)...` with 10%
progress increments.

**Home directory migration [F]** (`lib.rs`, `uteke_home()`): resolves
`UTEKE_HOME` env, default `~/.codecora/uteke`. Auto-migrates a legacy `~/.uteke`
via `rename`, with an EXDEV cross-device copy fallback and a symlink guard. They
have already relocated the home directory once and handled it carefully.

**Observability [F]** (`uteke-cli/src/logging.rs:1-36`): two `tracing` layers —

- console at WARN (DEBUG with `--verbose`)
- always-on daily-rotating DEBUG file log at `{uteke_home}/uteke.log`,
  non-blocking writer with a guard, falling back to the temp dir if the home dir
  is not writable

No metrics endpoint, no tracing export, no structured event stream. Grepping
`metrics|prometheus|EnvFilter` finds nothing beyond the layer setup. The only
counters anywhere are recall-cache hits/misses (`recall_cache.rs:155-157`),
surfaced through `uteke stats`.

**Updates [F]:** re-run `install.sh`. No self-update command, no update check,
no version notification. Schema handles version skew defensively (§3): older DB
migrates forward, newer DB hard-errors with instructions.

### Inference

**They wanted a single static binary and ONNX Runtime made it impossible [I].**
The chain is forced: ONNX Runtime is a large C++ library, so static linking is
impractical, so `load-dynamic` plus sidecar, so no musl (glibc symbol
resolution), so platform-specific builds. The comment "download-binaries is
intentionally removed" shows they even rejected letting `ort` fetch its own
library, preferring to ship it — the right supply-chain call.

**The AVX2/SSE4.2 split is a pure ONNX Runtime tax [I].** Prebuilt ORT assumes
AVX2, which faults with `SIGILL` on older CPUs and some hypervisor configs.
Shipping two Linux x86_64 builds with runtime detection is the workaround —
exactly the complexity inherited the moment you take an in-process C++ inference
runtime.

**`panic = "abort"` is considered [I]** — smaller binary, no unwind tables, and
a CLI panic should terminate anyway. `unsafe_code = "forbid"` workspace-wide is
a strong signal about correctness posture, notable given C++ dependencies
through FFI (the `forbid` applies to their code, not the bindings).

**Their first-run UX is the weakest part of the product [I]:** install a
"local-first, zero-config" tool, run your first command, wait for a 190MB
download. That is the moment users bounce. It is entirely a consequence of the
model-size choice (§4) — a 23MB MiniLM would make first run nearly instant.

**No self-update is a real gap for a fast-moving local tool [I]** — 15 schema
versions in ~11 minor releases means users are frequently behind, and the only
signal is when something breaks.

**The observability story is right-shaped for the product [I].** Always-on
DEBUG-to-file is genuinely smart for a local CLI: when a user reports "recall
returned nothing," you ask for the log and have the trace. Metrics and
distributed tracing would be cargo-culting server practice into a laptop tool.
The one thing missing is a rotation size cap.

### Relevance to Go

Go gives genuinely static binaries and trivial cross-compilation:

```bash
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o dist/zbrain-linux-amd64  ./cmd/zbrain
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o dist/zbrain-darwin-arm64 ./cmd/zbrain
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/zbrain-windows-amd64.exe ./cmd/zbrain
```

One `Makefile` target, no cross-toolchain, no matrix runners, no AVX2/SSE4.2
split, no musl question, no sidecar.

Two decisions preserve that, and each can destroy it:

| Decision | Preserves static | Destroys static |
|---|---|---|
| SQLite driver | `modernc.org/sqlite` | `mattn/go-sqlite3` (cgo) |
| Embedding | HTTP → Ollama / cloud API | `onnxruntime_go` (cgo plus sidecar) |

Taking both cgo-free options ships what Uteke wanted and could not. That is a
legitimate competitive advantage, worth explicitly protecting:

```go
//go:build cgo
package main
func init() { panic("zbrain must be built with CGO_ENABLED=0") }
```

A CI check that runs `CGO_ENABLED=0 go build` and fails otherwise costs nothing
and prevents a dependency from quietly reintroducing cgo.

**The `assets/` plus `go:embed` setup is already better than their model.**
Uteke downloads runtime content; zbrain ships it inside the binary. Preserve
that property. If an embedding model must be downloaded, make it the only
download, show progress, checksum it, and consider a `zbrain setup --model` step
so it is an explicit act rather than a surprise on first `ask`.

Copy from `install.sh` directly:

- SHA256 verification against a published `checksums.txt`
- the tar path-traversal rejection (`install.sh:132`) — Go's `archive/tar` has
  the same hazard; use `filepath.IsLocal()` (Go 1.20+) on every entry
- OS/arch detection with clear error messages for unsupported combos

Add what they lack — a self-update path. `goreleaser` handles multi-platform
builds, checksums, GitHub releases, and Homebrew taps, and pairs with a
version-check on startup:

```go
// non-blocking, cached, respects an opt-out env var
go checkLatestVersion(ctx)
```

Observability in Go:

```go
// console: WARN, human-readable
// file: DEBUG, JSON, rotating (lumberjack), size-capped
h := slog.NewJSONHandler(&lumberjack.Logger{
    Filename:   filepath.Join(zbrainHome, "zbrain.log"),
    MaxSize:    10,  // MB — the cap Uteke lacks
    MaxBackups: 5,
    MaxAge:     30,
}, &slog.HandlerOptions{Level: slog.LevelDebug})
```

`log/slog` is stdlib, structured, zero dependencies. Copy their always-on DEBUG
file log — the single highest-value ops decision in the repo for a local tool.
Add the size cap.

Log the resolved `ZBRAIN_HOME` at startup (INFO, one line). Their `~/.uteke` bug
(§5) cost someone real time; one log line makes it self-diagnosing.

### Alternative

**Ship the embedding model inside the binary.** A quantized MiniLM at ~23MB
embedded via `go:embed` gives a ~35MB binary with zero runtime downloads and
true first-run readiness — a dramatically better UX than Uteke's, and only
available when choosing the smaller model (§4).

**Or: no bundled model at all.** Require Ollama, fail with a clear message and a
one-line fix (`ollama pull nomic-embed-text`). Smallest binary, zero model
management, but a dependency moved onto the user.

**Or: a `zbrain-embed` companion binary** with the cgo/ONNX build, spoken to
over a Unix socket — keeping the main binary static while allowing
in-process-grade inference. Uteke's sidecar pattern done at the process boundary
instead of the linker. More moving parts, but it isolates the cgo blast radius.

### Open Question

**[U] What is the acceptable binary size ceiling?** Determines the
embed-the-model question. Note that `gh`, `terraform`, and `kubectl` are all
50-100MB, so a 35MB `zbrain` with a bundled model is well within norms.

**[U] Does `modernc.org/sqlite` perform acceptably for the write path?**
Transpiled C, slower than cgo SQLite on raw throughput. *How to find out:*
benchmark 10k inserts with the real schema and indexes on both drivers.

**[U] Will Windows users matter?** Uteke spent real effort there (#647, #684,
#732) and it shaped their vector index design. Pure-Go plus SQLite-in-one-file
sidesteps essentially all of it — another argument for the single-store design.

---

## 9. Performance Characteristics

### Observation

**They publish two benchmark files that contradict each other [F].**

`benchmarks/RESULTS.md:7-13`: ~800 inserts/sec, 0.3-5ms recall.
`docs/benchmarks.md:12-16`: 6-22 inserts/sec, 40-45ms recall.

A ~40× discrepancy on insert and ~10× on recall.

**The credible one is `docs/benchmarks.md` [F/I]:** it names hardware (Oracle
Cloud Ampere A1, 4 vCPU, 24GB, CPU-only), reports per-scale totals, and is
internally consistent. `RESULTS.md` self-labels its numbers "indicative" and its
competitive table still reads `*Pending run*` / `TBD` for their own LongMemEval
score while listing competitors (Hindsight 94.6%, Mem0 v3 Pro 91.6%, Mem0 Free
49.0%).

**Measured figures [F]** (`docs/benchmarks.md`):

| Memories | Insert | Recall avg | Recall p95 | DB size | Index size |
|---:|---:|---:|---:|---:|---:|
| 100 | 18.5/s | 40ms | 46ms | 708KB | 319KB |
| 1,000 | 21.8/s | 45ms | 51ms | 5.3MB | 3.2MB |
| 10,000 | 6.0/s | 42ms | 50ms | 81.3MB | 30.3MB |

**The load-bearing observation [F]: recall latency is flat at ~40ms across a
100× scale increase.** Their own analysis attributes this correctly — HNSW
search contributes under 1ms even at 10k; the ~40ms floor is ONNX embedding
inference of the query.

**Storage [F]:** ~10KB per memory total (81.3MB plus 30.3MB at 10k), linear. Of
which ~3KB is the f32 embedding BLOB and ~3KB the usearch copy — roughly 60% of
storage is duplicated vector data.

**Insert throughput degrades 3.6× from 1k to 10k [F]:** 21.8/s → 6.0/s. Their
stated attribution: HNSW graph traversal cost growth.

**No latency targets, SLOs, or regression gates anywhere [F].** No benchmark CI
job in `.github/workflows/`. `criterion` is not a dependency.

**Instrumentation [F]:** recall-cache hit/miss counters
(`recall_cache.rs:155-157`), surfaced via `uteke stats`. The only runtime
measurement in the product.

### Inference

**Their insert-cost attribution is probably wrong, or at least incomplete [I].**
Count what a single `remember()` does (`operations.rs`):

1. ONNX embed the content — ~40ms on their hardware
2. `check_duplicate()` — a full vector search (top-5, `operations.rs:214-265`)
3. SQLite insert plus FTS5 trigger plus tag junction writes
4. usearch `add()`
5. `save()` — serialize and rewrite the **entire** index to disk
   (`vector.rs:188-215`)
6. `auto_link_cosine()` — another vector search, plus edge inserts

At 10,000 memories, step 5 writes 30.3MB per single insert. On a 4-vCPU cloud
VM, a 30MB serialize plus write plus fsync plus rename is plausibly 100ms+ on
its own — which would fully account for the drop to 6/s, and it scales linearly
with index size exactly as observed. HNSW insertion is O(log n)-ish and cannot
explain a 3.6× drop over one decade of scale.

**So the insert bottleneck is almost certainly the full-index rewrite, not HNSW
[I].** A strong inference — the scaling shape matches — but not profiled.
Flagged as inference, and the most consequential one in this document, because
it means the fix is trivial (batch the persist) rather than architectural.

Note also that steps 1, 2, and 6 mean each insert performs one embedding and two
vector searches [I]. Uteke has no `EmbedBatch`, so a bulk import pays all of
this per item.

**The flat 40ms recall is the most important number here [I].** Search is not
the problem; inference is. Every optimization instinct pointed at HNSW, index
tuning, or query planning is misdirected effort. The only levers that move
recall latency are (a) a smaller/faster embedding model, (b) caching, (c) hiding
the embedding call behind concurrent work.

**Publishing two contradictory benchmark files is a credibility problem [I]**,
and the `*Pending run*` row next to competitors' published scores is worse — it
implies a comparison that was never made.

**No benchmark CI means performance regressions ship silently [I].** With 15
schema versions and heavy churn, that is a real risk.

### Relevance to Go

**1. Optimize the embedding path; ignore search.** At this scale everything else
is noise. Concretely:

- **Batch on bulk import.** `EmbedBatch` over 32-64 texts amortizes model
  overhead massively — often 5-10× throughput. Uteke has no batching at all.
- **Cache aggressively.** Their 300s recall cache exists for exactly this reason.
- **Dedup in-flight identical queries** with `golang.org/x/sync/singleflight` —
  Uteke has no such guard, so N concurrent identical recalls pay N embeddings.
- **Overlap work.** Kick off the FTS5 query while the embedding runs (§4).

**2. Never rewrite a whole index per write.** Keeping vectors in SQLite makes
this structurally impossible. If a separate index is kept, batch persistence:

```go
type Index struct {
    mu    sync.Mutex
    dirty bool
    // …
}
// flush on: N writes, T elapsed, or graceful shutdown — never per insert
```

Uteke has an `is_dirty()` (`vector.rs:347-350`) and does not use it to defer
saves. The fix was one flag away.

**3. Ship exactly one benchmark artifact, generated by CI.** `zbrain bench`
writes `docs/benchmarks.md`; a CI job regenerates it on tag. Add a regression
gate with `testing.B` plus `benchstat` comparing against the previous commit.

Storage math:

| Component | Uteke @10k | zbrain with `sqlite-vec` |
|---|---:|---:|
| SQLite (content, metadata, FTS) | 81.3MB | ~50MB |
| Embedding BLOB (768 f32) | *(included above)* | 30MB |
| Separate vector index | 30.3MB | 0 |
| **Total** | **111.6MB** | **~80MB** |

The duplicated vector copy is saved outright — ~28% at 10k. With int8
quantization, another 22MB.

Bottlenecks Uteke has not addressed, avoidable by construction:

| Their bottleneck | Your fix |
|---|---|
| Exclusive index file lock serializes readers | vectors in SQLite → WAL concurrent reads |
| Full index rewrite per insert | vectors in SQLite → row-level writes |
| `recompute_importance()` loads all memories into RAM (`store.rs:274-328`) | do it in SQL, batched or incremental |
| `load_all()` — same | paginate, or never load all |
| Two vector searches per insert (dedup, auto-link) | make both optional; skip auto-link (§2) |
| No embedding batching | `EmbedBatch` from day one |

### Alternative

**Change the constant, not the algorithm.** If the 40ms floor is unacceptable,
the only meaningful lever is a different model — MiniLM-L6 (22M params vs 300M)
plausibly lands in the 5-10ms range on the same hardware. A 4-8× improvement no
amount of index tuning can approach.

**Or accept the latency and hide it.** For an agent workflow, 40ms is fine — well
below human perception and trivial next to the LLM call it feeds. Do not
optimize this unless a user complains. The flat-across-100× curve is arguably a
feature: predictable latency regardless of store size.

**Precompute for known query patterns.** If `zbrain ask` has recurring shapes,
cache their embeddings at write time.

### Open Question

**[U] Is the full-index rewrite really the insert bottleneck?** The strongest
inference here and the one most worth confirming. *How to find out:* in their
repo, time `save()` in isolation at 1k vs 10k memories. It is the difference
between "HNSW is fundamentally slow to insert" (architectural) and "they forgot
to batch a disk write" (trivial) — and it changes whether separate vector
indexes are worth fearing at all.

**[U] What are zbrain's latency and throughput targets?** Nothing here is
evaluable without them. *How to find out:* define them now and write them into
the spec. A reasonable starting set: recall p95 under 100ms, insert over 50/s
single and over 500/s batched, store under 200MB at 100k memories.

**[U] What is the realistic upper bound on store size?** 10k memories is roughly
six months of moderate agent use. At 1M, brute-force loses and every §3
recommendation needs revisiting. *How to find out:* instrument an alpha for
memory count over time; extrapolate. Design for 100k, verify the architecture
can reach 1M, do not build for 1M now.

---

## 10. Trade-offs & Limitations

### Observation

#### What they explicitly chose NOT to do [F]

| Not done | Evidence |
|---|---|
| Sync / replication / multi-device | grep `sync\|replicat\|litestream\|crdt` across `docs/roadmap.md`, `docs/comparison.md` → zero hits. Export/import JSONL is the entire portability story. |
| Encryption at rest | no crypto dependencies; `0600`/`0700` file permissions only (`engine.rs:74-79, 422-430`) |
| Per-agent authorization | namespace and room `author` are unverified request-supplied strings |
| Cross-encoder reranking | RRF plus optional Jaccard is the whole ranking stack |
| GPU inference | CPU-only ORT; no CUDA/Metal EP configured |
| Static binaries / musl | glibc only, ORT sidecar (§8) |
| Self-update | re-run `install.sh` |
| Benchmark CI / regression gates | no `criterion`, no bench workflow |

#### Known limitations and rough edges [F]

**a) Four parallel content/relationship models** (§2): `memory_edges` plus
`graph_nodes`/`graph_edges` plus tags-as-JSON-and-junction plus a separate
`documents` tree. The tag dual-write is labeled "for backward compat"
(`schema.rs:475-478`) as of v5 and is still dual-writing at v15.

**b) `RecallStrategy::Fts5` has no ranking at all.** `recall_fts5_only`
(`operations.rs:718-726`):

```rust
let score = 1.0f32; // Placeholder — actual ranking done by RRF in hybrid
```

Every result gets an identical score. Consequence: `min_score` filtering against
FTS5-only results is meaningless (everything passes or nothing does), and
results come back in raw FTS order. Hybrid mode is unaffected — this only bites
the standalone strategy.

**c) Repair-on-every-open** (`schema.rs:130-166`): four repair/verification
passes run on every single command invocation because the migration system is
not trusted (§3). A permanent tax paid for historical bugs.

**d) The `ef` parameter is dead** (`vector.rs`):
`pub fn search(&self, query: &[f32], k: usize, _ef: usize)` — computed by
callers, ignored by the implementation, because the Rust bindings do not expose
it. The widening retry loop therefore only half-works.

**e) `timeline_events` is 33% implemented** (§7): six declared event types, two
emitters. The module doc describes hooks that do not exist.

**f) Contradictory benchmarks** (§9): two files, ~40× apart, plus a
`*Pending run*` self-score printed beside competitors' real numbers.

**g) Inconsistent write ordering:** `remember_precomputed` takes the index lock
then writes SQLite; `update_memory` (`operations.rs:1190-1200`) writes SQLite
then the index. Opposite orders in the same codebase.

**h) The exclusive index lock serializes readers** (`vector.rs:74-77`) — WAL's
concurrent-read benefit is forfeited, with no comment acknowledging the trade.

**i) Storing an embedding-less memory on retry exhaustion**
(`operations.rs`, #621) — silent two-tier store, manual repair required.

**j) `check_duplicate` silently discards writes** at cos ≥0.95
(`operations.rs:214-265`) — the insert is skipped entirely and the existing ID
returned.

#### A likely defect — static analysis only, NOT runtime-verified

`memory/fts5.rs:94` and `:101` (and the token variants at `:177`, `:184`) select
these columns:

```sql
SELECT m.id, m.content, m.embedding, m.tags, m.metadata,
       m.created_at, m.updated_at, m.namespace, m.access_count,
       m.last_accessed, m.deprecated, m.valid_from, m.valid_until,
       m.memory_type, m.importance, m.pinned, m.content_type,
       m.source, m.source_type, f.rank
FROM memories_fts f JOIN memories m ON …
```

**`m.slug` is absent.** Counting positions:

| Idx | SELECT provides | `row_to_memory` (`store.rs:443-445`) reads |
|---:|---|---|
| 16 | `content_type` | `content_type` — correct |
| **17** | `source` | **`slug`** — wrong |
| **18** | `source_type` | **`source`** — wrong |
| **19** | `f.rank` (float) | **`source_type`** — wrong |

So every memory returned through the FTS5 channel carries: `slug` = the source
value, `source` = the source_type value, and `source_type` = `"unknown"` — the
float-to-String parse fails and hits `.unwrap_or_else(|_| "unknown")`.

Separately, `let rank: f64 = row.get(14)?` reads `importance`, not `f.rank`.

**Why it went unnoticed [I]:** the rank value is discarded by both callers —
bound as `_rank_val` (`operations.rs:794`) and `_rank` — because RRF uses
ordinal position, not the score. The visibly-wrong value is never observed, and
`slug`/`source` are provenance metadata that recall does not rank on.

**Blast radius [I]:** ranking is unaffected; provenance fields on FTS5-sourced
results are corrupted. `recall_rrf` merges by ID, so whether a given memory
shows correct provenance depends on which channel returned it — vector-channel
results are correct, FTS5-channel results are not. That non-determinism is the
nastiest part.

**Confidence:** the column mismatch is a fact, verified by counting the SELECT
list against `row_to_memory`'s indices. The runtime consequence is inference —
the code was not executed. To confirm:

```bash
uteke remember --source "test-src" "unique-phrase-xyz"
uteke recall "unique-phrase-xyz" --strategy fts5 --json   # inspect slug/source/source_type
```

### Inference

The debt clusters into three root causes, and only one is really about Rust [I]:

1. **The two-file storage split** (§3) produces the lock, the Windows
   workarounds, the full-index rewrite, the desync repair path, the room
   over-fetch, and the temporal post-filter. One decision, six symptoms.
2. **Migrations that are not trusted** produce the permanent repair-on-open tax.
   Root cause: fresh DBs stamping HEAD, plus best-effort DDL.
3. **Features shipped at partial completion** — the timeline enum,
   `recall_fts5_only`'s placeholder score, the dead `ef`, `*Pending run*`
   benchmarks. Individually small; together a pattern of "ship the shape, fill it
   in later" where later did not come.

**The FTS5 column bug is the most instructive item in this document [I]**,
because it is not a Rust problem, an architecture problem, or a hard problem —
it is the inevitable consequence of writing the same 19-column SELECT list in
four places and mapping it positionally. Go's `rows.Scan` has exactly the same
shape and exactly the same hazard.

**What they got genuinely right, and should not be "improved" [I]:** RRF with
ordinal ranks; excluding `min_score` from the cache key; applying recency boosts
post-cache; shipping the ORT library rather than downloading it;
checksum-pinned model downloads; `unsafe_code = "forbid"`; the newer-schema hard
error; the tar path-traversal guard; always-on DEBUG file logging; the dual-role
read-only token.

### Relevance to Go

**Design against the column bug structurally, not by being careful.** Three
defenses, use all of them:

```go
// 1. ONE column list, ever.
const memoryCols = `m.id, m.content, m.tags, m.metadata, m.created_at,
  m.updated_at, m.namespace, m.access_count, m.last_accessed, m.deprecated,
  m.valid_from, m.valid_until, m.memory_type, m.importance, m.pinned,
  m.content_type, m.slug, m.source, m.source_type`

// 2. ONE scan function, name-based.
func scanMemory(rows *sql.Rows) (*Memory, error) {
    var m Memory
    return &m, rows.Scan(&m.ID, &m.Content, /* … */)   // or sqlx.StructScan
}

// 3. A test that fails if they ever drift.
func TestColumnListMatchesScan(t *testing.T) {
    rows, _ := db.Query("SELECT " + memoryCols + " FROM memories m LIMIT 0")
    cols, _ := rows.Columns()
    if len(cols) != reflect.TypeOf(Memory{}).NumField() {
        t.Fatalf("column count %d != struct fields", len(cols))
    }
}
```

`sqlx` with `db:"..."` struct tags gives (2) and most of (3) for free. This class
of bug is 100% preventable and 100% invisible in review.

Avoid each root cause deliberately:

| Their debt | Prevention |
|---|---|
| Two-file split | vectors in SQLite (§3) |
| Untrusted migrations | fresh DBs run all migrations from v0; no best-effort DDL |
| Positional scanning | one column constant, name-based scan, drift test |
| Placeholder scores | a code path that cannot rank returns an error, not `1.0` |
| Dead parameters | if the binding does not support it, do not accept the parameter |
| Half-built enums | test that every declared type has an emission site |
| Contradictory benchmarks | one file, CI-generated |
| Inconsistent lock ordering | one write path; document the order once |
| Silent write-drops on dedup | flag duplicates, do not discard them |
| Silent embedding-less rows | count them, surface in `zbrain status`, auto-repair on start |

Two Go-specific hazards Uteke's Rust does not have:

1. **`database/sql` opens multiple connections.** With SQLite that means
   concurrent writers inside one process. Set `SetMaxOpenConns(1)` on the writer
   pool.
2. **No `Result` type means errors get dropped.** Rust's `?` makes ignoring an
   error visible; Go's `_` makes it invisible. Run `errcheck` in CI — their #549
   (`let _ = execute(...)`) is the Rust equivalent of the bug Go makes easier,
   and they still shipped it.

And one thing Go gives that they would have wanted: `context.Context` threading
gives cancellation and timeouts on every operation. Uteke has none — a hung
embedding call cannot be aborted. Thread `ctx` from the MCP handler down to
`QueryContext` and the embedder from the start; retrofitting it is miserable.

### Alternative

**Deliberately re-take one of their trade-offs:** if users are multi-device
(laptop plus desktop), sync is the feature Uteke cannot offer, and
Litestream-to-S3 or a simple CRDT over the append-only log is a genuine
differentiator. But it is a whole product axis, not a v1 feature — and
soft-update (§7) plus a `version` column (§6) is exactly the substrate sync would
need, another reason to add both now even without sync plans.

**Deliberately keep one of their omissions:** no encryption at rest is fine for a
local tool on an already-encrypted disk. Do not build it speculatively.

### Open Question

**[U] Is the FTS5 column defect real at runtime?** *How to find out:* the
two-command test above — 60 seconds of work in the cloned repo. Worth doing both
to confirm the analysis and because filing it upstream is a cheap way to open a
line to their maintainers, who are the best source for the §1 and §6 open
questions.

**[U] Which of their debts are known-and-accepted vs unknown?** *How to find
out:* search their issue tracker for each item. A tracked issue means "known,
deprioritized"; nothing means "unnoticed."

**[U] Is `unsafe_code = "forbid"` doing real work given C++ FFI dependencies?**
Their own code is safe; usearch and ORT are not, and the FFI boundary is where
memory bugs live. The transferable question is your own cgo boundary — which is
exactly the reason to prefer `modernc.org/sqlite` and a non-cgo embedder (§8).

---

## Synthesis: Adopt / Adapt / Avoid

### Adopt as-is

| Pattern | Source | Why |
|---|---|---|
| SQLite plus FTS5 plus RRF(k=60) core | `operations.rs:748-874`, `fts5.rs` | proven, ~10 lines of fusion, no better option at this scale |
| FTS5 external-content table plus sync triggers | `fts5.rs:20-40` | no duplicated text; the triggers are the correct pattern |
| Ordinal-rank RRF (discard BM25) | `operations.rs:794` | canonical and correct — do not "fix" it |
| `Embedder` interface from commit one | `embed/mod.rs` | swap ONNX ↔ Ollama ↔ cloud without touching callers |
| Recall cache keyed `(query, ns, limit, strategy)`, excluding `min_score` | `recall_cache.rs` | caches the expensive part, filters cheap per-call |
| Recency/salience boosts applied after cache read | `operations.rs:593-613` | keeps cache entries time-invariant |
| Permanent-vs-transient init-error caching | `lib.rs` (#822) | prevents repeated large retry storms |
| Importance formula (access/recency/connectivity/pinned, 30-day half-life) | `store.rs:274-328` | well-shaped; reimplement in SQL, not RAM |
| SHA256-pinned downloads, atomic tmp+rename | `engine.rs:236-420` | supply-chain plus clear failure messages |
| Tar path-traversal rejection | `install.sh:132` | use `filepath.IsLocal()` in Go |
| Newer-schema is a hard error with instructions | `schema.rs:121-127` | never operate on a future schema |
| Always-on DEBUG file log plus WARN console | `logging.rs:1-36` | best ops decision in the repo — add a size cap |
| Dual-role bearer tokens, pre-hashed at startup | `context.rs:32-75` (#409) | if HTTP ships |
| HOME env override for test isolation | `lib.rs` `uteke_home()` | `ZBRAIN_HOME` already does this |

### Adapt

| Their approach | zbrain version | Payoff |
|---|---|---|
| Vectors in a separate usearch file | vectors in SQLite (brute-force or `sqlite-vec`) | kills six symptoms from one root cause |
| Sequential vector→FTS | goroutines; start FTS during embedding | hides the lexical channel in the 40ms |
| Room recall = over-fetch plus post-filter | `WHERE room_id = ?` pushed into SQL | exact top-K, no `.min(200)`, no silent under-return |
| Temporal filter in application code | pushed into SQL | does not waste recall budget |
| `--at` over current rows only | soft-update plus `supersedes` edges | real content history, small cost |
| RFC3339 TEXT timestamps | `INTEGER` Unix micros | correct sort, better index, no repair pass |
| Four relationship/content models | one `edges` table plus one `items` table with `kind` | one traversal, one search path |
| Nine memory types | four or five | matches what retrieval actually uses |
| 32 flat MCP tools | five to seven noun-grouped, resources for documents | ~4k tokens of context reclaimed |
| All errors → `-32603` | `-32602` for args, `isError` for domain | agents can self-correct |
| Full index rewrite per insert | batched flush (dirty flag, interval, shutdown) | fixes the §9 insert cliff |
| Fresh DB stamps HEAD | run every migration from v0 | eliminates #544/#549 by construction |
| `schema_version` table | `PRAGMA user_version` | atomic with the transaction |

### Avoid

- Two storage files. The single highest-leverage rejection.
- Positional `rows.Scan` across duplicated SELECT lists.
- Best-effort DDL (`let _ = execute(...)` / `db.Exec` with a dropped error).
- JSON tags column alongside a junction table.
- Placeholder scores.
- Declared-but-unemitted enum variants.
- Dead parameters the implementation ignores.
- Auto-linking on every insert until an ablation proves it earns its cost.
- Silent write-drops on dedup.
- Two benchmark files.
- `jaccard_weight`, graph reranking, and the `ef` widening loop — their own
  defaults say these did not pay.
- cgo, unless static binaries are consciously traded away.

---

## Recommendation

Build zbrain's memory engine as a single static Go binary with one SQLite file
containing content, FTS5 index, and vectors together; run the two retrieval
channels in goroutines and fuse with RRF(k=60); make the embedder an interface
with a local HTTP backend as the default.

**Why this and not something else:**

1. **It deletes Uteke's largest problem class rather than defending against
   it.** The two-file split causes the cross-process lock, the Windows I/O
   workarounds, the desync-repair machinery, the per-insert full-index rewrite,
   the room over-fetch, and the temporal post-filter — roughly a third of their
   hardening code, all traceable to one decision. And room- and
   namespace-filtered search pushes into SQL, which is strictly more correct
   than what they can express, not merely simpler.

2. **It preserves the one structural advantage Go gives.** Uteke wanted a single
   static binary and ONNX Runtime took it away — a 3-binary tarball with a
   sidecar `.so`, separate AVX2 and SSE4.2 Linux builds, no musl, and a 190MB
   first-run download. `modernc.org/sqlite` plus an HTTP embedder keeps
   `CGO_ENABLED=0` intact: one `go build` per platform, no matrix, no sidecar,
   no CPU-feature detection. That advantage is real and fragile — one cgo
   dependency destroys it.

3. **Their own measurements say the trade costs nothing.** Recall latency is
   flat at ~40ms from 100 to 10,000 memories, and HNSW contributes under 1ms.
   The entire floor is embedding inference. Brute-forcing 10k × 768 vectors is
   single-digit milliseconds — invisible against a 40ms constant. This trades a
   performance advantage that cannot be measured for correctness and
   distribution advantages that can.

**One addition they do not have,** cheap now and expensive later: soft-update
instead of in-place `UPDATE`, plus a `version` column. That gives genuine content
history (the thing `docs/time-travel.md` promises and their code does not
deliver), conflict detection, and — should sync ever be wanted — the exact
substrate it requires.

### Rejected alternatives

| Rejected | Why |
|---|---|
| Separate HNSW index (Uteke's design) | buys sublinear search that cannot be measured behind a 40ms embedding call, and costs six symptoms. Revisit only past ~100k memories with brute-force measured as the bottleneck. |
| In-process ONNX via `onnxruntime_go` | reproduces Uteke's exact distribution tax — cgo, sidecar, AVX2/SSE4.2 splits, no static binary — in a language with less mature bindings. Add later behind the same `Embedder` interface if needed. |
| Cloud embedding API as default | kills offline-first, the entire premise. Keep as a backend option. |
| Postgres plus pgvector | requires a server. Same reason Uteke rejected it. |
| Full event sourcing for time-travel | unbounded storage and a projection layer on every read, for a capability nobody asked for. Soft-update delivers 90% at ~1% of the complexity. |
| Rooms in v1 | 1917 lines and 8 MCP tools in Uteke for a feature with no notification mechanism and no visible usage signal. `workspace` covers isolation. |
| Entity knowledge graph in v1 | their own default has it opt-in, which is their honest assessment of its value. |

---

## Verification Checklist

Ordered by how much a wrong answer would cost. The first three are worth doing
before writing implementation code.

| # | Question | How | Cost | If wrong |
|---|---|---|---|---|
| 1 | Does `modernc.org/sqlite` have FTS5? | 5-line spike: `CREATE VIRTUAL TABLE t USING fts5(x)` | 10 min | driver choice changes; static binary at risk |
| 2 | Where does brute-force lose to HNSW on target hardware? | N random 768-d vectors, N ∈ {1k…1M}, time full scan | 1 hr | index architecture changes |
| 3 | Is the ~40ms embedding floor real for the chosen model/runtime? | benchmark the embedder end-to-end | 1 hr | invalidates "search does not matter" |
| 4 | Is the FTS5 column defect real? | `uteke remember --source X`; `uteke recall --strategy fts5 --json`; inspect fields | 5 min | confirms/refutes §10 |
| 5 | Is the full-index rewrite the insert bottleneck? | time `save()` at 1k vs 10k in their repo | 30 min | changes how much to fear separate indexes |
| 6 | Does tool count degrade agent accuracy at ~30? | 5 vs 30 tools, 20 fixed requests, count correct selections | 1 hr | informs every future API decision |
| 7 | Do MCP clients support resources well today? | register one trivial resource, check the client lists it | 20 min | resources-for-documents plan is moot |
| 8 | Does a smaller model (MiniLM) lose meaningful recall quality? | same query set, both models, compare recall@10 | 2 hrs | unlocks embedded-model UX and 8× storage cut |
| 9 | Were Uteke's hardening issues real or defensive? | read issues #543, #621, #647, #684, #822 | 30 min | calibrates how much to pre-emptively copy |
| 10 | What is the real `memory_type` distribution? | `SELECT memory_type, COUNT(*) … GROUP BY 1` on a populated store | 5 min | three types vs five |
| 11 | How often does the room `.min(200)` cap bind? | instrument `recall_room_semantic` at 10k memories / 50-memory room | 30 min | hard evidence for the SQL-filter approach |
| 12 | Does auto-linking improve recall? | ablation on their bench harness, graph rerank on/off | 2 hrs | reclaim insert throughput |
