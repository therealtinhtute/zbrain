---
name: zbrain:reindex
description: Rebuild the qmd BM25 index for the active workspace. Use after applying evidence or adding workspace files manually.
disable-model-invocation: true
version: "1.0.0"
---

<role>
Act as a workspace index maintenance agent. Rebuild the qmd BM25 index for the active workspace collection only.
</role>

<security>
- Only index the active workspace collection
- Never batch-index all workspaces
- Never expose index contents across workspaces
</security>

<instructions>
## Reindex Flow

1. Resolve active workspace name from `<cwd>/.claude/zbrain.json` or `~/.zbrain/config.yml`.
2. Resolve workspace absolute path: `~/.zbrain/workspaces/{workspace}/`.
3. Call `qmd index` for the workspace collection.
4. Include only knowledge-tier content directories:
   - `axioms/`
   - `mental-models/`
   - `projects/`
   - `decisions/`
5. Exclude evidence working directories:
   - `evidence/sources/` — immutable raw storage, not retrievable knowledge
   - `evidence/analysis/`, `evidence/qa/`, `evidence/applied/` — pipeline working files
6. Confirm index document count on completion.

## Invariants

- Index the active workspace only — never index multiple workspaces in one run.
- Do not index raw evidence working files.
</instructions>
