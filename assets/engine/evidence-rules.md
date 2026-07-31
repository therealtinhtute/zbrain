# Evidence Rules

## Current Pipeline

1. Capture a local snapshot with `zbrain evidence add --file <path> --origin <uri-or-path>`.
2. Draft claims that reference evidence IDs when the claim is based on external facts.
3. Approve only claims that pass their basis-specific proof rule.
4. Run `zbrain reindex` before trusted retrieval.

## Required Guards

- Evidence snapshots live under the owning workspace and are immutable after capture.
- Factual external claims require at least one evidence ID before approval.
- Owner preferences and decisions may be approved by owner confirmation.
- Derived claims require supporting claim IDs or evidence IDs.
- Evidence raw content is not searched as trusted memory; only approved claims are trusted context.
