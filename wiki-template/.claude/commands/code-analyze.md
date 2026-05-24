# Code Analyze

Phân tích **codebase có sẵn** → snapshot metadata → đẩy vào evidence pipeline để đề xuất bổ sung **platform patterns**, **platform contracts**, **project services**, và **decision (ADR) drafts** cho workspace active.

> Entry point user-friendly. Dưới capô gọi logic của `/evidence-ingest --source code` rồi `/evidence-analyze` (CORE-CODE prompts).
> Reference: [code-snapshot-conventions.md](../../agents/pipeline/code-snapshot-conventions.md), [code-analysis-prompts.md](../../agents/pipeline/code-analysis-prompts.md), [evidence-state-rules.md](../../agents/pipeline/evidence-state-rules.md).

---

## Input

| Arg              | Required | Notes                                                                |
|------------------|----------|----------------------------------------------------------------------|
| `--ref`          | optional | Repo path. Default: `<cwd>` của codebase (parent của `.claude/wiki.json`). **Mutually exclusive với `--bundle`.** |
| `--scope`        | optional | (Single-repo) Comma-separated paths/globs included. Default heuristic: `src/**`, `pom.xml`/`package.json`/`build.gradle`/`Cargo.toml`/`go.mod`, `application.*`, `Dockerfile`, `*.yaml` config. |
| `--bundle`       | optional | **Bundle mode**: đường dẫn tới thư mục staging chứa `bundle.yaml`. Nếu pass `--bundle`, ignore `--ref` và `--scope`. **Mutually exclusive với `--ref`.** |
| `--label`        | optional | Mô tả ngắn (≤ 30 ký tự). Single-repo: default `bootstrap-{project}` hoặc `refresh-{project}-{date}`. Bundle: override `bundle.yaml#label`. |
| `--skip-analyze` | optional | Chỉ ingest, không tự chạy `/evidence-analyze`. Dùng khi muốn review snapshot trước. |
| `--allow-configs` | optional | Cho phép đọc file config vào Section 3. Mặc định tất cả config bị block. Khi set, sẽ AskUserQuestion từng nhóm file trước khi đọc. Hard blocklist (prod/secret/key files) vẫn áp dụng. |
| `--with-drafts`  | optional | Chạy thêm ON-DEMAND `c05` (service drafts) + `c06` (decision drafts) + `c07` (config overrides) sau CORE. |

Nếu user gọi `/code-analyze` không có arg → confirm qua AskUserQuestion: "Single-repo (`--ref`) hay bundle (`--bundle`)?"

---

## Bước 0 — Workspace check

1. Tìm `.claude/wiki.json`: từ `<cwd>` đi lên parent. Lưu `wiki_json_dir`.
2. Đọc file → `workspace` + `wiki_root` resolve theo [system-prompt.md `wiki_root` Resolution Rule](../../agents/system-prompt.md):
   - Absolute path → dùng nguyên
   - Relative (`"."`, `"./..."`) → resolve relative TỚI `project_root` (= parent của `.claude/`)
   - `null`/empty → fallback `~/.claude/wiki-global.json#wiki_root`
3. STOP nếu file thiếu hoặc `.workspace` rỗng → guide `/switch-workspace` hoặc `/wiki-setup`.
4. Set `workspace_active = .workspace`, `effective_wiki_root = <resolved absolute>`, `{ws} = {effective_wiki_root}/workspaces/{workspace_active}/`.
5. Validate: `{ws}/workspace.md` tồn tại.

---

## Bước 1 — Resolve repo + scope  (single-repo mode — skip nếu `--bundle`)

1. `repo_path` = `--ref` (default `wiki_json_dir`).
2. Validate: `repo_path` có `.git/` HOẶC có 1 trong `pom.xml`/`package.json`/`build.gradle`/`Cargo.toml`/`go.mod`. Nếu không → STOP `not a recognized code repo at {path}`.
3. Resolve git info qua `git -C {repo_path} ...`:
   - `git_sha` = `git rev-parse HEAD` (full 40-char). Nếu repo không phải git → `git_sha = "unmanaged-{sha256-of-tree-manifest}"`.
   - `git_branch` = `git rev-parse --abbrev-ref HEAD`. Default `unmanaged` nếu không phải git.
   - `git_dirty` = check `git status --porcelain`. Nếu có uncommitted changes → WARN: "Working tree dirty. Snapshot sẽ chỉ chứa committed code (không bao gồm uncommitted). Tiếp tục?" (AskUserQuestion: Continue / Abort).
