## zbrain Integration

zbrain is a local-first trusted memory CLI. It returns versioned JSON context for agents; it does not call an LLM or write final answers.

### Workspace Resolution

1. Run `zbrain workspace current` to identify the primary workspace.
2. Use `--workspace <name>` only when the caller explicitly names a primary workspace.
3. Use `--include <name>` only when the caller explicitly permits a read-only secondary workspace.
4. If no workspace resolves, stop and ask the user to create one.

### Trusted Memory Flow

| Need | Command |
|---|---|
| Capture a local evidence snapshot | `zbrain evidence add --file <path> --origin <uri-or-path>` |
| Draft an atomic claim from stdin | `zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived>` |
| Promote a valid draft claim | `zbrain claim approve <id>` |
| Replace an approved claim | `zbrain claim supersede <id>` then approve the replacement |
| Revoke a claim | `zbrain claim revoke <id> --reason <text>` |
| Rebuild the derived index | `zbrain reindex` |
| Retrieve trusted context | `zbrain ask [--include <workspace>] <query>` |

### Invariants

- Only `approved` claims are trusted context.
- Drafts may appear only as `promotion_candidates`.
- Missing approved context returns a gap; unresolved explicit conflicts return blocked status.
- Evidence snapshots are immutable local files.
- Markdown claim files are canonical; SQLite indexes are disposable caches.
- Never infer cross-workspace access. Secondary scopes must be explicit.
