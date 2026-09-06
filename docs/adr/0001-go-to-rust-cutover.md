# ADR 0001: Cut over zbrain from Go to Rust

- Status: accepted
- Date: 2026-09-06
- Deciders: owner + migration plan `docs/plans/completed/go-to-rust-migration.md`

## Decision

Replace the Go implementation (`internal/`, `cmd/`, `go.mod`) with the Rust
workspace (`crates/zbrain`) in a single cutover. The Go tree is deleted from
`master`; the oracle survives only under tag `v0.2.0-go-final` and git history.

## Context

Big-bang rewrite (m0–m8) with the frozen Go binary as a living parity oracle:
golden fixtures, differential CLI harness, 1:1 trust-critical test ports,
byte-identical claim/evidence/index artifacts both directions.

## Consequences

- CI, Makefile, AGENTS.md, and user docs are Rust-authoritative.
- The differential parity harness retired with the oracle; committed fixtures
  and golden capture tests keep the regression value.
- `cargo audit` is a release gate (it caught an unsound YAML dependency at
  cutover: serde_yml/libyml → pure-Rust yaml-rust2).

## Recovery

`git checkout v0.2.0-go-final` rebuilds the last Go-native binary
(`CGO_ENABLED=0 go build ./cmd/zbrain`). Reverting the cutover commits
restores the Go tree; its CI workflow is preserved in history.
