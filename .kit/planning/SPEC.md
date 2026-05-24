# SPEC: Personal LLM-Wiki System (zwiki)

Status: locked
Input Type: new-spec
Lane: normal
Risk Flags: data-model, multi-domain, external-systems, cross-platform
Affected Surfaces: worker, docs
Downstream: plan full
Updated At: 2026-05-24 (MVP-1 consolidated — binary distribution + project integration)

---

## MVP-1 Definition

**Scope**: Full pipeline, end-to-end functional.

### In MVP-1
- Evidence pipeline: 4 stages (ingest → analyze → qa → apply)
- Retrieval pipeline: 3 stages (/ask with qmd BM25 search)
- Workspace isolation: dual enforcement (file-level + qmd collection)
- CLI commands: `zwiki setup`, `zwiki init`, `zwiki workspace create`
- Slash commands: /ask, /learn, /reflect, /workspace, /reindex
- Binary distribution: Bun compile → single executable
- 4 workspaces scaffolded: programming, finance, health, philosophy
- qmd MCP integration: BM25 only, no models
- Project integration: symlink + CLAUDE.md rules injection

### NOT in MVP-1
- Web UI / React frontend
- Vector search / hybrid mode / LLM reranking
- Auto-sync external sources (Notion, Obsidian)
- Real-time file watcher (manual /reindex)
- Cross-workspace references
- Mobile app
- Collaboration features
- npm package publish (binary only)

---

## Goal

Xây dựng hệ thống quản lý kiến thức cá nhân (personal knowledge management) phục vụ AI agents, với khả năng:

1. Tổ chức kiến thức theo domains isolated (tránh knowledge bleed)
2. Learning pipeline từ raw notes → verified knowledge
3. **BM25 retrieval pipeline** (qmd search) để LLM truy xuất context trước khi trả lời — keyword search, không cần local models
4. Citation requirement — mọi fact phải trace được về source

---

## Users / Actors

- **Primary**: AI agents (Claude Code) — execute tasks, retrieve knowledge, generate content within workspace constraints
- **Secondary**: Human (technical engineer, cross-stack) — decision-maker at gates:
  - Task definer: cung cấp questions/tasks cho agents
  - Knowledge gap resolver: trả lời khi agent thiếu knowledge
  - Evidence QA reviewer: trả lời P0/P1/P2/P3 questions trong `/learn` batches
  - Constraint conflict resolver: quyết định khi constraints conflict
  - Final approver: review agent output trước khi apply vào wiki

---

## Requirements

### Core

1. **Workspace isolation**: Mỗi domain (programming, finance, health, philosophy) là 1 workspace riêng biệt. Knowledge từ workspace A KHÔNG được dùng khi làm việc trong workspace B
2. **3-layer knowledge hierarchy**: Axioms (core facts, highest priority) > Mental Models (reusable frameworks) > Projects (books, courses, experiments)
3. **Evidence pipeline (4 stages)**: ingest → analyze → qa → apply
4. **BM25 retrieval pipeline (3 stages)**: parse intent → qmd BM25 search → answer using context (no local models)
5. **Citation requirement**: Mọi entry trong wiki phải có source traceability (book, article, experience, evidence ID)

### CLI Commands

6. `zwiki setup` — first-run: tạo `~/.zwiki/`, extract bundled assets (engine, templates, commands, agents), kiểm tra qmd dependency
7. `zwiki init` — project integration: interactive CLI (clack) chọn workspace + inject targets vào project hiện tại
8. `zwiki workspace create {name}` — tạo workspace mới từ template
9. `zwiki update` — re-extract bundled assets khi update binary version mới

### Slash Commands (Claude Code)

10. `/ask "question"` — retrieve + answer (triggers 3-stage retrieval pipeline)
11. `/learn <source>` — ingest raw notes vào evidence pipeline
12. `/learn --analyze {id}` — run 4 analysis prompts
13. `/learn --qa {id}` — batch Q&A → verified-facts.md
14. `/learn --apply {id}` — apply verified facts → update wiki + auto reindex
15. `/reflect` — trigger evidence pipeline sau khi đọc/học xong
16. `/workspace {name}` — switch active workspace
17. `/reindex` — trigger qmd reindex cho active workspace

### Infrastructure

