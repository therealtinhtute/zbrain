## zbrain Integration

zbrain is a local-first trusted memory CLI. It stores canonical Markdown claims and immutable local evidence snapshots, then returns trusted context JSON through `zbrain ask`.

### Expected Usage

- Capture local source material with `zbrain evidence add --file <path> --origin <uri-or-path>`.
- Draft claims from stdin with `zbrain claim draft`.
- Promote valid drafts with `zbrain claim approve <id>`.
- Rebuild the derived index with `zbrain reindex`.
- Retrieve trusted context with `zbrain ask <query>`.

### Invariants

- Only approved claims are trusted.
- Drafts remain promotion candidates until approved.
- Explicit conflicts block trusted context.
- Missing approved memory returns a gap.
- Secondary workspaces require explicit `--include`.
