# wiki-template — Project Spec for Brainstorm

> Purpose of this file: capture full analysis of `wiki-template/` so the next session can brainstorm building a personal LLM-wiki based on the same model.

---

## 1. What Is This Project

A **knowledge management system** for engineers who work across multiple companies/projects. It serves as **runtime context** for AI coding agents (Claude Code), not just human documentation. The agents read it before writing any code — as a constraint source, not a reference.

**Core claim:** Throwing a full wiki into a prompt causes hallucination or selective ignoring. This system filters, ranks, and structures knowledge *before* an agent sees it.

---

## 2. Mental Model

```
wiki-template = ENGINE (shared) + N WORKSPACES (isolated sandboxes)

wiki-template/
├── agents/          ← ENGINE: system prompt, coding rules, constraints, pipeline
├── templates/       ← ENGINE: skeletons for new workspaces and new doc types
├── .claude/commands/ ← ENGINE: slash commands
└── workspaces/
    ├── example-surgery/   ← Workspace A (Company/Project A)
    └── c08/               ← Workspace B (Company/Project B)
```

**Key invariant:** Knowledge from Workspace A is NEVER used when working in Workspace B. Active workspace is **per-codebase** (declared in `<codebase>/.claude/wiki.json`), not global.

---

## 3. Architecture

### 3.1 Engine Layer

| Component | Location | Role |
|-----------|----------|------|
| System prompt | `agents/system-prompt.md` | Agent role + workspace scope rules |
| Coding rules | `agents/coding-rules.md` | Universal code constraints (Java, Kafka, MQTT) |
| Constraints | `agents/constraints.md` | Hard don'ts — never bypass retry, never hardcode topic names, etc. |
| Pipeline | `agents/pipeline/` | 5-stage retrieval pipeline (see §5) |
| Slash commands | `.claude/commands/` | `/use-wiki`, `/update-wiki`, `/evidence-ingest`, etc. |
| Templates | `templates/` | Skeleton for workspace, service, ADR, runbook, pattern docs |

### 3.2 Workspace Layer

Each workspace = one company or project context.

```
workspaces/{name}/
├── workspace.md           ← identity: company, role, stack, period
├── patterns-index.md      ← quick lookup table (pattern name → file)
├── platform/
│   ├── contracts/         ← HIGHEST PRIORITY — API schemas, topic formats, MUST-FOLLOW
│   ├── patterns/          ← canonical implementations to reuse
│   ├── architecture/      ← system topology, service map
│   └── infrastructure/    ← deployment config
├── domains/{domain}/
│   └── workflow.md        ← business state machines, allowed transitions
├── projects/{project}/
│   ├── knowledge-map.md   ← entry point for each project
│   ├── services/          ← per-service docs, local config overrides
│   └── decisions/         ← project-level ADRs
├── runbooks/              ← incident handling procedures
├── decisions/             ← workspace-level ADRs
├── evidence/              ← raw data pipeline (source → wiki change)
│   ├── _index.md          ← state tracker for all evidence items
│   ├── sources/{id}/      ← immutable raw data (raw.md + source.yaml)
│   ├── analysis/{id}/     ← 4 CORE analysis files (tech-stack, service-map, patterns, contracts)
│   ├── qa/{id}/           ← Q&A batches + verified-facts.md
│   └── applied/{id}/      ← manifest + checkpoint of applied changes
└── agents/                ← OPTIONAL: workspace overrides engine defaults
    ├── constraints.md
    └── pipeline/validator-rules.md
```

### 3.3 Active Workspace Resolution

```
Priority 1: <cwd>/.claude/wiki.json → field "workspace"
Priority 2: ~/.claude/wiki-global.json → field "default_workspace"
Priority 3: if only 1 workspace exists → use it
Priority 4: STOP, ask user to /switch-workspace
```

---

## 4. Knowledge Priority Order

```
Contracts > Platform Patterns > Project Docs > Domain Knowledge
```

Applied strictly within the active workspace scope. When two sources conflict, higher-priority wins. When knowledge is missing, agent says so — never guesses, never borrows from another workspace.

---

## 5. The 5-Stage Coding Pipeline (`/use-wiki`)