18. **Active workspace resolution**: `<cwd>/.claude/zwiki.json` > `~/.zwiki/config.yml` > single workspace auto-detect > STOP
19. **Templates**: workspace, axiom, mental-model, project, evidence-index, evidence-source
20. **qmd as retrieval backend**: BM25 keyword search only (SQLite FTS5), MCP integration. Không dùng vector search hay LLM reranking

### Invariants

21. **I-1 Immutable sources**: `sources/{id}/raw.md` và `source.yaml` không được modify sau khi ingest
22. **I-2 Workspace lock**: `source.yaml#workspace_at_ingest` phải match active workspace ở mọi transition
23. **I-3 QA gate**: Apply STOP nếu P0/P1 questions marked `awaiting_external` hoặc `deferred`
24. **I-4 Citation**: Mọi entry trong `verified-facts.md` phải cite source question ID + affected wiki path
25. **I-5 Resumable**: `checkpoint.json` cho phép resume `/learn --apply` từ file bất kỳ nếu interrupted
26. **I-6 Workspace-scoped search**: qmd query PHẢI scoped tới active workspace collection. Cross-workspace search = violation

---

## Boundaries

### In Scope

- CLI binary: Bun-compiled TypeScript, interactive prompts (clack)
- Engine layer: system prompts, retrieval rules, evidence pipeline rules
- qmd integration: config, collection setup, MCP server, post-filter logic
- Templates: workspace, axiom, mental-model, project
- Slash commands + subagents for Claude Code
- Project integration: `zwiki init` → symlink commands/agents + inject CLAUDE.md rules
- 4 initial workspaces scaffolded
- Documentation: architecture overview, workflow guide (Vietnamese)

### Out of Scope

- Web UI (CLI/markdown-first)
- Multi-agent coordination (plan-reviewer, code-reviewer)
- Code validation pipeline
- Auto-sync với external sources
- Mobile app
- Collaboration features
- Custom qmd embedding model training
- Real-time file watcher
- npm package publish

---

## Tech Stack

```yaml
Runtime: Bun (build + dev)
Language: TypeScript 5.4+
Search: qmd (@tobilu/qmd) — BM25 keyword search (SQLite FTS5, no local models)

Core Dependencies:
  @tobilu/qmd: latest      # BM25 search (external, install riêng)
  commander: ^12.0.0        # CLI framework
  @clack/prompts: latest    # Interactive CLI (zwiki init/setup)
  js-yaml: ^4.1.0           # YAML parser (config.yml)
  marked: ^12.0.0           # Markdown parser
  zod: ^3.22.0              # Schema validation

Dev Dependencies:
  typescript: ^5.4.0
  vitest: ^1.4.0
  @types/bun: latest

Package Manager: bun
Build: bun build --compile (single binary per platform)
Release: GitHub Releases (zwiki-darwin-arm64, zwiki-linux-x64, zwiki-win-x64)
```

### qmd Mode: BM25 Only

- Không cần download models (~0 disk, ~10MB RAM)
- Dùng `qmd search` (SQLite FTS5) — keyword matching, prefix search, BM25 ranking
- Không dùng `qmd vsearch` (vector) hay `qmd query` (hybrid)
- Chỉ cần `qmd index`, KHÔNG cần `qmd embed`

---

## Constraints

### Technical

- File-based storage (markdown + YAML frontmatter)
- Config format: YAML (`~/.zwiki/config.yml`)
- Compatible với Claude Code workflow (slash commands via `.claude/commands/`)
- qmd MCP server cho BM25 retrieval (no models)
- UTF-8 encoding, support Vietnamese + English
- Git-friendly (plain text, diffable)
- Binary distribution — no Node.js required on target machine
- qmd still requires separate install (npm i -g @tobilu/qmd)

### UX

- **All CLI commands MUST use `@clack/prompts` style** — intro/outro, spinners, select/multiselect, notes, grouped prompts. No raw `console.log` or plain `process.stdout`. This applies to: `zwiki setup`, `zwiki init`, `zwiki workspace create`, `zwiki update`, and any future CLI commands.
- Expected UX per command:
  - `zwiki setup`: intro → spinner (extracting assets) → spinner (checking qmd) → note (summary) → outro
  - `zwiki init`: intro → select (workspace) → multiselect (inject targets) → spinner (creating symlinks) → note (created files) → outro
  - `zwiki workspace create`: intro → text (name) → confirm → spinner (scaffolding) → outro
  - `zwiki update`: intro → spinner (extracting) → note (what changed) → outro

