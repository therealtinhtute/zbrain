# PR Spec: Docs site + Landing page (GitHub Pages, Hallmark · Garden)

## Summary

Thêm website tài liệu + landing page cho zbrain: static HTML/CSS thuần trong
`site/`, thiết kế theo Hallmark theme Garden (botanical almanac), deploy tự
động lên GitHub Pages bằng GitHub Actions.

## Motivation

zbrain hiện chỉ có README + docs/ nội bộ. Một site docs giúp dev dùng AI
agents khám phá, cài đặt và hiểu trust model trước khi chạy CLI. Không thay
đổi runtime Go.

## Changes

### New

| Path | Nội dung |
|---|---|
| `site/index.html` | Landing page — value prop, features, trust flow, install CTA |
| `site/docs/getting-started.html` | Install, setup, workspace, quick tour |
| `site/docs/concepts.html` | OKF claims, tiers, lifecycle, evidence, trust outcomes |
| `site/docs/cli.html` | CLI reference (sinh từ `--help` thật) |
| `site/docs/mcp.html` | `zbrain mcp serve`, approval ceremony, `zbrain view` |
| `site/docs/architecture.html` | Kiến trúc + trust/data flow diagrams |
| `site/tokens.css` | Hallmark token system (Garden) — bắt buộc theo skill |
| `site/css/main.css` | Styles chính, stamp Hallmark ở dòng đầu |
| `site/js/main.js` | Tối thiểu: nav mobile, active link, reduced-motion |
| `site/assets/` | Copy 2 diagram từ `docs/diagrams/` (không sửa nguồn) |
| `.github/workflows/pages.yml` | Deploy `site/` lên Pages |

### Modified

- Không có. Không đụng Go code, `docs/`, `assets/` (runtime assets), README.

## Design decisions (Hallmark)

- Theme: **Garden** (user chọn) — paper oat-cream, ink botanical green,
  accent leaf-green + clay pop; Young Serif / Hanken Grotesk / Geist Mono.
- Genre: editorial. Macrostructure: Long Document (docs hub), nav/footer theo
  editorial cluster. Stamp CSS + `.hallmark/log.json`.
- Honest copy: mọi command, output, số liệu đều thật từ repo; không bịa
  metrics/testimonials. Không fake browser/terminal chrome.
- Mobile 320–768 không tràn ngang; `prefers-reduced-motion` được tôn trọng.
- Chạy 58-gate slop test trước khi sẵn sàng review.

## Deployment

- Workflow: `on: push` (master, path `site/**` + workflow) →
  `configure-pages` → `upload-pages-artifact` → `deploy-pages`.
- One-time manual step (ghi trong PR checklist): bật GitHub Pages
  (Settings → Pages → Source: GitHub Actions).
- URL: `https://therealtinhtute.github.io/zbrain/`

## Verification

1. `go test ./...` — vẫn xanh (không đổi Go code).
2. Preview local: `python3 -m http.server -d site` rồi mở browser kiểm tra
   landing + 5 docs pages, link không gãy.
3. Responsive: 320 / 375 / 414 / 768 / desktop, không horizontal scroll.
4. Hallmark slop test: 58/58.
5. CI: workflow pages chạy xanh, truy cập URL production thành công.

## Known limitations

- Không có search trên site (có thể thêm sau — client-side index đơn giản).
- Site là static — cập nhật CLI reference cần commit mới khi CLI đổi.
- zharness managed docs đang stale (docs/playbooks/{brainstorm,handoff,work}.md
  vs upstream 0.12.0) — không liên quan PR này nhưng chặn workflow stage.
