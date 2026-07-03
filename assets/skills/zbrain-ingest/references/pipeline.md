# Evidence Pipeline

## State Machine

```
ingested → reviewed → applied → archived
```

## Stage 1: Learn

1. Resolve active workspace by running `zbrain workspace current` (JSON output), or fallback to `~/.zbrain/config.yml`.
2. Generate a short unique evidence ID: `YYYYMMDD-{slug}`.
3. Create `evidence/sources/{id}/` directory.
4. Write raw content as `raw.md` — **immutable after this step**.
5. Write `source.yaml` with: `id`, `title`, `source_type`, `workspace_at_ingest`, `ingested_at`, `state: ingested`.

## Stage 2: Review

Invoked as `zbrain:ingest {id}`.

1. Read `raw.md` (read-only — do not modify).
2. Extract facts from the raw content:
   - Identify the core claim or assertion the source makes.
   - For each distinct fact, note which wiki tier it belongs to (`axioms/`, `mental-models/`, `projects/`, or `decisions/`).
   - Ask a single focused question for any fact where the target wiki path is ambiguous.
3. Present extracted facts to the user for confirmation.
4. Write confirmed facts to `evidence/qa/{id}/verified-facts.md`.
   - Every entry must cite `question_id` and a target wiki file path.
5. Gate check: surface only P0 (blocking) concerns. If a fact's accuracy cannot be confirmed from the source alone and the user cannot answer it now, mark it `awaiting_external` and stop.
6. Transition state `ingested → reviewed`.
7. Output: `→ Next: zbrain:ingest apply {id}`

## Stage 3: Apply

Invoked as `zbrain:ingest apply {id}`.

1. Read `verified-facts.md` — verify no P0/P1 questions are `awaiting_external` or `deferred`.
2. Read `source.yaml` — note the `origin` field; this is the canonical `resource` URL.
3. For each verified fact:
   - Locate or create the target wiki file (`axioms/`, `mental-models/`, `projects/`, or `decisions/`).
   - When creating a new wiki file, populate `resource:` from `source.yaml.origin`.
   - When updating an existing file, add `resource:` to frontmatter if missing or empty.
   - Append the fact with citation.
   - Record the change in `evidence/applied/{id}/manifest.yaml`.
4. For each directory that received new or updated wiki files, create or update `index.md` listing all `.md` files in that directory.
5. Preserve `raw.md` and `source.yaml` as immutable throughout.
6. Trigger internal reindexing to update the BM25 index for the workspace.
7. Transition state `reviewed → applied`.
8. Output: `→ Next: zbrain:ask "your question"` to query the updated workspace.

## Gate Rules

| Priority | `awaiting_external` | `deferred` |
|----------|---------------------|------------|
| P0 | Block apply | Block apply |
| P1 | Block apply | Block apply |
| P2 | Warn, allow | Allow |
| P3 | Allow | Allow |