### Product

- Learning-first: evidence pipeline là core workflow
- Simplicity over features: 3-stage retrieval thay vì 5-stage
- BM25 search > deterministic rules
- `zwiki init` phải non-destructive: không overwrite existing CLAUDE.md content, chỉ append

---

## Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        zwiki                                 │
│                                                              │
│  BINARY (Bun compiled)              ~/.zwiki/ (runtime)      │
│  ┌──────────────────┐     setup     ┌───────────────────┐   │
│  │ CLI + bundled     │ ──extract──▶ │ engine/            │   │
│  │ assets            │              │ templates/         │   │
│  └──────────────────┘              │ commands/          │   │
│                                     │ agents/            │   │
│         zwiki init                  │ workspaces/        │   │
│              │                      │ config.yml         │   │
│              ▼                      └────────┬──────────┘   │
│  ┌──────────────────┐                        │               │
│  │ Project .claude/  │              ┌────────▼──────────┐   │
│  │ zwiki.json        │              │ qmd (MCP Server)   │   │
│  │ commands/ (symlink)│              │ BM25 / SQLite FTS5 │   │
│  │ agents/ (symlink)  │              │ ~/.zwiki/.cache/   │   │
│  │ CLAUDE.md (rules)  │              └───────────────────┘   │
│  └──────────────────┘                                        │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Claude Code Harness                      │   │
│  │  /ask  /learn  /reflect  /workspace  /reindex         │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Retrieval Pipeline (`/ask`) — 3 Stages

```
[Stage 1] /ask "question"
   ↓ wiki-planner subagent: parse question → search keywords
   ↓ Output: { workspace, keywords, domain }

[Stage 2] wiki-qmd-selector subagent
   ↓ qmd MCP search: { query: keywords, collection: active_workspace, limit: 20 }
   ↓ Post-filter: classify by path prefix → re-rank by priority tier
   ↓   P0 axioms → P1 mental-models → P2 projects → P3 decisions
   ↓   (within tier: keep BM25 score ordering)
   ↓ Fetch full body for top 6-8 results (qmd.get)
   ↓ Write ranked results → current-task.md

[Stage 3] Main agent (Builder)
   ↓ Read current-task.md → answer using retrieved context
   ↓ If knowledge gaps → STOP, report gaps, KHÔNG đoán
```

### Evidence Pipeline (Learning Workflow) — 4 Stages

```
[ingest] /learn <source>
   ↓ Create sources/{id}/raw.md + source.yaml (immutable, workspace_at_ingest locked)
   ↓ Update _index.md state: ingested

[analyze] /learn --analyze {id}
   ↓ Run 4 CORE prompts → 01-summary, 02-contradiction, 04-questions, 08-gaps
   ↓ Update _index.md state: analyzed

[qa] /learn --qa {id}
   ↓ Batch P0/P1/P2/P3 questions → human answers → batch-N-answers.md (append-only)
   ↓ Generate verified-facts.md (must cite question ID + wiki path)
   ↓ Update _index.md state: qa_done

[apply] /learn --apply {id}
   ↓ Read verified-facts.md → update axioms/mental-models/projects
   ↓ Write manifest.yaml (audit trail) + checkpoint.json (per-file resume state)
   ↓ Update _index.md state: applied
   ↓ AUTO: trigger `qmd index` for active workspace collection
   ↓ Optional: archive to archive/
```

**Evidence state machine:**
```
ingested → analyzed → qa_in_progress → qa_done → applied → archived
                              ↕
                    qa_awaiting_external
```

### Workspace Isolation

**Active workspace resolution (priority order):**
```
Priority 1: <cwd>/.claude/zwiki.json → field "workspace"
Priority 2: ~/.zwiki/config.yml → field "default_workspace"
Priority 3: if only 1 workspace exists → auto-detect
Priority 4: STOP, ask user to run zwiki init
```

