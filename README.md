# zbrain

`zbrain` is a Bun-compiled CLI for building a personal LLM wiki that AI agents can use safely.

## MVP-1

The current MVP is a local-first CLI product with:

- Workspace-isolated knowledge bases for `programming`, `finance`, `health`, and `philosophy`
- A 3-verb knowledge workflow: `learn -> ingest -> ask`
- A retrieval pipeline for `zbrain ask` / `zbrain:ask` using qmd BM25 search only
- Bundled runtime assets extracted into `~/.zbrain/`
- Project registration through `zbrain init`, stored in SQLite (`~/.zbrain/zbrain.db`), readable via `zbrain workspace current`

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
- `zbrain learn`
- `zbrain ingest list|analyze|qa|apply`
- `zbrain ask <question>`
- `zbrain update`

Bundled runtime skills:

- `zbrain:learn`
- `zbrain:ingest`
- `zbrain:ask`

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
- `zbrain.db` (project registry + note/session/evidence index)
- `engine/`
- `templates/`
- `skills/`
- `agents/`
- `workspaces/`

Project-local integration is runtime-specific while project config stays in `~/.zbrain/`:

- Claude: optional `.claude/skills`, `.claude/agents`, `.claude/settings.local.json`, `CLAUDE.md`
- Codex: optional `AGENTS.md` rules injection

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

- learn: record raw source material into workspace evidence sources
- ingest: list, analyze, QA, and apply evidence into workspace knowledge
- ask: workspace-scoped qmd retrieval -> project `context_file` under `~/.zbrain/projects/`

Acceptance proof for the full path is in [docs/acceptance-walkthrough.md](/home/tinhpt/Lab/zbrain/docs/acceptance-walkthrough.md).

## Team setup

A workspace becomes shareable by turning it into a git repo. `personal` (or any workspace you
never `sync`) stays local-only; a workspace you run `zbrain sync init` on becomes a git repo that
teammates clone or pull.

First teammate (creates the shared workspace):

```bash
zbrain workspace create team-shared
zbrain sync init team-shared --remote <git-url>   # git init + remote add + first push
```

Teammate #2 (joins the shared workspace):

```bash
zbrain sync init team-shared --remote <git-url>   # clone-equivalent: init + remote + pull
zbrain setup
cd <project> && zbrain init                       # pick team-shared (or personal + team-shared as secondary)
zbrain sync team-shared                           # pull --rebase, push, reindex
```

Daily loop: run `zbrain sync <workspace>` at the start and end of a work session (or wire it into
a Claude Code `SessionStart`/`SessionEnd` hook) so local commits get pushed and teammates' commits
get pulled and reindexed before you rely on retrieval.

**What sync does and doesn't cover:**

- Advisory write leases (`zbrain lease acquire/release/list`) and optimistic locking
  (`content_sha` / `ShaMismatchError`) only protect **multiple agents writing on the same
  machine** against clobbering each other in real time.
- Consistency **across machines** is git's job: `zbrain sync` commits, `pull --rebase`, pushes,
  then reindexes. The supersede-not-overwrite note lifecycle keeps concurrent edits from
  different machines rare and, when they do collide, resolvable as an ordinary git conflict in a
  markdown file rather than silent data loss.
- `personal` workspace is intentionally never synced — keep team knowledge in a dedicated
  workspace (e.g. `team-shared`) instead of mixing it into a personal one.

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
