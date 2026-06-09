# Acceptance Walkthrough

## Covered Path

This MVP walkthrough demonstrates:

1. `zbrain setup`
2. `zbrain init`
3. learn flow via `zbrain learn` -> `zbrain ingest analyze/qa/apply`
4. ask flow via retrieval and the project `context_file`

## Validation Sources

- automated acceptance: `bun test --run tests/release/acceptance.test.ts`
- binary smoke: `ZBRAIN_HOME=/tmp/... ./dist/zbrain setup`

## Notes

- qmd remains an external prerequisite; the binary reports its absence but still extracts the runtime tree
- the automated acceptance test uses the real runtime commands and core pipeline modules with a stubbed retrieval adapter for deterministic proof
