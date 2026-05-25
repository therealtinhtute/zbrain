---
name: zbrain:workspace
description: Inspect or switch the active workspace pointer.
disable-model-invocation: true
---

# zbrain:workspace

## Purpose

Inspect or switch the active workspace pointer for the current project.

## Sources

- project pointer: `<cwd>/.claude/zbrain.json`
- fallback default: `~/.zbrain/config.yml`

## Rules

- Switching only changes the pointer for the current project.
- Do not borrow knowledge across workspaces.