Triggered before writing any code. Each stage has a narrow job + fixed output schema.

```
[Stage 0] Main agent         → resolve workspace + wiki_root from <cwd>/.claude/wiki.json
    ↓
[Stage 1] wiki-planner       → parse task → intent JSON
    ↓
[Stage 2] wiki-context-selector → map intent → file paths → slice sections → write current-task.md
    ↓
[Stage 2.5] wiki-plan-reviewer  → 4 checks: patterns exist? context complete? conflicts? gaps? → APPROVED or BLOCK
    ↓
[Stage 3] Main agent (Builder)  → read current-task.md → code using prompt-template
    ↓
[Stage 4] wiki-reviewer (opt)   → compare code vs contracts/patterns → APPROVED or Violations
```

**Intent JSON schema (Stage 1 output):**
```json
{
  "workspace": "example-surgery",
  "type": "implement_feature | fix_bug | design | incident | review",
  "domain": "surgery",
  "components": ["kafka", "mqtt", "batch", "http", "db"],
  "scope": "surgery-service",
  "patterns_needed": ["kafka-event-processing"],
  "contracts_touched": ["mqtt-topic-contract"],
  "missing_knowledge": []
}
```

**`current-task.md`** = single source of truth for the session. Contains: ranked context docs, knowledge gaps, plan review warnings.

**Builder output format (Stage 3):**
```
## Understanding
## Knowledge Mapping   ← link to sections in current-task.md
## Design
## Implementation
## Edge Cases
## Assumptions
```

---

## 6. Evidence Pipeline (wiki update from external sources)

When wiki needs to be updated from an external source (Confluence page, API response, incident report, codebase snapshot), use the evidence pipeline instead of editing wiki directly.

### State Machine

```
ingested → analyzed → qa_in_progress → qa_done → applied → archived
                              ↕
                    qa_awaiting_external
```

### Stages

| Command | What it does |
|---------|-------------|
| `/evidence-ingest` | Fetch raw from MCP/API/paste/code → store in `sources/{id}/raw.*` + `source.yaml` |
| `/evidence-analyze` | Run 4 CORE prompts: tech-stack, service-map, pattern-proposals, contract-proposals |
| `/evidence-qa` | Batch Q&A to resolve unknowns → build `verified-facts.md` |
| `/evidence-apply` | Apply verified facts → update wiki files, write `applied/{id}/manifest.yaml` |

### Key Invariants

- **I-1 Immutable sources**: `raw.*` and `source.yaml` cannot be modified after ingest
- **I-2 Workspace lock**: `source.yaml#workspace_at_ingest` must match current active workspace at every stage
- **I-3 State monotonicity**: no backward transitions except `qa_done → qa_in_progress`
- **I-5 Citation requirement**: every entry in `verified-facts.md` must cite a question ID + wiki file path

### Code Snapshot (`--source code`)

When source is a codebase, `raw.md` is a structured snapshot with 10 sections: project metadata, dependencies, configs (secrets redacted), REST endpoints, message consumers, services, DB schema, public APIs, git summary, notes. Each entry has file citations `(path:L{start}-L{end})`.

---

## 7. Slash Commands Reference

| Command | What it does |
|---------|-------------|
| `/wiki-setup` | Create `<cwd>/.claude/wiki.json` for a codebase |
| `/switch-workspace {name}` | Change active workspace for current codebase |
| `/list-workspaces` | Show all workspaces + which one is active |
| `/new-workspace {name}` | Scaffold new workspace from template |
| `/use-wiki "task"` | Run 5-stage pipeline before coding |
| `/update-wiki` | Sync wiki after code changes |
| `/rebase-wiki` | Verify wiki vs codebase, flag stale docs |
| `/code-analyze` | Snapshot codebase → evidence pipeline (ingest + analyze) |
| `/evidence-ingest` | Step 1 of evidence pipeline |
| `/evidence-analyze` | Step 2 |
| `/evidence-qa` | Step 3 |
| `/evidence-apply` | Step 4 |

---

## 8. Design Philosophy

