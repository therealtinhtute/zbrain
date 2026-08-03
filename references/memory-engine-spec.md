# zbrain — Memory Engine Spec (Proposed)

> Forward-looking design for zbrain's retrieval layer. **Not in force.**
> The authoritative spec for what is built today is [`trusted-memory-spec.md`](../trusted-memory-spec.md).
> Evidence base: [`uteke-analysis.md`](uteke-analysis.md).

**Status:** proposed · **Created:** 2026-08-03 · **Supersedes:** nothing

## 0. Read This First

`trusted-memory-spec.md` §2 lists as **out of scope for the current slice**: vector
databases, MCP integration, LLM/model-provider calls, hosted sync, and
background services. This document specifies several of those.

That is intentional and it is not a contradiction — this is a **later slice**.
Nothing here should be implemented until `trusted-memory-spec.md` §2 is amended to pull an
item into scope. Treat every section below as a decision already reasoned
through, waiting for a green light, not as work queued up.

What this document is for: when a future session opens the question "how should
zbrain do vector search / hybrid retrieval / temporal queries," the answer is
here with its reasoning, so the analysis is not re-derived.

## 1. Evidence Base

Every decision below traces to source-level analysis of **Uteke v0.11.0**
(`github.com/codecoradev/uteke`, commit `afb98c6`), recorded in
`references/uteke-analysis.md`. Section references like *(analysis §3)* point
there.

Note the URL: the commonly-cited `codecora/uteke` is 404. The real repo is
`codecoradev/uteke`.

Claims in the analysis are tagged **[F]** fact / **[I]** inference /
**[U]** undiscoverable. Decisions here that rest on **[I]** are marked
*(inference)* and should be verified before they become expensive to reverse —
see §10.

## 2. Scope

**In scope for this spec:**

- Storage topology for content, lexical index, and vectors
- Hybrid retrieval and rank fusion
- Embedder abstraction and model identity
- Schema migration discipline
- Temporal model (content history)
- Concurrency and write ordering

**Out of scope, deliberately:**

- Multi-device sync / replication — Uteke has zero prior art here
  (analysis §10); needs its own investigation
- Encryption at rest — local tool on an already-encrypted disk
- Rooms / multi-agent sharing — deferred, see §9
- Entity knowledge graph — deferred, see §9
- GPU inference
- Anything in `trusted-memory-spec.md` §2 that stays out of scope

## 3. Decision: One Store, One File

**Vectors live in SQLite alongside content and the FTS5 index. There is no
separate index file.**

This is the load-bearing decision and everything else follows from it.

Uteke keeps a `usearch` HNSW index in a file next to SQLite. Roughly a third of
their hardening code exists to manage the consequences (analysis §3, §10):

| Symptom | Root cause |
|---|---|
| Cross-process `fs2` exclusive lock (serializes readers) | two files, no shared transaction |
| Windows `MAX_PATH` / AV / mmap workarounds | C++ file I/O on a second file |
| Full index rewrite on **every** insert | no row-level persistence |
| `uteke repair` + desync recovery | index can diverge from SQLite |
| Room recall = fetch 200 globally, then filter | index cannot express `WHERE room_id = ?` |
| Temporal filter applied in application code | same |

One decision, six symptoms. Collapsing into one file deletes all six —
structurally, not by defending against them.

**Justified by their own numbers** (analysis §9, `docs/benchmarks.md`): recall
latency is flat at ~40ms from 100 to 10,000 memories, and HNSW contributes under
1ms. The floor is embedding inference. Brute-forcing 10k × 768 f32 vectors is
single-digit milliseconds — invisible behind a 40ms constant. Sublinear search
buys nothing measurable at this scale.

Implementation, in preference order:

1. **Brute-force scan** over the f32 BLOB column. Zero dependencies, trivially
   correct. Correct choice for the first slice.
2. **`sqlite-vec`** (`github.com/asg017/sqlite-vec-go-bindings`) if brute-force
   is measured as a bottleneck.
3. **Separate HNSW index** only past ~100k memories with measurement in hand —
   and then re-read analysis §3 for what it costs.

Put all three behind one interface so the swap is a one-file change:

```go
type VectorIndex interface {
    Add(ctx context.Context, id string, vec []float32) error
    Search(ctx context.Context, vec []float32, k int, filter Filter) ([]Hit, error)
    Delete(ctx context.Context, id string) error
}
```

