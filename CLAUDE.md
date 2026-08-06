# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Current direction

zbrain has been reset from Bun/TypeScript to a fresh Go-native CLI. Do not reintroduce Bun, Node, TypeScript, Commander, or clack unless the user explicitly asks.

The current implementation is intentionally minimal:

- standalone Go binary
- embedded runtime assets from `assets/`
- `zbrain setup`
- `zbrain workspace create <name>`
- `zbrain workspace current`
- `zbrain evidence add`
- `zbrain claim draft`
- `zbrain claim approve <id>`
- `zbrain claim supersede <id>`
- `zbrain claim revoke <id>`
- `zbrain migrate okf`
- `zbrain reindex`
- `zbrain ask <query>`
- `zbrain version`

## Commands

```bash
go test ./...                       # Run tests
make test                           # Same test gate
make build                          # Build dist/zbrain
make smoke                          # Build and run isolated smoke
```

Manual smoke:

```bash
go build -o dist/zbrain ./cmd/zbrain
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain workspace create research
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain workspace current
```

`ZBRAIN_HOME` overrides the default `~/.zbrain` runtime directory everywhere — useful for test isolation and smoke runs.

## Architecture

```text
cmd/zbrain/         CLI entrypoint
internal/cli/       command dispatch and user-facing command behavior
internal/runtime/   runtime paths, config, embedded asset extraction, workspace layout
assets/             source of truth for runtime assets embedded via Go embed
```

## Implementation rules

- Keep changes Go-native and minimal.
- Preserve `assets/` as the runtime content source of truth.
- Add tests with each command or runtime behavior.
- Prove changes with `go test ./...` and a relevant isolated `ZBRAIN_HOME` smoke command.
- Use `trash`, never `rm`, for deletions.

## Runtime model

Default runtime root: `~/.zbrain/`.

Runtime and workspace layout:

```text
~/.zbrain/
  config.yml
  README.md                  # extracted runtime asset
  agents/                    # extracted runtime agents
  engine/                    # extracted engine rules
  skills/                    # extracted skills and references
  templates/                 # extracted templates
  indexes/                   # created when a workspace is reindexed
    <workspace>.sqlite
    <workspace>.dirty        # present while a rebuild is incomplete
  workspaces/<workspace>/
    workspace.md
    agents/
    wiki/
      axioms/
      mental-models/
      projects/
      decisions/
    evidence/
      _index.md
      sources/
      analysis/
      qa/
      applied/
      archive/
```

`zbrain setup` extracts embedded files directly under the runtime root; workspace seed assets are not activated. `zbrain workspace current` prints JSON with `project_root`, `workspace`, and `secondary_workspaces`; it does not advertise session transcript or `context_file` storage. It resolves the primary workspace from `config.yml` `default_workspace`. `ZBRAIN_HOME` replaces `~/.zbrain/` for isolated tests and smoke runs.

Runtime ownership permissions are owner-only: directories use `0700`, mutable
metadata and canonical Markdown use `0600`, immutable evidence snapshots and
metadata use `0400`, and derived index databases/dirty markers use `0600`.
