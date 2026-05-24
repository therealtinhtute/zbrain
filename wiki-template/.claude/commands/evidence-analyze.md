# Evidence Analyze

Chạy critical-analysis prompts trên 1 evidence set. CORE prompts chạy mặc định — đủ để mở Q&A loop. ON-DEMAND prompts chạy qua `--prompt`.

Pipeline branches theo `source.yaml#source_type`:

| `source_type`             | CORE set                            | Reference                                                                          |
|---------------------------|-------------------------------------|------------------------------------------------------------------------------------|
| `paste` / `api` / `mcp`   | `[01, 02, 04, 08]`                  | [critical-analysis-prompts.md](../../agents/pipeline/critical-analysis-prompts.md) |
| `code`                    | `[c01, c02, c03, c04, 04, 08]`      | [code-analysis-prompts.md](../../agents/pipeline/code-analysis-prompts.md)         |

> Step 2 trong pipeline: ingest → **analyze** → qa → apply.
> Reference: [evidence-state-rules.md](../../agents/pipeline/evidence-state-rules.md). CORE 4 (`04-questions.md`) và CORE 8 (`08-knowledge-gaps.md`) DÙNG CHUNG filename + output schema ở cả hai pipeline.

---

## Input

| Arg          | Required | Notes                                                                |
|--------------|----------|----------------------------------------------------------------------|
| `--id`       | optional | Evid-id. Mặc định: latest entry state=`ingested` trong `_index.md`.  |
| `--prompt`   | optional | Chạy thêm ON-DEMAND prompt. Text pipeline: `03` \| `05` \| `06` \| `07` \| `09` \| `10`. Code pipeline: `c05` \| `c06` \| `c07`. Có thể repeat. |
| `--persona`  | optional | Cho prompt 03 (expert briefing). Default: `engineering lead`.        |
| `--tone`     | optional | Cho prompt 10 (final report). `academic|professional|conversational`. Default: professional. |

---

## Bước 0 — Workspace check

1. Tìm `.claude/wiki.json`: từ `<cwd>` đi lên parent cho tới khi gặp file. Lưu `wiki_json_dir`.
2. Đọc file → `workspace` + `wiki_root` resolve theo [agents/system-prompt.md `wiki_root` Resolution Rule](../../agents/system-prompt.md):
   - Absolute path → dùng nguyên
   - Relative (`"."`, `"./..."`) → resolve relative TỚI `project_root` (= parent của `.claude/`)
   - `null`/empty → fallback `~/.claude/wiki-global.json#wiki_root`
3. STOP nếu file thiếu hoặc `.workspace` rỗng.
4. Set `{ws} = {effective_wiki_root}/workspaces/{workspace}/`.

## Bước 1 — Resolve evidence

1. Nếu không có `--id`:
   - Đọc `{ws}/evidence/_index.md`, lấy entry mới nhất state=`ingested`.
   - Nếu không có → STOP, hướng dẫn `/evidence-ingest`.
2. Đọc `{ws}/evidence/sources/{evid-id}/source.yaml`.
3. **Workspace lock check (I-2)**: `source.yaml#workspace_at_ingest` PHẢI khớp `{active}`. Nếu không → STOP với error format trong `evidence-state-rules.md`.
4. State PHẢI ∈ {`ingested`, `analyzed`}. Nếu `analyzed` và không có `--prompt` → in warning "đã analyze rồi, dùng --prompt để rerun ON-DEMAND" → STOP.

## Bước 2 — Decide prompts to run

Branch theo `source.yaml#source_type`:

### Khi `source_type ∈ {paste, api, mcp}` (text pipeline)
- Nếu state = `ingested` → CORE set: `[01, 02, 04, 08]` từ [critical-analysis-prompts.md](../../agents/pipeline/critical-analysis-prompts.md). Plus `--prompt` flags.
- Nếu state = `analyzed` (rerun) → chỉ ON-DEMAND `--prompt` flags.
- Validate `--prompt` values ∈ {03, 05, 06, 07, 09, 10}. Reject unknown.

### Khi `source_type = code` (code pipeline)
- Nếu state = `ingested` → CORE set: `[c01, c02, c03, c04, 04, 08]` từ [code-analysis-prompts.md](../../agents/pipeline/code-analysis-prompts.md).
  - `c01`–`c04` là code-specific (tech stack, service map, pattern proposals, contract proposals).
  - `04` (questions) và `08` (gaps) DÙNG CHUNG filename với text pipeline; nội dung khác (xem code-analysis-prompts.md "CORE 4 — Question Generator (shared)" và "CORE-CODE 8").
- Nếu state = `analyzed` (rerun) → chỉ ON-DEMAND `--prompt` flags.
- Validate `--prompt` values ∈ {c05, c06, c07}. Reject unknown (text-pipeline prompts như `03` KHÔNG hợp lệ cho code source).

## Bước 3 — Load contexts

Đọc song song để feed các prompt:

**Always**:
- `sources/{id}/raw.normalized.md` (nếu có) HOẶC `raw.{ext}` (nếu là markdown/text)
- `sources/{id}/source.yaml`

**For prompts 02, 08** (so sánh với wiki):
- `{ws}/patterns-index.md`
- `{ws}/platform/contracts/*.md` (tất cả file)
- `{ws}/platform/patterns/*.md` (tất cả file)
- `{ws}/projects/*/services/*.md` lọc theo `source.yaml#related_projects`/`related_files` (nếu rỗng → load top-3 service docs khớp keyword trong raw)
- `{ws}/domains/*/workflow.md` nếu `source.yaml#related_domains` non-empty

