# Release Proof

## Build

zbrain is a standalone Rust binary. Build it with:

```bash
make build
```

This runs `cargo build --release -p zbrain --bin zbrain` and produces `dist/zbrain`
plus `dist/zbrain.stripped`.
The binary needs no JavaScript runtime, package manager, external database, or
retrieval service.

## Packaging strategy

Build on the target platform with the same Rust-native command. The binary embeds
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
cargo run -q -p zbrain -- --help
cargo run -q -p zbrain -- workspace --help
cargo run -q -p zbrain -- evidence --help
cargo run -q -p zbrain -- claim --help
cargo run -q -p zbrain -- migrate --help
cargo run -q -p zbrain -- reindex --help
cargo run -q -p zbrain -- ask --help
cargo run -q -p zbrain -- status --help
cargo run -q -p zbrain -- doctor --help
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
cargo test --workspace
cargo clippy --workspace --all-targets -- -D warnings
cargo fmt --all -- --check
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
ZBRAIN_BENCH_100K=1 cargo test -p zbrain --test bench_100k
```

A benchmark result above two seconds is a release blocker.

## Release checklist

1. `cargo run -q -p zbrain -- --help` matches the documented shipped surface.
2. Every documented command group returns the expected `--help` output.
3. `cargo test --workspace` passes.
4. `cargo clippy --workspace --all-targets -- -D warnings` passes.
5. `cargo fmt --all -- --check` passes.
6. `make build` passes.
7. `make smoke` passes with isolated `ZBRAIN_HOME`.
8. The 100k-claim query p95 is at or below two seconds.
9. `git diff --check` passes.
