# Context: Evidence Pipeline

Phase: p6-evidence-pipeline
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: high
Expected Proof: integration

## Goal
Implement `/learn` from ingest through apply while enforcing immutable sources, QA gates, workspace lock, citation, and resumability.

## Scope Boundary
### Allowed Surfaces
- `src/core/evidence-*.ts`
- slash-command integration code required for `/learn`
- evidence-focused tests and fixtures
- apply-time workspace mutation helpers

### Forbidden Surfaces
- retrieval ranking logic beyond reindex triggering
- non-MVP ingestion sources or sync integrations
- cross-workspace operations

## Spec Hooks
- evidence pipeline stages are ingest, analyze, qa, apply
- invariants I-1 through I-5 are mandatory
- successful apply triggers reindex for the active workspace

## Locked Decisions
- raw source files are append-only after ingest
- apply is resumable via checkpoint manifests
- QA status is a first-class gate, not advisory metadata

## Assumptions
- analysis artifacts can be generated deterministically enough for file layout and orchestration tests
- reindex can be triggered through a narrow runtime hook rather than inline shell logic everywhere

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- Phase 2 evidence state model
- Phase 3 evidence templates and `/learn` command asset

## Rejected Options
- mutate `raw.md` or `source.yaml` after ingest because that breaks I-1
- allow apply with unresolved P0/P1 questions because that breaks I-3

## Deferred Ideas
- richer archival workflows after apply
- automated source-import adapters

## Escalate If
- phase implementation needs to violate immutable-source or workspace-lock rules
- analysis or apply steps require new product behavior outside the locked spec