**Dual enforcement:**
1. **File-level**: All operations scoped to `~/.zwiki/workspaces/{active_workspace}/`
2. **qmd-level**: Each workspace = 1 qmd collection. Query MUST specify `collection: {active_workspace}`. Cross-collection search = VIOLATION

**Evidence workspace lock:**
- `source.yaml#workspace_at_ingest` recorded at ingest
- Every state transition validates: current active workspace == workspace_at_ingest
- Mismatch → CROSS-WORKSPACE VIOLATION error, refuse operation

### `zwiki init` — Project Integration

```
$ cd ~/my-project
$ zwiki init

┌  zwiki — Project Integration
│
◆  Chọn workspace cho project này?
│  ● programming  ○ finance  ○ health  ○ philosophy
│
◆  Inject gì vào project? (multi-select)
│  ☑ .claude/zwiki.json (workspace pointer) — always
│  ◻ CLAUDE.md rules (agent biết cách dùng zwiki)
│  ◻ Slash commands (.claude/commands/ — symlink)
│  ◻ Subagents (.claude/agents/ — symlink)
│  ◻ MCP config (.claude/settings.local.json)
│
└  Done!
```

**Symlink strategy**: commands/agents là symlinks trỏ về `~/.zwiki/commands/` và `~/.zwiki/agents/`. Update zwiki binary → `zwiki update` → tất cả project nhận changes tự động.

**CLAUDE.md injection** (appended, non-destructive):
```markdown
## zwiki Integration

This project uses zwiki for knowledge retrieval. Active workspace: {workspace}.

### Available Commands
- `/ask "question"` — Search wiki and answer from retrieved knowledge
- `/learn <source>` — Ingest new material into evidence pipeline
- `/reflect` — Review and trigger evidence pipeline
- `/workspace {name}` — Switch active workspace
- `/reindex` — Reindex workspace in qmd

### Rules
- Before answering domain questions, use /ask to retrieve context
- Never guess when wiki has no relevant entry — report knowledge gap
- After learning new material, use /learn to ingest
- All wiki operations scoped to workspace: {workspace}
```

### qmd Configuration

```yaml
# ~/.zwiki/engine/qmd-config.yml
collections:
  programming:
    path: ~/.zwiki/workspaces/programming
    pattern: '**/*.md'
    ignore:
      - evidence/sources/
      - evidence/qa/
      - evidence/applied/
      - evidence/archive/
    context:
      '/': 'Programming knowledge base'
      '/axioms': 'Core programming axioms (P0)'
      '/mental-models': 'Reusable frameworks and patterns (P1)'
      '/projects': 'Books, courses, experiments (P2)'
      '/decisions': 'Decision records (P3)'
    includeByDefault: false
  # finance, health, philosophy: same pattern
```

**MCP integration (per-project, injected by `zwiki init`):**
```json
// <project>/.claude/settings.local.json
{
  "mcpServers": {
    "qmd": {
      "command": "qmd",
      "args": ["mcp", "--config-name", "zwiki"]
    }
  }
}
```

### Harness Summary

- Subagents: `wiki-planner` (parse intent → keywords, model: sonnet), `wiki-qmd-selector` (qmd search → current-task.md, model: sonnet)
- System prompt composed from: engine defaults + workspace overrides (additive only, prefix `WS:`)
- Session bridge: `current-task.md` written by Stage 2, read by Stage 3

---

## File Architecture

