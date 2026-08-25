# Benchmark — FTS5/perf baseline

This document tracks the disposable SQLite FTS5 baseline for `zbrain` trusted memory retrieval.

> **Hardware note:** single run on local machine, not vendor comparison. Report `go env`, `ls -lh dist/zbrain`, and `uname -a` alongside numbers. Use the JSON artifact for diffing across commits.

## How to reproduce

```bash
# Fast sanity (100, 1k) — used by `make bench`
go run ./scripts/bench-fts5.go --sizes=100,1000 --json /tmp/b.json

# Full baseline (100, 1k, 10k) — reference like mcp-fts5-starter
go run ./scripts/bench-fts5.go --sizes=100,1000,10000 --json docs/proofs/bench-baseline.json

# Custom workspace / sizes
go run ./scripts/bench-fts5.go --sizes=100,1000 --workspace bench --json /tmp/b.json
cat /tmp/b.json | python3 -m json.tool
```

Flags:

- `--sizes` — comma-separated corpus sizes (default `100,1000,10000`)
- `--json` — write JSON array of results to file
- `--workspace` — workspace name (default `bench`, must satisfy `IsSafeWorkspaceName`)

Isolation: the harness creates a temporary `ZBRAIN_HOME` (`mktemp -d`), runs `EnsureConfig` + `CreateWorkspace(bench)` inside it, generates synthetic claims via `internal/runtime.ClaimStore`, approves them, then times cold `IndexStore.Rebuild("bench")`. The real `~/.zbrain` is never touched. The temp directory is removed with `os.RemoveAll`.

## Corpus shape (mirrors mcp-fts5-starter note)

- **Vocab narrow:** 15 fixed phrases that each hit >30% of corpus (e.g., `local trusted memory`, `evidence snapshot`, `workspace isolation`, `claim draft approved`, `reindex disposable`, `verified digest`, `trust validation`, `index fts5 sqlite`, `hybrid retrieval`, `promotion candidates`, `trust input manifest`, `canonical markdown`, `derived evidence`, `supporting claim`, `stale blocked gap`). Each synthetic document contains 3–5 of these phrases deterministically, plus filler to reach avg **3.5 KB** per doc. Queries use the same 15 phrases so FTS5 hit rate is meaningful.
- **Avg doc size:** ~3.5 KB body (title + description + tags + body). Total ~350 KB @100, ~3.4 MB @1k, ~34 MB @10k.
- **Approval:** all generated claims are `basis: owner`, drafted via `ClaimStore.WriteDraft` then approved via `ClaimStore.Approve` (verified digest computed automatically).

## Metrics captured per corpus size

| Field | Meaning |
|---|---|
| Corpus size | N claims (100 / 1k / 10k) |
| Index time | cold `IndexStore.Rebuild` wall time |
| Throughput | `N / seconds` (doc/s) |
| DB size | `os.Stat(indexes/<workspace>.sqlite).Size()` |
| Peak heap | `runtime.ReadMemStats` `HeapAlloc`/`Alloc` after `runtime.GC()` (post-rebuild) |
| Query p50 / p95 / p99 | lexical search latency measured via `IndexStore.Search` (15 queries × 3 iterations = 45 samples, warm-up 1× unmeasured, sorted, nearest-rank percentile) |

## Latest baseline

> Fill this table by pasting the stdout markdown from `go run ./scripts/bench-fts5.go --sizes=100,1000,10000`. Keep the previous row as history if you want diffing, or replace with current commit's numbers. Commit the JSON in `docs/proofs/bench-baseline.json` if you capture one.

| Corpus size | Index time | Throughput | DB size | Peak heap | Query p50 | p95 | p99 |
|---:|---|---|---|---|---|---|---|
| 100 | — | — | — | — | — | — | — |
| 1000 | — | — | — | — | — | — | — |
| 10000 | — | — | — | — | — | — | — |

Example (illustrative, not measured on this machine):

| Corpus size | Index time | Throughput | DB size | Peak heap | Query p50 | p95 | p99 |
|---:|---|---|---|---|---|---|---|
| 100 | 120ms | 833 doc/s | 420 KB | 8.2 MB | 0.45ms | 0.90ms | 1.20ms |
| 1000 | 950ms | 1052 doc/s | 3.2 MB | 14.5 MB | 1.10ms | 2.30ms | 3.80ms |
| 10000 | 9.8s | 1020 doc/s | 31 MB | 19.8 MB | 4.2ms | 9.1ms | 14ms |

## JSON artifact

`--json` writes an array like:

```json
[
  {
    "corpus_size": 100,
    "workspace": "bench",
    "index_time_ms": 123.4,
    "index_time": "123.4ms",
    "throughput_docs_per_sec": 810.2,
    "db_size_bytes": 431200,
    "db_size_human": "421.1 KB",
    "peak_heap_bytes": 8600000,
    "peak_heap_human": "8.2 MB",
    "query_p50_ms": 0.45,
    "query_p95_ms": 0.90,
    "query_p99_ms": 1.20,
    "query_p50": "450µs",
    "query_p95": "900µs",
    "query_p99": "1.2ms",
    "total_queries": 45,
    "approved": 100
  }
]
```

Use `diff` across runs to evaluate perf regressions after Phase 1 (WAL+NORMAL, stripped build).

## References

- `internal/runtime/index_benchmark_test.go` `TestAskP95At100K` — claim generation pattern for 100k corpus (adapted here).
- `internal/runtime/index_test.go` `indexClaim` helper — claim shape.
- `mcp-fts5-starter/docs/benchmark.md` — narrow-vocab methodology, table shape (corpus size | index time | throughput | DB size | peak heap | p50/p95/p99).
