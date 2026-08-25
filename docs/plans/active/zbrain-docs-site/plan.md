# Plan: Docs site + Landing page cho zbrain (GitHub Pages)

Ngày: 2026-08-25 · Trạng thái: chờ xác nhận

## Mục tiêu

Trang web tài liệu + landing page cho zbrain, deploy qua GitHub Pages tại
`https://therealtinhtute.github.io/zbrain/`, thiết kế theo skill Hallmark với
theme **Garden** (botanical almanac).

## Quyết định đã chốt (qua AskUserQuestion)

| Điểm | Chọn |
|---|---|
| Vị trí | `site/` trong repo zbrain, deploy bằng GitHub Actions |
| Stack | Static HTML/CSS thuần, không build step |
| Phạm vi | Landing + docs đầy đủ (Getting started, Concepts, CLI reference, MCP, Architecture) |
| Audience | Developer dùng AI agents, cần local-first trusted memory |

## Ràng buộc

- Không đụng `docs/` — đó là tài liệu repo + managed docs của zharness.
- Nội dung site phải lấy từ sự thật hiện tại của repo: README.md, CLI surface
  (`internal/cli/cli.go`), runtime behavior, docs/diagrams/. Không bịa metrics,
  số liệu, hay lời chứng thực (Hallmark discipline: honest copy).
- Hallmark: theme Garden → genre editorial; token hệ thống đặt trong
  `site/tokens.css`; mọi màu qua `var(--color-*)`, không inline giá trị.
- Mobile 320/375/414/768 phải không tràn ngang; `prefers-reduced-motion`.

## Các bước

1. **Cấu trúc thư mục**
   - `site/` (index.html, docs/*.html, css/, js/, assets/)
   - `.github/workflows/pages.yml` — deploy `site/` lên Pages bằng
     `actions/configure-pages` + `upload-pages-artifact` + `deploy-pages`.

2. **Hallmark design pass (theme Garden)**
   - Pre-flight: vanilla static project, không có gì để preserve.
   - Genre: editorial. Theme: Garden — oat-cream paper `oklch(95.5% 0.022 92)`,
     botanical-ink `oklch(24% 0.052 152)`, leaf-green accent
     `oklch(47% 0.13 140)`, clay pop `oklch(54% 0.14 46)`; Young Serif display +
     Hanken Grotesk body + Geist Mono labels (như garden-01 example).
   - Macrostructure: Index-First / Long Document (chọn lúc implement, ưu tiên
     Long Document cho docs hub); nav + footer theo editorial cluster
     (N6 Masthead / N9 Edge-min; Ft6 Letter close).
   - Stamp CSS + `.hallmark/log.json` đầy đủ; chạy 58-gate slop test trước khi
     giao.

3. **Landing page (`site/index.html`)**
   - Nội dung lấy từ README: zbrain là gì (Go-native CLI, local-first trusted
     memory, workspace-isolated agent context), features (claim lifecycle,
     evidence snapshot, FTS5, fail-closed retrieval, MCP gateway, `zbrain view`),
     trust & data flow, installation (`go install ...`, `zbrain setup`).
   - CTA chính: hướng tới Getting started. Không bịa số liệu; dùng command
     output thật hoặc placeholder "—" có nhãn.

4. **Docs pages (`site/docs/*.html`)**
   - `getting-started.html` — install, setup, workspace create, quick tour
   - `concepts.html` — OKF claims, tiers, lifecycle, evidence, trust & retrieval
     outcomes (ready/gap/blocked)
   - `cli.html` — CLI reference từ `internal/cli/cli.go` (dùng `--help` thật)
   - `mcp.html` — `zbrain mcp serve`, approval ceremony, `zbrain view`
   - `architecture.html` — dùng 2 diagram sẵn có trong `docs/diagrams/`
     (copy vào `site/assets/`, KHÔNG sửa nguồn)

5. **Điều hướng + UX site**
   - Nav đơn giản, link giữa các trang docs, footer liên kết repo + license.
   - Không fake chrome (không mô phỏng terminal giả — dùng `<pre><code>` thật
     với command output thật).

6. **GitHub Pages**
   - Workflow `.github/workflows/pages.yml` deploy từ `site/`.
   - Bước thủ công một lần: bật Pages (Settings → Pages → Source: GitHub
     Actions) — không thể automate từ CLI trừ khi dùng API admin, ghi vào PR
     checklist.

7. **Kiểm chứng**
   - `go test ./...` (đảm bảo không phá gì — thực tế không đụng Go).
   - Mở site bằng browser: 4 kích thước mobile + desktop, không horizontal
     scroll; run slop test 58 gates.
   - Chờ Actions chạy xanh, xác nhận URL `https://therealtinhtute.github.io/zbrain/`.

## Deliverables session này

- `plan.md` (file này)
- `pr-spec.md` — mô tả PR đầy đủ: tóm tắt, files, verification

## Ghi chú

- zharness preflight `to-plan` bị blocked: managed docs stale
  (`docs/playbooks/{brainstorm,handoff,work}.md` khác upstream 0.12.0, có
  local commit chủ ý). Không tự overwrite; cần owner quyết khi nào refresh.
  Việc này không chặn planning (read-only) nhưng cần resolve trước khi chạy
  workflow stage.
