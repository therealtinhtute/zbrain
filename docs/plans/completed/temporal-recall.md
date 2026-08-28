---
id: 01M0ZCT4ER8HS9Y72W5AAB1HZ0
type: plan
intake_id: 01M0ZCYZ38DE4FLTN2DXUQU5YF
lane: normal
status: completed
created: 2026-08-28
updated: 2026-08-28
---

# Plan: Temporal recall

## Outcome
- result: `zbrain ask` and `mcp serve` (`memory_ask`) support deterministic point-in-time and range temporal filtering (`--after`, `--before`, `--as-of`) over claim timestamps (`verified_at`, `created_at`, `stale_after`, and lifecycle transitions) without network calls or external dependencies.
- success_signals:
  - `ask --after <rfc3339>` filters out claims verified/created before the given timestamp.
  - `ask --before <rfc3339>` filters out claims verified/created after the given timestamp.
  - `ask --as-of <rfc3339>` reconstructs the active knowledge state at that point in time (including superseded/revoked claims if they were active as-of `<t>`, and excluding claims verified after `<t>` or stale before `<t>`).
  - `mcp serve` tool `memory_ask` exposes matching `after`, `before`, `as_of` parameters.
  - Existing query behavior without temporal flags is completely unchanged.

## Authority and Requirements
- authority:
  - `docs/plans/completed/mcp-v2-protocol-upgrade.md` backlog F2 (temporal recall: `--after/--before/--as-of` bi-temporal-lite read path).
  - `trusted-memory-spec.md` §6 claim lifecycle, transitions history, verified attestation timestamps.
  - Owner request: continue with next task from local-first memory roadmap.
- requirements:
  - R1 [accepted]: `TrustedQueryOptions` accepts optional `After`, `Before`, `AsOf` RFC3339 timestamp strings and validates format before query execution.
  - R2 [accepted]: `--after` filters claims to those verified/created at or after the timestamp ($\ge t$).
  - R3 [accepted]: `--before` filters claims to those verified/created at or before the timestamp ($\le t$).
  - R4 [accepted]: `--as-of` performs point-in-time filtering: claim verified $\le t$, not stale as of $t$ (`stale_after` empty or $> t$), and if superseded/revoked, the transition occurred $> t$.
  - R5 [accepted]: `zbrain ask` CLI flags `--after`, `--before`, `--as-of` and `memory_ask` MCP schema expose the filters.
  - R6 [accepted]: Pure Go, deterministic, no LLM/network calls; all tests, race detector, `make smoke`, and `CGO_ENABLED=0 go build` pass.

## Non-goals
- NG1: Natural language relative time parsing ("yesterday", "2 weeks ago") — strict RFC3339 ISO timestamps only.
- NG2: Write-path or indexing changes — filtering is evaluated on the query read path.
- NG3: Cross-workspace time travel — workspace isolation rules remain strict.
- NG4: Modifying historical claim files on disk — temporal query is strictly read-only.

## Approach and Risks
- approach: Implement temporal predicate evaluation in `internal/runtime/query.go`. When temporal filters are set, filter the retrieved candidate claims against their parsed RFC3339 timestamps (`VerifiedAt`, `CreatedAt`, `StaleAfter`, and `Transitions`). For `--as-of`, also allow claims whose historical transition timestamp indicates they were active at the requested time. Wire flags to CLI `internal/cli/cli.go` and MCP `internal/mcp/tools.go`.
- constraints:
  - RFC3339 parsing via standard library `time.Parse(time.RFC3339, ...)`.
  - Invalid timestamp strings must return explicit error before searching.
  - Fail-closed: if timestamp parsing of claim metadata fails, reject or fail query rather than silent bypass.
- risks:
  - Timezone / UTC mismatch: parse with `time.Parse(time.RFC3339, ...)` and compare using `.UTC().Equal()` / `.Before()` / `.After()`.
  - `--as-of` interaction with search index: index currently stores `status` from latest canonical state. When `--as-of` is active, include `ClaimStatusSuperseded` and `ClaimStatusRevoked` in the search query candidate set if searching for past active states, then evaluate point-in-time predicates against transitions.

