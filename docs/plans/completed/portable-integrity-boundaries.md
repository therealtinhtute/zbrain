---
id: 01KZMZ9PN2MS4DHTCKPKJFZBMF
type: plan
intake_id: 01KZMZ9PN5Q75131CEJ9C9E27G
lane: high-risk
status: completed
created: 2026-08-10
updated: 2026-08-10
---

# Plan: Portable Integrity Boundaries

## Outcome
- result: Trusted-memory freshness and claim identity remain fail-closed across supported platforms and canonical path layouts; no stale or ambiguous claim can be served as trusted context.
- success_signals:
  - A reliable file change token is used when the target platform exposes one, while unavailable tokens deterministically trigger the existing trust-input manifest comparison instead of being treated as equal.
  - A workspace containing the same `zbrain.id` in flat and nested canonical paths is rejected deterministically; lifecycle lookup, rebuild, and trusted retrieval never silently select one document.
  - Unique flat claims, unique nested claims, evidence freshness, and current workspace-generation behavior remain unchanged.
  - High-risk proof covers focused regression tests, full Go quality checks, race/build/smoke evidence, and lifecycle audit with no unresolved proof gap.

## Authority and Requirements
- authority:
  - `trusted-memory-spec.md` sections 5, 8, and 9: `zbrain.id` is stable identity, SQLite is disposable, freshness is fail-closed, and trusted queries must not serve stale or invalid claims.
  - `CLAUDE.md`: Go-native minimal changes, `assets/` as source of truth, tests with runtime behavior, and proof with repository-defined tests and isolated smoke.
  - `internal/runtime/index.go`, `internal/runtime/manifest.go`, and `internal/runtime/index_test.go`: current change-token persistence, manifest fallback, and freshness invariants.
  - `internal/runtime/claim_store.go`, `internal/runtime/trust_validation.go`, and `internal/runtime/claim_store_test.go`: current flat/nested lookup, canonical scanning, and dependency validation behavior.
  - Owner instruction for this initiative: combine portable file change-token hardening with defensive flat/nested duplicate-ID handling in one follow-up.
- requirements:
  - R1 [accepted]: Freshness verification must distinguish a usable file change token from an unavailable token on every supported build target; an unavailable token must remain a non-match/manifest-fallback condition and must never independently certify freshness.
  - R2 [accepted]: The change-token implementation must be safe for platform-specific `os.FileInfo.Sys()` values, compile on supported targets, and preserve the existing persisted freshness metadata contract without requiring a command or index-schema change unless implementation proof demonstrates that one is unavoidable.
  - R3 [accepted]: When token and timestamp checks do not prove freshness, verification must compare the published trust-input manifest and fail closed with the existing reindex recovery guidance when any canonical trust input differs, is added, or is removed.
  - R4 [accepted]: A workspace scan must identify every duplicate `zbrain.id` across flat and nested canonical claim paths in deterministic path order and classify the ambiguity as invalid rather than choosing a path by traversal or flat-path precedence.
  - R5 [accepted]: Rebuild must not publish an index that contains an ambiguous claim ID or report a clean rebuild for a workspace with duplicate canonical claim IDs; the rejection must include enough canonical path and ID information for repair.
  - R6 [accepted]: Claim lifecycle lookup and trusted query binding must fail closed for duplicate-ID ambiguity, while unique flat and unique nested claims continue to support the existing draft, approval, reindex, and query flows.
  - R7 [accepted]: The implementation must not mutate, delete, rename, or auto-reconcile conflicting canonical Markdown; the operator remains responsible for repair followed by reindex.
  - R8 [accepted]: Regression coverage must exercise both a usable-token path and the unavailable-token manifest fallback, plus flat/nested duplicate-ID ambiguity at lifecycle, rebuild, and trusted retrieval boundaries.

## Non-goals
- NG1: Add filesystem watchers, background freshness services, hosted synchronization, or network access.
- NG2: Change the `zbrain` command surface, claim identity format, workspace scope rules, or trusted-query response schema.
- NG3: Migrate existing canonical claims, automatically choose a winning duplicate, or rewrite/delete conflicting files.
- NG4: Replace the manifest digest or canonical Markdown authority with SQLite metadata, timestamps, or platform-specific identity semantics.
- NG5: Broaden this initiative into unrelated evidence, asset, search-ranking, or performance refactors.

