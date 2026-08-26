---
id: 01M0ZAT3BS4GS7X61V4ZZB9GY9
type: plan
intake_id: 01M0ZAYX27CD3EKSN1CWTPT4XE
lane: normal
status: active
created: 2026-08-26
updated: 2026-08-26
---

# Plan: Conflict-aware drafts

## Outcome
- result: `claim draft` deterministically flags contradictions against approved claims at write time and `ask` surfaces `status: "conflict"` candidates instead of silently merging conflicting knowledge.
- success_signals:
  - Creating a draft that negates, swaps a value of, or flips status of an existing approved claim is recorded with contradiction metadata pointing at the conflicting approved claim(s).
  - `ask` for a query matching both sides returns the approved claim as `ready` and the draft(s) marked with conflict info, without auto-resolving which is true.
  - Existing `claim draft` → `claim approve` → `reindex` → `ask` happy path is unchanged when no contradiction exists.

## Authority and Requirements
- authority:
  - `docs/plans/completed/mcp-v2-protocol-upgrade.md` backlog F1 (ordered by fit with trust model; deterministic, no LLM/network, canonical mutations only through ceremony)
  - Owner request 2026-08-26: continue MCP v2 track; brainstorm selected conflict-aware drafts as next initiative (F1)
  - `docs/planning/MCP-V2-BRAINSTORM.md` §Agent memory research — admission state machine and poisoning defense; zbrain already has claim lifecycle supersession, digest closure validation, FTS5
  - `trusted-memory-spec.md` §6 claim lifecycle (`draft -> approved -> superseded|revoked`), §8 trust validation at reindex, §9 fail-closed gaps
- requirements:
  - R1 [accepted]: `claim draft` evaluates deterministic contradiction heuristics (negation, value swap, status change) against approved claims in the same workspace and records any hits as draft metadata (conflicting claim ids + heuristic kind). | source: mcp-v2 backlog F1
  - R2 [accepted]: Contradiction metadata is advisory only: it does not change claim `status`, does not block `claim draft`, and is preserved through `claim approve`/`reindex` without mutating canonical approved content. | source: trust model — drafts are untrusted candidates
  - R3 [accepted]: `ask` retrieval surfaces drafts that carry contradiction metadata with `status: "conflict"` (or equivalent conflict candidate signal) alongside the conflicting approved `ready` result, instead of silently suppressing or merging. | source: mcp-v2 backlog F1 spec
  - R4 [accepted]: No LLM, embedding, or network call is introduced; detection is rule-based over normalized claim fields available at draft time. | source: mcp-v2 backlog constraints (deterministic)
  - R5 [accepted]: Existing lifecycle and file modes unchanged; `go test ./...`, `go vet`, `CGO_ENABLED=0 go build`, and `make smoke` remain green with new regression tests for contradiction cases. | source: CLAUDE.md implementation rules

## Non-goals
- NG1: Auto-resolving or auto-superseding on contradiction — owner ceremony (`claim approve`/`supersede`) remains the only promotion path.
- NG2: Semantic/embedding similarity or LLM-based contradiction detection — rule heuristics only in this initiative.
- NG3: Cross-workspace contradiction checks — same-workspace only; secondary workspace inclusion via `ask --include` is read-only and out of scope for draft-time detection.
- NG4: Changing `ask` ranking, context packing, temporal filters, or consolidation proposals — deferred to F2-F5.
- NG5: New transports, Tasks/MRTR, or MCP surface changes — stdio gateway unchanged (per mcp-v2 D2-D4).

## Approach and Risks
- approach: At `claim draft` time, load approved claims in the same workspace, normalize their `title`+`body`+`tags` (lowercase, trim), and apply three deterministic heuristics against the incoming draft: negation flip (not/no/never), value swap (same subject different object), status change. Record hits as advisory `contradicts: [{claim_id, heuristic}]` in draft frontmatter. On retrieval, include those drafts with a `conflict` signal alongside the `ready` approved result; do not change FTS index, trust validation, or lifecycle promotion. Keeps detection cheap (approved set is small) and preserves single-file claim truth.
- constraints:
  - Go-native only; no Bun/Node/TS; no new dependencies beyond `crypto/sha256` already used for digests.
  - Markdown canonical, SQLite disposable; contradictions are draft metadata only, never canonical approved content.
  - No LLM/embed/network; rule-based only (R4).
  - Deletions via `trash` only if needed; file modes unchanged.
- risks:
  - Heuristic false positives/negatives if patterns too broad/narrow. Mitigation: narrow negation lexicon + exact value comparison, with regression tests for each heuristic and a non-contradiction control; tune only via tests.
  - Draft-time scan of all approved claims could grow with workspace size. Mitigation: approved set stays small (trust-gated); scan is bounded and in-memory; no index added in this phase.
  - Metadata lost on `claim approve`/`reindex` if not preserved. Mitigation: treat `contradicts` as preserved frontmatter through `Parse`/`Render` and `ClaimVerificationDigest` — cover with round-trip test.
- rejected_alternatives:
  - Embedding/semantic similarity: non-deterministic, requires model/network, violates R4.
  - Query-time index scan for conflicts: duplicates draft-time work and couples ask hot path to extra parsing.
  - Auto-supersede on detection: violates trust model (NG1) — owner ceremony stays authoritative.

