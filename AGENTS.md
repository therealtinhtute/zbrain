# AGENTS.md

Go 1.25.13 (CI matrix 1.25.x), Go-native CLI at `cmd/zbrain`. Do not reintroduce Bun/Node/TypeScript.

## Layout

- `cmd/zbrain/` — binary entrypoint (thin, delegates to runtime)
- `internal/cli/` — arg parsing, dispatch, JSON/text output; `cli.go:Version = "0.2.0"`
- `internal/runtime/` — durable logic: paths, config, assets, workspaces, claims, evidence, trust validation, index (FTS5), query; 30+ `*_test.go` alongside
- `internal/mcp/`, `internal/view/` — stdio MCP gateway and loopback viewer
- `assets/` — embedded source of truth (`assets.go` via `go:embed`), copied by `zbrain setup`; never edit extracted runtime directly
- `docs/` — specs and authored docs; `docs/README.md` is the doc map

## Commands — use these exact forms

```bash
go test ./...                          # full gate (CI: ubuntu+macos)
go test ./internal/runtime -run ^TestFoo$ -count=1 -v  # single test
go test ./internal/runtime -count=1 -v                 # single package
go test -race ./internal/runtime ./internal/cli ./internal/view ./internal/mcp
go vet ./...
make build                             # → dist/zbrain (embeds assets/)
make smoke                             # full lifecycle in isolated ZBRAIN_HOME (uses trash)
CGO_ENABLED=0 go build ./cmd/zbrain    # CI requires CGO-free build
git diff --check
ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v
```

CI order in `.github/workflows/test.yml` (push `master`/`v2/**`, PR→`master`): `go test ./...` → `go vet ./...` → `go test -race ...` → `make build` → `make smoke` → `git diff --check` → `CGO_ENABLED=0 go build`.

Verify CLI surface: `go run ./cmd/zbrain --help` and sub-helps (`workspace`, `evidence`, `claim`, `migrate`, `reindex`, `ask`, `status`, `doctor`, `mcp serve`, `view`, `approval`).

## Workspace & Runtime Gotchas

- `ZBRAIN_HOME` overrides `~/.zbrain` for every command. Must be same value for the whole session; tests and smoke must use a temp dir (`mktemp -d`) and never touch real `~/.zbrain`. `make smoke` does this via `ZBRAIN_HOME=$tmp_home ./dist/zbrain setup`.
- `zbrain setup` extracts `README.md`, `agents/`, `engine/`, `skills/`, `templates/` directly under the runtime root. `workspace create <name>` then sets `default_workspace` in `config.yml`.
- Indexes are disposable SQLite FTS5 at `indexes/<workspace>.sqlite` with a `.dirty` marker during rebuild. Never commit or hand-edit them; rebuild with `zbrain reindex` (or `reindex --embed` for local hybrid). `ask` fails closed on missing/dirty/stale/rejected indexes — check `status`/`doctor` (doctor exits `2` on domain findings).
- `claim draft` reads body from **stdin**; metadata via flags. Lifecycle is `draft -> approved -> superseded|revoked` — approved claims are superseded, never edited in place. `claim approve` records `verified.at/by/digest`; `reindex` validates before publishing.
- `ask` default is lexical; `--embed` opts into local loopback embedding sidecar (also `memory_ask`/`memory_reindex` `embedding: true`). Missing sidecar falls back to lexical, no network calls.
- `mcp serve` is stdio-only (stdout=protocol, stderr=diagnostics). `view` binds `127.0.0.1` only, `GET`/`HEAD` only, strict CSP/`nosniff`, no CORS. Owner-pinned lifecycle: `claim_lifecycle prepare` → `approval show <id>` → `approval grant <id>` (TTY, confirm last 16 hex of digest) → `claim_lifecycle apply`. Challenge 15m, token 5m capped by challenge, single-use.
- File modes enforced in `internal/runtime/paths.go`: dirs `0700`, mutable metadata/canonical Markdown `0600`, evidence snapshots+`source.yaml` `0400`, derived indexes/dirty `0600`.

## Trust Rules — do not violate

- Workspace isolation is hard: never read across workspaces without explicit `--include <name>` (read-only secondary).
- Evidence snapshots (`evidence add --file <path> --origin <uri>`) are immutable local copies; raw evidence is untrusted, never indexed.
- Only `type: zbrain.claim` + `zbrain.profile: zbrain.trusted-memory/v1` with `status: approved` and valid digest/closure may enter trusted results. Drafts are `promotion_candidates` only. Conflicts → `status: "blocked"`, no match → `status: "gap"` (not permission to use drafts/evidence).
- `reindex` rejects invalid claims/evidence/broken `support` closures without rewriting canonical files.

## Assets & Style

- After editing `assets/`, rebuild and run tests+smoke — binary embeds them.
- Skill files `assets/skills/*/SKILL.md` require frontmatter `name`, `description`, `version`. Templates use `{{placeholder}}` tokens; claim templates must be OKF Markdown; evidence `source.yaml` must match `evidence.go`.
- Keep handlers thin, put durable behavior in `internal/runtime/`. Use `trash`, never `rm` (see `Makefile:32,40`). Use standard `gofmt`; no extra linter config.
- Gitignored: `dist/`, `harness.db*`, `.kit/`, `.opencode/`, `workspaces/`, `.cache/` — never commit runtime output or secrets.

## Commits & Docs

- Conventional Commits: `feat(cli): ...`, `fix(runtime): ...`, `docs(spec): ...` with specific scope.
- Authoritative sources: `README.md`, `CONTRIBUTING.md`, `trusted-memory-spec.md`, `docs/trusted-agent-gateway-spec.md`. If docs conflict with `go run ./cmd/zbrain --help` or `internal/runtime/`, trust the executable.

<!-- ZHARNESS:BEGIN -->
## Harness

Run `zharness --version`, then `zharness preflight <stage> [--mode <mode>] --json` for every workflow skill invocation. Follow a returned stop and recovery exactly.

Read `docs/WORKFLOW.md`, then only the returned stage playbook and the repository material relevant to the requested outcome — start that search at `docs/README.md`, this repository's authored documentation map; if it is absent, proceed without it, which is not an error. Repository docs, code, tests, and observable behavior are authoritative; the database is a lifecycle ledger and recovery index.

Read-only and bounded work may use reduced mode and must not mutate harness state. Durable planning, full execution, full checks, and durable handoffs require an initialized database. Claim completion only with executable or observable evidence.
<!-- ZHARNESS:END -->
