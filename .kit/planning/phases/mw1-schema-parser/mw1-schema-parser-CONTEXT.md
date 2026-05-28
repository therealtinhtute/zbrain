# Context: Schema + Query Parser

Phase: mw1-schema-parser
Status: ready
Spec Link: ../../multi-workspace-context.md
Roadmap Link: ../../ROADMAP-multi-workspace.md
Blast Radius: low
Expected Proof: unit

## Goal
Extend the project pointer schema to accept `secondary_workspaces` and build the query parser that extracts `@workspace` tags and matches keywords against the config. This is the foundation — no retrieval changes yet.

## Scope Boundary
### Allowed Surfaces
- `src/schemas/config.ts` — add `secondaryWorkspaceSchema` and extend `projectPointerSchema`
- `src/core/query-parser.ts` — new file: `extractWorkspaceTags()`, `matchKeywords()`, `parseQuery()`
- `src/core/secondary-resolver.ts` — new file: `resolveSecondaryWorkspaces()` validates workspace dirs exist
- `tests/core/config.test.ts` — extend with secondary_workspaces parsing tests
- `tests/core/query-parser.test.ts` — new test file
- `tests/core/secondary-resolver.test.ts` — new test file

### Forbidden Surfaces
- `src/core/retrieval.ts` — no retrieval changes in this phase
- `src/core/current-task.ts` — no output format changes yet
- `src/core/qmd-adapter.ts` — no search changes
- Any evidence pipeline files
- Any CLI command files

## Spec Hooks
- R1 (secondary workspace config), R7 (zbrain.json schema)
- R2 (keyword auto-trigger), R3 (@workspace manual tag)
- I-7 (no silent cross-workspace — only explicit config/tags)
- I-9 (secondary validation — warning + skip for missing)

## Locked Decisions
- `secondary_workspaces` is optional on `projectPointerSchema` (backward compat)
- Keywords matched case-insensitively via `.toLowerCase().includes()` — whole-query substring, not word-boundary
- `@workspace` regex: `/@([a-zA-Z0-9_-]+)/g` — extracted and stripped from query before BM25
- `resolveSecondaryWorkspaces()` returns only workspaces that exist on disk; logs warning for missing ones (does not throw)
- Secondary workspace deduplication: if same workspace triggered by keyword AND @tag, include once

## Assumptions
- `projectPointerSchema` uses `.passthrough()` so unknown fields are already tolerated — adding `secondary_workspaces` as optional won't break existing consumers
- Workspace names use only `[a-zA-Z0-9_-]` characters (matches existing `readdirSync` usage)

## Canonical Refs
- `src/schemas/config.ts:1-17` — current Zod schemas
- `src/core/workspace-resolver.ts:27-41` — `assertWorkspaceExists` pattern to reuse
- `tests/core/config.test.ts` — existing test patterns

## Rejected Options
- **Glob/regex keywords** (e.g., `file-*`): adds complexity for unclear benefit at this stage. Deferred.
- **Workspace aliases** (`@fw` → `framework-core`): neat but not required for MVP. Deferred.
- **Word-boundary matching**: `\bkeyword\b` regex is more precise but fails on hyphenated compound words like "file-storage". Substring match is simpler and good enough.

## Deferred Ideas
- Workspace alias mapping
- Glob keyword patterns
- Keyword suggestions from workspace content analysis

## Escalate If
- Zod schema extension breaks existing `parseProjectPointer()` callers (indicates backward compat issue → re-examine schema approach)
- Workspace name format assumptions prove wrong (names with dots, spaces, unicode)
