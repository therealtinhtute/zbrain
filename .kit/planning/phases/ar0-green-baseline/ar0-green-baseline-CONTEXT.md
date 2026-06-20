# Context: Green Baseline

Phase: ar0-green-baseline
Status: ready
Spec Link: ../../SPEC-audit-remediation.md
Roadmap Link: ../../ROADMAP-audit-remediation.md
Blast Radius: low
Expected Proof: unit

## Goal
Make `bun test` fully green by shipping the `workspaces/` asset group the runtime already expects, fixing the pre-existing `assets.test.ts` failure.

## Scope Boundary
### Allowed Surfaces
- `assets/workspaces/` (new)
- `src/generated/bundled-assets.ts` (regenerated, never hand-edited)
- `tests/assets.test.ts` (only if needed — preferred outcome is no test change)

### Forbidden Surfaces
- any `src/core/` or `src/commands/` logic
- the asset generator script `scripts/generate-bundled-assets.mjs`
- any other `assets/` group

## Spec Hooks
- Done-When: `bun test` green including the formerly-failing `assets.test.ts`.
- Reversed decision #1: ship the asset group, do not gut the test.

## Locked Decisions
- Create `assets/workspaces/.gitkeep` (single tracked file) so the directory exists, persists in git, and bundles. No starter-workspace name/content is invented.
- Regenerate `src/generated/bundled-assets.ts` via `bun run generate:assets` and commit it.

## Assumptions
- `bun run generate:assets` walks `assets/` and embeds every file (incl. `.gitkeep`).
- `assets.ts:30` `shouldPreserveExisting("workspaces/...")` continues to skip overwrite on resetup — a `.gitkeep` under it is harmless.

## Canonical Refs
- `.kit/planning/SPEC-audit-remediation.md`
- `src/core/assets.ts:29-30`
- `tests/assets.test.ts:9-20`

## Rejected Options
- Remove `workspaces` from the test's expected dirs — contradicts the runtime's own preserve-existing logic; leaves the documented asset group unimplemented.
- Ship a full starter workspace (`workspace.md`, tier dirs) — injects unrequested runtime behavior and requires naming a starter workspace; deferred.

## Deferred Ideas
- A real seeded starter workspace shipped on `zbrain setup` (additive, future).

## Escalate If
- the asset generator does not pick up `.gitkeep` (then ship a minimal `assets/workspaces/README.md` instead) → plan phase.
