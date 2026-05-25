# Plan: Planning Alignment

Phase: p0-planning-alignment
Status: ready
Wave Count: 2
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Make planning artifacts and repo-level direction consistent before implementation begins.

## Inputs
- `.kit/planning/SPEC.md`
- existing `.kit/planning/ROADMAP.md`
- existing `.kit/workflow-state.yml`
- root README and `wiki-template/README.md`

## Wave 1
### T1 — Normalize roadmap and workflow pointers
- type: implementation
- inputs:
  - `.kit/planning/SPEC.md`
  - existing roadmap and workflow index
- touches:
  - `.kit/planning/ROADMAP.md`
  - `.kit/workflow-state.yml`
- avoid:
  - `src/`
  - `assets/`
  - `wiki-template/`
- steps:
  1. Rewrite the roadmap to include `P0` through `P7` in dependency order.
  2. Set `p0-planning-alignment` as both `entry_phase` and `current_phase`.
  3. Point `active_context` and `active_plan` at the new phase files.
- expected outputs:
  - a root-aligned roadmap
  - a pointer-only workflow index with valid paths
- verification:
  - inspect `.kit/planning/ROADMAP.md` for the eight-phase sequence
  - inspect `.kit/workflow-state.yml` for `p0-planning-alignment` pointers
- stop if:
  - the spec no longer supports root implementation
- escalate to:
  - brainstorm refine

## Wave 2
### T2 — Refresh all phase artifacts for root-product execution
- type: implementation
- inputs:
  - roadmap written in T1
  - `.kit/planning/SPEC.md`
- touches:
  - `.kit/planning/phases/**`
- avoid:
  - runtime code
  - non-planning docs unless they are required to remove repo-shape ambiguity
- steps:
  1. Add missing `p0-planning-alignment` context and plan files.
  2. Rewrite `P1-P7` context and plan files so they describe root `src/`, `assets/`, and runtime paths instead of `~/Lab/zwiki/` or `wiki-template/` implementation.
  3. Make each phase plan wave-based with explicit verification and escalation paths.
- expected outputs:
  - complete phase folders for every roadmap phase
  - executable plans that stay inside the spec boundaries
- verification:
  - confirm every phase directory has both `-CONTEXT.md` and `-PLAN.md`
  - search `.kit/planning/phases` for obsolete `~/Lab/zwiki/` implementation instructions
- stop if:
  - a phase requires scope that is explicitly out of spec
- escalate to:
  - plan phase

## Risks / Watch-fors
- stale path references hidden in older phase files
- phase tasks that mix planning work with implementation work