```
SOURCE REPO (dev only, e.g. ~/Lab/zwiki/):
├── src/
│   ├── index.ts                       ← CLI entry point
│   ├── commands/
│   │   ├── setup.ts                   ← zwiki setup handler
│   │   ├── init.ts                    ← zwiki init handler (clack)
│   │   ├── workspace.ts               ← zwiki workspace create handler
│   │   └── update.ts                  ← zwiki update handler
│   ├── core/
│   │   ├── workspace-resolver.ts      ← active workspace resolution
│   │   ├── evidence-state-machine.ts  ← state transitions + invariants
│   │   ├── qmd-retrieval.ts           ← qmd query + post-filter
│   │   ├── current-task-writer.ts     ← generate current-task.md
│   │   └── asset-extractor.ts         ← extract bundled assets → ~/.zwiki/
│   └── parsers/
│       ├── markdown.ts                ← marked wrapper
│       └── yaml.ts                    ← js-yaml wrapper
├── assets/                            ← bundled into binary
│   ├── engine/
│   │   ├── system-prompt.md
│   │   ├── constraints.md
│   │   ├── retrieval-rules.md
│   │   ├── evidence-pipeline.md
│   │   └── qmd-config.yml
│   ├── templates/
│   │   ├── workspace.md
│   │   ├── axiom.md
│   │   ├── mental-model.md
│   │   ├── project.md
│   │   ├── evidence-index.md
│   │   └── evidence-source.yaml
│   ├── commands/
│   │   ├── ask.md
│   │   ├── learn.md
│   │   ├── reflect.md
│   │   ├── workspace.md
│   │   └── reindex.md
│   ├── agents/
│   │   ├── wiki-planner.md
│   │   └── wiki-qmd-selector.md
│   └── claude-md-rules.md             ← template for CLAUDE.md injection
├── tests/
├── package.json
├── tsconfig.json
└── build.ts                           ← Bun compile + asset embedding

INSTALLED BINARY:
~/.local/bin/zwiki                     ← single Bun-compiled binary

RUNTIME DATA (~/.zwiki/, created by `zwiki setup`):
├── engine/                            ← extracted from binary
│   ├── system-prompt.md
│   ├── constraints.md
│   ├── retrieval-rules.md
│   ├── evidence-pipeline.md
│   └── qmd-config.yml
├── templates/                         ← extracted from binary
├── commands/                          ← extracted, symlink targets
├── agents/                            ← extracted, symlink targets
├── workspaces/
│   └── {domain}/
│       ├── workspace.md               ← domain identity
│       ├── axioms/                    ← P0: core facts
│       ├── mental-models/             ← P1: reusable frameworks
│       ├── projects/                  ← P2: books, courses, experiments
│       ├── decisions/                 ← P3: decision records
│       ├── evidence/
│       │   ├── _index.md              ← state tracker
│       │   ├── sources/{id}/
│       │   │   ├── source.yaml        ← metadata (immutable)
│       │   │   └── raw.md             ← original payload (immutable)
│       │   ├── analysis/{id}/
│       │   │   ├── 01-summary.md
│       │   │   ├── 02-contradiction.md
│       │   │   ├── 04-questions.md
│       │   │   └── 08-gaps.md
│       │   ├── qa/{id}/
│       │   │   ├── todo.json
│       │   │   ├── batch-N-answers.md
│       │   │   └── verified-facts.md
│       │   ├── applied/{id}/
│       │   │   ├── manifest.yaml
│       │   │   └── checkpoint.json
│       │   └── archive/
│       └── agents/                    ← OPTIONAL workspace overrides
│           └── constraints.md
├── config.yml                         ← global config
└── .cache/                            ← qmd SQLite index

PROJECT INTEGRATION (created by `zwiki init`):
<any-project>/
├── .claude/
│   ├── zwiki.json                     ← workspace pointer
│   ├── settings.local.json            ← qmd MCP config (if selected)
│   ├── commands/                      ← symlinks → ~/.zwiki/commands/
│   └── agents/                        ← symlinks → ~/.zwiki/agents/
└── CLAUDE.md                          ← appended with zwiki rules
```

---

## Done When (MVP-1)

### Functional
- [ ] `zwiki setup` creates `~/.zwiki/` structure + extracts all assets
- [ ] `zwiki setup` checks qmd availability, shows install guide if missing
- [ ] `zwiki workspace create programming` scaffolds workspace from template
- [ ] `zwiki init` in any project → interactive workspace selection + injection
- [ ] `zwiki init` creates symlinks for commands/agents that work in Claude Code
- [ ] `zwiki init` appends rules to CLAUDE.md without destroying existing content
- [ ] `zwiki update` re-extracts assets from newer binary version
- [ ] `/learn <source>` creates `sources/{id}/raw.md` + `source.yaml` (immutable)
- [ ] `/learn --analyze {id}` produces 4 analysis files
- [ ] `/learn --qa {id}` produces verified-facts.md with citations
- [ ] `/learn --apply {id}` updates wiki entries + auto-triggers `qmd index`
- [ ] `/ask "question"` returns answer from active workspace only, axioms ranked first
- [ ] `/workspace {name}` switches context; subsequent /ask scoped correctly
- [ ] `/reindex` triggers manual qmd reindex
- [ ] Cross-workspace query → error (not fallback)
- [ ] Evidence state machine transitions all work: ingested → analyzed → qa_done → applied

