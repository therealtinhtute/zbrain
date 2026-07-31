# Contributing to zbrain

Thanks for your interest in zbrain. zbrain is a local-first trusted-memory CLI for AI agent context: locally owned, human-reviewed, workspace-isolated, and traceable to source.

## Quick Start

```bash
git clone <repo>
cd zbrain
go test ./...
make build
make smoke
```

Smoke tests run against an isolated `ZBRAIN_HOME` and must not touch real `~/.zbrain` data.

## Repository Layout

```text
cmd/zbrain/         CLI entrypoint
internal/cli/       command dispatch and user-facing behavior
internal/runtime/   runtime paths, config, assets, workspaces, claims, evidence, index, query
assets/             source of truth for runtime assets embedded by Go
docs/               durable plans and supporting docs
```

## Architecture

zbrain stores canonical Markdown claims under a workspace and builds disposable local SQLite FTS5 indexes from those files.

Supported flow:

1. `zbrain setup`
2. `zbrain workspace create <name>`
3. `zbrain evidence add --file <path> --origin <uri-or-path>`
4. `zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived>`
5. `zbrain claim approve <id>`
6. `zbrain reindex`
7. `zbrain ask <query>`

`zbrain ask` returns trusted context JSON only. It does not call an LLM.

## Test Conventions

- Tests live next to Go packages as `*_test.go`.
- Use temp directories and explicit runtime paths.
- Use `ZBRAIN_HOME` for integration and smoke checks.
- Add focused tests with every command or runtime behavior change.
- Run `go test ./...` before claiming completion.

## PR Conventions

1. One concern per PR.
2. Tests required for runtime or CLI behavior changes.
3. No new runtime dependencies without explicit approval.
4. Run `go test ./...`, `make build`, and `make smoke` before pushing.
5. Keep changes Go-native unless a maintainer explicitly approves otherwise.

## Scope Boundaries

zbrain is deliberately small. Hosted sync, vector search, background services, GUI editors, model-provider integration, network crawling, team/auth features, and auto-capture-everything memory are out of scope for the current slice.

## Security

- Do not commit secrets, personal workspace data, or populated runtime output.
- Workspace isolation is a hard rule.
- Secondary workspace retrieval requires explicit `--include`.
- Evidence snapshots are immutable local copies.
- Only approved claims are trusted context; drafts are promotion candidates.

## License

By contributing, you agree your contributions are MIT-licensed — same as the project. See `LICENSE`.
