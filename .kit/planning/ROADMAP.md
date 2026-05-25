# ROADMAP: zwiki MVP-1

## Planning Basis
- source spec: `.kit/planning/SPEC.md`
- planning mode: `full`
- recommended entry phase: `p0-planning-alignment`
- execution mode: sequential with limited parallel work inside phases only when explicitly noted

## Phase 0: Planning Alignment
**Goal:** remove repo-layout ambiguity before code exists.

**Deliverables:**
- planning artifacts point to repo root as the product home
- `wiki-template/` is framed as migration/reference input, not the implementation home
- workflow entry is set to the alignment phase

**Dependencies:**
- `.kit/planning/SPEC.md`
- current repo docs and planning artifacts

**Risks / Watch-fors:**
- stale docs can reintroduce the wrong implementation location
- future phases drift if repo shape is not locked first

## Phase 1: Product Scaffold
**Goal:** establish the root Bun/TypeScript CLI skeleton and bundled asset tree.

**Deliverables:**
- root `package.json`, `tsconfig.json`, `vitest.config.ts`, build entry, and `src/index.ts`
- root `assets/` tree with engine/templates/commands/agents/workspaces placeholders
- smoke tests for CLI boot and module loading

**Dependencies:**
- Phase 0 decisions

**Risks / Watch-fors:**
- Bun compile asset embedding may need an explicit strategy
- asset layout must be root-only to avoid duplicate truth with `wiki-template/`

## Phase 2: Core Runtime
**Goal:** implement config, workspace resolution, safe filesystem helpers, asset sync, and evidence state invariants.

**Deliverables:**
- config loader for `~/.zwiki/config.yml`
- deterministic workspace resolver and filesystem helper layer
- asset extraction/version sync logic and evidence state model with tests

**Dependencies:**
- Phase 1 scaffold

**Risks / Watch-fors:**
- resolution precedence bugs can invalidate every later command
- evidence invariants must be enforced centrally, not spread across commands

## Phase 3: Runtime Assets and Command Content
**Goal:** convert the spec into shippable runtime assets under root `assets/`.

**Deliverables:**
- workspace/evidence templates used by runtime scaffolding
- slash command docs for `/ask`, `/learn`, `/reflect`, `/workspace`, `/reindex`
- bundled agents and engine rules adapted from `wiki-template/`

**Dependencies:**
- Phase 1 asset tree
- Phase 2 parser/validation helpers where needed

**Risks / Watch-fors:**
- copying `wiki-template/` wholesale would preserve obsolete behavior
- content files must match current command names and invariants

## Phase 4: CLI Commands
**Goal:** make `setup`, `update`, `workspace create`, and `init` function end to end with clack UX.

**Deliverables:**
- interactive command handlers wired into the CLI
- runtime extraction to `~/.zwiki/`
- non-destructive project integration under `<cwd>/.claude/`

**Dependencies:**
- Phases 2 and 3

**Risks / Watch-fors:**
- cross-platform symlink behavior
- preserving existing `CLAUDE.md` and project-local config

## Phase 5: Retrieval Pipeline
**Goal:** make `/ask` retrieve only from the active workspace via qmd BM25.

**Deliverables:**
- qmd adapter and collection layout rules
- tier classification and ranking logic
- `current-task.md` materialization and knowledge-gap handling

**Dependencies:**
- Phases 2 and 3
- qmd installed externally

**Risks / Watch-fors:**
- cross-workspace leakage must be impossible in the adapter
- qmd response shape is an external dependency

## Phase 6: Evidence Pipeline
**Goal:** make `/learn` work from ingest through apply with invariants and resumability.

**Deliverables:**
- ingest/analyze/qa/apply implementation
- QA gating and checkpoint-based resume behavior
- apply-time mutation guards and reindex trigger

**Dependencies:**
- Phases 2 and 3
- Phase 5 retrieval for post-apply reindex expectations

**Risks / Watch-fors:**
- invalid transitions can corrupt workspace knowledge
- apply must preserve immutable sources and auditability

## Phase 7: Release Hardening
**Goal:** prove the MVP is shippable as a compiled binary with seeded workspaces and documentation.

**Deliverables:**
- compiled binary smoke-tested with embedded assets
- release path documented or automated
- end-to-end walkthrough and final install/usage docs

**Dependencies:**
- Phases 1 through 6

**Risks / Watch-fors:**
- Bun compile behavior across platforms
- README and release docs drifting from actual command behavior