## Phases and Verification
- planning_status: planned
- phases:
  - phase_slug: temporal-query-engine
    story_id: 01M0ZCZ87GHAE5Q9FGHBJACPPV
    status: checked
    goal: Implement temporal filtering logic in TrustedQuery and cover with unit tests
    depends_on: none
    requirements: [R1, R2, R3, R4, R6]
    allowed_surfaces: [internal/runtime/query.go, internal/runtime/query_test.go, internal/runtime/index.go]
    avoided_surfaces: [internal/runtime/claim.go, assets/]
    waves:
      - wave: W1
        goal: Temporal options validation and predicate filtering
        tasks:
          - id: W1.T1
            task: Add `After`, `Before`, `AsOf` fields to `TrustedQueryOptions` in `internal/runtime/query.go` with RFC3339 validation helper.
            depends_on: none
            expected_output: Invalid timestamp returns explicit validation error.
          - id: W1.T2
            task: Implement temporal filter predicates in `TrustedQuery` for `--after`, `--before`, and `--as-of` (including historical transitions evaluation).
            depends_on: W1.T1
            expected_output: Passing unit tests in `query_test.go` covering after, before, as-of, and combined time ranges.
    checks:
      - command: go test ./internal/runtime -run TestTrustedQueryTemporal -v
        expects: All temporal query tests pass.
      - command: go test ./...
        expects: All package tests pass.

  - phase_slug: temporal-cli-mcp-surface
    story_id: 01M0ZD098695ATGCPYSNBW5A2
    status: checked
    goal: Expose temporal flags on ask CLI command and memory_ask MCP tool
    depends_on: temporal-query-engine
    requirements: [R5, R6]
    allowed_surfaces: [internal/cli/cli.go, internal/cli/cli_test.go, internal/mcp/tools.go, internal/mcp/tools_test.go]
    avoided_surfaces: [internal/runtime/claim.go, assets/]
    waves:
      - wave: W1
        goal: Wire CLI flags and MCP tool parameters
        tasks:
          - id: W1.T1
            task: Add `--after`, `--before`, `--as-of` flags to `runAsk` and help text in `internal/cli/cli.go`, with CLI integration tests in `cli_test.go`.
            depends_on: none
            expected_output: `zbrain ask --after ...` executes temporal query and returns filtered results.
          - id: W1.T2
            task: Add `after`, `before`, `as_of` parameters to `memory_ask` tool definition and handler in `internal/mcp/tools.go`, with MCP tests in `tools_test.go`.
            depends_on: W1.T1
            expected_output: MCP tool passes temporal parameters through to `TrustedQuery`.
    checks:
      - command: go test ./...
        expects: Full test suite passes.
      - command: go test -race ./internal/runtime ./internal/cli ./internal/mcp ./internal/view
        expects: Race clean.
      - command: make smoke
        expects: Smoke tests pass.
      - command: CGO_ENABLED=0 go build ./cmd/zbrain
        expects: Builds without CGO.
      - command: git diff --check
        expects: Clean.

## Progress
- none
- 2026-08-28T08:50Z | phase: temporal-query-engine | wave: W1 | task: W1.T1 | task_status: done | run_id: none (zharness installer-only, no DB lifecycle) | verification: `TestTrustedQueryTemporalOptionsValidation` pass (invalid after/before/as_of and after > before rejected with explicit errors) | surfaces: internal/runtime/query.go, internal/runtime/query_test.go
- 2026-08-28T08:50Z | phase: temporal-query-engine | wave: W1 | task: W1.T2 | task_status: done | run_id: none | verification: `TestTrustedQueryTemporalAfterBeforeRange` and `TestTrustedQueryTemporalAsOfHistoricalAndStaleness` pass — temporal predicates accurately filter claims across verified_at, created_at, stale_after, and historical transitions | surfaces: internal/runtime/query.go, internal/runtime/query_test.go
- 2026-08-28T08:58Z | phase: temporal-cli-mcp-surface | wave: W1 | task: W1.T1 | task_status: done | run_id: none | verification: `TestRunAskTemporalFlags` pass in `cli_test.go`; `docs/proofs/surface.txt` snapshot updated and verified in `TestSurface` | surfaces: internal/cli/cli.go, internal/cli/cli_test.go, docs/proofs/surface.txt
- 2026-08-28T08:58Z | phase: temporal-cli-mcp-surface | wave: W1 | task: W1.T2 | task_status: done | run_id: none | verification: `TestToolInputSchemas` and `TestMemoryAskTemporalFiltering` pass in `internal/mcp/tools_test.go` | surfaces: internal/mcp/tools.go, internal/mcp/tools_test.go

## Decisions
- none

## Validation
- none
- 2026-08-28T08:50Z | phase: temporal-query-engine | `go test ./internal/runtime -run TestTrustedQueryTemporal -v` pass | `go test ./...` pass | `go vet ./...` clean | verdict: checked | proof_gaps: none
- 2026-08-28T08:58Z | phase: temporal-cli-mcp-surface | `go test ./...` pass (all packages) | `go vet ./...` clean | `go test -race ./internal/runtime ./internal/cli ./internal/mcp ./internal/view` pass | `make smoke` pass | `git diff --check` clean | `CGO_ENABLED=0 go build ./cmd/zbrain` ok | verdict: checked | proof_gaps: none

## Current State and Next Action
- active_phase: temporal-cli-mcp-surface
- lifecycle_status: checked
- latest_run_id: none (DB unavailable; zharness 0.15.0 is installer-only)
- latest_trace_ids: []
- latest_check_id: none
- latest_handoff_id: none
- blockers: none
- open_items: [all 2 phases completed and checked]
- exact_next_action: complete initiative
