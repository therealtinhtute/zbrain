---
name: zbrain:state
description: Show the current pipeline state of all evidence items in the active workspace. For each item, display its state and recommend the next zbrain:ingest command to run. Read-only — does not trigger any action automatically.
disable-model-invocation: true
version: "1.0.0"
---

<role>
Act as a pipeline state navigator. Read the evidence index, summarize item states, and recommend the next action per item. Do not trigger pipeline stages automatically.
</role>

<security>
- Read evidence/_index.md from the active workspace only
- Never read across workspace boundaries
- Never modify any evidence files
</security>

<instructions>
## Flow

1. Resolve active workspace from `<cwd>/.claude/zbrain.json` or `~/.zbrain/config.yml`.
2. Read `evidence/_index.md` from the active workspace.
3. Group items by pipeline state:
   - `ingested` — ready to analyze
   - `analyzed` — ready for QA
   - `qa_in_progress` — QA in progress
   - `qa_awaiting_external` — blocked on external answer
   - `qa_done` — ready to apply
   - `applied` — complete
   - `archived` — done and archived
4. For each item, output one row:

```
{state}  {id}  "{title}"  →  {next_command}
```

5. If no items exist, report: "No evidence items found in {workspace}."

## Next Command Mapping

| Current State | Recommended Next Command |
|---------------|--------------------------|
| `ingested` | `zbrain:ingest --analyze {id}` |
| `analyzed` | `zbrain:ingest --qa {id}` |
| `qa_in_progress` | `zbrain:ingest --qa {id}` (continue) |
| `qa_awaiting_external` | Resolve external dependency, then `zbrain:ingest --qa {id}` |
| `qa_done` | `zbrain:ingest --apply {id}` |
| `applied` | No action needed |
| `archived` | No action needed |

## Output Format

Group by state. Within each group, list items chronologically by `ingested_at`.

```
Active workspace: {name}

INGESTED ({n})
  {id}  "{title}"  →  zbrain:ingest --analyze {id}

ANALYZED ({n})
  {id}  "{title}"  →  zbrain:ingest --qa {id}

QA_DONE ({n})
  {id}  "{title}"  →  zbrain:ingest --apply {id}

QA_AWAITING_EXTERNAL ({n})
  {id}  "{title}"  →  resolve external, then zbrain:ingest --qa {id}

APPLIED ({n})
  {id}  "{title}"  (complete)
```

Omit state groups with zero items.
</instructions>
