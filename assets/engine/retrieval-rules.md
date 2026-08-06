# Retrieval Rules

## Pipeline

1. Resolve the primary workspace from `zbrain workspace current` or an explicit `--workspace` flag.
2. Include secondary workspaces only when passed with `--include`.
3. Run `zbrain ask [--workspace <name>] [--include <name>]... <query>` to retrieve trusted context JSON.
4. Treat `status: ready` as usable context, `status: gap` as insufficient approved memory, and `status: blocked` as an unresolved explicit conflict.

## Ranking

- Scope and lifecycle are hard filters first.
- Only current approved OKF claim concepts are trusted context.
- Trusted claim concepts use `type: zbrain.claim` and `zbrain.profile: zbrain.trusted-memory/v1`.
- Ranking is BM25 over the derived local SQLite FTS5 index.
- Tiers describe semantic role; they do not override relevance score.

## Failure Mode

If `zbrain ask` returns a gap, blocked conflict, dirty index, or missing workspace, stop and report the exact reason. Do not answer trusted-memory questions from unstored assumptions.
