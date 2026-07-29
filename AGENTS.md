# Repository Guidelines

## Project Structure & Module Organization

zbrain is a Bun-compiled TypeScript CLI for personal LLM wiki workflows. Core layout:

- `src/index.ts` — CLI entry point (Commander)
- `src/commands/` — CLI command handlers: `setup`, `init`, `workspace`, `update`, `ui`, `helpers`
- `src/core/` — business logic: evidence pipeline, retrieval, qmd adapter, runtime paths, workspace resolver, config, assets, `current-task.md` writer
- `src/schemas/` — Zod schemas for `config.yml` and `zbrain.json`
- `src/generated/bundled-assets.ts` — auto-generated asset manifest (do not edit by hand)
- `assets/` — runtime assets bundled into the binary:
  - `assets/engine/` — system prompt, constraints, retrieval rules, evidence rules, claude rules
  - `assets/skills/` — Claude Code skill definitions (`zbrain:ask`, `zbrain:learn`, `zbrain:reflect`, `zbrain:workspace`, `zbrain:reindex`)
  - `assets/templates/` — scaffolds for workspaces and evidence doc types
  - `assets/workspaces/` — seed `workspace.md` files for the 4 default domains
- `tests/` — Vitest tests (`tests/**/*.test.ts`)
- `scripts/` — build utilities, asset generation
- `docs/` — acceptance walkthrough, release guidance
- `.kit/planning/` — locked SPEC.md, roadmap, phased execution plans

Keep business logic in `src/core/`. Keep command handlers thin — they parse args, call core, and print output. Do not mix workspace-specific content into engine or template files.

## Build, Test, and Development Commands

```bash
bun install                          # install dependencies
bun run build                        # compile binary → dist/zbrain
bun run generate:assets              # regenerate src/generated/bundled-assets.ts
bun test --run                       # run all tests once
bunx tsc --noEmit                    # type-check without emitting
```

Smoke test the binary after build:

```bash
./dist/zbrain --help
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
```

The `qmd` CLI is a runtime dependency, not a test dependency. Retrieval tests use a stubbed adapter for deterministic proof when qmd is not installed.

## Coding Style & Naming Conventions

- TypeScript, strict mode, ESM (`"type": "module"`)
- 2-space indentation; no semicolons in object schemas (follow existing file style)
- `camelCase` for functions and variables; `PascalCase` for types and classes
- Zod schemas for all config and pointer validation — extend `src/schemas/config.ts`
- Keep core functions pure when possible; side-effectful I/O goes in `src/commands/` or explicit `*-store.ts` files
- `src/generated/bundled-assets.ts` is auto-generated — always regenerate with `bun run generate:assets` after changing `assets/`

## Testing Guidelines

- Tests live in `tests/**/*.test.ts` and run with Vitest
- Prefer unit tests that use temp directories (`os.tmpdir()`) and avoid touching `~/.zbrain/`
- Use the stubbed qmd adapter (`src/core/qmd-adapter.ts`) for retrieval tests — do not require a real qmd install
- Add focused tests near the module they cover; follow the existing `*.test.ts` naming pattern
- Run `bunx tsc --noEmit` before committing to catch type errors early

## Asset Authoring Guidelines

- Skill files in `assets/skills/*/SKILL.md` must have frontmatter: `name`, `description`, `version`
- Engine files in `assets/engine/` are plain Markdown — no frontmatter required
- Template files in `assets/templates/` use `{{placeholder}}` tokens matching the scaffold logic in `src/core/`
- After editing any file under `assets/`, run `bun run generate:assets` to update the bundled manifest

## Commit & Pull Request Guidelines

Use Conventional Commit style, matching recent history such as `feat(core): ...`, `fix(cli): ...`, `docs(spec): ...`. Keep scopes specific to the area changed. PRs should include a short summary, affected paths, and commands run to verify.

## Security & Configuration Tips

- Do not commit secrets, personal workspace data, or any populated `~/.zbrain/` output
- Treat workspace isolation as a hard rule — never add logic that reads across workspace boundaries
- `raw.md` and `source.yaml` inside `evidence/sources/` are immutable by design — never write code that modifies them after creation
- `src/generated/bundled-assets.ts` embeds asset file contents — review diffs carefully to avoid accidentally bundling sensitive local files

<!-- ZHARNESS:BEGIN -->
## Harness

Run `zharness --version`, then `zharness preflight <stage> [--mode <mode>] --json` for every workflow skill invocation. Follow a returned stop and recovery exactly.

Read `docs/WORKFLOW.md`, then only the returned stage playbook and repository material relevant to the requested outcome. Repository docs, code, tests, and observable behavior are authoritative; the database is a lifecycle ledger and recovery index.

Read-only and bounded work may use reduced mode and must not mutate harness state. Durable planning, full execution, full checks, and durable handoffs require an initialized database. Claim completion only with executable or observable evidence.
<!-- ZHARNESS:END -->
