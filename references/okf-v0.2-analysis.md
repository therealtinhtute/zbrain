---
title: "Open Knowledge Format v0.2 — Analysis"
description: "Analysis of OKF v0.2 as a Markdown/Git-native knowledge exchange and trust format."
source_title: "Open Knowledge Format (OKF)"
source_url: "https://raw.githubusercontent.com/GoogleCloudPlatform/knowledge-catalog/refs/heads/main/okf/SPEC.md"
source_kind: "technical specification"
source_repository: "GoogleCloudPlatform/knowledge-catalog"
source_version: "0.2"
accessed_at: "2026-07-31"
fetch_method: "direct raw GitHub"
status: provisional
tags: [okf, knowledge-format, provenance, trust, attestation]
---

# Open Knowledge Format v0.2 — Analysis

## Kết luận

OKF v0.2 là một nền tảng rất thực dụng cho việc trao đổi knowledge bằng Markdown/Git, nhưng phần “trusted knowledge” và “attested computation” mới dừng ở contract thiết kế, chưa đủ chặt để tạo niềm tin có thể kiểm chứng trong production.

Nguồn được đọc đầy đủ qua raw GitHub: 1.003 dòng, 37.544 byte; không có lỗi trích xuất hoặc dấu hiệu prompt injection.

## 1. OKF định nghĩa gì?

OKF coi một kho tri thức là một cây thư mục:

```text
bundle/
  index.md
  log.md
  tables/
    customers.md
  metrics/
    revenue.md
```

Mỗi concept là một file Markdown có YAML frontmatter:

```yaml
---
type: Metric
title: Revenue
description: Recognized revenue
resource: ...
tags: [finance]
---
```

Sau frontmatter là nội dung Markdown tự do.

| Thành phần | Cách OKF biểu diễn |
|---|---|
| Đơn vị phân phối | Knowledge Bundle |
| Đơn vị tri thức | Một file Markdown |
| Định danh concept | Đường dẫn file bỏ `.md` |
| Metadata bắt buộc | Chỉ có `type` |
| Quan hệ | Markdown link |
| Provenance | `sources` |
| Người/máy tạo | `generated` |
| Xác minh | `verified` |
| Lifecycle | `status`, `stale_after` |
| Tính toán chuẩn | `Attested Computation` |

Triết lý xuyên suốt là tối thiểu hóa quy chuẩn để tăng khả năng đọc, diff, trao đổi và tồn tại lâu dài.

## 2. Phần thiết kế tốt nhất

### Cực kỳ đơn giản và bền

Không cần database, schema registry hay SDK riêng. Người đọc được file, agent parse được YAML, Git quản lý history và diff.

Đây là lựa chọn tốt cho knowledge cần sống lâu hơn một vendor hay runtime cụ thể.

### Phân tách đúng các khái niệm trust

Spec phân biệt rõ:

- `generated`: ai tạo hoặc sửa nội dung.
- `verified`: ai kiểm tra nội dung.
- `sources`: nội dung được suy ra từ đâu.
- `stale_after`: khi nào nội dung hết hạn.
- Attestation: một lần chạy cụ thể có thực hiện đúng computation được duyệt hay không.

Đặc biệt, spec phân biệt verification của **định nghĩa** với attestation của **một lần thực thi**. Đây là mô hình đúng và quan trọng đối với agent-generated knowledge.

### Không lưu trust score chủ quan

OKF lưu các tín hiệu như:

- `author`
- `usage_count`
- `last_modified`
- human/process verification

Consumer tự suy ra mức tin cậy. Cách này tốt hơn lưu một con số như `trust_score: 0.87`, vốn khó giải thích và nhanh lỗi thời.

### Per-claim provenance hợp lý

Một claim dùng Markdown footnote, còn footnote ID nối đến `sources[].id`:

```markdown
This table is sharded daily.[^ga4-schema]
```

Việc dùng ID ổn định thay vì vị trí `sources[0]` tránh sai attribution khi agent sắp xếp lại danh sách.

### Attested Computation có mô hình tốt

Computation là concept độc lập, có:

- runtime;
- typed parameters;
- executor;
- receipt shape;
- deterministic attester.

Agent chỉ được điền parameter, không được tự viết lại query. Điều này biến câu hỏi “agent có chạy đúng phép tính đã duyệt không?” thành một kiểm tra cơ học thay vì phán đoán bằng LLM.

## 3. Những khoảng trống kỹ thuật quan trọng

### 3.1 Trust tier có lỗi về thời gian

Spec quy định chỉ cần từng có verifier `human:` thì concept được xem là **human-reviewed**.

Nhưng nó không yêu cầu:

```text
latest verified.at >= generated.at
```

Chính ví dụ trong Appendix có:

```yaml
generated.at: 2026-06-28
verified.at:  2026-06-25
```

Nội dung đã thay đổi sau khi con người xác minh, nhưng theo §5.3 nó vẫn thuộc tier `human-reviewed`.

Đây là lỗ hổng logic nghiêm trọng. Verification hợp lệ phải được vô hiệu hóa khi có meaningful content change, hoặc consumer chỉ tính verification mới hơn lần generation gần nhất.

