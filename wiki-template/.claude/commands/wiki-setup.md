# Wiki Setup

Tạo hoặc cập nhật file `.claude/wiki.json` cho dự án (codebase) hiện tại.
Chạy một lần khi bắt đầu tích hợp wiki vào một dự án mới — hoặc lại khi đổi workspace.

> Wiki được tổ chức theo workspace. Mỗi codebase phải khai báo `workspace` để biết đọc knowledge từ `{wiki_root}/workspaces/{workspace}/`.

---

## Bước 1 — Đọc global config

Đọc `~/.claude/wiki-global.json`.

Nếu file không tồn tại → dừng và hướng dẫn:
```
⚠️  Chưa có global config ~/.claude/wiki-global.json.

Cách nhanh nhất (recommended) — chạy installer:
   bash {wiki-template-root}/scripts/install-to-claude.sh

  Script này:
  - Tự dò wiki_root từ vị trí script
  - Sync slash commands + subagents về ~/.claude/{commands,agents}/
  - Tạo ~/.claude/wiki-global.json với wiki_root đã set sẵn
  - Idempotent: chạy lại để cập nhật khi pull wiki-template mới

Cách thủ công (nếu không dùng được bash):
  1. Copy {wiki-template-root}/templates/wiki-global.json → ~/.claude/wiki-global.json
  2. Sửa "wiki_root" thành absolute path tới wiki-template repo
  3. Chạy lại /wiki-setup
```

Nếu main agent đang chạy chính từ trong wiki-template repo (detect: cwd hoặc wiki_json_dir trùng vị trí của script `scripts/install-to-claude.sh`) → có thể offer chạy `bash scripts/install-to-claude.sh` luôn cho user (chỉ chạy sau khi user confirm; KHÔNG tự chạy).

Lấy `wiki_root` (và optional `default_workspace`, `default_domain`) từ global config.

---

## Bước 2 — Kiểm tra .claude/wiki.json hiện tại

Nếu `.claude/wiki.json` đã tồn tại → đọc và hỏi:
```
⚠️  .claude/wiki.json đã tồn tại với config:
   workspace: {workspace}
   project  : {project}
   domain   : {domain}

Tiếp tục sẽ cập nhật file này. Giữ nguyên các giá trị không thay đổi.
```
Tiếp tục với giá trị hiện có làm default.

---

## Bước 3 — Chọn workspace

Liệt kê `{wiki_root}/workspaces/*/workspace.md` để hiển thị options.

Thứ tự pre-fill:
1. Giá trị `workspace` trong `.claude/wiki.json` cũ (nếu có).
2. `default_workspace` từ `~/.claude/wiki-global.json`.
3. Workspace duy nhất nếu chỉ có 1.
4. Hỏi user (AskUserQuestion với danh sách workspace + option "Tạo mới" → trigger `/new-workspace`).

Validate: `{wiki_root}/workspaces/{workspace}/workspace.md` tồn tại. Nếu không → STOP, gợi ý `/new-workspace {name}` trong wiki-template.

Set biến `{ws} = workspaces/{workspace}/` cho các bước sau (paths relative tới `wiki_root`).

---

## Bước 4 — Scan dự án để pre-fill

Chạy detection tương tự `/wiki-detect` nhưng scope vào `{wiki_root}/{ws}/`:

**Detect project name:**
- Đọc `pom.xml` → `<artifactId>`
- Đọc `package.json` → `name`
- Fallback: tên thư mục gốc của dự án

**Detect components từ dependencies:**
- `spring-kafka` / `kafka-clients` → patterns: thêm `kafka-event-processing` (nếu tồn tại trong `{ws}/platform/patterns/`)
- `eclipse-paho` / mqtt libs → patterns: thêm `mqtt-routing`, contracts: thêm `mqtt-topic-contract` (nếu tồn tại trong `{ws}/platform/contracts/`)
- `spring-data` / `jpa` → component: `db` (ghi chú, không thêm vào config)

> Nếu pattern/contract đề xuất KHÔNG tồn tại trong workspace → đánh dấu `(missing — sẽ cần /update-wiki tạo trong workspace {workspace})` thay vì im lặng skip.

**Detect domain từ package names:**
- Tìm thư mục/package trùng tên với `{wiki_root}/{ws}/domains/*/`
- Ví dụ: package `com.example.surgery` → domain: `surgery`

**Detect knowledge_map:**
- Tìm trong `{wiki_root}/{ws}/projects/` có tên gần với project name
- Ví dụ: project `surgery-service` → `projects/surgery-service/knowledge-map.md`

