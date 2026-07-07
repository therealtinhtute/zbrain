# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Prerequisites

`qmd` is required for the retrieval pipeline (`zbrain ask`). Install once:
```bash
npm i -g @tobilu/qmd
```

## Commands

```bash
bun run typecheck                 # TypeScript type check (no emit)
bun run build                     # generate assets + compile binary → dist/zbrain
bun run generate:assets           # regenerate src/generated/bundled-assets.ts only
```

The binary is a single Bun-compiled executable. Run it directly during development:
```bash
bun run src/index.ts setup
bun run src/index.ts init
bun run src/index.ts workspace create research
bun run src/index.ts learn --workspace research --label note
bun run src/index.ts ingest list
bun run src/index.ts ask "question"
```

Binary smoke test (isolated from `~/.zbrain`):
```bash
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
```

`ZBRAIN_HOME` overrides the default `~/.zbrain` runtime directory everywhere — useful for test isolation and smoke runs.

## Architecture

### Three Layers

```
dist/zbrain (binary)          ← bun build --compile
  ↓ zbrain setup extracts
~/.zbrain/ (runtime)          ← engine, skills, agents, templates, workspaces
  ↓ zbrain init injects
<project>/.claude/ (project)  ← optional Claude-specific skills/agents/settings
```

**CLI layer** (`src/commands/`) handles interactive setup and project integration. All CLI commands use `@clack/prompts` for UX (intro/outro, spinners, select, notes) — no raw `console.log`. Running `zbrain` with no arguments and a TTY launches an interactive menu (`src/commands/interactive.ts`) rather than the Commander program.

**Core layer** (`src/core/`) is pure logic: no I/O side effects in functions that don't explicitly take a `paths: RuntimePaths` parameter. All filesystem operations flow through `RuntimePaths` so tests can redirect to temp directories.

**Asset layer** (`assets/`) is the source of truth for all runtime content: skills, agents, engine rules, templates, starter workspaces. The script `scripts/generate-bundled-assets.mjs` walks `assets/` and embeds everything as a TypeScript literal into `src/generated/bundled-assets.ts`. **Regenerate after any change to `assets/`** — the generated file is committed.

Asset subdirectories:
- `assets/engine/` — core engine rules injected into Claude's context
- `assets/skills/` — bundled Claude Code skills (e.g. `zbrain:learn`, `zbrain:ingest`, `zbrain:ask`)
- `assets/agents/` — agent definitions
- `assets/templates/` — workspace scaffolding templates
- `assets/commands/` — flat markdown files for runtime command skill definitions (frontmatter `name: zbrain:*`)
- `assets/workspaces/` — starter workspace seed files

### Workspace and Project Pointer

Active workspace resolution order (highest priority first):
1. SQLite `projects` table entry matching the current project root → `workspace` (read via `zbrain workspace current --json`; `~/.zbrain/projects.json` is gone as of AC-P1-9 — a one-time migration on first `initDb` call imports any legacy file into SQLite and renames it to `.bak`)
2. `~/.zbrain/config.yml` → `default_workspace`
3. Auto-detect if exactly one workspace exists
4. Error

Project registry entries also accept an optional `secondary_workspaces` array for cross-workspace context loading — see `src/schemas/config.ts` for the full Zod schema.

### Retrieval Pipeline

`zbrain:ask` uses a two-function retrieval layer:

- `retrieveWorkspaceContext()` — single workspace; used by skills directly.
- `retrieveMultiWorkspaceContext()` — primary workspace + optional secondary workspaces resolved from `zbrain.json`. Secondaries are triggered by keyword config or `@workspace` tags in the query. Primary results fill slots first; secondaries share remainder.

Both functions write a ranked context to the registered project's `context_file` under `~/.zbrain/projects/`. Runtime adapters then read that file to answer the user's question.

Retrieval ranking is tier-first: `axioms/` (P0) > `mental-models/` (P1) > `projects/` (P2) > `decisions/` (P3), then by BM25 score within tier.

### Evidence Pipeline

State machine: `ingested → reviewed → applied → archived`

Invariants enforced in code:
- `zbrain learn` creates `sources/{id}/raw.md` and `source.yaml`; they are immutable afterward (SHA-256 checked on every access).
- `source.yaml#workspace_at_ingest` must match the active workspace at every state transition.
- Apply stage (`reviewed → applied`) blocks if any P0/P1 question is `awaiting_external` or `deferred`.

Evidence files live in `~/.zbrain/workspaces/{workspace}/evidence/`. The `evidence-store.ts` module handles all path construction, ID generation, and index updates. State validation is in `evidence-state.ts`.

### Key Schema Types

Defined in `src/schemas/config.ts` with Zod:
- `ProjectBinding` — central project registration shape (`project_root`, `workspace`, `context_file`, optional `secondary_workspaces`, `runtimes`)
- `SecondaryWorkspaceEntry` — one secondary workspace entry (`workspace`, `keywords`, optional `limit`)
- `GlobalConfig` — `~/.zbrain/config.yml` shape

Both schemas use `.passthrough()` — unknown fields survive parsing rather than throwing.

## zbrain Integration

zbrain is a workspace-isolated knowledge retrieval layer. Skills live in `.claude/skills/zbrain-*`.
Runtime root: `~/.zbrain/`. Project registry: SQLite (`~/.zbrain/zbrain.db`) — read it via `zbrain workspace current`.

### Workspace Resolution

1. Run `zbrain workspace current` (JSON output) — gives `workspace` and `context_file` for the current project root.
2. Fallback: `~/.zbrain/config.yml` → `default_workspace`.
3. If neither resolves, stop and report — never guess a workspace.

### Skill Triggers

| When you need to… | Use |
|--------------------|-----|
| Answer domain questions (architecture, decisions, patterns) | `zbrain:ask` |
| Record a file, URL content, pasted text, or observation | `zbrain:learn` |
| List, analyze, QA, or apply evidence | `zbrain:ingest` |
| Write trusted, already-verified knowledge directly (no external source to gate) | `zbrain note add` |

**Before answering any question about domain knowledge, project decisions, or architectural patterns — invoke `zbrain:ask` first. Never answer from memory.**

### Retrieval Tier Priority

`axioms/` (P0) → `mental-models/` (P1) → `projects/` (P2) → `decisions/` (P3)

Higher-tier results rank first regardless of BM25 score.

### Evidence Pipeline

Each piece of external material moves through three public verbs:

```
learn → ingest → ask
```

Use `zbrain:ingest list` to see which stage each item is in and what command runs next.
**Never advance to apply if any P0 or P1 question is unresolved.**

**Fast path (`zbrain note add`):** for knowledge that is already trusted and first-party
(no external source to gate — e.g. a decision made in this conversation, a verified fact),
write directly to the wiki instead of going through `learn` → `ingest`. Still conflict-checked
and still governed by the same lifecycle (supersede, not overwrite). Reserve `learn`/`ingest`
for material from outside the conversation that needs a human review step.

### Secondary Workspaces (optional)

Each project registry entry supports a `secondary_workspaces` array for cross-workspace context.
Each entry has `workspace`, `keywords`, and optional `limit`.
Secondary results fill remaining slots after primary results.

### Invariants

- **Cite all retrieved context.** Never answer domain questions from memory.
- **One workspace per query.** Never cross workspace boundaries in a single retrieval.
- **Evidence is immutable after ingest.** Never edit `raw.md` or `source.yaml`.
- **Apply gate.** Block apply if any P0 or P1 QA question is `awaiting_external`.
