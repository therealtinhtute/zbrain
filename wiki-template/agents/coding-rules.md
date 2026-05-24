# Coding Rules — Engine (Universal)

> Stack-specific rules belong in `workspaces/{ws}/agents/coding-rules.md` (workspace additive override). This engine file contains only principles that apply to any backend, regardless of language, framework, or messaging stack.
>
> Resolution: agent reads engine rules first, then workspace rules (additive, may tighten — never relax). Same convention as `agents/constraints.md` "Workspace Extension".

---

## Dependency Injection

- Inject collaborators via **constructor parameters** — never via mutable field/property setters or service-locator lookups.
- Constructor-injected dependencies should be immutable (`final` / `readonly` / equivalent in your language).
- A class's collaborators must be visible from its constructor signature — no hidden globals.

## Configuration

- **No hardcoded** connection strings, endpoints, topic/queue names, region codes, gateway IDs, credentials, batch sizes, timeouts, or concurrency limits.
- All such values come from configuration (env, config file, secret store) — sourced once at startup, not re-read per request.
- Defaults live in the platform pattern doc, not scattered across services. Local overrides are recorded in the service's "Config Overrides" table.

## Statelessness

- Service / handler classes carry **no mutable instance state** between requests or messages.
- Anything request-scoped lives in method parameters or a request-scoped context object.
- Long-lived state (caches, counters, sessions) belongs in an explicit, named component — not bolted onto a service.

## Error Handling Boundaries

- Every async/IO/integration boundary has explicit error handling — no silent `catch` that swallows.
- Classify errors at the boundary: **transient → retry**, **permanent → dead-letter / quarantine + log**.
- Log on failure with: correlation ID, input summary, error type, error message. Never log secrets or full payloads containing PII.
- Never let an unhandled exception cross a message-consumer boundary without first updating the consumer's progress marker (offset / ack / cursor) correctly — see workspace messaging rules.

## Idempotency & Atomicity

- Any handler that can be re-delivered (message consumers, webhook handlers, retried jobs) must be idempotent or guarded by a dedup key.
- Process units of work atomically where the underlying store allows it; otherwise document the failure window.

## Observability

- Emit structured logs (key=value or JSON) — not freeform prose. Include correlation/trace ID on every log line crossing a boundary.
- Emit metrics for: processed count, error count, latency, queue depth (where applicable). One metric per outcome class; do not over-cardinality on user-supplied values.
- Health checks must reflect actual dependency health (downstream reachable, consumer lag bounded), not just "process is alive".

## Reuse Over Reinvention

- If a platform pattern exists for the task, apply it. Do not rewrite it inline.
- If a contract defines a format (topic name, payload schema, API shape), use the contract's helper / generated client — do not construct the format string inline.

## Testing

- Unit tests for pure business logic.
- Integration tests for every external boundary (DB, message broker, HTTP) — using realistic test doubles (in-memory engine, container, or sandbox), not naive mocks of the boundary protocol.
- The failure path (retry exhaustion, dead-letter, rollback) is tested explicitly — not just the happy path.

---

## Where Stack-Specific Rules Live

Workspace files extend (never relax) these rules with concrete annotations, frameworks, and defaults:

- `workspaces/{ws}/agents/coding-rules.md` — heading prefix `WS:` to disambiguate when merged.
- Cross-link from workspace rules to the relevant platform pattern (`workspaces/{ws}/platform/patterns/...`) and contract (`workspaces/{ws}/platform/contracts/...`).