4. Resolve `code_scope`:
   - Nếu `--scope` set → dùng nguyên (split comma, expand glob khi enumerate file).
   - Nếu không → default heuristic list ở Input table.
   - Validate scope tồn tại tương đối `repo_path`. Cảnh báo nếu glob không match file nào.

---

## Bước 1.5 — Resolve bundle manifest  (bundle mode — skip nếu `--ref`)

1. `bundle_dir` = `--bundle` (absolute hoặc resolve relative tới `<cwd>`).
2. Validate: `bundle_dir/bundle.yaml` tồn tại. Nếu không → STOP, gợi ý `cp templates/bundle.yaml {bundle_dir}/bundle.yaml`.
3. Parse `bundle.yaml`:
   - `label` → `bundle_label`. Nếu `--label` passed → override.
   - `repos[]` → list `(path, role, name, scope)`.
   - `docs[]` → list `(path, type, label)`. Optional — empty nếu không có.
4. Validate từng repo trong `repos[]`:
   - Resolve path: relative → `bundle_dir/{path}`. Absolute → dùng nguyên. Follow symlinks.
   - Validate: có `.git/` HOẶC build file. Nếu không → STOP `not a recognized code repo at {resolved-path}`.
   - Nếu `name` bỏ trống → auto-detect từ `pom.xml`/`package.json`/`build.gradle` (giống Bước 2 logic).
5. Per-repo git info (lặp qua từng repo):
   - `git_sha` = full SHA (40-char).
   - `git_branch` = branch tên.
   - `git_dirty` = check uncommitted. Nếu dirty → WARN per-repo (KHÔNG block cả bundle). Ghi note.
6. Validate docs (nếu có):
   - Resolve path: relative → `bundle_dir/{path}`.
   - Validate: file tồn tại. Nếu không → WARN (không STOP) + skip doc đó.
7. Tổng hợp `bundle_repos` = list đầy đủ (name, resolved-path, role, git_sha, git_branch, scope).

---

## Bước 2 — Detect project name + label + evid-id

### Single-repo mode

1. **Project name detection** (theo thứ tự ưu tiên):
   - `pom.xml#artifactId` (qua grep XPath-like)
   - `package.json#name`
   - `build.gradle` `rootProject.name = ...`
   - `Cargo.toml#package.name`
   - `go.mod` module name
   - Fallback: basename của `repo_path`.
2. **Label**:
   - User pass `--label` → dùng.
   - Else: check `{ws}/evidence/_index.md` xem đã có evid-id `*-code-{project-slug}` chưa.
     - Chưa có → label = `"bootstrap-{project-name}"`.
     - Đã có → label = `"refresh-{project-name}-{YYYY-MM-DD}"`.
3. **evid-id** = `{YYYY-MM-DD}-code-{slug-from-label}`. Append `-{n}` nếu trùng entry trong `_index.md`.

### Bundle mode

1. **Label** = `bundle.yaml#label`. Override nếu user pass `--label`.
2. **evid-id** = `{YYYY-MM-DD}-platform-{slug-from-label}`. Append `-{n}` nếu trùng trong `_index.md`.
   - Note: prefix `platform` thay `code` để phân biệt trong `_index.md`. `source_type` vẫn là `code`.
3. **Per-repo names**: đã được resolve ở Bước 1.5.

---

## Bước 3 — Confirm preview

### Single-repo mode — in:
```
📋 Snapshot preparation

  Workspace : {workspace_active}
  Repo      : {repo_path}
  Project   : {project-name}
  Branch    : {git_branch}
  Git SHA   : {git_sha[..7]}{... if dirty: " (working tree DIRTY — snapshot of HEAD only)"}
  Scope     :
    - {scope-1}
    - {scope-2}
    ...
  Evid-id   : {evid-id}
  Label     : {label}

Sẽ tạo:
  {ws}/evidence/sources/{evid-id}/
    ├── source.yaml
    ├── raw.md            ← snapshot metadata theo templates/code-snapshot.md
    └── raw.normalized.md ← (auto if raw.md > 50KB)

Sau ingest sẽ {auto-run /evidence-analyze | dừng (--skip-analyze)}.

Tiếp tục? (yes/edit-scope/cancel)
```

