# Cook Run: p7-release-hardening

Mode: full
Phase: p7-release-hardening
Started At: 2026-05-25 03:20
Status: done

## Preflight
- verdict: ready
- artifact check: `.kit/planning/SPEC.md`, `.kit/planning/ROADMAP.md`, `p7-release-hardening` context, and plan exist
- contract drift: asset embedding was a release blocker and was fixed inside the phase boundary through build/runtime changes

## Scope Confirmation
- phase goal: validate packaging, documentation, seeded workspaces, and one end-to-end acceptance path for MVP-1 shipment
- wave execution:
  - T1 compiled binary and embedded assets
  - T2 packaging/release path documentation
  - T3 starter workspace verification
  - T4 end-to-end acceptance and docs finalization

## Task Status
### T1 — Verify compiled binary and embedded assets
- status: DONE
- evidence:
  - `scripts/generate-bundled-assets.mjs` generates `src/generated/bundled-assets.ts`
  - compiled binary runs `setup` and extracts runtime assets without reading the live repo `assets/` tree
- verification:
  - `bun run build.ts`
  - `./dist/zwiki --help`
  - `ZWIKI_HOME=/tmp/... ./dist/zwiki setup`

### T2 — Validate supported packaging targets
- status: DONE
- evidence:
  - `docs/release.md` records the reproducible native-build release path for macOS arm64, Linux x64, and Windows x64
  - build constraints are documented from the local `bun build --help` output
- verification:
  - `bun build --help`

### T3 — Seed and verify initial workspaces
- status: DONE
- evidence:
  - `setup` extracts programming, finance, health, and philosophy starter workspaces from the embedded asset bundle
- verification:
  - `ZWIKI_HOME=/tmp/... ./dist/zwiki setup`

### T4 — Run end-to-end acceptance and finalize docs
- status: DONE
- evidence:
  - `tests/release/acceptance.test.ts` proves setup -> init -> learn -> ask in one temp environment
  - `README.md`, `docs/release.md`, and `docs/acceptance-walkthrough.md` reflect the actual workflow and known limits
- verification:
  - `bun test --run tests/release/acceptance.test.ts`
  - `bun test --run`
  - `bunx tsc --noEmit`
  - `bun run build.ts`
