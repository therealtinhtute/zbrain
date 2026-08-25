---
id: 01M0VZ25PESKQ2MZXWWJ1QHBNM
type: plan
intake_id: 01M0VZ25PH00WH6PY06V8MGS0B
lane: normal
status: completed
created: 2026-08-25
updated: 2026-08-25
---

# zbrain — Audit nói vs làm + Metrics + Plan tối ưu (Chi tiết)

> **Status:** `active` · **Tạo:** 2026-08-25 · **Cập nhật:** 2026-08-25 · **Tác giả:** audit song song code + Exa research
> **Nguồn chân lý:** `go vet ./...`, `go test ./...`, `make build/smoke`, `trusted-memory-spec.md`, `docs/trusted-agent-gateway-spec.md`, `internal/cli/cli.go:23`, `internal/runtime/*.go`, `assets/`, `.github/workflows/test.yml`
> **Exa research:** CLI quality loop / `cli-agent-lint` 34 checks / ANC 8 principles / 37signals RUBRIC / RAG triad (LatentEval, NVIDIA RAGAS, MIRAGE, ACL AwF) / FTS5 (sqlite.org, mcp-fts5-starter, jpdillingham, andrewmara) / MCP MSSS 24 controls / local-first (Kleppmann, LoRe, EnCRDT, Turso, Ubimate/PortBay)

---

## 0. Tóm tắt điều hành

zbrain là **Go-native CLI local-first trusted memory** cho agent: Markdown OKF + evidence bất biến là canonical, SQLite FTS5 là cache disposable, chỉ `approved+digest hợp lệ` mới vào trusted result, fail-closed `gap/blocked/dirty/stale/rejected`, workspace hard isolation. Hiện trạng code khớp promise ~90% (216 tests, coverage 74.8%, vet 0, race pass, 22M unstripped, gate p95@100k <2s). Gap lớn nhất: **không có harness đo retrieval/perf**, **binary chưa strip + thiếu WAL**, **hybrid ranking sai (interleave re-score)**, **thiếu lint/vuln/SARIF trong CI**, **coverage thiếu 8pp cli**.

Plan này bung thành **7 phase, 8 ngày công (1 dev), không sửa trust contract**, mỗi phase có done-criteria đo được bằng `make bench` / `make eval` / `go test -cover`.

---

## 1. Audit chi tiết: nói gì → làm gì

### 1.1 Promise matrix

| # | Tuyên bố | Nguồn | Mức ưu tiên |
|---|---|---|---|
| P1 | Local-first, Go-native, không gọi LLM, binary standalone | `README.md:3` `trusted-memory-spec.md:15` | P0 |
| P2 | Canonical = Markdown OKF `type: zbrain.claim` `profile: zbrain.trusted-memory/v1` + evidence `raw`+`source.yaml` immutable | `trusted-memory-spec.md:140` | P0 |
| P3 | Trust: chỉ `approved` + `verified.digest` khớp `RenderClaimMarkdown` + evidence/support closure hợp lệ mới vào `ask` | `trusted-memory-spec.md:174` `query.go:275` | P0 |
| P4 | Draft = `promotion_candidates`, raw evidence không trusted, `gap`/`blocked`/dirty/missing/stale/rejected fail-closed | `trusted-memory-spec.md:270` `query.go:192` | P0 |
| P5 | Lifecycle `draft → approved → superseded\|revoked`, approve ghi `verified.at/by/digest` | `trusted-memory-spec.md:174` `claim.go:xxx` | P0 |
| P6 | Workspace isolation hard, chỉ `--include` mới đọc chéo, `ZBRAIN_HOME` override `~/.zbrain` | `README.md:18` `workspace_boundary.go:12` | P0 |
| P7 | Index `indexes/<ws>.sqlite` + `.dirty`, `reindex` validate không rewrite canonical, mtime+changeToken | `trusted-memory-spec.md:245` `index.go:181` | P0 |
| P8 | Gateway `zbrain mcp serve` stdio-only + challenge 15m/token 5m/TTY 16hex + viewer loopback CSP | `docs/trusted-agent-gateway-spec.md:34` `cli.go:321` | P0 |
| P9 | Lexical default, `--embed` opt-in loopback sidecar fallback lexical | `README.md:232` `cli.go:473` `query.go:209` | P1 |
| P10 | Release gate `test→vet→race→build→smoke→diff→CGO0` | `CONTRIBUTING.md:120` `.github/workflows/test.yml:27` | P0 |

### 1.2 Executable truth (đã chạy)

| Trục | Thực tế | File:line | Kết luận |
|---|---|---|---|
| CLI surface | `setup, workspace create/current, evidence add, claim draft/approve/supersede/revoke, migrate okf, reindex --embed, ask --workspace/--include/--embed, status, doctor --probe-embedder, mcp serve, view, approval show/grant, version` parse nghiêm `noFlags/askFlags/…` | `internal/cli/cli.go:23-60,95-150,940-969` | ✅ khớp README |
| Handler | mỏng, delegate `internal/runtime` | `cli.go:153-534` | ✅ đúng quy tắc |
| `ZBRAIN_HOME` | `ResolvePaths` đọc env, smoke dùng `mktemp -d` + `trash` | `paths.go:30` `Makefile:17-32` | ✅ |
| Permission | `0700` dir, `0600` md, `0400` evidence, `0600` index | `paths.go:6` | ✅ |
| Trust validate | digest, `sha256:evidence-v1`, `sha256:span-v1` 1-based inclusive, cycle, duplicate | `trust_validation.go`, `index.go:751`, `query.go:275` | ✅ |
| Fail-closed | dirty/missing/stale/rejected + symlink/regular-file check | `index.go:181-333` | ✅ |
| FTS5 | `claims_fts` trigger `ai/ad/au`, `fts5Query` quote token join space, `ORDER BY rank` | `index.go:1153,1144` | ✅ đúng, nhưng WAL thiếu |
| Hybrid | `mergeVectorResults` interleave + dedup + `Score=float64(i)` | `query.go:209-273` | ⚠️ mất BM25 rank gốc |
| Gateway/viewer | stdio+MCP, loopback `127.0.0.1`, CSP `default-src 'self'` `nosniff` no CORS 405 | `mcp/`, `view/server.go:104` | ✅ |
| Build | `dist/zbrain` 22M ELF unstripped `with debug_info` | `file dist/zbrain` | ⚠️ chưa strip |
| Tests | 216 func, `runtime 76.2% cli 66.7% mcp 75.5% view 69.5% total 74.8%` vet 0 race pass | `go test -cover` | ⚠️ thiếu 8pp cli |
| CI | `ubuntu+macos 1.24.0 test→vet→race→build→smoke→diff→CGO0` | `test.yml:12-44` | ✅ nhưng thiếu lint/vuln |

### 1.3 Lệch cần sửa

| ID | Lệch | Ảnh hưởng |
|---|---|---|
| D1 | `trusted-memory-spec.md:58` ghi gateway/view là "future milestone not shipped" trong khi code đã shipped | Docs parity, agent nhầm tính năng |
| D2 | `query.go:269` re-score mất rank | Retrieval NDCG/MRR |
| D3 | `index.go:createIndexSchema` không set `WAL/NORMAL` | Perf fsync thừa |
| D4 | Thiếu harness đo RAG/perf | Không đo được R1-R9/P1-P8 |

---

## 2. Khung đánh giá (tổng hợp Exa)

### 2.1 CLI (Go CLI builder, golang-cli-review, ANC, 37signals RUBRIC, cli-agent-lint)

- **5 trục:** Flow safety, Token efficiency, Self-describing, Automation safety, Predictability
- **RUBRIC /84:** T1 Agent Contract /26, T2 Reliability /16, T3 Agent Integration /13, T4 Distribution /29
- **Áp zbrain:** non-interactive default (trừ `approval grant` TTY), stdout=protocol stderr=diag, JSON 100%, exit `0/1/2`, `doctor` có `next_action`

### 2.2 Retrieval/RAG (LatentEval 6 metrics, NVIDIA RAGAS, MIRAGE 7560q, ACL AwF, arXiv Unified)

