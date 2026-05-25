# Context: Release Hardening

Phase: p7-release-hardening
Status: ready
Spec Link: ../../SPEC.md
Roadmap Link: ../../ROADMAP.md
Blast Radius: medium
Expected Proof: platform

## Goal
Prove the MVP is shippable as a compiled binary with seeded workspaces, working install flow, and matching documentation.

## Scope Boundary
### Allowed Surfaces
- build and release scripts
- packaging docs
- final README and release docs
- end-to-end acceptance fixtures

### Forbidden Surfaces
- feature expansion beyond the locked MVP
- architectural rewrites of earlier phases unless a release blocker is found

## Spec Hooks
- Bun compile must produce a binary distribution
- supported packaging targets are macOS arm64, Linux x64, and Windows x64
- setup, init, learn, and ask must be demonstrated in one acceptance path

## Locked Decisions
- qmd remains an external prerequisite
- documentation must describe actual behavior, not planned behavior
- release automation may be deferred only if the manual path is explicit and reproducible

## Assumptions
- cross-platform build verification may be packaging-level rather than full functional execution on every OS
- starter workspaces from earlier phases are sufficient for smoke acceptance

## Canonical Refs
- `.kit/planning/SPEC.md`
- `.kit/planning/ROADMAP.md`
- outputs from Phases 1 through 6

## Rejected Options
- claiming release readiness without binary and walkthrough verification
- documenting install paths that assume Node.js on the target machine

## Deferred Ideas
- npm publishing
- broader release automation beyond MVP needs

## Escalate If
- compiled assets are missing from the binary
- one of the required target packages cannot be produced with the current toolchain
