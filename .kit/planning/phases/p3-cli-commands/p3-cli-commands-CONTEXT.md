# Phase Context: P3 — CLI Commands

## Goal
`zwiki setup`, `zwiki init`, `zwiki workspace create`, `zwiki update` — all with clack UX, fully functional.

## Boundaries
- **Allowed surfaces**: src/commands/, tests/commands/, src/index.ts (wire up real handlers)
- **Forbidden surfaces**: assets/ content (P1), src/core/ logic changes (P2 done)
- **Blast radius**: New command files + update index.ts to replace stubs

## Implementation Decisions
- ALL commands use @clack/prompts exclusively (D9 — no console.log)
- `zwiki init` creates symlinks (D6 — not copies) for commands/ and agents/
- `zwiki init` appends to CLAUDE.md (D8 — non-destructive, checks for existing zwiki section)
- `zwiki workspace create` only creates workspace structure, doesn't create qmd collection (that's manual `qmd index`)
- `zwiki setup` auto-runs on first use if ~/.zwiki/ doesn't exist

## UX Flows (from SPEC)
- setup: intro → spinner (extracting) → spinner (checking qmd) → note (summary) → outro
- init: intro → select (workspace) → multiselect (inject targets) → spinner (symlinks) → note (files) → outro
- workspace create: intro → text (name) → confirm → spinner (scaffolding) → outro
- update: intro → spinner (extracting) → note (changed) → outro

## Assumptions
- P2 core modules (workspace-resolver, asset-extractor) are tested and working
- ~/.zwiki/ may or may not exist when commands run
- Project .claude/ directory may or may not exist

## Expected Proof
- `bun src/index.ts setup` in a clean environment creates ~/.zwiki/ with correct structure
- `bun src/index.ts init` in a test project creates symlinks and appends CLAUDE.md
- `bun src/index.ts workspace create test-ws` creates workspace directory from template
- All commands show clack-styled output (not raw console.log)
- Symlinks resolve correctly on macOS
