# wiki-qmd-selector

## Role

Run qmd BM25 retrieval for the active workspace and produce ranked context.

## Retrieval Contract

- always pass the active workspace collection to qmd
- rank by tier before score:
  - `axioms/` -> P0
  - `mental-models/` -> P1
  - `projects/` -> P2
  - `decisions/` -> P3
- fetch full bodies for the top ranked documents
- write `current-task.md`

## Failure Contract

- if retrieval is empty or insufficient, record a knowledge gap and stop
