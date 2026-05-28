# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## zbrain Integration

Use the active workspace only. Cite the retrieved evidence. Do not infer facts from another workspace.

Project-local pointer: `<cwd>/.claude/zbrain.json`
Runtime root: `~/.zbrain/`

---

## Commands

```bash
bun test                          # run full test suite
bun test tests/core/config.test.ts  # run a single test file
bun test --watch                  # watch mode
bun run typecheck                 # TypeScript type check (no emit)
bun run build                     # generate assets + compile binary → dist/zbrain
bun run generate:assets           # regenerate src/generated/bundled-assets.ts only
```

Run a single named test with `--test-name-pattern`:
```bash
bun test tests/retrieval/ --test-name-pattern "keyword trigger"
```

The binary is a single Bun-compiled executable. Run it directly during development:
```bash
bun run src/index.ts setup
bun run src/index.ts init
bun run src/index.ts workspace create research
```

## Architecture

### Three Layers

```
dist/zbrain (binary)          ← bun build --compile
  ↓ zbrain setup extracts
~/.zbrain/ (runtime)          ← engine, skills, agents, templates, workspaces
  ↓ zbrain init injects
<project>/.claude/ (project)  ← zbrain.json pointer, symlinked skills/agents
```

**CLI layer** (`src/commands/`) handles interactive setup and project integration. All CLI commands use `@clack/prompts` for UX (intro/outro, spinners, select, notes) — no raw `console.log`.

**Core layer** (`src/core/`) is pure logic: no I/O side effects in functions that don't explicitly take a `paths: RuntimePaths` parameter. All filesystem operations flow through `RuntimePaths` so tests can redirect to temp directories.

**Asset layer** (`assets/`) is the source of truth for all runtime content: skills, agents, engine rules, templates, starter workspaces. The script `scripts/generate-bundled-assets.mjs` walks `assets/` and embeds everything as a TypeScript literal into `src/generated/bundled-assets.ts`. **Regenerate after any change to `assets/`** — the generated file is committed.

### Workspace and Project Pointer

Active workspace resolution order (highest priority first):
1. `<cwd>/.claude/zbrain.json` → `workspace` field
2. `~/.zbrain/config.yml` → `default_workspace`
3. Auto-detect if exactly one workspace exists
4. Error

`zbrain.json` also accepts an optional `secondary_workspaces` array for cross-workspace context loading — see `src/schemas/config.ts` for the full Zod schema.

### Retrieval Pipeline

`zbrain:ask` uses a two-function retrieval layer:

- `retrieveWorkspaceContext()` — single workspace; used by skills directly.
- `retrieveMultiWorkspaceContext()` — primary workspace + optional secondary workspaces resolved from `zbrain.json`. Secondaries are triggered by keyword config or `@workspace` tags in the query. Primary results fill slots first; secondaries share remainder.

Both functions write a ranked context to `.claude/context/current-task.md`. The Claude Code skill then reads that file to answer the user's question.

Retrieval ranking is tier-first: `axioms/` (P0) > `mental-models/` (P1) > `projects/` (P2) > `decisions/` (P3), then by BM25 score within tier.

### Evidence Pipeline

State machine: `ingested → analyzed → qa_in_progress → qa_done → applied → archived`

Invariants enforced in code:
- `sources/{id}/raw.md` and `source.yaml` are immutable after ingest (SHA-256 checked on every access).
- `source.yaml#workspace_at_ingest` must match the active workspace at every state transition.
- Apply stage (`qa_done → applied`) blocks if any P0/P1 question is `awaiting_external`.

Evidence files live in `~/.zbrain/workspaces/{workspace}/evidence/`. The `evidence-store.ts` module handles all path construction, ID generation, and index updates. State validation is in `evidence-state.ts`.

### Testing Patterns

- **Framework**: `bun:test` (`describe`/`test`/`expect`). No `vitest` at runtime despite it being listed in devDeps.
- **Filesystem tests**: use `mkdtempSync` + `rmSync` in `finally` blocks — never mock the filesystem.
- **Retrieval adapter injection**: `retrieveWorkspaceContext` and `retrieveMultiWorkspaceContext` accept an optional `RetrievalAdapter` argument, so tests inject a fake `searchWorkspace()` without needing `qmd` installed.
- **Test layout mirrors `src/`**: `tests/core/` ↔ `src/core/`, `tests/retrieval/` ↔ retrieval modules.

### Key Schema Types

Defined in `src/schemas/config.ts` with Zod:
- `ProjectPointer` — `.claude/zbrain.json` shape (`workspace`, optional `secondary_workspaces`)
- `SecondaryWorkspaceEntry` — one secondary workspace entry (`workspace`, `keywords`, optional `limit`)
- `GlobalConfig` — `~/.zbrain/config.yml` shape

Both schemas use `.passthrough()` — unknown fields survive parsing rather than throwing.