**Constraint:** `modernc.org/sqlite` (pure Go, no cgo). This preserves
`CGO_ENABLED=0` — see §8.

## 4. Storage Model

### Pragmas

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;   -- safe under WAL; Uteke left this at FULL
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;
PRAGMA temp_store   = MEMORY;
PRAGMA cache_size   = -64000;   -- 64MB
```

Uteke sets only the first, third, and fourth (analysis §3). `synchronous=NORMAL`
under WAL removes an fsync per commit and risks at most the last transaction on
OS crash — free throughput for a local store.

### Connection policy

`database/sql` opens multiple connections by default; with SQLite that means
concurrent writers inside one process. Set `db.SetMaxOpenConns(1)` on the writer.
Reads may use a separate WAL pool.

### Timestamps

`INTEGER` Unix microseconds. Never RFC3339 TEXT.

Uteke uses text and carries a `repair_datetime_timezones()` pass that runs on
every database open, because some writer emitted strings without `+00:00` and it
only failed at read time (analysis §3, §7). Integers sort correctly by
construction and have no parse-failure class.

### Relationship model — one table

One `edges` table serves memory↔memory, entity↔memory, and doc↔memory. Uteke has
four parallel models (`memory_edges`, `graph_nodes`/`graph_edges`, tags as both
JSON and junction, plus a separate `documents` tree) that accreted across schema
versions v5→v15 and never converged (analysis §2).

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

### Tags

Junction table only — `tags(item_id, tag)` with an index on `tag`. Never a JSON
column. Uteke dual-writes JSON *and* a junction table, labeled "for backward
compat" at schema v5 and still dual-writing at v15 (analysis §2).

### `wiki/` vs `evidence/`

`trusted-memory-spec.md` already declares two content spaces. **One `items` table with a
`kind` discriminator, one FTS index, one search path.** Uteke chose separate
tables for memories vs documents and pays with duplicated FTS setup, duplicated
search code, and seven extra MCP tools (analysis §2).

### Column scanning — non-negotiable

The one confirmed defect in Uteke's codebase (analysis §10) is a column-index
mismatch: `fts5.rs` omits `m.slug` from four duplicated SELECT lists while
`store.rs` scans positionally, so every FTS-channel result gets a wrong `slug`, a
shifted `source`, and `source_type = "unknown"`. Go's `rows.Scan` has the
identical hazard.

Three defenses, all of them:

1. One `const memoryCols` string, used everywhere.
2. Name-based scanning (`sqlx.StructScan` with `db:"..."` tags).
3. A test asserting column count matches struct fields.

## 5. Retrieval

### Two channels, in parallel

```go
vecCh, ftsCh := make(chan chanResult, 1), make(chan chanResult, 1)
go func() { h, err := s.vectorSearch(ctx, qvec, limit*3); vecCh <- chanResult{h, err} }()
go func() { h, err := s.ftsSearch(ctx, query, limit*3);   ftsCh <- chanResult{h, err} }()
v, f := <-vecCh, <-ftsCh
```

Uteke runs these sequentially — a Rust ergonomics artifact, not a design choice
(analysis §4, *inference*). Go makes it four lines.

Go further: the FTS query does not depend on the embedding, so start it *before*
the embed call returns. The entire lexical channel hides inside the 40ms.

### FTS5 external-content table

```sql
CREATE VIRTUAL TABLE items_fts USING fts5(
    content, tags, namespace, kind,
    content='items', content_rowid='rowid'
);
```

Plus `_ai` / `_ad` / `_au` sync triggers using the `('delete', ...)` insert form
external-content tables require. This part of Uteke is correct — copy it
(analysis §4).

FTS5 has no `ALTER TABLE ADD COLUMN`; adding a column means drop + recreate +
rebuild. Choose the column set deliberately.

### Fusion: RRF, k=60

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
    // normalize by maxRRF, sort desc, truncate
}
```

**Ordinal rank only — discard BM25 and cosine scores.** This is canonical RRF
(Cormack et al. 2009) and it is correct, not a shortcut: a BM25 of 12.4 and a
cosine of 0.83 share no scale, so normalizing them would be worse.

Parameterize `maxRRF` by `len(channels)`. Uteke hardcodes `2.0`, which silently
breaks if a third channel is added.

`min_score` must **not** be applied to fused output — RRF scores are not
cosine-comparable.