| Metric | Hỏi gì | Bẫy Exa nhấn |
|---|---|---|
| Precision@K/Recall@K | Trong K bao nhiêu relevant / trong relevant bao nhiêu lấy được | Tradeoff K |
| MAP/NDCG/MRR | Ranking order-aware | Cần BM25 weight |
| Context precision/recall | Retrieval đủ/đúng? | Phải tách retrieval vs generation |
| Faithfulness/groundedness | Answer supported bởi context? | Stale index vẫn pass nếu chỉ so context cũ |
| Answer relevancy | Có trả lời đúng câu hỏi? | Khác coverage |
| Noise sensitivity/containment | Retrieval fault có bị chặn? | zbrain fail-closed nên cao |
| Drift | Trôi theo thời gian (McNemar) | 1 lần chạy không đo được |

### 2.3 FTS5 (sqlite.org, mcp-fts5-starter, jpdillingham 13k qps, andrewmara 100×)

- 100/1k/10k docs: 97→77→41 doc/s, DB 676KB→5MB→58MB, p50 0.38ms→7.2ms→49ms
- FTS5 ~200× vs LIKE (0.38s→0.0017s/1M), Whoosh vs FTS5 78× ingest
- **WAL + batched commit 1.4-2×** (439s→241s @10k), `ORDER BY rank` nhanh hơn `bm25()` nếu abandon early, `detail=column` giảm size nếu chỉ cần title/body

### 2.4 MCP security (MSSS 24 ctrl/8 domains, OWASP, lirantal, mcprisk)

- L1 Essential 6 ctrl → L4 24 ctrl. zbrain stdio-only = an toàn nhất, đã có schema validate + path traversal block. Thiếu L2: bounds/timeout/audit-log/secret-redaction/rate-limit

### 2.5 Local-first (Kleppmann 7 principles, LoRe, EnCRDT, Turso 312×, Ubimate/PortBay)

- Offline-first, ownership, CRDT, exposure minimization. zbrain đã đạt, sync đa máy cố ý out-of-scope

### 2.6 Code quality (kyber, faultline, go-crap, gocyclo)

- Cyclomatic p95<10, CRAP<15, readability/testability, LOC p95 — chưa có trong CI

---

## 3. Metrics scorecard (fail build nếu vượt)

### 3.1 Trust P0 blocker

| ID | Metric | Target | Cách đo | Phase |
|---|---|---|---|---|
| T1 | Fail-closed 100% | 100% | `index_state_test.go` `query_test.go` | 3 |
| T2 | Digest mismatch rejected | 100% | mutation test | 3 |
| T3 | Evidence/span invalid → invalid | 100% | `evidence_test.go` | 3 |
| T4 | Workspace isolation | 100% | `workspace_boundary_test.go` | 3 |
| T5 | Symlink rejected | 100% | `index.go:103` test | 3 |

### 3.2 Retrieval P1

| ID | Metric | Target | Cách đo | Phase |
|---|---|---|---|---|
| R1 | Precision@10 | ≥0.75 | `make eval` golden set 50q | 2 |
| R2 | Recall@10 | ≥0.80 | — | 2 |
| R3 | MRR | ≥0.80 | 1/rank | 2 |
| R4 | NDCG@10 | ≥0.80 | graded | 2 |
| R5 | MAP@10 | report | — | 2 |
| R6 | Faithfulness | ≥0.95 | `validateIndexedClaimBinding` pass | 3 |
| R7 | Gap rate | <20% | known-answer set | 2 |
| R8 | Blocked rate | <5% | clean corpus | 2 |
| R9 | Drift ΔP/R | <5% | 2 lần reindex + McNemar | 3 |

### 3.3 Perf P1

| ID | Metric | Target | Cách đo | Phase |
|---|---|---|---|---|
| P1 | ask p50 @1k | <10ms | `TestAskP50P95P99` ×20 | 0,1 |
| P2 | ask p95 @100k | <2s (gate) | `ZBRAIN_BENCH_100K=1` | 0,1 |
| P3 | ask p99 @10k | <100ms | thêm p99 | 0 |
| P4 | reindex @10k | >50 doc/s | `time zbrain reindex` | 1 |
| P5 | DB @10k | <60MB | `du` | 0 |
| P6 | peak heap @10k | <20MB | `MemStats` | 0 |
| P7 | build stripped | <18M | `-ldflags="-s -w"` | 1 |
| P8 | warm <0.7×cold | pass | 2 search liên tiếp | 1 |

### 3.4 CLI UX P1

| ID | Metric | Target | Cách đo | Phase |
|---|---|---|---|---|
| C1 | --help 100% | 100% | sub-help check | 4 |
| C2 | JSON 100% | 100% | `writeJSON` audit | 4 |
| C3 | exit 0/1/2 typed | 100% | `cli_test.go` | 4 |
| C4 | stdout/stderr separated | 100% | `mcp_test.go` | 4 |
| C5 | doctor actionable | 100% | JSON `next_action` | 4 |
| C6 | cli-agent-lint | ≥B 70% | passive+active | 4 |
| C7 | anc audit | ≥90% A | `anc audit --binary` | 4 |

### 3.5 Security L1→L2 P1

| ID | Metric | Target | Phase |
|---|---|---|---|
| S1 | stdio-only | pass | 5 |
| S2 | path traversal 100% | 100% fuzz | 3 |
| S3 | schema validation | 100% | 5 |
| S4 | perm 0700/0600/0400 | 100% | 3 |
| S5 | secret redaction 0 | 0 | 5 |
| S6 | timeout ≤5s | ≤5s | 5 |
| S7 | govulncheck 0 | 0 | 5 |
| S8 | SARIF high 0 | 0 | 5 |

### 3.6 Maintainability P2

| ID | Metric | Target | Công cụ | Phase |
|---|---|---|---|---|
| M1 | runtime coverage | ≥80% (76.2→80) | `go test -cover` | 3 |
| M2 | cli coverage | ≥75% (66.7→75) | — | 3,4 |
| M3 | cyclomatic p95 | <10 | `gocyclo`/`kyber` | 5 |
| M4 | CRAP | <15 | `go-crap` | 5 |
| M5 | func len avg | <30 | `kyber` | 5 |
| M6 | golangci-lint high | 0 | `golangci-lint` | 5 |
| M7 | size regression | <+5%/release | CI artifact | 1 |

---

## 4. Gap → Task mapping (không đoán)

| Gap | Bằng chứng | Metric | Mức | Phase xử lý |
|---|---|---|---|---|
| G1 Binary 22M unstripped | `file ... with debug_info` | P7 | M | 1 |
| G2 Thiếu WAL/NORMAL | `createIndexSchema` không PRAGMA | P4 | M | 1 |
| G3 Hybrid re-score sai | `query.go:269 Score=float64(i)` | R3/R4 | M | 2 |
| G4 Thiếu eval harness | không có `queries.json` | R1-R9 | H | 0 |
| G5 Thiếu lint/vuln/SARIF | `test.yml` chỉ `go vet` | M6/S7 | M | 5 |
| G6 Coverage thiếu 8pp cli 4pp runtime | `view/server.go:58 0%` `writeTransitionBytesAtomic 58.8%` | M1/M2 | M | 3 |
| G7 Hybrid chưa benchmark fallback | `hybrid_test.go` mỏng | R9/P1 | M | 0,2 |
| G8 Viewer coverage 53% | `cover -func` | M2 | L | 3 |
| G9 Spec lệch | `spec:58` vs `README` | C1 | L | 6 |

---

## 5. Roadmap tổng quan

| Phase | Slug | Mục tiêu 1 câu | Thời gian | Owner | Gate |
|---|---|---|---|---|---|
| 0 | `baseline-harness` | Có số để so — bench + eval chạy <2p | 1d | dev | `make bench && make eval` pass |
| 1 | `perf-lowhang` | Giảm binary + tăng throughput không đổi logic | 1d | dev | P7<18M P4>50doc/s P2<2s |
| 2 | `retrieval-correctness` | RRF + eval đạt R1≥0.75 R3≥0.8 | 2d | dev | `make eval` đạt target |
| 3 | `trust-drift-coverage` | Fail-closed 100% + drift + coverage 80% | 1.5d | dev | T1-T5 100% M1≥80% |
| 4 | `cli-agent-contract` | Help/JSON/exit/ lint B anc 90% | 1d | dev | C6≥B C7≥90% |
| 5 | `security-supplychain` | L1→L2 lint/vuln/bounds/SARIF | 1d | dev | S7 0 M6 0 |
| 6 | `docs-parity-release` | Spec khớp --help, tag v0.2.x | 0.5d | dev | docs diff 0 |

