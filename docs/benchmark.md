# Benchmark — FTS5/perf baseline

This document tracks the disposable SQLite FTS5 baseline for `zbrain` trusted memory retrieval.

> **Hardware note:** single run on local machine, not vendor comparison. Report `rustc --version`, `ls -lh dist/zbrain`, and `uname -a` alongside numbers. Use the JSON artifact for diffing across commits.

## How to reproduce

```bash
# Ask p95 bench (env-gated; set ZBRAIN_BENCH_100K=1 for the full 100k corpus)
ZBRAIN_BENCH_100K=1 cargo test -p zbrain --test bench_100k

# The legacy Go harness (scripts/bench-fts5.go) was retired at the Rust
# cutover; historical numbers live in docs/proofs/bench-baseline*.json.
```

Flags:

- `ZBRAIN_BENCH_100K` — run the full 100k corpus (default off; smaller corpus otherwise)

Isolation: the harness creates a temporary `ZBRAIN_HOME` (`mktemp -d`), runs `ensure_config` + `create_workspace(bench)` inside it, generates synthetic claims via the claim store, approves them, then times cold index rebuilds. The real `~/.zbrain` is never touched.

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

> Measured 2026-08-25 on `go1.24.0 linux/amd64 6.17.0-40-generic`, single run, `ZBRAIN_HOME` temp. Commit JSON in `docs/proofs/bench-baseline.json` (100/1k) and `docs/proofs/bench-baseline-full.json` (100/1k/10k). Use diff across commits to evaluate regressions after WAL+NORMAL and stripped build. Previous placeholder kept for reference below.

**Current (after Wave 0-3, WAL+NORMAL, stripped 15M):**

| Corpus size | Index time | Throughput | DB size | Peak heap | Query p50 | p95 | p99 |
|---:|---|---|---|---|---|---|---|
| 100 | 108ms | 924 doc/s | 592 KB | 466 KB | 6.86ms | 7.24ms | 7.36ms |
| 1000 | 370ms | 2700 doc/s | 5.1 MB | 793 KB | 52.4ms | 54.0ms | 54.2ms |
| 10000 | 3.17s | 3155 doc/s | 50.0 MB | 4.6 MB | 484ms | 498ms | 509ms |

**Wave 0 baseline (before WAL, unstripped 22M):**

| Corpus size | Index time | Throughput | DB size | Peak heap | Query p50 | p95 | p99 |
|---:|---|---|---|---|---|---|---|
| 100 | 79ms | 1262 doc/s | 592 KB | 463 KB | 6.64ms | 7.26ms | 7.36ms |
| 1000 | 1.33s | 752 doc/s | 5.1 MB | 1.0 MB | 50.7ms | 52.8ms | 53.4ms |

> Δ after WAL: 1000 throughput 752→2700 doc/s (+259%), index 1.33s→0.37s (-72%). Query p50 stable. 10k shows FTS5 scan cost dominates (p50 484ms vs target <100ms — needs Phase 2 RRF/index tuning per plan §12).

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

- `index_benchmark_test.go` `TestAskP95At100K` (Go era) — claim generation pattern for 100k corpus (adapted here).
- `index_test.go` `indexClaim` helper (Go era) — claim shape.
- `mcp-fts5-starter/docs/benchmark.md` — narrow-vocab methodology, table shape (corpus size | index time | throughput | DB size | peak heap | p50/p95/p99).
