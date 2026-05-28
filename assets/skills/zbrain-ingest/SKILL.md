---
name: zbrain:ingest
description: Run the four-stage evidence pipeline (ingest → analyze → qa → apply) for existing material — files, project directories, URLs, or pasted text. Use when you have a source in hand and want to add it to the active workspace.
disable-model-invocation: true
version: "1.0.0"
---

<role>
Act as an evidence pipeline driver. Move existing material through four stages: ingest, analyze, qa, apply. Stop at any gate failure instead of proceeding.
</role>

<security>
- Never modify raw.md or source.yaml after ingest
- Never apply facts that have unresolved P0 or P1 QA questions
- Never mix evidence across workspaces
- Never expose source content from one workspace to another
</security>

<instructions>
## Stage Dispatch

Run the stage matching the argument, or run stage 1 if no argument is given:

| Argument | Stage | Action |
|----------|-------|--------|
| (none) | ingest | Interactive: prompt for source text, file path, or URL |
| `--from-project {path}` | ingest (batch) | Walk project directory, identify key files, create one evidence item per logical chunk |
| `--analyze {id}` | analyze | Generate structured notes and questions |
| `--qa {id}` | qa | Resolve questions, build verified-facts.md |
| `--apply {id}` | apply | Update workspace knowledge and reindex |

See `references/pipeline.md` for detailed per-stage flows, state machine, and QA gate rules.

## --from-project Behavior

When `--from-project {path}` is given:

1. Walk the project directory. Identify files matching these priorities:
   - P0 (always ingest): `README.md`, `CLAUDE.md`, `AGENTS.md`, architecture docs, spec files
   - P1 (ingest if present): `docs/`, `wiki-spec.md`, ADR files, design docs
   - P2 (skip unless small): source files, configs, lock files
2. For each identified file, create one evidence item via stage 1 ingest.
3. Report a summary: N items created, list of IDs and titles.
4. Do not auto-advance to analyze — use `zbrain:state` to see next actions.

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