Tổng 8 ngày công 1 dev, làm tuần tự 0→6. Có thể song song 1+2 sau khi 0 xong.

```
Tuần 1: [0 baseline][1 perf][2 retrieval —————————————]
Tuần 2: [3 trust/drift][4 cli][5 security][6 docs]
```

Depend: `0 → 1,2 → 3 → 4 → 5 → 6`. `1` và `2` chỉ cần `0`.

---

## 6. Phase chi tiết (đủ để work ngay)

### Phase 0 — Baseline & Harness (1d, không sửa logic)

**Mục tiêu:** Mọi metric sau đều có baseline so sánh. Không đụng `index.go`/`query.go` logic.

**Why (Exa):** `mcp-fts5-starter/docs/benchmark.md` chứng minh nếu không có bench shape, mọi optimize là bluff. LatentEval nhấn drift chỉ đo được khi có 2 measurement cùng query set.

**Scope:**
- In: viết harness
- Out: không sửa ranking, WAL, build flags

**Tasks:**

| ID | Task | File tạo/sửa | Ước lượng | Done criteria |
|---|---|---|---|---|
| 0.1 | `bench-fts5` script — gen syn corpus 100/1k/10k (avg 3.5KB/doc, vocab hẹp để hit rate cao), time cold `reindex`, capture throughput/DB size/peak heap (`runtime.MemStats` hoặc `tracemalloc` style), warm 15 query ×3 lần báo p50/p95/p99 | `scripts/bench-fts5.go` (Go, không Python để khỏi thêm dep), `docs/benchmark.md` | 4h | `go run ./scripts/bench-fts5.go --sizes=100,1000,10000` chạy <90s, in bảng markdown như Exa reference |
| 0.2 | `eval` harness — `docs/eval/queries.json` 50 query + `docs/eval/expected.json` map `relevant_claim_ids` (dùng corpus syn 1k, label thủ công 1 lần). Viết `internal/eval/eval.go` tính P@10/R@10/MRR/NDCG/MAP từ `IndexStore.Search` | `docs/eval/queries.json`, `docs/eval/expected.json`, `internal/eval/eval.go`, `Makefile: eval` | 3h | `make eval` chạy <30s, in JSON `{"P@10":0.72,"R@10":...}` |
| 0.3 | Wrapper benchmark p50/p95/p99 — thêm `TestAskP50P95P99` wrap `TestAskP95At100K` log cả 3, chạy 40 sample sort | `internal/runtime/index_benchmark_test.go:10` | 0.5h | `go test -run TestAskP50P95P99 -count=5 -v` in p50/p95/p99 |
| 0.4 | Baseline capture — chạy `make bench` + `make eval` + `go test -coverprofile=/tmp/cov.out` + `ls -lh dist/zbrain` + `file dist/zbrain`, lưu `docs/proofs/baseline-2026-08-25.json` | `docs/proofs/baseline-*.json`, `/tmp/cov.out` | 0.5h | File JSON commit được, có timestamp/hardware note |

**Verification:**
```bash
go run ./scripts/bench-fts5.go --sizes=100,1000 --json docs/proofs/bench-baseline.json
cat docs/benchmark.md  # bảng 100/1k/10k
make eval  # P@10/R@10/MRR/NDCG
go test -coverprofile=/tmp/cov.out ./... && go tool cover -func=/tmp/cov.out | tail -5
ls -lh dist/zbrain && file dist/zbrain
```

**Risks:** syn corpus vocab rộng → query miss hết → R thấp giả. Mitigate: vocab hẹp như mcp-fts5-starter note (3.5KB/doc, 15 phrase cố định hit >30% corpus).

**Exit:** `make bench && make eval` chạy local <2p, có baseline JSON.

---

### Phase 1 — Perf Low-hanging (1d, P7/P4, zero risk trust)

**Mục tiêu:** Giảm binary, tăng ingest throughput, không đổi retrieval correctness.

**Why (Exa):** mcp-fts5-starter 1.4-2× speedup từ WAL+batched commit, Medium 200× FTS5 vs LIKE, `jpdillingham` 13k qps mem cho thấy fsync là bottleneck.

**Tasks:**

| ID | Task | File | Ước lượng | Done |
|---|---|---|---|---|
| 1.1 | Strip build — `Makefile: build` thêm `dist/zbrain.stripped` bằng `go build -ldflags="-s -w" -trimpath ./cmd/zbrain`, CI upload cả 2, so sánh size. Giữ `dist/zbrain` unstripped cho debug local | `Makefile:10` ` .github/workflows/test.yml:36` | 1h | `ls -lh dist/*` show 22M→14-16M, `file` no debug_info |
| 1.2 | WAL + NORMAL — `index.go:createIndexSchema:1212` thêm sau `pragma user_version = 3`: `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;` (hoặc trong `Rebuild` sau `sql.Open(tmpPath)`). Đo P4 lại | `internal/runtime/index.go:1212` | 2h | `go test ./internal/runtime -run TestRebuild -count=5` pass, `make bench` P4 +30-80% @10k, atomic rename vẫn pass `TestRebuildGeneration` |
| 1.3 | (Optional, để TODO) Ghi chú `detail=column`/`columnsize=0` nếu >10k: thêm comment `// TODO(perf): eval detail=column if corpus>10k` | `index.go:1156` comment | 0.5h | comment có, không đổi schema |
| 1.4 | Re-baseline — chạy `make bench` + `make eval` lại, diff với baseline Phase 0, đảm bảo P2 vẫn <2s và R1 không giảm >2% | `docs/proofs/bench-after-phase1.json` | 1h | diff JSON <5% regression |

**Verification:**
```bash
make build && ls -lh dist/zbrain* && file dist/zbrain.stripped
go test ./internal/runtime -run TestRebuild -count=1 -v
go run ./scripts/bench-fts5.go --sizes=1000,10000 --json /tmp/p1.json && diff docs/proofs/bench-baseline.json /tmp/p1.json
ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v  # p95 <2s
```

**Risks:** WAL đổi durability (NORMAL mất power-loss guarantee nhưng index rebuildable nên ok — như mcp-fts5-starter note). Rollback: xóa 2 dòng PRAGMA.

**Exit:** P7<18M stripped, P4>50 doc/s @10k, P2<2s, không regress R.

---

### Phase 2 — Retrieval Correctness (2d, R1-R4, G3)

**Mục tiêu:** Sửa hybrid ranking, đạt R1≥0.75 R3≥0.8 trên `make eval`.

**Why (Exa):** `query.go:269` `Score=float64(i)` mất BM25 gốc, LatentEval nói precision/recall phải đo riêng, Redis blog nói BM25 ranking là core.

**Tasks:**

