# ROADMAP: Multi-Workspace Context Loading

## Planning Basis
- source spec: `.kit/planning/multi-workspace-context.md`
- planning mode: `full`
- entry phase: `mw1-schema-parser`
- execution mode: sequential (3 phases, each unlocks the next)

---

## Phase 1: mw1-schema-parser
**Goal:** Extend the project pointer schema and build the query parser that extracts `@workspace` tags and matches keywords — the two input mechanisms for secondary workspace selection.

**Deliverables:**
- Extended `projectPointerSchema` in `src/schemas/config.ts` with optional `secondary_workspaces` array
- New `src/core/query-parser.ts` module: `@workspace` tag extraction + keyword matcher
- New `src/core/secondary-resolver.ts`: validate secondary workspace names, skip missing with warning
- Unit tests for schema, tag parsing, keyword matching, secondary resolution

**Dependencies:**
- Existing `src/schemas/config.ts` (Zod schema)
- Existing `src/core/workspace-resolver.ts` (validation pattern)

**Risks / Watch-fors:**
- Schema extension must be backward compatible (`.passthrough()` on Zod already handles unknown fields, but `secondary_workspaces` must be optional)
- Keyword matching must be case-insensitive and handle partial word boundaries carefully (substring vs. word match)

---

## Phase 2: mw2-retrieval-orchestration
**Goal:** Wire query parser + secondary resolver into the retrieval pipeline. Implement primary-first merge strategy and workspace provenance labels in `current-task.md`.

**Deliverables:**
- New `retrieveMultiWorkspaceContext()` in `src/core/retrieval.ts`
- Updated `current-task.ts` to include workspace column and labels
- Merge logic: primary fills first, secondaries fill remaining up to total limit

**Dependencies:**
- Phase 1 outputs: `query-parser.ts`, `secondary-resolver.ts`, extended schema
- Existing `src/core/retrieval.ts`, `src/core/current-task.ts`, `src/core/qmd-adapter.ts`

**Risks / Watch-fors:**
- `current-task.md` format change is additive but downstream consumers (skills reading this file) should still work
- Multiple secondaries sharing remaining slots — need clear proportional allocation

---

## Phase 3: mw3-integration-proof
**Goal:** End-to-end integration tests proving all 9 validation scenarios (V1–V9) from the spec, plus backward compatibility with existing single-workspace behavior.

**Deliverables:**
- Integration test file `tests/retrieval/multi-workspace.integration.test.ts`
- All V1–V9 scenarios passing
- Existing test suite still green (no regressions)

**Dependencies:**
- Phase 2 complete (full retrieval pipeline wired)
- Existing test infrastructure (`bun:test`, temp dirs, injected adapters)

**Risks / Watch-fors:**
- V6 (primary fills all slots) edge case needs careful limit arithmetic
- V7 (proportional slot sharing) needs deterministic tie-breaking