## Phases and Verification
<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. Each planned phase records phase_slug, story_id, status, goal, depends_on, waves, tasks, and checks. -->
- planning_status: planned
- phases:
  - phase_slug: contradiction-detection
    story_id: 01M0ZBG7SHDE95Q9FGHBJACPPV
    status: planned
    goal: Detect negation/value-swap/status-change contradictions at claim draft time and record advisory metadata
    depends_on: none
    requirements: [R1, R2, R4, R5]
    allowed_surfaces: [internal/runtime/claim.go, internal/runtime/claim_store.go, internal/runtime/claim_test.go, internal/runtime/claim_store_test.go, assets/]
    avoided_surfaces: [internal/runtime/query.go, internal/runtime/index.go, internal/mcp/]
    waves:
      - wave: W1
        goal: Define heuristics with isolated unit coverage
        tasks:
          - id: W1.T1
            task: Add `DetectContradictions(draft, approvedClaims) []Contradiction` in `internal/runtime/claim.go` implementing negation/value-swap/status-change checks over normalized fields, plus table tests in `claim_test.go` for each heuristic and a non-contradiction control.
            depends_on: none
            expected_output: Passing unit tests proving each heuristic triggers only on intended cases.
          - id: W1.T2
            task: Add `Contradiction` type and `contradicts` frontmatter field to claim model; ensure `ParseClaimMarkdown`/`RenderClaimMarkdown` round-trip preserves it and `ClaimVerificationDigest` covers it.
            depends_on: W1.T1
            expected_output: Round-trip test draft → render → parse keeps `contradicts` and digest stable.
      - wave: W2
        goal: Wire detection into draft creation path
        tasks:
          - id: W2.T1
            task: Call `DetectContradictions` inside `ClaimStore.Draft` (or `claim draft` CLI handler) after parsing incoming draft and before writing; persist any hits into the written draft file's frontmatter.
            depends_on: W1.T2
            expected_output: `claim draft` writing a contradicting draft produces a file whose frontmatter lists the conflicting approved id(s).
          - id: W2.T2
            task: Ensure `ClaimStore.ScanWorkspace` and `IndexStore.Rebuild` preserve `contradicts` through reindex without treating it as invalid; approved claims remain `approved` regardless of being contradicted.
            depends_on: W2.T1
            expected_output: `zbrain reindex` keeps draft `contradicts` and does not mark approved as invalid.
    checks:
      - command: go test ./...
        expects: All packages pass including new contradiction and round-trip tests.
      - command: go vet ./...
        expects: Clean.
      - command: CGO_ENABLED=0 go build ./cmd/zbrain
        expects: Builds.
    stop_condition: W1 heuristic tests show excessive false positives on realistic fixtures; stop and narrow lexicon before wiring W2.
    escalation: Report failing fixture to owner; do not broaden heuristics to make test pass.

  - phase_slug: conflict-surface
    story_id: 01M0ZBGRD7695ATGCPYSNBW5A1
    status: planned
    goal: Surface contradiction drafts as status conflict alongside ready approved claims in ask retrieval
    depends_on: contradiction-detection
    requirements: [R3, R5]
    allowed_surfaces: [internal/runtime/query.go, internal/runtime/index.go, internal/runtime/query_test.go, internal/runtime/index_test.go, internal/cli/cli_test.go]
    avoided_surfaces: [internal/runtime/claim.go, assets/]
    waves:
      - wave: W1
        goal: Include conflict drafts in trusted query results
        tasks:
          - id: W1.T1
            task: Extend `TrustedQuery` (or `ScanWorkspace` index include) to return drafts carrying `contradicts` as separate results with conflict signal, while keeping approved `ready` result unchanged; add `query_test.go` covering both-sides query.
            depends_on: none
            expected_output: `zbrain ask` for a query matching both returns approved `ready` + draft `conflict`.
          - id: W1.T2
            task: Update `ask` JSON mapping to expose `status: "conflict"` (or `conflict: true` + `conflicts_with`) for those drafts; keep existing `ready`/`gap`/`blocked` semantics.
            depends_on: W1.T1
            expected_output: JSON output distinguishes `ready` vs `conflict` without breaking existing clients.
      - wave: W2
        goal: Prove end-to-end and non-regression
        tasks:
          - id: W2.T1
            task: Add smoke-like integration test: draft approved claim, draft contradicting draft, reindex, ask — assert `ready` + `conflict` present; then verify clean case (non-contradicting draft) leaves `ask` unchanged.
            depends_on: W1.T2
            expected_output: Integration test passes; clean path unaffected.
    checks:
      - command: go test ./...
        expects: All packages pass including query conflict tests.
      - command: go test -race ./internal/runtime ./internal/cli ./internal/mcp
        expects: Race clean.
      - command: make smoke
        expects: Exit 0 — setup → draft approved → draft contradicting → reindex → ask shows ready+conflict, then clean draft shows no conflict.
      - command: git diff --check
        expects: Clean.
    stop_condition: `ask` hot path regresses p95 or breaks existing `ready`/`gap` cases; stop and revert query change.
    escalation: Report to owner; do not suppress `ready` result to surface `conflict`.

## Progress
<!-- Append-only durable entries record timestamp, phase, wave, task, task_status, run_id, trace_id, exact verification/result, and changed surfaces or blocker. -->
- none

## Decisions
<!-- Append-only durable entries record timestamp, phase/task, decision, and rationale. -->
- none

## Validation
<!-- Append-only durable entries record timestamp, phase, exact command/result/output, run_id, check_id, verdict, and proof_gaps. -->
- none

## Current State and Next Action
- active_phase: contradiction-detection
- lifecycle_status: planned
- latest_run_id: none
- latest_trace_ids: []
- latest_check_id: none
- latest_handoff_id: none
- blockers: none
- open_items: [run work full phase contradiction-detection]
- exact_next_action: work full phase contradiction-detection