| ID | Task | File | Ước lượng | Done |
|---|---|---|---|---|
| 2.1 | RRF hybrid — `query.go:mergeVectorResults` + `interleaveClaims:243` thay re-score bằng Reciprocal Rank Fusion: `rrf = 1/(60+rank_lex) + 1/(60+rank_vec)`, dedup giữ rrf cao nhất, sort desc rrf rồi gán `Score` tăng dần để `sortQueryClaims` giữ order. Fallback lexical khi `vecIDs==nil` giữ nguyên | `internal/runtime/query.go:209-273` | 4h | `hybrid_test.go` thêm case: lex=[a,b] vec=[b,c] → [a,b,c] với rrf b cao nhất, unit `TestInterleaveRRF` pass |
| 2.2 | FTS5 query semantics — `index.go:fts5Query:1144` hiện quote từng token join space (=AND). Thêm test: phrase `"exact phrase"` giữ `"` không split, token `foo*` không quote `*`, `NEAR` không hỗ trợ thì reject. Không đổi default AND | `internal/runtime/index.go:1144`, `search_test.go` | 2h | `TestFTS5Query` cases: `hello world`→`"hello" "world"`, `"foo bar"`→`"foo bar"`, `foo*`→`foo*` |
| 2.3 | Eval tuning — chạy `make eval`, nếu R1<0.75: thử `title` weight 2× bằng cách trong `createIndexSchema` sửa `claims_fts` thành `fts5(title, description, tags, body, tokenize='porter unicode61')` hoặc trong query boost `title:` field (test A/B). Ghi `docs/eval/tuning.md` | `index.go:1168`, `docs/eval/tuning.md` | 6h | `make eval` R1≥0.75 R3≥0.8, P1 không tăng >15% |
| 2.4 | Re-baseline — lưu `docs/proofs/eval-after-phase2.json` | `docs/proofs/eval-after-phase2.json` | 0.5h | JSON commit |

**Verification:**
```bash
go test ./internal/runtime -run TestInterleave -count=1 -v
go test ./internal/runtime -run TestFTS5Query -count=1 -v
make eval  # expect R1>=0.75 MRR>=0.8
go test ./internal/runtime -run TestAskP50P95P99 -count=1 -v  # P1 <15% regress
```

**Risks:** RRF có thể làm lexical pure tốt hơn hybrid ở corpus nhỏ → test cả `embedding=false` path. Rollback: revert `interleaveClaims` 1 commit.

**Exit:** `make eval` đạt target, `hybrid_test.go` pass với RRF.

---

### Phase 3 — Trust Hardening + Drift + Coverage (1.5d, T1-T5, R9, M1-M2)

**Mục tiêu:** Fail-closed 100%, drift harness, coverage `runtime 80% cli 75%`.

**Tasks:**

| ID | Task | File | Ước lượng | Done |
|---|---|---|---|---|
| 3.1 | Drift harness — viết `internal/eval/drift.go`: snapshot R sau ingest 1k claim mới, chạy cùng `queries.json`, tính ΔP/R và McNemar test (paired). Template `docs/eval/drift.md` | `internal/eval/drift.go`, `docs/eval/drift.md` | 3h | `go run ./internal/eval/drift.go --before baseline.json --after new.json` in `drift: +1.2% p=0.34` |
| 3.2 | Fuzz boundary — thêm `workspace_boundary_test.go: fuzz TestWorkspaceBoundaryFuzz` với `../`, `%2e%2e`, `\0`, `//`, symlink, `wiki/../../etc/passwd` | `workspace_boundary_test.go:12` | 2h | `go test -fuzz FuzzWorkspaceBoundary -fuzztime=10s` 0 crash |
| 3.3 | Coverage lấp — viết test cho `transition.go:writeTransitionBytesAtomic 58.8%` (mock `os.WriteFile` error path), `view/server.go:58 Run 0%` (start+close harness với `ZBRAIN_HOME` temp), `handleStatic/Claims/Evidence 53%` (happy+404) | `transition_test.go:350`, `view/view_test.go:36`, `index_state_test.go` | 4h | `go test -coverprofile=/tmp/cov.out ./... && go tool cover -func | grep -E "view/server|transition"` → ≥80% |
| 3.4 | Faithfulness metric — thêm `R6` tính `validateIndexedClaimBinding` pass rate trên 100 claim approved sample | `internal/eval/eval.go: R6` | 1h | `make eval` in `faithfulness: 0.98` |

**Verification:**
```bash
go test ./internal/runtime -run TestWorkspaceBoundaryFuzz -fuzz -fuzztime=10s
go test -coverprofile=/tmp/cov.out ./... && go tool cover -func=/tmp/cov.out | tail -20
# expect runtime >=80% (từ 76.2)
go run ./internal/eval/drift.go --help
```

**Risks:** Fuzz có thể phát hiện symlink bypass chưa nghĩ → fix `validateIndexBoundaryPath` ngay.

**Exit:** T1-T5 100%, M1≥80% M2≥75% (hoặc ≥70% nếu viewer khó), drift harness chạy.

---

### Phase 4 — CLI / Agent Contract (1d, C1-C7)

**Mục tiêu:** `cli-agent-lint ≥B`, `anc ≥90%`, exit code typed, JSON 100%.

**Tasks:**

| ID | Task | File | Ước lượng | Done |
|---|---|---|---|---|
| 4.1 | --quiet/--no-color decision — audit: zbrain đã JSON nên `--quiet` = JSON, không cần thêm. Nếu thiếu, thêm global flag `--quiet` suppress `fmt.Fprintf(Stderr, "workspace created")` trong `cli.go:835`. Ghi `docs/cli-contract.md` lý do | `internal/cli/cli.go:835` `docs/cli-contract.md` | 1h | `cli-agent-lint --category TE` pass `p7-quiet` hoặc ghi `skip: TE-quiet reason: json-only` |
| 4.2 | Exit code chuẩn hóa — review `cli.go:70` `commandExitError` đảm bảo `unknownFlag/usage` →2, runtime error →1, `doctor` findings →2 (đã có `doctor:242`). Thêm `TestExitCodes` table-driven | `internal/cli/cli.go:932`, `cli_test.go` | 2h | `go test -run TestExitCodes -count=1 -v` 0/1/2 đúng |
| 4.3 | Help coverage — chạy `go run ./cmd/zbrain --help` + 12 sub-help, ensure mỗi command có `Usage:` + example. Thêm `surface` snapshot test như 37signals RUBRIC 2A.2 | `internal/cli/cli_test.go: surface` `docs/proofs/surface.txt` | 2h | `go test -run TestSurface -count=1 -v` snapshot khớp, CI fail nếu xóa command |
| 4.4 | Audit — chạy `cli-agent-lint` passive+active (nếu cài được) hoặc checklist thủ công anc.dev 8 principles, ghi `docs/proofs/cli-audit.md` | `docs/proofs/cli-audit.md` | 2h | `cli-agent-lint` ≥B, `anc` ≥90% hoặc manual PASS list |

**Verification:**
```bash
go run ./cmd/zbrain --help
go run ./cmd/zbrain claim draft --help && go run ./cmd/zbrain ask --help
go test ./internal/cli -run TestExitCodes -count=1 -v
go test ./internal/cli -run TestSurface -count=1 -v
```

**Risks:** Thêm `--quiet` có thể break script parse stdout → giữ stderr cho log.

**Exit:** C1-C5 100%, C6≥B.

---

### Phase 5 — Security & Supply Chain L1→L2 (1d, S1-S8, M6)

**Mục tiêu:** CI cứng, L2 bounds, SARIF.

**Tasks:**

| ID | Task | File | Ước lượng | Done |
|---|---|---|---|---|
| 5.1 | CI lint/vuln — `.github/workflows/test.yml` thêm `golangci-lint run` (với `golangci-lint` action), `govulncheck ./...`, giữ `go vet` | `.github/workflows/test.yml:30` | 2h | CI `golangci-lint` 0 high, `govulncheck` 0 vuln |
| 5.2 | Kyber/crap — thêm step `kyber analyze --format sarif -o kyber.sarif` và `go-crap --fail-on 15` (warn không block v0). Có thể dùng `faultline scan` alternative | `test.yml: race` after, `kyber.toml` | 2h | `kyber.sarif` artifact, `go-crap` CRAP<15 |
| 5.3 | Bounds/timeout — `internal/mcp/tools.go:51` thêm check `len(arg)>1MB → -32602`, `context.WithTimeout 5s` per tool call, log `workspace/tool/duration` không log body | `internal/mcp/tools.go`, `mcp/tools_test.go` | 2h | `go test ./internal/mcp -run TestBounds -count=1 -v` pass |
| 5.4 | SARIF upload — thêm `github/codeql-action/upload-sarif@v3` cho `kyber.sarif` + `faultline.sarif` | `test.yml: upload` | 1h | GitHub Security tab có SARIF |

