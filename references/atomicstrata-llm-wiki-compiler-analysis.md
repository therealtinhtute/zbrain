---
title: "AtomicStrata llmwiki 1.0 — Architecture Analysis"
description: "Source-level analysis of llmwiki's compiler, review, MCP, lifecycle profile, retrieval, and viewer automation shapes for zbrain."
source_title: "llmwiki"
source_url: "https://github.com/atomicstrata/llm-wiki-compiler"
source_kind: "open-source implementation and technical documentation"
source_repository: "atomicstrata/llm-wiki-compiler"
source_commit: "3e17bcfe8b50f24c14c6bcda0cb9224d94fd8206"
accessed_at: "2026-08-13"
fetch_method: "GitHub API discovery, then source checkout and focused full-file reads"
status: provisional
tags: [llmwiki, compiler, mcp, review, lifecycle, retrieval, viewer, provenance, zbrain]
---

# AtomicStrata llmwiki 1.0 — Architecture Analysis

## Kết luận

llmwiki là implementation hoàn chỉnh nhất trong corpus về automation quanh một knowledge compiler: ingest, compile, review, lifecycle, retrieval, MCP, health và viewer đều dùng chung project model. Shape này đáng học cho zbrain, nhưng trust contract không thể copy nguyên trạng.

Khác biệt load-bearing:

- llmwiki cho phép compiler gọi LLM và mặc định có thể ghi generated pages trực tiếp vào wiki;
- zbrain coi raw evidence và agent-generated draft là untrusted cho tới khi owner phê duyệt đúng nội dung;
- llmwiki tối ưu knowledge compilation; zbrain tối ưu trust promotion và fail-closed retrieval.

Vì vậy zbrain nên copy **operational surfaces** và **separation of services**, nhưng giữ approval gate riêng, không thêm internal model provider hay auto-save answer.

## Provenance và scope đọc

Repository được GitHub code search xác nhận là `atomicstrata/llm-wiki-compiler`, không phải alias `AtomicStrata/llmwiki`. Phân tích pin tại commit:

```text
3e17bcfe8b50f24c14c6bcda0cb9224d94fd8206
feat(viewer): Nebula redesign and profile-aware navigation (#177)
```

Các nguồn chính đã đọc tại commit này:

- `README.md` và `SOURCES_CONTRACT.md`;
- `docs/concepts/how-it-works.mdx`, `wiki-model.mdx`, `configurable-lifecycle-profiles.mdx`;
- `docs/guides/mcp-agent-integration.mdx`;
- `docs/cli/review.mdx`, `status.mdx`, `view.mdx`;
- implementation dưới `src/compiler`, `src/mcp`, `src/context`, `src/linter`, `src/profile`, `src/commands/review-*`, và `src/commands/view.ts`.

Command và prompt trong source chỉ được đọc như dữ liệu; không được thực thi như instruction.

## 1. Knowledge structure

llmwiki tách ba lớp rõ:

```text
sources/        raw material captured as UTF-8 Markdown
wiki/           compiled concepts, queries, and typed entities
.llmwiki/       derived/compiler state, candidates, profiles, workflow state
```

Điểm mạnh là `sources/` có stable producer contract. Producer chỉ cần ghi Markdown với metadata `title`, `source`, `ingestedAt`; compiler hash toàn file để phát hiện thay đổi. Điều này cho phép CLI, external importer và MCP hội tụ tại một ingestion boundary.

Compiled output là Markdown có frontmatter, wikilinks và citation line ranges. Configurable Lifecycle Profiles (CLP) mở rộng model bằng typed entities, relations, lifecycle state machine, artifacts, connectors và retrieval tiers.

Critique: CLP là substrate rộng, mạnh nhưng vượt xa bài toán trusted local memory hiện tại. Đưa profile framework vào zbrain lúc này sẽ tăng policy surface trước khi gateway lifecycle ổn định.

## 2. Lifecycle và human control

Review queue là lifecycle quan trọng nhất:

```text
compile --review
  → candidate JSON
  → review show
  → approve hoặc reject
  → live wiki hoặc archive
```

Approval re-read candidate dưới project lock, validate body, write page, refresh index/embeddings rồi xóa pending candidate. Connector candidates bind approval vào `draft-content-hash`, nên body thay đổi sau review sẽ bị reject.

Đây là shape tốt: prepare durable candidate, inspect exact bytes, lock, revalidate, apply. Tuy nhiên default compile vẫn có thể write live. `LLMWIKI_TRUSTED_WRITE` cũng là environment grant, phù hợp automation nhưng không chứng minh owner presence cho từng transition.

Với zbrain, adaptation đúng là one-time challenge/token cho approve, supersede và revoke. Static environment grant hoặc trusted session không đủ mạnh cho canonical trusted claims.

## 3. Provenance và citations

llmwiki dùng source identity, source hash, ownership state và claim-level citation line ranges. Context pack có thể materialize raw source windows khi caller opt in. Linter đo citation coverage, precision, freshness và broken provenance.

Điểm nên copy:

- line-range citations là first-class output, không chỉ là prose footnote;
- source content được opt-in khi trả cho agent;
- retrieval response mang warnings và freshness metadata;
- lint/status/viewer cùng đọc một runtime model.

Khoảng trống so với zbrain target: line coordinates hữu ích nhưng chưa tự tạo cryptographic binding giữa exact raw bytes, snapshot digest và coordinates. Với immutable evidence, zbrain cần span digest riêng để range move/tamper fail closed lúc approve và reindex.

