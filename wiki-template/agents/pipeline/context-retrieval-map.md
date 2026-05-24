# Context Retrieval Map

## Purpose

Map intent types and components to the exact wiki docs to retrieve. Do not retrieve docs not listed here for a given intent — more is not better.

> Engine baseline docs (`agents/constraints.md`, `agents/coding-rules.md`) are always loaded for every intent and are **out-of-budget** — see [context-filter.md → Baseline (out-of-budget) Docs](context-filter.md#baseline-out-of-budget-docs).

> Mọi path đều prefix bằng `{ws} = workspaces/{intent.workspace}/`. KHÔNG retrieve file ngoài `{ws}/`.

## Retrieval by Intent Type

| Intent Type | Always Retrieve | Conditionally Retrieve |
|------------|----------------|----------------------|
| `implement_feature` | `{ws}/platform/contracts/`, `{ws}/platform/patterns/`, `{ws}/projects/{scope}/knowledge-map.md` | `{ws}/domains/{domain}/workflow.md` (if domain known) |
| `fix_bug` | `{ws}/runbooks/`, `{ws}/projects/{scope}/services/{service}.md` | `{ws}/platform/patterns/` (if component known) |
| `design` | `{ws}/platform/architecture/`, `{ws}/decisions/` | `{ws}/platform/patterns/`, `{ws}/domains/{domain}/` |
| `incident` | `{ws}/runbooks/` | `{ws}/projects/{scope}/services/{service}.md` |
| `review` | `agents/constraints.md`*, `agents/coding-rules.md`*, `{ws}/platform/patterns/` (+ `{ws}/agents/constraints.md`* nếu có) | `{ws}/domains/{domain}/workflow.md`, `{ws}/platform/contracts/` |

> *Baseline (tracked but out-of-budget per [context-filter.md](context-filter.md#baseline-out-of-budget-docs)).*

## Retrieval by Component

| Component | Docs to Retrieve |
|-----------|-----------------|
| `kafka` | `{ws}/platform/patterns/kafka-event-processing.md` |
| `mqtt` | `{ws}/platform/patterns/mqtt-routing.md`, `{ws}/platform/contracts/mqtt-topic-contract.md` |
| `batch` | `{ws}/platform/patterns/kafka-event-processing.md` (batch section) |
| `http` | `{ws}/decisions/` (relevant ADR if exists) |
| `db` | `{ws}/projects/{scope}/services/{service}.md` (for connection pool / timeout config) |

> Nếu file không tồn tại trong workspace active → ghi vào Knowledge Gaps, KHÔNG fallback sang workspace khác.

## Retrieval by Domain

| Domain | Docs to Retrieve |
|--------|----------------|
| `{domain}` | `{ws}/domains/{domain}/workflow.md` |

## Retrieval by Scope (project)

| Scope | Docs to Retrieve |
|-------|----------------|
| `{project}` | `{ws}/projects/{project}/knowledge-map.md`, relevant service docs trong `{ws}/projects/{project}/services/` |

## Worked Example

Intent:
```json
{
  "workspace": "example-surgery",
  "type": "implement_feature",
  "domain": "surgery",
  "components": ["kafka", "mqtt", "batch"],
  "scope": "surgery-service"
}
```

`{ws}` = `workspaces/example-surgery/`

Retrieved docs:
```
workspaces/example-surgery/platform/contracts/mqtt-topic-contract.md           ← contracts (always first)
workspaces/example-surgery/platform/patterns/kafka-event-processing.md          ← kafka + batch
workspaces/example-surgery/platform/patterns/mqtt-routing.md                    ← mqtt
workspaces/example-surgery/projects/surgery-service/knowledge-map.md            ← scope
workspaces/example-surgery/projects/surgery-service/services/kafka-consumer.md  ← scope
workspaces/example-surgery/domains/surgery/workflow.md                          ← domain
```

Pass this list to [Context Filter + Rank](context-filter.md).
