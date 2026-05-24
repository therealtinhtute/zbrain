# Evidence Q&A

Q&A loop với user dựa trên question pool + knowledge gaps + contradictions. Hỏi theo priority batches (P0→P3), hỗ trợ **defer-to-expert** (checkpoint cho người trả lời chậm), tự build `verified-facts.md`.

> Step 3 trong pipeline: ingest → analyze → **qa** → apply.
> Reference: [agents/pipeline/qa-batching.md](../../agents/pipeline/qa-batching.md), [agents/pipeline/evidence-state-rules.md](../../agents/pipeline/evidence-state-rules.md).

---

## Input

| Arg            | Required | Notes                                                                  |
|----------------|----------|------------------------------------------------------------------------|
| `--id`         | optional | Evid-id. Default: latest entry state ∈ `analyzed|qa_in_progress|qa_awaiting_external` |
| `--batch-size` | optional | 1-4. Default 4 (max do `AskUserQuestion` 4 questions/call).            |
| `--resume`     | optional | Flag — resume từ `qa_awaiting_external`. Đọc `external-answers.md`/hỏi user về pending. |
| `--priority`   | optional | Chỉ chạy 1 priority bucket: `P0|P1|P2|P3`. Default: P0 → P3 sequence.   |

---

## Bước 0 — Workspace check

1. Tìm `.claude/wiki.json`: từ `<cwd>` đi lên parent cho tới khi gặp file. Lưu `wiki_json_dir`.
2. Đọc file → `workspace` + `wiki_root` resolve theo [agents/system-prompt.md `wiki_root` Resolution Rule](../../agents/system-prompt.md):
   - Absolute path → dùng nguyên
   - Relative (`"."`, `"./..."`) → resolve relative TỚI `project_root` (= parent của `.claude/`)
   - `null`/empty → fallback `~/.claude/wiki-global.json#wiki_root`
3. STOP nếu file thiếu hoặc `.workspace` rỗng.
4. Set `{ws} = {effective_wiki_root}/workspaces/{workspace}/`.

## Bước 1 — Resolve evidence + state

1. Resolve `--id` (latest state ∈ {`analyzed`, `qa_in_progress`, `qa_awaiting_external`} nếu không truyền).
2. Workspace lock check (I-2).
3. Validate state theo flag:
   - Không `--resume`: state PHẢI = `analyzed`. Nếu = `qa_in_progress` → in warning "đã có Q&A đang chạy, dùng `--resume`".
   - Có `--resume`: state PHẢI ∈ {`qa_in_progress`, `qa_awaiting_external`}. Nếu `analyzed` → in warning "chưa có Q&A để resume, bỏ flag".

## Bước 2 — Build/load todo.json

### Initial run (state = `analyzed`)

1. Đọc `analysis/{id}/04-questions.md`, `analysis/{id}/02-contradiction.md`, `analysis/{id}/08-knowledge-gaps.md`.
2. Score mỗi question theo formula trong [qa-batching.md](../../agents/pipeline/qa-batching.md).
3. Tạo `qa/{id}/todo.json` từ template `templates/evidence-qa-todo.json`:
   - `state = qa_in_progress`
   - `questions[]` populated với priority + batch assignment
   - `batches[]` empty list ban đầu, sẽ append khi dispatch
4. Update `_index.md`: state → `qa_in_progress`.

### Resume run

1. Đọc `qa/{id}/todo.json` — bỏ qua questions đã `answered|skipped|deferred`.
2. Nếu có `qa/{id}/external-answers.md` → parse và sync:
   - Mỗi entry `## q-XXX` trong file → match question → status `awaiting_external` → `answered`, ghi `answer_ref`.
   - Append vào `batch-{n}-answers.md` của batch gốc với note "from external-answers.md".

## Bước 3 — Conversation context scan (heuristic, optional)

Trước khi mở batch đầu, scan recent user messages trong session hiện tại:
- Nếu user đã state thông tin liên quan đến 1 question pending → auto-fill answer với `confidence: medium` + flag `needs_confirm: true`.
- Push các câu này vào batch confirm-only đầu tiên (chỉ cần user gật đầu).