### Filters push into SQL

Room, namespace, and temporal filters go in the `WHERE` clause, not
application-side post-filtering:

```sql
SELECT i.id, i.content, vec_distance_cosine(v.embedding, ?) AS dist
FROM   vec_items v
JOIN   items i          ON i.id = v.item_id
JOIN   room_items r     ON r.item_id = i.id
WHERE  r.room_id = ? AND i.deprecated = 0
  AND  i.created_at <= ?
  AND  (i.valid_until IS NULL OR i.valid_until >  ?)
  AND  (i.valid_from  IS NULL OR i.valid_from  <= ?)
ORDER  BY dist LIMIT ?;
```

Exact top-K within the filter, always. Uteke cannot express this — they fetch
`(limit*5).min(200)` globally then drop non-members, which silently under-returns
when the filtered set is a small or atypical slice of the store (analysis §6).

### Caching

- Key: `(query_hash, namespace, limit, strategy)` — **exclude `min_score`**, so
  the expensive part caches and the cheap filter runs per-call.
- Apply recency/salience boosts **after** the cache read, so entries stay
  time-invariant under a 300s TTL.
- Wrap with `golang.org/x/sync/singleflight` so concurrent identical queries
  share one embedding call. Uteke has no such dedup.

Both cache decisions are Uteke's and both are good (analysis §4).

### Not building

`jaccard_weight`, graph reranking, and the `ef` widening loop. Uteke ships the
first two **disabled by default** — read that as their own evaluation. The third
is dead code in their repo (`ef` is accepted and ignored) and irrelevant under
brute-force.

## 6. Embedding

### Interface first

```go
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
    Dims() int
    ID() string   // model identity
}
```

**`ID()` is the addition Uteke lacks and it matters.** Switching embedding
models invalidates every stored vector — different model, different space. Uteke
has no guard, so moving from ONNX to OpenAI silently degrades recall with no
error. Stamp the model ID per row; refuse or auto-reindex on mismatch.

### Default backend: local HTTP (Ollama / llama.cpp)

| Option | cgo | Static binary | First run |
|---|---|---|---|
| **HTTP → Ollama** | no | **yes** | user installs Ollama |
| `onnxruntime_go` | yes | no | 190MB model download |
| Cloud API | no | yes | API key, kills offline-first |

Uteke chose in-process ONNX and lost static linking to it (§8). Keeping the
embedder behind HTTP preserves `CGO_ENABLED=0`. Add an ONNX backend later behind
the same interface if the external dependency proves unacceptable.

### Write path

1. `EmbedBatch` on bulk import — 32-64 texts amortize model overhead 5-10×.
   Uteke has no batching at all, and this is part of their insert-throughput
   problem (analysis §9).
2. On embed failure after retry (3 attempts, 200/400/800ms), store the item
   **without** an embedding — but **count it** and surface it in `zbrain status`,
   and repair automatically on next start. Uteke logs a line and requires the
   user to notice they need `uteke repair`.
3. Dedup at cosine ≥0.95 should **flag** the duplicate, not silently skip the
   insert. Uteke discards the write (analysis §4) — a data-loss failure mode.

### Copy from Uteke

