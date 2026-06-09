## zbrain Integration

zbrain is a workspace-isolated knowledge retrieval layer. Skills live in `.claude/skills/zbrain-*`.
Runtime root: `~/.zbrain/`. Project registry: `~/.zbrain/projects.json`.

### Workspace Resolution

1. Read `~/.zbrain/projects.json`.
2. Find the entry whose `project_root` matches the current project root.
3. Use that entry's `workspace` and `context_file`.
4. Fallback: `~/.zbrain/config.yml` → `default_workspace`.
5. If neither resolves, stop and report — never guess a workspace.

### Skill Triggers

| When you need to… | Use |
|--------------------|-----|
| Answer domain questions (architecture, decisions, patterns) | `zbrain:ask` |
| Record a file, URL content, pasted text, or observation | `zbrain:learn` |
| List, analyze, QA, or apply evidence | `zbrain:ingest` |

**Before answering any question about domain knowledge, project decisions, or architectural patterns — invoke `zbrain:ask` first. Never answer from memory.**

### Retrieval Tier Priority

`axioms/` (P0) → `mental-models/` (P1) → `projects/` (P2) → `decisions/` (P3)

Higher-tier results rank first regardless of BM25 score.

### Evidence Pipeline

Each piece of external material moves through three public verbs:

```
learn → ingest → ask
```

Use `zbrain:ingest list` to see which stage each item is in and what command runs next.
**Never advance to apply if any P0 or P1 question is unresolved.**

### Secondary Workspaces (optional)

Each project registry entry supports a `secondary_workspaces` array for cross-workspace context.
Each entry has `workspace`, `keywords`, and optional `limit`.
Secondary results fill remaining slots after primary results.

### Invariants

- **Cite all retrieved context.** Never answer domain questions from memory.
- **One workspace per query.** Never cross workspace boundaries in a single retrieval.
- **Evidence is immutable after ingest.** Never edit `raw.md` or `source.yaml`.
- **Apply gate.** Block apply if any P0 or P1 QA question is `awaiting_external`.
