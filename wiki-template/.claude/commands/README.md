# Slash Commands — Index

Đây là slash commands cho workflow wiki-aware. Mọi command resolve **active workspace** từ `<cwd>/.claude/wiki.json` (xem [`wiki_root` Resolution Rule](../../agents/system-prompt.md) để hiểu cách path được resolve). Pipeline kiến trúc tổng thể: [agents/pipeline/README.md](../../agents/pipeline/README.md).

Quy tắc chung:
- Mỗi command có Bước 0 resolve workspace + wiki_root trước khi làm bất cứ gì.
- Mọi knowledge access scope CHỈ trong workspace active (không cross-workspace).
- Command edit wiki (update-wiki, rebase-wiki, evidence-apply) delegate qua `wiki-curator` subagent + main agent verify path sau khi curator return.

---

## Workspace setup & navigation

| Command | Purpose | When to use |
|---------|---------|-------------|
| [`/wiki-setup`](wiki-setup.md) | Tạo `.claude/wiki.json` cho codebase hiện tại; detect project name + components và pre-fill config | Lần đầu tích hợp wiki vào codebase mới, hoặc đổi workspace |
| [`/wiki-detect`](wiki-detect.md) | Validate `.claude/wiki.json` của codebase + check workspace tồn tại + scan dependency để propose update | Sanity check sau khi setup, hoặc khi `/use-wiki` lỗi resolve |
| [`/switch-workspace`](switch-workspace.md) `{name}` | Đổi `workspace` field trong `<cwd>/.claude/wiki.json` sang workspace khác | Khi cùng codebase phục vụ nhiều domain workspace, hoặc khi clone codebase nội bộ |
| [`/new-workspace`](new-workspace.md) `{name}` | Scaffold workspace mới trong `{wiki_root}/workspaces/{name}/` từ template | Khi join công ty/dự án mới, cần knowledge sandbox riêng |
| [`/list-workspaces`](list-workspaces.md) | In bảng mọi workspace + đánh dấu workspace của codebase hiện tại | Khám phá workspace nào có sẵn trước khi `/switch-workspace` |

---

## Wiki usage (per-task pipeline)

| Command | Purpose | When to use |
|---------|---------|-------------|
| [`/use-wiki`](use-wiki.md) | Chạy 5-stage pipeline (planner → context-selector → plan-reviewer → main agent code → reviewer) trước khi viết bất kỳ code wiki-aware nào | Trước MỌI task implement_feature / fix_bug / design / incident / review |
| [`/update-wiki`](update-wiki.md) | Sync wiki với code đã thay đổi (git diff → curator áp dụng) | Sau khi code merge để wiki không drift; tự detect engine vs workspace scope |
| [`/rebase-wiki`](rebase-wiki.md) | Quét wiki vs codebase thực tế để vá mọi chỗ wiki nói khác code chạy | Định kỳ (hằng tuần/tháng) hoặc khi nghi wiki lỗi thời lớn |

---

## Codebase analysis (bootstrap wiki từ source code)

| Command | Purpose | When to use |
|---------|---------|-------------|
| [`/code-analyze`](code-analyze.md) | Snapshot metadata codebase → ingest vào evidence pipeline với `source_type=code` → sinh proposals patterns/contracts/services/ADRs để đưa vào wiki | Onboard codebase legacy; refresh platform knowledge sau major change; bootstrap workspace mới |

> Dưới capô gọi `/evidence-ingest --source code` rồi `/evidence-analyze` (CORE-CODE prompts c01–c04 + CORE 4/8). Sau đó user dùng `/evidence-qa` + `/evidence-apply` như mọi evidence khác.

---

## Evidence pipeline (raw data → wiki)

Pipeline 4 bước: ingest → analyze → qa → apply. Áp dụng cho mọi `source_type` ∈ {paste, api, mcp, code}.
Reference: [agents/pipeline/evidence-state-rules.md](../../agents/pipeline/evidence-state-rules.md), [agents/pipeline/critical-analysis-prompts.md](../../agents/pipeline/critical-analysis-prompts.md) (text), [agents/pipeline/code-analysis-prompts.md](../../agents/pipeline/code-analysis-prompts.md) (code).

| Command | Purpose | When to use |
|---------|---------|-------------|
| [`/evidence-ingest`](evidence-ingest.md) | Pull raw data từ MCP / API / paste / code vào `{ws}/evidence/sources/{evid-id}/` (immutable sau ingest) | Khi có nguồn ngoài (Confluence, Linear, Slack, doc paste, codebase) cần đưa vào wiki |
| [`/evidence-analyze`](evidence-analyze.md) | Chạy CORE prompts sinh `analysis/{id}/`. Text: `[01,02,04,08]`. Code: `[c01,c02,c03,c04,04,08]` | Sau ingest, để có analysis cho Q&A |
| [`/evidence-qa`](evidence-qa.md) | Q&A loop với user theo batches P0/P1/P2/P3, defer-to-expert option, sinh `verified-facts.md` khi xong | Sau analyze, để verify facts trước khi apply vào wiki |
| [`/evidence-apply`](evidence-apply.md) | Apply verified facts vào wiki docs với checkpoint/resume per-file. Router edit-vs-create theo `Affects:` path | Sau QA done; wrap `/update-wiki` hoặc `/rebase-wiki` để có manifest audit trail |

---

## Related references

- [agents/pipeline/README.md](../../agents/pipeline/README.md) — Pipeline architecture (5 stage)
- [agents/pipeline/multi-agent-pipeline.md](../../agents/pipeline/multi-agent-pipeline.md) — Subagent roles + I/O schema
- [agents/system-prompt.md](../../agents/system-prompt.md) — Engine system prompt + wiki_root resolution rule
- [workspaces/README.md](../../workspaces/README.md) — Workspace philosophy (per-codebase, no cross-workspace knowledge bleed)
