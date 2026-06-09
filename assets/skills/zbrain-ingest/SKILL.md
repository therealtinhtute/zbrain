---
name: zbrain:ingest
description: Process learned evidence through list, analyze, qa, and apply. Use after zbrain:learn has created an evidence source.
disable-model-invocation: true
version: "2.0.0"
---

<role>
Act as an evidence pipeline driver. Move existing evidence from raw source to verified workspace knowledge. Stop at any gate failure instead of proceeding.
</role>

<security>
- Never modify raw.md or source.yaml after learn creates them
- Never apply facts that have unresolved P0 or P1 QA questions
- Never mix evidence across workspaces
- Never expose source content from one workspace to another
</security>

<instructions>
## Stage Dispatch

Run the stage matching the argument:

| Argument | Stage | Action |
|----------|-------|--------|
| `list` | status | Show evidence items and next actions |
| `analyze {id}` | analyze | Generate structured notes and questions |
| `qa {id}` | qa | Resolve questions and build `verified-facts.md` |
| `apply {id}` | apply | Update workspace knowledge and reindex internally |

See `references/pipeline.md` for detailed per-stage flows, state machine, and QA gate rules.

## Cross-Stage Invariants

- New raw material must enter through `zbrain:learn`, not `zbrain:ingest`.
- `raw.md` and `source.yaml` are immutable after creation.
- `workspace_at_ingest` must match the active workspace at every state transition.
- Apply stops if any P0 or P1 question is `awaiting_external` or `deferred`.
- Every verified fact must cite `question_id` and the target wiki file path.
</instructions>

<references>
Load as needed from `{baseDir}/references/`:
- `pipeline.md` - per-stage flows, state machine, QA gate rules
</references>
