---
name: zbrain:ask
description: Retrieve ranked workspace context for one question.
argument-hint: "[question]"
---

# zbrain:ask

## Purpose

Retrieve ranked context for one question from the active workspace only.

## Inputs

- `question`: free-form question text
- active workspace from `<cwd>/.claude/zbrain.json` or `~/.zbrain/config.yml`

## Flow

1. Parse the question into workspace-scoped search keywords.
2. Query qmd BM25 against the active workspace collection only.
3. Re-rank results by tier:
   - P0 `axioms/`
   - P1 `mental-models/`
   - P2 `projects/`
   - P3 `decisions/`
4. Write the ranked context into `current-task.md`.
5. Stop if the result set has a knowledge gap instead of guessing.

## Invariants

- Never query another workspace collection.
- Never answer without retrieved context.
- Always preserve citation paths in the generated context file.
