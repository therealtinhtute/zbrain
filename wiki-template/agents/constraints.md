# Agent Constraints

Hard rules. No exceptions. If a constraint conflicts with a user request, state the conflict explicitly before proceeding.

## Workspace Extension (additive only)

Engine constraints in this file áp dụng cho MỌI workspace, KHÔNG bị override. Workspace có thể THÊM constraint tại `workspaces/{active}/agents/constraints.md`. Quy tắc:

- **Additive only**: workspace chỉ được thêm constraint mới, không xóa/nới lỏng engine constraint.
- **Tên không trùng engine**: heading constraint trong file workspace nên dùng prefix `WS:` (vd `WS: No Direct DB Writes`) để dễ phân biệt khi merge view.
- **Strict-only direction**: workspace có thể làm chặt hơn (vd cấm thêm framework), KHÔNG được nới lỏng. Muốn nới lỏng → patch ENGINE file qua git, KHÔNG silent override trong workspace.

Resolution: agent đọc engine constraints trước, rồi đọc workspace constraints (nếu có), apply tất cả.

## Architecture Constraints

- **Do not create APIs** outside the defined contracts — no new endpoints, topics, or schemas without a contract update
- **Do not bypass retry logic** — every Kafka consumer must implement retry + DLQ per the platform pattern
- **Do not add new Kafka topics** without updating the contract and this knowledge base
- **Do not add new MQTT types** without registering them in the MQTT topic contract

## Code Constraints

- **Do not hardcode** topic names, connection strings, region codes, or gateway IDs
- **Do not inline** MQTT topic construction — use the contract format helper
- **Do not commit offset before processing** — this is a data-loss risk
- **Do not add state** to service classes — all services must remain stateless

## Domain Constraints

- **Do not add workflow states** beyond what is defined in `domains/{domain}/workflow.md`
- **Do not allow transitions** not listed in the workflow doc
- **Do not auto-approve** domain actions that require explicit actor identity

## Knowledge Constraints

- **Do not assume** API signatures, payload schemas, or topic formats — read the contract
- **Do not fill knowledge gaps with guesses** — if information is missing, report it
- **Do not duplicate** pattern implementations — if a pattern exists, apply it; do not rewrite it

## When a Constraint Cannot Be Met

State the conflict clearly:
```
CONSTRAINT CONFLICT: [constraint name]
Reason: [why it cannot be met in this case]
Options: [what the user can do — update the constraint, update the knowledge base, or accept a deviation with explicit documentation]
```