**Verification:**
```bash
golangci-lint run ./... 2>&1 | head -20
govulncheck ./... 2>&1 | tail -5
kyber analyze --format json ./... 2>&1 | head -30
go test ./internal/mcp -run TestBounds -count=1 -v
```

**Risks:** `golangci-lint` lần đầu có thể ra 50 lỗi → đặt `fail_on_threshold: false` v0, fix dần.

**Exit:** S7 0 vuln, M6 0 high, SARIF upload ok.

---

### Phase 6 — Docs Parity & Release (0.5d)

**Mục tiêu:** Spec khớp `--help`, tag `v0.2.x`.

**Tasks:**

| ID | Task | File | Ước lượng | Done |
|---|---|---|---|---|
| 6.1 | Spec fix — `trusted-memory-spec.md:52-65` chuyển gateway/view/challenge từ "Authorized future milestones (not shipped)" sang "Shipped 2026-08-13 (see gateway-spec.md)" | `trusted-memory-spec.md:52` | 0.5h | `git diff --check` 0, `go run ./cmd/zbrain --help` khớp spec |
| 6.2 | Docs map — `docs/README.md:11` thêm row `benchmark/eval/proofs` | `docs/README.md:11` | 0.5h | link đúng |
| 6.3 | Tag — `git tag v0.2.3 -m "perf+eval harness"` + `CHANGELOG.md` note | `CHANGELOG.md`, `git tag` | 1h | `git log --oneline -3` có tag |

**Verification:**
```bash
go run ./cmd/zbrain --help | grep -E "mcp serve|view|approval"
grep -r "Shipped" trusted-memory-spec.md | head -5
git diff --check
```

**Exit:** Docs parity, tag pushed (khi user approve).

---

## 7. Timeline chi tiết (1 dev)

| Ngày | Phase | Deliverable commit |
|---|---|---|
| D1 AM | 0.1 bench script | `feat(bench): add bench-fts5 harness` |
| D1 PM | 0.2 eval harness + 0.3 wrapper + 0.4 baseline | `feat(eval): add golden queries + eval runner` |
| D2 AM | 1.1 strip + 1.2 WAL | `perf(runtime): WAL+NORMAL, stripped build` |
| D2 PM | 1.4 re-baseline + review | `docs: benchmark baseline after perf` |
| D3-4 | 2.1 RRF + 2.2 FTS5 query + 2.3 tuning | `fix(runtime): RRF hybrid, FTS5 phrase` |
| D5 AM | 3.1 drift + 3.2 fuzz | `test(runtime): drift harness, boundary fuzz` |
| D5 PM | 3.3 coverage lấp + 3.4 faithfulness | `test(runtime,view): coverage 80%` |
| D6 AM | 4.1 quiet + 4.2 exit code | `feat(cli): typed exit codes` |
| D6 PM | 4.3 surface + 4.4 audit | `docs: cli audit B` |
| D7 | 5.1-5.4 lint/vuln/SARIF/bounds | `ci(security): golangci, govulncheck, SARIF` |
| D8 AM | 6.1-6.3 spec + docs + tag | `docs(spec): mark gateway shipped` |

Nếu gấp 1 tuần (như §5 cũ): D1-D2 = Phase 0+1, D3-4 = Phase 2, D5-6 = Phase 3+4, D7 = Phase 5, D8 = Phase 6.

---

## 8. Cross-cutting

### Testing strategy
- Unit: mỗi phase thêm `*_test.go` cạnh package (`runtime`, `cli`, `mcp`, `view`)
- Integration: `make smoke` đã có, thêm `make eval` vào CI sau Phase 2
- Fuzz: `workspace_boundary` + `fts5Query`
- Bench: `TestAskP50P95P99` + `bench-fts5.go` lưu JSON để so sánh

### CI changes (`.github/workflows/test.yml:27-44`)

```yaml
- name: Lint
  run: golangci-lint run ./...
- name: Vuln
  run: govulncheck ./...
- name: Kyber
  run: kyber analyze --format sarif -o kyber.sarif || true
- name: Eval
  run: make eval  # từ Phase 2
  if: hashFiles('docs/eval/queries.json') != ''
```

### File impact summary

| File | Phase | Loại |
|---|---|---|
| `scripts/bench-fts5.go` (new) | 0 | bench harness |
| `docs/eval/*` (new) | 0,2,3 | eval harness |
| `docs/benchmark.md` (new) | 0 | bảng 100/1k/10k |
| `docs/proofs/*` (new) | 0,1,2,4 | baseline/audit |
| `internal/runtime/index_benchmark_test.go` | 0 | p50/p99 wrapper |
| `internal/runtime/index.go:1168,1212` | 1,2 | WAL + FTS5 schema |
| `internal/runtime/query.go:209` | 2 | RRF |
| `internal/runtime/workspace_boundary_test.go` | 3 | fuzz |
| `internal/cli/cli.go:70,932` | 4 | exit code |
| `.github/workflows/test.yml` | 5 | lint/vuln/SARIF |
| `trusted-memory-spec.md:52` | 6 | docs parity |
| `Makefile` | 0,1 | bench/eval targets |

### Rollback plan
- Mỗi phase 1 commit, revert 1 commit là rollback. Phase 1 WAL revert xóa 2 dòng PRAGMA. Phase 2 RRF revert `interleaveClaims`.

---

## 9. Rủi ro & không làm (giữ scope)

| Rủi ro | Mitigate |
|---|---|
| Syn corpus vocab rộng → R thấp giả | Vocab hẹp 15 phrase như mcp-fts5-starter |
| WAL đổi durability | Index rebuildable nên ok, ghi chú trong PR |
| golangci-lint ra 50 lỗi | `fail_on_threshold:false` v0, fix dần |
| Hybrid RRF tệ hơn lexical pure ở small corpus | Test cả `embedding=false` path |

**Không làm trong plan này:** vector DB hosted, HTTP MCP, sync đa máy/CRDT, trigram LIKE, redesign trust contract.

---

## 10. Appendix

### A. `docs/eval/queries.json` schema (draft)

```json
{
  "version": 1,
  "queries": [
    {"id": "q001", "text": "cache performance Redis", "relevant_claim_ids": ["clm_..."], "graded": {"clm_...": 2}},
    {"id": "q002", "text": "evidence span authority", "relevant_claim_ids": ["clm_..."]}
  ]
}
```

`make eval` tính P@10 = |retrieved ∩ relevant|/|retrieved|, R@10 = |∩|/|relevant|, MRR = 1/rank first, NDCG = DCG/IDCG.

### B. `bench-fts5.go` sketch

```go
for _, n := range []int{100,1000,10000} {
  genCorpus(n, avgBytes=3500) // vocab hẹp
  t0:=time.Now(); reindex(); dt:=time.Since(t0)
  throughput:=float64(n)/dt.Seconds()
  dbSize:=os.Stat(indexPath).Size()
  // warm 1 query, then 15*3 measure p50/p95/p99
}
```

### C. Nguồn Exa trích (đủ để audit lại)

- `skills/go-cli-builder/references/cli-quality-loop.md` — shipcheck/docs parity
- `cli-agent-lint` 34 checks FS/TE/SD/SA/PV + `anc audit` 8 principles
- LatentEval 6 metrics, NVIDIA RAGAS `context_recall/precision/faithfulness`, ACL AwF, arXiv Unified `relevance/faithfulness/correctness`
- `sqlite.org/fts5.html`, `mcp-fts5-starter/docs/benchmark.md` WAL 1.4-2×, jpdillingham 13k qps, andrewmara 100×, Medium 200×
- MSSS 24 controls, OWASP playbook, lirantal, mcprisk POLICY.md
- Kleppmann `local-first.pdf` 7 principles, LoRe, EnCRDT, Turso 312×, Ubimate/PortBay

---

## 11. Checklist handoff cho dev nhận plan

- [ ] Đọc `docs/plans/active/zbrain-optimization-plan.md` này + `docs/README.md`
- [ ] Chạy `zharness preflight` nếu đi theo workflow skill (optional cho plan này)
- [ ] Bắt đầu Phase 0: `go run ./scripts/bench-fts5.go --help` phải chạy trước khi sửa logic
- [ ] Mỗi phase xong chạy `go test ./... && go vet ./... && make build && make smoke` như CONTRIBUTING gate
- [ ] Commit message `feat|fix|perf|test|docs|ci(scope): ...` như `AGENTS.md`

