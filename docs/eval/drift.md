# Drift Harness — `internal/eval/drift.go`

Compares two eval JSON runs (same `queries.json`) and flags retrieval drift.

## Usage

```bash
go run ./internal/eval/drift.go --before docs/proofs/eval-baseline.json --after /tmp/new.json
go run ./internal/eval/drift.go --before docs/proofs/eval-baseline.json --after /tmp/new.json --json /tmp/drift.json
go run ./internal/eval/drift.go --before a.json --after b.json --threshold 0.05
go run ./internal/eval/drift.go --help
```

Flags:

| Flag | Required | Default | Description |
|---|:---:|---|---|
| `--before` | yes | — | Baseline eval JSON (e.g. `docs/proofs/eval-baseline.json` from `make eval`) |
| `--after` | yes | — | New eval JSON to compare |
| `--json` | no | — | Write machine-readable drift JSON to path (also printed after markdown if omitted) |
| `--threshold` | no | `0.05` | Absolute |ΔP| or |ΔR| threshold for drift (0.05 = 5%) |
| `--help`, `-h` | no | — | Show help |

## Inputs

Both JSON files are `internal/eval/eval.go` outputs:

```json
{
  "corpus_size": 1000,
  "precision_at_k": 1.0,
  "recall_at_k": 0.05,
  "mrr": 1.0,
  "ndcg_at_k": 1.0,
  "queries": [{"id":"q001","precision_at_k":1.0,"recall_at_k":0.02,"hits":10,"gap":false}]
}
```

Queries are paired by `id`. Unpaired IDs are listed in output but not used for McNemar.

## Outputs

- **Markdown table** to stdout: summary delta, McNemar contingency, per-query delta
- **JSON** to `--json` file or to stdout after `--- JSON ---` separator when `--json` omitted

### Drift rule

Flag `drift: true` if `|ΔP@K| > threshold` or `|ΔR@K| > threshold` (default 5%).

McNemar-like paired test:

- Binary hit = `hits > 0` (relevant doc retrieved in top-K).
- Contingency: `a` both hit, `b` before-only, `c` after-only, `d` both miss.
- If `b+c > 0`: `chi2 = (b-c)^2/(b+c)` and continuity-corrected `(|b-c|-1)^2/(b+c)`.
- `p = erfc(sqrt(chi2/2))` (df=1). Significant if `p < 0.05`.
- If `b+c == 0` or no paired queries: `feasible: false`, use simple per-query delta (`after.hits - before.hits`).

## Example output (markdown)

```
# Drift Report

- **Before:** `docs/proofs/eval-baseline.json` (corpus=1000, P@10=1.000, R@10=0.054)
- **After:** `/tmp/new.json` (corpus=1000, P@10=0.950, R@10=0.051)
- **Threshold:** |ΔP| or |ΔR| > 5.0%
- **Drift:** ⚠️ DETECTED — ΔP=-0.050 > 5.0%

## Delta (after − before)

| Metric | Before | After | Δ | Status |
|---|---|---|---|---|
| P@K | 1.0000 | 0.9500 | -0.0500 | ⚠️ drift |
| R@K | 0.0536 | 0.0510 | -0.0026 | ok |

## McNemar (paired per-query hits)

| Stat | Value |
|---|---|
| paired queries | 50 |
| a (both hit) | 48 |
| b (before hit, after miss) | 2 |
| c (before miss, after hit) | 0 |
| chi² | 2.0000 |
| p-value | 0.1573 |

## Per-Query Delta

| Query | Text | P_before | P_after | ΔP | Hits | ΔHits |
|---|---|---|---|---|---|---|
| q001 | local trusted memory | 1.000 | 0.900 | -0.100 | 10→9 | -1 |
```

Stderr also logs one-liner:

```
drift: DETECTED | ΔP=-0.050 ΔR=-0.003 threshold=5.0% reason: ΔP=-0.050 > 5.0% | McNemar chi2=2.000 p=0.157 feasible=true
```

## JSON schema (truncated)

```json
{
  "before": "docs/proofs/eval-baseline.json",
  "after": "/tmp/new.json",
  "threshold": 0.05,
  "delta": {"precision_delta": -0.05, "recall_delta": -0.002},
  "drift": true,
  "drift_reason": "ΔP=-0.050 > 5.0%",
  "mcnemar": {"a_both_hit":48,"b_before_only":2,"c_after_only":0,"chi2":2.0,"p_value":0.157,"feasible":true},
  "per_query": [{"id":"q001","delta_precision":-0.1,"delta_hits":-1}]
}
```

## CI use

```bash
make eval  # writes docs/proofs/eval-baseline.json
# ... after 1k new claims + reindex ...
go run ./internal/eval --corpus=1000 --limit=10 --json /tmp/new.json
go run ./internal/eval/drift.go --before docs/proofs/eval-baseline.json --after /tmp/new.json --json /tmp/drift.json
cat /tmp/drift.json | python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("drift", d["drift"], d["delta"])' /tmp/drift.json
```

## Notes

- Keep corpus vocab narrow (15 phrases) so hit rate >30%; otherwise drift is noise.
- `internal/eval/drift.go` has `//go:build ignore` so `go vet ./internal/eval/...` passes with two `package main` files;
  run drift via explicit file path `go run ./internal/eval/drift.go`.
- For McNemar, `hits>0` is the binary outcome; blocked/gap queries count as miss.
  Per-query `ΔHits` is the simple delta fallback when `b+c==0`.

## References

- Plan: `docs/plans/active/zbrain-optimization-plan.md` §6 Phase 3.1, §12.3 C1
- Baseline: `docs/proofs/eval-baseline.json`
- Queries: `docs/eval/queries.json`
