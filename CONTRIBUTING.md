# Contributing to zbrain

Thanks for your interest in zbrain. zbrain is a provenance-gated trust layer for AI agent memory — locally owned, human-reviewed, traceable to source. We welcome contributions that respect this design.

## Quick Start

```bash
git clone <repo>
cd zbrain
bun install
bun run typecheck        # verify TypeScript
bun test                 # run test suite
bun run build            # compile binary into dist/zbrain
```

The binary is a single Bun-compiled executable. Run it directly during development:

```bash
bun run src/index.ts setup
bun run src/index.ts init
bun run src/index.ts ask "question"
```

Smoke test in isolation (does not touch your real `~/.zbrain`):

```bash
ZBRAIN_HOME=/tmp/zbrain-smoke ./dist/zbrain setup
```

## Repository Layout

```
src/
  index.ts                # entry + commander wiring
  commands/               # thin orchestration (one writer)
  core/                   # pure logic; takes RuntimePaths
  adapters/               # RetrievalAdapter, IngestAdapter, RuntimeAdapter
  db/                     # SQLite schema + repos
  schemas/                # zod
  generated/              # bundled-assets.ts (committed; regenerated)
assets/                   # source of truth for runtime content
  engine/ skills/ agents/ templates/ workspaces/
scripts/                  # codegen + maintenance
docs/                     # user-facing docs
.kit/planning/            # locked SPEC + ROADMAP + phase files (local)
.kit/runs/                # cook run artifacts (local)
```

`assets/` is the source of truth for runtime content. The script
`scripts/generate-bundled-assets.mjs` walks `assets/` and embeds everything
into `src/generated/bundled-assets.ts`. **Regenerate after any change to
`assets/`** — the generated file is committed.

## Architecture (60 seconds)

Three layers:

```
dist/zbrain (binary)          ← bun build --compile
  ↓ zbrain setup extracts
~/.zbrain/ (runtime)          ← engine, skills, agents, templates, workspaces
  ↓ zbrain init injects
<project>/.claude/ (project)  ← optional Claude-specific skills/agents/settings
```

Core layer is pure logic: no I/O in functions that don't take
`paths: RuntimePaths`. Adapters (`QmdAdapter`, `QmdRunner`, `RetrievalAdapter`)
are injectable — design seam for testing.

For deeper detail see `CLAUDE.md` and `wiki-spec.md`.

## Test Conventions

- Tests live in `tests/` (Bun test runner, `*.test.ts`).
- One test file per module in `src/core/`. Name with the same slug.
- Fixtures go in `tests/fixtures/` (gitignored, synthesized per test).
- Tests must run in isolation: each test seeds its own `ZBRAIN_HOME` in a temp
  dir and tears it down.
- Smoke test (`tests/smoke.test.ts`) is the minimum viable harness check.

Run:

```bash
bun test                  # full suite
bun test tests/lifecycle  # one file
bun test --bail           # stop on first failure
```

## PR Conventions

1. **One concern per PR.** Don't bundle refactors with features.
2. **Tests required for any change under `src/`.** No test, no merge.
3. **No new runtime deps without explicit approval** in the PR description.
4. **Run `bun test` + `bun run typecheck` before pushing.** Both must exit 0.
5. **Update CHANGELOG.md** for any user-visible change.
6. **Reference the phase plan** (`./.kit/planning/phases/NN-*/PLAN.md`) in the PR
   body for any non-trivial change.

## Before Building a Feature

Open an issue first. zbrain is deliberately small — features that pull in
embeddings, hosted SaaS, GUI editors, or auto-capture-everything memory are
explicitly out of scope. If your idea touches one of those, the answer is
probably "no" or "defer to V3." Discuss before coding.

## Code Style

- TypeScript strict; no `any` in committed code.
- Comments explain *why*, not *what*. The code is the what.
- Match existing style in the surrounding module. Don't refactor what isn't
  broken.
- Imports ordered: stdlib, third-party, local.

## Reporting Issues

Use GitHub issues. Include:

- `zbrain --version` output
- Steps to reproduce
- Expected vs actual behavior
- `ZBRAIN_HOME` location (don't paste the contents)

## License

By contributing, you agree your contributions are MIT-licensed — same as the
project. See `LICENSE`.
