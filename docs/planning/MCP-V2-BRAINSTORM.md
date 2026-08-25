# MCP 2026-07-28 server brainstorm and crashlens prototype PRD

| | |
|---|---|
| Authored | 2026-08-25 |
| Status | Proposal (planning source material, not user-facing docs) |
| Scope | Ten candidate MCP servers evaluated against the stateless MCP spec revision `2026-07-28`; selection rationale and a mini-PRD for the chosen prototype (`crashlens`) |

## 1. Context: what changed in MCP `2026-07-28`

MCP specification revision `2026-07-28` (informally "MCP v2"; the official name is the date-based revision) was released on 2026-07-28. It is the largest protocol change since launch. Key facts, all verifiable in the sources below:

- **Stateless core.** The `initialize`/`notifications/initialized` handshake and the `Mcp-Session-Id` header are removed (SEP-2567, SEP-2575). Every request is self-describing: `_meta` carries `io.modelcontextprotocol/protocolVersion`, `io.modelcontextprotocol/clientCapabilities`, and (SHOULD) `io.modelcontextprotocol/clientInfo`. Servers SHOULD identify themselves via `io.modelcontextprotocol/serverInfo` in result `_meta`. Any request can land on any instance behind a plain round-robin load balancer.
- **`server/discover`** replaces up-front capability negotiation; servers MUST implement it, clients MAY call it.
- **Multi Round-Trip Requests (MRTR)** (SEP-2322) replace server-initiated requests (`elicitation/create`, `sampling/createMessage`, `roots/list`). A server returns `resultType: "input_required"` with `inputRequests` + opaque `requestState`; the client retries the original request with `inputResponses`. Only `prompts/get`, `resources/read`, `tools/call` may return it.
- **Tasks** move into the official extension `io.modelcontextprotocol/tasks` (SEP-2663): poll-based `tasks/get`, mid-flight input via `tasks/update`, cooperative `tasks/cancel`; statuses `working`, `input_required`, `completed`, `failed`, `cancelled`.
- **Routable headers** (SEP-2243): every POST must carry `MCP-Protocol-Version` (must match body) plus `Mcp-Method` and `Mcp-Name`; gateways can route without parsing bodies. Header/body mismatch → `-32020 HeaderMismatch`; unknown method → HTTP 404 + JSON-RPC `-32601`.
- **Cacheable lists** (SEP-2549): `tools/list`, `prompts/list`, `resources/list`, `resources/read`, `resources/templates/list` carry `ttlMs` + `cacheScope` ("public"/"private").
- **Subscriptions**: the GET stream endpoint and `resources/subscribe` are replaced by a single opt-in `subscriptions/listen` long-lived POST-response stream.
- **Auth hardening**: RFC 9207 `iss` validation is mandatory for clients redeeming codes; Dynamic Client Registration is formally deprecated in favor of Client ID Metadata Documents (CIMD); client credentials are issuer-bound; Protected Resource Metadata (RFC 9728) is mandatory for discovery.
- **Deprecation policy** (SEP-2577): minimum twelve-month window. Roots, Sampling, Logging are deprecated; the legacy HTTP+SSE transport is deprecated.
- **Error-code policy**: `-32000..-32019` legacy, `-32020..-32099` spec-reserved (`-32020` HeaderMismatch, `-32021` MissingRequiredClientCapability, `-32022` UnsupportedProtocolVersion); resource-not-found moves from `-32002` to `-32602`.

