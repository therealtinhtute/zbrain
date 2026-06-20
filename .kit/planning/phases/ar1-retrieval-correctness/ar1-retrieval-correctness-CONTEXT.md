# Context: Retrieval Correctness

Phase: ar1-retrieval-correctness
Status: ready
Spec Link: ../../SPEC-audit-remediation.md
Roadmap Link: ../../ROADMAP-audit-remediation.md
Blast Radius: medium
Expected Proof: unit, integration

## Goal
Fix retrieval ranking, multi-workspace slot allocation, query parsing, and limit validation — ISSUE-008, 009, 022, 023.

## Scope Boundary
### Allowed Surfaces
- `src/core/retrieval-ranking.ts` (008)
- `src/commands/ask.ts` (022)
- `src/core/query-parser.ts` (023 + `ParsedQuery.tags` for 009)
- `src/core/retrieval.ts` (009)
- `tests/retrieval/*`, `tests/commands.integration.test.ts`

### Forbidden Surfaces
- `qmd-adapter.ts` collection/isolation logic (ISSUE-004 already fixed — do not touch)
- evidence pipeline modules
- `current-task.ts` rendering (owned by ar3)

## Spec Hooks
- I-4: one workspace per query, no cross-workspace escape — slot changes must not weaken isolation.
- Done-When: within-tier BM25 desc; `@tag` workspaces always ≥1 slot; dedup by `workspace:path`; bad `--limit` → 8; keyword match on cleaned query w/ boundaries.

## Locked Decisions
- 008: keep tier as primary sort key; add BM25 `score desc` as the secondary key, original index as final tiebreak.
- 009: `ParsedQuery` gains `tags: string[]` (explicit `@tags`, distinct from keyword matches) so the retriever can reserve a slot floor for them.
- 009: clamp secondary slots with `Math.min(entryLimit, remaining, Math.ceil(remaining / remainingSecondaries))`; dedup merged results by `${workspace}:${path}`.
- 022: validate parsed limit in `ask.ts` (`Number.isInteger(n) && n > 0 ? n : 8`); `rankRetrievalResults` already has a `safeLimit` backstop.
- 023: `matchKeywordWorkspaces` runs on `cleanQuery`, using `\b`-anchored, case-insensitive regex per keyword.

## Assumptions
- `RankedRetrievalResult` carries `score` (it extends `QmdSearchResult`) — confirmed.
- Tests can inject a fake `RetrievalAdapter` / `searchWorkspace`, so `qmd` is not required.

## Canonical Refs
- `src/core/retrieval-ranking.ts:34-52`
- `src/core/retrieval.ts:50-107`
- `src/core/query-parser.ts:10-39`
- `src/commands/ask.ts:37`

## Rejected Options
- Folding recency/importance into ranking now — depends on ISSUE-019 fields that do not exist (deferred).
- Reserving tag slots inside `query-parser` — wrong layer; allocation lives in `retrieval.ts`.

## Deferred Ideas
- Relevance × recency × importance blend (ISSUE-019, scale-gated).

## Escalate If
- adding `ParsedQuery.tags` breaks a consumer that cannot be updated within this phase → plan phase.
- tag-floor allocation cannot guarantee ≥1 slot without starving primary results → brainstorm refine.
