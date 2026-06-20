# Plan: Green Baseline

Phase: ar0-green-baseline
Status: ready
Wave Count: 1
Execution Owner: cook
Updated At: 2026-06-20

## Goal
Ship the missing `workspaces/` asset group so `tests/assets.test.ts` passes and the suite is fully green (135 pass / 0 fail).

## Inputs
- `src/core/assets.ts` (preserve-existing logic confirming the group is expected)
- `tests/assets.test.ts` (the failing expectation)
- `bun run generate:assets` task (CLAUDE.md)

## Wave 1
### T1 — Ship the workspaces asset group + regenerate bundle
- type: implementation
- inputs:
  - `src/core/assets.ts:29-30`
  - `tests/assets.test.ts:9-20`
- touches:
  - `assets/workspaces/.gitkeep` (new)
  - `src/generated/bundled-assets.ts` (regenerated)
- avoid:
  - `src/core/` / `src/commands/` logic
  - editing the generator script
  - inventing starter-workspace content
- steps:
  1. Create `assets/workspaces/.gitkeep` (empty tracked file).
  2. Run `bun run generate:assets` to embed it into `src/generated/bundled-assets.ts`.
  3. Confirm `assets/` top-level now lists exactly `README.md, agents, engine, skills, templates, workspaces`.
- expected outputs:
  - `assets/workspaces/` exists and is committed
  - regenerated bundled assets including the `workspaces/.gitkeep` entry
- verification:
  - `bun test tests/assets.test.ts` → pass
  - `bun test` → 135 pass / 0 fail
  - `bun run typecheck` → clean
- stop if:
  - `bun run generate:assets` does not include `.gitkeep` (switch to `assets/workspaces/README.md`)
- escalate to:
  - plan phase

## Risks / Watch-fors
- Committing the `.gitkeep` but forgetting the regenerated `bundled-assets.ts` leaves runtime extraction inconsistent with `assets/`.
