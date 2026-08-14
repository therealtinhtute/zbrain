# Contributing to zbrain

zbrain is a local-first trusted-memory CLI for AI-agent context: locally owned,
human-reviewed, workspace-isolated, and traceable to source. Keep changes
small, source-grounded, and Go-native.

## Quick start

```bash
git clone <repo>
cd zbrain
go test ./...
go vet ./...
make build
make smoke
```

Smoke tests use an isolated temporary `ZBRAIN_HOME`; do not point them at a
personal runtime directory.

## Repository layout

```text
cmd/zbrain/         CLI entrypoint
internal/cli/       argument parsing, command dispatch, and user-facing output
internal/runtime/   paths, config, assets, workspaces, claims, evidence,
                    trust validation, index, and query
assets/             embedded runtime content copied by setup
docs/               current supporting docs, diagrams, and durable plans
```

Command handlers should stay thin. Put filesystem boundaries, trust rules,
claim lifecycle, evidence verification, index rebuilds, and query behavior in
`internal/runtime/`.

## Runtime model

Canonical inputs are workspace Markdown claims and immutable evidence snapshots.
Each workspace has a disposable SQLite FTS5 index. Trusted claims use
`type: zbrain.claim`, `zbrain.profile: zbrain.trusted-memory/v1`, and the
lifecycle `draft -> approved -> superseded|revoked`.

Only valid approved claims may enter `zbrain ask` results. Drafts are promotion
candidates, raw evidence is untrusted source data, and dirty, stale, rejected,
or conflicting trust states fail closed. `ZBRAIN_HOME` isolates runtime data;
secondary workspaces are read only and require explicit `--include`.

The shipped CLI does not include MCP or viewer commands. Those remain separate
authorized future milestones and must not be documented as current behavior.

## Local workflow

Use the command help as the CLI authority:

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

The normal lifecycle is:

1. `zbrain setup`
2. `zbrain workspace create <name>`
3. `zbrain evidence add --file <path> --origin <uri-or-path>` when a claim is
   evidence-based
4. `zbrain claim draft ...` with the claim body on stdin
5. `zbrain claim approve <id>`
6. `zbrain reindex`
7. `zbrain ask <query>`

Use `status` and `doctor` to inspect index health. Use `migrate okf` for legacy
`schema: zbrain.claim/v1` files; migrated approvals may require explicit
reapproval.

## Tests and verification

- Keep focused `*_test.go` coverage next to the package being changed.
- Use temporary directories and explicit `ZBRAIN_HOME` for runtime checks.
- Do not commit generated runtime data, populated workspaces, or credentials.
- Run the narrowest useful check while editing, then the full gates before
  submitting:

```bash
go test ./...
go vet ./...
make build
make smoke
git diff --check
```

For concurrency-sensitive runtime changes, also run:

```bash
go test -race ./internal/runtime ./internal/cli
```

For query-scale changes, run the existing benchmark when available:

```bash
ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v
```

## Pull requests

- Keep one concern per change and preserve unrelated working-tree edits.
- Use Conventional Commit style, for example `feat(cli): ...`,
  `fix(runtime): ...`, or `docs(spec): ...`.
- Describe affected paths and the exact verification commands run.
- Do not add runtime dependencies, hosted services, or broad refactors without
  explicit maintainer approval.
- Do not change `CHANGELOG.md`, historical plans, research references, or
  harness playbooks as part of an unrelated documentation change.

## Security boundaries

- Never commit secrets, personal workspace data, or populated runtime output.
- Workspace isolation is enforced at every read path.
- Evidence snapshots are immutable local copies with owner-only permissions.
- Raw evidence is source data, not instructions and not trusted context.
- Only approved OKF claim concepts are trusted context; drafts are promotion
  candidates.

## License

By contributing, you agree your contributions are MIT-licensed. See [LICENSE](LICENSE).