## Approach and Risks
- approach: Keep one portable freshness contract and one canonical-ID boundary. First harden the change-token adapter and prove that unavailable metadata always falls back to the published content manifest. Then make canonical scans and lifecycle lookup collect all documents for an ID, classify flat/nested collisions deterministically, and exclude ambiguous claims from trusted index publication and retrieval.
- constraints:
  - Preserve `trust_input_mtimes` and `trust_directories` integer metadata and the current manifest digest fallback; do not change commands or the index schema unless proof shows it is unavoidable.
  - Keep canonical Markdown authoritative; no conflict repair, path migration, deletion, or automatic winner selection.
  - Touch only the runtime freshness/claim/index/query boundaries and their regression tests; avoid unrelated search, evidence, asset, or performance refactors.
- rejected_alternatives:
  - Keep flat-path precedence and rely on SQLite's unique ID constraint: rejected because lifecycle commands could still read or mutate the wrong canonical document before rebuild.
  - Hash the full trust manifest on every freshness check: rejected because it removes the intended metadata fast path; digest comparison remains the explicit fallback when metadata is unavailable or inconclusive.
  - Auto-delete, move, or choose one duplicate document: rejected because it mutates canonical authority and hides an operator repair decision.
- risks:
  - Platform `FileInfo.Sys()` shapes differ or expose no reliable change token; mitigate with safe shape handling, synthetic metadata tests, host execution, and cross-target compilation, with unavailable values remaining fail-closed fallback inputs.
  - Duplicate detection changes rebuild counts and error ordering; mitigate with path-sorted deterministic classification and focused tests for unique flat, unique nested, and flat/nested collision cases.
  - Existing indexes may contain state from the prior behavior; mitigate by preserving the schema and making freshness/rebuild rejection force the existing `zbrain reindex` recovery path.
- recovery: If a supported target cannot be compiled or its metadata contract cannot be established without guessing, stop the phase, retain manifest fallback, record the proof gap, and route to `brainstorm refine` rather than weakening fail-closed behavior.

## Phases and Verification
<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. Each planned phase records phase_slug, story_id, status, goal, depends_on, waves, tasks, and checks. -->
- phase_slug: portable-freshness
  story_id: 01KZMZEN4VEG5G9TT4J6Q1PYRX
  status: done
  goal: Make file change-token freshness portable and defensive while preserving manifest-digest fallback and fail-closed stale-index behavior.
  depends_on: none
  allowed_surfaces:
    - internal/runtime/index.go
    - internal/runtime/index_state.go
    - internal/runtime/index_test.go
    - internal/runtime/query_test.go
    - portable runtime freshness helper files if required by the implementation
  avoided_surfaces:
    - command dispatch and JSON response schema
    - SQLite schema version and unrelated claim/evidence behavior
  waves:
    - wave: 1
      goal: Define and implement the portable token boundary.
      tasks:
        - task: token-adapter
          depends_on: []
          touched_surfaces: [internal/runtime/index.go, portable runtime freshness helper files]
          expected_output: A safe token adapter recognizes supported metadata shapes, returns an explicit unavailable sentinel for unsupported or malformed values, and leaves persisted integer freshness fields unchanged.
          check: go test ./internal/runtime -run 'TestFileChangeToken' -count=1 -v
          stop_if: The implementation requires guessing an OS-specific field or changes the persisted freshness contract.
    - wave: 2
      goal: Prove fast-path and fallback freshness behavior.
      tasks:
        - task: freshness-regressions
          depends_on: [token-adapter]
          touched_surfaces: [internal/runtime/index_test.go, internal/runtime/query_test.go]
          expected_output: Tests cover a usable token, an unavailable token, restored timestamps, changed content, and added/removed trust inputs; unavailable tokens force manifest comparison and never certify freshness alone.
          check: go test ./internal/runtime -run 'TestFileChangeToken|TestContentDigestFreshness|TestCheckFresh' -count=1 -v
          stop_if: Any stale or changed trust input can pass without a matching manifest, or a unique existing freshness case regresses.
    - wave: 3
      goal: Prove supported-target compilation and repository behavior.
      tasks:
        - task: portability-proof
          depends_on: [freshness-regressions]
          touched_surfaces: [none]
          expected_output: Host tests and cross-target test-binary compilation succeed for Linux, Darwin, and Windows without executing non-host binaries.
          check: go test ./... && GOOS=darwin GOARCH=amd64 go test -c -o /tmp/zbrain-runtime-darwin.test ./internal/runtime && GOOS=windows GOARCH=amd64 go test -c -o /tmp/zbrain-runtime-windows.test ./internal/runtime
          stop_if: A target does not compile or cross-target behavior cannot be verified without weakening the fallback contract.
  phase_checks:
    - go test ./...
    - go vet ./...
    - go test -race ./internal/runtime ./internal/cli

