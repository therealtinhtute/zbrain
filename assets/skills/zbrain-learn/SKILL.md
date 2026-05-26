---
name: zbrain:learn
description: Run the four-stage evidence pipeline (ingest → analyze → qa → apply) for the active workspace. Use when ingesting new material — articles, books, notes, code snapshots.
disable-model-invocation: true
version: "1.0.0"
---

<role>
Act as an evidence pipeline driver. Move a learning item through four stages: ingest, analyze, qa, apply. Stop at any gate failure instead of proceeding.
</role>

<security>
- Never modify raw.md or source.yaml after ingest
- Never apply facts that have unresolved P0 or P1 QA questions
- Never mix evidence across workspaces
- Never expose source content from one workspace to another
</security>

<instructions>
## Stage Dispatch

Run the stage matching the argument or prompt the user to choose:

| Argument | Stage | Action |
|----------|-------|--------|
| (none) | ingest | Create immutable source files |
| `--analyze {id}` | analyze | Generate structured notes and questions |
| `--qa {id}` | qa | Resolve questions, build verified-facts.md |
| `--apply {id}` | apply | Update workspace knowledge and reindex |

See `references/pipeline.md` for detailed per-stage flows, state machine, and QA gate rules.

## Cross-Stage Invariants

- `raw.md` and `source.yaml` are immutable after ingest.
- `workspace_at_ingest` must match the active workspace at every stage transition.
- Apply stops if any P0 or P1 question is `awaiting_external` or `deferred`.
- Every verified fact must cite `question_id` and the target wiki file path.
</instructions>

<references>
Load as needed from `{baseDir}/references/`:
- `pipeline.md` — per-stage flows, state machine, QA gate rules
</references>
