# Retrieval Rules

## Pipeline

1. Parse task intent into search keywords.
2. Query qmd BM25 for the active workspace collection only.
3. Re-rank results by path tier before handing them to the main agent.
4. Materialize ranked context into `current-task.md`.

## Ranking

- `axioms/` -> P0
- `mental-models/` -> P1
- `projects/` -> P2
- `decisions/` -> P3

## Failure Mode

If no adequate context is found, report the gap and stop.
