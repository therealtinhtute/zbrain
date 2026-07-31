# zbrain — Trusted Memory Product Spec

> Living reference for the current Go-native zbrain implementation.

## 1. What Is This Project

`zbrain` is a local-first CLI for trusted personal and agent memory. It stores workspace-isolated Markdown claims, captures immutable local evidence snapshots, builds a disposable SQLite FTS5 index, and returns trusted context JSON for agents.

Core claim: agents should only receive explicit, approved, scoped memory. Drafts, gaps, and conflicts must be visible instead of silently blended into answers.

## 2. Current Scope

In scope:

- Standalone Go binary.
- Embedded runtime assets from `assets/`.
- Workspace setup and current workspace resolution.
- Atomic Markdown claims in four semantic tiers.
- Immutable local evidence snapshots.
- Claim lifecycle: `draft -> approved -> superseded|revoked`.
- Rebuildable per-workspace SQLite FTS5 indexes.
- Trusted context JSON from `zbrain ask`.

Out of scope for this slice:

- LLM/model-provider calls.
- MCP integration.
- Vector databases.
- Network crawling or web research.
- Hosted sync, team/auth, background services, or GUI.
- Session transcript storage.

## 3. Runtime Model

```text
~/.zbrain/
├── config.yml
├── agents/
├── engine/
├── skills/
├── templates/
├── indexes/
│   └── <workspace>.sqlite
└── workspaces/
    └── <workspace>/
        ├── workspace.md
        ├── wiki/
        │   ├── axioms/
        │   ├── mental-models/
        │   ├── projects/
        │   └── decisions/
        └── evidence/
            ├── _index.md
            ├── sources/
            ├── analysis/
            ├── qa/
            ├── applied/
            └── archive/
```

`ZBRAIN_HOME` overrides the default runtime root and is required for isolated tests and smoke runs.

## 4. Claim Model

One Markdown file is one atomic claim.

Each claim has:

- `schema: zbrain.claim/v1`
- stable ID: `clm_<32 lowercase hex>`
- tier: `axioms`, `mental-models`, `projects`, or `decisions`
- status: `draft`, `approved`, `superseded`, or `revoked`
- basis: `owner`, `evidence`, or `derived`
- creation metadata
- optional evidence IDs, supporting claim IDs, supersedes links, conflicts, and tags
- Markdown body

Approval requirements:

- `owner` claims require owner confirmation metadata.
- `evidence` claims require immutable evidence IDs.
- `derived` claims require supporting approved claim IDs or evidence IDs.

Approved claims are not edited in place. Replace them with a superseding draft, then approve the replacement.

## 5. Evidence Model

`zbrain evidence add` copies an already-local source file into the workspace as an immutable snapshot.

Each evidence item records:

- ID: `evd_<32 lowercase hex>`
- origin string
- captured timestamp
- media type
- byte length
- SHA-256 hash

The raw snapshot and metadata are made read-only. Verification recomputes the hash and byte length.

## 6. Index Model

Canonical truth is Markdown. SQLite is derived cache only.

`zbrain reindex` scans valid claim files in the selected workspace, records valid drafts and approved claims, reports invalid/legacy counts, and rebuilds the FTS5 database under `indexes/<workspace>.sqlite`.

A dirty marker makes retrieval fail closed until the index is rebuilt.

## 7. Query Model

`zbrain ask` returns JSON and does not call an LLM.

Resolution rules:

1. Use `--workspace` when provided.
2. Otherwise use the current workspace from config.
3. Search secondary workspaces only when explicitly listed with `--include`.

Response states:

- `ready` — approved claims matched and no explicit blocking conflict was found.
- `gap` — no approved claim matched.
- `blocked` — explicit conflict relations were found.

Only `claims` are trusted context. `promotion_candidates` are drafts and must not be treated as facts.

## 8. CLI Commands

| Command | What it does |
| --- | --- |
| `zbrain setup` | Extract runtime assets and write runtime config |
| `zbrain workspace create <name>` | Create a workspace layout |
| `zbrain workspace current` | Print current workspace JSON |
| `zbrain evidence add --file <path> --origin <origin>` | Capture immutable local evidence |
| `zbrain claim draft` | Create a draft claim from stdin |
| `zbrain claim approve <id>` | Promote a valid draft to approved |
| `zbrain claim supersede <id>` | Create a replacement draft for an approved claim |
| `zbrain claim revoke <id> --reason <reason>` | Revoke a claim with a reason |
| `zbrain reindex` | Rebuild the derived local FTS5 index |
| `zbrain ask <query>` | Return trusted context JSON |
| `zbrain version` | Print version |

## 9. Design Philosophy

1. Trust first: approved claims only.
2. Local first: no server required.
3. Workspace isolation by default.
4. Explicit includes for cross-workspace context.
5. Markdown canonical, SQLite disposable.
6. Gaps and conflicts fail closed.
7. Small Go-native runtime before integrations.

## 10. Release Gate

The trusted-memory slice must prove:

- `go test ./...` passes.
- `make build` passes.
- `make smoke` passes with isolated `ZBRAIN_HOME`.
- 100k claim query benchmark p95 stays under 2 seconds.