**Detect service docs:**
- Tìm trong `{wiki_root}/{ws}/projects/{project}/services/` tất cả file `.md`

---

## Bước 5 — Hiển thị preview và xác nhận

Hiển thị config sẽ được tạo:
```
📝 Chuẩn bị tạo .claude/wiki.json:

{
  "project"       : "surgery-service",
  "workspace"     : "example-surgery",
  "wiki_root"     : null,              ← dùng ~/.claude/wiki-global.json
  "knowledge_map" : "projects/surgery-service/knowledge-map.md",
  "domain"        : "surgery",
  "patterns"      : ["kafka-event-processing", "mqtt-routing"],
  "contracts"     : ["mqtt-topic-contract"],
  "services"      : ["projects/surgery-service/services/kafka-consumer.md"]
}

Lưu ý:
- Mọi path (knowledge_map, services) đều relative đến {wiki_root}/workspaces/{workspace}/
- Nếu cần điều chỉnh, hãy cho biết trước khi ghi file.

Gõ "ok" hoặc mô tả thay đổi cần thiết.
```

Chờ user xác nhận hoặc yêu cầu chỉnh sửa.

---

## Bước 6 — Ghi file .claude/wiki.json

Sau khi user xác nhận, ghi file `.claude/wiki.json` vào thư mục gốc dự án.

Xác nhận:
```
✅ Đã tạo .claude/wiki.json
   Workspace: {workspace}
   Project  : {project}

Bước tiếp theo:
  /use-wiki      → bắt đầu làm task với context từ wiki workspace {workspace}
  /wiki-detect   → kiểm tra lại config vừa tạo
  /code-analyze  → bootstrap wiki từ codebase này (sinh patterns/services/decisions từ source code)
```

---

## Bước 7 — Kiểm tra knowledge-map trong wiki

Sau khi ghi file, kiểm tra `knowledge_map` đã trỏ vào file thực tế chưa:

Nếu `{wiki_root}/{ws}/{knowledge_map}` không tồn tại:
```
⚠️  knowledge-map chưa có trong workspace {workspace}.

Để tạo:
1. Copy {wiki_root}/templates/service.md (làm starting point)
2. Lưu vào {wiki_root}/workspaces/{workspace}/projects/{project}/knowledge-map.md
3. Điền thông tin cho dự án này — link tới patterns/contracts trong CÙNG workspace
4. Chạy /wiki-detect để verify
```

---

## Bước 8 — (Optional) Set global default

Hỏi user: "Set `default_workspace = {workspace}` trong `~/.claude/wiki-global.json`? (để các codebase mới chưa có `.claude/wiki.json` mặc định dùng workspace này)"

Nếu Yes → cập nhật field `default_workspace` trong global config.

---

## Bước 8.5 — (Optional) Bootstrap wiki từ codebase

Hỏi user qua AskUserQuestion: "Run `/code-analyze` ngay bây giờ để bootstrap wiki từ codebase này?"

**Recommended `Yes`** khi:
- Workspace mới tạo (workspace.md vừa được khởi tạo, chưa có service docs nào)
- `{ws}/projects/{project}/services/` rỗng hoặc thiếu service docs cho project hiện tại
- `{ws}/platform/patterns/` thiếu pattern phổ biến cho stack đã detect (vd Kafka detected nhưng không có pattern Kafka)
- User mới onboard codebase legacy

**Recommended `No`** khi:
- `{ws}/projects/{project}/services/` đã đầy đủ và up-to-date
- User chỉ muốn `.claude/wiki.json` để start làm task ngay, sẽ analyze sau

Nếu `Yes`:
- Invoke logic `/code-analyze` (xem [code-analyze.md](code-analyze.md)). Default scope, label = `bootstrap-{project}`.
- Sau khi xong → suggest `/evidence-qa --id {evid-id}` để confirm proposals.

Nếu `No`:
- Skip — user có thể chạy `/code-analyze` bất kỳ lúc nào sau này.

---

## Khi nào nên chạy

- Lần đầu tiên bắt đầu làm việc với dự án mới
- Khi join công ty mới → tạo workspace mới (`/new-workspace`) rồi chạy `/wiki-setup` cho từng codebase
- Khi thêm component mới vào dự án (Kafka, MQTT, domain mới)
- Khi wiki được di chuyển sang đường dẫn mới (`wiki_root` thay đổi)
- Khi chuyển project sang workspace khác (đổi `workspace` field)
