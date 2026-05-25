# Plan: Release Hardening

Phase: p7-release-hardening
Status: ready
Wave Count: 4
Execution Owner: cook
Updated At: 2026-05-25

## Goal
Validate packaging, documentation, seeded workspaces, and one end-to-end acceptance path for MVP-1 shipment.

## Inputs
- phases 1 through 6 completed
- `.kit/planning/SPEC.md`

## Wave 1
### T1 — Verify compiled binary and embedded assets
- type: implementation
- inputs:
  - build entry from Phase 1
  - runtime asset extraction from Phase 2
- touches:
  - build scripts
  - binary smoke-test notes or tests
- avoid:
  - feature additions
- steps:
  1. Compile the binary with Bun for the primary development platform.
  2. Run smoke checks for `--help` and `setup` against the compiled artifact.
  3. Confirm bundled assets extract correctly from the binary build.
- expected outputs:
  - verified binary build for at least one platform
- verification:
  - compile log plus smoke-test results
- stop if:
  - assets are missing or unusable in the compiled binary
- escalate to:
  - brainstorm refine

## Wave 2
### T2 — Validate supported packaging targets
- type: implementation
- inputs:
  - T1 compile strategy
- touches:
  - release notes or packaging scripts
- avoid:
  - unsupported platform-specific product changes
- steps:
  1. Produce or document packaging commands for macOS arm64, Linux x64, and Windows x64.
  2. Record target-specific blockers if any remain.
  3. Keep the release path reproducible whether automated or manual.
- expected outputs:
  - packaging plan or automation for the three target classes
- verification:
  - packaging artifact inventory or documented manual commands
- stop if:
  - one target needs a materially different architecture path
- escalate to:
  - user clarification

## Wave 3
### T3 — Seed and verify initial workspaces
- type: test
- inputs:
  - bundled starter workspaces
  - setup and workspace commands
- touches:
  - asset seed files if fixes are needed
  - acceptance fixtures
- avoid:
  - expanding beyond the four required workspaces
- steps:
  1. Verify programming, finance, health, and philosophy are present after setup.
  2. Check each workspace for minimum viable structure and retrieval compatibility.
  3. Fix only blocking seed issues discovered during validation.
- expected outputs:
  - verified starter workspaces ready for demos and first use
- verification:
  - temp-home acceptance test or manual inspection output
- stop if:
  - starter workspaces lack required structure for qmd indexing or evidence storage
- escalate to:
  - plan phase

## Wave 4
### T4 — Run end-to-end acceptance and finalize docs
- type: docs
- inputs:
  - completed product phases
  - acceptance environment with qmd available
- touches:
  - `README.md`
  - release documentation
  - acceptance walkthrough notes
- avoid:
  - describing unsupported behavior
- steps:
  1. Execute one full path: setup, init, learn one source, ask one question.
  2. Update install, prerequisite, usage, and known-limit docs to match actual behavior.
  3. Record the exact validation commands used for shipment readiness.
- expected outputs:
  - final user-facing docs aligned with real behavior
  - documented acceptance walkthrough
- verification:
  - manual acceptance record
  - README inspection against command behavior
- stop if:
  - end-to-end acceptance fails on a core invariant
- escalate to:
  - check

## Risks / Watch-fors
- docs lagging behind the implemented command surface
- treating packaging success as product success without end-to-end validation
