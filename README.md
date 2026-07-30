# zbrain

`zbrain` is a Go-native CLI for local-first personal memory and workspace-isolated agent context.

This repository has been reset from the previous Bun/TypeScript implementation to a fresh Go implementation. The current Go foundation intentionally starts small: standalone binary, embedded runtime assets, runtime setup, and workspace creation/resolution.

## Current command surface

```bash
zbrain setup
zbrain workspace create <name>
zbrain workspace current
zbrain ask <query>
zbrain version
```

Planned commands to rebuild next:

- `zbrain note ...`
- `zbrain learn ...`
- `zbrain ingest ...`
- `zbrain mcp ...`
- `zbrain doctor`
- `zbrain sync`
- `zbrain export` / `zbrain import`

## Development

Prerequisite: Go.

```bash
make test
make build
make smoke
```

Equivalent raw commands:

```bash
go test ./...
go build -o dist/zbrain ./cmd/zbrain
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
```

## Runtime layout

By default, runtime data lives under `~/.zbrain/`. Use `ZBRAIN_HOME` to isolate tests or experiments.

After `zbrain setup` and `zbrain workspace create research`:

```text
~/.zbrain/
  config.yml
  agents/
  engine/
  skills/
  templates/
  workspaces/
    research/
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

`zbrain workspace current` prints JSON with:

- `project_root`
- `workspace`
- `secondary_workspaces`
- `context_file`

## Repository layout

```text
cmd/zbrain/         CLI entrypoint
internal/cli/       command dispatch
internal/runtime/   runtime paths, config, assets, workspace layout
assets/             embedded runtime assets
```

## Reset note

The previous Bun/TypeScript implementation was intentionally removed from the working tree. The Go version uses a fresh runtime/schema direction and does not promise compatibility with the old Bun-created database yet.
