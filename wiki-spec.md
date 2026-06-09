# zbrain — Product Spec

> Living reference for the zbrain product. Updated to reflect the current implementation.

---

## 1. What Is This Project

`zbrain` is a **local-first CLI for building a personal LLM wiki** that AI agents can use safely. It stores knowledge in workspace-isolated directories, retrieves it via BM25 search before answering questions, and ingests new material through a structured evidence pipeline.

**Core claim:** Dumping a full wiki into a prompt causes hallucination or selective ignoring. zbrain filters, ranks, and scopes knowledge *before* an agent sees it — using BM25 retrieval and knowledge-tier re-ranking, not full-context injection.

**Distribution:** Bun-compiled binary (`zbrain`) + bundled Claude Code skills extracted to `~/.zbrain/` on `zbrain setup`.

---

## 2. Mental Model

```
zbrain = ENGINE (shared runtime) + N WORKSPACES (isolated personal domains)

~/.zbrain/
├── engine/          ← prompts, constraints, retrieval rules
├── templates/       ← scaffolds for workspaces and doc types
├── commands/        ← bundled skills
├── agents/          ← agent definitions
└── workspaces/
    ├── programming/     ← Workspace: software engineering knowledge
    ├── finance/         ← Workspace: personal finance
    ├── health/          ← Workspace: health and fitness
    └── philosophy/      ← Workspace: philosophical frameworks
```

**Key invariant:** Knowledge from workspace A is NEVER used when working in workspace B. Active workspace is **per-project** (declared in `~/.zbrain/projects.json`), not global.

---

## 3. Architecture

### 3.1 CLI Layer

| Command | What it does |
|---------|-------------|
| `zbrain setup` | Extract bundled assets → `~/.zbrain/`, install config |
| `zbrain init` | Register the current project in `~/.zbrain/projects.json` |
| `zbrain workspace create <name>` | Scaffold a new workspace from template |
| `zbrain learn` | Record raw source material into workspace evidence sources |
| `zbrain ingest` | List, analyze, QA, and apply learned evidence |
| `zbrain ask` | Retrieve ranked workspace context for a question |
| `zbrain update` | Sync runtime assets from a new binary version |

### 3.2 Engine Layer

| Component | Location | Role |
|-----------|----------|------|
| System prompt | `assets/engine/system-prompt.md` | Agent role + workspace scope rules |
| Constraints | `assets/engine/constraints.md` | Hard rules: no cross-workspace, no unsourced answers |
| Retrieval rules | `assets/engine/retrieval-rules.md` | BM25 + tier re-ranking logic |
| Evidence rules | `assets/engine/evidence-rules.md` | Evidence pipeline state machine + invariants |
| Claude rules | `assets/engine/claude-rules.md` | Non-destructive CLAUDE.md injection rules |

### 3.3 Skills Layer

| Skill | Invocation | Role |
|-------|-----------|------|
| `zbrain:learn` | `/zbrain:learn [source]` | Record raw source material into workspace evidence sources |
| `zbrain:ingest` | `/zbrain:ingest list|analyze|qa|apply` | Process learned evidence into workspace knowledge |
| `zbrain:ask` | `/zbrain:ask "question"` | 3-stage retrieval → `current-task.md` |

### 3.4 Workspace Structure

```
~/.zbrain/workspaces/{name}/
├── workspace.md           ← identity: domain, purpose, operating rules
├── axioms/                ← P0: core facts that other knowledge must not contradict
├── mental-models/         ← P1: reusable frameworks and thinking patterns
├── projects/              ← P2: book notes, course notes, experiments
├── decisions/             ← P3: logged personal decisions with reasoning
└── evidence/
    ├── _index.md          ← state tracker for all evidence items
    ├── sources/{id}/      ← immutable raw data (raw.md + source.yaml)
    ├── analysis/{id}/     ← structured analysis output (analysis.md)
    ├── qa/{id}/           ← Q&A batches + verified-facts.md
    └── applied/{id}/      ← manifest + checkpoint of applied changes
```

### 3.5 Active Workspace Resolution

