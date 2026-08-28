# Conflict-aware drafts — F1 (MCP v2 backlog)

| | |
|---|---|
| Authored | 2026-08-26 |
| Status | Planned — `docs/plans/active/conflict-aware-drafts.md` locked (`01M0ZAT3BS4GS7X61V4ZZB9GY9`, lane `normal`) |
| Intake | `01M0ZAYX27CD3EKSN1CWTPT4XE` — `new-initiative` |
| Harness | `zharness query phases` shows 2 stories `planned` (contradiction-detection, conflict-surface); next `work full phase contradiction-detection` |

## Why

zbrain's trust model gates promotion through `claim approve` but `claim draft` and `ask` currently ignore contradictions — a draft that negates an approved claim is stored silently and retrieval never surfaces the conflict. For a local-first memory vault this is a poisoning vector: conflicting knowledge merges without audit. F1 makes contradictions explicit, deterministic, and visible without auto-resolution.

Upstream framing: mcp-v2-protocol-upgrade backlog (Exa research) ranks this #1 by fit; admission state machine + digest closure validation already exist — this adds the missing contradiction signal.

## What

- **Input:** `claim draft` in a workspace with approved claims
- **Heuristics (rule-based, no LLM/embed/network):** negation flip, value swap, status change over normalized `title`+`body`+`tags`
- **Storage:** advisory `contradicts: [{claim_id, heuristic}]` in draft frontmatter; preserved through `claim approve`/`reindex` via `Parse`/`Render` + `ClaimVerificationDigest`
- **Retrieval:** `ask` returns approved `ready` + contradicting drafts as `status: "conflict"` (or `conflict: true` + `conflicts_with`) instead of silently suppressing

## Non-goals (explicit)

- No auto-supersede/resolve (owner ceremony only)
- No semantic/embed/LLM detection
- No cross-workspace checks
- No ranking/context-packing/temporal filter changes (F2-F5)
- No MCP transport changes (stdio only, D2-D4)

## Checklist — Phases & Tasks

### Phase 1: `contradiction-detection` (story `01M0ZBG7SHDE95Q9FGHBJACPPV`, `planned`, depends none, req R1,R2,R4,R5)

- [ ] **W1.T1** `DetectContradictions` in `internal/runtime/claim.go` + table tests in `claim_test.go` (negation / value-swap / status-change + non-contradiction control)
- [ ] **W1.T2** `Contradiction` type + `contradicts` frontmatter + round-trip `Parse`/`Render` + digest coverage
- [ ] **W2.T1** Wire into `ClaimStore.Draft` / `claim draft` handler — persist hits to written draft file
- [ ] **W2.T2** `ScanWorkspace`/`IndexStore.Rebuild` preserve `contradicts`; approved stays `approved`

**Checks:** `go test ./...` → pass, `go vet` clean, `CGO_ENABLED=0 go build ./cmd/zbrain` builds. **Stop:** excessive false positives → narrow lexicon, report fixture.

### Phase 2: `conflict-surface` (story `01M0ZBGRD7695ATGCPYSNBW5A1`, `planned`, depends `contradiction-detection`, req R3,R5)

- [ ] **W1.T1** Extend `TrustedQuery` to return `contradicts`-bearing drafts with conflict signal; `query_test.go` both-sides query
- [ ] **W1.T2** `ask` JSON → `status: "conflict"` (or `conflict` fields) alongside `ready`/`gap`/`blocked`
- [ ] **W2.T1** Integration test: approved → contradicting draft → reindex → ask shows `ready`+`conflict`; clean draft shows no conflict

**Checks:** `go test ./...` + `go test -race ./internal/runtime ./internal/cli ./internal/mcp` + `make smoke` (setup→draft approved→draft contradicting→reindex→ask ready+conflict) + `git diff --check` clean. **Stop:** `ask` hot-path regression → revert.

## Next Actions

1. `work full phase contradiction-detection` — implement W1.T1 heuristics + W1.T2 type/round-trip, then W2 wiring; verify via phase checks.
2. `work full phase conflict-surface` — extend query + JSON mapping, add integration test.
3. `check full` on final phase, then `handoff` → squash-merge.

## Authority

- `docs/plans/completed/mcp-v2-protocol-upgrade.md` backlog F1
- Owner request 2026-08-26 (continue MCP v2)
- `docs/planning/MCP-V2-BRAINSTORM.md` agent-memory research
- `trusted-memory-spec.md` §6, §8, §9

## Approach (summary)

Scan approved claims at draft time (small, trust-gated set), normalize fields, apply three cheap heuristics, store advisory metadata. Retrieval includes conflict drafts without changing FTS/trust validation. Rejected: embedding, query-time scan, auto-supersede. Risks mitigated via narrow lexicon + bounded in-memory scan + round-trip preservation.