- SHA256-pinned model downloads, atomic tmp+rename, tmp cleanup on start
- Permanent-vs-transient init-error caching (their #822) — a
  `struct{ err error; until time.Time }` prevents repeated large retry storms
- Taking the model's own pooled output rather than manual mean-pooling; masked
  mean-pooling is one of the most common embedding bugs

## 7. Temporal Model

**Soft-update, not in-place `UPDATE`.**

Uteke's time travel is a filter over *current* rows — `created_at` /
`valid_from` / `valid_until` compared against a point in time. `update_memory`
overwrites content in place with no history, so `--at` returns present content
with a historical existence filter. Their `docs/time-travel.md` promises "trace
how knowledge evolved" and "compare state between two points," which the design
cannot do (analysis §7).

They already had the pieces — a `deprecate()` path and a `supersedes` edge type
declared at schema v8 — and never wired them into updates.

```go
func (s *Store) Update(ctx context.Context, id, newContent string) (string, error) {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil { return "", err }
    defer tx.Rollback()

    now, newID := time.Now().UnixMicro(), uuid.NewString()

    // 1. close the old row's validity interval
    // 2. insert the successor with valid_from = now
    // 3. edge: newID --supersedes--> id

    return newID, tx.Commit()
}
```

Cost: an item edited 10 times keeps 11 rows. Prune superseded rows older than N
days if it ever matters.

Gain: the same `--at` filter returns genuinely correct historical content, the
`supersedes` chain gives version history, and — should sync ever be built — this
is exactly the substrate it needs.

### `version` column

Add `version INTEGER` now. Eight bytes, and it turns last-write-wins into
detectable conflict:

```go
res, _ := tx.ExecContext(ctx,
    `UPDATE items SET content=?, version=version+1, updated_at=?
     WHERE id=? AND version=?`, content, now, id, expected)
if n, _ := res.RowsAffected(); n == 0 { return ErrConflict }
```

Impossible to retrofit cheaply. Uteke has no version column and cannot detect
lost updates (analysis §6).

### Indexes

```sql
CREATE INDEX idx_items_temporal ON items(valid_from, valid_until) WHERE deprecated = 0;
CREATE INDEX idx_items_created  ON items(created_at);
CREATE INDEX idx_edges_super    ON edges(dst_id, rel) WHERE rel = 'supersedes';
```

Partial indexes matter — most queries filter `deprecated = 0`, and under
soft-update the deprecated set grows.

### Timeline events

If a timeline table is built, **every declared event type must have an emission
site**, enforced by a test. Uteke declares six types and emits two; the module
doc describes lifecycle hooks that do not exist (analysis §7). An
enum-without-emitters is a trap for anyone reading the code later.

## 8. Migrations & Distribution

### Migration discipline — three rules, all learned from their scars

1. **Fresh databases run every migration from v0.** Never stamp HEAD. Uteke
   stamps `CURRENT_SCHEMA_VERSION` on fresh DBs, so side effects living only
   inside migration functions were absent on new installs — that is their #544
   (FTS5 never created on fresh DBs). Their fix was a repair function, so the
   check now runs on *every* database open, forever.
2. **No best-effort DDL.** Their #549 is `let _ = execute(...)` — a silent
   failure that left the version stamp lying. Go makes this *easier* to do by
   accident: run `errcheck` in CI.
3. **`PRAGMA user_version`**, not a `schema_version` table. Atomic 4-byte header
   field, participates in the transaction, no query needed.

Keep their newer-than-supported hard error — never operate on a future schema.

```go
var v int
db.QueryRow("PRAGMA user_version").Scan(&v)
if v > len(migrations) { return ErrSchemaTooNew }
for i := v; i < len(migrations); i++ {
    tx, _ := db.Begin()
    if err := migrations[i](tx); err != nil { tx.Rollback(); return err }
    tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1))
    tx.Commit()
}
```

### Static binary is a protected property

Uteke wanted a single binary and ONNX Runtime took it: their release tarball
ships three binaries plus an ORT sidecar `.so`, with separate AVX2 and SSE4.2
Linux builds, no musl, and a 190MB first-run model download (analysis §8).

zbrain can ship what they wanted:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/zbrain ./cmd/zbrain
```

Two decisions preserve this and either one destroys it:

| Decision | Preserves | Destroys |
|---|---|---|
| SQLite driver | `modernc.org/sqlite` | `mattn/go-sqlite3` |
| Embedder | HTTP / cloud | `onnxruntime_go` |

Guard it in CI — a job that runs `CGO_ENABLED=0 go build` and fails otherwise
costs nothing and stops a dependency from quietly reintroducing cgo.

The existing `assets/` + `go:embed` model is already better than Uteke's runtime
download. Preserve it.

### Observability

```go
h := slog.NewJSONHandler(&lumberjack.Logger{
    Filename: filepath.Join(zbrainHome, "zbrain.log"),
    MaxSize:  10, MaxBackups: 5, MaxAge: 30,   // the size cap Uteke lacks
}, &slog.HandlerOptions{Level: slog.LevelDebug})
```

Always-on DEBUG file log plus WARN console is the single best ops decision in
their repo — when a user reports "recall returned nothing," you ask for the log.
Add the rotation cap they do not have.

Log the resolved `ZBRAIN_HOME` at startup, one INFO line. Uteke had a bug where
the MCP binary hardcoded `~/.uteke` and *"silently opened an empty second store
and never saw anything the CLI had written"* — one log line makes that
self-diagnosing.

### Install

Copy from their `install.sh`: SHA256 verification against a published
`checksums.txt`, OS/arch detection, and the tar path-traversal rejection at
`install.sh:132`. Go's `archive/tar` has the same hazard — use
`filepath.IsLocal()` on every entry.

Add what they lack: `goreleaser` plus a non-blocking, cached version check.

## 9. Deferred

| Feature | Why deferred |
|---|---|
| **Rooms / multi-agent sharing** | 1917 lines and 8 MCP tools in Uteke for a feature with no notification mechanism and no visible usage signal. `workspace` already covers isolation. Build when two agents actually need to share, and let that case shape it. If built: junction table (memories stay in their author's namespace, rooms are a view), and push the filter into SQL per §5. |
| **Entity knowledge graph** | Uteke's own default has graph reranking opt-in — their honest assessment of its value. Ship memory↔memory edges only. |
| **Auto-linking on insert** | Uteke runs a vector search and writes edges at cos ≥0.80 on *every* insert — write amplification for a signal only consumed in opt-in mode. Requires an ablation before it earns its cost. |
| **MCP surface** | Out of scope per `trusted-memory-spec.md` §2. When in scope: 5-7 noun-grouped tools, not 32 flat ones (~4k tokens of context reclaimed); `resources/*` for addressable content, `tools/*` for mutations; error codes mapped properly (`-32602` for bad args, `isError` for domain failures, `-32603` only for genuine faults) so agents can self-correct. Use `github.com/modelcontextprotocol/go-sdk` — Uteke hand-rolled JSON-RPC and their content-union encoding shows the cost. |
| **Sync / replication** | Uteke has zero prior art (grep for `sync\|replicat\|litestream\|crdt` across their docs returns nothing). Needs its own research pass. §7's soft-update + `version` column is the substrate it would need — which is another reason to add both now. |

## 10. Verify Before Building

Ordered by cost of a wrong answer. The first three block implementation.

| # | Question | How | Cost | If wrong |
|---|---|---|---|---|
| 1 | Does `modernc.org/sqlite` ship FTS5? | `CREATE VIRTUAL TABLE t USING fts5(x)` on `:memory:` | 10 min | driver changes; static binary at risk |
| 2 | Where does brute-force lose to HNSW on target hardware? | N random 768-d vectors, N ∈ {1k…1M}, time full scan | 1 hr | §3 changes |
| 3 | Is the ~40ms embedding floor real for the chosen model? | benchmark the embedder end-to-end | 1 hr | invalidates "search does not matter" |
| 4 | Is Uteke's FTS5 column defect real at runtime? | `uteke remember --source X`; `uteke recall --strategy fts5 --json` | 5 min | confirms analysis §10 |
| 5 | Is the full-index rewrite their insert bottleneck? *(inference)* | time `save()` at 1k vs 10k in their repo | 30 min | changes how much to fear separate indexes |
| 6 | Does a smaller model (MiniLM-L6) lose meaningful recall? | same query set, both models, recall@10 | 2 hrs | unlocks embedded-model UX + 8× storage cut |

### Targets to define

None of the performance work is evaluable without these. Proposed starting
point, to be confirmed:

- recall p95 < 100ms
- insert > 50/s single, > 500/s batched
- store < 200MB at 100k items

Design for 100k. Verify the architecture can reach 1M. Do not build for 1M.

### One benchmark artifact

`zbrain bench` writes `docs/benchmarks.md`; CI regenerates it on tag; a
`testing.B` + `benchstat` gate catches regressions. Uteke publishes two benchmark
files that disagree by ~40× on insert, and their competitive comparison table
still reads `*Pending run*` for their own score next to competitors' published
numbers (analysis §9).

## 11. Anti-Patterns

Do not do these. Each is observed in Uteke's codebase with a traceable cost.

- Two storage files
- Positional `rows.Scan` across duplicated SELECT lists
- Best-effort DDL — `db.Exec` with a dropped error
- A JSON tags column alongside a junction table
- Placeholder scores (`let score = 1.0f32` — makes `min_score` meaningless)
- Declared-but-unemitted enum variants
- Parameters the implementation ignores (`_ef`)
- Silent write-drops on dedup
- Storing items without embeddings and only logging it
- Two benchmark files
- Inconsistent lock ordering between write paths
- `cgo`, unless static binaries are consciously traded away
