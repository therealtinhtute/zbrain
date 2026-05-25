# Context: Planning Alignment

Phase: p0-planning-alignment
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: low
Expected Proof: unit

## Goal
Lock repo shape and planning language before implementation starts.

## Scope Boundary
### Allowed Surfaces
- `.kit/planning/**`
- `.kit/workflow-state.yml`
- root product docs that describe repo layout if needed during execution

### Forbidden Surfaces
- runtime code under `src/`
- bundled assets under `assets/`
- `wiki-template/` source material other than referencing it

## Spec Hooks
- MVP-1 is a Bun-compiled root CLI product
- `wiki-template/` is not the permanent code home
- command surface and runtime paths must be unambiguous

## Locked Decisions
- implementation home is the repo root
- `wiki-template/` remains reference/input material until adapted into root `assets/`
- workflow execution starts with planning alignment before scaffold work

## Assumptions
- the spec remains locked during planning refresh
- no code has been generated yet that would force backward compatibility

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- `README.md`
- `wiki-template/README.md`

## Rejected Options
- keep the old roadmap unchanged because it points at conflicting locations
- move product work into a separate repo because that contradicts the locked direction

## Deferred Ideas
- doc polish outside the repo-shape problem
- implementation details for Bun compile and qmd integration

## Escalate If
- the spec changes repo home or command surface
- another planning artifact introduces a second implementation location
