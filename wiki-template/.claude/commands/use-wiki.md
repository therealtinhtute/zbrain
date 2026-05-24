# Sử dụng Wiki

Chạy pipeline này trước khi viết bất kỳ dòng code nào. Pipeline delegate hai bước nặng (intent parsing + context retrieval) cho subagent chuyên biệt để giữ context window của main agent sạch và tránh agent chính tự "trượt" vai trò.

```
[main]   Bước 0 — resolve workspace + wiki_root
   ↓
[wiki-planner]            Bước 1 — phân tích task → intent JSON
   ↓
[wiki-context-selector]   Bước 2 — retrieve + slice + ghi current-task.md
   ↓
[wiki-plan-reviewer]      Bước 2.5 — review plan, APPROVED hoặc BLOCK
   ↓
[main]   Bước 3 — verify gap, code theo prompt template
   ↓
[wiki-reviewer]           Bước 4 — (manual/optional) review code đã sinh
```

---

## Bước 0 — Resolve workspace & wiki root (main agent)

1. Tìm `.claude/wiki.json` của codebase: bắt đầu từ `<cwd>`, đi lên parent directory cho tới khi gặp file. Lưu lại `wiki_json_dir` = directory chứa file đó.
2. Nếu không tìm thấy → chạy `/wiki-detect` trước, gợi ý user `/wiki-setup`. KHÔNG tự đoán workspace.
3. Đọc `wiki.json`:
   - `workspace` → tên workspace active của codebase này
   - `wiki_root` → resolve theo [agents/system-prompt.md `wiki_root` Resolution Rule](../../agents/system-prompt.md):
     - Absolute path → dùng nguyên
     - Relative (`"."`, `"./..."`, `"../..."`) → resolve relative TỚI `project_root` (= parent của `.claude/`), KHÔNG phải `.claude/` directory, KHÔNG phải cwd
     - `null`/empty → fallback `~/.claude/wiki-global.json#wiki_root`
   - giữ toàn bộ object làm `config_hint`
4. Nếu `.workspace` rỗng → STOP:
   ```
   ✗ .claude/wiki.json thiếu field "workspace". Chạy /switch-workspace {name} hoặc /wiki-setup.
   ```
5. Set:
   - `workspace = config_hint.workspace`
   - `effective_wiki_root = <resolved wiki_root absolute>`
   - `project_dir = wiki_json_dir` (= project root, parent của `.claude/`)
6. Verify `{effective_wiki_root}/workspaces/{workspace}/workspace.md` tồn tại. Nếu không → STOP, báo workspace bị broken.

KHÔNG retrieve hay đọc file pattern/contract ở bước này — đó là việc của subagent.

---

## Bước 1 — Invoke `wiki-planner` (subagent)

Gọi Agent tool với `subagent_type=wiki-planner`. Prompt phải chứa:

- `user_task`: task gốc của user (nguyên văn)
- `effective_wiki_root`: đường dẫn tuyệt đối
- `workspace`: tên workspace active
- `config_hint`: nội dung `.claude/wiki.json` (nếu có), hoặc "none"

**Output mong đợi**: JSON intent đúng schema trong [agents/wiki-planner.md](.claude/agents/wiki-planner.md).

Nếu planner trả `MISSING INPUT` hoặc JSON có `missing_knowledge` không rỗng → review với user trước khi tiếp tục, KHÔNG tự đoán.

Lưu intent JSON vào biến `intent_json` trong context của main agent.

---

## Bước 2 — Invoke `wiki-context-selector` (subagent)

Gọi Agent tool với `subagent_type=wiki-context-selector`. Prompt phải chứa:

- `intent_json`: output JSON từ Bước 1
- `effective_wiki_root`
- `project_dir`
- `user_task` (để gắn vào header context file)

Subagent sẽ:
- Map intent → file wiki cụ thể theo `agents/pipeline/context-retrieval-map.md`
- Slice section liên quan theo `agents/pipeline/context-filter.md`
- Ghi đè `{project_dir}/.claude/context/current-task.md`

