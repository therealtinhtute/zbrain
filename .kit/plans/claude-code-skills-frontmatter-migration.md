# Plan: Claude Code Skills + Frontmatter Migration for `zbrain`

Last updated: 2026-05-25

## 1) Research Summary (Official Docs)

Authoritative findings from current Claude Code docs:

- Custom commands are now merged into Skills; existing `.claude/commands/*.md` still works, but `SKILL.md` is the preferred model.
  - Source: https://code.claude.com/docs/en/skills
- Skill configuration is YAML frontmatter on `SKILL.md` with fields like `description`, `argument-hint`, `allowed-tools`, `disable-model-invocation`, `user-invocable`, `model`, `effort`, `paths`, and more.
  - Source: https://code.claude.com/docs/en/skills
- Standalone/project skills use `.claude/skills/<skill-name>/SKILL.md` and invoke as `/skill-name` (non-namespaced).
  - Source: https://code.claude.com/docs/en/skills
- Namespaced form `/plugin-name:skill-name` is plugin-only (via `.claude-plugin/plugin.json` + `skills/`).
  - Source: https://code.claude.com/docs/en/plugins
- Docs recommend using `skills/` for new plugin work; plugin `commands/` exists mainly for compatibility.
  - Source: https://code.claude.com/docs/en/plugins

## 2) Current Repo State (Gap Analysis)

- Runtime currently extracts command docs to `~/.zbrain/commands/*.md`, then `zbrain init` links them to `.claude/commands/`.
- Current command files are:
  - `assets/commands/zbrain:ask.md`
  - `assets/commands/zbrain:learn.md`
  - `assets/commands/zbrain:reflect.md`
  - `assets/commands/zbrain:workspace.md`
  - `assets/commands/zbrain:reindex.md`
- These files are plain markdown (no frontmatter contract), and current flow is command-file-based, not `SKILL.md`-based.

## 3) Decision and Target Architecture

New project rule: use **flat command files** with frontmatter-defined skill names.

- File naming convention:
  - `ask.md`, `learn.md`, `reflect.md`, `workspace.md`, `reindex.md`
- Frontmatter naming convention:
  - `name: zbrain:ask`
  - `name: zbrain:learn`
  - `name: zbrain:reflect`
  - `name: zbrain:workspace`
  - `name: zbrain:reindex`

Target shape:

- Runtime source remains command-file-based (no behavior change), but every file carries skill frontmatter metadata.
- Canonical files under `assets/commands/` become:
  - `assets/commands/ask.md`
  - `assets/commands/learn.md`
  - `assets/commands/reflect.md`
  - `assets/commands/workspace.md`
  - `assets/commands/reindex.md`
- Each file includes frontmatter for:
  - `name` (required by this new rule, namespaced as `zbrain:*`)
  - `description`
  - `argument-hint` (where relevant)
  - `disable-model-invocation` (for side-effectful workflows)
  - `allowed-tools` (minimal, explicit grants)

## 4) Implementation Plan (Phased)

### Phase A — Introduce Skills without breaking current behavior

1. Rename command assets from `zbrain:*.md` filenames to flat names (`ask.md`, `learn.md`, `reflect.md`, `workspace.md`, `reindex.md`).
2. Add YAML frontmatter to each file with `name: zbrain:*` and other skill metadata.
3. Keep command body content and task logic unchanged.
4. Regenerate bundled assets and update extraction paths/tests.

### Phase B — Wire product integration

1. Update docs to make frontmatter-based skill metadata explicit (`name: zbrain:*` in `*.md` files).
2. Update `zbrain init` messaging:
   - current mode: links `.claude/commands/`
   - new mode: clarify these command files are skill-compatible and frontmatter-driven.
3. Add migration note:
   - flat command filenames are now canonical
   - invocation identity comes from frontmatter `name`

### Phase C — Cleanup (after compatibility window)

1. Remove any filename-based assumptions (`zbrain:ask.md`) from tests/docs.
2. Keep frontmatter `name` as the only identity contract.
3. Keep thin compatibility shims only if needed by downstream users.

## 5) Verification Plan

Required checks after implementation:

1. `bun test --run` (all existing tests green, plus new tests below).
2. `bunx tsc --noEmit`.
3. Add tests for skill artifacts:
   - each command file (`assets/commands/*.md`) has parseable YAML frontmatter.
   - required keys exist (`name`, `description`, and policy keys where expected).
   - `name` values match `zbrain:*` rule.
4. Manual Claude Code smoke:
   - verify `/zbrain:ask`, `/zbrain:learn`, `/zbrain:reflect`, `/zbrain:workspace`, `/zbrain:reindex` appear in `/help`.
   - verify argument passthrough for `ask`.

## 6) Scope Guardrails

- Keep current core `zbrain` logic and use cases unchanged.
- This migration is interface/configuration modernization only (Skills + frontmatter + packaging).
- Do not refactor retrieval/evidence pipelines during this work.
