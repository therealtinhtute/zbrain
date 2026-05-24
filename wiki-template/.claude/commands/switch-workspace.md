# Đổi Active Workspace

Đổi workspace cho **codebase hiện tại** bằng cách cập nhật field `workspace` trong `<cwd>/.claude/wiki.json`.

> Active workspace là per-codebase. Mỗi project repo tự khai báo workspace của nó trong `.claude/wiki.json`. Command này chỉ tác động lên cwd hiện tại — KHÔNG có file pointer global ở wiki-template.

## Bước 0 — Resolve `wiki_root`

1. Tìm `.claude/wiki.json` (nếu có): từ `<cwd>` đi lên parent. Lưu `wiki_json_dir`.
2. Đọc `wiki_root` resolve theo [agents/system-prompt.md `wiki_root` Resolution Rule](../../agents/system-prompt.md):
   - Absolute path → dùng nguyên
   - Relative (`"."`, `"./..."`) → resolve relative TỚI `project_root` (= parent của `.claude/`)
   - `null`/empty → fallback `~/.claude/wiki-global.json#wiki_root`
3. Nếu cả hai đều thiếu → STOP, yêu cầu user chạy `/wiki-setup` trước.

## Bước 1 — Lấy target name

- Nếu user truyền tham số: dùng làm `{name}`.
- Nếu không: liệt kê `{wiki_root}/workspaces/*/workspace.md`, hỏi user chọn (AskUserQuestion).

## Bước 2 — Validate

- `{wiki_root}/workspaces/{name}/` phải tồn tại và là directory.
- `{wiki_root}/workspaces/{name}/workspace.md` phải tồn tại.

Nếu fail → STOP, in lỗi rõ ràng:

```
✗ Workspace "{name}" không tồn tại hoặc thiếu workspace.md.
  Có sẵn: {list từ /list-workspaces}
  Tạo mới: /new-workspace {name}
```

KHÔNG ghi gì khi validate fail.

## Bước 3 — Cập nhật `<cwd>/.claude/wiki.json`

Hai trường hợp:

### 3a. File đã tồn tại

Đọc, set `.workspace = "{name}"`, ghi đè (giữ nguyên các field khác). Nếu `.workspace` cũ ≠ `{name}` → in dòng `Previous: {old} → {name}`.

### 3b. File chưa tồn tại

Tạo `<cwd>/.claude/wiki.json` minimal:

```json
{
  "project": "{cwd basename}",
  "workspace": "{name}",
  "wiki_root": null
}
```

Nhắc user chạy `/wiki-setup` để fill các field còn lại (`knowledge_map`, `domain`, `patterns`, `contracts`, `services`).

## Bước 4 — Confirm

Đọc `{wiki_root}/workspaces/{name}/workspace.md`, parse Identity block, in:

```
✓ Active workspace cho codebase này: {name}
  Company: {company}
  Role:    {role}
  Period:  {period}

Pipeline scope: {wiki_root}/workspaces/{name}/
Config file:    <cwd>/.claude/wiki.json
```

## Bước 5 — Reset task context (optional)

Nếu file `<cwd>/.claude/context/current-task.md` tồn tại VÀ field `Workspace:` của nó khác `{name}` → cảnh báo user:

```
⚠ .claude/context/current-task.md đang ghi context cho workspace "{old}".
  Chạy /use-wiki để regenerate cho workspace mới trước khi tiếp tục code.
```

## Bước 6 — (Optional) Set global default

Nếu user pass flag `--global` HOẶC chưa có `~/.claude/wiki-global.json.default_workspace` → hỏi:

> "Set `default_workspace = {name}` trong `~/.claude/wiki-global.json` để các codebase mới (chưa có `.claude/wiki.json`) mặc định dùng workspace này?"

Yes → cập nhật global config.
