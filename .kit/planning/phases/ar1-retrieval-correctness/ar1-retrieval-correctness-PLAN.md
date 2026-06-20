# Plan: Retrieval Correctness

Phase: ar1-retrieval-correctness
Status: ready
Wave Count: 3
Execution Owner: cook
Updated At: 2026-06-20

## Goal
Land ISSUE-008, 009, 022, 023 so retrieval ranks by tier→BM25, validates limits, routes explicit `@tags`, and dedups.

## Inputs
- `src/core/retrieval-ranking.ts`, `src/core/retrieval.ts`, `src/core/query-parser.ts`, `src/commands/ask.ts`
- fake `RetrievalAdapter` pattern from existing retrieval tests

## Wave 1
### T1 — BM25 score tiebreak in ranking (ISSUE-008)
- type: implementation
- inputs:
  - `src/core/retrieval-ranking.ts:42-49`
- touches:
  - `src/core/retrieval-ranking.ts`
  - `tests/retrieval/` ranking test
- avoid:
  - changing tier classification or `safeLimit`
- steps:
  1. In the comparator, after the tier diff and before `left._index - right._index`, add: if `left.score !== right.score` return `right.score - left.score`.
  2. Keep `_index` as the final stable tiebreak.
- expected outputs:
  - within a tier, higher BM25 score ranks first; equal scores keep qmd order
- verification:
  - unit test: two same-tier results with differing scores → higher score first; cross-tier order unchanged
- stop if:
  - `score` is absent on some results (it is typed required) → plan phase
- escalate to:
  - plan phase

### T2 — Validate `--limit` in ask (ISSUE-022)
- type: implementation
- inputs:
  - `src/commands/ask.ts:37`
- touches:
  - `src/commands/ask.ts`
  - `tests/commands.integration.test.ts`
- avoid:
  - retrieval slot math (handled in Wave 3)
- steps:
  1. After parsing, set `limit = Number.isInteger(n) && n > 0 ? n : 8` (where `n` is the numeric parse).
  2. Pass the sanitized value into both retrieval calls.
- expected outputs:
  - `--limit abc` and `--limit 0` behave as default 8, not "0 results"
- verification:
  - integration test: `runAsk(query, { limit: "abc" })` returns the default-sized result set
- stop if:
  - none expected
- escalate to:
  - check

## Wave 2
### T3 — Keyword match hardening + expose tags (ISSUE-023 + 009 prerequisite)
- type: implementation
- inputs:
  - `src/core/query-parser.ts:10-39`
- touches:
  - `src/core/query-parser.ts`
  - `tests/retrieval/` query-parser test
- avoid:
  - retrieval allocation logic (Wave 3)
- steps:
  1. Change `matchKeywordWorkspaces` to accept and match against `cleanQuery`, using `new RegExp("\\b" + escaped(kw) + "\\b", "i")` per keyword.
  2. Update `parseQuery` to call it with `cleanQuery` and add `tags: string[]` to `ParsedQuery` (the explicit `@tags`, kept distinct from keyword matches).
  3. Keep `secondaryWorkspaces` as the unique union for backward compatibility.
- expected outputs:
  - `auth` no longer matches "author"; `@tag` text no longer self-triggers keyword matches; `ParsedQuery.tags` available
- verification:
  - unit: substring false-positive gone; boundary match still hits; `tags` populated from `@x` only
- stop if:
  - a consumer of `ParsedQuery` cannot compile with the new field → plan phase
- escalate to:
  - plan phase

## Wave 3
### T4 — Multi-workspace slot floor, clamp, dedup (ISSUE-009)
- type: implementation
- inputs:
  - `ParsedQuery.tags` from T3
  - `src/core/retrieval.ts:70-94`
- touches:
  - `src/core/retrieval.ts`
  - `tests/retrieval/multi-workspace.integration.test.ts`
- avoid:
  - weakening single-workspace isolation (I-4)
- steps:
  1. Reserve ≥1 slot for each resolved secondary that appears in `tags`, even when primary saturated `totalLimit`.
  2. Clamp per-secondary slots: `Math.min(entryLimit, remaining, Math.ceil(remaining / remainingSecondaries))`.
  3. Dedup the merged `allResults` by `${workspace ?? primary}:${path}` before writing context.
- expected outputs:
  - explicit `@tag` workspace returns rows even with a saturated primary; no duplicate path rows
- verification:
  - integration: saturated primary + `@secondary` tag → secondary still represented; duplicate path across workspaces collapses to one
- stop if:
  - guaranteeing a tag floor forces dropping a primary result in a way that violates the spec ordering → brainstorm refine
- escalate to:
  - brainstorm refine

## Risks / Watch-fors
- T4 depends on T3's `ParsedQuery.tags`; do not start T4 until T3 compiles.
- Tag floor must add slots, not silently evict higher-tier primary results.