### Bundle mode — in:
```
📦 Platform Bundle Snapshot

  Workspace : {workspace_active}
  Bundle    : {bundle_dir}/bundle.yaml
  Label     : {bundle_label}
  Evid-id   : {evid-id}

  Repos ({n}):
  | # | Name                | Role       | Branch | SHA     | Dirty |
  |---|---------------------|------------|--------|---------|-------|
  | 1 | core-framework      | framework  | main   | a1b2c3d | no    |
  | 2 | shared-libs         | shared-lib | main   | def5678 | yes ⚠ |
  | 3 | sample-project      | application| main   | ghi9012 | no    |

  Docs ({m}):
  | # | Label                        | Type         |
  |---|------------------------------|--------------|
  | 1 | Platform Architecture        | architecture |

Sẽ tạo:
  {ws}/evidence/sources/{evid-id}/
    ├── source.yaml      (code_repos: [{n} repos], include_docs: [{m} docs])
    ├── raw.md            ← Section 0 overview + per-repo Sections 1–9 + Docs section
    └── raw.normalized.md ← (auto if raw.md > 50KB)

Sau ingest sẽ {auto-run /evidence-analyze | dừng (--skip-analyze)}.

Tiếp tục? (yes/edit-bundle/cancel)
```

Wait user. `edit-scope`/`edit-bundle` → quay lại Bước 1/1.5. `cancel` → STOP.

---

## Bước 4 — Build snapshot (raw.md)

> Đây là phần "ingest" tương đương `/evidence-ingest --source code` Bước 2 nhánh `code`.

### Single-repo mode

Tạo `{ws}/evidence/sources/{evid-id}/raw.md` từ `templates/code-snapshot.md`. Fill 10 section theo [code-snapshot-conventions.md Section 4](../../agents/pipeline/code-snapshot-conventions.md):

### 4.1 — Section 1 (Project metadata)
- name, version, build tool, language(s), top-level dirs (1 cấp), README excerpt (≤ 200 chars).
- Cite: `(pom.xml:L..)` / `(package.json:L..)` / `(README.md:L..)`.

### 4.2 — Section 2 (Dependencies)
- Parse `pom.xml` `<dependencies>` thành table production/test/build.
- Parse `package.json` `dependencies`/`devDependencies`.
- Parse `build.gradle` `dependencies { ... }` block.
- Parse `Cargo.toml` `[dependencies]`, `[dev-dependencies]`.
- Parse `go.mod` `require ( ... )` block.
- Mỗi row: `{group} | {artifact} | {version} | {purpose-heuristic} | (file:L..)`.

### 4.3 — Section 3 (Configs)
Guard đầy đủ: [code-snapshot-conventions.md Section 6](../../agents/pipeline/code-snapshot-conventions.md).

**Mặc định: tất cả config bị block.** Chỉ đọc khi user pass `--allow-configs`.

**4.3.1 — Kiểm tra flag**:
- `--allow-configs` KHÔNG set → ghi `_(Configs skipped — rerun with --allow-configs to include)_` vào Section 3. Kết thúc.
- `--allow-configs` set → tiếp tục 4.3.2.

**4.3.2 — Hard blocklist check** (load engine list + `{ws}/evidence/STORAGE.md` nếu có):
- Khớp → ghi `[HARD-BLOCKED: {path}]`, không đọc. Hard-blocked không có flag nào bypass.
- Ví dụ blocked: `.env`, `*-prod.yaml`, `*.key`, `*.pem`, `vault.yaml`, file trong `secrets/`.

**4.3.3 — AskUserQuestion** (file còn lại sau blocklist):
- Hiển thị danh sách + hỏi user chọn file nào include (format tại `code-snapshot-conventions.md Section 6.1 Tầng 2`).
- File không được chọn → `[SKIPPED-BY-USER: {path}]`.

**4.3.4 — Redact** (file user đã chọn):
- Tokens, passwords, keys, URLs có credentials → `<REDACTED-SECRET>`.
- Cite: `(path/to/file.yaml:L..-L..)`.

**4.3.5 — Guard log** (cuối Section 3):
- Append "Config guard log": included / hard-blocked / skipped-by-user.

### 4.4 — Section 4 (REST endpoints)
- Grep markers theo `code-snapshot-conventions.md` Section 9 (workspace có thể bổ sung qua `{ws}/evidence/STORAGE.md`).
- Default Java/Spring: `@RestController`, `@RequestMapping`, `@GetMapping`/`@PostMapping`/`@PutMapping`/`@DeleteMapping`, `@PatchMapping`.
- Default Node/Express: `app\.(get|post|put|delete|patch)\(`, `router\.(...)`.
- NestJS: `@Controller`, `@Get`, `@Post`, ...
- FastAPI: `@app\.(get|post)`, `@router\.(...)`.
- Build table: `Method | Path | Handler class:method | Auth | Citation`.

