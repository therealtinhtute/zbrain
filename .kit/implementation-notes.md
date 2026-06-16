# Implementation Notes: SQLite Migration (p8 + p9-core)

**Spec:** `.kit/planning/SPEC-sqlite.md`
**Phases:** p8-sqlite-core-db, p9-sqlite-session-memory (partial)
**Result:** 131/131 tests pass, 0 type errors

---

## Decisions Not in the Spec

### 1. `resolveActiveWorkspace` calls `initDb`, not `openDb`

The spec said "open the DB and query projects." `openDb` just opens the file without creating the schema. Any caller that hadn't run `zbrain setup` first got a hard crash: `SQLiteError: no such table: projects`.

Fixed by using `initDb` in `workspace-resolver.ts`. Since `initDb` uses `CREATE TABLE IF NOT EXISTS` everywhere, calling it multiple times is free. The version guard (`user_version > SCHEMA_VERSION → throw`) is also desirable here — fail loudly if the CLI is older than the DB.

**Tradeoff**: slight "auto-migrate" behavior on first use; intentional.

---

### 2. `source_sha256` hardcodes `state: "ingested"` during verification

`source_sha256` is computed at ingest time over a struct that includes `state: "ingested"`. After state transitions, the DB row's `state` column changes but `source_sha256` is fixed.

`verifyEvidenceIntegrity` reconstructs the original record by hardcoding `state: "ingested"` regardless of the current row state. This matches how the hash was originally computed and ensures integrity checks don't break after transitions.

---

### 3. `projects.json` is no longer written

The old code wrote `~/.zbrain/projects.json` on every `zbrain init`. The new `upsertProjectBinding` writes to the DB only. Any external tooling that read that file needs to use the DB instead.

Six test files had assertions against `projects.json` that all needed to be updated to use `readProject(db, path)`.

---

### 4. `source.yaml` is no longer written on ingest

`ingestEvidence` now writes only `raw.md`. All metadata goes to the DB. The `_index.md` in `evidence/` is still scaffolded by `createWorkspaceScaffold` but never updated by the pipeline.

Tests that read `source.yaml` or `_index.md` were updated to use `readEvidence(db, workspace, id)` or check `raw.md`.

---

### 5. DB parameter threading — one exception

Most core functions receive `db: Database` as their first parameter. `resolveActiveWorkspace` opens the DB internally because it's called from dozens of places and always has access to `paths.runtimeDir`. Requiring callers to pass a DB would ripple through every command.

---

## Tests Changed Beyond the Planned Scope

| File | What changed |
|------|-------------|
| `tests/core/config.test.ts` | Removed `readProjectRegistry` (now DB-based; already tested in `db-projects.test.ts`) |
| `tests/core/workspace-resolver.test.ts` | Test 1 uses DB insertion instead of writing `projects.json`; all tests work because `initDb` is called by the resolver |
| `tests/evidence/evidence-pipeline.test.ts` | Full rewrite: `initDb` in fixture, `db` param to all calls, replaced `source.yaml`/`_index.md` assertions with DB queries |
| `tests/release/acceptance.test.ts` | `db` param to evidence functions, `projects.json` check replaced with DB query |
| `tests/commands.integration.test.ts` | All `projects.json` reads → `readProject(db, path)`, `source.yaml` check → `raw.md` + DB row check |
| `tests/assets/commands.test.ts` | Added `zbrain-research` to expected skills (was added in a prior commit, test was never updated) |

---

## What Still Needs to Happen

1. **Migration script** (`scripts/migrate-to-db.ts`): reads existing `projects.json` + `source.yaml` files → seeds the DB. Not yet written.
2. **Binary smoke test**: `bun run build && ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup && ls /tmp/zbrain-smoke/zbrain.db`
3. **Phase p9 (session memory)**: `db-sessions.ts` with `upsertSession`, `insertQuery`, `listRecentQueries`; wire into `ask.ts` to persist query history.

---

# Implementation Notes: Multi-Workspace Context Loading

**Spec:** `.kit/planning/multi-workspace-context.md`
**Completed:** 2026-05-28
**Result:** 102/102 tests pass, 0 type errors

---

## Decisions Not in the Spec

### 1. @tag keyword matching uses the original query, not the clean query

**Spec says**: "Parse @workspace tags from query → add to secondary set, strip from query text" then "Match remaining query against keyword maps."

**What I implemented**: Keyword matching uses the original query (with @tags still present), not the clean query. The tag stripping happens only to produce `cleanQuery` for BM25 search.

**Why**: The phrase "remaining query" in the spec is ambiguous. Using the original query is more permissive and there's no realistic case where an @tag would accidentally trigger a keyword match in another workspace — workspace names are generally different from domain keywords.

**If you want the other behavior**: Change `matchKeywordWorkspaces(query, secondaries)` in `query-parser.ts` to receive `cleanQuery` instead.

---

### 2. @tag-only workspaces (not in secondary_workspaces config) get `DEFAULT_SECONDARY_LIMIT = 3`

**Spec says**: Users can write `@workspace` to include any workspace. Config entries have a `limit` field.

**Gap**: If a user writes `@some-workspace` but that workspace is not in `secondary_workspaces` config, there's no configured limit.

**What I implemented**: A module-level constant `DEFAULT_SECONDARY_LIMIT = 3` applies to any @tag-triggered workspace without a config entry. This matches the config schema default (`.default(3)` on the Zod field).

---

### 3. Slot allocation: `Math.floor(remaining / remainingSecondaries) || 1`