### 3.2 Identity và trust hoàn toàn tự khai báo

Các giá trị như:

```yaml
verified: { by: human:ahormati }
generated: { by: reference_agent/gemini-2.5-pro }
```

chỉ là chuỗi. Bất kỳ producer nào cũng có thể tự ghi `human:*`.

Spec chưa có:

- chữ ký số;
- actor registry;
- artifact hash;
- signed verification event;
- content digest gắn với verification.

Vì vậy `human-reviewed` hiện chỉ có nghĩa là file tuyên bố rằng một người đã review. Nó chưa phải bằng chứng người đó thực sự review đúng phiên bản nội dung này.

### 3.3 Attestation chưa interoperable

Spec thừa nhận đang trì hoãn:

- receipt/verdict wire format;
- attester ABI;
- sandboxing;
- portability;
- caching;
- runtime protocol.

Do đó hai hệ thống cùng đọc OKF vẫn chưa chắc chạy được executor hoặc attester của nhau.

Ngoài ra, `computation` và `attester.resource` không có hash cố định. Script attester có thể bị thay đổi sau khi concept được verify, làm mất ý nghĩa của verification trước đó.

### 3.4 Semantic interoperability còn yếu

Chỉ `type` là bắt buộc, nhưng type không có registry:

```yaml
type: Metric
type: Business Metric
type: KPI
type: Finance Metric
```

Tất cả đều hợp lệ nhưng consumer không biết chúng có cùng nghĩa hay không.

Tương tự:

- link là edge không có type;
- runtime là chuỗi mở;
- parameter types không có vocabulary chuẩn;
- source resource có thể là URL, path hoặc một mô tả scope không resolvable.

Vì thế OKF mạnh ở **syntactic interoperability**, nhưng khá yếu ở **semantic interoperability**.

### 3.5 Đường dẫn vừa là identity vừa là location

Concept ID chính là path. Nếu đổi:

```text
metrics/revenue.md
→ finance/revenue.md
```

thì identity cũng đổi, mọi external reference có thể hỏng.

Ngoài ra:

- Link bắt đầu bằng `/` được hiểu là bundle-relative theo OKF, nhưng GitHub và Markdown renderer thông thường hiểu là host-root.
- Relative path cho phép `../`; spec chưa yêu cầu canonicalization hoặc ngăn path thoát khỏi bundle.
- Broken links vẫn conformant.

Consumer cần logic riêng, hơi mâu thuẫn với mục tiêu “không cần bespoke tooling”.

### 3.6 Versioning tự mâu thuẫn

§12 nói minor version chỉ thêm thay đổi backward-compatible. Nhưng §13 xác nhận v0.2 có hai breaking changes:

- `timestamp` → `generated.at`;
- `# Citations` → `sources`.

Có fallback cho consumer cũ, nhưng xét theo chính quy tắc versioning của spec thì đây vẫn không phải minor-compatible change.

### 3.7 “Markdown chuẩn” nhưng dùng extension

Per-claim attribution phụ thuộc vào Markdown footnotes. Footnotes không thuộc CommonMark cốt lõi và không phải parser nào cũng hỗ trợ giống nhau.

YAML date/datetime cũng có thể được parse thành string hoặc date object tùy YAML version và library. Consumer cần normalize rõ ràng hơn những gì spec hiện yêu cầu.

## 4. Đánh giá độ trưởng thành

| Mặt | Đánh giá |
|---|---|
| Human-readable knowledge format | Rất tốt |
| Git-native exchange | Rất tốt |
| Agent ingestion/retrieval | Tốt |
| Semantic standardization | Trung bình |
| Provenance metadata | Tốt về mô hình |
| Authenticity/integrity | Yếu |
| Trust-tier correctness | Có lỗ hổng |
| Portable attested execution | Chưa hoàn thiện |

## 5. Ý nghĩa đối với zbrain

OKF phù hợp với hướng của zbrain ở tầng **knowledge document interchange**: Markdown-first, dễ đọc, dễ version-control, không khóa vào runtime.

Nhưng nếu dùng cho trusted memory, zbrain sẽ cần một profile chặt hơn OKF cơ bản:

1. Stable concept ID tách khỏi path.
2. Chỉ tính verification khi nó mới hơn generation hiện tại.
3. Gắn verification với content digest.
4. Hash hoặc pin computation, executor và attester.
5. Vocabulary có kiểm soát cho `type`, runtime và parameter type.
6. Phân biệt rõ conformant, validated và trusted.
7. Canonicalize path và không cho resource thoát bundle.
8. Chuẩn hóa receipt/verdict trước khi hỗ trợ thực thi.

## Tổng kết

OKF v0.2 là một format trao đổi knowledge tốt và dễ adoption, với tư duy provenance/attestation rất đúng hướng. Tuy nhiên, nó chưa phải một trust system hoàn chỉnh: metadata trust hiện phần lớn vẫn là assertion, còn attested computation chưa có protocol đủ chuẩn để triển khai portable và an toàn.