### 4.5 — Section 5 (Message consumers)
- Kafka: `@KafkaListener`, `@StreamListener`, KafkaConsumer instantiation.
- MQTT: subscribe calls (Paho `client.subscribe`, Mosquitto, etc).
- RabbitMQ: `@RabbitListener`.
- SQS: `@SqsListener`.
- Build table per protocol: `Topic | Group/Queue | Consumer class:method | Citation`.

### 4.6 — Section 6 (Services & components)
- `@Service`, `@Component`, `@Repository`, `@Configuration`.
- TypeScript/Nest: `@Injectable`, `@Module`.
- Build table: `Class FQN | Stereotype | Responsibility (1 line) | Citation`.
- Responsibility 1-line: từ class-level Javadoc nếu có; else `_(no doc)_`.

### 4.7 — Section 7 (DB schema)
- Entities: `@Entity`, `@Table`.
- Repositories: interfaces extends `JpaRepository`/`CrudRepository`.
- Migrations: list file trong `src/main/resources/db/migration/` (Flyway) hoặc `liquibase/` (Liquibase) hoặc `migrations/` (general).
- 2 sub-table: Entities + Migrations.

### 4.8 — Section 8 (Public APIs)
- Class signatures package-public (top-level interfaces, public classes có annotation `@PublicApi` hoặc tương đương).
- Code block 1 cho mỗi class quan trọng (≤ 5 class chính). KHÔNG dump toàn bộ method body — chỉ signature.

### 4.9 — Section 9 (Git summary)
- Last 50 commits oneline: `git -C {repo_path} log --oneline -50` → trim sha tới 7 chars.
- Top contributors: `git -C {repo_path} shortlog -sn --since="1 year ago"` → top 10 rows.
- Notable commits: grep commits 1 năm gần nhất chứa từ khóa `^(feat|fix|chore|refactor)` HOẶC `(decided|chose|switched|deprecat|migrat|drop)`. Limit 20.
- **Redact emails**: tên giữ, email → `<REDACTED-EMAIL>`.

### 4.10 — Redaction post-pass

Sau khi build raw.md xong, BẮT BUỘC scan toàn file cho pattern:
- `password\s*[:=]\s*[^<\s]+` → replace value bằng `<REDACTED-SECRET>`
- `(token|api[-_]?key|secret|jwt[-_]?key)\s*[:=]\s*[^<\s]+` → idem
- Email regex `[\w.+-]+@[\w-]+\.[\w.-]+` ngoài Section 1 README excerpt → `<REDACTED-EMAIL>`
- Internal hostnames có credentials inline (`https?://\w+:\w+@`) → `<REDACTED-URL>`

Nếu **vẫn còn** match sau redaction (ngoại lệ user chấp nhận) → STOP với error `SECRET DETECTED IN SNAPSHOT — fix manually before continue`.

### Bundle mode

Tạo `raw.md` theo cấu trúc bundle (xem [code-snapshot-conventions.md Section 4 — Bundle mode](../../agents/pipeline/code-snapshot-conventions.md)):

**4.B.1 — Section 0 (Bundle overview)**:
```markdown
# Platform Bundle: {bundle_label}
Generated: {YYYY-MM-DD} | Repos: {n} | Docs: {m}

## Section 0: Bundle Overview
| Repo | Role | Project | Branch | Git SHA | Dirty |
...
```

**4.B.2 — Per-repo loop** (lặp qua từng entry trong `bundle_repos`):
- Mỗi repo chạy đúng Bước 4.1–4.9 như single-repo.
- Mỗi section thêm suffix `[{repo-name}]` vào heading: `### Section 1: Project Metadata [{repo-name}]`.
- Citations dùng prefix: `({repo-name}/path/to/file:L..-L..)`.
- Git commands dùng `git -C {repo.resolved_path} ...`.

**4.B.3 — Docs section** (nếu `include_docs` non-empty):
```markdown
## Docs

### Doc: {doc.label} [{doc.type}]
> Source: ({relative-path-from-bundle-dir}:L1-L{last})

{Key excerpts, ≤ 800 chars total, trích phần quan trọng nhất (vd headings, decision statements, endpoint list)}
(docs/{filename}:L..-L..)
```

**4.B.4 — Redaction post-pass**: áp dụng giống 4.10 cho toàn bộ raw.md bundle.

**4.B.5 — Size check bundle**: sau redaction, nếu `raw.md > 300KB` → STOP, gợi ý hẹp scope từng repo trong `bundle.yaml`.

