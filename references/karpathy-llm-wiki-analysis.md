---
title: "Karpathy LLM Wiki via OpenKnowledge — Deep Analysis"
description: "Deep analysis of the LLM-wiki pattern, OpenKnowledge's provisional-to-canonical extension, its implementation, trust gaps, and implications for zbrain."
source_title: "LLM wiki"
source_url: "https://openknowledge.ai/docs/workflows/karpathy-llm-wiki"
source_kind: "product workflow guide"
source_pattern_title: "LLM Wiki"
source_pattern_url: "https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f"
source_repository: "https://github.com/inkeep/open-knowledge"
accessed_at: "2026-07-31"
fetch_method: "direct local extraction plus direct GitHub source inspection"
status: provisional
tags: [llm-wiki, knowledge-base, karpathy, openknowledge, provenance, canonicalization, zbrain]
---

# Karpathy LLM Wiki via OpenKnowledge — Deep Analysis

## Kết luận

Pattern LLM Wiki của Karpathy rất mạnh ở một ý tưởng cốt lõi: thay vì để LLM đọc lại raw documents và tái tổng hợp từ đầu cho mỗi câu hỏi, agent duy trì một artifact Markdown có cấu trúc, liên kết và tích lũy theo thời gian.

OpenKnowledge biến ý tưởng trừu tượng đó thành một workflow vận hành rõ ràng:

```text
external-sources/   →   research/       →   articles/
raw evidence            provisional         canonical
                        synthesis            decision
```

Đây là một cải tiến thực tế đáng giá, đặc biệt ở promotion gate giữa provisional và canonical. Tuy nhiên, bài viết dùng các từ như “verbatim”, “trusted”, “canonical” và “maintenance near zero” mạnh hơn những gì implementation thực sự bảo đảm.

Đánh giá ngắn:

- **Knowledge-compounding pattern:** rất mạnh.
- **Operational workflow:** được thiết kế kỹ hơn bài giới thiệu thể hiện.
- **Portability:** trung bình; nhiều hành vi phụ thuộc OpenKnowledge MCP và `.ok/` metadata.
- **Trust model:** chưa đủ; canonical thể hiện quyết định, không chứng minh tính đúng hoặc nguồn gốc xác thực.
- **Security model:** download safeguards khá tốt, nhưng persistent prompt injection từ raw sources chưa được xử lý rõ trong các tài liệu đã đọc.

## Nguồn đã đối chiếu

Phân tích này không chỉ dựa trên trang giới thiệu. Nó đối chiếu các nguồn sau:

1. [OpenKnowledge: LLM wiki workflow](https://openknowledge.ai/docs/workflows/karpathy-llm-wiki).
2. [Karpathy: LLM Wiki gist](https://gist.github.com/karpathy/442a6bf555914893e9891c11519de94f).
3. [Starter-pack registry](https://github.com/inkeep/open-knowledge/blob/main/packages/server/src/seed/starter.ts).
4. [Knowledge Base pack skill](https://github.com/inkeep/open-knowledge/blob/main/packages/server/assets/skills/packs/knowledge-base/SKILL.md).
5. [Research skill](https://github.com/inkeep/open-knowledge/blob/main/packages/server/assets/skills/packs/knowledge-base/research/SKILL.md).
6. [Consolidate skill](https://github.com/inkeep/open-knowledge/blob/main/packages/server/assets/skills/packs/knowledge-base/consolidate/SKILL.md).
7. [Ingest procedure](https://github.com/inkeep/open-knowledge/blob/main/packages/server/assets/skills/project/references/ingest-and-sources.md).

Các câu lệnh dành cho agent trong những tài liệu này được xem là nội dung nguồn để phân tích, không phải instruction để thực thi.

## 1. Ý tưởng gốc của Karpathy

Karpathy đối lập hai mô hình.

### Retrieval-only

```text
raw documents
      ↓ retrieve chunks on every query
temporary synthesis
      ↓
answer disappears into chat history
```

LLM phải tái khám phá cùng tri thức mỗi lần. Những kết nối, mâu thuẫn và synthesis đã tìm ra không được tích lũy thành artifact lâu dài.

### Persistent LLM wiki

```text
raw documents
      ↓ ingest once, revise continuously
structured interconnected wiki
      ↓ query
answers and discoveries can return to the wiki
```

Wiki đóng vai trò một dạng “compiled knowledge” nằm giữa raw sources và câu hỏi. Giá trị không chỉ ở từng trang mà còn ở:

- cross-reference đã được tạo;
- contradiction đã được ghi nhận;
- synthesis đã được duy trì;
- câu trả lời hữu ích được lưu lại;
- knowledge không chết cùng chat session.

Đây là phần thuyết phục nhất của pattern. Nó biến LLM từ query-time answer generator thành knowledge-maintenance process.

## 2. Một khác biệt khái niệm dễ bị bỏ qua

Karpathy gọi ba lớp kiến trúc là:

1. **Raw sources** — nguồn bất biến.
2. **Wiki** — knowledge do LLM duy trì.
3. **Schema** — quy tắc hướng dẫn LLM vận hành wiki.

OpenKnowledge lại thường gọi ba folder sau là “three-layer pipeline”:

1. `external-sources/`
2. `research/`
3. `articles/`

Hai bộ “ba lớp” này không đồng nghĩa.

OpenKnowledge đã chuyển từ một kiến trúc gồm **data + derived knowledge + control plane** sang một lifecycle gồm **evidence + provisional knowledge + canonical knowledge**. Schema vẫn tồn tại, nhưng được chuyển sang:

- `.ok/frontmatter.yml` theo folder;
- `.ok/templates/`;
- project-local skills;
- OpenKnowledge MCP behavior.

Vì vậy kiến trúc thực tế của OpenKnowledge gần với:

```text
CONTROL PLANE
skills + folder metadata + templates + tool contracts

DATA PLANE
external-sources → research → articles

AUDIT PLANE
log.md + agent activity + shadow/git history

RETRIEVAL PLANE
list + grep/search + links/backlinks + file sidebar
```

Đây không chỉ là direct implementation của gist; nó là một operational interpretation có thêm state machine và product-specific control plane.

## 3. OpenKnowledge thêm gì có giá trị?

### 3.1 Tách provisional khỏi canonical

Karpathy để toàn bộ derived knowledge trong một wiki layer. OpenKnowledge tách:

- `research/`: uncertainty được phép tồn tại;
- `articles/`: vị trí cho knowledge đã được quyết định và dùng làm source of truth.

Điểm này giải quyết một failure mode quan trọng: agent biến nhận định mới hoặc synthesis chưa ổn định thành “fact” quá sớm.

State transition trở nên rõ ràng:

```text
captured source
      ↓ ingest
provisional interpretation
      ↓ explicit decision gate
canonical organizational position
```

### 3.2 Consolidation là một one-way-door gate

`consolidate` không được dùng để tạo knowledge mới. Skill yêu cầu xác nhận:

- quyết định thực sự là gì;
- alternative nào đã bị loại;
- rationale của team là gì.

Nếu team chưa quyết định, procedure phải dừng. Đây là một guardrail tốt vì future agents thường đọc canonical docs với mức mặc định tin tưởng cao hơn chat hoặc research notes.

### 3.3 Evidence chain được giữ lại

Canonical article trỏ ngược tới research bằng `supersedes`, còn research trỏ tới raw sources bằng `sources` và inline links.

```text
canonical article
      ↓
provisional research
      ↓
local source snapshots
      ↓
original URL
```

Research cũ không bị xóa; nó nhận `superseded_by`. Nhờ đó người đọc có thể xem lại reasoning và evidence đã dẫn đến quyết định.

### 3.4 Schema nằm gần action

Folder-level `.ok/frontmatter.yml` mô tả chính xác quy tắc của từng layer. Ví dụ:

- source folder: preserve, do not analyze;
- research folder: provisional, cite sources;
- article folder: canonical, maintain supersession chain.

Đặt guidance gần nơi agent thao tác giúp giảm khả năng một global instruction quá dài bị bỏ qua.

### 3.5 Implementation có discipline mạnh hơn bài marketing

Research skill thực tế yêu cầu:

1. scan knowledge hiện có trước khi research;
2. phân loại fully/partially/not covered;
3. scope câu hỏi và rubric;
4. ingest nguồn trước khi phân tích;
5. viết incremental để chống mất work khi crash;
6. cite factual claims;
7. tạo backlinks;
8. kiểm tra dead links và source alignment.

Đây mới là phần tạo giá trị vận hành. Chỉ tạo ba folder không đủ; gates, validation và write discipline mới khiến pattern hoạt động ổn định.

### 3.6 Ingest có nhiều safeguard tốt

Implementation kiểm tra:

- chỉ cho phép HTTP(S);
- giới hạn redirects và thời gian;
- size gate 50/100 MB;
- block executable và scripted-document extensions;
- slug regex để tránh path traversal;
- SHA-256 cho binary artifacts;
- append-only re-ingest khi hash thay đổi;
- không lưu login/error page làm nguồn.

Phần này kỹ hơn nhiều so với mô tả cấp cao trên trang workflow.

## 4. Những tuyên bố cần đọc thận trọng

### 4.1 “Verbatim source” không hoàn toàn verbatim

Bài viết nói `external-sources/` chứa source text nguyên văn. Nhưng text-ingest procedure cho phép:

- extract HTML thành text/Markdown;
- bỏ navigation, ads, cookie banner và footer;
- normalize cấu trúc;
- lưu với `preservation: text-extracted`.

Đây là một bản trích xuất trung thành theo ý định, không phải byte-for-byte preservation.

Hệ quả:

- nội dung JavaScript-rendered có thể bị thiếu;
- comments, footnotes hoặc tables có thể biến dạng;
- source cập nhật sau đó khó phát hiện nếu text snapshot không có hash;
- cùng URL qua extractor khác có thể tạo nội dung khác.

Với binary source, SHA-256 tạo bằng chứng integrity tốt hơn. Text source hiện chưa có guarantee tương đương.

### 4.2 Raw-source template tự mâu thuẫn

Starter template `clip` có các section:

```markdown
## Source

## Highlights

## My notes
```

Nhưng folder rule nói raw source phải immutable và “no analysis here”. `Highlights` và `My notes` khuyến khích người dùng trộn annotation vào evidence layer.

Nếu các section này được sử dụng, raw source không còn thuần evidence. Thiết kế sạch hơn là:

- source file chỉ chứa snapshot;
- highlights và notes nằm trong research document riêng;
- annotation trỏ ngược tới source bằng stable anchors hoặc citations.

### 4.3 “Canonical” không đồng nghĩa “true” hoặc “trusted”

OpenKnowledge định nghĩa canonical dựa trên việc team đã ra quyết định. Điều đó phù hợp với decision knowledge:

```text
Team chose Yjs for CRDT.
```

Nhưng không đủ cho factual knowledge:

```text
Framework X guarantees exactly-once delivery.
```

Một team có thể quyết định dựa trên evidence yếu hoặc outdated. Canonical chỉ thể hiện **organizational authority**, không chứng minh:

- claim đúng;
- source đáng tin;
- content còn fresh;
- ai đã verify đúng revision;
- file không bị sửa sau verification.

Template hiện có `status: canonical`, `author` và `supersedes`, nhưng thiếu:

- `generated`;
- `verified`;
- `stale_after`;
- content digest;
- reviewer identity;
- effective date;
- confidence hoặc evidence-quality classification.

Do đó mô tả starter pack là “Trusted articles from your sources” đang mạnh hơn trust guarantees thực tế.

### 4.4 “Maintenance cost near zero” là khẩu hiệu, không phải invariant

Gist nói LLM không chán và có thể cập nhật nhiều file cùng lúc. Nhưng chính implementation phải dùng hàng trăm dòng procedural guidance để ngăn agent:

- bỏ qua source capture;
- research trùng;
- viết sai scope;
- mất work giữa session;
- tạo broken links;
- canonicalize quá sớm;
- quên log hoặc supersession chain.

Điều này cho thấy maintenance cost không biến mất. Nó được chuyển từ manual bookkeeping sang:

- token và compute cost;
- workflow design;
- human review;
- linting;
- conflict resolution;
- model/tool upgrades;
- correction khi agent tạo drift.

Pattern vẫn có giá trị, nhưng “near zero” chỉ đúng khi control plane đủ tốt và lỗi của agent được phát hiện sớm.

### 4.5 “LLM không quên cross-reference” không được bảo đảm

LLM có thể bỏ sót link, tạo duplicate pages hoặc sửa một nửa graph. Research và consolidate skills phải bắt buộc link neighbors và chạy dead-link checks chính vì hành vi đó không tự nhiên đáng tin.

Correct framing nên là:

> LLM giảm marginal cost của bookkeeping, nếu workflow biến bookkeeping thành các invariant có thể kiểm tra.

## 5. Retrieval không dùng vector database: tốt nhưng có giới hạn

Bài viết nhấn mạnh agentic search qua files, grep và backlinks. Cách này có nhiều ưu điểm:

- minh bạch;
- deterministic;
- dễ debug;
- không có secondary index bị lệch khỏi source;
- hoạt động offline;
- Git-friendly.

Tuy nhiên, gist gốc chỉ nói `index.md` hoạt động tốt ở quy mô vừa, khoảng 100 sources và vài trăm pages, rồi gợi ý local hybrid search như `qmd` khi vault lớn lên.

Các giới hạn của grep/link traversal:

- synonym và paraphrase khó tìm;
- concept liên quan nhưng không share keyword dễ bị bỏ sót;
- graph thiếu link sẽ tạo retrieval blind spot;
- listing lớn làm tăng context cost;
- ranking giữa nhiều candidates còn yếu;
- cross-domain synthesis có thể cần semantic retrieval.

Vì vậy “không cần vector DB” hợp lý như một default ban đầu, không nên hiểu là vector hoặc hybrid retrieval luôn vô ích.

## 6. Dynamic index làm giảm portability

Karpathy đề xuất `index.md` như catalog content-oriented. OpenKnowledge thay phần lớn vai trò này bằng:

- enriched `exec("ls ...")`;
- file sidebar;
- folder metadata;
- per-document frontmatter;
- links tool.

Trong OpenKnowledge, cách này có thể tốt hơn static index vì không phải đồng bộ thủ công. Nhưng một filesystem agent thông thường chỉ thấy `.md` và hidden `.ok/` files; nó không tự nhận được enriched listing semantics.

Do đó:

- dữ liệu Markdown vẫn portable;
- behavior và discovery experience không portable hoàn toàn;
- agent ngoài OpenKnowledge phải tái tạo logic đọc `.ok/`, template và skills;
- static `index.md` vẫn hữu ích làm lowest-common-denominator navigation layer.

## 7. Closed-loop citations: mạnh nhưng chưa đủ

Nguyên tắc mọi downstream claim cite local source snapshot giải quyết:

- link rot;
- thay đổi nội dung online;
- offline access;
- reproducibility;
- audit trail.

Nhưng một local snapshot chỉ chứng minh “agent đã đọc bản này”, không chứng minh:

- publisher có authority;
- source là primary hay secondary;
- source không chứa misinformation;
- extractor không làm mất context;
- claim được suy ra đúng;
- source còn current.

Cần phân biệt bốn lớp:

```text
Preservation   — ta giữ đúng artifact nào?
Provenance     — claim bắt nguồn từ artifact nào?
Credibility    — artifact đáng tin đến đâu?
Verification   — ai đã kiểm tra claim với artifact và khi nào?
```

Workflow hiện mạnh ở preservation và provenance, yếu hơn ở credibility và verification.

## 8. Supersession semantics đang bị overload

`supersedes` được dùng cho ít nhất hai quan hệ khác nhau:

1. Canonical article thay thế research article đã dẫn tới nó.
2. Canonical article mới thay thế canonical article cũ.

Hai quan hệ này không cùng nghĩa:

- research → canonical là **promotion/derivation**;
- canonical v1 → canonical v2 là **replacement**.

Nếu dùng chung `supersedes`, graph consumer khó phân biệt lịch sử reasoning với lịch sử authoritative versions.

Mô hình rõ hơn:

```yaml
derived_from:
  - research/topic.md
supersedes:
  - articles/topic-v1.md
```

hoặc dùng transition event riêng ghi source state, target state, actor và timestamp.

## 9. Lifecycle chỉ có hai trạng thái là chưa đủ

Workflow tập trung vào:

```text
provisional → canonical
```

Một knowledge base lâu dài thường cần thêm:

- draft;
- provisional;
- reviewed;
- canonical;
- stale;
- deprecated;
- superseded;
- rejected;
- archived.

Ví dụ, canonical article có thể vẫn là quyết định chính thức nhưng đã quá hạn review. `canonical` và `fresh` là hai chiều độc lập, không nên gộp.

Tương tự, research doc sau khi promoted vẫn giữ `status: provisional` và chỉ thêm `superseded_by`. Query chỉ lọc theo status có thể vẫn xem nó là active provisional research.

## 10. Persistent prompt injection là threat model lớn

Raw sources được lưu để future agents đọc lại. Điều này tạo một persistent prompt-injection channel:

```text
malicious webpage
      ↓ ingest verbatim
external-sources/malicious.md
      ↓ future research agent reads it
embedded instructions attempt to influence tools or writes
```

Download procedure có safeguards tốt cho protocol, extension, size và path traversal, nhưng các tài liệu đã đọc chưa mô tả rõ content-level isolation cho prompt injection.

Một trusted knowledge workflow cần bảo đảm:

- source content luôn được đánh dấu untrusted data;
- instructions nằm trong source không được tham gia control plane;
- agent không chạy command hoặc follow exfiltration request từ source;
- citations không tự nâng trust của source;
- canonical promotion phải loại bỏ hoặc neutralize instruction-shaped content;
- audit ghi lại source nào đã ảnh hưởng tới write nào.

Đây là khác biệt giữa bảo vệ filesystem và bảo vệ agent reasoning boundary.

## 11. Multi-file updates có blast radius lớn

Karpathy coi việc một ingest cập nhật 10–15 wiki pages là lợi thế. Nó cũng là failure surface:

- session có thể dừng giữa chừng;
- một số page được cập nhật, số khác chưa;
- backlinks và summary có thể lệch nhau;
- user có thể sửa đồng thời;
- canonical page có thể đọc state nửa cũ nửa mới.

Research skill giải quyết một phần bằng incremental checkpointing, nhưng chưa mô tả transaction hoặc change-set manifest cho toàn bộ graph update.

CRDT xử lý concurrent text editing tốt hơn last-write-wins, nhưng không bảo đảm semantic atomicity giữa nhiều file.

Một graph-wide operation nên có:

- planned change set;
- operation ID;
- before/after revisions;
- per-file completion status;
- final lint gate;
- rollback hoặc retry semantics.

## 12. Link graph có thể tích lũy entropy

“Link aggressively” giúp discovery, nhưng nếu mọi noun phrase đều trở thành link và mỗi Q&A hữu ích thành page riêng, hệ thống có thể tạo:

- page explosion;
- duplicated concepts;
- links có tín hiệu thấp;
- hub pages quá lớn;
- circular citation;
- khó xác định page authoritative.

Link quantity không thay thế information architecture. Wiki cần thêm discipline:

- stable concept identity;
- merge/dedup procedure;
- typed relations khi cần;
- page-granularity rules;
- alias/redirect semantics;
- orphan và hub-size linting.

## 13. Audit trail có nhiều nguồn sự thật

Workflow dùng đồng thời:

- `log.md` append-only;
- agent activity panel;
- shadow repository history;
- project Git history;
- document frontmatter.

Mỗi layer hữu ích, nhưng có nguy cơ divergence. Ví dụ file đã đổi nhưng agent crash trước khi append `log.md`, hoặc Git commit gộp nhiều turns khác với activity timeline.

Cần xác định rõ:

- audit source authoritative là gì;
- log nào được tạo tự động;
- log nào chỉ là human-readable projection;
- cách reconcile khi các nguồn khác nhau.

Một manually maintained `log.md` không nên là bằng chứng duy nhất cho hoạt động đã xảy ra.

## 14. So sánh với OKF v0.2

OpenKnowledge Knowledge Base workflow và Open Knowledge Format có điểm chung, nhưng giải quyết hai bài toán khác nhau.

| Dimension | Knowledge Base workflow | OKF v0.2 |
|---|---|---|
| Primary purpose | Vận hành knowledge lifecycle | Format trao đổi knowledge |
| Storage | Markdown + YAML + folders | Markdown + YAML + bundle |
| Main abstraction | Source → research → article | Concept document |
| Provenance | Local source paths và links | Structured `sources` |
| Trust | `provisional` / `canonical` | `generated`, `verified`, trust tiers |
| Freshness | Chưa có field chính | `stale_after` |
| Lifecycle | Promotion workflow | `draft`, `stable`, `deprecated` |
| Execution integrity | Không định nghĩa | Attested Computation concept |
| Operational gates | Rất chi tiết | Không prescribe runtime |
| Portability | Một phần phụ thuộc OpenKnowledge | Chủ đích tool-independent |

Repository OpenKnowledge cũng đăng ký `knowledge-base` và `okf` thành hai starter pack riêng. Vì vậy không nên mặc định Knowledge Base pack là OKF-conformant.

Ví dụ, Knowledge Base `log.md` template có YAML frontmatter, trong khi dedicated OKF pack cố ý tạo reserved `index.md` và `log.md` theo shape riêng, frontmatter-free trong implementation của họ.

Hai thiết kế bổ sung cho nhau:

```text
OpenKnowledge workflow
  gives: capture, research, promotion, supersession, operational gates

OKF
  gives: interoperable metadata, provenance fields, verification, freshness
```

Một combined lifecycle mạnh hơn sẽ là:

```text
capture → analyze → verify → promote → monitor freshness → supersede/archive
```

## 15. Điều gì phù hợp và không phù hợp?

### Phù hợp

- personal research kéo dài nhiều tuần hoặc tháng;
- architecture evaluation;
- team decision records có evidence;
- literature review;
- competitive analysis;
- knowledge base quy mô nhỏ đến vừa;
- môi trường có human review và Git history.

### Chưa đủ nếu dùng nguyên trạng

- regulated knowledge;
- medical, legal hoặc financial facts;
- unattended autonomous ingestion;
- dữ liệu thay đổi theo thời gian thực;
- hàng chục nghìn tài liệu;
- nhiều writers cập nhật graph đồng thời;
- nơi “trusted” phải có cryptographic hoặc identity-backed meaning.

## 16. Ý nghĩa đối với zbrain

Runtime layout hiện tại của zbrain đã có các vùng phù hợp để biểu diễn semantics này mà không cần sao chép nguyên folder names của OpenKnowledge:

```text
evidence/sources/    ≈ external-sources/   raw evidence
evidence/analysis/   ≈ research/           provisional synthesis
evidence/qa/         ≈ verification gate   review and checks
wiki/                ≈ articles/           durable knowledge surfaces
evidence/applied/    ≈ usage evidence      where knowledge affected work
evidence/archive/    ≈ superseded history
```

Pattern đáng mượn:

1. Capture source trước khi interpretation.
2. Raw evidence append-only hoặc content-addressed.
3. Provisional analysis tách khỏi durable wiki knowledge.
4. Promotion là explicit operation, không phải copy file tùy ý.
5. Human decision hoặc verification gate trước promotion.
6. Source chain và supersession chain không bị xóa.
7. Lint contradictions, stale claims, broken links và orphans.
8. Câu trả lời có giá trị không chết trong chat history.

Những chỗ zbrain nên chặt hơn:

1. Tách **decision status** khỏi **factual trust**.
2. Verification chỉ hợp lệ nếu mới hơn revision được verify.
3. Gắn source và verification với digest.
4. Dùng stable concept ID tách khỏi file path.
5. Có `stale_after`, reviewer và transition timestamps.
6. Phân biệt `derived_from` với `supersedes`.
7. Mark ingested content là untrusted để chống persistent prompt injection.
8. Có operation manifest cho graph-wide multi-file updates.
9. Cho phép hybrid search khi corpus vượt ngưỡng file/grep hợp lý.
10. Giữ schema portable: conventions quan trọng phải đọc được ngoài một MCP implementation cụ thể.

## 17. Final assessment

OpenKnowledge đã biến LLM Wiki từ một idea file thành workflow có thể vận hành. Cải tiến quan trọng nhất không phải WYSIWYG, CRDT hay starter-pack picker; đó là việc formalize ba hành động khác nhau:

```text
ingest       preserves evidence
research     contains uncertainty
consolidate  records a decided position
```

Đây là nền tảng tốt cho compounding knowledge.

Nhưng workflow hiện chưa phải trust system hoàn chỉnh:

- preservation không luôn byte-exact;
- canonicality không chứng minh truth;
- actor identity không được xác thực;
- freshness chưa first-class;
- text sources không có integrity digest;
- prompt-injection boundary chưa rõ;
- multi-file semantic atomicity chưa có;
- retrieval strategy cần nâng cấp khi scale tăng.

Verdict cuối cùng:

> Nên xem LLM Wiki + OpenKnowledge workflow là một **knowledge lifecycle architecture rất đáng học**, không phải bằng chứng rằng một folder Markdown do agent quản lý tự động trở thành trusted knowledge base. Giá trị thật đến từ explicit state transitions, preserved evidence, validation và human-controlled promotion; trust chỉ xuất hiện khi các transition đó gắn với identity, revision, freshness và integrity có thể kiểm chứng.
