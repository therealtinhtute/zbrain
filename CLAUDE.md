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

Workspace layout:

```text
workspaces/<workspace>/
  workspace.md
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

`zbrain workspace current` prints JSON and currently resolves from `config.yml` `default_workspace`.
