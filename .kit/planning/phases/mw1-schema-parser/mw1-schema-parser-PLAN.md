# Plan: Schema + Query Parser

Phase: mw1-schema-parser
Status: ready
Wave Count: 3
Execution Owner: work
Updated At: 2026-05-28

## Goal
Extend project pointer schema and build query parser foundations for multi-workspace retrieval.

## Inputs
- `src/schemas/config.ts` — current Zod schemas
- `src/core/workspace-resolver.ts` — workspace validation pattern
- `.kit/planning/multi-workspace-context.md` — spec (R1, R2, R3, R7, I-7, I-9)

## Wave 1
### T1 — Extend projectPointerSchema with secondary_workspaces
- type: implementation
- inputs:
  - `src/schemas/config.ts`
- touches:
  - `src/schemas/config.ts`
- avoid:
  - `src/core/config.ts` (no reader changes needed — passthrough handles it)
  - any retrieval or evidence files
- steps:
  1. Add `secondaryWorkspaceEntrySchema` — Zod object: `workspace` (string, required), `keywords` (array of non-empty strings, required), `limit` (positive integer, optional, default 3)
  2. Add `secondary_workspaces` as optional array of `secondaryWorkspaceEntrySchema` to `projectPointerSchema`
  3. Export `SecondaryWorkspaceEntry` type
- expected outputs:
  - `projectPointerSchema` accepts `{ workspace: "x", secondary_workspaces: [...] }`
  - `projectPointerSchema` still accepts `{ workspace: "x" }` without secondary field
- verification:
  - `bun test tests/core/config.test.ts`
- stop if:
  - existing config tests fail after schema change
- escalate to:
  - plan phase (schema approach may need rethinking)

### T2 — Unit tests for extended schema
- type: test
- inputs:
  - T1 output (extended schema)
- touches:
  - `tests/core/config.test.ts`
- avoid:
  - creating new test files (extend existing)
- steps:
  1. Add test: parses pointer with valid `secondary_workspaces` array
  2. Add test: parses pointer without `secondary_workspaces` (backward compat)
  3. Add test: rejects `secondary_workspaces` with empty workspace name
  4. Add test: rejects `secondary_workspaces` with empty keywords array
  5. Add test: accepts `secondary_workspaces` entry without explicit `limit` (defaults to 3)
- expected outputs:
  - 5 new test cases passing
- verification:
  - `bun test tests/core/config.test.ts`
- stop if:
  - backward compat test fails
- escalate to:
  - plan phase

## Wave 2
### T3 — Create query-parser.ts
- type: implementation
- inputs:
  - `SecondaryWorkspaceEntry` type from T1
- touches:
  - `src/core/query-parser.ts` (new file)
- avoid:
  - retrieval.ts, current-task.ts
- steps:
  1. Create `src/core/query-parser.ts`
  2. Implement `extractWorkspaceTags(query: string)` → `{ tags: string[], cleanQuery: string }` — regex `/@([a-zA-Z0-9_-]+)/g`, strip matched tags from query
  3. Implement `matchKeywordWorkspaces(query: string, secondaries: SecondaryWorkspaceEntry[])` → `string[]` — case-insensitive substring match of each secondary's keywords against the query, return deduplicated workspace names
  4. Implement `parseQuery(query: string, secondaries: SecondaryWorkspaceEntry[])` → `{ cleanQuery: string, secondaryWorkspaces: string[] }` — combines tag extraction + keyword matching, deduplicates workspace names
- expected outputs:
  - `src/core/query-parser.ts` with 3 exported functions
- verification:
  - `bun test tests/core/query-parser.test.ts` (written in T4)
- stop if:
  - regex doesn't handle edge cases (empty query, tag at start/end, adjacent tags)
- escalate to:
  - user clarification (if keyword matching rules need refinement)

### T4 — Unit tests for query-parser.ts
- type: test
- inputs:
  - T3 output
- touches:
  - `tests/core/query-parser.test.ts` (new file)
- avoid:
  - integration-level tests (those go in phase 3)
- steps:
  1. Test `extractWorkspaceTags`: query with one tag, multiple tags, no tags, tag at boundaries, tag-only query
  2. Test `matchKeywordWorkspaces`: single keyword match, multiple keywords, no match, case insensitivity, duplicate workspace dedup
  3. Test `parseQuery`: combined tag + keyword, same workspace from both sources (dedup), empty secondaries array, empty query
- expected outputs:
  - 12+ test cases covering edge cases
- verification:
  - `bun test tests/core/query-parser.test.ts`
- stop if:
  - edge case failures in tag extraction regex
- escalate to:
  - plan phase (regex approach may need adjustment)

## Wave 3
### T5 — Create secondary-resolver.ts
- type: implementation
- inputs:
  - `src/core/workspace-resolver.ts` (pattern reference)
  - `src/core/runtime-paths.ts` (RuntimePaths type)
- touches:
  - `src/core/secondary-resolver.ts` (new file)
- avoid:
  - modifying `workspace-resolver.ts` (this is additive, not a refactor)
- steps:
  1. Create `src/core/secondary-resolver.ts`
  2. Implement `resolveSecondaryWorkspaces(workspacesDir: string, names: string[])` → `{ resolved: string[], warnings: string[] }` — check each name exists as a directory under `workspacesDir`, collect warnings for missing ones
  3. Return only names that resolve to existing directories
- expected outputs:
  - `src/core/secondary-resolver.ts` with 1 exported function
- verification:
  - `bun test tests/core/secondary-resolver.test.ts` (written in T6)
- stop if:
  - workspace directory structure assumptions are wrong
- escalate to:
  - plan phase

### T6 — Unit tests for secondary-resolver.ts
- type: test
- inputs:
  - T5 output
- touches:
  - `tests/core/secondary-resolver.test.ts` (new file)
- avoid:
  - integration tests
- steps:
  1. Test: all secondary workspaces exist → all returned, no warnings
  2. Test: one missing workspace → excluded from resolved, included in warnings
  3. Test: all missing → empty resolved, all in warnings
  4. Test: empty names array → empty resolved, no warnings
  5. Test: workspacesDir doesn't exist → empty resolved, all names in warnings
- expected outputs:
  - 5 test cases
- verification:
  - `bun test tests/core/secondary-resolver.test.ts`
- stop if:
  - filesystem access patterns differ from existing resolver
- escalate to:
  - plan phase

## Risks / Watch-fors
- Zod `.passthrough()` means extra fields survive parsing, but `.strict()` would reject them — verify current schema uses passthrough
- Tag regex must not match `@` in email addresses or other non-workspace contexts — the skill/retrieval context makes this safe since queries are short knowledge questions, not prose
