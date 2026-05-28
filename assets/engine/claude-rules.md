## zbrain Integration

zbrain is a workspace-isolated knowledge retrieval layer. Skills live in `.claude/skills/zbrain-*`.
Runtime root: `~/.zbrain/`. Project pointer: `<cwd>/.claude/zbrain.json`.

### Workspace Resolution

1. Read `<cwd>/.claude/zbrain.json` → `workspace` field (highest priority).
2. Fallback: `~/.zbrain/config.yml` → `default_workspace`.
3. If neither resolves, stop and report — never guess a workspace.

### Skill Triggers

| When you need to… | Use |
|--------------------|-----|
| Answer domain questions (architecture, decisions, patterns) | `zbrain:ask` |
| Add a file, URL, or pasted text to the workspace | `zbrain:ingest` |
| Research a topic online and ingest results | `zbrain:learn` |
| Check evidence pipeline status | `zbrain:state` |
| See or switch the active workspace | `zbrain:workspace` |
| Rebuild the search index after bulk changes | `zbrain:reindex` |

**Before answering any question about domain knowledge, project decisions, or architectural patterns — invoke `zbrain:ask` first. Never answer from memory.**

### Retrieval Tier Priority

`axioms/` (P0) → `mental-models/` (P1) → `projects/` (P2) → `decisions/` (P3)

Higher-tier results rank first regardless of BM25 score.

### Evidence Pipeline

Each piece of external material moves through four stages:

```
ingest → analyze → qa → apply
```

Use `zbrain:state` to see which stage each item is in and what command runs next.
**Never advance to apply if any P0 or P1 question is unresolved.**

### Secondary Workspaces (optional)

`zbrain.json` supports a `secondary_workspaces` array for cross-workspace context.
Each entry has `workspace`, `keywords`, and optional `limit`.
Secondary results fill remaining slots after primary results.

### Invariants

- **Cite all retrieved context.** Never answer domain questions from memory.
- **One workspace per query.** Never cross workspace boundaries in a single retrieval.
- **Evidence is immutable after ingest.** Never edit `raw.md` or `source.yaml`.
- **Apply gate.** Block apply if any P0 or P1 QA question is `awaiting_external`.
