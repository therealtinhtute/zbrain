# Trust-Integrity Eval Corpus

Adversarial corpus for `TestEvalTrustIntegrity` (runner lives in
`internal/eval/trust_integrity_test.go` so it can drive **both** query
layers with one fixture set: the `zbrain ask` CLI path via `internal/cli.App`
and the `memory_ask` MCP tool over the in-memory client harness).

## Corpus shape

Fixtures are generated deterministically by the test into isolated temp
runtimes (`t.TempDir()`; `ZBRAIN_HOME` is never touched). No large binaries —
each case is one tiny workspace with 1–3 OKF Markdown claims. Cases:

| Case | Fixture state | Required outcome (trusted-memory-spec.md §9) |
|---|---|---|
| `draft_alongside_approved` | approved claim + draft sharing query terms | `ready`; draft appears **only** in `promotion_candidates`, never as a trusted claim |
| `revoked_claim` | claim approved then revoked | `gap`; revoked content absent from every response field |
| `superseded_claim` | claim approved, then superseded by an approved replacement | `ready`; only the replacement is returned |
| `conflicting_approved` | two approved claims with `conflicts_with` | `blocked` with explicit conflict pair |
| `digest_tampered_approved` | approved claim's `verified_digest` rewritten, then reindex | index state `rejected`; `ask`/`memory_ask` fail closed with an error |
| `stale_index` | canonical body edited after rebuild | explicit stale error, fail closed |
| `dirty_index` | `.dirty` marker present | explicit error, fail closed |
| `missing_index` | index database removed | explicit error, fail closed |
| `unindexed_legacy_doc` | new canonical claim file written after rebuild | explicit stale error, fail closed |

## Fail-closed invariant

For every case and both layers: no draft, revoked, superseded, or
digest-invalid claim text may surface as a trusted (`ready`) result. Blocked
must be an explicit status, gaps must be explicit, and index failures must be
hard errors — never silent lexical fallback.

## Results

The runner writes `docs/proofs/eval-trust-integrity.json`
(`zbrain.eval.trust-integrity/v1`): one record per case per layer with the
observed status or error class. The file is deterministic (no timestamps or
machine paths) so re-runs diff clean.
