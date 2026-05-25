# Context: Retrieval Pipeline

Phase: p5-retrieval-pipeline
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: high
Expected Proof: integration

## Goal
Implement the qmd-backed `/ask` retrieval path with strict workspace scoping and tiered ranking.

## Scope Boundary
### Allowed Surfaces
- `src/core/qmd-*.ts`
- `src/core/current-task*.ts`
- retrieval-focused tests
- asset updates required to keep agent instructions aligned

### Forbidden Surfaces
- evidence apply logic
- non-BM25 retrieval features
- cross-workspace access of any kind

## Spec Hooks
- `/ask` uses qmd BM25 only
- I-6 requires collection-scoped search
- knowledge gaps must stop the answer path rather than guess

## Locked Decisions
- qmd is an external prerequisite, not bundled
- post-filtering is deterministic by path tier, not model reranking
- `current-task.md` is the handoff artifact between retrieval and answer generation

## Assumptions
- qmd CLI or MCP response can be wrapped behind one adapter surface
- seeded or temp workspace fixtures can stand in for live knowledge bases during tests

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- Phase 2 workspace resolver
- Phase 3 `wiki-qmd-selector` asset

## Rejected Options
- vector or hybrid search because they are outside MVP
- querying multiple workspace collections and filtering later because that violates isolation

## Deferred Ideas
- result snippet beautification
- advanced ranking heuristics beyond tier plus BM25 order

## Escalate If
- qmd cannot enforce per-collection search in a way that satisfies I-6
- external qmd behavior differs from the assumptions needed for deterministic tests