Primary sources: [specification](https://modelcontextprotocol.io/specification/2026-07-28), [changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog), [GA announcement](https://blog.modelcontextprotocol.io/posts/2026-07-28/), [RC announcement](https://blog.modelcontextprotocol.io/posts/2026-07-28-release-candidate/), [SEP-2575](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/2575), [SEP-2322](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/2322), [SEP-2663](https://github.com/modelcontextprotocol/modelcontextprotocol/issues/2663), [Go SDK](https://github.com/modelcontextprotocol/go-sdk) (v1.7.0+ speaks `2026-07-28`).

Important nuance: *stateless protocol does not mean stateless application.* Cross-call state moves from hidden transport sessions to explicit, server-minted handles passed as ordinary tool arguments — visible to the model, composable across tools.

## 2. Relevance to zbrain

Short version, three points:

1. `internal/mcp/` already documents the `2026-07-28` stateless handshake among its supported protocol revisions. Everything learned building `crashlens` against the new core (per-request metadata, header validation, MRTR, Tasks store) transfers directly to the zbrain gateway's stateless path.
2. The explicit-handle pattern mirrors zbrain's existing design language: durable, digest-verifiable handles (claims, approval challenges/tokens) instead of implicit sessions. `crashlens` is a low-risk place to exercise HMAC-signed handles end to end before touching gateway code.
3. The human-approval flow planned for `crashlens` (MRTR elicitation + scope gating) is structurally the same problem as zbrain's owner-pinned claim approval lifecycle; comparing both implementations will inform the gateway's long-term UX.

## 3. Ten candidate MCP servers

Constraints locked during selection: goal is a self-use tool shipped in ~one week full-time; used daily from CLI agents, IDE plugins, and a custom harness; MVP auth is a static bearer token; done means dogfooding on a real incident.

### 3.1 Ubuntu system diagnostics

Answer "why is my box slow" without remembering commands. Tools sketch: `disk.usage`, `mem.top`, `proc.kill` (destructive), `svc.status`, `svc.restart` (destructive), `apt.dry-upgrade`. MRTR approves restarts; a Task simulates full upgrades. State-free reads; approvals ephemeral. Security: destructive ops gated by scope + approval. Risks: thin differentiation; mostly read-only surface. Commercialization: weak (commodity).

### 3.2 Go codebase researcher

Navigate large Go repos fast: `symbols.search`, `graph.callers`, `deps.why`, `vet.run`, `bench.compare`. Task: full-repo index build; MRTR disambiguates module choice. State: index-version handle. Risks: overlaps with plain grep/gopls; indexing cost. Commercialization: moderate.

### 3.3 Multi-JDK/SDKMAN assistant

Manage JDK/Gradle/Kotlin matrices: `jdk.list`, `jdk.use` (writes `.sdkmanrc`, needs approval), `build.verify` across candidates, `gradle.wrapper.audit`. Task: verify matrix; MRTR approves default-JDK switch. State lives in `.sdkmanrc`; session handle optional. Risks: niche audience. Commercialization: moderate within Java shops.

### 3.4 IntelliJ project assistant

Drive IntelliJ headless: `project.open`, `inspections.run`, `refactors.preview` (approval-gated). Tasks run whole-repo inspections. State: `project_id` handle. Risks: depends on IntelliJ remote-API stability; hardest integration of the ten. Commercialization: moderate.

### 3.5 GitHub repository auditor

Org/repo hygiene: `repo.list`, `branch.stale`, `pr.risk-score`, `actions.minutes`, `codeowners.check`. Tasks sweep an org; MRTR picks tenant context. State: `audit_run_id` handle + cached lists. Security: read-only GitHub token; no write. Risks: rate limits; value drops as platform tools improve. Commercialization: crowded space.

### 3.6 SDLC workflow auditor

Policy-as-code over process data: `policy.define`, `compliance.scan`, `exceptions.list`, `report.render`. Weekly scan Task; MRTR asks audit scope. State: `policy_set_id`, `report_id` handles. Risks: policy modeling is a product in itself; scope creep. Commercialization: decent for enterprise, slow sale.

### 3.7 Linux crash/log analyzer (`crashlens`)

Root-cause crashes from local evidence: journalctl, coredumps, Java thread dumps. See mini-PRD (§5). Strengths: fully local data, natural case-handle state model, obvious destructive-op approval story, natural long-running scan task. Risks: parsing depth rabbit holes; journald permissions. Commercialization: moderate (SRE teams).

### 3.8 Vector search knowledge server

Semantic search over internal docs: `kb.index`, `kb.search`, `kb.related`, `kb.reindex`. Indexing runs as Task; MRTR selects collection. State: `collection_id` handle. Risks: embedding model ops; overlaps with existing retrieval products. Commercialization: commodity.

### 3.9 CI/CD incident assistant

Incident context assembly: `ci.runs.failed`, `ci.log.tail`, `flaky.detect`, `rollback.plan` (approval), `incident.timeline`; bisect-by-commit as Task; MRTR approves rollbacks. State: `incident_id` handle. Strengths: highest raw value. Risks: external API dependencies (GitHub/Jenkins tokens), trigger-permissions, timeline pressure. Commercialization: strong.

### 3.10 Dependency/security update assistant

Safe dependency bumps: `dep.outdated`, `osv.scan`, `bump.pr` (creates PRs), `test.matrix`, merge approval. Batch bump+test as Task; MRTR approves major upgrades. State: `upgrade_batch_id` handle. Risks: needs repo write access and CI minutes; test flakiness pollutes results. Commercialization: strong but competitive (Renovate et al.).

### 3.11 Scoring

All columns higher-is-better (1–5). `SecSim`/`DataSim` = security/data-access *simplicity* (6 − complexity). FC = MCP feature coverage; TM = time-to-MVP speed; DF = differentiation.

| # | Idea | TF | UV | SecSim | DataSim | FC | TM | DF | Total |
|---|---|---|---|---|---|---|---|---|---|
| 1 | Ubuntu diagnostics | 5 | 4 | 3 | 4 | 3 | 5 | 2 | 26 |
| 2 | Go codebase researcher | 4 | 4 | 3 | 4 | 3 | 4 | 3 | 25 |
| 3 | Multi-JDK/SDKMAN assistant | 5 | 3 | 3 | 4 | 3 | 4 | 3 | 25 |
| 4 | IntelliJ project assistant | 2 | 3 | 3 | 3 | 3 | 2 | 3 | 19 |
| 5 | GitHub repository auditor | 4 | 4 | 3 | 3 | 4 | 4 | 2 | 23 |
| 6 | SDLC workflow auditor | 3 | 3 | 3 | 3 | 4 | 3 | 4 | 22 |
| 7 | **Linux crash/log analyzer** | **5** | **5** | **3** | **4** | **5** | **4** | **4** | **30** |
| 8 | Vector search knowledge server | 4 | 4 | 3 | 3 | 3 | 3 | 2 | 22 |
| 9 | CI/CD incident assistant | 4 | 5 | 2 | 3 | 5 | 3 | 4 | 26 |
| 10 | Dependency/security update assistant | 4 | 4 | 2 | 3 | 4 | 3 | 3 | 23 |

## 4. Selection

Top three by score: **crashlens (30)**, Ubuntu diagnostics (26), CI/CD incident assistant (26).

- **crashlens wins** because it uniquely satisfies every locked constraint at once: data is already on the machine (zero external APIs → fits one week), it exercises the full `2026-07-28` feature set naturally (explicit `case_id` handles, MRTR approval for destructive fixes, a genuinely long-running scan Task), it is self-use for someone who runs Linux servers and debugs JVM services, and its dual-transport deployment is realistic (read-only tools behind round-robin HTTP; heavy tools unchanged).
- **Ubuntu diagnostics (deferred):** easiest build, but the feature coverage is shallow — few resources/prompts worth exposing, weak differentiation, and the approval story adds little beyond one restart prompt.
- **CI/CD incident assistant (deferred):** highest ceiling, but GitHub Actions/Jenkins integration, token management, and bisect-triggering builds do not fit a one-week, zero-external-dependency MVP. Revisit as crashlens v2 or a separate project.

## 5. Mini-PRD: `crashlens`

**One-liner:** an MCP server that turns Linux crash and log evidence (journalctl, coredumps, JVM thread dumps) into investigated cases with suggested, approval-gated fixes.

**Problem statement:** diagnosing a crashed service takes 30–60 minutes of grepping journals, reading coredumps, and eyeballing thread dumps — knowledge that evaporates after each incident. crashlens makes that workflow a repeatable, auditable tool surface for LLM agents.

**Persona:** backend/on-call engineer comfortable with Go/Linux/Java; uses Claude Code-style CLI agents and IDE plugins daily; occasionally exposes tools to a custom harness over HTTP.

**Goals**

- Cut time-to-root-cause for common Linux service crashes.
- Produce a durable, shareable artifact per investigation (`case_id`).
- Exercise the full stateless `2026-07-28` feature set in production-like conditions (both transports, MRTR, Tasks, observability).

**Non-goals**

- No auto-remediation without explicit human approval.
- No Windows/macOS support in MVP.
- No APM/metrics ingestion; no log shipping pipeline.
- No cross-host fleet aggregation in MVP.

**User stories**

- US1: "nginx died last night" → ask why → get a journal timeline for the unit since boot.
- US2: list recent coredumps → generate a backtrace for one → get a plain-language summary.
- US3: parse a JVM thread dump → spot the stuck/parking threads.
- US4: kick off an overnight anomaly scan across `/var/log` → poll status next morning.
- US5: server proposes a fix (restart unit, rotate/truncate a file) → I approve via elicitation → action executes and is recorded in the case.

**Tool contracts** (names final unless noted; input/output sketches)

| Tool | Input | Output / behavior |
|---|---|---|
| `journ.query` | `unit?`, `since?`, `grep?`, `boot?` | Filtered journal lines (bounded count); read-only |
| `coredump.list` | `since?` | Coredump inventory via `coredumpctl`/`/var/lib/systemd/coredump` |
| `backtrace.gen` | `core_id` | Symbolized backtrace text (gdb if installed); may take seconds |
| `td.parse` | `path` (allowlisted) | Parsed thread dump: blocked/waiting groups, hot threads |
| `anomaly.scan` | `paths[]`, `window` | Returns `resultType: "task"`; background scan writes findings to case |
| `fix.suggest` | `case_id`, `action` | MRTR `input_required` confirmation (or accepts `confirm: true` fallback argument); on approve executes and appends to case |
| `case.create` / `case.close` | title / `case_id` | Mints/closes the case handle; close requires open case |

**Resource contracts**

| URI | Content | Cache hint |
|---|---|---|
| `journal://unit/{unit}` | Recent unit journal excerpt | `cacheScope: "private"` |
| `coredump://{id}/meta` | Metadata for one coredump | `cacheScope: "private"` |
| `case://{case_id}/summary` | Case timeline, findings, actions taken | `ttlMs` short; private |

**Prompt contracts**

- `triage-crash(symptom, unit?)`: structured triage checklist driving the tools above.
- `weekly-health(host, window)`: trend report over recent failures/anomalies.

**State model**

- `case_id` is an HMAC-signed explicit handle: `kind:id:user:exp` + HMAC-SHA256, TTL 7 days, bound to the authenticated user; clients never receive raw row IDs.
- Case content lives in SQLite (`cases.db`, mode `0600`, WAL). One table per entity plus `tasks` for background work. Pure-Go SQLite driver to respect the repo's CGO-free build constraint.
- Tasks are durably created before their `resultType: "task"` response returns; any instance can serve `tasks/get` because state is in SQLite.

**Security model**

- Local stdio: trusts the invoking OS user; credentials from environment only (per spec guidance for stdio).
- Remote HTTP: static bearer token with scopes `read` (query/list/read resources) and `operate` (fix.suggest execution, case.close). Token supplied via env/secret store; never logged.
- Destructive operations require BOTH the `operate` scope AND human approval (MRTR elicitation; `confirm: true` argument accepted as fallback for hosts that cannot render elicitations — logged as `approval=fallback-flag`).
- Filesystem access restricted to allowlist roots (`/var/log/**`, `/var/lib/systemd/coredump/**`, explicitly registered dump dirs); journal access only through `journalctl`. No URL fetching → no SSRF surface.
- Handles fail closed: bad signature, wrong user, expired → `invalid_handle` tool error. Tenant isolation derives from token claims, never from arguments.
- Audit: append-only log of tool calls, approvals, executed fixes (what/when/who/outcome).

**Deployment model**

- Single Go binary inside this repository: `cmd/crashlens` entrypoint delegating to `internal/crashlens/*`, following existing layout conventions (thin cmd, durable logic internal, handlers thin).
- stdio transport for local hosts; Streamable HTTP (stateless mode) for remote use — demo topology is nginx round-robin across two systemd instances sharing the same SQLite store, proving instance-affinity is unnecessary.
- Repository implications: subject to existing CI gates (`go test ./...`, vet, race, CGO_ENABLED=0 build); Conventional Commits; no changes to assets/embedding or workspace trust rules; crashlens never reads zbrain workspaces.

**API lifecycle and error handling**

- Every request validated independently: `MCP-Protocol-Version` header vs body `_meta` (mismatch → `-32020`), unsupported version → `-32022` with `supportedVersions`, unknown method → 404 + `-32601`, missing capability for tasks → `-32021`, unknown resource URI → `-32602`.
- Domain errors map to clean tool errors: `unit_not_found`, `core_not_found`, `path_not_allowed`, `declined_by_user`, `invalid_handle`, `scope_required`.
- Cancellation: closing the response stream cancels the request context; workers check context between chunks.

**Observability**

- Structured logs (slog): ts, level, msg, request_id, jsonrpc id, mcp_method, mcp_name, protocol_version, user_id, case_id/task_id when present, result_type, error_code, authz_decision, duration_ms. Tokens and log contents are redacted/never dumped.
- Metrics (proposed naming, not spec-defined): `mcp_requests_total{method,result_type,code}`, `mcp_request_duration_seconds`, `mcp_tool_calls_total{name,outcome}`, `mcp_tool_errors_total{name,error_code}`, `mcp_authorization_denied_total{reason}`, `mcp_tasks_created_total`, `mcp_tasks_completed_total`, `mcp_tasks_failed_total`, `mcp_mrtr_input_required_total{tool}`.
- Tracing: OpenTelemetry spans rooted at the HTTP POST; W3C `traceparent`/`tracestate` propagated through `_meta` per SEP-414 conventions.

**Test plan**

- Unit: handle sign/verify/expiry/user-binding; scope enforcement; MRTR two-phase state machine incl. tampered `requestState`; task lifecycle transitions and terminal-state immutability; error-code mapping table.
- Integration: two replicas behind nginx with one shared store (same `case_id` across instances); kill -9 mid-request then client re-issue; duplicate `fix.suggest` retries; timeout; cancellation stops work; task survives process restart.
- Conformance: upstream conformance suite for the stateless core, MRTR, and cache fields, run nightly.
- Security: wrong/expired token, scope escalation attempts, path traversal outside allowlist, replayed `requestState`, approval bypass attempts.
- Performance baseline: p95 for read tools under concurrent load; recorded once, re-checked before release.

**Milestones (one week full-time)**

- Day 1: spec/schema notes; scaffold `cmd/crashlens` + CI green; Go SDK v1.7 stdio spike; lock contracts above.
- Day 2: `internal/crashlens` protocol types + stdio transport + first tools (`journ.query`, `coredump.list`, `case.create/close`) + error-code tests.
- Day 3: stateless HTTP handler (headers, SSE scoping, 202/400/404 mapping), `server/discover`, SQLite store + HMAC handles, nginx two-replica test.
- Day 4: MRTR `fix.suggest` (+ fallback flag), `anomaly.scan` Task surviving restart, bearer-token middleware, slog + OTel wiring.
- Day 5: conformance/security/load pass, failure drills, README + threat model, dogfood on a real incident.

**Definition of Done**

- Dogfooded end-to-end on one real incident on my own machine (the bar that matters).
- All seven tools and three resources work over BOTH transports; approval flow demonstrable; scan task survives restart; conformance suite passes; docs + threat model merged.

## 6. Open questions

1. Exact placement inside zbrain (`cmd/crashlens` + `internal/crashlens/*` assumed) — confirm no conflicts with planned gateway refactors.
2. Do current target hosts (Claude Code, IDE plugins) render MRTR `input_required` elicitations today? Until verified, the `confirm: true` fallback stays mandatory.
3. Static-token distribution/rotation procedure for remote mode — acceptable for MVP, revisit before any third-party exposure.
4. journald read rights for a non-root service account (`systemd-journal` group membership) — verify on target Ubuntu LTS.
5. Minimum Ubuntu baseline for `coredumpctl` behavior consistency (assume 24.04+, verify).
6. Is SQLite WAL sufficient as the task store under the intended load, or plan a Postgres adapter now?
7. Which upstream conformance cases cover extension methods (tasks) vs core only?
8. Thread-dump parsing depth for MVP: `jstack` text format only, or also async-profiler output?

## 7. Decision log

| Decision | Choice | Rationale |
|---|---|---|
| Goal | Ship a self-use tool | Learning follows usage |
| Budget | ~1 week full-time | Caps external dependencies |
| Clients | CLI agent + IDE + custom harness | Dual transport mandatory; approval needs fallback |
| Prototype | crashlens (#3.7) | Only candidate satisfying all constraints simultaneously |
| Auth (MVP) | Static bearer token, `read`/`operate` | OAuth/CIMD deferred until multi-user need |
| Approval UX | MRTR + `confirm` fallback flag | Spec-correct primary path; graceful degradation |
| Code home | This repository | Reuses CI/conventions; keeps planning traceability |
| Done means | Dogfood a real incident | Prevents checklist-only completion |
