# Phase Plan: P4 — Slash Commands + Subagents

Inputs: P1 deliverables (asset directory with placeholder files)
Depends on: P1 (for file locations)

---

## Wave 1: Subagents

### Task 1.1: wiki-planner subagent
- Write assets/agents/wiki-planner.md
- Frontmatter: name, description ("Parse user question into search keywords"), model: sonnet, tools: [Read, Glob, Grep]
- Body instructions:
  1. Receive question from /ask command
  2. Resolve active workspace (read .claude/zwiki.json or use resolver logic)
  3. Extract 3-5 search keywords from the question
  4. Output JSON: { workspace, keywords, domain }
  5. Rules: no guessing, no cross-workspace, output must be valid JSON
- **Verification**: File has valid YAML frontmatter, instructions are clear and complete
- **Touched**: assets/agents/wiki-planner.md

### Task 1.2: wiki-qmd-selector subagent
- Write assets/agents/wiki-qmd-selector.md
- Frontmatter: name, description ("Search qmd and build current-task.md"), model: sonnet, tools: [Read, Glob, Grep, Write, qmd MCP tools]
- Body instructions:
  1. Receive keywords + workspace from wiki-planner output
  2. Call qmd MCP `search` tool: { query: keywords, collection: workspace, limit: 20 }
  3. Post-filter results by path prefix:
     - /axioms/* → P0
     - /mental-models/* → P1
     - /projects/* → P2
     - /decisions/* → P3
  4. Re-rank: P0 first → P1 → P2 → P3 (within tier: keep BM25 ordering)
  5. Fetch full body for top 6-8 results via qmd `get` tool
  6. Write current-task.md with: search keywords, retrieved docs table by priority, full context slices, knowledge gaps
  7. Rules: MUST specify collection parameter (I-6), axioms get full body, others 400 tokens
- **Verification**: File references correct MCP tool names, post-filter logic is explicit
- **Touched**: assets/agents/wiki-qmd-selector.md

---

## Wave 2: Retrieval Commands

### Task 2.1: /ask command
- Write assets/commands/ask.md
- Frontmatter: description ("Search wiki and answer using retrieved knowledge"), arguments: "question (required)"
- Body:
  1. Dispatch wiki-planner subagent with the question
  2. Dispatch wiki-qmd-selector with planner output
  3. Read current-task.md
  4. Answer the question using ONLY retrieved context
  5. If no relevant results → STOP, report "Knowledge gap: {topic}"
  6. Cite sources in answer (file path + priority tier)
- **Verification**: Instructions reference both subagents by name
- **Touched**: assets/commands/ask.md

### Task 2.2: /workspace command
- Write assets/commands/workspace.md
- Frontmatter: description ("Switch active workspace"), arguments: "name (required)"
- Body:
  1. Validate workspace exists at ~/.zwiki/workspaces/{name}/
  2. Write .claude/zwiki.json: { "workspace": "{name}" }
  3. Confirm switch with workspace info
- **Verification**: Instructions include validation step
- **Touched**: assets/commands/workspace.md

### Task 2.3: /reindex command
- Write assets/commands/reindex.md
- Frontmatter: description ("Reindex active workspace in qmd"), arguments: none
- Body:
  1. Resolve active workspace
  2. Run: `qmd --config-name zwiki index --collection {workspace}`
  3. Run: `qmd --config-name zwiki status` to verify
  4. Report index stats
- **Verification**: Includes both index and status commands
- **Touched**: assets/commands/reindex.md

---

## Wave 3: Evidence Commands

### Task 3.1: /learn command
- Write assets/commands/learn.md
- Frontmatter: description ("Evidence pipeline — ingest, analyze, QA, apply"), arguments: "source (for ingest) OR --analyze|--qa|--apply {id}"
- Body — 4 modes:
  **Ingest (default)**: `/learn <source>`
  1. Generate evidence ID (YYYYMMDD-slug format)
  2. Create ~/.zwiki/workspaces/{ws}/evidence/sources/{id}/
  3. Write raw.md (copy source content, immutable after this point)
  4. Write source.yaml (id, type, workspace_at_ingest, ingested_at, state: ingested)
  5. Update _index.md: add row with id, state=ingested, date

  **Analyze**: `/learn --analyze {id}`
  1. Validate state == ingested (I-2: check workspace_at_ingest matches active)
  2. Read raw.md
  3. Run 4 analysis prompts → write to analysis/{id}/:
     - 01-summary.md (summarize key points)
     - 02-contradiction.md (find contradictions with existing wiki knowledge)
     - 04-questions.md (generate P0/P1/P2/P3 questions)
     - 08-gaps.md (identify knowledge gaps)
  4. Update _index.md state → analyzed

  **QA**: `/learn --qa {id}`
  1. Validate state == analyzed
  2. Read 04-questions.md, batch by priority (P0 first)
  3. Present questions to human, collect answers
  4. Write qa/{id}/batch-N-answers.md (append-only)
  5. Generate verified-facts.md (each fact MUST cite question ID + affected wiki path)
  6. Update _index.md state → qa_done
  7. If P0/P1 questions deferred → state = qa_awaiting_external

  **Apply**: `/learn --apply {id}`
  1. Validate state == qa_done (I-3: block if P0/P1 awaiting_external)
  2. Read verified-facts.md
  3. For each fact: update or create entry in axioms/mental-models/projects/
  4. Write applied/{id}/manifest.yaml (audit trail: which files changed)
  5. Write applied/{id}/checkpoint.json (per-file progress for resume — I-5)
  6. Update _index.md state → applied
  7. AUTO: trigger `qmd --config-name zwiki index --collection {workspace}`
- **Verification**: All 4 modes documented, invariants I-1 through I-5 explicitly mentioned
- **Touched**: assets/commands/learn.md

### Task 3.2: /reflect command
- Write assets/commands/reflect.md
- Frontmatter: description ("Review recent learning and trigger evidence pipeline"), arguments: none
- Body:
  1. Resolve active workspace
  2. Read _index.md, find evidence with state != applied
  3. Summarize pending evidence (what's ingested, analyzed, awaiting QA)
  4. Suggest next action (/learn --analyze, --qa, or --apply for each)
- **Verification**: Instructions reference _index.md as source of truth
- **Touched**: assets/commands/reflect.md

---

## Stop Conditions
- Claude Code command format unclear → check Claude Code documentation
- MCP tool names for qmd wrong → verify with `qmd mcp --help`

## Escalation
- If subagent model "sonnet" is not available → use default model
- If MCP tool interface differs from expected → adjust tool names in agent .md
