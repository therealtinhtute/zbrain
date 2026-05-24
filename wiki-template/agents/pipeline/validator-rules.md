# Validator Rules

## Purpose

Catch violations in agent output before it reaches a human or gets committed. Two layers: fast rule-based checks, then a prompt-based self-check for deeper issues.

## Workspace Extension (additive only)

Rules in this file are **engine baseline** — chạy cho MỌI workspace, KHÔNG bị override.

Mỗi workspace có thể **thêm** rules tại `workspaces/{active}/agents/pipeline/validator-rules.md`. Quy tắc:

1. **Additive only**: workspace chỉ được THÊM rule mới. Engine rules luôn chạy, không có cơ chế tắt.
2. **Tên rule không trùng engine**: rule trong file workspace MUST có prefix `ws-` (vd `ws-no-mongodb-direct`) để không gây ambiguity với engine rule.
3. **Strict-only direction**: workspace có thể làm rule chặt hơn (vd thêm rule cấm thêm pattern), KHÔNG được nới lỏng. Nếu workspace có lý do chính đáng để nới lỏng 1 engine rule → phải patch ENGINE file (có git audit trail), KHÔNG silent override trong workspace.

Resolution flow: engine rules chạy trước, rồi workspace rules chạy thêm. Output cuối cùng = union các violations từ cả 2.

Vi phạm phát hiện được:
- Workspace file có rule trùng tên engine → fail-fast, báo lỗi `WORKSPACE RULE NAME CONFLICT: {name} — phải dùng prefix ws-`.
- Workspace file ghi đè engine rule (cùng check function name) → fail-fast.

## Layer 1 — Rule-Based Checks

Run these as code or regex against the generated output. Fast, deterministic, zero LLM cost.

> **Source of truth: [`scripts/validate.py`](../../scripts/validate.py).**
> The script is the authoritative implementation — the tables below are
> documentation only. If the doc and the script disagree, the script wins.

### How to run

```bash
python scripts/validate.py --file <path-to-code-file> [--workspace <name>] [--wiki-root <path>] [--pretty]
```

