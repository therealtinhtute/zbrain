# Context: CLI Commands

Phase: p4-cli-commands
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: high
Expected Proof: integration

## Goal
Implement the interactive CLI commands that install runtime assets, create workspaces, update bundled content, and integrate the current project.

## Scope Boundary
### Allowed Surfaces
- `src/commands/**`
- `src/index.ts`
- integration helpers needed for project wiring
- integration tests for temp home and temp project flows

### Forbidden Surfaces
- major changes to phase-2 core contracts unless a bug is discovered
- non-MVP command surfaces
- retrieval and evidence business logic beyond command prerequisites

## Spec Hooks
- user-facing commands are `setup`, `update`, `workspace create`, and `init`
- all CLI UX must use `@clack/prompts`
- `init` must be non-destructive and preserve existing `CLAUDE.md`

## Locked Decisions
- command handlers call shared core modules instead of implementing raw filesystem logic inline
- project integration writes `<cwd>/.claude/zwiki.json` as the active workspace pointer
- symlink-first integration may fall back only if platform behavior forces it

## Assumptions
- qmd presence checks can be reported during setup without bundling qmd itself
- integration tests can use temp directories for both home and project roots

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- Phase 2 core runtime modules
- Phase 3 runtime assets

## Rejected Options
- raw console output because the spec mandates clack-style UX
- overwriting project files on init because the spec requires append-only behavior for `CLAUDE.md`

## Deferred Ideas
- richer update diff output
- advanced project integration targets beyond the locked set

## Escalate If
- symlink behavior is not portable enough for supported targets
- qmd prerequisite checks need unsupported platform-specific behavior