---

## 12. Fanout cho subagents — làm song song 1 lần

> Mục tiêu ní yêu cầu: chia nhỏ để **fanout tối đa, độc lập, 1 lần bắn hết**. Dưới đây là ma trận đã tách theo **file ownership** để tránh conflict merge.

### 12.1 Nguyên tắc tách

1. **File ownership = merge boundary** — 2 task đụng cùng file thì xếp cùng lane hoặc wave khác nhau.
2. **Wave 0 bắt buộc trước** — harness + baseline là dependency cho mọi metric sau. Xong Wave 0 mới fanout Wave 1.
3. **Wave 1 fanout tối đa** — 4 lane độc lập theo file, mỗi lane bung thêm 2-3 sub-task con.
4. **Mỗi subagent 1 commit nhỏ, 1 file chính** — dễ review, revert 1 commit không vỡ lane khác.

### 12.2 DAG & Wave

```
Wave 0 (foundation, 3 song song) ─┬─► Wave 1 (optimization, 10 song song)
                                  │
  0.1 bench-fts5 ─┐               ├─► Lane A perf:    A1 Makefile strip ─┐
  0.2 eval harness─┼─► 0.4 baseline┤                       A2 index.go WAL ─┼─► A3 re-baseline
  0.3 p50 wrapper ─┘               ├─► Lane B retrieval: B1 query.go RRF ─┐
                                  │                      B2 index.go FTS5 ─┼─► B3 tuning/eval
                                  ├─► Lane C trust:   C1 drift.go ─┐
                                  │                    C2 fuzz boundary─┼─► C3 coverage/faithfulness
                                  │                    C3a/b/c (3 file con) ─┘
                                  └─► Lane D cli:     D1 quiet doc ─┐
                                                       D2 exit code ─┼─► D4 audit
                                                       D3 surface    ─┘
Wave 1 xong ─► Wave 2 (security, 3 song song) ─► Wave 3 (docs, 2 song song + tag)
```

**Số subagent tối đa 1 lần sau Wave 0: 10** (A1+A2 + B1+B2 + C1+C2+C3a/b/c + D1+D2+D3 → gộp gọn còn 10 nếu C3 gộp).

### 12.3 Ma trận task độc lập (copy-paste cho Task tool)

#### Wave 0 — Foundation (3 subagents, chạy song song ngay)

| Task ID | File chính (ownership) | Phụ thuộc | Lệnh verify | Subagent prompt (dùng cho `Task` tool) |
|---|---|---|---|---|
| **0.1** | `scripts/bench-fts5.go` (new), `docs/benchmark.md` | none | `go run ./scripts/bench-fts5.go --sizes=100,1000 --json /tmp/b.json` | `Implement scripts/bench-fts5.go per §6 Phase 0.1: gen syn corpus 100/1k/10k avg 3.5KB vocab hẹp, time cold reindex via IndexStore.Rebuild, capture throughput/DB size/peak heap, 15 query×3 warm p50/p95/p99, write docs/benchmark.md table. Do not touch index.go/query.go.` |
| **0.2** | `docs/eval/queries.json`, `docs/eval/expected.json`, `internal/eval/eval.go`, `Makefile` | none | `make eval` | `Implement docs/eval/queries.json 50 queries + expected.json + internal/eval/eval.go per §6 0.2: calculate P@10/R@10/MRR/NDCG/MAP from IndexStore.Search. Add Makefile target eval. Do not touch bench script.` |
| **0.3** | `internal/runtime/index_benchmark_test.go:10` | none | `go test -run TestAskP50P95P99 -count=1 -v` | `Add TestAskP50P95P99 wrapper around TestAskP95At100K per §6 0.3: 40 samples, sort, log p50/p95/p99, gate p95<2s. Only edit index_benchmark_test.go.` |

Sau 3 task xong, 1 agent chạy **0.4** baseline capture: `make bench && make eval && go test -coverprofile=/tmp/cov.out ./...` → `docs/proofs/baseline-2026-08-25.json`.

#### Wave 1 — Optimization lanes (10 subagents, fanout 1 lần sau Wave 0)

| Lane | Task ID | File chính | Phụ thuộc | Prompt |
|---|---|---|---|---|
| **A perf** | **A1** | `Makefile:10`, `.github/workflows/test.yml:36` | Wave 0 | `Makefile: add dist/zbrain.stripped via go build -ldflags="-s -w" -trimpath, CI upload both. Verify ls -lh dist/* 22M→14-16M. Only Makefile+workflow.` |
| | **A2** | `internal/runtime/index.go:1212` | Wave 0 | `index.go:createIndexSchema after pragma user_version=3 add PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; Ensure temp DB path still atomic rename. Test: go test -run TestRebuild -count=5. Only index.go.` |
| **B retrieval** | **B1** | `internal/runtime/query.go:209-273` | Wave 0 | `query.go: replace interleaveClaims re-score Score=float64(i) with RRF k=60 per §6 2.1, keep dedup, add hybrid_test.go TestInterleaveRRF. Only query.go+hybrid_test.go.` |
| | **B2** | `internal/runtime/index.go:1144` (`fts5Query`), `internal/runtime/search_test.go` | Wave 0 | `index.go:fts5Query handle phrase "x y"→"x y" and foo*→foo* per §6 2.2, add TestFTS5Query. Only index.go+search_test.go. Do not touch query.go RRF.` |
| **C trust** | **C1** | `internal/eval/drift.go`, `docs/eval/drift.md` | Wave 0 (needs eval) | `Implement internal/eval/drift.go per §6 3.1: snapshot before/after ingest 1k, same queries.json, ΔP/R + McNemar. Only eval/drift files.` |
| | **C2** | `internal/runtime/workspace_boundary_test.go` | Wave 0 | `Add fuzz TestWorkspaceBoundaryFuzz per §6 3.2: ../, %2e%2e, \0, //, symlink, wiki/../../. Only workspace_boundary_test.go.` |
| | **C3a** | `internal/runtime/transition_test.go` | Wave 0 | `Cover transition.go:writeTransitionBytesAtomic 58.8% per §6 3.3: mock WriteFile error path. Only transition_test.go (+ transition.go if needed).` |
| | **C3b** | `internal/view/view_test.go`, `internal/view/server.go:58` | Wave 0 | `Cover view/server.go:58 Run 0% per §6 3.3: harness temp ZBRAIN_HOME start+close. Only view files.` |
| | **C3c** | `internal/runtime/index_state_test.go` | Wave 0 | `Cover index_state/handleStatic per §6 3.3. Only index_state files.` |
| **D cli** | **D1** | `docs/cli-contract.md`, `internal/cli/cli.go:835` | Wave 0 | `Audit --quiet/--no-color per §6 4.1: zbrain JSON-only so document skip reason, optionally add --quiet suppress Stderr. Only cli.go+cli-contract.md.` |
| | **D2** | `internal/cli/cli.go:70`, `internal/cli/cli_test.go` | Wave 0 | `Typed exit codes per §6 4.2: unknownFlag→2, runtime→1, doctor→2, add TestExitCodes table-driven. Only cli files.` |
| | **D3** | `internal/cli/cli_test.go` (surface), `docs/proofs/surface.txt` | Wave 0 | `Surface snapshot per §6 4.3: go run --help + 12 sub-helps, TestSurface snapshot, fail on removal. Only cli_test.go+surface.txt.` |

> Lưu ý file conflict: `A2` và `B2` cùng đụng `index.go` nhưng khác hàm (`createIndexSchema` vs `fts5Query`). Để fanout 1 lần không conflict, gộp `A2+B2` thành 1 subagent **AB-index** hoặc dùng `git worktree` mỗi lane. Khuyến nghị gộp → Wave 1 còn **9 subagents**.

#### Wave 2 — Security (3 subagents, sau Wave 1, hoặc song song Wave 1 nếu chấp nhận risk)

