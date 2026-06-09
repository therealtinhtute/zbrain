# Evidence Rules

## Pipeline

1. learn
2. ingest
3. ask

## Skill Routing

| Need | Entry skill |
|------|------------|
| Record raw source material | `zbrain:learn` |
| Analyze, QA, apply, or list evidence | `zbrain:ingest` |
| Retrieve context before answering | `zbrain:ask` |

## Required Guards

- Immutable source files after ingest
- Workspace lock at every transition
- QA gate before apply
- Citation coverage for every verified fact
- Checkpoint-based resume during apply
