# Plan: Core Runtime

Phase: p2-core-engine
Status: ready
Wave Count: 4
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Deliver deterministic runtime primitives for config, workspace resolution, filesystem safety, asset sync, and evidence invariants.

## Inputs
- Phase 1 scaffold
- `.kit/planning/SPEC.md`

## Wave 1
### T1 — Implement config and schema loading
- type: implementation
- inputs:
  - config path and fields from the spec
- touches:
  - `src/core/config.ts`
  - `src/schemas/**`
  - `tests/core/config*.test.ts`
- avoid:
  - command handlers
  - asset content authoring
- steps:
  1. Define schemas for global config and project pointer files.
  2. Implement config read/parse helpers for YAML and JSON inputs.
  3. Cover missing-file, invalid-file, and default-value behavior with unit tests.
- expected outputs:
  - validated config loader modules
- verification:
  - `bun test --run tests/core/config*.test.ts`
- stop if:
  - the spec lacks enough detail for a stable config schema
- escalate to:
  - plan phase

## Wave 2
### T2 — Implement workspace resolution and filesystem helpers
- type: implementation
- inputs:
  - T1 config loader
  - resolution precedence from requirement 18
- touches:
  - `src/core/workspace-resolver.ts`
  - `src/core/fs.ts`
  - `tests/core/workspace-resolver*.test.ts`
- avoid:
  - command UX
  - qmd adapter code
- steps:
  1. Implement resolver precedence: project pointer, global config, single-workspace autodetect, then actionable stop.
  2. Add helpers for safe mkdir, guarded writes, symlink handling, and normalized paths.
  3. Test deterministic resolution paths and error cases with temp directories.
- expected outputs:
  - reusable workspace resolver
  - portable filesystem helper layer
- verification:
  - `bun test --run tests/core/workspace-resolver*.test.ts`
- stop if:
  - path behavior differs materially across supported platforms
- escalate to:
  - brainstorm refine

## Wave 3
### T3 — Implement asset extraction and runtime sync
- type: implementation
- inputs:
  - Phase 1 root `assets/`
  - T2 filesystem helpers
- touches:
  - `src/core/assets.ts`
  - `tests/core/assets*.test.ts`
- avoid:
  - user-facing CLI flow
  - workspace content mutation beyond extraction targets
- steps:
  1. Read bundled assets from the root asset tree or embedded manifest.
  2. Implement extraction into `~/.zwiki/{engine,templates,commands,agents,workspaces}`.
  3. Preserve user workspace content while allowing versioned refresh of bundled runtime assets.
- expected outputs:
  - asset extraction and update logic suitable for `setup` and `update`
- verification:
  - `bun test --run tests/core/assets*.test.ts`
- stop if:
  - asset refresh semantics would overwrite user-authored workspace data
- escalate to:
  - plan phase

## Wave 4
### T4 — Implement evidence state model and invariant guards
- type: implementation
- inputs:
  - invariants I-1 through I-5 from the spec
  - T1 schema helpers
- touches:
  - `src/core/evidence-state.ts`
  - `tests/core/evidence-state*.test.ts`
- avoid:
  - slash-command content
  - retrieval-specific code
- steps:
  1. Define the evidence states and allowed transitions.
  2. Enforce immutable-source, workspace-lock, QA-gate, citation, and resumable-apply rules in central guards.
  3. Add unit tests for valid transitions, invalid transitions, and actionable error messages.
- expected outputs:
  - tested evidence state model ready for the learning pipeline
- verification:
  - `bun test --run tests/core/evidence-state*.test.ts`
- stop if:
  - transition rules require product-scope changes
- escalate to:
  - brainstorm refine

## Risks / Watch-fors
- spreading invariant checks into multiple modules
- treating asset refresh and user workspace preservation as the same problem
