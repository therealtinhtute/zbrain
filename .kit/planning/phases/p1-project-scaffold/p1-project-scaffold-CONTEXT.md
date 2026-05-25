# Context: Product Scaffold

Phase: p1-project-scaffold
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: medium
Expected Proof: unit

## Goal
Create the root Bun/TypeScript CLI skeleton and bundled asset tree that later phases build on.

## Scope Boundary
### Allowed Surfaces
- `package.json`
- `tsconfig.json`
- `vitest.config.ts`
- build entry such as `build.ts`
- `src/index.ts` and bootstrap-only modules
- root `assets/` directory
- `tests/` smoke coverage

### Forbidden Surfaces
- `~/.zwiki/` runtime state
- project-local `.claude/` integration logic beyond CLI registration
- qmd execution logic
- evidence and retrieval business logic

## Spec Hooks
- Bun is runtime, package manager, and compiler
- command surface includes `setup`, `init`, `workspace create`, and `update`
- binary distribution must embed runtime assets

## Locked Decisions
- source-of-truth bundled assets live only under root `assets/`
- `src/index.ts` owns top-level command registration
- smoke tests prove CLI boot and module loading before deeper logic exists

## Assumptions
- Bun is available in the execution environment
- compile-time asset embedding can be deferred to a documented approach if Bun needs a workaround

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- `wiki-template/` content only as migration source

## Rejected Options
- implement product code under `wiki-template/` because it conflicts with the locked repo shape
- postpone tests until later phases because scaffold regressions are cheap to catch now

## Deferred Ideas
- full asset content fidelity
- qmd binary checks and workspace creation logic

## Escalate If
- Bun compile cannot package runtime assets with an acceptable approach
- root asset layout conflicts with the spec runtime paths