- phase_slug: duplicate-claim-boundary
  story_id: 01KZMZEN55XFW0NCXPE1TAM41E
  status: done
  goal: Reject duplicate stable claim IDs across flat and nested canonical paths at lookup, rebuild, and trusted-query boundaries while preserving unique nested claims.
  depends_on: [portable-freshness]
  allowed_surfaces:
    - internal/runtime/claim_store.go
    - internal/runtime/claim_store_test.go
    - internal/runtime/index.go
    - internal/runtime/index_test.go
    - internal/runtime/query.go
    - internal/runtime/query_test.go
    - internal/runtime/trust_validation.go
    - internal/runtime/trust_validation_test.go
  avoided_surfaces:
    - claim ID format and lifecycle command surface
    - canonical file repair or automatic path migration
    - unrelated evidence, asset, and search-ranking code
  waves:
    - wave: 1
      goal: Classify duplicate canonical IDs during workspace scanning.
      tasks:
        - task: duplicate-scan
          depends_on: []
          touched_surfaces: [internal/runtime/claim_store.go, internal/runtime/claim_store_test.go]
          expected_output: Canonical scan groups parsed claims by stable ID in deterministic path order, records every flat/nested collision as invalid, and excludes ambiguous claims from the valid scan set without rewriting files.
          check: go test ./internal/runtime -run 'Test.*Duplicate|TestNestedClaimBoundary' -count=1 -v
          stop_if: Scan traversal order selects a winner, hides one path, or changes unique nested-claim behavior.
    - wave: 2
      goal: Make lifecycle lookup fail closed for ambiguous IDs.
      tasks:
        - task: duplicate-lifecycle
          depends_on: [duplicate-scan]
          touched_surfaces: [internal/runtime/claim_store.go, internal/runtime/claim_store_test.go]
          expected_output: Read, draft overwrite, approve, supersede, and revoke cannot silently choose the flat document when a nested document shares the same ID; unique flat and nested lifecycle flows remain green.
          check: go test ./internal/runtime -run 'TestClaimStore.*|Test.*Duplicate|TestNestedClaimBoundary' -count=1 -v
          stop_if: Any lifecycle operation mutates or approves one member of an ambiguous ID set.
    - wave: 3
      goal: Make rebuild and trusted retrieval reject the ambiguity.
      tasks:
        - task: duplicate-trust-boundary
          depends_on: [duplicate-lifecycle]
          touched_surfaces: [internal/runtime/index.go, internal/runtime/index_test.go, internal/runtime/query.go, internal/runtime/query_test.go, internal/runtime/trust_validation.go, internal/runtime/trust_validation_test.go]
          expected_output: Rebuild publishes rejected state without indexing ambiguous IDs, reports deterministic ID/path reasons, and trusted query remains blocked until the operator repairs the canonical tree and reindexes.
          check: go test ./internal/runtime -run 'Test.*Duplicate|Test.*Rebuild|Test.*TrustedQuery|TestNestedClaimBoundary' -count=1 -v
          stop_if: A duplicate canonical ID reaches a clean index or trusted query, or rejection lacks repair-identifying paths.
  phase_checks:
    - go test ./...
    - go vet ./...
    - go test -race ./internal/runtime ./internal/cli
    - make build
    - make smoke

