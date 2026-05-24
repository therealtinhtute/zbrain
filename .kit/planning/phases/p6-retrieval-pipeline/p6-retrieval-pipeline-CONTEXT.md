# Phase Context: P6 — Retrieval Pipeline

## Goal
/ask end-to-end: parse intent → qmd BM25 search → priority post-filter → answer with citations. This is the primary user-facing feature.

## Boundaries
- **Allowed surfaces**: src/core/qmd-retrieval.ts, src/core/current-task-writer.ts, tests/, assets/agents/ (refinements)
- **Forbidden surfaces**: src/commands/ (CLI — P3), evidence pipeline files (P5)
- **Blast radius**: qmd configuration, current-task.md format

## Implementation Decisions
- qmd accessed via MCP tools (search, get, multi_get, status) — not CLI
- Post-filter is path-prefix based (D4): axioms/→P0, mental-models/→P1, projects/→P2, decisions/→P3
- Within each tier, original BM25 score ordering preserved
- Top 6-8 results written to current-task.md
- Axioms get full body in context, others truncated to ~400 tokens
- current-task.md is the session bridge between Stage 2 (selector) and Stage 3 (builder)

## Key Requirements
- R4: BM25 retrieval pipeline (3 stages)
- R10: /ask triggers the pipeline
- R20: qmd BM25 only
- I-6: Workspace-scoped search (MUST specify collection parameter)

## Assumptions
- qmd is installed and indexed (zwiki setup + qmd index already run)
- qmd MCP server is configured in .claude/settings.local.json
- Active workspace has at least some content to search

## Expected Proof
- `/ask "what is SOLID?"` in programming workspace → returns axioms/mental-models about SOLID, ranked correctly
- `/ask "SOLID"` in finance workspace → returns no results or unrelated results (isolation proof)
- Priority ordering: if axiom and project both match "SOLID", axiom appears first
- Knowledge gap: `/ask "quantum computing"` in programming workspace (no entries) → reports gap, doesn't guess