```
Priority 1: ~/.zbrain/projects.json → matching `project_root` entry → field `workspace`
Priority 2: ~/.zbrain/config.yml       → field "default_workspace"
Priority 3: STOP — report missing pointer, do not auto-select
```

### 3.6 Project Integration Files

```
<cwd>/
└── .claude/
    ├── zbrain.json         ← {"workspace": "programming"}
    ├── settings.json       ← Claude Code settings (zbrain rules injected non-destructively)
    └── commands/           ← optional symlinks to ~/.zbrain/commands/
```

---

## 4. Knowledge Priority Order

```
axioms/ > mental-models/ > projects/ > decisions/
```

Applied strictly within the active workspace. When two sources conflict, higher-priority wins. When knowledge is missing, the agent stops and reports the gap — never guesses, never borrows from another workspace.

---

## 5. Retrieval Pipeline (`zbrain:ask`)

Triggered before answering any question about the active workspace. Three stages.

```
[Stage 1] Keyword parse
    Parse the question into 3–7 workspace-scoped BM25 keywords
    ↓
[Stage 2] BM25 search
    Call qmd search against the active workspace collection only
    ↓
[Stage 3] Tier re-rank + write
    Re-rank results by tier before score:
      P0 axioms/ → P1 mental-models/ → P2 projects/ → P3 decisions/
    Fetch full bodies for top-ranked documents
    Write ranked context + citation paths → current-task.md
```

**`current-task.md`** = single source of truth for the current retrieval session. Contains: ranked context docs, citation paths (`workspace/tier/file`), and knowledge gap report if results are insufficient.

**On empty or insufficient results:** record the knowledge gap and stop. Never answer from memory.

**qmd prerequisite:** `npm i -g @tobilu/qmd` — installed separately, not bundled in the binary.

---

## 6. Learn + Evidence Pipeline (`zbrain:learn`, `zbrain:ingest`)

When new material arrives, use `zbrain learn` to record it as immutable evidence. Use `zbrain ingest` to analyze, QA, and apply it instead of editing workspace knowledge directly.

### State Machine

```
ingested → analyzed → qa_in_progress → qa_done → applied → archived
                            ↕
                  qa_awaiting_external
```

Only `qa_done → qa_in_progress` is a valid backward transition.

### Stages

| Stage | Command | What it does |
|-------|---------|-------------|
| Learn | `zbrain:learn` | Store raw content as `sources/{id}/raw.md` + `source.yaml`; append to `_index.md` |
| List | `zbrain:ingest list` | Show evidence items and next-action guidance |
| Analyze | `zbrain:ingest analyze {id}` | Four structured passes → `analysis/{id}/` |
| QA | `zbrain:ingest qa {id}` | User-resolved facts → `qa/{id}/verified-facts.md` |
| Apply | `zbrain:ingest apply {id}` | Write verified facts to knowledge-tier files and reindex internally |

### Key Invariants

- **I-1 Immutable sources:** `raw.md` and `source.yaml` cannot be modified after ingest.
- **I-2 Workspace lock:** `source.yaml#workspace_at_ingest` must match current active workspace at every stage.
- **I-3 State monotonicity:** no backward transitions except `qa_done → qa_in_progress`.
- **I-4 QA gate:** apply blocks if any P0 or P1 question is `awaiting_external` or `deferred`.
- **I-5 Citation requirement:** every entry in `verified-facts.md` must cite a `question_id` and a target wiki file path.

### QA Gate Rules

| Priority | `awaiting_external` | `deferred` |
|----------|---------------------|------------|
| P0 | Block apply | Block apply |
| P1 | Block apply | Block apply |
| P2 | Warn, allow | Allow |
| P3 | Allow | Allow |

---

## 8. CLI Commands Reference

| Command | What it does |
|---------|-------------|
| `zbrain setup` | First-run: extract bundled assets into `~/.zbrain/`, write `config.yml` |
| `zbrain init` | Register the current project in `~/.zbrain/projects.json`; inject runtime-specific rules non-destructively |
| `zbrain workspace create <name>` | Scaffold a new workspace directory from template |
| `zbrain learn` | Record raw source material into workspace evidence sources |
| `zbrain ingest list/analyze/qa/apply` | Process learned evidence |
| `zbrain ask <question>` | Retrieve ranked workspace context |
| `zbrain update` | Re-extract assets from the current binary version into `~/.zbrain/` |

