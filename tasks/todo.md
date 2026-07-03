# MVP: Team-Shared Agentic Memory (v2.1)

> Decisions locked 2026-07-03: git-per-workspace sync · fast-path note write · wire `sessions` table.
> Base: v2/07-oss-polish (78 tests pass, 20/23 AC closed).

## Goal

Small team (2–5 người) share memory qua git; Claude Code là runtime chính.
Taxonomy: workspaces = scope chia sẻ (`personal`, `team-shared`, `research`);
4 wiki tiers giữ nguyên bên trong mỗi workspace làm ranking. Không sửa schema tiers.

## Tasks (thứ tự thực hiện)

### 1. `zbrain sync <workspace>` — git sync layer
- [ ] Detect workspace root là git repo (`.git` tồn tại) → nếu không, hướng dẫn `zbrain workspace link <ws> <repo>`
- [ ] `sync` = `git pull --rebase` → incremental `reindex <ws>` → `git add/commit/push` (chỉ `wiki/`, `evidence/`, `workspace.md`; exclude `.trash/`?  → quyết khi implement: include để forget cũng sync)
- [ ] Conflict path: pull fail → báo rõ, không auto-resolve; supersede model khiến conflict hiếm
- [ ] Verify: 2 clone giả lập trong test tmp dir, ghi note ở A, sync, B sync → B `ask` thấy note

### 2. `zbrain note add` — fast-path write (bỏ qua evidence gate cho note trực tiếp)
- [ ] Reuse `note-service.ts` create + conflict detection + supersede hiện có
- [ ] CLI: `zbrain note add --tier <t> --title <t> [--supersedes <id>]`, body từ stdin/file
- [ ] MCP: thêm tool `add_note` (ghi wiki trực tiếp) song song `remember` (vẫn đi evidence)
- [ ] Evidence pipeline giữ nguyên cho `learn`/`ingest` (nguồn ngoài)
- [ ] Verify: test conflict bị chặn, supersede flow đúng, note mới retrievable ngay (incremental index)

### 3. Wire bảng `sessions` (SQLite session metadata)
- [ ] `ask`/MCP `recall`: upsert row (id, project_root, workspace, started_at, last_activity_at)
- [ ] Context body vẫn là file `.md` per-session (giữ nguyên)
- [ ] `doctor`: GC session idle > N ngày (row + file)
- [ ] Verify: test session row được ghi/update, doctor GC đúng

### 4. Đóng AC-P1-9 — giết `projects.json` mirror
- [ ] `init` không ghi `projects.json` nữa; engine rules đọc qua CLI/session file
- [ ] Migration: lần chạy đầu import `projects.json` cũ vào DB rồi rename thành `.bak`
- [ ] Verify: grep không còn write-path nào tới projects.json; tests pass

### 5. Team onboarding docs
- [ ] README section: teammate mới = clone repo → `zbrain setup` → `zbrain workspace link` → `zbrain sync`
- [ ] Ghi rõ: leases/locking chỉ bảo vệ multi-agent trên 1 máy; cross-machine do git lo

## Out of scope (MVP)
- Server/API sync, vectors/embeddings, fact graph, dynamic plugins
- AC-P2-4 dead-code PR, AC-P3-2 ULID migration (giữ deferred như AC-AUDIT)

## Success criteria
- 2 máy (giả lập 2 ZBRAIN_HOME) share 1 workspace qua git repo, ask thấy note của nhau sau sync
- Agent ghi note qua MCP `add_note` không cần review, nhưng conflict/supersede vẫn được enforce
- `bun test` xanh, typecheck xanh, `bun run build` ra binary chạy smoke được
