# Plan: CLI Commands

Phase: p4-cli-commands
Status: ready
Wave Count: 4
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Make the four public CLI commands work end to end against a temp runtime and project.

## Inputs
- Phase 2 core runtime
- Phase 3 runtime assets
- `.kit/planning/SPEC.md`

## Wave 1
### T1 — Implement `zwiki setup`
- type: implementation
- inputs:
  - asset extraction and qmd-check helpers
- touches:
  - `src/commands/setup.ts`
  - setup integration tests
- avoid:
  - project-local `.claude/` integration
- steps:
  1. Build the clack flow for intro, asset extraction, qmd status, summary note, and outro.
  2. Materialize `~/.zwiki/` with bundled assets and initial config.
  3. Make re-runs idempotent or explicitly guarded.
- expected outputs:
  - functional first-run setup command
- verification:
  - temp-home integration test covering clean install and re-run behavior
- stop if:
  - asset sync would overwrite user workspace content
- escalate to:
  - plan phase

## Wave 2
### T2 — Implement `zwiki workspace create`
- type: implementation
- inputs:
  - template assets
  - config helpers
- touches:
  - `src/commands/workspace.ts`
  - workspace creation integration tests
- avoid:
  - retrieval or evidence pipeline logic
- steps:
  1. Prompt for workspace name with validation and confirmation.
  2. Scaffold the workspace tree and starter files from bundled templates.
  3. Register the workspace in config or runtime metadata if required.
- expected outputs:
  - user-creatable workspaces under `~/.zwiki/workspaces/`
- verification:
  - temp-home integration test for workspace creation and default-workspace behavior
- stop if:
  - runtime layout for workspaces is still unstable
- escalate to:
  - plan phase

## Wave 3
### T3 — Implement `zwiki update`
- type: implementation
- inputs:
  - asset sync logic
- touches:
  - `src/commands/update.ts`
  - update integration tests
- avoid:
  - user workspace data mutation
- steps:
  1. Build the clack flow for refreshing bundled runtime assets.
  2. Preserve user-created workspace content and config.
  3. Report a usable summary of what changed.
- expected outputs:
  - functional update command
- verification:
  - temp-home integration test for updating bundled files without touching user content
- stop if:
  - update semantics cannot distinguish bundled assets from user data
- escalate to:
  - brainstorm refine

## Wave 4
### T4 — Implement `zwiki init`
- type: implementation
- inputs:
  - workspace listing and resolver helpers
  - project integration asset paths
- touches:
  - `src/commands/init.ts`
  - init integration tests
- avoid:
  - rewriting unrelated project files
- steps:
  1. Prompt for workspace selection and inject-target selection with clack.
  2. Write or update `<cwd>/.claude/zwiki.json`, project links, and optional settings files.
  3. Append zwiki rules to `CLAUDE.md` without duplicating existing content.
- expected outputs:
  - project-local integration flow that can be rerun safely
- verification:
  - temp-project integration test with preexisting `CLAUDE.md`
  - symlink or fallback behavior verified by inspection
- stop if:
  - supported platforms cannot share one safe integration strategy
- escalate to:
  - brainstorm refine

## Risks / Watch-fors
- burying clack UX inside low-level modules
- duplicate `CLAUDE.md` injection or destructive rewrites
