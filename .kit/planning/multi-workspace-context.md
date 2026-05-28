# SPEC: Multi-Workspace Context Loading

Status: locked
Input Type: change-request
Lane: normal
Risk Flags: data-model, existing-behavior
Affected Surfaces: worker, api
Downstream: plan full
Updated At: 2026-05-28

---

## Goal

Allow a project bound to one primary workspace to selectively load context from secondary workspaces — on-demand via keyword auto-triggers and manual `@workspace` tags — without breaking workspace isolation as the default.

**Example**: Project `admin` belongs to workspace `ttdvkh`. When the query mentions "file-storage" or "starter-kit", results from `framework-core` are also retrieved. User can also write `@research` to explicitly pull from the `research` workspace.

---

## Users / Actors

- **AI agent (Claude Code)**: queries workspace context during `/ask` and skill execution
- **Human engineer**: configures secondary workspace mappings per project

---

## Requirements

### Core

1. **R1 — Secondary workspace config**: `zbrain.json` gains an optional `secondary_workspaces` array. Each entry specifies a workspace name, trigger keywords, and an optional result cap.
2. **R2 — Keyword auto-trigger**: During retrieval, if the query contains any keyword mapped to a secondary workspace, that workspace is queried automatically.
3. **R3 — `@workspace` manual tag**: User can write `@workspace-name` anywhere in the query to explicitly include that workspace in retrieval. The tag is stripped before BM25 search.
4. **R4 — Primary-first merge**: Primary workspace results fill slots first (up to their natural count). Secondary workspace results fill remaining slots up to the total limit. If multiple secondaries match, they share remaining slots proportionally.
5. **R5 — Source labeling**: Every result in `current-task.md` includes its source workspace name so the agent knows provenance.
6. **R6 — Backward compatible**: Projects without `secondary_workspaces` behave exactly as today. Single-workspace isolation remains the default.

### Config Schema

7. **R7 — zbrain.json extension**:

```jsonc
{
  "workspace": "ttdvkh",
  "secondary_workspaces": [
    {
      "workspace": "framework-core",
      "keywords": ["file-storage", "starter-kit", "boilerplate", "template"],
      "limit": 3
    },
    {
      "workspace": "research",
      "keywords": ["paper", "research", "study"],
      "limit": 2
    }
  ]
}
```

- `workspace` (required): secondary workspace name, must exist in `~/.zbrain/workspaces/`
- `keywords` (required): array of trigger strings, matched case-insensitively against the query
- `limit` (optional, default 3): max results from this secondary workspace per query

### Retrieval Changes

8. **R8 — Multi-workspace query flow**:
   1. Parse `@workspace` tags from query → add to secondary set, strip from query text
   2. Match remaining query against keyword maps → add matching workspaces to secondary set
   3. Query primary workspace (full limit)
   4. For each secondary workspace in set: query with secondary's own limit
   5. Merge: primary results first, then secondary results fill remaining slots up to total limit
   6. Rank within each workspace by tier (P0→P3) then BM25 score (existing behavior)

9. **R9 — Total limit unchanged**: Default total limit stays at 8 (or whatever the caller passes). Primary gets first pick; secondaries share the rest.

### Invariants

10. **I-7 — No silent cross-workspace**: Secondary workspace queries only happen when explicitly configured (config keywords) or explicitly requested (`@tag`). Never auto-discover or auto-expand.
11. **I-8 — Evidence pipeline unchanged**: Evidence operations (ingest, analyze, qa, apply) remain strictly single-workspace. This feature only affects retrieval.
12. **I-9 — Secondary validation**: All secondary workspace names validated at resolution time. Missing workspace → warning log + skip (not hard error), so a stale config entry doesn't break retrieval.

---

## Boundaries

### In Scope

- `ProjectPointer` schema extension (Zod) for `secondary_workspaces`
- Query parser for `@workspace` tags
- Keyword matcher against secondary workspace config
- Multi-workspace retrieval orchestration in `retrieval.ts`
- `current-task.md` output with workspace provenance labels
- Unit tests for: schema parsing, tag extraction, keyword matching, merge logic, backward compat

### Out of Scope

- Global workspace graph (workspace-to-workspace links in `config.yml`)
- Automatic keyword inference from workspace content
- UI for managing secondary workspaces (edit `zbrain.json` directly)
- Changes to evidence pipeline or workspace isolation for write operations
- Skill/agent changes (they consume `current-task.md` as before, just with richer labels)
- Changes to qmd adapter (it already supports querying any collection)

---

## Architecture

### Data Flow

```
Query: "how does the file-storage starter work? @research"
                    │
                    ▼
         ┌─────────────────────┐
         │  1. Parse @tags      │  → extracts: ["research"]
         │  2. Match keywords   │  → "file-storage" hits framework-core
         │                      │  → secondary set: {framework-core, research}
         │  3. Strip tags       │  → clean query: "how does the file-storage starter work?"
         └──────────┬──────────┘
                    │
        ┌───────────┼───────────────┐
        ▼           ▼               ▼
   ┌─────────┐ ┌──────────────┐ ┌──────────┐
   │ ttdvkh  │ │framework-core│ │ research  │
   │ (primary)│ │ (secondary)  │ │(secondary)│
   │ limit: 8 │ │ limit: 3     │ │ limit: 2  │
   └────┬────┘ └──────┬───────┘ └─────┬────┘
        │              │               │
        └──────────────┼───────────────┘
                       ▼
              ┌────────────────┐
              │ Merge (primary │
              │ first, fill    │
              │ remaining)     │
              │ total limit: 8 │
              └───────┬────────┘
                      ▼
             current-task.md
             (with workspace labels)
```