---

## 9. Skills Reference

| Skill | Description |
|-------|-------------|
| `zbrain:learn` | Record raw source material into workspace evidence sources |
| `zbrain:ingest` | List, analyze, QA, and apply learned evidence |
| `zbrain:ask` | Retrieve ranked workspace context before answering a question |

---

## 10. Design Philosophy

1. **Workspace isolation is sacred** — one context bleed = hallucination risk; no cross-workspace reads
2. **Narrow retrieval > full dump** — BM25 + tier re-ranking filters before agent sees; more context ≠ better
3. **Evidence trail** — every wiki fact must be traceable to a source; never answer from memory
4. **Stop on gaps** — agent stops and reports missing knowledge instead of guessing
5. **Immutable raw sources** — ingested material is immutable; analysis builds on top, never replaces
6. **Local-first** — no external sync, no server, no network required for retrieval
7. **BM25 only (MVP-1)** — vector search and hybrid retrieval explicitly out of scope

---

## 11. Goals

| Goal | Mechanism |
|------|-----------|
| Agent follows personal knowledge, not hallucinations | axioms/ is highest priority; agent cannot bypass |
| One person works across N domains without knowledge bleed | Workspace isolation + per-project active pointer |
| Knowledge stays in sync with learning | `zbrain:learn` records sources; `zbrain:ingest` applies verified facts and reindexes internally |
| Every wiki fact has a source | Evidence pipeline with immutable raw + citation requirement |
| Context window efficiency | 3-stage retrieval pipeline, not full wiki dump |
| Resume sessions mid-task | `current-task.md` persists ranked context for the session |

---

## 12. MVP-1 Scope

**In scope:**
- 4 default workspaces: `programming`, `finance`, `health`, `philosophy`
- 4-stage evidence pipeline: ingest → analyze → qa → apply
- 3-stage retrieval: keyword parse → qmd BM25 → tier re-rank
- Bun binary distribution with bundled asset extraction
- Project integration via `zbrain init`

**Out of scope:**
- Web UI
- Vector search or hybrid retrieval
- External sync integrations (Confluence, Notion, etc.)
- Cross-workspace retrieval
- npm package distribution
- qmd bundled into the binary

---

## 13. File Map (Key Files)

```
zbrain/
├── README.md                              ← quick-start and command surface
├── CLAUDE.md                              ← zbrain runtime pointer rules
├── wiki-spec.md                           ← this file — full product spec
├── src/
│   ├── index.ts                           ← CLI entry point
│   ├── commands/                          ← setup, init, workspace, update, update
│   ├── core/                              ← evidence pipeline, retrieval, qmd adapter, assets
│   └── schemas/config.ts                  ← globalConfigSchema, projectPointerSchema
├── assets/
│   ├── engine/                            ← system-prompt, constraints, retrieval-rules, evidence-rules
│   ├── skills/
│   │   ├── zbrain-ask/SKILL.md            ← 3-stage retrieval spec
│   │   ├── zbrain-learn/SKILL.md          ← evidence pipeline driver spec
│   │   ├── zbrain-learn/references/pipeline.md ← per-stage flows + QA gate rules
│   │   ├── zbrain-reflect/SKILL.md        ← reflection flow spec
│   │   ├── zbrain-workspace/SKILL.md      ← workspace pointer manager spec
│   │   └── zbrain-reindex/SKILL.md        ← BM25 reindex spec
│   ├── templates/                         ← workspace.md, axiom.md, mental-model.md, project.md, evidence-*
│   └── workspaces/                        ← seed workspace.md for 4 default domains
├── docs/
│   ├── acceptance-walkthrough.md          ← end-to-end proof of full path
│   └── release.md                         ← release guidance
└── .kit/planning/                         ← locked SPEC.md, roadmap, phase plans
```