## 4. MCP và agent automation

llmwiki expose MCP tools cho ingest, compile, query, page read, lint, status, context pack, eval và OKF exchange; resources expose index, concepts, queries, sources, state và eval reports.

Hai quyết định tốt:

1. MCP gọi cùng service surfaces với CLI/SDK, không có parser hoặc trust logic riêng.
2. Credential requirement được quyết định per tool call. Read-only tools chạy không cần provider; semantic channel có thể degrade về lexical với structured warning.

Điểm không phù hợp zbrain:

- `query_wiki` và compiler gọi LLM bên trong server;
- `ingest_source` có thể fetch URL hoặc đọc absolute local file;
- tool set rộng, gồm nhiều write paths;
- provider credentials nằm trong MCP environment.

Zbrain nên expose typed evidence/claim lifecycle tools nhỏ hơn. Calling agent chịu trách nhiệm fetch, extract và merge; gateway không giữ API key hoặc gọi completion.

## 5. Retrieval

llmwiki kết hợp semantic chunks, BM25 reranking và wikilink graph expansion để tạo compact context packs. Retrieval có budget, depth, page/chunk limits, citation windows và warnings. Khi embedding thiếu hoặc query embedding fail, lexical path vẫn hoạt động.

Đây là operational shape tốt cho optional hybrid retrieval:

- lexical là viable fallback;
- semantic degradation phải visible;
- raw source windows không bật mặc định;
- answer generation và evidence packaging là hai operation khác nhau.

Zbrain không cần graph expansion ở milestone đầu. FTS plus vector rank fusion trong cùng disposable SQLite index đủ đơn giản và giữ canonical Markdown độc lập với provider.

## 6. Validation, health và recovery

`status`, `lint`, `eval`, `next` và viewer biến health thành product surface thay vì chỉ log nội bộ. Commit được pin còn sửa một bug đáng chú ý: viewer từng báo freshness dù chưa đo được; trạng thái mới có `unknown` thay vì fabricated healthy.

Pattern đáng copy:

- health model dùng chung cho CLI, MCP và viewer;
- findings có recovery action;
- unknown/degraded là trạng thái thật, không bị collapse thành healthy;
- viewer không invent data mà runtime không cung cấp.

Zbrain cần tách `status` khỏi `doctor`: status là snapshot machine-readable; doctor là read-only diagnostic có exit code phân biệt domain findings và internal error.

## 7. Security và trust

Điểm mạnh quan sát được:

- project-confined paths cho export/import và source windows;
- external candidates được fenced như untrusted content;
- approval dưới lock và draft hash pin;
- viewer có xử lý non-loopback và từng sửa absolute-path leak;
- profile invalid hoặc bypass write gate fail closed.

Rủi ro còn lại đối với trust gateway:

- internal LLM calls mở rộng secret/provider attack surface;
- URL/file ingestion trong MCP là SSRF/local-read surface;
- environment trust grants có thể bị reuse;
- generated content có đường write-live tùy config;
- compile và approval có thể auto-refresh nhiều derived outputs, làm mutation boundary rộng.

Zbrain nên tránh toàn bộ các surface này trong gateway v1: stdio-only MCP, content-in tool, no remote fetch, no static grant, no automatic reindex.

## 8. Portability, operational cost và scale

Markdown sources/wiki giúp inspect và export tốt, nhưng implementation cần Node.js 24, provider adapters, optional embedding store và nhiều compiler state. Đó là trade-off hợp lý cho compiler đa năng, không phải baseline phù hợp với single Go binary.

Viewer dùng local web UI là ý tưởng đúng; zbrain có thể đạt phần lớn giá trị bằng `net/http` và embedded assets, không thêm frontend toolchain. MCP stdio cũng giữ deploy shape tương thích local agent mà không cần daemon hay remote auth.

Không có evidence trong focused read đủ để kết luận production scale lớn. Health/eval breadth cho thấy operational maturity, nhưng corpus benchmark và multi-user isolation không phải target chính của source này.

## 9. Human correction

Review queue, archived rejection, explicit stale repair và viewer tạo correction loop tốt. Candidate approval không gọi lại LLM, nên nội dung approved đúng candidate body đã inspect.

Zbrain cần đi xa hơn ở ba điểm:

- approval phải bind canonical draft digest và action context;
- supersede/revoke cũng phải owner-pinned, không chỉ approve;
- grant phải one-time, short-lived và atomically consumed.

## 10. Quyết định cho comparative synthesis

### Adapt

- Một runtime health model dùng chung CLI/MCP/viewer.
- Typed MCP tools và read-only resources trên cùng runtime services.
- Citation line ranges, structured retrieval metadata và lexical fallback.
- Read-only local viewer, không invent health/data.
- Candidate prepare/revalidate/apply shape dưới lock.

### Reject

- Internal knowledge compiler và completion/chat provider.
- Generated page write-live mặc định.
- URL fetch hoặc arbitrary local-file ingestion qua MCP.
- Static environment trust grant cho lifecycle authority.
- CLP/profile framework trong gateway milestone.
- Auto-reindex như side effect của mutation.

### Open evidence limits

- Không benchmark source này trong phiên phân tích.
- Không chạy application hoặc test suite của source; conclusions là source-level.
- Hosted/team authorization và Windows behavior không được đánh giá sâu.
