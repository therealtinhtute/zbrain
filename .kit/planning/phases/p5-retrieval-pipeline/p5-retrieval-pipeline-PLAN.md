# Plan: Retrieval Pipeline

Phase: p5-retrieval-pipeline
Status: ready
Wave Count: 4
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Make `/ask` retrieve, rank, and materialize context only from the active workspace via qmd BM25.

## Inputs
- Phase 2 core runtime
- Phase 3 slash-command and agent assets
- qmd installed externally

## Wave 1
### T1 — Implement qmd adapter and collection rules
- type: implementation
- inputs:
  - active workspace resolution
  - qmd usage constraints from the spec
- touches:
  - `src/core/qmd-adapter.ts`
  - `tests/retrieval/qmd-adapter*.test.ts`
- avoid:
  - evidence pipeline code
  - unscoped search helpers
- steps:
  1. Define a narrow adapter for indexing and searching a single workspace collection.
  2. Encode collection naming and cache/index path rules under `~/.zwiki/`.
  3. Reject or fail fast on any call that omits the active workspace collection.
- expected outputs:
  - workspace-scoped qmd adapter
- verification:
  - tests for collection scoping and empty-result behavior
- stop if:
  - qmd cannot be invoked or mocked with a stable interface
- escalate to:
  - brainstorm refine

## Wave 2
### T2 — Implement tier classification and ranking
- type: implementation
- inputs:
  - qmd adapter search result shape
  - path-tier rules from the spec
- touches:
  - `src/core/retrieval-ranking.ts`
  - `tests/retrieval/ranking*.test.ts`
- avoid:
  - any model-based reranking
- steps:
  1. Map paths to tiers P0 through P3.
  2. Preserve BM25 order within tier while promoting higher-priority tiers first.
  3. Cover unknown-path and mixed-result cases with focused tests.
- expected outputs:
  - deterministic post-filter and ranking logic
- verification:
  - `bun test --run tests/retrieval/ranking*.test.ts`
- stop if:
  - path taxonomy in assets does not match the tier rules
- escalate to:
  - plan phase

## Wave 3
### T3 — Implement `current-task.md` materialization
- type: implementation
- inputs:
  - ranked retrieval results
  - slash-command expectations from Phase 3
- touches:
  - `src/core/current-task.ts`
  - `tests/retrieval/current-task*.test.ts`
- avoid:
  - final answer generation
- steps:
  1. Define the markdown contract for ranked results, fetched bodies, and knowledge gaps.
  2. Write the current-task artifact to the project-local path expected by Claude Code.
  3. Ensure empty tiers are omitted and unresolved gaps are explicit.
- expected outputs:
  - deterministic context artifact for the answering agent
- verification:
  - tests for markdown shape and gap reporting
- stop if:
  - the project-local artifact path conflicts with init-time integration rules
- escalate to:
  - plan phase

## Wave 4
### T4 — Wire retrieval integration and manual acceptance
- type: test
- inputs:
  - T1-T3 modules
  - seeded workspace fixtures
- touches:
  - retrieval integration tests
  - manual acceptance notes if needed
- avoid:
  - evidence pipeline behaviors
- steps:
  1. Add integration coverage for search within a seeded workspace.
  2. Verify cross-workspace bleed is impossible through adapter use.
  3. Verify knowledge-gap behavior stops instead of inferring missing facts.
- expected outputs:
  - tested retrieval path suitable for `/ask`
- verification:
  - retrieval integration test suite
  - optional manual run against qmd if available
- stop if:
  - test environment cannot represent qmd behavior credibly
- escalate to:
  - user clarification

## Risks / Watch-fors
- accidentally treating qmd search as a multi-workspace index
- coupling retrieval logic to slash-command markdown too tightly
