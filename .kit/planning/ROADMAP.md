# ROADMAP: zwiki MVP-1

Spec: `.kit/planning/SPEC.md`
Entry Phase: `p1-project-scaffold`
Execution Mode: sequential (phases depend on prior deliverables)
Updated At: 2026-05-24

---

## Phases

### P1: Project Scaffold
**Goal**: Bun project initialized, all dependencies installed, asset directory structure created, build compiles empty binary.
**Deliverables**: `package.json`, `tsconfig.json`, `build.ts`, `src/index.ts` (commander shell), `assets/` directory with all template/engine/command/agent `.md` files.
**Requirements covered**: Tech Stack, File Architecture (source repo layer)
**Risk**: Low
**Effort**: ~2h

### P2: Core Engine
**Goal**: Workspace resolver, YAML config parser, evidence state machine, and asset extractor — all with unit tests.
**Deliverables**: `src/core/workspace-resolver.ts`, `src/core/evidence-state-machine.ts`, `src/core/asset-extractor.ts`, `src/parsers/yaml.ts`, `src/parsers/markdown.ts`
**Requirements covered**: R1, R18, R21-R26 (invariants), config format
**Risk**: Medium (state machine invariants are the most complex logic)
**Effort**: ~3h

### P3: CLI Commands
**Goal**: `zwiki setup`, `zwiki init`, `zwiki workspace create`, `zwiki update` — all with clack UX, fully functional.
**Deliverables**: `src/commands/setup.ts`, `src/commands/init.ts`, `src/commands/workspace.ts`, `src/commands/update.ts`
**Requirements covered**: R6-R9, UX constraints (D9), symlink strategy (D6), non-destructive init (D8)
**Depends on**: P2 (workspace resolver, asset extractor)
**Risk**: Medium (symlink cross-platform, CLAUDE.md injection)
**Effort**: ~3h

### P4: Slash Commands + Subagents
**Goal**: All 5 slash command `.md` files + 2 subagent `.md` files authored and functional in Claude Code.
**Deliverables**: `assets/commands/ask.md`, `learn.md`, `reflect.md`, `workspace.md`, `reindex.md`, `assets/agents/wiki-planner.md`, `wiki-qmd-selector.md`
**Requirements covered**: R10-R17
**Depends on**: P1 (assets directory exists)
**Risk**: Low (markdown authoring, no code logic)
**Effort**: ~2h

### P5: Evidence Pipeline
**Goal**: `/learn` full cycle works: ingest → analyze → qa → apply, with all invariants enforced.
**Deliverables**: Evidence pipeline logic in slash command + state machine integration, `assets/templates/evidence-*.md|yaml`
**Requirements covered**: R3, R11-R14, R21-R25 (invariants I-1 through I-5)
**Depends on**: P2 (state machine), P4 (slash commands)
**Risk**: High (most invariants live here — immutable sources, workspace lock, QA gate, resumable checkpoint)
**Effort**: ~3h

### P6: Retrieval Pipeline
**Goal**: `/ask` end-to-end: parse intent → qmd BM25 search → priority post-filter → answer with citations.
**Deliverables**: `src/core/qmd-retrieval.ts`, `src/core/current-task-writer.ts`, qmd config generation, MCP integration verified
**Requirements covered**: R4, R10, R20, R26 (I-6 workspace-scoped search), D2, D3, D4
**Depends on**: P2 (workspace resolver), P4 (subagents), qmd installed
**Risk**: High (external dependency qmd, MCP integration, priority post-filter correctness)
**Effort**: ~3h

### P7: Build + Release
**Goal**: Binary compiles with embedded assets, `zwiki setup` extracts correctly, README complete, first workspace indexed.
**Deliverables**: Working `bun build --compile` output, GitHub Release workflow (or manual), README.md, example workspace entries
**Requirements covered**: D5, Done When (Structure + Documentation)
**Depends on**: All previous phases
**Risk**: Medium (Bun compile asset embedding, binary size)
**Effort**: ~2h

---

## Dependency Graph

```
P1 ──→ P2 ──→ P3
 │      │      │
 │      │      └──→ P7
 │      │
 └──→ P4 ──→ P5
        │
        └──→ P6 ──→ P7
```

**Parallelizable**: P4 can start alongside P2 (asset authoring vs core logic). P5 and P6 can run in parallel after P4 completes. Everything else sequential.

---

## Total Estimated Effort

~18h (normal lane, single developer)
