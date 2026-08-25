# Plan: MCP v2 (2026-07-28) Protocol Upgrade

**Status:** active · **Created:** 2026-08-25 ·
**Authority:** owner request (research MCP v2 via Exa, plan implementation) ·
**Amends:** `docs/trusted-agent-gateway-spec.md` §MCP surface

## Outcome

zbrain's `mcp serve` gateway speaks both the legacy handshake era
(`2024-11-05` … `2025-11-25`) and the stateless `2026-07-28` revision
("MCP v2") by upgrading `github.com/modelcontextprotocol/go-sdk` from
v1.4.1 to v1.7.x, without changing the seven-tool surface, the trust
invariants, or the stdio-only transport.

## Research basis (Exa, 2026-08)

### What "MCP v2" is — spec `2026-07-28`

Released 2026-07-28 with coordinated SDK updates (TS/Py/Go/C#). The Go SDK
v1.7.0 (2026-07-28) implements it; compatibility table: v1.7.0+ →
`2026-07-28`, `2025-11-25`, `2025-06-18`, `2025-03-26`, `2024-11-05`.
GitHub's MCP server already runs the pre-release in production.

Core changes:

| Change | SEP | Effect on zbrain |
|---|---|---|
| Stateless protocol: no `initialize`/`initialized`; per-request `_meta` carries `protocolVersion`, `clientCapabilities`, optional `clientInfo` | SEP-2575 | Server-side only; SDK handles. Tests must cover new wire shape. |
| `server/discover` RPC replaces handshake for capability/version discovery | SEP-2575 | Free via SDK (`Server.Connect` registers it); must be asserted in tests. |
| Sessions removed on 2026 path (no `Mcp-Session-Id`); HTTP requires `Stateless = true` for 2026 traffic | SEP-2567 | No impact: stdio only. |
| MRTR: server-initiated elicitation/sampling replaced by `input_required` result + client re-issue with `requestState` | SEP-2322 | Not adopted — see Decisions. |
| Tasks moved out of core into `io.modelcontextprotocol/tasks` extension; poll-based `tasks/get` + `tasks/update` | SEP-2663 | Not adopted (no long-running tools; `memory_reindex` stays synchronous and bounded). |
| Unified `subscriptions/listen` stream replaces free-floating change notifications | 2026-07-28 | Not advertised (zbrain has no change notifications today). |
| Standardized HTTP headers (`Mcp-Method`, `Mcp-Name`, `Mcp-Param-*`) | SEP-2243 | N/A for stdio. |
| roots / sampling / logging deprecated | SEP-2577 | zbrain never used them. |
| Cache hints (`ttlMs`, `cacheScope`) on list responses | SEP-2549 | Accept SDK defaults; no custom work. |
| JSON-RPC error codes realigned with spec (SDK #1016) | 2026-07-28 | Re-verify `-32602` invalid params / `-32603` internal mapping contract. |

Legacy compatibility: over the old `initialize` handshake the negotiated
version caps at `2025-11-25` even if a newer client offers more; a client
offering exactly `2025-06-18` still settles on `2025-06-18`. New-era clients
probe `server/discover` first and fall back to legacy initialize.

### Agent memory, local-first — feature landscape

Production archetypes (2026): Mem0 (write-time extraction + dedup),
Zep/Graphiti (bi-temporal knowledge graph, fact invalidation),
Letta/MemGPT (OS-style tiers, agent self-edits), LangMem (primitives).
Local-first OSS wave (smysle/agent-memory, remembrane, local-memory-mcp,
aver, Awareness) converges on: SQLite(+WAL)+FTS5 single-file stores,
hybrid BM25+vector+recency+importance ranking (RRF fusion), write-time
dedup/conflict detection, decay+consolidation lifecycle, provenance per
record, feedback signals, temporal recall filters, budget-capped context
packing. Academic framing (unified framework arXiv 2604.01707): four
stages — extraction, management, storage, retrieval; management ops are
connect/integrate/update/filter/transform.

Security consensus: memory poisoning is a first-class threat; admission
state machine (candidate → active → consolidated/superseded/quarantined)
with explicit audit transitions beats silent merging; deletion must
propagate to indexes/caches with receipts; decay must recompute from an
immutable base score plus checkpoint, never subtract repeatedly from an
already-decayed value.

**zbrain already has** (trust-gated equivalents): claim lifecycle
supersession (= temporal invalidation), immutable evidence + spans
(= provenance), digest closure validation (= poisoning defense), FTS5 +
opt-in hybrid embed sidecar, workspace isolation, status/doctor
diagnostics, owner-pinned ceremony for every canonical mutation.

### Feature backlog for local-first memory (follow-up initiative, not this plan)

Ordered by fit with the trust model; all deterministic, no LLM/network,
canonical mutations only through the existing ceremony:

1. **F1 Conflict-aware drafts** — deterministic contradiction heuristics
   (negation, value swap, status change) against approved claims at
   `claim draft` time; record as draft metadata; `ask` surfaces
   `status: "conflict"` candidates instead of silently resolving.
2. **F2 Temporal recall** — `ask --after/--before/--as-of` filtering on
   verified-at/observed-at (bi-temporal-lite read path).
3. **F3 Usefulness ranking** — read-time score = base quality × freshness
   (from immutable base + `last_evaluated_at` checkpoint) × use factor;
   optional anonymous use-feedback counter recorded at ask time (derived,
   never canonical).
4. **F4 Context packing** — `memory_pack` MCP tool: budget-capped greedy
   selection over ranked claims, near-dup suppression, token estimate.
5. **F5 Consolidation proposals** — cluster near-duplicate drafts/approved
   claims; emit a proposed supersede set consumed by the owner ceremony
   (one challenge covers the set); never auto-applied.
6. **F6 Doctor additions** — stale-recall rate, revocation propagation
   check (revoked/superseded absent from derived index), conflict-candidate
   counts.

## Non-goals

- Streamable HTTP transport of any kind (loopback or otherwise) — stdio
  remains the only transport this plan; revisit behind a separate spec.
- Tasks extension, MRTR adoption, subscriptions/listen advertisement.
- Any change to tool schemas, trust validation, file modes, or the owner
  ceremony (CLI TTY stays the only grant path).

## Approach and Risks

Upgrade the dependency, then prove dual-era behavior at the wire level.
The existing subprocess test harness in `internal/mcp/mcp_test.go` already
asserts raw stdout frames against `protocolVersion: 2025-06-18`; extend it
into a revision matrix instead of rewriting.

Risks:

- **SDK wire rewrite regressions on stdio**: v1.7.0 rewrote much of the
  wire layer. Mitigation: extend the raw-frame matrix (legacy initialize,
  legacy capped negotiate, `server/discover`, 2026 `_meta` request) before
  touching anything else.
- **Error-code drift** (SDK #1016): `tools.go` maps domain failures to
  `isError`, bad params to `-32602`, server faults to `-32603`. Re-run the
  error-mapping tests and amend if codes moved.
- **Legacy negotiation surprise**: a `2025-06-18`-only client must still
  get `2025-06-18`. Assert explicitly.
- **Stdout purity**: any stray SDK log line on stdout breaks clients.
  Keep `Logger` on stderr handler; smoke asserts stdout lines are all
  valid JSON-RPC frames (existing check).

## Phases and Verification

Phase 0 — Baseline (done):
- [x] `go test ./internal/mcp -count=1` green on go-sdk v1.4.1 (2026-08-25).
- [x] Full gate recorded in prior release proof (commit `e85c3ae`).

Phase 1 — Dependency bump (done):
- [x] `go.mod`: go-sdk v1.4.1 → v1.7.0 (transitives: jsonschema-go v0.4.3,
  oauth2 v0.35.0, sync v0.20.0, sys v0.41.0).
- [x] One behavioral fix: SDK v1.7 maps generated-schema input violations to
  tool-level `isError` instead of a `-32602` wire error; `TestToolSurface`
  updated (schema violation → isError, empty query guard → isError, unknown
  tool → `-32602`); bounds-check `-32602` unchanged; comment updated.
- [x] Verified: `CGO_ENABLED=0 go build ./cmd/zbrain`, mcp package green.

Phase 2 — Dual-era conformance tests (done):
- [x] New `TestProtocolRevisionMatrix` in `internal/mcp/mcp_test.go` drives
  raw stdio frames against a subprocess server over a real runtime:
  legacy negotiate `2025-06-18`; legacy cap at `2025-11-25`;
  `server/discover` advertises `2026-07-28` + capabilities; stateless
  `tools/call workspace_current` succeeds without any handshake; missing
  `_meta.clientCapabilities` → `-32602`; future version → `-32022`.
- [x] Error-mapping regression: domain → `isError`, oversized → `-32602`,
  unknown tool → `-32602` (existing tests + updated TestToolSurface).

Phase 3 — Surface review (done):
- [x] Advertised capabilities are SDK defaults (`logging`,
  `resources.listChanged`, `tools.listChanged`) — identical semantics to
  pre-upgrade; no tasks/MRTR/subscriptions advertised (see D6).
- [x] `tools/list` shape untouched: tool names, schemas, and
  `schema_version` payloads unchanged (TestToolSurface asserts surface).
- [x] Stdout purity re-proven on real pipes (`TestStdoutPurity`, subprocess).

Phase 4 — Docs + version (done):
- [x] `docs/trusted-agent-gateway-spec.md`: revisions paragraph, error-map
  amendment, non-adoption note.
- [x] `README.md` gateway bullet + `AGENTS.md` gotcha line updated.
- [x] `internal/cli/cli.go` Version `0.2.3` → `0.3.0`.

Phase 5 — Release gate (done, CI order, 2026-08-25):
- [x] `go test ./...` all green → `go vet ./...` clean →
  `go test -race` (runtime/cli/view/mcp) green →
  `make build` (dist/zbrain 0.3.0) → `make smoke` exit 0 →
  `git diff --check` clean → `CGO_ENABLED=0 go build ./cmd/zbrain` ok.

## Decisions

- **D1 Adopt SDK-level upgrade only; no hand-rolled protocol code.** The Go
  SDK negotiates eras transparently; zbrain keeps thin handlers.
- **D2 Reject MRTR for lifecycle confirmation.** Routing the owner grant
  through agent-mediated multi-round-trip would move the trust ceremony
  into the agent loop; the CLI TTY ceremony stays authoritative.
- **D3 Reject tasks extension.** All seven tools are bounded, synchronous,
  local operations; `memory_reindex` is the longest and stays synchronous
  with its `.dirty` marker as progress state.
- **D4 Keep stdio as sole transport.** Stateless 2026 HTTP would ease
  remote use but violates the no-remote-bind invariant; defer.
- **D5 Feature backlog F1–F6 tracked separately**, not bundled into the
  protocol bump (minimal change; independent verification).
- **D6 Keep SDK-default capability advertisement.** `logging`,
  `resources.listChanged`, `tools.listChanged` match pre-upgrade semantics;
  the SDK itself serves the logging-level paths (legacy session + per-request
  `_meta.logLevel`). No tasks/MRTR/subscriptions capabilities are advertised.

## Progress

- 2026-08-25: research complete (Exa sources: go-sdk v1.7.0 release notes,
  MCP blog 2026-07-28, TS/Py/C# SDK v2 migration guides, SEP-2663;
  memory surveys: Mem0/Zep/Letta/LangMem comparisons, unified framework
  arXiv 2604.01707, local-first OSS repos, memory-lifecycle article).
- 2026-08-25: Phase 0 baseline partial — `go test ./internal/mcp` green.
- 2026-08-25: Phases 1–4 done (SDK v1.7.0, TestToolSurface era fix,
  TestProtocolRevisionMatrix, docs amendment, version 0.3.0).
- 2026-08-25: Phase 5 release gate green end to end.

## Current State and Next Action

Complete: zbrain 0.3.0 speaks MCP through the v1.7.0 Go SDK with both eras
proven on raw stdio frames. Next initiative when picked up: feature backlog
F1–F6 (conflict-aware drafts first).
