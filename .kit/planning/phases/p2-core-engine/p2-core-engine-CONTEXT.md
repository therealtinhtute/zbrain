# Phase Context: P2 — Core Engine

## Goal
Workspace resolver, YAML config parser, evidence state machine, and asset extractor — all with unit tests. These are the shared core modules that P3 (CLI commands) and P5/P6 (pipelines) depend on.

## Boundaries
- **Allowed surfaces**: src/core/, src/parsers/, tests/
- **Forbidden surfaces**: src/commands/ (P3), assets/ content changes (P1 done), ~/.zwiki/ (runtime)
- **Blast radius**: New files only — no existing logic to break

## Implementation Decisions
- Workspace resolver: pure function, reads config files, returns workspace name or throws
- Evidence state machine: Zod schemas for state validation, pure transition functions
- Asset extractor: reads bundled assets from binary, writes to ~/.zwiki/ — uses Bun's fs API
- YAML parser: thin wrapper around js-yaml with Zod validation
- Markdown parser: thin wrapper around marked for frontmatter extraction

## Key Invariants (from SPEC)
- I-1: Sources immutable after ingest
- I-2: workspace_at_ingest must match active workspace at every transition
- I-3: Apply blocks if P0/P1 questions awaiting_external or deferred
- I-5: checkpoint.json enables resume from any file
- I-6: qmd query must be scoped to active workspace collection

## Assumptions
- Config files may not exist yet (resolver must handle missing files gracefully)
- Evidence state machine is called by slash commands, not directly by CLI

## Expected Proof
- Unit tests: workspace resolution priority chain (4 levels)
- Unit tests: every state transition (valid + invalid)
- Unit tests: invariant violations throw specific errors
- Unit tests: asset extractor creates correct directory structure
- `bun test` all green