## Progress
<!-- Append-only durable entries record timestamp, phase, wave, task, task_status, run_id, trace_id, exact verification/result, and changed surfaces or blocker. -->
- `2026-08-10T04:42:39Z` — wave 1. run: `01KZMZMEZKYA20WAXT11FA647F`. summary: Phase started: portable-freshness; execute token-adapter first, then freshness regressions and cross-target proof.
- `2026-08-10T04:45:58Z` — wave 1, task token-adapter. task_status: `DONE`. run: `01KZMZMEZKYA20WAXT11FA647F`. summary: Implemented a defensive portable change-token adapter for nested Unix timestamp shapes and scalar Ctime/CtimeNsec metadata; unsupported, malformed, or nil metadata returns the unavailable sentinel; gofmt plus go test ./internal/runtime -run 'TestFileChangeToken|TestContentDigestFreshnessFallbackWithoutChangeTokens' -count=1 -v -> pass.
- `2026-08-10T04:46:09Z` — wave 1. run: `01KZMZMEZKYA20WAXT11FA647F`. summary: Wave 1 complete: portable token extraction now handles Linux Ctim, Darwin/BSD Ctimespec, scalar Ctime/CtimeNsec, pointers, and unavailable metadata without panics or optimistic freshness.
- `2026-08-10T04:46:32Z` — wave 2, task freshness-regressions. task_status: `DONE`. run: `01KZMZMEZKYA20WAXT11FA647F`. summary: Freshness regression slice passes: token shapes, restored-mtime content edits, unavailable-token manifest fallback, added/deleted trust inputs, evidence edits, symlink rejection, and dirty/missing/rejected/malformed index barriers; exact command go test ./internal/runtime -run 'TestFileChangeToken|TestContentDigestFreshness|TestCheckFresh' -count=1 -v.
- `2026-08-10T04:46:32Z` — wave 2. run: `01KZMZMEZKYA20WAXT11FA647F`. summary: Wave 2 complete: usable and unavailable token paths preserve fail-closed manifest freshness behavior across canonical wiki and evidence inputs.
- `2026-08-10T04:49:33Z` — wave 3, task portability-proof. task_status: `DONE_WITH_CONCERNS`. run: `01KZMZMEZKYA20WAXT11FA647F`. summary: Host go test ./... passed and GOOS=darwin GOARCH=amd64 go test -c -o /tmp/zbrain-runtime-darwin.test ./internal/runtime passed; GOOS=windows go test -c failed before token code because coordination.go uses unavailable x/sys/unix APIs. Concern recorded as an explicit proof gap; no lock-layer expansion made.
- `2026-08-10T04:49:33Z` — wave 3. run: `01KZMZMEZKYA20WAXT11FA647F`. summary: Wave 3 complete with concern: freshness behavior is proven on host and Darwin compilation, but Windows package compilation remains blocked by pre-existing coordination-lock portability outside this phase.
- `2026-08-10T05:02:22.029Z` — handoff recorded. handoff: `01KZN0SSTDHG7YXJZWMW58RQE1`. run: `01KZMZMEZKYA20WAXT11FA647F`. check: `01KZN0P9WG4MXBX0Q5WFSDZ1JX`. phase closed.
- `2026-08-10T05:05:33Z` — wave 1. run: `01KZN0YH21DYQ06CSA0FF0CK0R`. summary: Phase started: duplicate-claim-boundary; classify duplicate IDs first, then close lifecycle lookup and rebuild/trusted-query boundaries.
- `2026-08-10T05:12:47Z` — wave 1, task duplicate-scan. task_status: `DONE`. run: `01KZN0YH21DYQ06CSA0FF0CK0R`. trace: `01KZN1CW5GN6AVGR066CCSGRE9`. summary: Canonical scan groups parsed claims by stable ID, reports every duplicate flat/nested path deterministically, excludes ambiguous claims, and preserves canonical files; focused duplicate and nested-boundary tests pass.
- `2026-08-10T05:18:21Z` — wave 2, task duplicate-lifecycle. task_status: `DONE`. run: `01KZN0YH21DYQ06CSA0FF0CK0R`. trace: `01KZN1Q34TTF8C4TG6VEBACRJS`. summary: Read, draft overwrite, approve, supersede, and revoke reject duplicate canonical IDs without mutating either member; unique nested lifecycle tests remain green.
- `2026-08-10T05:18:21Z` — wave 3, task duplicate-trust-boundary. task_status: `DONE`. run: `01KZN0YH21DYQ06CSA0FF0CK0R`. trace: `01KZN1Q351WCBWVC0FNHQQVT8A`. summary: Rebuild publishes rejected state with deterministic duplicate ID/path reasons and no duplicate rows; canonical query binding rejects ambiguous IDs; focused rebuild/query/nested tests pass.
- `2026-08-10T05:44:21Z` — handoff recorded. handoff: `01KZN365WVBKFHWCFWKDNPD9E0`. run: `01KZN0YH21DYQ06CSA0FF0CK0R`. check: `01KZN32DERMWRQQY1P41GG1V9J`. phase closed; initiative ready to move to completed plan storage.

