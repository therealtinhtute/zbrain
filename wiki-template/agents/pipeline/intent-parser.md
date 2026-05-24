# Intent Parser

## Purpose

Parse the user task into a structured intent before retrieving any context. This prevents retrieving irrelevant docs and narrows the retrieval scope.

## Intent Schema

```json
{
  "workspace": "{name}",
  "type": "implement_feature | fix_bug | design | incident | review",
  "domain": "{domain-folder-name} | null",
  "components": ["kafka", "mqtt", "batch", "http", "db", ...],
  "scope": "{project-folder-name} | global | null"
}
```

## Field Resolution

### `workspace`

- Mặc định = `<cwd>/.claude/wiki.json.workspace` (active workspace của codebase đang làm).
- Fallback nếu file thiếu: `~/.claude/wiki-global.json.default_workspace`.
- Nếu user task chỉ rõ workspace khác (vd "trong workspace company-b, ...") → cảnh báo user context không khớp `.claude/wiki.json`, gợi ý `/switch-workspace` trước. KHÔNG silently override.
- Nếu cả hai nguồn đều thiếu → STOP, yêu cầu user `/switch-workspace` hoặc `/wiki-setup`.

### `domain` & `scope`

Phải tồn tại trong workspace active:
- `domain` ∈ tên thư mục con của `workspaces/{active}/domains/`
- `scope` ∈ tên thư mục con của `workspaces/{active}/projects/`

Nếu không khớp → gắn `null` và ghi vào Knowledge Gaps trong `current-task.md`.

## Type Definitions

| Type | Triggered When | Example Task |
|------|---------------|--------------|
| `implement_feature` | Building new functionality | "Add Kafka consumer for file events" |
| `fix_bug` | Diagnosing or fixing a failure | "Consumer is DLQing all messages" |
| `design` | Architecture or approach decisions | "How should we handle MQTT routing?" |
| `incident` | Live production issue | "Kafka lag spiking on surgery topic" |
| `review` | Code or doc review | "Review this consumer implementation" |

## Component Detection

Map keywords in the task to components:

| Keyword | Component |
|---------|-----------|
| kafka, consumer, producer, topic, offset, DLQ | `kafka` |
| mqtt, publish, subscribe, topic format, gateway | `mqtt` |
| batch, chunk, bulk | `batch` |
| rest, http, endpoint, api, feign | `http` |
| db, database, jpa, repository, query | `db` |

## Implementation Options

**Option A — Rule-based (fast, predictable):**
- Keyword match on task text
- Map to intent schema
- Suitable for well-defined task vocabularies

**Option B — LLM-based (flexible, slower):**
- Feed task to a small/fast model with the schema above
- Ask it to output valid JSON
- Use when tasks are free-form or multi-language

## Example

Task: `"Implement Kafka consumer for surgery file processed events and publish result via MQTT"`
Active workspace (from `<cwd>/.claude/wiki.json.workspace`): `example-surgery`

```json
{
  "workspace": "example-surgery",
  "type": "implement_feature",
  "domain": "surgery",
  "components": ["kafka", "mqtt", "batch"],
  "scope": "surgery-service"
}
```

## Output

Pass the intent JSON to the [Context Retriever](context-retrieval-map.md). Retriever sẽ resolve mọi path qua `{ws} = workspaces/{workspace}/`.