| Task ID | File | Prompt |
|---|---|---|
| **S1** | `.github/workflows/test.yml` | `CI lint/vuln per §6 5.1: add golangci-lint run, govulncheck ./..., keep go vet. Only workflow.` |
| **S2** | `kyber.toml`, `test.yml` (kyber/crap step) | `Add kyber analyze --format sarif + go-crap --fail-on 15 per §6 5.2, warn not block. Only kyber.toml+workflow.` |
| **S3** | `internal/mcp/tools.go`, `internal/mcp/tools_test.go` | `MCP bounds/timeout per §6 5.3: len>1MB→-32602, WithTimeout 5s, log workspace/tool/duration. Only mcp files.` |

`S1` vs `S2` cùng đụng `test.yml` → gộp thành **S12-workflow** nếu fanout 1 lần. Khi đó Wave 2 còn **2 subagents** (`S12` + `S3`).

#### Wave 3 — Docs (2 subagents, cuối cùng)

| Task ID | File | Prompt |
|---|---|---|
| **F1** | `trusted-memory-spec.md:52` | `Docs parity per §6 6.1: move gateway/view/challenge from future→Shipped 2026-08-13.` |
| **F2** | `docs/README.md:11` | `Docs map per §6 6.2: add row benchmark/eval/proofs.` |

`F3` tag `v0.2.3` chạy sau khi F1+F2 merge, 1 agent.

### 12.4 Cách chạy fanout thực tế với opencode Task tool

**Wave 0 (3 song song):**
```ts
Task(subagent_type="general", description="bench harness 0.1", prompt="Implement scripts/bench-fts5.go per docs/plans/active/zbrain-optimization-plan.md §6 0.1 ...")
Task(subagent_type="general", description="eval harness 0.2", prompt="Implement docs/eval/queries.json + internal/eval/eval.go per §6 0.2 ...")
Task(subagent_type="general", description="p50 wrapper 0.3", prompt="Add TestAskP50P95P99 per §6 0.3 ...")
```

**Wave 1 (9 song song sau Wave 0 xong):**
```ts
Task(description="A1 strip", prompt="Makefile strip per §12.3 A1...")
Task(description="AB-index WAL+FTS5", prompt="index.go WAL+fts5Query per A2+B2...")
Task(description="B1 RRF", prompt="query.go RRF per B1...")
Task(description="C1 drift", prompt="drift.go per C1...")
Task(description="C2 fuzz", prompt="workspace_boundary fuzz per C2...")
Task(description="C3a transition", prompt="cover transition per C3a...")
Task(description="C3b view", prompt="cover view per C3b...")
Task(description="D1 quiet", prompt="cli quiet doc per D1...")
Task(description="D2 exit", prompt="cli exit codes per D2...")
Task(description="D3 surface", prompt="cli surface per D3...")
```

**Wave 2 (2 song song):**
```ts
Task(description="S12 workflow", prompt="CI lint/vuln/kyber per S1+S2...")
Task(description="S3 mcp bounds", prompt="mcp bounds per S3...")
```

**Wave 3 (2 song song):**
```ts
Task(description="F1 spec", prompt="trusted-memory-spec parity per F1...")
Task(description="F2 docs map", prompt="docs/README map per F2...")
```

### 12.5 Merge strategy & tránh conflict

- **Branch per lane:** `feat/phase0-bench`, `feat/phase0-eval`, `perf/wal`, `fix/rrf`, `test/drift`, `feat/cli-exit` … mỗi subagent 1 branch, PR nhỏ.
- **File ownership đã tách:** 95% task không đụng file nhau (xem bảng). Chỉ `index.go` có 2 task → gộp AB-index.
- **Verify per lane:** mỗi subagent chạy `go test ./internal/<pkg> -run TestX -count=1 -v` + `go vet` trước khi push, không cần full suite.
- **Final gate:** 1 agent chạy `go test ./... && go vet ./... && make build && make smoke && git diff --check && CGO_ENABLED=0 go build` sau khi merge tất cả lane (như CONTRIBUTING gate).

### 12.6 Ước lượng khi fanout

| Cách làm | Thời gian wall-clock | Subagents |
|---|---|---|
| Tuần tự 1 dev (cũ) | 8 ngày | 1 |
| Fanout Wave 0→1→2→3 (đề xuất) | **2.5 ngày** (Wave 0: 0.5d, Wave 1: 1d, Wave 2: 0.5d, Wave 3: 0.5d) | 3→9→2→2 |
| Fanout max 1 lần (risk cao, bỏ Wave 0 dependency) | 1.5 ngày | 15 cùng lúc nhưng phải mock baseline |

Khuyến nghị **4 waves** để giữ metric baseline đúng, vẫn giảm 8→2.5 ngày.

### 12.7 Checklist fanout cho người điều phối

- [ ] Wave 0 xong mới bắn Wave 1 — kiểm `docs/proofs/baseline-2026-08-25.json` tồn tại
- [ ] Mỗi subagent chỉ sửa file trong cột "File chính" của nó
- [ ] Mỗi PR gắn label `wave-0/1/2/3` + `file-ownership: index.go` để reviewer biết conflict scope
- [ ] Sau mỗi wave merge vào `master` rồi rebase lane tiếp theo
- [ ] Final gate 1 agent chạy full suite trước tag `v0.2.3`

---

## Approach and Risks

- approach: wave-1 fanout theo file ownership rồi wave-2 gate đơn — phần còn lại của plan (phase 5 gate + proofs) tách 2 wave độc lập file, không đụng logic retrieval/trust
- constraints:
  - Go 1.24 line giữ nguyên (patch bump `1.24.0 → 1.24.1` chấp nhận, không đổi support contract)
  - Không sửa trust contract, không sửa golden label để che metric (R2 cap đã ghi decision)
  - CI lint/vuln steps chuyển từ `continue-on-error: true` → blocking, đúng exit criteria §5 "S7 0 / M6 0"
- dependencies:
  - wave 1: 2 task song song (security-gates: go.mod+workflow+test files | proofs-close: docs/eval + docs/proofs)
  - wave 2: gate cuối phụ thuộc wave 1 xong
- rejected_alternatives:
  - Downgrade/disable govulncheck step → từ chối: che metric
  - Upgrade Go 1.25 → từ chối: đổi support contract, plan đã quyết giữ 1.24
  - Viết plan mới riêng → từ chối: cùng initiative, dùng file active hiện tại
- risks:
  - go1.24.1 toolchain cần download khi build → môi trường offline fail; mitigate: go.mod directive + CI matrix `1.24.x`, rollback = revert 1 dòng go.mod
  - 43 golangci issues chủ yếu test files → fix theo hint errcheck/staticcheck/unused, không refactor logic
  - kyber không cài local → S8 SARIF chỉ verify CI (step `|| true`), ghi nhận limitation
- recovery:
  - Lint fix vỡ test → `git checkout -- <file>` rồi sửa lại từng hint
  - Toolchain fail → revert go.mod, ghi decision, gate vẫn chạy với golangci blocking

## Phases and Verification

<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. -->

