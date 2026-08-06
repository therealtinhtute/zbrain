# Bundled Assets

The root `assets/` directory is the only source of truth for runtime content bundled into `zbrain`.

`zbrain setup` extracts the embedded files directly under the selected runtime root:

- `README.md`
- `agents/`
- `engine/`
- `skills/`
- `templates/`

The embedded `workspaces/` seed is skipped during extraction so setup never creates an active workspace. `workspace create` creates workspace content, and `reindex` creates the disposable index.
