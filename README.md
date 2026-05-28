# zbrain

`zbrain` is a Bun-compiled CLI for building a personal LLM wiki that AI agents can use safely.

## MVP-1

The current MVP is a local-first CLI product with:

- Workspace-isolated knowledge bases for `programming`, `finance`, `health`, and `philosophy`
- A 4-stage evidence pipeline: `ingest -> analyze -> qa -> apply`
- A 3-stage retrieval pipeline for `zbrain:ask` using qmd BM25 search only
- Bundled runtime assets extracted into `~/.zbrain/`
- Project integration through `zbrain init` and `<cwd>/.claude/zbrain.json`

Out of scope for MVP-1:

- Web UI
- Vector search or hybrid retrieval
- External sync integrations
- Cross-workspace retrieval
- npm package distribution

## Command Surface

Public CLI commands:

- `zbrain setup`
- `zbrain init`
- `zbrain workspace create <name>`
- `zbrain update`

Bundled Claude Code slash commands:

- `zbrain:ask`
- `zbrain:learn`
- `zbrain:reflect`
- `zbrain:workspace`
- `zbrain:reindex`

Command asset format:

- files are stored as flat markdown under `assets/commands/*.md`
- invocation identity is defined by frontmatter `name: zbrain:*`

## Install

Development setup:

```bash
bun install
bun run build
```

qmd prerequisite:

```bash
npm i -g @tobilu/qmd
```

Binary smoke:

```bash
./dist/zbrain --help
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
```

## Runtime Layout

After `zbrain setup`, the runtime lives in `~/.zbrain/`:

- `config.yml`
- `engine/`
- `templates/`
- `commands/`
- `agents/`
- `workspaces/`

Project-local integration lives in the current repo under `.claude/`:

- `.claude/zbrain.json`
- optional symlinked `commands/` and `agents/`
- non-destructive `CLAUDE.md` rules injection

## CLI Usage

First run:

```bash
zbrain setup
```

Create or use a workspace:

```bash
zbrain workspace create programming
zbrain init
```

The learning and retrieval cores are implemented in the runtime:

- learn flow: ingest -> analyze -> qa -> apply
- ask flow: workspace-scoped qmd retrieval -> `current-task.md`

Acceptance proof for the full path is in [docs/acceptance-walkthrough.md](/home/tinhpt/Lab/zbrain/docs/acceptance-walkthrough.md).

## Repository Layout

The product implementation for MVP-1 lives in the repository root.

- `.kit/planning/` contains the locked spec, roadmap, and per-phase execution plans
- root `src/`, `assets/`, and Bun project files are the target implementation surfaces

## Prerequisites

- Bun for development, testing, and binary compilation
- `qmd` installed separately for BM25 indexing and search

Example prerequisite install:

```bash
npm i -g @tobilu/qmd
```

## Release

- release guidance: [docs/release.md](/home/tinhpt/Lab/zbrain/docs/release.md)
- acceptance walkthrough: [docs/acceptance-walkthrough.md](/home/tinhpt/Lab/zbrain/docs/acceptance-walkthrough.md)

## Known Limits

- qmd is not bundled into the binary
- slash commands are shipped as assets for project integration, not as standalone CLI subcommands
- retrieval tests use a stubbed adapter for deterministic proof when qmd is not installed locally

## Status

Planning is locked in `.kit/planning/SPEC.md` and phased execution starts from `.kit/workflow-state.yml`.
