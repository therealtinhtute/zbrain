# Acceptance Walkthrough

## Covered Path

This walkthrough exercises the shipped Go-native CLI in an isolated runtime:

1. `zbrain --help`
2. `zbrain setup`
3. `zbrain workspace create <name>`
4. `zbrain workspace current`
5. `zbrain evidence add --file <path> --origin <uri-or-path> [--media-type <type>] [--workspace <name>]`
6. `zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]`
7. `zbrain claim approve <id> [--workspace <name>]`
8. `zbrain reindex [--workspace <name>]`
9. `zbrain ask [--workspace <name>] [--include <name>]... <query>`

`claim supersede`, `claim revoke`, and `migrate okf` are covered by the Go
runtime and CLI test suites.

## Validation Commands

Run the repository gates from the project root:

```bash
go test ./...
go vet ./...
go test -race ./internal/runtime ./internal/cli
make build
make smoke
ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v
```

The benchmark must report a 100k-claim query p95 at or below two seconds.

## Trust and Recovery Gates

The runtime tests prove that:

- outside edits, additions, deletions, and symlinks make the next query fail closed;
- invalid approved claims, evidence, and recursive supporting-claim closures are
  rejected during rebuild without mutating canonical files;
- a rejected rebuild excludes invalid claims from trusted results;
- evidence restoration followed by a clean rebuild restores trusted querying;
- an interrupted supersession is journaled, recovered before mutation, and blocks
  `ask` while the pending transition remains unresolved.

## Isolated Runtime

Use `ZBRAIN_HOME` for every manual or smoke run so the test cannot touch the
operator's real `~/.zbrain` directory:

```bash
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain workspace create research
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain workspace current
```

`setup` extracts embedded `README.md`, `agents/`, `engine/`, `skills/`, and
`templates/` directly under the selected runtime root. Workspace seed assets are
not treated as active workspaces; `workspace create` creates the workspace and
`reindex` creates its disposable index. The runtime has no external runtime or
retrieval prerequisite; `zbrain ask` returns trusted context JSON and does not
call an LLM.
