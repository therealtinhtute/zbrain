# Phase Context: P1 — Project Scaffold

## Goal
Bun project initialized, all dependencies installed, asset directory structure created, build compiles empty binary.

## Boundaries
- **Allowed surfaces**: src/, assets/, package.json, tsconfig.json, build.ts, tests/
- **Forbidden surfaces**: ~/.zwiki/ (runtime — not created yet), .claude/ (project integration — P3)
- **Blast radius**: New project only — no existing files to break

## Implementation Decisions
- Bun as package manager AND runtime (not pnpm — SPEC changed to Bun)
- Commander.js for CLI framework with subcommands (setup, init, workspace, update)
- Assets directory structure mirrors ~/.zwiki/ layout exactly
- build.ts uses Bun.build with embedded assets via import

## Assumptions
- Bun 1.1+ installed on dev machine
- No existing package.json (fresh project)

## Rejected Options
- pnpm as package manager (Bun handles both runtime + packages)
- Single flat CLI entry (commander subcommands cleaner for 4+ commands)

## Expected Proof
- `bun run build` produces a binary at `./zwiki`
- `./zwiki --help` shows commander help with subcommands
- `bun test` passes (even if tests are stubs)
- `assets/` contains all .md and .yaml template files

## Escalation
- If Bun compile fails with asset embedding → escalate to /brainstorm for alternative build approach