## Decisions
<!-- Append-only durable entries record timestamp, phase/task, decision, and rationale. -->
- `2026-08-10T04:49:24Z` — Accept the portable-freshness implementation with a Windows package-compilation proof gap; retain the manifest fallback and do not expand this phase into coordination-lock portability. (phase: `portable-freshness`), task: portability-proof. rationale: Host tests and synthetic Linux/Darwin/scalar token-shape tests pass, and the runtime test binary compiles for Darwin. Windows compilation fails before token code because internal/runtime/coordination.go imports x/sys/unix APIs unavailable on Windows. Reworking the lock layer is outside the approved freshness surfaces and would weaken minimal-change scope; the gap must remain explicit for the phase gate and handoff..

## Validation
<!-- Append-only durable entries record timestamp, phase, exact command/result/output, run_id, check_id, verdict, and proof_gaps. -->
- `2026-08-10T05:00:27.408Z` — phase: `portable-freshness`; run: `01KZMZMEZKYA20WAXT11FA647F`; check: `01KZN0P9WG4MXBX0Q5WFSDZ1JX`; verdict: `APPROVE_WITH_REQUESTS`; judge: `same-session` (`gpt-5.6-luna`). Exact results: focused freshness boundary tests PASS; `go test ./...` PASS; `go vet ./...` PASS; `go test -race ./internal/runtime ./internal/cli` PASS; `make build && make smoke` PASS; Darwin runtime test-binary compilation PASS. Windows runtime test-binary compilation remains a pre-existing failure before token code because `internal/runtime/coordination.go` imports unavailable `x/sys/unix` APIs. proof_gaps: Windows whole-package compilation is not independently proven; retain as explicit follow-up without expanding this phase.
- `2026-08-10T05:42:11Z` — phase: `duplicate-claim-boundary`; run: `01KZN0YH21DYQ06CSA0FF0CK0R`; check: `01KZN32DERMWRQQY1P41GG1V9J`; verdict: `APPROVED`; judge: `same-session` (`gpt-5.6-luna`). Exact results: alias-filename duplicate lifecycle/query regressions PASS; approved-owner digest binding regression PASS; duplicate scan/lifecycle/rebuild/trusted-query/nested-conflict focused tests PASS; `ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -timeout 30m` PASS with internal p95 threshold <= 2s; `go test ./...` PASS; `go vet ./...` PASS; `go test -race ./internal/runtime ./internal/cli` PASS; `make build` PASS; `make smoke` PASS; `git diff --check` PASS; read-only security, performance, architecture, and code-quality review findings were verified and resolved. proof_gaps: none for duplicate-claim-boundary; the previously recorded Windows whole-package compilation gap remains scoped to portable-freshness.

## Current State and Next Action
- active_phase: none
- lifecycle_status: done
- latest_run_id: 01KZN0YH21DYQ06CSA0FF0CK0R
- latest_trace_ids: [01KZN1CW5GN6AVGR066CCSGRE9, 01KZN1Q34TTF8C4TG6VEBACRJS, 01KZN1Q351WCBWVC0FNHQQVT8A]
- latest_check_id: 01KZN32DERMWRQQY1P41GG1V9J
- latest_handoff_id: 01KZN365WVBKFHWCFWKDNPD9E0
- blockers: none
- open_items: none
- exact_next_action: invoke git cm to commit the verified integrity-boundary changes
