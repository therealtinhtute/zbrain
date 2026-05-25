# Evidence Rules

## Pipeline

1. ingest
2. analyze
3. qa
4. apply

## Required Guards

- Immutable source files after ingest
- Workspace lock at every transition
- QA gate before apply
- Citation coverage for every verified fact
- Checkpoint-based resume during apply
