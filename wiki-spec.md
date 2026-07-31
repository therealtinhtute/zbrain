# zbrain — Trusted Memory Product Spec

> Living reference for the current Go-native zbrain implementation.

## 1. What Is This Project

`zbrain` is a local-first CLI for trusted personal and agent memory. It stores workspace-isolated OKF-style Markdown claim concepts, captures immutable local evidence snapshots, builds a disposable SQLite FTS5 index, and returns trusted context JSON for agents.

Core claim: agents should only receive explicit, approved, scoped memory. Drafts, gaps, and conflicts must be visible instead of silently blended into answers.

## 2. Current Scope

In scope:

- Standalone Go binary.
- Embedded runtime assets from `assets/`.
- Workspace setup and current workspace resolution.
- OKF claim concept documents in four semantic tiers.
- zbrain trusted-memory profile metadata on top of OKF frontmatter.
- Immutable local evidence snapshots.
- Claim lifecycle: `draft -> approved -> superseded|revoked`.
- Rebuildable per-workspace SQLite FTS5 indexes.
- Trusted context JSON from `zbrain ask`.
- Explicit migration from legacy `schema: zbrain.claim/v1` claims to OKF claim concepts.

Out of scope for this slice:

- LLM/model-provider calls.
- MCP integration.
- Vector databases.
- Network crawling or web research.
- Hosted sync, team/auth, background services, or GUI.
- Session transcript storage.
- Generic OKF editing for arbitrary concept types.

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

## 4. Claim Concept Model

One Markdown file is one OKF concept document. Trusted zbrain claims are the subset with `type: zbrain.claim` and `zbrain.profile: zbrain.trusted-memory/v1`.

Canonical frontmatter shape:

```yaml
type: zbrain.claim
title: "..."
description: "..."
resource: "..."
tags: []
sources:
  - id: evd_<32 lowercase hex>
    resource: evidence/sources/evd_<32 lowercase hex>/raw
    title: "file://source.txt"
    digest: sha256:<hex>
generated:
  at: "<RFC3339>"
  by: owner
verified:
  at: "<RFC3339>"
  by: owner
  digest: sha256:<hex>
status: draft | approved | superseded | revoked
stale_after: "<RFC3339>"
zbrain:
  profile: zbrain.trusted-memory/v1
  id: clm_<32 lowercase hex>
  tier: axioms | mental-models | projects | decisions
  basis: owner | evidence | derived
  evidence_ids: []
  supporting_claim_ids: []
  supersedes: []
  conflicts_with: []
```

Rules:

- OKF `type` is required; trusted claim concepts use `zbrain.claim`.
- `zbrain.id` is the stable trust identity. Path is not trusted identity.
- `status` is the zbrain lifecycle vocabulary for this OKF profile.
- `generated.at` and `generated.by` record creation provenance.
- `verified` appears when a claim is approved and is tied to the current rendered concept digest.
- `sources` is populated from verified local evidence metadata.
- Non-zbrain OKF documents may live in the wiki but are not trusted context.

Approval requirements:

- `owner` claims require owner confirmation metadata.
- `evidence` claims require existing immutable evidence IDs whose hashes verify.
- `derived` claims require approved supporting claim IDs or verified evidence IDs.

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

The raw snapshot and metadata are made read-only. Verification recomputes the hash and byte length. Raw evidence content is source data, not trusted context, and is not indexed by `zbrain ask`.

## 6. Index Model

Canonical truth is Markdown. SQLite is derived cache only.

`zbrain reindex` scans valid zbrain OKF claim concepts in the selected workspace, records valid drafts and approved claims, reports invalid/legacy counts, and rebuilds the FTS5 database under `indexes/<workspace>.sqlite`.

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
| `zbrain claim draft` | Create a draft OKF claim concept from stdin |
| `zbrain claim approve <id>` | Promote a valid draft to approved and record verification metadata |
| `zbrain claim supersede <id>` | Create a replacement draft for an approved claim |
| `zbrain claim revoke <id> --reason <reason>` | Revoke a claim with a reason |
| `zbrain migrate okf` | Convert legacy `zbrain.claim/v1` files to OKF claim concepts |
| `zbrain reindex` | Rebuild the derived local FTS5 index |
| `zbrain ask <query>` | Return trusted context JSON |
| `zbrain version` | Print version |

## 9. Design Philosophy

1. Trust first: approved claim concepts only.
2. Local first: no server required.
3. Workspace isolation by default.
4. Explicit includes for cross-workspace context.
5. Markdown canonical, SQLite disposable.
6. OKF-compatible files with stricter zbrain trust metadata.
7. Gaps and conflicts fail closed.
8. Small Go-native runtime before integrations.

## 10. Release Gate

The trusted-memory slice must prove:

- `go test ./...` passes.
- `make build` passes.
- `make smoke` passes with isolated `ZBRAIN_HOME`.
- 100k claim query benchmark p95 stays under 2 seconds.
