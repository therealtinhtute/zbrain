# Phase Context: P4 — Slash Commands + Subagents

## Goal
All 5 slash command .md files + 2 subagent .md files authored and functional in Claude Code.

## Boundaries
- **Allowed surfaces**: assets/commands/, assets/agents/
- **Forbidden surfaces**: src/ (no TypeScript changes), ~/.zwiki/ (runtime)
- **Blast radius**: Replaces P1 placeholder .md files with real content

## Implementation Decisions
- Slash commands are markdown files in .claude/commands/ (Claude Code convention)
- Each command .md contains: description, argument schema, step-by-step instructions for the agent
- Subagents defined in .claude/agents/ with YAML frontmatter (name, description, model, tools)
- wiki-planner uses sonnet model (fast, cheap for intent parsing)
- wiki-qmd-selector uses sonnet model (search + post-filter)
- /learn command handles all 4 stages via flags (--analyze, --qa, --apply)

## Key Design Patterns
- /ask triggers 2 subagents sequentially (planner → selector), then main agent answers
- /learn is a single command with subcommands via arguments (not 4 separate commands)
- /reflect is a convenience wrapper that suggests /learn for recent activity
- /workspace writes to .claude/zwiki.json (project-level config)
- /reindex shells out to `qmd --config-name zwiki index`

## Assumptions
- Claude Code supports .claude/commands/*.md and .claude/agents/*.md
- Subagents can use MCP tools (qmd search, get, multi_get)
- current-task.md written to project root or .claude/ directory

## Expected Proof
- Each command .md has valid YAML frontmatter
- /ask instructions reference both subagents correctly
- /learn instructions cover all 4 stages with correct flags
- Agent .md files specify correct tools and model