1. **Write for machines, not just humans** — every doc must be parseable by agent, not just readable by human
2. **Workspace isolation is sacred** — one context bleed = hallucination risk
3. **Don't invent architecture** — agent follows knowledge base, doesn't create new patterns
4. **Narrow retrieval > full dump** — pipeline filters before agent sees; more context ≠ better
5. **Evidence trail** — every wiki fact must be traceable to a source
6. **Gate early** — Plan-Reviewer (Stage 2.5) catches issues before Builder spends tokens
7. **Fixed schemas** — each stage has a fixed output format → auditable, resumable on failure
8. **Workspace overrides engine** — workspace-specific rules take priority over engine defaults when conflicting

---

## 9. Goals

| Goal | Mechanism |
|------|-----------|
| AI follows organizational constraints, not hallucinations | Contracts are highest priority, agent cannot bypass |
| One engineer works across N companies without knowledge bleed | Workspace isolation + per-codebase active pointer |
| Knowledge stays in sync with code | `/update-wiki` after code changes; `/rebase-wiki` for drift check |
| Every wiki fact has a source | Evidence pipeline with immutable raw + citation requirement |
| Context window efficiency | Narrow retrieval pipeline, not full wiki dump |
| Resume sessions mid-task | `current-task.md` persists full context for session |

---

## 10. Metrics / Quality Signals

| Signal | What it measures |
|--------|-----------------|
| Plan-Reviewer BLOCK rate | Gaps/conflicts caught before code generation |
| Violations Found rate (Stage 4) | Contract/pattern violations in generated code |
| `missing_knowledge` frequency in intent JSON | Coverage gaps in wiki relative to actual tasks |
| Evidence pipeline completion rate | % of external knowledge ingested vs left in ad-hoc notes |
| `/rebase-wiki` staleness count | Drift between wiki and actual codebase |

---

## 11. What to Build for Personal LLM-Wiki (Next Brainstorm)

The template is designed for **multi-company engineering work with AI agents**. For a personal knowledge wiki the same model applies but with different domain types.

### Potential Adaptations

| wiki-template concept | Personal LLM-wiki equivalent |
|----------------------|------------------------------|
| Workspace = company/project | Workspace = domain/topic area (e.g. finance, health, programming) |
| `platform/contracts/` | Core facts / axioms that other knowledge must not contradict |
| `platform/patterns/` | Reusable frameworks / mental models |
| `domains/{domain}/workflow.md` | Decision trees / processes for a domain |
| `projects/{project}/` | Specific books, courses, experiments |
| Evidence pipeline | Learning pipeline: raw notes → analysis → Q&A → knowledge entries |
| Agent coding rules | Agent writing/reasoning rules |
| `/use-wiki "task"` | `/use-wiki "question"` to retrieve relevant personal knowledge before answering |

### Key Questions for Brainstorm

1. What are the "workspace" boundaries in personal knowledge? (by topic? by life area? by project?)
2. What replaces "contracts" — what is highest-priority personal knowledge that cannot be overridden?
3. How does the evidence pipeline map to personal learning flow (raw → notes → verified → published)?
4. What is the personal equivalent of "don't invent architecture" — don't fabricate facts, cite sources?
5. Should the 5-stage pipeline be simplified for personal use or kept full?
6. What does "/update-wiki" mean for personal knowledge — after reading a book? after an experience?

---

## 12. File Map (Key Files to Re-read)

```
wiki-template/
├── README.md                                   ← project overview + workflow
├── CLAUDE.md                                   ← AI agent instructions (complete)
├── agents/system-prompt.md                     ← agent role + workspace scope rules
├── agents/pipeline/multi-agent-pipeline.md     ← 5-stage pipeline design
├── agents/pipeline/intent-parser.md            ← intent JSON schema
├── agents/pipeline/context-retrieval-map.md    ← intent → file path mapping
├── agents/pipeline/evidence-state-rules.md     ← evidence state machine + invariants
├── workspaces/README.md                        ← workspace model + override rules
├── templates/workspace.md                      ← workspace scaffold
├── .claude/commands/use-wiki.md                ← execution flow (spec-of-record)
├── .claude/commands/evidence-ingest.md         ← evidence pipeline step 1
└── workspaces/example-surgery/                 ← full working example
```
