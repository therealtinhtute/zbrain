# Context: Runtime Assets and Command Content

Phase: p3-runtime-assets-command-content
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: medium
Expected Proof: unit

## Goal
Turn the locked command model, templates, and engine rules into real bundled assets under root `assets/`.

## Scope Boundary
### Allowed Surfaces
- `assets/templates/**`
- `assets/commands/**`
- `assets/agents/**`
- `assets/engine/**`
- content validation tests

### Forbidden Surfaces
- CLI command handler logic
- runtime extraction code
- qmd execution modules

## Spec Hooks
- slash commands are `/ask`, `/learn`, `/reflect`, `/workspace`, `/reindex`
- templates include workspace, axiom, mental-model, project, and evidence artifacts
- engine rules must encode workspace isolation, citation, and no-guessing behavior

## Locked Decisions
- adapt from `wiki-template/` selectively instead of copying whole directories
- asset names match the MVP command surface exactly
- content validation tests should assert placeholders and parseability where possible

## Assumptions
- some legacy `wiki-template` files map conceptually to new asset names but require trimming
- bundled workspaces can start as starter content rather than full knowledge bases

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- `wiki-template/agents/**`
- `wiki-template/templates/**`
- `wiki-template/.claude/**`

## Rejected Options
- keep older command names like `use-wiki` or `switch-workspace` because the spec locks a different surface
- preserve older multi-agent files that are explicitly out of MVP scope

## Deferred Ideas
- richer authoring guidance beyond the required command and engine assets
- non-MVP workspaces or templates

## Escalate If
- required asset content cannot be derived without changing the locked spec
- command or template naming collides with existing runtime path rules
