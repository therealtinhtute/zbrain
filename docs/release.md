# Release Proof

## Build

zbrain is a standalone Go binary. Build it with the repository target:

```bash
make build
```

This runs `go build -o dist/zbrain ./cmd/zbrain` and produces `dist/zbrain`.
No JavaScript runtime, package manager, or external retrieval service is required.

## Packaging Strategy

Build on the target platform with the same Go-native command. The binary embeds
the runtime content from `assets/`; `zbrain setup` extracts that content into the
selected runtime root.

## Acceptance and Smoke Checks

Verify the command surface and run the isolated smoke target:

```bash
go run ./cmd/zbrain --help
make smoke
```

The smoke target builds the binary, checks help output, creates an isolated
`ZBRAIN_HOME`, runs setup and workspace creation, captures evidence, drafts and
approves a claim, rebuilds the index, and queries trusted context.

## Trust Release Gates

Run all quality and trust gates before release:

```bash
go test ./...
go vet ./...
go test -race ./internal/runtime ./internal/cli
git diff --check
```

These tests cover freshness invalidation, dependency and evidence rejection,
canonical-input preservation, rejected rebuild fail-closed behavior, and
interrupted supersession recovery.

## Scale Gate

The 100k-claim query benchmark must keep p95 below two seconds:

```bash
ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v
```

A benchmark result above two seconds is a release blocker.

## Release Checklist

1. `go run ./cmd/zbrain --help` matches the documented command surface.
2. `go test ./...` passes.
3. `go vet ./...` passes.
4. `go test -race ./internal/runtime ./internal/cli` passes.
5. `make build` passes.
6. `make smoke` passes with isolated `ZBRAIN_HOME`.
7. The 100k-claim query p95 is at or below two seconds.
8. `git diff --check` passes.
