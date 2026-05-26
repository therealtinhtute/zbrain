---
name: zbrain:workspace
description: Inspect or switch the active workspace pointer for the current project. Use to see which workspace is active or to change it to a different one.
disable-model-invocation: true
version: "1.0.0"
---

<role>
Act as a workspace pointer manager. Read or write the active workspace pointer for the current project only.
</role>

<security>
- Never copy or share knowledge between workspaces
- Only modify the pointer file — never modify workspace content
- Refuse requests to read from the old workspace after switching
</security>

<instructions>
## Resolution Order

1. Project pointer: `<cwd>/.claude/zbrain.json` field `workspace` (highest priority)
2. Global default: `~/.zbrain/config.yml` field `default_workspace`
3. If neither resolves: stop and report — do not auto-select.

## Inspect (no argument)

1. Read and display the active workspace name and its resolution source.
2. List available workspaces from `~/.zbrain/workspaces/`.
3. Flag if the current pointer references a workspace directory that does not exist.

## Switch (`{workspace-name}`)

1. Validate that the target workspace exists in `~/.zbrain/workspaces/`.
2. Write `{ "workspace": "{name}" }` to `<cwd>/.claude/zbrain.json`.
3. Confirm the switch with the new active workspace name.

## Invariants

- Switching affects only the current project pointer, not other projects.
- Never borrow knowledge from the previous workspace after switching.
- Never create a workspace — use `zbrain workspace create` for that.
</instructions>
