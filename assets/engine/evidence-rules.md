# Evidence Rules

## Pipeline

1. ingest
2. analyze
3. qa
4. apply

## Skill Routing

| Source type | Entry skill |
|-------------|------------|
| Existing file, URL, or pasted text | `zbrain:ingest` |
| Existing project directory | `zbrain:ingest --from-project {path}` |
| New topic to research | `zbrain:learn "topic"` → routes to `zbrain:ingest` |
| Post-task reflection | `zbrain:reflect` → routes to `zbrain:ingest` |
| Check pipeline progress | `zbrain:state` |

## Required Guards

- Immutable source files after ingest
- Workspace lock at every transition
- QA gate before apply
- Citation coverage for every verified fact
- Checkpoint-based resume during apply