* Workspace + `wiki_root` are auto-resolved from `<cwd-walk-up>/.claude/wiki.json` per [system-prompt.md `wiki_root` Resolution Rule](../system-prompt.md#wiki_root-resolution-rule).
* Output: JSON `{violations: [...], summary: {errors, warnings}, context: {...}}` on stdout.
* Exit code: `0` if no errors (warnings allowed), `1` if any errors, `2` for bad invocation.

### Rule list (implemented)

| Rule ID | Severity | Check |
|---------|----------|-------|
| `kafka-no-hardcoded-topic`     | error | Quoted lowercase dotted literal near a Kafka API verb (`@KafkaListener`, `KafkaTemplate`, `send(`, `subscribe(`…) and not on a config-read line (`@Value`, `getProperty`, `System.getenv`…). |
| `kafka-commit-before-process`  | error | `commitSync(` / `commitAsync(` appears BEFORE the first processing call (`process`, `for (`, `forEach`, `onMessage`, `publish(`…) in the same enclosing brace block. |
| `kafka-dlq-required`           | error | File looks like a Kafka consumer (has `@KafkaListener`, `KafkaConsumer`, `ConsumerRecord`, `@StreamListener`, or `poll(`) but contains no reference to `dlq` / `deadLetter` / `.dlq.`. |
| `kafka-batch-processing`       | warn  | Per-message loop `for (X x : messages)` while a batch hint is present in the file (`max.poll.records`, `setBatchListener`, `containerFactory=`…). |
| `mqtt-no-inline-topic`         | error | String concat / `String.format` / template-literal building of a topic literal that begins with `topic/`, when no helper (`buildTopic`, `topicFor`, `MqttTopic.`, `TopicFormatter`…) is referenced on the same line. |
| `mqtt-unregistered-type`       | error | Literal `topic/.../up/<type>` whose `<type>` is not in the `Registered Types` table of `{ws}/platform/contracts/mqtt-topic-contract.md`. |
| `domain-unknown-state`         | error | UPPER_CASE_SNAKE string literal that looks state-like (has `_` or ends in `ED`/`ING`/`AL`/`ANT`/`OUS`) and is not in `{ws}/domains/{domain}/workflow.md`. |
| `no-hardcoded-config`          | warn  | Numeric literal assigned to a config-like identifier (`batchSize`, `timeoutMs`, `concurrency`, `maxPollRecords`, `pollTimeout`, `retries`, `backoffMs`). |
| `constructor-injection`        | error | `@Autowired` annotation immediately followed by a field declaration (next non-blank line ends with `;` and has no `(`). |

### How to add a new rule

1. Open `scripts/validate.py`.
2. Implement a rule function `rule_<id>(file_path, lines, ctx) -> List[Dict]` and append to `ALL_RULES`. Use the `_vio(rule_id, severity, file_path, line, snippet, message)` helper.
3. Add the rule to the table above (engine rule) **or** add it to `{ws}/agents/pipeline/validator-rules.md` with the `ws-` prefix (workspace rule — see Workspace Extension below).
4. Add a fixture line to `scripts/test-fixtures/bad-consumer.java` (or a new fixture) that triggers the rule, and verify the script catches it.

### Heuristic limitations (Layer 1, by design)

* No real parser — comment / string-aware brace tracking is naive.
* `kafka-no-hardcoded-topic` requires a Kafka API verb on the same line; topics defined via static-final constants will be missed (false negative).
* `kafka-commit-before-process` is block-local — cross-method commit-before-process is invisible.
* `domain-unknown-state` depends on a parseable workflow.md and a single domain (or `wiki.json#domain` set explicitly).
* Workspace `ws-` rule loader is a stub: the script reports the presence of `{ws}/agents/pipeline/validator-rules.md` in `context.workspace_rules_file` but does not yet execute additive rules. (TODO marked in `scripts/validate.py`.)
* Layer 1 catches the common drift; Layer 2 (below) covers the rest.

## Layer 2 — Prompt-Based Self-Check

Run this as a second LLM call on the agent's output. Use a small/fast model — it's a verification pass, not a generation pass.

```md
# SELF-CHECK TASK

Review the solution below and check it against the following constraints.
For each violation found, describe: what it is, where it occurs, and how to fix it.
If no violations found, respond with: "PASS"

## Constraints to Check

### Kafka
- Offset committed only after processing completes
- DLQ path implemented for all failure scenarios
- Batch processing used (not per-message loop)
- No hardcoded topic names

### MQTT
- Topic format matches: topic/{region}/{gatewayId}/up/{type}
- Only registered types used: heartbeat, alert (update this list per project)
- No inline topic string construction

### Domain
- No workflow states added beyond: {list states from `{ws}/domains/{domain}/workflow.md`}
- No transitions added beyond: {list transitions from `{ws}/domains/{domain}/workflow.md`}

### General
- No hardcoded config values
- Constructor injection used (not field injection)

## Solution to Review

{{agent_output}}
```

## Escalation

If Layer 1 finds a violation → block output, return violation report to user.

If Layer 2 finds a violation → append the self-check result to the agent output as a `## Violations Found` section. Do not silently fix.

## When to Update These Rules

- When a new constraint is added to [agents/constraints.md](../constraints.md) → add a corresponding check here (engine) HOẶC vào `{ws}/agents/pipeline/validator-rules.md` nếu chỉ áp dụng 1 workspace (nhớ prefix `ws-`)
- When an agent generates the same bug twice → add a rule for it (workspace-scoped trừ khi reproducible ở mọi workspace)
- When a new contract is registered in `{ws}/platform/contracts/` → add MQTT type check vào `{ws}/agents/pipeline/validator-rules.md` (prefix `ws-`)
- Khi workspace thấy 1 engine rule quá strict cho domain của họ → KHÔNG override, mà mở PR vào engine file giải thích lý do — đây là điểm chặn cố ý để tránh wiki phân mảnh

## Related

- [Constraints](../constraints.md)
- [Prompt Template](prompt-template.md)