- planning_status: planned
- phases:
  - slug: `security-gate-close` | story: `01M0VX528E4F7F7CHYAC23VJQD` | status: in-progress | depends_on: `retrieval-correctness`
    - goal: Close phase-5 gates: govulncheck 0 (go1.24.1), golangci-lint 0 high + CI blocking, proofs C6/C7 + R9 drift, final gate + commit + handoff
    - allowed_surfaces: `go.mod`, `.github/workflows/test.yml`, `internal/**/*_test.go`, `docs/eval/`, `docs/proofs/`
    - avoided_surfaces: `internal/runtime/index.go`, `internal/runtime/query.go`, `trusted-memory-spec.md` trust contract, golden label
    - waves:
      - wave 1 (2 task song song, file-disjoint):
        - t1 `security-gates` — bump `go 1.24.0 → 1.24.1` (go.mod + CI matrix), bỏ `continue-on-error` ở Lint/Govulncheck steps, fix 43 golangci issues (errcheck/ineffassign/staticcheck/unused, chủ yếu `workspace_boundary_test.go`, `view_test.go`)
          - verify: `go version` ≥ 1.24.1 · `govulncheck ./...` exit 0 (0 vuln) · `golangci-lint run ./...` exit 0
        - t2 `proofs-close` — viết `docs/proofs/cli-audit.md` (C6 cli-agent-lint ≥B hoặc manual PASS + C7 anc ≥90%), đo R9 drift (`go run ./internal/eval/drift.go` before/after → ΔP/R + McNemar, ghi `docs/eval/drift.md` + proof JSON), capture coverage JSON (runtime ≥80%, cli ≥75%)
          - verify: 3 artifact tồn tại · drift ΔP/R < 5% · coverage runtime ≥80% cli ≥75%
      - wave 2 (1 task):
        - t3 `final-gate` — `go test ./... && go vet ./... && go test -race ./internal/runtime ./internal/cli ./internal/view ./internal/mcp && make build && make smoke && git diff --check && CGO_ENABLED=0 go build`; check record + commit + handoff
          - verify: gate exit 0 · check verdict APPROVED · commit sạch
    - checks:
      - check 1 (wave 1): `govulncheck ./...` 0 vuln + `golangci-lint run ./...` exit 0
      - check 2 (wave 1): proofs artifacts + drift Δ < 5%
      - check 3 (wave 2): full gate + APPROVED verdict

## Progress

<!-- Append-only durable entries record timestamp, phase, wave, task, task_status, run_id, trace_id, exact verification/result, and changed surfaces or blocker. -->
- `2026-08-25T07:22:59Z` — wave 2, task eval-gate. task_status: `DONE_WITH_CONCERNS`. run: `01M0VWQN4G6ZTXG9B3K125G7C5`. summary: make eval (corpus=1000, limit=10, 50q): P@10=1.000 R@10=0.054 F1=0.102 MRR=1.000 NDCG@10=1.000 MAP@10=1.000 gap=0% blocked=0% faith=1.000; artifact docs/proofs/eval-after-phase2.json captured; R2 cap: relevantForQuery AND-token -> relevant_total>10 nen recall@10 khong the >=0.80.
- `2026-08-25T07:59:18Z` — wave 1, task security-gates. task_status: `DONE`. run: `01M0VXMD4J6B401A87V4PYH5C1`. summary: go.mod+CI go 1.24.0->1.25.13, go-sdk v1.4.1 (owner approved bump; go1.24.1 govulncheck=39 affected, EOL); Lint+Govulncheck continue-on-error removed; 43 golangci issues fixed (25 errcheck, 2 ineffassign, 13 staticcheck, 3 unused; ~20 files, mechanical per-hint); golangci-lint run exit 0, govulncheck No vulnerabilities, go test ./... + go vet clean.
- `2026-08-25T07:59:18Z` — wave 1, task proofs-close. task_status: `DONE`. run: `01M0VXMD4J6B401A87V4PYH5C1`. summary: drift: baseline 1000 vs after-ingest 2000, P=1.000->1.000 R=0.0536->0.0270, McNemar p=1.0 (no discordant pairs), verdict no drift (Delta<5%, drop = corpus dilution at fixed K); coverage runtime 80.1 cli 78.3 mcp 76.5 view 92.8; cli-audit PASS 5/5 (12/12 help, TestExitCodes 28/28, JSON stdout clean); artifacts docs/proofs/{drift-after-phase3,coverage-after-phase3,cli-audit}.{json,json,md}.
- `2026-08-25T07:59:20Z` — wave 1. run: `01M0VXMD4J6B401A87V4PYH5C1`. summary: Wave 1 done: security gates closed (S7 govulncheck 0, M6 lint 0, CI blocking) + proofs complete (R9 drift no-drift, coverage runtime 80.1/cli 78.3, C6/C7 manual PASS). Owner approved Go 1.25.13 contract bump..
- `2026-08-25T08:02:37Z` — handoff recorded. handoff: `01M0VZ2MTWW2MERK7V89ZBRWW0`. run: `01M0VXMD4J6B401A87V4PYH5C1`. check: `01M0VZ02KDXYYWST3M43BY25RN`. phase closed.
- `2026-08-25T08:02:46Z` — handoff recorded. handoff: `01M0VZ2XP0YB3J1HWASME3A4FG`. run: `01M0VWQN4G6ZTXG9B3K125G7C5`. check: `01M0VWT5VQYD31GZNZMS8JHA09`. phase closed.

## Decisions

<!-- Append-only durable entries record timestamp, phase/task, decision, and rationale. -->
- `2026-08-25T07:23:21Z` — R2 (Recall@10>=0.80) khong do duoc voi golden set hien tai: relevantForQuery dung AND-token tren toan haystack + corpus vocab hep nen relevant_total>10, cap vat ly recall@10 ~0.02-0.05. (phase: `retrieval-correctness`), task: eval-gate. rationale: Lam label chat hon (chi primary phrase) hoac K lon hon de do recall; ghi nhan gap, khong sua label che giau metric..
- `2026-08-25T07:23:21Z` — Capture docs/proofs/eval-after-phase2.json tu ket qua make eval hien tai (post-WAL/RRF). (phase: `retrieval-correctness`), task: eval-gate. rationale: Plan task 2.4 yeu cau proof JSON re-baseline sau phase 2; Makefile ghi de eval-baseline.json nen phai copy rieng..
- `2026-08-25T07:59:02Z` — Accept Go 1.25.13 + go-sdk v1.4.1: owner approved toolchain bump after verified evidence (Go 1.24 EOL 2026-08, 39 stdlib vulns reachable on go1.24.1, fixes only in 1.25.8+; S7 govulncheck=0 impossible on 1.24). (phase: `security-gate-close`), task: security-gates. rationale: Verified empirically: GOTOOLCHAIN=go1.24.1 govulncheck ./... on original go.mod reports 39 affected; CI matrix and go.mod bumped to 1.25.13, Lint+Govulncheck steps now blocking (continue-on-error removed). Supersedes decision 01M0EQBM3W0F2C98ZJ73XWJVC5 rationale re go-sdk pin..
- `2026-08-25T08:03:11Z` — plan completed. rationale: every phase_slug is a done story.

## Validation

<!-- Append-only durable entries record timestamp, phase, exact command/result/output, run_id, check_id, verdict, and proof_gaps. -->
- `2026-08-25T07:23:02Z` — check. verdict: `APPROVE_WITH_REQUESTS`. check: `01M0VWT5VQYD31GZNZMS8JHA09`. run: `01M0VWQN4G6ZTXG9B3K125G7C5`. phase: `retrieval-correctness`. judge: `same-session` (deepseek-v4-flash).
  - `make eval` → P@10=1.000 R@10=0.054 F1=0.102 MRR=1.000 NDCG@10=1.000 MAP@10=1.000 gap=0% blocked=0% faith=1.000
- `2026-08-25T08:01:13Z` — check. verdict: `APPROVED`. check: `01M0VZ02KDXYYWST3M43BY25RN`. run: `01M0VXMD4J6B401A87V4PYH5C1`. phase: `security-gate-close`. judge: `same-session` (deepseek-v4-flash).
  - `go test ./...` → all packages ok
  - `go vet ./...` → vet 0
  - `go test -race ./internal/runtime ./internal/cli ./internal/view ./internal/mcp` → 4 packages ok
  - `make build` → dist/zbrain.stripped 15M
  - `make smoke` → full lifecycle pass, index fresh
  - `golangci-lint run ./...` → 0 issues
  - `govulncheck ./...` → No vulnerabilities found, 0 affected
  - `git diff --check` → clean
  - `CGO_ENABLED=0 go build ./cmd/zbrain` → CGO-free build ok

## Current State and Next Action

- active_phase: security-gate-close
- lifecycle_status: in-progress
- latest_run_id: 01M0VXMD4J6B401A87V4PYH5C1
- latest_trace_ids: []
- latest_check_id: none
- latest_handoff_id: none
- blockers: none
- open_items:
  - t1 security-gates: bump go1.24.1 + CI blocking + fix 43 golangci issues
  - t2 proofs-close: cli-audit.md + R9 drift + coverage JSON
- exact_next_action: wave 1 fanout t1+t2 song song → wave 2 final-gate

