## zbrain Integration

zbrain is a workspace-isolated knowledge retrieval layer for this project.
Runtime root: `~/.zbrain/`. Project registry: SQLite (`~/.zbrain/zbrain.db`) — read it via `zbrain workspace current`.

### Workspace Resolution

1. Run `zbrain workspace current` (JSON output) — gives `workspace` and `context_file` for the current project root.
2. Fallback: `~/.zbrain/config.yml` → `default_workspace`.
3. If neither resolves, stop and report — never guess a workspace.

### Expected Usage

- Use `zbrain learn` to record raw source material.
- Use `zbrain ingest` to list, analyze, QA, and apply evidence.
- Use `zbrain note add` to write trusted, already-verified knowledge directly to the wiki
  (no external source to gate) — still conflict-checked, still supersede-not-overwrite.
- Before answering domain questions, use `zbrain ask` retrieval first.
- Read the registry entry's `context_file` after retrieval to inspect ranked context and citations.
- Keep all retrieval and evidence work inside the active workspace.

### Retrieval Tier Priority

`axioms/` (P0) → `mental-models/` (P1) → `projects/` (P2) → `decisions/` (P3)

Higher-tier results rank first regardless of BM25 score.

### Invariants

- Never answer domain questions from memory when zbrain coverage is expected.
- Never cross workspace boundaries in a single retrieval.
- Never edit `raw.md` or `source.yaml` after ingest.
