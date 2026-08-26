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
- approach: not-planned
- constraints:
  - none
- risks:
  - none

## Phases and Verification
<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. Each planned phase records phase_slug, story_id, status, goal, depends_on, waves, tasks, and checks. -->
- planning_status: not-planned
- phases: none

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
- active_phase: none
- lifecycle_status: not-planned
- latest_run_id: none
- latest_trace_ids: []
- latest_check_id: none
- latest_handoff_id: none
- blockers: none
- open_items: [to-plan must define stable phases, stories, waves, tasks, and checks]
- exact_next_action: to-plan