---

## Bước 5 — Compute size + sha256 + write source.yaml

1. `raw_size_bytes` = size of `raw.md`.
2. `sha256` = SHA256 of `raw.md` content.
3. Đọc `{ws}/evidence/_index.md`. Search sha256:
   - Trùng → STOP `Duplicate snapshot detected. Existing evidence: {old-evid-id}. Codebase chưa thay đổi (cùng git_sha + scope) — reuse evid-id cũ.`
4. Build `source.yaml` từ `templates/evidence-source.yaml`:

   **Single-repo**:
   - `source_type: code`
   - `origin: "code:{project-slug}@{git_sha[..7]}"`
   - `git_sha`, `git_branch`, `code_scope`, `code_repo_path`
   - `workspace_at_ingest: {workspace_active}`  ← CRITICAL (I-2)
   - `notes: "Redacted: secrets in configs, contributor emails. {... if dirty: 'Snapshot at HEAD — uncommitted changes excluded.'}"`

   **Bundle mode**:
   - `source_type: code`
   - `origin: "platform-bundle:{bundle_label}@{YYYY-MM-DD}"`
   - `code_repo_path: {bundle_dir}` (bundle root — audit chỉ)
   - `code_scope: null` (scope per-repo nằm trong `code_repos[].scope`)
   - `code_repos: [{name, path, role, git_sha, git_branch, scope}, ...]`
   - `include_docs: [{path, type, label}, ...]` (nếu có)
   - `workspace_at_ingest: {workspace_active}`  ← CRITICAL (I-2)
   - `notes: "Bundle: {n} repos. Redacted: secrets in configs, contributor emails. {dirty repos note}"`

5. Nếu `raw.md > 50KB` → tạo `raw.normalized.md` chunked theo Section heading (xem [raw-storage-conventions.md Section 2](../../agents/pipeline/raw-storage-conventions.md)). Set `normalized: true`.
6. Size STOP threshold: **200KB** (single-repo) | **300KB** (bundle). Hint user thu hẹp scope.

---

## Bước 6 — Update _index.md

Append row vào `{ws}/evidence/_index.md` Active table:
```markdown
| {evid-id} | code | {label} | ingested | {date} | {date} | — | — |
```

---

## Bước 7 — Trigger evidence-analyze (nếu không `--skip-analyze`)

Invoke logic của `/evidence-analyze --id {evid-id}` (xem [evidence-analyze.md](evidence-analyze.md) Bước 2 nhánh code pipeline):
- Run CORE-CODE: `c01-tech-stack.md`, `c02-service-map.md`, `c03-pattern-proposals.md`, `c04-contract-proposals.md`, `08-knowledge-gaps.md`, `04-questions.md`.
- Nếu user pass `--with-drafts` → thêm `--prompt c05 --prompt c06 --prompt c07`.
- State transition: `ingested → analyzed`.

---

## Bước 8 — Confirm

### Single-repo confirm:
```
✅ Code snapshot ingested + analyzed — {evid-id}
   Workspace : {workspace_active}
   Repo      : {repo_path} @ {git_sha[..7]}
   Snapshot  : {ws}/evidence/sources/{evid-id}/raw.md ({raw_size_bytes} bytes)
   Analysis  : {ws}/evidence/analysis/{evid-id}/
                 ├── c01-tech-stack.md
                 ├── c02-service-map.md
                 ├── c03-pattern-proposals.md  ({N} proposals: {n_new} NEW, {n_ext} EXTENDS, {n_dup} DUPLICATE)
                 ├── c04-contract-proposals.md ({N} proposals)
                 ├── 08-knowledge-gaps.md      ({N_blocking} blocking, {N_nice} nice)
                 └── 04-questions.md           ({N_p0} P0, {N_p1} P1, {N_p2} P2, {N_p3} P3)
   {... if --with-drafts:
   Drafts    : c05-service-drafts.md ({N} services), c06-decision-drafts.md ({N} ADRs), c07-config-overrides.md
   }

Highlights:
  - {top 3 blocking gaps tóm tắt}

Next:
  /evidence-qa --id {evid-id}                                    → confirm proposals via Q&A
  /evidence-apply --id {evid-id} --mode update --dry-run        → preview file create/edit khi qa_done
```

