# Tạo Workspace Mới

Scaffold một workspace mới trong `{wiki_root}/workspaces/{name}/` từ `templates/workspace.md`.

## Bước 0 — Resolve `wiki_root`

1. Tìm `.claude/wiki.json` (nếu có): từ `<cwd>` đi lên parent. Lưu `wiki_json_dir`.
2. Đọc `wiki_root` resolve theo [agents/system-prompt.md `wiki_root` Resolution Rule](../../agents/system-prompt.md):
   - Absolute path → dùng nguyên
   - Relative (`"."`, `"./..."`) → resolve relative TỚI `project_root` (= parent của `.claude/`)
   - `null`/empty → fallback `~/.claude/wiki-global.json#wiki_root`
3. Nếu cả hai đều thiếu → STOP, hướng dẫn user chạy `bash {wiki-template}/scripts/install-to-claude.sh` trước.

## Bước 1 — Thu thập metadata

Hỏi user (dùng AskUserQuestion nếu chưa rõ):

| Field | Yêu cầu |
|-------|---------|
| `name` | kebab-case, không trùng workspace nào trong `workspaces/` |
| `company` | tên công ty / khách hàng |
| `role` | vị trí của user trong workspace này |
| `period` | YYYY-MM (start) → present hoặc YYYY-MM (end) |
| `stack.languages` | comma-separated |
| `stack.messaging` | kafka / mqtt / rabbit / sqs / none |
| `stack.storage` | postgres / mongo / mysql / redis / ... |

Nếu user chỉ đưa `name`, dùng default placeholder cho các field khác và đánh dấu "TBD" trong file.

## Bước 2 — Validate

- `workspaces/{name}/` chưa tồn tại. Nếu có → STOP, báo user chọn tên khác.
- `name` chỉ chứa `[a-z0-9-]`. Nếu vi phạm → STOP.
- `templates/workspace.md` tồn tại. Nếu thiếu → STOP, báo user repo bị lỗi.

## Bước 3 — Tạo cấu trúc folder

```
workspaces/{name}/
├── platform/
│   ├── architecture/
│   ├── contracts/
│   ├── infrastructure/
│   └── patterns/
├── domains/
├── projects/
├── runbooks/
├── decisions/
└── agents/                  # rỗng — chỉ tạo nếu workspace cần override
```

Dùng `mkdir -p` (Bash trên Windows: forward slashes ok).

## Bước 4 — Generate workspace.md

Đọc `templates/workspace.md`, thay placeholder bằng metadata ở Bước 1, ghi ra `workspaces/{name}/workspace.md`.

## Bước 5 — Generate patterns-index.md trống

Ghi `workspaces/{name}/patterns-index.md` với nội dung:

```md
# Patterns Index — {name}

Quick lookup table for AI agents. Find the pattern name, follow the link, read before generating code.

> Paths are relative to this file (workspace root).

## Platform Patterns

| Pattern | When to Use | Path |
|---------|-------------|------|
| _(empty — thêm khi tạo pattern đầu tiên trong `platform/patterns/`)_ | | |

## Contracts

| Contract | What It Governs | Path |
|----------|----------------|------|
| _(empty)_ | | |

## Domain Workflows

| Domain | Path |
|--------|------|
| _(empty)_ | |
```

## Bước 6 — Hỏi switch cho codebase hiện tại

Hỏi user (AskUserQuestion 2 options):

> "Set workspace `{name}` cho codebase hiện tại (`<cwd>`)?"

- Yes → cập nhật field `workspace = "{name}"` trong `<cwd>/.claude/wiki.json` (tạo file minimal nếu chưa có — xem `/switch-workspace` Bước 3).
- No → in hướng dẫn `/switch-workspace {name}` để dùng sau ở codebase phù hợp.

> Active workspace là per-codebase. Tạo workspace mới KHÔNG tự động set nó làm active ở mọi nơi — phải chạy `/switch-workspace` trong codebase muốn dùng.

## Bước 7 — Confirm

In bảng tóm tắt:

```
✓ Workspace created: {wiki_root}/workspaces/{name}/
  - workspace.md       ({company}, {role})
  - patterns-index.md  (empty)
  - 7 folders          (platform/{arch,contracts,infra,patterns}, domains, projects, runbooks, decisions)

Active for this codebase: {name | unchanged}    (file: <cwd>/.claude/wiki.json)

Next steps:
  1. Thêm platform contracts vào {wiki_root}/workspaces/{name}/platform/contracts/
  2. Thêm patterns vào workspaces/{name}/platform/patterns/ và update patterns-index.md
  3. Tạo project đầu tiên: workspaces/{name}/projects/{project}/knowledge-map.md (dùng templates/service.md cho service docs)
```