**Spec says**: "Each secondary workspace gets min(its own limit, remaining / number of secondaries) slots."

**Edge case not addressed in spec**: If `Math.floor(remaining / remainingSecondaries) = 0` (e.g. 1 remaining slot, 3 secondaries), the formula would give 0 slots to all secondaries. That means none get queried even though there's a free slot.

**What I implemented**: `Math.floor(remaining / remainingSecondaries) || 1` — the `|| 1` ensures at least 1 slot is given when there's any remaining capacity at all. The first secondary gets the slot, subsequent ones get 0 and are skipped.

**Tradeoff**: Slightly favors the first-configured secondary over later ones when slots are very tight. This is predictable (config-order priority) and explicit.

---

### 4. Secondary workspace deduplication happens in parseQuery, not in retrieveMultiWorkspaceContext

**Spec says**: "if same workspace triggered by keyword AND @tag, include once."

**Where deduplication lives**: `parseQuery()` in `query-parser.ts` uses `[...new Set([...tags, ...keywordMatches])]`. This deduplicates before any resolver or retrieval happens.

**Why here**: Cleaner to have one authoritative list of unique secondary workspace names emerge from `parseQuery` rather than deduplicating in the retrieval orchestration.

---

### 5. Primary workspace is NOT excluded from secondaries list

**What I did not add**: No filter to exclude the primary workspace if it appears in secondaries (e.g. `secondary_workspaces: [{workspace: "ttdvkh", ...}]` where `ttdvkh` is also the primary).

**Why**: The spec doesn't mention this case. It would be an unusual config. The current behavior: primary is queried once normally, then if it also appears in secondaries, it gets queried again. This is slightly wasteful but not harmful — the duplicate results would just land in the merge output, and the user would see them as they configured.

**If this becomes a problem**: Filter out the primary workspace from `resolved` list in `retrieveMultiWorkspaceContext` before the secondary loop.

---

### 6. Slot allocation preserves config entry order

**Spec says**: "secondary results fill remaining slots, allocated in config order."

**What I implemented**: The `resolved` array from `resolveSecondaryWorkspaces` preserves insertion order of `names`, which comes from `parseQuery`'s `secondaryWorkspaces` output. `parseQuery` uses `[...new Set([...tags, ...keywordMatches])]` — tags come first (in order of appearance), then keyword matches (in config order).

**Result**: @tags have priority over keyword matches in slot allocation when both appear. Within each group, order is preserved.

---

### 7. current-task.md "multi-workspace" detection uses result.workspace field, not secondaryWorkspaces list

**Spec says**: Show workspace column and labels when multi-workspace results are present.

**Detection logic**: `const hasMultiWorkspace = results.some((r) => r.workspace !== undefined)` — checks actual results, not the `secondaryWorkspaces` input field.

**Why**: If secondaries are configured but all return empty results, there's nothing multi-workspace to display. Checking the results array is the source of truth. A non-empty `secondaryWorkspaces` header is still shown (it lists which were queried), but the table stays single-column.

---

### 8. Primary results have workspace = undefined (not set to primaryWorkspace)

**Design choice**: Primary results are not tagged with `workspace: primaryWorkspace`. Only secondary results get a `workspace` tag.

**Why**: Backward compat. Existing code that creates `RankedRetrievalResult` objects doesn't set `workspace`. Setting it for primary would be a change with wider blast radius.

**In current-task.ts**: When displaying, `r.workspace ?? workspace` resolves to the primary workspace name for results without a workspace tag, so labels are still correct in multi-workspace display.

---

## Things I Changed vs. the Plan

### Slot allocation formula

**Plan (T8, step f)**: `min(entry.limit, remaining / remainingSecondaries)` with no `|| 1` fallback.

**Actual**: Added `|| 1` guard for zero-slot edge case (see Decision #3 above).

### current-task.ts "Secondary Workspaces" header only when resolvedSecondaries is non-empty

**Plan**: Add `Secondary Workspaces:` header when `secondaryWorkspaces` is present.

**Actual**: In `retrieveMultiWorkspaceContext`, I pass `secondaryWorkspaces: resolvedSecondaries` only when `resolved.length > 0`. If secondaries were configured but none resolved (all missing dirs), the header is omitted — no point listing workspaces that weren't queried.

---

## Files Created / Modified

| File | Type |
|------|------|
| `src/schemas/config.ts` | Modified — added `secondaryWorkspaceEntrySchema`, extended `projectPointerSchema` |
| `src/core/query-parser.ts` | New |
| `src/core/secondary-resolver.ts` | New |
| `src/core/retrieval-ranking.ts` | Modified — added optional `workspace?: string` to `RankedRetrievalResult` |
| `src/core/retrieval.ts` | Modified — added `retrieveMultiWorkspaceContext` and `MultiWorkspaceRetrievalOptions` |
| `src/core/current-task.ts` | Modified — conditional workspace column + section labels |
| `tests/core/config.test.ts` | Modified — 5 new schema tests |
| `tests/core/query-parser.test.ts` | New — 15 test cases |
| `tests/core/secondary-resolver.test.ts` | New — 5 test cases |
| `tests/retrieval/current-task.test.ts` | Modified — 2 new tests (backward compat + multi-workspace display) |
| `tests/retrieval/retrieval.integration.test.ts` | Modified — 4 new orchestration tests |
| `tests/retrieval/multi-workspace.integration.test.ts` | New — V1–V9 scenarios |
