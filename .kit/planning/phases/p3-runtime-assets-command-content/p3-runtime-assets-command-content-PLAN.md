# Plan: Runtime Assets and Command Content

Phase: p3-runtime-assets-command-content
Status: ready
Wave Count: 4
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Populate root `assets/` with runtime-ready templates, slash commands, subagents, and engine rules aligned to the spec.

## Inputs
- Phase 1 root asset tree
- Phase 2 parsing or validation helpers if available
- `wiki-template/` as migration source material

## Wave 1
### T1 — Build template assets
- type: implementation
- inputs:
  - template requirements from the spec
  - matching `wiki-template/templates/**` files
- touches:
  - `assets/templates/**`
  - `tests/assets/templates*.test.ts`
- avoid:
  - command handlers
  - retrieval runtime code
- steps:
  1. Author workspace, axiom, mental-model, project, evidence-index, source manifest, QA artifact, and apply checkpoint templates.
  2. Keep file names stable and aligned to runtime expectations.
  3. Add parsing tests for YAML, JSON, and frontmatter-bearing markdown where feasible.
- expected outputs:
  - parseable runtime templates under root `assets/templates/`
- verification:
  - `bun test --run tests/assets/templates*.test.ts`
- stop if:
  - a template format is undefined by the spec and cannot be inferred from existing source material
- escalate to:
  - plan phase

## Wave 2
### T2 — Build slash command assets
- type: implementation
- inputs:
  - command behavior from the spec
  - relevant legacy files under `wiki-template/.claude/commands/`
- touches:
  - `assets/commands/**`
  - `tests/assets/commands*.test.ts`
- avoid:
  - obsolete command names
  - code that performs the command actions
- steps:
  1. Write `/ask`, `/learn`, `/reflect`, `/workspace`, and `/reindex` command docs.
  2. Ensure each command references current runtime paths and invariants.
  3. Validate that command docs reference only the locked command names and phases.
- expected outputs:
  - shippable slash command markdown assets
- verification:
  - `bun test --run tests/assets/commands*.test.ts`
- stop if:
  - legacy command content requires broader product-scope changes
- escalate to:
  - brainstorm refine

## Wave 3
### T3 — Build agent and engine assets
- type: implementation
- inputs:
  - `wiki-template/agents/**`
  - slash command contract from T2
- touches:
  - `assets/agents/**`
  - `assets/engine/**`
  - `tests/assets/engine*.test.ts`
- avoid:
  - extra agent roles outside MVP
- steps:
  1. Author `wiki-planner` and `wiki-qmd-selector`.
  2. Build engine files for system prompt, constraints, retrieval rules, evidence rules, and CLAUDE rules text.
  3. Keep workspace isolation and citation requirements explicit in every relevant asset.
- expected outputs:
  - complete engine and subagent asset set for runtime extraction
- verification:
  - `bun test --run tests/assets/engine*.test.ts`
- stop if:
  - selected source material still encodes cross-workspace behavior
- escalate to:
  - plan phase

## Wave 4
### T4 — Seed starter workspaces
- type: implementation
- inputs:
  - workspace list from MVP-1
  - templates from T1
- touches:
  - `assets/workspaces/**`
  - validation tests if added
- avoid:
  - deep domain knowledge content beyond starter scaffolding
- steps:
  1. Create starter workspace directories for programming, finance, health, and philosophy.
  2. Populate each with minimum metadata or seed files needed for setup.
  3. Ensure no seeded content violates workspace isolation assumptions.
- expected outputs:
  - bundled starter workspaces available during `zwiki setup`
- verification:
  - inspect `assets/workspaces/` for the four expected workspace names
- stop if:
  - seeding requires content not supported by the spec
- escalate to:
  - user clarification

## Risks / Watch-fors
- silent drift between asset names and the CLI/runtime expectations
- carrying forward obsolete legacy command flows from `wiki-template`
