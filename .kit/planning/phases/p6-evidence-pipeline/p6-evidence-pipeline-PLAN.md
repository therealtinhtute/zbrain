# Plan: Evidence Pipeline

Phase: p6-evidence-pipeline
Status: ready
Wave Count: 4
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Make `/learn` function across ingest, analyze, qa, and apply with tested invariants and resume behavior.

## Inputs
- Phase 2 evidence state and filesystem helpers
- Phase 3 evidence templates and command assets
- Phase 5 reindex hook expectations

## Wave 1
### T1 — Implement ingest and evidence registration
- type: implementation
- inputs:
  - evidence templates
  - workspace resolver
- touches:
  - `src/core/evidence-ingest.ts`
  - evidence ingest tests
- avoid:
  - apply-time wiki mutation
- steps:
  1. Create stable evidence IDs and workspace-scoped directories.
  2. Persist `raw.md`, `source.yaml`, and index state without later mutation paths.
  3. Test workspace-lock metadata and immutable-source guarantees from the start.
- expected outputs:
  - functioning ingest stage
- verification:
  - ingest integration tests using temp workspaces
- stop if:
  - source layout conflicts with the template or invariant rules
- escalate to:
  - plan phase

## Wave 2
### T2 — Implement analyze and QA orchestration
- type: implementation
- inputs:
  - ingest outputs
  - analysis and QA templates
- touches:
  - `src/core/evidence-analyze.ts`
  - `src/core/evidence-qa.ts`
  - evidence analyze/qa tests
- avoid:
  - final apply mutations
- steps:
  1. Generate the required analysis artifacts in deterministic locations.
  2. Represent question severities, statuses, and human answers in append-safe files.
  3. Generate `verified-facts.md` only from QA-complete material with citations.
- expected outputs:
  - functioning analyze and QA stages
- verification:
  - integration tests for artifact creation and QA status handling
- stop if:
  - citation requirements cannot be represented in the chosen file shapes
- escalate to:
  - brainstorm refine

## Wave 3
### T3 — Implement apply and checkpoint resume
- type: implementation
- inputs:
  - verified facts
  - checkpoint and manifest templates
- touches:
  - `src/core/evidence-apply.ts`
  - apply tests
- avoid:
  - changing immutable source files
- steps:
  1. Translate verified facts into workspace-file updates under the active workspace only.
  2. Write checkpoint and manifest data that can resume after interruption.
  3. Enforce QA-gate and workspace-lock checks immediately before mutation.
- expected outputs:
  - resumable apply stage with audit trail
- verification:
  - tests for interrupted apply, invalid workspace, and blocked QA states
- stop if:
  - apply requires partial writes that cannot be resumed safely
- escalate to:
  - brainstorm refine

## Wave 4
### T4 — Trigger reindex and end-to-end evidence validation
- type: test
- inputs:
  - T1-T3 pipeline modules
  - reindex hook from the retrieval phase
- touches:
  - end-to-end evidence tests
- avoid:
  - non-MVP source integrations
- steps:
  1. Trigger reindex only after successful apply.
  2. Run an end-to-end temp-workspace flow across all four stages.
  3. Verify invalid transitions are rejected with explicit errors.
- expected outputs:
  - tested evidence pipeline suitable for `/learn`
- verification:
  - evidence end-to-end test suite
- stop if:
  - the reindex dependency cannot be isolated cleanly for testability
- escalate to:
  - plan phase

## Risks / Watch-fors
- allowing apply-time writes before QA gating completes
- weak checkpoint semantics that do not actually protect resume behavior
