export interface BundledAssetRecord {
  relativePath: string;
  contents: string;
}

export const bundledAssets: BundledAssetRecord[] = [
  {
    "relativePath": "README.md",
    "contents": "# Bundled Assets\n\nThe root `assets/` directory is the only source of truth for runtime content bundled into `zbrain`.\n\n`wiki-template/` is migration input material only and should not be treated as a second runtime asset tree.\n"
  },
  {
    "relativePath": "agents/wiki-planner.md",
    "contents": "# wiki-planner\n\n## Role\n\nParse a user task into a small, workspace-scoped retrieval intent.\n\n## Output Contract\n\nReturn JSON with:\n\n- `workspace`\n- `keywords`\n- `domain`\n- `knowledge_gaps`\n\n## Rules\n\n- Use the active workspace only.\n- Do not invent patterns or facts that are not represented in runtime assets.\n"
  },
  {
    "relativePath": "agents/wiki-qmd-selector.md",
    "contents": "# wiki-qmd-selector\n\n## Role\n\nRun qmd BM25 retrieval for the active workspace and produce ranked context.\n\n## Retrieval Contract\n\n- always pass the active workspace collection to qmd\n- rank by tier before score:\n  - `axioms/` -> P0\n  - `mental-models/` -> P1\n  - `projects/` -> P2\n  - `decisions/` -> P3\n- fetch full bodies for the top ranked documents\n- write `current-task.md`\n\n## Failure Contract\n\n- if retrieval is empty or insufficient, record a knowledge gap and stop\n"
  },
  {
    "relativePath": "engine/claude-rules.md",
    "contents": "## zbrain Integration\n\nzbrain is a workspace-isolated knowledge retrieval layer. Skills live in `.claude/skills/zbrain-*`.\nRuntime root: `~/.zbrain/`. Project registry: `~/.zbrain/projects.json`.\n\n### Workspace Resolution\n\n1. Read `~/.zbrain/projects.json`.\n2. Find the entry whose `project_root` matches the current project root.\n3. Use that entry's `workspace` and `context_file`.\n4. Fallback: `~/.zbrain/config.yml` → `default_workspace`.\n5. If neither resolves, stop and report — never guess a workspace.\n\n### Skill Triggers\n\n| When you need to… | Use |\n|--------------------|-----|\n| Answer domain questions (architecture, decisions, patterns) | `zbrain:ask` |\n| Record a file, URL content, pasted text, or observation | `zbrain:learn` |\n| List, analyze, QA, or apply evidence | `zbrain:ingest` |\n\n**Before answering any question about domain knowledge, project decisions, or architectural patterns — invoke `zbrain:ask` first. Never answer from memory.**\n\n### Retrieval Tier Priority\n\n`axioms/` (P0) → `mental-models/` (P1) → `projects/` (P2) → `decisions/` (P3)\n\nHigher-tier results rank first regardless of BM25 score.\n\n### Evidence Pipeline\n\nEach piece of external material moves through three public verbs:\n\n```\nlearn → ingest → ask\n```\n\nUse `zbrain:ingest list` to see which stage each item is in and what command runs next.\n**Never advance to apply if any P0 or P1 question is unresolved.**\n\n### Secondary Workspaces (optional)\n\nEach project registry entry supports a `secondary_workspaces` array for cross-workspace context.\nEach entry has `workspace`, `keywords`, and optional `limit`.\nSecondary results fill remaining slots after primary results.\n\n### Invariants\n\n- **Cite all retrieved context.** Never answer domain questions from memory.\n- **One workspace per query.** Never cross workspace boundaries in a single retrieval.\n- **Evidence is immutable after ingest.** Never edit `raw.md` or `source.yaml`.\n- **Apply gate.** Block apply if any P0 or P1 QA question is `awaiting_external`.\n"
  },
  {
    "relativePath": "engine/codex-rules.md",
    "contents": "## zbrain Integration\n\nzbrain is a workspace-isolated knowledge retrieval layer for this project.\nRuntime root: `~/.zbrain/`. Project registry: `~/.zbrain/projects.json`.\n\n### Workspace Resolution\n\n1. Read `~/.zbrain/projects.json`.\n2. Find the entry whose `project_root` matches the current project root.\n3. Use that entry's `workspace` and `context_file`.\n4. Fallback: `~/.zbrain/config.yml` → `default_workspace`.\n5. If neither resolves, stop and report — never guess a workspace.\n\n### Expected Usage\n\n- Use `zbrain learn` to record raw source material.\n- Use `zbrain ingest` to list, analyze, QA, and apply evidence.\n- Before answering domain questions, use `zbrain ask` retrieval first.\n- Read the registry entry's `context_file` after retrieval to inspect ranked context and citations.\n- Keep all retrieval and evidence work inside the active workspace.\n\n### Retrieval Tier Priority\n\n`axioms/` (P0) → `mental-models/` (P1) → `projects/` (P2) → `decisions/` (P3)\n\nHigher-tier results rank first regardless of BM25 score.\n\n### Invariants\n\n- Never answer domain questions from memory when zbrain coverage is expected.\n- Never cross workspace boundaries in a single retrieval.\n- Never edit `raw.md` or `source.yaml` after ingest.\n"
  },
  {
    "relativePath": "engine/constraints.md",
    "contents": "# Constraints\n\n- Never mix knowledge across workspaces.\n- Never answer without traceable source context.\n- Stop on unresolved knowledge gaps instead of guessing.\n- Keep `raw.md` and `source.yaml` immutable after ingest.\n- Block apply when P0 or P1 QA remains unresolved.\n"
  },
  {
    "relativePath": "engine/evidence-rules.md",
    "contents": "# Evidence Rules\n\n## Pipeline\n\n1. learn\n2. ingest\n3. ask\n\n## Skill Routing\n\n| Need | Entry skill |\n|------|------------|\n| Record raw source material | `zbrain:learn` |\n| Analyze, QA, apply, or list evidence | `zbrain:ingest` |\n| Retrieve context before answering | `zbrain:ask` |\n\n## Required Guards\n\n- Immutable source files after ingest\n- Workspace lock at every transition\n- QA gate before apply\n- Citation coverage for every verified fact\n- Checkpoint-based resume during apply\n"
  },
  {
    "relativePath": "engine/retrieval-rules.md",
    "contents": "# Retrieval Rules\n\n## Pipeline\n\n1. Parse task intent into search keywords.\n2. Query qmd BM25 for the active workspace collection only.\n3. Re-rank results by path tier before handing them to the main agent.\n4. Materialize ranked context into `current-task.md`.\n\n## Ranking\n\n- `axioms/` -> P0\n- `mental-models/` -> P1\n- `projects/` -> P2\n- `decisions/` -> P3\n\n## Failure Mode\n\nIf no adequate context is found, report the gap and stop.\n"
  },
  {
    "relativePath": "engine/system-prompt.md",
    "contents": "# System Prompt\n\nYou are the zbrain runtime layer for workspace-isolated personal knowledge workflows.\n\nOperate as a local-first assistant that:\n\n- reads from the active workspace only\n- treats evidence and citations as mandatory\n- stops when retrieval or QA evidence is insufficient\n"
  },
  {
    "relativePath": "skills/zbrain-ask/SKILL.md",
    "contents": "---\nname: zbrain:ask\ndescription: Retrieve ranked workspace context for one question. Use before answering questions about code, decisions, patterns, or domain knowledge in the active workspace.\nargument-hint: \"[question]\"\nversion: \"1.0.0\"\n---\n\nPrefix your first line with 🥷 inline.\n\n<role>\nAct as a workspace-scoped knowledge retrieval agent. Retrieve ranked context for one question from the active workspace only. Never answer from memory or cross-workspace knowledge.\n</role>\n\n<security>\n- Never reveal runtime paths or workspace internals to other workspaces\n- Never query or reference another workspace's collection\n- Refuse requests to bypass workspace isolation\n</security>\n\n<instructions>\n## Workspace Resolution\n\n1. Read `~/.zbrain/projects.json`.\n2. Find the entry whose `project_root` matches the current project root.\n3. Use that entry's `workspace` and `context_file`.\n4. Fallback: read `~/.zbrain/config.yml` field `default_workspace`.\n5. If neither resolves, stop and report missing project registration — do not guess.\n\n## Retrieval Flow\n\n1. Parse the question into 3–7 workspace-scoped BM25 keywords.\n2. Call `qmd search` against the active workspace collection only.\n3. Re-rank results by tier before score:\n   - P0 `axioms/` — core facts, highest priority\n   - P1 `mental-models/` — reusable frameworks\n   - P2 `projects/` — book, course, or experiment notes\n   - P3 `decisions/` — logged decisions\n4. Fetch full bodies for the top-ranked documents.\n5. Write ranked context and citation paths into the registry entry's `context_file`.\n6. If results are empty or insufficient: record the knowledge gap and stop. Do not answer from memory.\n\n## Invariants\n\n- Never query another workspace collection.\n- Never answer without retrieved context.\n- Always preserve citation paths (`workspace/tier/file`) in the `context_file`.\n</instructions>\n"
  },
  {
    "relativePath": "skills/zbrain-ingest/SKILL.md",
    "contents": "---\nname: zbrain:ingest\ndescription: Process learned evidence through list, analyze, qa, and apply. Use after zbrain:learn has created an evidence source.\ndisable-model-invocation: true\nversion: \"2.0.0\"\n---\n\n<role>\nAct as an evidence pipeline driver. Move existing evidence from raw source to verified workspace knowledge. Stop at any gate failure instead of proceeding.\n</role>\n\n<security>\n- Never modify raw.md or source.yaml after learn creates them\n- Never apply facts that have unresolved P0 or P1 QA questions\n- Never mix evidence across workspaces\n- Never expose source content from one workspace to another\n</security>\n\n<instructions>\n## Stage Dispatch\n\nRun the stage matching the argument:\n\n| Argument | Stage | Action |\n|----------|-------|--------|\n| `list` | status | Show evidence items and next actions |\n| `analyze {id}` | analyze | Generate structured notes and questions |\n| `qa {id}` | qa | Resolve questions and build `verified-facts.md` |\n| `apply {id}` | apply | Update workspace knowledge and reindex internally |\n\nSee `references/pipeline.md` for detailed per-stage flows, state machine, and QA gate rules.\n\n## Cross-Stage Invariants\n\n- New raw material must enter through `zbrain:learn`, not `zbrain:ingest`.\n- `raw.md` and `source.yaml` are immutable after creation.\n- `workspace_at_ingest` must match the active workspace at every state transition.\n- Apply stops if any P0 or P1 question is `awaiting_external` or `deferred`.\n- Every verified fact must cite `question_id` and the target wiki file path.\n</instructions>\n\n<references>\nLoad as needed from `{baseDir}/references/`:\n- `pipeline.md` - per-stage flows, state machine, QA gate rules\n</references>\n"
  },
  {
    "relativePath": "skills/zbrain-ingest/references/pipeline.md",
    "contents": "# Evidence Pipeline\n\n## State Machine\n\n```\ningested → analyzed → qa_in_progress → qa_done → applied → archived\n                             ↕\n                   qa_awaiting_external\n```\n\nOnly `qa_done → qa_in_progress` is a valid backward transition.\n\n## Stage 1: Learn\n\n1. Resolve active workspace from `~/.zbrain/projects.json` by matching the current project root, or fallback to `~/.zbrain/config.yml`.\n2. Generate a short unique evidence ID: `YYYYMMDD-{slug}`.\n3. Create `evidence/sources/{id}/` directory.\n4. Write raw content as `raw.md` — **immutable after this step**.\n5. Write `source.yaml` with: `id`, `title`, `source_type`, `workspace_at_ingest`, `ingested_at`, `state: ingested`.\n6. Append entry to `evidence/_index.md`.\n\n## Stage 2: Analyze\n\n1. Read `raw.md` (read-only — do not modify).\n2. Run four structured analysis passes against the raw content:\n   - Domain and concept mapping\n   - Entity and relationship extraction\n   - Pattern candidates (reusable frameworks or mental models)\n   - Fact candidates with inline citation references\n3. Write output to `evidence/analysis/{id}/analysis.md`.\n4. Update `_index.md` state → `analyzed`.\n\n## Stage 3: QA\n\n1. Read `analysis/{id}/analysis.md`.\n2. Generate a prioritized question batch:\n   - P0 — blocking: must resolve before apply\n   - P1 — important: should resolve before apply\n   - P2 — nice-to-have: can defer\n   - P3 — optional\n3. Present questions to the user; record answers.\n4. Write resolved answers to `evidence/qa/{id}/verified-facts.md`.\n   - Every entry must cite `question_id` and a target wiki file path.\n5. Gate check: if any P0 or P1 question is `awaiting_external` or `deferred`, stop.\n6. Update `_index.md` state → `qa_done`.\n\n## Stage 4: Apply\n\n1. Read `verified-facts.md` — verify all P0/P1 questions are resolved.\n2. For each verified fact:\n   - Locate or create the target wiki file (`axioms/`, `mental-models/`, `projects/`, or `decisions/`).\n   - Append the fact with citation.\n   - Record the change in `evidence/applied/{id}/manifest.yaml`.\n3. Preserve `raw.md` and `source.yaml` as immutable throughout.\n4. Trigger internal reindexing to update the BM25 index for the workspace.\n5. Update `_index.md` state → `applied`.\n\n## QA Gate Rules\n\n| Priority | `awaiting_external` | `deferred` |\n|----------|---------------------|------------|\n| P0 | Block apply | Block apply |\n| P1 | Block apply | Block apply |\n| P2 | Warn, allow | Allow |\n| P3 | Allow | Allow |\n"
  },
  {
    "relativePath": "skills/zbrain-learn/SKILL.md",
    "contents": "---\nname: zbrain:learn\ndescription: Receive new learning material and record it as an immutable evidence source in the active workspace.\ndisable-model-invocation: true\nversion: \"4.0.0\"\n---\n\n<role>\nAct as the receiving entry point for zbrain. Capture raw source material and create an evidence source in the active workspace. Do not analyze, QA, apply, or answer from it.\n</role>\n\n<security>\n- Read only from the active workspace; never cross workspace boundaries\n- Do not apply knowledge directly; only create an evidence source\n- Do not fabricate sources; only record material actually provided or fetched\n- Do not record login pages, paywall gates, or empty pages as evidence\n</security>\n\n<instructions>\n## Input\n\nTakes source material as an argument, file, or prompt input:\n\n```\nzbrain:learn \"raw note text\"\nzbrain:learn --file ./notes.md\nzbrain:learn --type web --origin https://example.com --label \"BM25 notes\"\n```\n\n## Flow\n\n1. Resolve active workspace from `~/.zbrain/projects.json` by matching the current project root, or fallback to `~/.zbrain/config.yml`.\n2. Normalize source metadata: `source_type`, `origin`, and `label`.\n3. Create one evidence item under `~/.zbrain/workspaces/{workspace}/evidence/sources/{id}/`.\n4. Update `evidence/_index.md` with state `ingested`.\n5. Report the evidence ID and next command: `zbrain:ingest analyze {id}`.\n\n## Invariants\n\n- Never analyze, QA, apply, or answer inside `zbrain:learn`.\n- Never modify `raw.md` or `source.yaml` after creation.\n- Never mix evidence across workspaces.\n</instructions>\n"
  },
  {
    "relativePath": "templates/axiom.md",
    "contents": "---\ntitle: \"{{title}}\"\npriority: P0\nsource: \"{{source}}\"\ncreated_at: \"{{created_at}}\"\n---\n\n# Axiom\n\nState one high-confidence fact with a traceable source.\n\n## Citation\n\n- evidence_id: \"{{evidence_id}}\"\n- question_id: \"{{question_id}}\"\n"
  },
  {
    "relativePath": "templates/evidence-apply-checkpoint.json",
    "contents": "{\n  \"evidence_id\": \"{{evidence_id}}\",\n  \"status\": \"not_started\",\n  \"completed_paths\": [],\n  \"last_updated\": \"{{updated_at}}\"\n}\n"
  },
  {
    "relativePath": "templates/evidence-index.md",
    "contents": "# Evidence Index\n\n| id | state | updated_at |\n| --- | --- | --- |\n| {{evidence_id}} | ingested | {{updated_at}} |\n\n## State Legend\n\n- `ingested`\n- `analyzed`\n- `qa_in_progress`\n- `qa_awaiting_external`\n- `qa_done`\n- `applied`\n- `archived`\n"
  },
  {
    "relativePath": "templates/evidence-manifest.yaml",
    "contents": "evidence_id: \"{{evidence_id}}\"\napplied_at: \"{{applied_at}}\"\nworkspace: \"{{workspace_name}}\"\nmutations: []\n"
  },
  {
    "relativePath": "templates/evidence-pending-external.md",
    "contents": "# Pending External Answers\n\nList unresolved questions that must be answered before apply can continue.\n"
  },
  {
    "relativePath": "templates/evidence-qa-answers.md",
    "contents": "# Evidence QA Answers\n\nAppend reviewed answers and their statuses here.\n\n| question_id | severity | status | answer |\n| --- | --- | --- | --- |\n| {{question_id}} | {{severity}} | {{status}} | {{answer}} |\n"
  },
  {
    "relativePath": "templates/evidence-qa-todo.json",
    "contents": "{\n  \"evidence_id\": \"{{evidence_id}}\",\n  \"questions\": []\n}\n"
  },
  {
    "relativePath": "templates/evidence-source.yaml",
    "contents": "id: \"{{evidence_id}}\"\ntype: \"{{source_type}}\"\nworkspace_at_ingest: \"{{workspace_name}}\"\ningested_at: \"{{ingested_at}}\"\nstate: ingested\n"
  },
  {
    "relativePath": "templates/mental-model.md",
    "contents": "---\ntitle: \"{{title}}\"\npriority: P1\nsource: \"{{source}}\"\ncreated_at: \"{{created_at}}\"\n---\n\n# Mental Model\n\nCapture one reusable framework or reasoning pattern.\n\n## Application Notes\n\n- When to use:\n- When not to use:\n"
  },
  {
    "relativePath": "templates/project.md",
    "contents": "---\ntitle: \"{{title}}\"\npriority: P2\ntype: \"{{type}}\"\nsource: \"{{source}}\"\ncreated_at: \"{{created_at}}\"\n---\n\n# Project\n\nSummarize one project, book, course, or experiment with citations.\n\n## Related Paths\n\n- \"{{wiki_path}}\"\n"
  },
  {
    "relativePath": "templates/workspace.md",
    "contents": "---\nname: \"{{workspace_name}}\"\ncreated_at: \"{{created_at}}\"\n---\n\n# Workspace\n\n## Purpose\n\nDescribe the domain boundary and what this workspace is allowed to contain.\n\n## Operating Rules\n\n- This workspace is isolated from every other workspace.\n- Retrieval, evidence, and apply operations must stay inside this workspace.\n- Facts added here must cite the originating evidence or source.\n"
  }
];