### Files to Change

| File | Change |
|------|--------|
| `src/schemas/config.ts` | Add `secondary_workspaces` to `projectPointerSchema` |
| `src/core/config.ts` | No change (passthrough already works) |
| `src/core/retrieval.ts` | New `retrieveMultiWorkspaceContext()` that orchestrates primary + secondary queries |
| `src/core/query-parser.ts` | **New file** — extract `@workspace` tags, match keywords |
| `src/core/current-task.ts` | Add workspace label to result rows and full-context sections |
| `src/core/workspace-resolver.ts` | Add `resolveSecondaryWorkspaces()` that validates secondary names |
| `tests/` | Unit tests for each changed module |

### current-task.md Output Change

Before:
```
| Score | File | Preview |
```

After:
```
| Score | Workspace | File | Preview |
```

Full Context sections gain a workspace prefix:
```
### [ttdvkh] axioms/clean-arch.md (P0)
```

---

## Validation Expectations

| # | Scenario | Expected |
|---|----------|----------|
| V1 | Query with no keywords, no tags, no secondary config | Identical to current behavior — single workspace only |
| V2 | Query hits keyword "file-storage" mapped to framework-core | Primary results + up to 3 framework-core results, total ≤ 8 |
| V3 | Query contains `@research` tag | Tag stripped; research workspace queried; results merged |
| V4 | Query hits keyword AND has `@tag` for same workspace | Workspace queried once (deduplicated), not twice |
| V5 | Secondary workspace doesn't exist | Warning logged, that secondary skipped, retrieval continues |
| V6 | Primary fills all 8 slots | No secondary results included (primary-first) |
| V7 | Primary returns 3 results, two secondaries triggered | Remaining 5 slots split: min(secondary.limit, remaining) each |
| V8 | Evidence pipeline operations | Completely unaffected — still single-workspace strict |
| V9 | Project without `secondary_workspaces` in zbrain.json | Works exactly as before (backward compat) |

---

## Key Decisions

| ID | Decision | Rationale | Alternatives Rejected |
|----|----------|-----------|----------------------|
| D10 | Per-project secondary config (not global) | Cross-workspace access should be intentional per project, not a blanket policy. A global graph would erode isolation by default. | **Global workspace graph in config.yml** — rejected: too broad, harder to reason about per-project access |
| D11 | Keyword triggers + @tag (not keyword-only or tag-only) | Keywords reduce friction for known patterns; tags give escape hatch for ad-hoc. Either alone is incomplete. | **Keyword-only** — rejected: no way to do ad-hoc cross-workspace queries. **Tag-only** — rejected: too much friction for repeated patterns |
| D12 | Primary-first merge (not interleaved by score) | Respects workspace ownership; secondary is supplementary. Interleaving by raw BM25 score would let secondary dominate on common terms. | **Interleave by score** — rejected: secondary results could outrank primary on generic terms, defeating the "this project belongs to workspace X" intent |
| D13 | Warning + skip for missing secondary (not hard error) | A stale config entry shouldn't break retrieval. The primary workspace always works; secondaries are best-effort. | **Hard error** — rejected: too fragile for a supplementary feature |
| D14 | New `query-parser.ts` module (not inline in retrieval.ts) | Tag extraction and keyword matching are testable units independent of retrieval orchestration. | **Inline in retrieval.ts** — rejected: harder to test, mixes parsing with orchestration |

---

## Deferred Ideas

- **Workspace alias** (`@fw` → `framework-core`): convenience shorthand for long workspace names
- **Wildcard keywords** (`file-*`): glob-style keyword matching
- **Secondary workspace priority** (P0 from secondary ranks above P2 from primary): cross-workspace tier interleaving
- **`zbrain workspace link`** CLI command: interactive way to add secondary workspaces to a project
- **Automatic keyword suggestion**: scan workspace content to suggest keywords

---

## Risks

| Risk | Severity | Mitigation |
|------|----------|-----------|
| Keyword false positives (common word triggers wrong workspace) | Medium | Keywords should be specific terms, not generic words. User controls the list. |
| Performance: N+1 qmd queries per retrieval | Low | Max 2-3 secondaries typical; qmd BM25 is fast (~10ms per query) |
| Stale keyword config as workspace content evolves | Low | Keywords are user-maintained; no auto-sync needed for MVP |
| current-task.md format change breaks downstream consumers | Low | Additive change (new column); existing parsers that don't expect it won't break |

---

## Done When

- [ ] `projectPointerSchema` accepts `secondary_workspaces` array (optional)
- [ ] `@workspace` tags extracted from query and workspace added to secondary set
- [ ] Keyword matching against `secondary_workspaces[].keywords` works case-insensitively
- [ ] `retrieveMultiWorkspaceContext()` queries primary + matched secondaries
- [ ] Merge logic: primary fills first, secondaries fill remaining up to total limit
- [ ] `current-task.md` shows workspace provenance on every result
- [ ] Missing secondary workspace logs warning and skips (no crash)
- [ ] All validation scenarios (V1–V9) pass as unit tests
- [ ] Existing tests still pass (backward compat)

---

## Next Steps

1. → Run `/plan` to generate implementation phases
2. → Implement wave-by-wave

Classification: change-request · normal lane · downstream: plan full
