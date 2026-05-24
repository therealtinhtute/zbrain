# Liệt Kê Workspaces

In bảng tất cả workspace trong `{wiki_root}/workspaces/` và đánh dấu workspace của codebase hiện tại.

## Bước 0 — Resolve `wiki_root` và active

1. Tìm `.claude/wiki.json` (nếu có): từ `<cwd>` đi lên parent. Lưu `wiki_json_dir`.
2. Đọc:
   - `wiki_root` resolve theo [agents/system-prompt.md `wiki_root` Resolution Rule](../../agents/system-prompt.md):
     - Absolute path → dùng nguyên
     - Relative (`"."`, `"./..."`) → resolve relative TỚI `project_root` (= parent của `.claude/`)
     - `null`/empty → fallback `~/.claude/wiki-global.json#wiki_root`
   - `workspace` → đây là **active của cwd**
3. Nếu cwd không có `.claude/wiki.json` → fallback `~/.claude/wiki-global.json#default_workspace` làm active.
4. Nếu không xác định được `wiki_root` → STOP, hướng dẫn user chạy `/wiki-setup`.

## Bước 1 — Quét workspace folders

- Glob: `{wiki_root}/workspaces/*/workspace.md`.
- Với mỗi file, parse Identity block để lấy: company, role, period.
- Nếu workspace folder tồn tại nhưng thiếu `workspace.md` → liệt kê với cờ `⚠ missing workspace.md`.

## Bước 2 — In bảng

```
Codebase: {cwd}
Active for this codebase: {active}    (source: .claude/wiki.json | global default | none)

| Name              | Company           | Role               | Period           |
|-------------------|-------------------|--------------------|------------------|
| ▶ example-surgery | Example Hospital  | Backend Engineer   | 2026-01 → present |
|   company-b       | ACME Corp         | Senior Backend     | 2025-06 → 2025-12 |
|   ⚠ broken-folder | (no workspace.md) |                    |                  |
```

Workspace active có dấu `▶` ở đầu Name.

## Bước 3 — Hint

Cuối output, in:

```
Switch (this codebase): /switch-workspace {name}
Create new           : /new-workspace {name}
Setup full config    : /wiki-setup
```
