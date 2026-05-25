# Context: Core Runtime

Phase: p2-core-engine
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: high
Expected Proof: unit

## Goal
Implement the core runtime primitives that every CLI and slash-command path depends on.

## Scope Boundary
### Allowed Surfaces
- `src/core/**`
- `src/lib/**` or equivalent helper modules
- `src/schemas/**`
- `tests/core/**`

### Forbidden Surfaces
- user-facing CLI flow modules in `src/commands/**`
- final asset content in `assets/**`
- retrieval-specific qmd adapter logic

## Spec Hooks
- `~/.zwiki/config.yml` is the runtime config source
- workspace resolution precedence is fixed
- invariants I-1 through I-5 belong in central runtime enforcement

## Locked Decisions
- workspace resolution is deterministic and test-first
- filesystem safety helpers are shared primitives, not ad hoc command code
- evidence state validation lives in a dedicated model or state machine

## Assumptions
- home directory paths can be abstracted for testing
- versioned asset sync may use manifest metadata or file hashing, but must preserve user content

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- Phase 1 scaffold outputs

## Rejected Options
- resolve workspaces directly inside each command because that duplicates precedence logic
- enforce evidence invariants only in slash-command markdown because runtime code must guard them too

## Deferred Ideas
- qmd collection management details
- project integration symlink policy beyond helper primitives

## Escalate If
- the spec needs a different workspace precedence order
- resumable apply requires a storage model that conflicts with immutable-source rules