### Structure
- [ ] Binary builds successfully for macOS ARM64 (`bun build --compile`)
- [ ] `~/.zwiki/` folder tree matches File Architecture
- [ ] All templates exist and produce valid workspace structure
- [ ] qmd config generated, `qmd --config-name zwiki status` shows indexed collections
- [ ] Symlinks from project `.claude/commands/` → `~/.zwiki/commands/` resolve correctly

### Documentation
- [ ] README.md with install + setup + workflow (Vietnamese)
- [ ] Example entries in at least 1 workspace
- [ ] Slash commands with clear instructions

---

## Setup Workflow

```bash
# 1. Install binary (GitHub Release)
curl -fsSL https://github.com/.../releases/latest/download/zwiki-darwin-arm64 \
  -o ~/.local/bin/zwiki && chmod +x ~/.local/bin/zwiki

# 2. First-time setup
zwiki setup
# → Creates ~/.zwiki/ + extracts engine, templates, commands, agents
# → Checks qmd, shows install guide if missing

# 3. Install qmd (if missing)
npm i -g @tobilu/qmd

# 4. Create first workspace
zwiki workspace create programming

# 5. Index
qmd --config-name zwiki index

# 6. Verify
qmd --config-name zwiki status

# 7. Init in any project
cd ~/my-project
zwiki init
# → Interactive: workspace, inject targets
```

---

## Key Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| D1 | 3-layer model (axiom > model > project) | YAGNI — personal wiki needs isolation + priority, not safety gates |
| D2 | qmd BM25 only (no vector/hybrid) | Sufficient for personal wiki; no models to download; instant setup |
| D3 | qmd MCP integration (not SDK/CLI) | Clean tool interface for Claude Code subagents |
| D4 | Path-prefix post-filter (not separate collections per tier) | Simpler config (N vs 3N collections); BM25 score useful within tier |
| D5 | Bun compile (not Node SEA / npm global) | Single binary, no runtime dependency, simplest build |
| D6 | Symlink commands/agents (not copy) | Update once → all projects get changes; no sync needed |
| D7 | YAML config (not JSON) | More readable for human editing; js-yaml already in deps |
| D8 | `zwiki init` non-destructive (append CLAUDE.md) | Never overwrite user's existing CLAUDE.md content |
| D9 | Clack style for all CLI commands (not raw console.log) | Consistent polished UX; clack handles spinners, prompts, colors out of the box |

---

## Risks

| Risk | Severity | Mitigation |
|------|----------|-----------|
| Priority inversion (project ranked above axiom) | High | Post-filter by tier BEFORE scoring; test with known queries |
| Cross-workspace leakage via qmd | High | Validate collection param; unit test isolation |
| Symlinks break on different OS/path | Medium | Test on macOS + Linux; fallback copy if symlink fails |
| BM25 index staleness after wiki edit | Medium | Auto-reindex in /learn --apply; manual /reindex |
| qmd not installed on target | Medium | `zwiki setup` checks + guides; clear error message |
| Bun compile binary size (~60MB) | Low | Acceptable for CLI tool; no runtime dependency tradeoff |
| BM25 miss on synonyms | Low | Accept; upgrade hybrid later if needed |

---

## Dependencies / Assumptions

### Dependencies

- Bun runtime (build only, not on target machine)
- qmd (`@tobilu/qmd`) — BM25 search backend, installed separately
- Claude Code harness (slash commands via `.claude/commands/`)
- Git (optional, for version control of knowledge)

### Assumptions

- User có discipline để maintain workspace boundaries
- User dùng evidence pipeline thay vì edit wiki trực tiếp
- qmd MCP server chạy khi Claude Code session active
- Vietnamese là ngôn ngữ chính cho docs, knowledge entries có thể English/Vietnamese tùy source
- Target machines: macOS (ARM64/x64), Linux (x64). Windows deferred.

---

## Next Steps

1. → Run `/plan` to generate roadmap + executable phases
2. → Run `/cook` to implement wave-by-wave

Classification: new-spec · normal lane · downstream: plan full
