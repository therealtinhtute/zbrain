# Release Proof

## Build

zbrain is a standalone Go binary. Build it with:

```bash
make build
```

This runs `go build -o dist/zbrain ./cmd/zbrain` and produces `dist/zbrain`.
The binary needs no JavaScript runtime, package manager, external database, or
retrieval service.

## Packaging strategy

Build on the target platform with the same Go-native command. The binary embeds
runtime content from `assets/`; `zbrain setup` extracts `README.md`, `agents/`,
`engine/`, `skills/`, and `templates/` under `ZBRAIN_HOME` or the default
`~/.zbrain/`. Any embedded `workspaces/` seed is skipped. `workspace create`
creates active workspace paths, and `reindex` creates the disposable SQLite
FTS5 index.

Do not package secrets, populated personal workspaces, evidence snapshots, or
runtime output from another operator.

## Command and smoke checks

Verify the root command surface and command groups:

```bash
go run ./cmd/zbrain --help
go run ./cmd/zbrain workspace --help
go run ./cmd/zbrain evidence --help
go run ./cmd/zbrain claim --help
go run ./cmd/zbrain migrate --help
go run ./cmd/zbrain reindex --help
go run ./cmd/zbrain ask --help
go run ./cmd/zbrain status --help
go run ./cmd/zbrain doctor --help
```

Run the isolated smoke target:

```bash
make smoke
```

The target builds the binary, runs setup and workspace creation with a
temporary `ZBRAIN_HOME`, captures evidence, drafts and approves a claim,
rebuilds the index, and queries trusted context.

## Trust release gates

Run all quality and trust gates before release:

```bash
go test ./...
go vet ./...
go test -race ./internal/runtime ./internal/cli
make build
make smoke
git diff --check
```

These checks cover freshness invalidation, evidence and dependency rejection,
canonical-input preservation, rejected rebuild fail-closed behavior, and
interrupted supersession recovery.

## Scale gate

The 100k-claim query benchmark must keep p95 below two seconds:

```bash
ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v
```

A benchmark result above two seconds is a release blocker.

## Release checklist

1. `go run ./cmd/zbrain --help` matches the documented shipped surface.
2. Every documented command group returns the expected `--help` output.
3. `go test ./...` passes.
4. `go vet ./...` passes.
5. `go test -race ./internal/runtime ./internal/cli` passes.
6. `make build` passes.
7. `make smoke` passes with isolated `ZBRAIN_HOME`.
8. The 100k-claim query p95 is at or below two seconds.
9. `git diff --check` passes.