**Output mong đợi**: 1 dòng confirm `Context written: {path}, {N} docs, {M} gaps`.

Nếu confirm không xuất hiện hoặc file `current-task.md` không được tạo → STOP, báo lỗi cho user.

---

## Bước 2.5 — Invoke `wiki-plan-reviewer` (subagent)

Gọi Agent tool với `subagent_type=wiki-plan-reviewer`. Prompt phải chứa:

- `intent_json`: output từ Bước 1
- `context_file`: `{project_dir}/.claude/context/current-task.md`
- `effective_wiki_root`
- `user_task`

Subagent sẽ chạy 4 check (pattern/contract tồn tại, context đủ component, conflict nội tại, gap blocking) và trả:

- `APPROVED` (có thể kèm `## Warnings`) → tiếp tục Bước 3
- `BLOCK` kèm `## Issues` → STOP pipeline, báo user, KHÔNG tự sửa context

**Nếu BLOCK do pattern/contract thiếu** → đề xuất user `/update-wiki` để tạo trước, hoặc brief lại task để dùng pattern đã có.

**Nếu BLOCK do conflict nội tại** → cần update wiki để giải quyết conflict, KHÔNG tự bypass.

**Nếu APPROVED kèm Warnings** → ghi warnings vào cuối `current-task.md` dưới section `## Plan Review Warnings` để main agent thấy khi đọc lại ở Bước 3.

---

## Bước 3 — Verify gap & xác nhận trước khi code (main agent)

1. Đọc lại `{project_dir}/.claude/context/current-task.md`.
2. Trả lời 6 câu hỏi:
   - Workspace nào đang active?
   - Pattern nào sẽ được áp dụng?
   - Contract nào bị ràng buộc?
   - Project có override gì không?
   - Domain workflow có ràng buộc gì?
   - Section `## Knowledge Gaps` có entry nào không?

3. Nếu `## Knowledge Gaps` không rỗng → DỪNG, báo user, KHÔNG đoán, KHÔNG fallback workspace khác.

4. Nếu pass → tiếp tục Bước 4.

---

## Bước 4 — Code theo prompt template (main agent)

Main agent đóng vai builder. Output theo cấu trúc cố định trong `agents/pipeline/prompt-template.md`:

```
## Understanding
## Knowledge Mapping   ← link tới các section trong .claude/context/current-task.md
## Design
## Implementation
## Edge Cases
## Assumptions
```

Mọi quyết định kỹ thuật phải reference được vào 1 entry trong `## Referenced Docs` của `current-task.md`. Nếu phải vượt ra ngoài → ghi vào `## Assumptions`, không im lặng.

---

## Bước 5 — Review (subagent, tuỳ task)

Sau khi code đã sinh và file đã sửa, gọi Agent tool với `subagent_type=wiki-reviewer`:

- `solution_files`: danh sách file vừa Edit/Write
- `context_file`: `{project_dir}/.claude/context/current-task.md`
- `effective_wiki_root`

Output:
- `APPROVED` → chuyển user
- `Violations Found` → append vào cuối `current-task.md` dưới section `## Violations`, báo user trước khi commit. KHÔNG tự sửa khi user chưa duyệt.

**Bỏ qua Bước 5 khi**: task chỉ là `design`/`review` không sinh code, hoặc fix bug 1 dòng quá nhỏ.

---

## Quy tắc cứng

- KHÔNG inline làm việc của planner/context-selector trong main agent — luôn delegate qua Agent tool. Lý do: subagent có context window riêng, output có schema cố định, dễ audit.
- KHÔNG skip Bước 2 — `current-task.md` là single source of truth cho session này; thiếu nó thì reviewer cũng không hoạt động đúng.
- KHÔNG đọc file ngoài `workspaces/{workspace}/` (trừ `agents/` chung của wiki).
- Sau Bước 4, nếu phát hiện cần thêm context mới → re-run từ Bước 1, KHÔNG patch tay vào `current-task.md`.