### Bundle mode confirm:
```
✅ Platform bundle ingested + analyzed — {evid-id}
   Workspace : {workspace_active}
   Bundle    : {bundle_dir}/bundle.yaml
   Repos ({n}):
     core-framework @ a1b2c3d [framework]
     shared-libs    @ def5678 [shared-lib]
     sample-project @ ghi9012 [application]
   Docs ({m}): Platform Architecture [architecture]
   Snapshot  : {ws}/evidence/sources/{evid-id}/raw.md ({raw_size_bytes} bytes)
   Analysis  : {ws}/evidence/analysis/{evid-id}/
                 ├── c01-tech-stack.md
                 ├── c02-service-map.md          ({N_s} services, {N_cross} cross-repo)
                 ├── c03-pattern-proposals.md    ({N} proposals: {n_new} NEW, {n_cross} CROSS-REPO)
                 ├── c04-contract-proposals.md   ({N} proposals, {n_inc} CROSS-REPO-INCONSISTENCY)
                 ├── 08-knowledge-gaps.md        ({N_blocking} blocking, {N_nice} nice)
                 └── 04-questions.md             ({N_p0} P0, {N_p1} P1, {N_p2} P2, {N_p3} P3)

Highlights:
  - {top 3 blocking gaps / cross-repo findings tóm tắt}

Next:
  /evidence-qa --id {evid-id}                                    → confirm proposals via Q&A
  /evidence-apply --id {evid-id} --mode update --dry-run        → preview file create/edit khi qa_done
```

---

## Khi nào nên chạy

**Single-repo (`--ref`)**:
- **Bootstrap wiki cho codebase mới onboard**: workspace mới setup, hoặc codebase chuyển sang dùng wiki lần đầu, services chưa có doc nào.
- **Refresh sau major change**: codebase tiến hóa lớn (thêm service, đổi framework, refactor architecture) — chạy lại để wiki không drift xa code.
- **Sau khi pull legacy repo**: muốn wiki phản ánh code thực tế thay vì viết tay từ đầu.

**Bundle mode (`--bundle`)**:
- **Bootstrap platform tổng thể**: onboard workspace mới cần phân tích đồng thời core framework + shared libs + sample projects để phát hiện cross-repo patterns.
- **Platform-wide pattern audit**: muốn so sánh convention giữa nhiều repo (Kafka topic naming, REST versioning, retry policy) — cross-repo inconsistency findings tự động.
- **Khi có tài liệu mô tả đi kèm**: architecture doc, OpenAPI spec — muốn analysis prompts cross-ref code với doc trong cùng 1 evidence.
- **Chuẩn bị**: copy `templates/bundle.yaml` → điền repos + docs → `/code-analyze --bundle <dir>`.

**KHI KHÔNG NÊN**:
- Chỉ thay đổi 1-2 file lẻ → dùng `/update-wiki` direct (không cần evidence flow).
- Codebase nhỏ < 5 file → viết tay nhanh hơn.
- Cần verify với external doc (Confluence, ticket) chứ không phải code → dùng `/evidence-ingest --source paste|api|mcp`.
- Bundle với nhiều repo lớn và scope rộng → raw.md vượt 300KB. Thu hẹp scope từng repo trong `bundle.yaml`.

---

## Common errors

| Error                                       | Fix                                                            |
|---------------------------------------------|----------------------------------------------------------------|
| Not a recognized code repo                  | Pass đúng `--ref {repo-path}` có `.git/` hoặc build file       |
| Working tree dirty                          | Commit/stash trước, hoặc accept warning để snapshot HEAD       |
| `raw.md > 200KB` (single-repo)              | Thu hẹp `--scope` (ignore generated code, vendor, build output) |
| `raw.md > 300KB` (bundle)                   | Thu hẹp `scope` từng repo trong `bundle.yaml`; hoặc bỏ bớt repo |
| `bundle.yaml` not found                     | Copy `templates/bundle.yaml` vào thư mục staging, điền repos   |
| Repo in bundle not recognized               | Check path trong `bundle.yaml`; resolve symlinks nếu cần       |
| Section 3 trống / no config data            | Thêm `--allow-configs` để cho phép đọc config (sẽ hỏi confirm) |
| Config file bị hard-block không mong muốn  | Xem hard blocklist tại `code-snapshot-conventions.md Section 6` |
| Duplicate snapshot (same sha256)            | Reuse evid-id cũ; codebase chưa thay đổi đáng kể               |
| `SECRET DETECTED IN SNAPSHOT`               | Manual edit raw.md xóa secret, rerun ingest với evid-id mới    |
| Workspace lock fail                         | `/switch-workspace {workspace}` về workspace_at_ingest          |
