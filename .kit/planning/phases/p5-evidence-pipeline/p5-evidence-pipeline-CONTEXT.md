# Phase Context: P5 — Evidence Pipeline

## Goal
/learn full cycle works: ingest → analyze → qa → apply, with all invariants enforced. This is the most invariant-heavy phase.

## Boundaries
- **Allowed surfaces**: tests/ (integration tests), assets/commands/learn.md (refinements), assets/templates/evidence-*
- **Forbidden surfaces**: src/core/evidence-state-machine.ts (done in P2), src/commands/ (CLI — P3)
- **Blast radius**: Evidence directory structure within workspaces, _index.md state

## Implementation Decisions
- Evidence pipeline runs entirely within Claude Code slash commands (no TypeScript runtime needed)
- State machine validation is described in /learn command instructions (agent follows rules)
- Analysis prompts (01-summary, 02-contradiction, 04-questions, 08-gaps) are defined inline in learn.md
- QA batching follows priority order: P0 first, P1, P2, P3

## Key Invariants
- **I-1**: sources/{id}/raw.md and source.yaml are IMMUTABLE after ingest. Command must explicitly state "DO NOT MODIFY these files"
- **I-2**: Every transition validates workspace_at_ingest == active workspace
- **I-3**: /learn --apply refuses if P0/P1 questions are awaiting_external
- **I-4**: verified-facts.md entries must cite question ID + wiki path
- **I-5**: checkpoint.json tracks per-file progress for --apply resume

## Assumptions
- Agent (Claude Code) follows /learn instructions faithfully
- Human answers QA questions when prompted
- Workspace has at least some existing entries for contradiction analysis

## Expected Proof
- Manual test: ingest a text file → source.yaml + raw.md created correctly
- Manual test: analyze → 4 analysis files created with meaningful content
- Manual test: qa → questions presented, answers recorded, verified-facts generated
- Manual test: apply → wiki entries updated, manifest written, qmd reindex triggered
- Manual test: attempt apply with pending P0 question → blocked
- Manual test: attempt ingest in wrong workspace → blocked
