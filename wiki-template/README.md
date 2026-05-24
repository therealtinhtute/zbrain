# Knowledge Wiki

## Purpose

Single source of truth for system knowledge — consumed by both developers and AI agents, **được tổ chức theo workspace** vì user làm việc ở nhiều công ty/dự án độc lập.

> AI agents read this wiki as **runtime context**, not documentation. Write for machines, not just humans. Mỗi workspace là một sandbox knowledge — knowledge của workspace này KHÔNG bao giờ áp dụng cho task của workspace khác.

## Mental Model

Wiki = **engine** (dùng chung) + **N workspaces** (mỗi nơi 1 sandbox).

```
wiki-template/
├── agents/         ← ENGINE — system prompt, pipeline, coding rules (workspace-agnostic)
├── templates/      ← ENGINE — skeleton cho workspace mới và doc mới
├── .claude/        ← ENGINE — slash commands
└── workspaces/     ← N workspaces, mỗi cái có platform/domains/projects/... riêng
    └── {name}/...

# Active workspace là per-codebase, sống trong <project>/.claude/wiki.json.workspace
# (không có file pointer global trong wiki-template)
```

## Engine — Folder Reference

| Folder | Purpose |
|--------|---------|
| [agents/](agents/) | System prompt, coding rules, constraints, pipeline (intent → retrieval → filter → validate). Engine defaults áp dụng mọi workspace; workspace có thể override tại `{ws}/agents/`. |
| [templates/](templates/) | Skeleton cho workspace mới ([workspace.md](templates/workspace.md)) và doc mới ([service.md](templates/service.md), [adr.md](templates/adr.md), [runbook.md](templates/runbook.md)) |
| [.claude/commands/](.claude/commands/) | Slash commands cho Claude Code |

## Workspaces — Folder Reference

Xem [workspaces/README.md](workspaces/README.md) để biết cấu trúc một workspace + cơ chế active pointer + override.

Mỗi workspace có:

| Folder | Purpose |
|--------|---------|
| `platform/` | Shared technical knowledge của công ty/dự án: patterns, contracts, architecture, infra |
| `domains/` | Business domain rules và workflows |
| `projects/` | Per-system implementation knowledge |
| `runbooks/` | Incident handling procedures |
| `decisions/` | ADRs cấp workspace |
| `patterns-index.md` | Quick lookup table cho patterns/contracts của workspace |
| `workspace.md` | Metadata: company, role, period, stack |
| `agents/` (optional) | Override engine constraints/validator-rules |

## How to Use

### Cài lần đầu (làm 1 lần)

Sync slash commands + subagents vào `~/.claude/` và setup `~/.claude/wiki-global.json`:

```bash
bash scripts/install-to-claude.sh             # sync mọi thứ
bash scripts/install-to-claude.sh --dry-run   # preview
bash scripts/install-to-claude.sh --force     # overwrite global config nếu cần
```

Re-run mỗi khi `git pull` wiki-template để cập nhật commands/agents mới nhất. Script idempotent, không động vào file user tự tạo trong `~/.claude/commands/`.

### Bắt đầu phiên làm việc (TRONG codebase)

```
/list-workspaces                    # xem có workspace nào, codebase này đang dùng workspace nào
/switch-workspace {name}            # đổi workspace cho codebase hiện tại (ghi <cwd>/.claude/wiki.json)
```

> Mỗi codebase tự khai báo workspace nó dùng trong `.claude/wiki.json`. Chạy `/wiki-setup` lần đầu để tạo file này.

### Khi nhận task

```
/use-wiki "Add Kafka consumer..."   # auto: parse intent → retrieve {ws}/... → ghi .claude/context/current-task.md
```

### Sau khi code xong

```
/update-wiki                        # đồng bộ wiki với code đã thay đổi (chỉ trong workspace active)
```

### Khi onboard project mới hoặc nghi ngờ wiki lỗi thời

```
/rebase-wiki                        # verify wiki vs codebase trong workspace active
```

### Tạo workspace mới (vào công ty mới)

```
/new-workspace {name}               # scaffold từ templates/workspace.md
```

## Priority Order (for AI agents)

```
Contracts > Platform Patterns > Project Documentation > Domain Knowledge
```

Áp dụng **trong scope của workspace active** — KHÔNG mix knowledge giữa các workspace.

## Maintenance Rules

- **Single source of truth per workspace**: patterns sống trong `{ws}/platform/patterns/`, không copy giữa workspaces
- **Link, don't duplicate**: cross-reference bằng relative path trong cùng workspace
- **Rebase continuously**: khi code thay đổi, chạy `/update-wiki` (mỗi workspace tự sync)
- **Agent-first design**: viết để AI parse được, không chỉ cho người
- **Workspace isolation is sacred**: nếu rule áp dụng cho mọi workspace → để ở engine; nếu chỉ 1 workspace → để trong `{ws}/agents/`
