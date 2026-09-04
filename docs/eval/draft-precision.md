# Draft-Precision Judge Protocol

Protocol for scoring whether a **campaign draft materially traces to its bound
evidence**. zbrain core runs **zero model calls** in this protocol: an external
LLLM judge runs outside the runtime, and zbrain only (a) exports the drafts a
campaign produced, (b) imports the judge's verdicts as data, and (c) validates
the import and recomputes the metric.

## Rubric (for the external judge)

For each submitted campaign draft D with bound evidence snapshots E(D):

- Read D's body and every evidence snapshot in E(D) (local immutable copies only).
- Score **material tracing**: does D's substantive assertion rest on content
  that is actually present in E(D)?
  - `supported` (trace_score ≥ threshold): the draft's core claim statement is
    directly grounded in quoted/paraphrased evidence content.
  - `unverified` (trace_score < threshold): the draft introduces claims not
    traceable to any bound evidence.
  - `contradicted`: the draft asserts the opposite of what the bound evidence
    says.
- A draft with no bound evidence can never be `supported`.

## Import format — `zbrain.eval.draft-precision/v1`

```json
{
  "schema": "zbrain.eval.draft-precision/v1",
  "run": {"workspace": "research", "run_label": "golden", "created_at": "2026-09-01T09:00:00Z"},
  "judge": {"name": "mock-judge-fixture", "model": "none (imported verdicts)", "rubric_version": 1, "threshold": 0.7},
  "verdicts": [
    {
      "draft_index": 0,
      "title": "Ingestion Flush Cadence",
      "body_sha256": "…64 hex…",
      "bound_evidence_count": 1,
      "verdict": "supported",
      "trace_score": 1.0,
      "rationale": "body restates the evidence snapshot verbatim"
    }
  ],
  "metrics": {"total": 3, "supported": 2, "draft_precision": 0.6667}
}
```

Field rules (validated by `TestEvalDraftPrecisionGolden`, fail closed):

- `schema` must be exactly `zbrain.eval.draft-precision/v1`.
- `verdicts` must be sorted by `draft_index`, contiguous from 0, and cover
  every draft of the run — no unknown index, no gaps.
- `verdict` ∈ {`supported`, `unverified`, `contradicted`}.
- `trace_score` ∈ [0, 1]; `supported` requires score ≥ `judge.threshold`.
- `metrics.supported` must equal the number of `supported` verdicts;
  `metrics.draft_precision = supported / total` (0 when total = 0).
- Golden files identify drafts by stable content (`title`, `body_sha256`,
  `bound_evidence_count`) — never by random claim/evidence IDs.

## Golden run

`docs/proofs/eval-draft-precision-golden.json` is the committed golden
artifact: a fixture campaign (deterministic clock via `CampaignStore{Now}`)
produced three evidence-bound drafts, an offline mock judge scored them, and
the verdicts were imported as data. The runner rebuilds the same fixture
campaign, imports the golden verdicts, validates every field rule plus the
content identity of each verdict, and recomputes the metric — asserting the
import format round-trips and that precision is reproducible.
