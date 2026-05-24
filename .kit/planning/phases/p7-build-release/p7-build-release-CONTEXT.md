# Phase Context: P7 — Build + Release

## Goal
Binary compiles with embedded assets, zwiki setup extracts correctly, README complete, first workspace indexed and searchable.

## Boundaries
- **Allowed surfaces**: build.ts, README.md, .github/ (if CI), package.json scripts
- **Forbidden surfaces**: src/ logic changes (frozen), assets/ content changes (frozen)
- **Blast radius**: Build output, documentation, GitHub repo metadata

## Implementation Decisions
- Bun compile with `--compile` flag for single binary (D5)
- Assets embedded via Bun's file import or string inlining in build.ts
- Binary targets: macOS ARM64 (primary), macOS x64, Linux x64
- GitHub Releases for distribution (manual first, CI later)
- README in Vietnamese (project language preference)

## Assumptions
- Bun compile works with embedded string assets
- Binary size ~50-80MB is acceptable
- GitHub repo exists at some URL (can be created during this phase)

## Expected Proof
- `bun run build` produces ./zwiki binary
- `./zwiki setup` creates correct ~/.zwiki/ structure
- `./zwiki init` in test project works end-to-end
- `./zwiki --version` shows correct version
- README covers: install, setup, workflow, command reference
- Example workspace has ≥1 axiom, ≥1 mental-model, ≥1 project entry
