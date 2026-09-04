# PROJECT — identity (answer inline; this is the single forced write step at
# brainstorm lock; keep the whole file at or under 50 lines)

## What is this project?
- `zbrain` is a local-first, Go-native CLI for workspace-isolated trusted
  memory: canonical OKF Markdown claims, immutable evidence snapshots, a
  disposable SQLite FTS5 index, and fail-closed trusted context JSON for
  coding agents.

## Who is it for?
- Solo developers and small teams running coding agents (Claude Code, Codex,
  OpenCode, Cursor) locally who need agents to read only explicitly approved,
  digest-verified knowledge — never drafts or raw evidence.

## Non-goals
- LLM/model-provider calls from zbrain core; HTTP/remote MCP transport;
  hosted sync, team auth, background services, GUI; network crawling or
  connectors fetching remote sources; session transcript storage; viewer
  mutation APIs or remote binds.

## How do we run the tests?
- `go test ./...` · `go vet ./...` · `make build` · `make smoke`
- plus `go test -race ./internal/runtime ./internal/cli ./internal/view ./internal/mcp`
  and `CGO_ENABLED=0 go build ./cmd/zbrain`

## Architecture in one breath
- runtime shape: thin CLI in `cmd/zbrain`/`internal/cli`; durable behavior in
  `internal/runtime`; stdio MCP gateway in `internal/mcp`; loopback viewer in
  `internal/view`
- where state lives: `~/.zbrain` (or `ZBRAIN_HOME`) — canonical OKF Markdown
  claims (0600), immutable evidence snapshots (0400), disposable SQLite FTS5
  indexes (0600, rebuildable via `zbrain reindex`)
- entrypoints: `zbrain` CLI, `zbrain mcp serve` (stdio), `zbrain view`
  (127.0.0.1 only)

## What are we working on right now?
- plan: none active (trusted-memory-hygiene completed 2026-09-04; PR #28 merged)