**For prompt 04**: cộng thêm `01-resource-upload.md` + `02-contradiction.md` + `08-knowledge-gaps.md` (nếu đã sinh ở step 4 dưới).

**For prompt 10**: cộng thêm `qa/{id}/verified-facts.md` (nếu đã có).

## Bước 4 — Run prompts (sequential, có dependency)

Tạo folder `{ws}/evidence/analysis/{evid-id}/` nếu chưa có.

### Text pipeline (`source_type ∈ {paste, api, mcp}`) — Run order
1. **01-resource-upload.md** — chạy đầu tiên, không depend gì.
2. **02-contradiction.md** — depend on 01 + wiki context.
3. **08-knowledge-gaps.md** — depend on 01 + 02 + wiki.
4. **04-questions.md** — depend on 01 + 02 + 08.
5. **ON-DEMAND** (theo thứ tự `--prompt` flags):
   - 03 → depend on 01, 02, raw
   - 05 → depend on 01, raw
   - 06 → depend on 01, raw
   - 07 → depend on 01, 02, 09 (nếu có)
   - 09 → depend on 01, 02, 04
   - 10 → depend on **all** files có trong `analysis/{id}/` + `verified-facts.md` (nếu có)

### Code pipeline (`source_type = code`) — Run order
1. **c01-tech-stack.md** — depend on raw.md Section 1, 2.
2. **c02-service-map.md** — depend on raw.md Section 4, 5, 6, 7 + wiki services context.
3. **c03-pattern-proposals.md** — depend on c01, c02, wiki patterns context.
4. **c04-contract-proposals.md** — depend on c02, raw.md Section 4/5/7, wiki contracts context.
5. **08-knowledge-gaps.md** (code variant) — depend on c01–c04 + wiki context.
6. **04-questions.md** (shared, code-mode prompt) — depend on c01–c04 + 08.
7. **ON-DEMAND** (theo thứ tự `--prompt` flags):
   - c05 (service drafts) → depend on c02 + raw, dùng `templates/service.md`
   - c06 (decision drafts) → depend on raw Section 9 + Section 2/6, dùng `templates/adr.md`
   - c07 (config overrides) → depend on raw Section 3 + c03 + wiki patterns Default Config tables

Với mỗi prompt:
- Build prompt theo template trong [critical-analysis-prompts.md](../../agents/pipeline/critical-analysis-prompts.md).
- Output PHẢI tuân schema markdown nêu trong reference.
- Mọi claim phải có citation (`raw.normalized.md#section-N` hoặc `{ws}/path`).
- Nếu output thiếu citation → re-prompt 1 lần với reminder "missing citations". Sau đó vẫn thiếu → ghi với marker `[NO-CITE]` và warn user.

## Bước 5 — Validate (V-04)

Check mỗi file CORE vừa ghi:
- Non-empty
- Có ít nhất 1 entry với citation
- Schema match (heading hierarchy đúng)

Nếu fail → STOP, không transition state. Báo file nào fail.

## Bước 6 — Update state

Transition `ingested → analyzed` chỉ khi **đủ CORE set** cho `source_type` đó:
- Text pipeline: 4 files (`01`, `02`, `04`, `08`).
- Code pipeline: 6 files (`c01`, `c02`, `c03`, `c04`, `04`, `08`).

Nếu pass → Update `{ws}/evidence/_index.md` row tương ứng: state → `analyzed`, last_updated = today.

Nếu state đang `analyzed` và chỉ rerun ON-DEMAND: KHÔNG đổi state, chỉ update `last_updated`.

## Bước 7 — Confirm

In:
```
✅ Analysis complete — {evid-id}
   Workspace : {active}
   CORE      : 01-resource-upload, 02-contradiction, 04-questions, 08-knowledge-gaps
   On-demand : {list nếu có}
   State     : analyzed

Highlights:
  - {N} contradictions found ({M} vs wiki, {K} internal)
  - {N} questions generated (P0={x}, P1={y}, P2={z}, P3={w})
  - {N} blocking gaps identified

Next:
  /evidence-qa --id {evid-id}    → start Q&A loop with user
```

---

## Khi nào on-demand

| Prompt | Khi nào dùng                                                              |
|--------|---------------------------------------------------------------------------|
| 03     | Cần brief cho stakeholder/lead về evidence này                            |
| 05     | Có nhiều claim mạnh, cần grade trước khi hỏi user                         |
| 06     | Domain có yếu tố lịch sử (incident, evolution)                            |
| 07     | Trước khi present kết quả ra ngoài, muốn anticipate phản biện             |
| 09     | Sau khi qa_done, muốn extract insight không hiển nhiên                    |
| 10     | Sinh báo cáo cuối cho stakeholder hoặc PR description                     |

---

## Common errors

| Error                          | Fix                                                                  |
|--------------------------------|----------------------------------------------------------------------|
| State ≠ ingested/analyzed      | Check `_index.md`; rerun previous step                               |
| Workspace lock fail (I-2)      | `/switch-workspace {workspace_at_ingest}`                            |
| Output missing citations       | Manual edit, add citation, then `/evidence-qa` continues             |
| Wiki context too large         | `source.yaml#related_files` quá rộng — narrow lại và rerun           |
