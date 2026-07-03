## zbrain Integration

zbrain is a workspace-isolated knowledge retrieval layer. Skills live in `.claude/skills/zbrain-*`.
Runtime root: `~/.zbrain/`. Project registry: SQLite (`~/.zbrain/zbrain.db`) — read it via `zbrain workspace current`.

### Workspace Resolution

1. Run `zbrain workspace current` (JSON output) — gives `workspace` and `context_file` for the current project root.
2. Fallback: `~/.zbrain/config.yml` → `default_workspace`.
3. If neither resolves, stop and report — never guess a workspace.

### Skill Triggers

| When you need to… | Use |
|--------------------|-----|
| Answer domain questions (architecture, decisions, patterns) | `zbrain:ask` |
| Record a file, URL content, pasted text, or observation | `zbrain:learn` |
| List, analyze, QA, or apply evidence | `zbrain:ingest` |
| Write trusted, already-verified knowledge directly (no external source to gate) | `zbrain note add` |

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

**Fast path (`zbrain note add`):** for knowledge that is already trusted and first-party
(no external source to gate — e.g. a decision made in this conversation, a verified fact),
write directly to the wiki instead of going through `learn` → `ingest`. Still conflict-checked
and still governed by the same lifecycle (supersede, not overwrite). Reserve `learn`/`ingest`
for material from outside the conversation that needs a human review step.

### Secondary Workspaces (optional)

Each project registry entry supports a `secondary_workspaces` array for cross-workspace context.
Each entry has `workspace`, `keywords`, and optional `limit`.
Secondary results fill remaining slots after primary results.

### Invariants

- **Cite all retrieved context.** Never answer domain questions from memory.
- **One workspace per query.** Never cross workspace boundaries in a single retrieval.
- **Evidence is immutable after ingest.** Never edit `raw.md` or `source.yaml`.
- **Apply gate.** Block apply if any P0 or P1 QA question is `awaiting_external`.
