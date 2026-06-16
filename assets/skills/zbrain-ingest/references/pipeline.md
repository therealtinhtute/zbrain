# Evidence Pipeline

## State Machine

```
ingested → analyzed → qa_in_progress → qa_done → applied → archived
                             ↕
                   qa_awaiting_external
```

Only `qa_done → qa_in_progress` is a valid backward transition.

## Stage 1: Learn

1. Resolve active workspace from `~/.zbrain/projects.json` by matching the current project root, or fallback to `~/.zbrain/config.yml`.
2. Generate a short unique evidence ID: `YYYYMMDD-{slug}`.
3. Create `evidence/sources/{id}/` directory.
4. Write raw content as `raw.md` — **immutable after this step**.
5. Write `source.yaml` with: `id`, `title`, `source_type`, `workspace_at_ingest`, `ingested_at`, `state: ingested`.
6. Append entry to `evidence/_index.md`.

## Stage 2: Analyze

1. Read `raw.md` (read-only — do not modify).
2. Run four structured analysis passes against the raw content:
   - Domain and concept mapping
   - Entity and relationship extraction
   - Pattern candidates (reusable frameworks or mental models)
   - Fact candidates with inline citation references
3. Write output to `evidence/analysis/{id}/analysis.md`.
4. Update `_index.md` state → `analyzed`.

## Stage 3: QA

1. Read `analysis/{id}/analysis.md`.
2. Generate a prioritized question batch:
   - P0 — blocking: must resolve before apply
   - P1 — important: should resolve before apply
   - P2 — nice-to-have: can defer
   - P3 — optional
3. Present questions to the user; record answers.
4. Write resolved answers to `evidence/qa/{id}/verified-facts.md`.
   - Every entry must cite `question_id` and a target wiki file path.
5. Gate check: if any P0 or P1 question is `awaiting_external` or `deferred`, stop.
6. Update `_index.md` state → `qa_done`.

## Stage 4: Apply

1. Read `verified-facts.md` — verify all P0/P1 questions are resolved.
2. Read `source.yaml` — note the `origin` field; this is the canonical `resource` URL.
3. For each verified fact:
   - Locate or create the target wiki file (`axioms/`, `mental-models/`, `projects/`, or `decisions/`).
   - When creating a new wiki file, populate `resource:` from `source.yaml.origin`.
   - When updating an existing file, add `resource:` to frontmatter if it is missing or empty.
   - Append the fact with citation.
   - Record the change in `evidence/applied/{id}/manifest.yaml`.
4. For each directory that received new or updated wiki files, create or update `index.md` listing all `.md` files in that directory (one entry per file, filename as a relative markdown link).
5. Preserve `raw.md` and `source.yaml` as immutable throughout.
6. Trigger internal reindexing to update the BM25 index for the workspace.
7. Update `_index.md` state → `applied`.

## QA Gate Rules

| Priority | `awaiting_external` | `deferred` |
|----------|---------------------|------------|
| P0 | Block apply | Block apply |
| P1 | Block apply | Block apply |
| P2 | Warn, allow | Allow |
| P3 | Allow | Allow |
