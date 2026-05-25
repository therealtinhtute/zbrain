# Plan: Product Scaffold

Phase: p1-project-scaffold
Status: ready
Wave Count: 4
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Establish a runnable Bun CLI shell in the repo root with bundled asset placeholders and basic test coverage.

## Inputs
- `.kit/planning/SPEC.md`
- `p0-planning-alignment` decisions
- repo root with no existing product scaffold

## Wave 1
### T1 — Initialize Bun project metadata
- type: implementation
- inputs:
  - tech stack section in the spec
- touches:
  - `package.json`
  - `tsconfig.json`
  - `vitest.config.ts`
- avoid:
  - `wiki-template/`
  - `.kit/planning/**`
- steps:
  1. Create root package metadata for Bun, TypeScript, and test scripts.
  2. Configure TypeScript for ESM, strict checks, and root `src/` / `tests/` paths.
  3. Add Vitest configuration compatible with the Bun runtime.
- expected outputs:
  - installable Bun project metadata
  - typecheck and test configuration files
- verification:
  - `bun test --run` starts the test runner successfully
  - `bunx tsc --noEmit` or equivalent typecheck command resolves config
- stop if:
  - Bun is unavailable or package resolution fails before any code exists
- escalate to:
  - user clarification

## Wave 2
### T2 — Create CLI bootstrap
- type: implementation
- inputs:
  - T1 project config
- touches:
  - `src/index.ts`
  - bootstrap modules needed for command registration
- avoid:
  - full command logic
  - runtime filesystem mutation
- steps:
  1. Add the top-level commander program with `setup`, `init`, `workspace create`, and `update`.
  2. Wire each command to a stub handler or thin module boundary.
  3. Ensure the CLI help text reflects the MVP command surface.
- expected outputs:
  - executable CLI entrypoint
  - command names locked for later phases
- verification:
  - `bun run src/index.ts --help`
  - `bun run src/index.ts workspace --help`
- stop if:
  - command naming conflicts with the locked spec
- escalate to:
  - plan phase

## Wave 3
### T3 — Create root asset tree
- type: implementation
- inputs:
  - asset categories named in the spec
  - `wiki-template/` as source material index
- touches:
  - `assets/engine/**`
  - `assets/templates/**`
  - `assets/commands/**`
  - `assets/agents/**`
  - `assets/workspaces/**`
- avoid:
  - copying all of `wiki-template/` unchanged
  - final command or agent content that belongs to later phases
- steps:
  1. Create the runtime asset directory structure that mirrors `~/.zwiki/`.
  2. Add placeholder or starter files for each asset group.
  3. Record a single source-of-truth rule in comments or docs where needed.
- expected outputs:
  - root asset tree present and loadable by the runtime
- verification:
  - inspect `assets/` for required subdirectories and starter files
  - run a smoke test that imports or reads bundled assets
- stop if:
  - asset layout deviates from the runtime paths in the spec
- escalate to:
  - plan phase

## Wave 4
### T4 — Add smoke coverage and build entry
- type: test
- inputs:
  - T2 CLI bootstrap
  - T3 asset tree
- touches:
  - `tests/**`
  - build entry such as `build.ts`
- avoid:
  - deep command behaviors from later phases
- steps:
  1. Add smoke tests for CLI boot and asset/module loading.
  2. Add a Bun build entry or compile script for the binary target.
  3. Document a temporary blocker only if asset embedding cannot be completed yet.
- expected outputs:
  - passing smoke tests
  - compile path established for later hardening
- verification:
  - `bun test --run`
  - `bun build --compile` or documented compile attempt result
- stop if:
  - Bun compile blocks on unresolved asset embedding strategy
- escalate to:
  - brainstorm refine

## Risks / Watch-fors
- overbuilding command logic during scaffold work
- allowing `wiki-template/` and root `assets/` to become duplicate sources of truth