## Bước 4 — Batch loop

For mỗi `priority ∈ [P0, P1, P2, P3]` (skip nếu `--priority` chỉ định 1 bucket):

  Lấy questions chưa answered/skipped/deferred ở priority này.
  Chia thành batches ≤ `--batch-size`.

  For mỗi batch:

  ### 4a. Display context

  Hiển thị `batch-{n}-questions.md` (tạo từ template `templates/evidence-qa-answers.md` ở trạng thái câu hỏi-only):
  - List N câu hỏi với context inline (vd nếu hỏi về `Config Overrides`, in 5 dòng table hiện tại của service doc).

  ### 4b. AskUserQuestion (1 call, N≤4 questions)

  Mỗi question dùng 4 options fixed:
  ```
  - "Trả lời" (description: paste answer in Other text input)
  - "Defer to expert" (description: claude sẽ hỏi assignee + channel + deadline)
  - "Skip" (description: P0/P1 sẽ block apply; P2/P3 OK)
  - "Defer to next session" (description: park, không gửi expert)
  ```

  ### 4c. Process answers

  Loop từng question:

  - **"Trả lời"** + Other text:
    - Append `## q-XXX` block vào `batch-{n}-answers.md` (status=answered, by=self, confidence=high)
    - Update `todo.json`: `status=answered`, `answer_ref`, `answered_at`, `answered_by=self`

  - **"Defer to expert"**:
    - Gọi AskUserQuestion lần nữa (1 question gộp): "Expert info cho q-XXX: assigned_to (email/name)? channel (email/slack/jira/teams/in_person/other)? expected_by (date)?" → user paste structured.
    - Update `todo.json`: `status=awaiting_external`, populate `external{}`.
    - Append/update `pending-external.md` (template `templates/evidence-pending-external.md`):
      - Tạo file nếu chưa có
      - Append section `## ✉️ Copy-paste block` cho question này (gồm context wiki + question phrased cho expert)
      - Update tracking table

  - **"Skip"**:
    - AskUserQuestion 1 lần ngắn: "Lý do skip? (optional)"
    - Update `todo.json`: `status=skipped`, log reason vào `batch-{n}-answers.md`

  - **"Defer to next session"**:
    - Update `todo.json`: `status=deferred`. Không touch pending-external.

  ### 4d. Mini Contradiction Hunter

  Sau khi process xong batch:
  - Chạy mini prompt: "Có answer nào trong batch này mâu thuẫn với (a) answers trước đó trong evidence này, (b) `02-contradiction.md`, (c) wiki hiện tại của `{ws}`?"
  - Nếu có conflict → tạo follow-up question `q-XXX-followup` với cùng priority gốc, push vào batch tiếp theo (cùng priority bucket).

  ### 4e. Update todo.json + _index.md

  - `todo.json#updated_at` = now
  - Nếu có ≥1 `awaiting_external` → state `qa_awaiting_external` (in `_index.md` cập nhật cột `blocked_on`).
  - Else state vẫn `qa_in_progress`.

## Bước 5 — Stop check (sau khi finish 1 priority bucket)

After P0+P1 batches done:
- Nếu mọi P0/P1 question status ∈ {`answered`, `skipped`, `deferred`} (KHÔNG awaiting_external):
  - Hỏi 1 lần qua AskUserQuestion: **"P0+P1 đã clear. Tiếp tục P2/P3 hay đủ?"** Options: ["Continue P2/P3", "Stop here"].
  - Nếu Stop → mọi P2/P3 pending mark `deferred` → đi tới Bước 6.
  - Nếu Continue → tiếp Bước 4 cho P2 → P3.

After all assigned priorities done:
- Nếu state vẫn `qa_in_progress` (không awaiting_external) → đi Bước 6.
- Nếu `qa_awaiting_external` → STOP, in instruction (xem Bước 8).

## Bước 6 — Build verified-facts.md

Khi state ready để chuyển `qa_done`:

1. Tạo/overwrite `qa/{id}/verified-facts.md` theo schema trong [qa-batching.md](../../agents/pipeline/qa-batching.md).
2. Group facts theo affected file (`Block: Contracts`, `Block: Service config`, `Block: Domain workflow`, ...).
3. Mỗi fact PHẢI có:
   - `Confidence: high|medium|low`
   - `Source: q-XXX (self|expert: <email>) — answered <date>`
   - `Affects: {ws}/path/to/file.md` (relative)
4. Section "Open / deferred" liệt kê P2/P3 deferred (informational).

## Bước 7 — Transition to qa_done

1. Update `_index.md`: state → `qa_done`, `last_updated` = today, `blocked_on` = `—`.
2. Update `todo.json#state = qa_done`.
3. In:
   ```
   ✅ Q&A complete — {evid-id}
      Answered : {N} (self: {x}, expert: {y})
      Skipped  : {N}
      Deferred : {N} (P2/P3 only)
      State    : qa_done

   Verified facts ready: {ws}/evidence/qa/{id}/verified-facts.md

   Next:
     /evidence-apply --id {id} --mode update    → push facts to wiki
     /evidence-apply --id {id} --mode rebase    → rebase wiki using facts as source-of-truth
   ```

## Bước 8 — Pause for awaiting_external

Khi có ≥1 P0/P1 status=`awaiting_external` và không thể đi tiếp:

1. Update `_index.md`: state=`qa_awaiting_external`, `blocked_on` = list `assigned_to (q-id, due {date})`.
2. Đảm bảo `pending-external.md` đã có copy-paste block cho mọi external entry.
3. In:
   ```
   ⏸  Q&A paused — waiting for external answers
      Evidence : {evid-id}
      Pending  : {N} questions (P0={x}, P1={y})
      File     : {ws}/evidence/qa/{id}/pending-external.md

   To send to experts:
     1. Open pending-external.md
     2. Copy ✉️ blocks → email/slack/jira
     3. Set deadline reminders

   When answers arrive:
     /evidence-qa --resume --id {evid-id}
     (paste answers inline OR edit external-answers.md first)
   ```

---

## Resume mode chi tiết

`--resume` flow:

1. Đọc `todo.json` lọc `status=awaiting_external`.
2. Đọc `external-answers.md` nếu có:
   - Format expected: section `## q-XXX` + `**Answer**: ...` + `**From**: <expert>` + `**Received at**: <ts>`.
   - Parse → match q-id → process như "Trả lời" trong Bước 4c (status → answered, by=expert).
3. Cho mọi entry awaiting_external CHƯA có trong external-answers.md:
   - Gọi AskUserQuestion: "q-XXX (assigned to {expert}, due {date}): Đã có answer?"
   - Options: ["Yes — paste answer", "Still waiting", "Expired/escalate", "Skip"]
   - "Yes" → user paste answer → process như answered (by=expert)
   - "Still waiting" → giữ awaiting_external, optional update expected_by
   - "Expired/escalate" → ask cho new assignee/deadline OR mark `skipped` với reason "expired"
   - "Skip" → status=skipped
4. Sau khi xử lý hết → re-evaluate stop condition (Bước 5).
5. Nếu mọi P0/P1 cleared → đi tiếp Bước 6 (verified-facts) và Bước 7.
6. Nếu vẫn còn awaiting_external → trở lại Bước 8.

---

## Common errors

| Error                              | Fix                                                                |
|------------------------------------|--------------------------------------------------------------------|
| State ≠ analyzed (no --resume)     | `/evidence-analyze` first                                          |
| State = qa_done                    | Đã xong, dùng `/evidence-apply`                                    |
| Workspace lock fail                | `/switch-workspace {workspace_at_ingest}`                          |
| `external-answers.md` malformed    | Edit theo schema; rerun `--resume`                                 |
| User dừng giữa batch (Ctrl+C)      | Re-run command — state đã persist trong `todo.json`                |
| Câu hỏi đã answered nhưng follow-up vô tận | Hard cap: 2 follow-up rounds per question; thứ 3 → mark `INVESTIGATE` trong verified-facts |
