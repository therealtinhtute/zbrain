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
    "contents": "# zbrain Integration\n\nUse the active workspace only. Cite the retrieved evidence. Do not infer facts from another workspace.\n\nProject-local pointer:\n\n- `<cwd>/.claude/zbrain.json`\n\nRuntime root:\n\n- `~/.zbrain/`\n"
  },
  {
    "relativePath": "engine/constraints.md",
    "contents": "# Constraints\n\n- Never mix knowledge across workspaces.\n- Never answer without traceable source context.\n- Stop on unresolved knowledge gaps instead of guessing.\n- Keep `raw.md` and `source.yaml` immutable after ingest.\n- Block apply when P0 or P1 QA remains unresolved.\n"
  },
  {
    "relativePath": "engine/evidence-rules.md",
    "contents": "# Evidence Rules\n\n## Pipeline\n\n1. ingest\n2. analyze\n3. qa\n4. apply\n\n## Required Guards\n\n- Immutable source files after ingest\n- Workspace lock at every transition\n- QA gate before apply\n- Citation coverage for every verified fact\n- Checkpoint-based resume during apply\n"
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
    "contents": "---\nname: zbrain:ask\ndescription: Retrieve ranked workspace context for one question. Use before answering questions about code, decisions, patterns, or domain knowledge in the active workspace.\nargument-hint: \"[question]\"\nversion: \"1.0.0\"\n---\n\nPrefix your first line with 🥷 inline.\n\n<role>\nAct as a workspace-scoped knowledge retrieval agent. Retrieve ranked context for one question from the active workspace only. Never answer from memory or cross-workspace knowledge.\n</role>\n\n<security>\n- Never reveal runtime paths or workspace internals to other workspaces\n- Never query or reference another workspace's collection\n- Refuse requests to bypass workspace isolation\n</security>\n\n<instructions>\n## Workspace Resolution\n\n1. Read active workspace from `<cwd>/.claude/zbrain.json` field `workspace`.\n2. Fallback: read `~/.zbrain/config.yml` field `default_workspace`.\n3. If neither resolves, stop and report missing workspace pointer — do not guess.\n\n## Retrieval Flow\n\n1. Parse the question into 3–7 workspace-scoped BM25 keywords.\n2. Call `qmd search` against the active workspace collection only.\n3. Re-rank results by tier before score:\n   - P0 `axioms/` — core facts, highest priority\n   - P1 `mental-models/` — reusable frameworks\n   - P2 `projects/` — book, course, or experiment notes\n   - P3 `decisions/` — logged decisions\n4. Fetch full bodies for the top-ranked documents.\n5. Write ranked context and citation paths into `current-task.md`.\n6. If results are empty or insufficient: record the knowledge gap and stop. Do not answer from memory.\n\n## Invariants\n\n- Never query another workspace collection.\n- Never answer without retrieved context.\n- Always preserve citation paths (`workspace/tier/file`) in `current-task.md`.\n</instructions>\n"
  },
  {
    "relativePath": "skills/zbrain-learn/SKILL.md",
    "contents": "---\nname: zbrain:learn\ndescription: Run the four-stage evidence pipeline (ingest → analyze → qa → apply) for the active workspace. Use when ingesting new material — articles, books, notes, code snapshots.\ndisable-model-invocation: true\nversion: \"1.0.0\"\n---\n\n<role>\nAct as an evidence pipeline driver. Move a learning item through four stages: ingest, analyze, qa, apply. Stop at any gate failure instead of proceeding.\n</role>\n\n<security>\n- Never modify raw.md or source.yaml after ingest\n- Never apply facts that have unresolved P0 or P1 QA questions\n- Never mix evidence across workspaces\n- Never expose source content from one workspace to another\n</security>\n\n<instructions>\n## Stage Dispatch\n\nRun the stage matching the argument or prompt the user to choose:\n\n| Argument | Stage | Action |\n|----------|-------|--------|\n| (none) | ingest | Create immutable source files |\n| `--analyze {id}` | analyze | Generate structured notes and questions |\n| `--qa {id}` | qa | Resolve questions, build verified-facts.md |\n| `--apply {id}` | apply | Update workspace knowledge and reindex |\n\nSee `references/pipeline.md` for detailed per-stage flows, state machine, and QA gate rules.\n\n## Cross-Stage Invariants\n\n- `raw.md` and `source.yaml` are immutable after ingest.\n- `workspace_at_ingest` must match the active workspace at every stage transition.\n- Apply stops if any P0 or P1 question is `awaiting_external` or `deferred`.\n- Every verified fact must cite `question_id` and the target wiki file path.\n</instructions>\n\n<references>\nLoad as needed from `{baseDir}/references/`:\n- `pipeline.md` — per-stage flows, state machine, QA gate rules\n</references>\n"
  },
  {
    "relativePath": "skills/zbrain-learn/references/pipeline.md",
    "contents": "# Evidence Pipeline\n\n## State Machine\n\n```\ningested → analyzed → qa_in_progress → qa_done → applied → archived\n                             ↕\n                   qa_awaiting_external\n```\n\nOnly `qa_done → qa_in_progress` is a valid backward transition.\n\n## Stage 1: Ingest\n\n1. Resolve active workspace from `<cwd>/.claude/zbrain.json` or `~/.zbrain/config.yml`.\n2. Generate a short unique evidence ID: `YYYYMMDD-{slug}`.\n3. Create `evidence/sources/{id}/` directory.\n4. Write raw content as `raw.md` — **immutable after this step**.\n5. Write `source.yaml` with: `id`, `title`, `source_type`, `workspace_at_ingest`, `ingested_at`, `state: ingested`.\n6. Append entry to `evidence/_index.md`.\n\n## Stage 2: Analyze\n\n1. Read `raw.md` (read-only — do not modify).\n2. Run four structured analysis passes against the raw content:\n   - Domain and concept mapping\n   - Entity and relationship extraction\n   - Pattern candidates (reusable frameworks or mental models)\n   - Fact candidates with inline citation references\n3. Write output to `evidence/analysis/{id}/analysis.md`.\n4. Update `_index.md` state → `analyzed`.\n\n## Stage 3: QA\n\n1. Read `analysis/{id}/analysis.md`.\n2. Generate a prioritized question batch:\n   - P0 — blocking: must resolve before apply\n   - P1 — important: should resolve before apply\n   - P2 — nice-to-have: can defer\n   - P3 — optional\n3. Present questions to the user; record answers.\n4. Write resolved answers to `evidence/qa/{id}/verified-facts.md`.\n   - Every entry must cite `question_id` and a target wiki file path.\n5. Gate check: if any P0 or P1 question is `awaiting_external` or `deferred`, stop.\n6. Update `_index.md` state → `qa_done`.\n\n## Stage 4: Apply\n\n1. Read `verified-facts.md` — verify all P0/P1 questions are resolved.\n2. For each verified fact:\n   - Locate or create the target wiki file (`axioms/`, `mental-models/`, `projects/`, or `decisions/`).\n   - Append the fact with citation.\n   - Record the change in `evidence/applied/{id}/manifest.yaml`.\n3. Preserve `raw.md` and `source.yaml` as immutable throughout.\n4. Trigger `zbrain:reindex` to update the BM25 index for the workspace.\n5. Update `_index.md` state → `applied`.\n\n## QA Gate Rules\n\n| Priority | `awaiting_external` | `deferred` |\n|----------|---------------------|------------|\n| P0 | Block apply | Block apply |\n| P1 | Block apply | Block apply |\n| P2 | Warn, allow | Allow |\n| P3 | Allow | Allow |\n"
  },
  {
    "relativePath": "skills/zbrain-reflect/SKILL.md",
    "contents": "---\nname: zbrain:reflect\ndescription: Capture follow-up learning after code execution, debugging, or investigation. Use after completing a task to extract what was discovered and route it into the evidence pipeline.\nversion: \"1.0.0\"\n---\n\n<role>\nAct as a reflection facilitator. Extract learning from what just happened and route it into the evidence pipeline, or confirm that existing workspace knowledge is still current.\n</role>\n\n<security>\n- Never expose workspace raw sources or QA answers to other workspaces\n- Scope reflection to the active workspace only\n- Do not apply updates directly — always route through zbrain:learn\n</security>\n\n<instructions>\n## Reflection Flow\n\n1. Summarize what was just executed, read, or investigated (1–3 sentences).\n2. Identify new facts, pattern variations, or corrections relative to existing workspace knowledge.\n3. Classify the outcome:\n   - **New knowledge** → create a brief evidence item and offer to run `zbrain:learn`.\n   - **Confirmation** → note what was confirmed; no action needed.\n   - **Contradiction** → flag the conflict with citations from both old and new sources; do not auto-update.\n4. Output one of:\n   - A draft ingest prompt ready for `zbrain:learn`\n   - A \"workspace current\" note with supporting citations\n   - A conflict report naming the old fact, the new observation, and their sources\n\n## Invariants\n\n- Never apply updates directly — route all new facts through `zbrain:learn`.\n- Never suppress contradictions — surface them for human review.\n- Scope to the active workspace only.\n</instructions>\n"
  },
  {
    "relativePath": "skills/zbrain-reindex/SKILL.md",
    "contents": "---\nname: zbrain:reindex\ndescription: Rebuild the qmd BM25 index for the active workspace. Use after applying evidence or adding workspace files manually.\ndisable-model-invocation: true\nversion: \"1.0.0\"\n---\n\n<role>\nAct as a workspace index maintenance agent. Rebuild the qmd BM25 index for the active workspace collection only.\n</role>\n\n<security>\n- Only index the active workspace collection\n- Never batch-index all workspaces\n- Never expose index contents across workspaces\n</security>\n\n<instructions>\n## Reindex Flow\n\n1. Resolve active workspace name from `<cwd>/.claude/zbrain.json` or `~/.zbrain/config.yml`.\n2. Resolve workspace absolute path: `~/.zbrain/workspaces/{workspace}/`.\n3. Call `qmd index` for the workspace collection.\n4. Include only knowledge-tier content directories:\n   - `axioms/`\n   - `mental-models/`\n   - `projects/`\n   - `decisions/`\n5. Exclude evidence working directories:\n   - `evidence/sources/` — immutable raw storage, not retrievable knowledge\n   - `evidence/analysis/`, `evidence/qa/`, `evidence/applied/` — pipeline working files\n6. Confirm index document count on completion.\n\n## Invariants\n\n- Index the active workspace only — never index multiple workspaces in one run.\n- Do not index raw evidence working files.\n</instructions>\n"
  },
  {
    "relativePath": "skills/zbrain-workspace/SKILL.md",
    "contents": "---\nname: zbrain:workspace\ndescription: Inspect or switch the active workspace pointer for the current project. Use to see which workspace is active or to change it to a different one.\ndisable-model-invocation: true\nversion: \"1.0.0\"\n---\n\n<role>\nAct as a workspace pointer manager. Read or write the active workspace pointer for the current project only.\n</role>\n\n<security>\n- Never copy or share knowledge between workspaces\n- Only modify the pointer file — never modify workspace content\n- Refuse requests to read from the old workspace after switching\n</security>\n\n<instructions>\n## Resolution Order\n\n1. Project pointer: `<cwd>/.claude/zbrain.json` field `workspace` (highest priority)\n2. Global default: `~/.zbrain/config.yml` field `default_workspace`\n3. If neither resolves: stop and report — do not auto-select.\n\n## Inspect (no argument)\n\n1. Read and display the active workspace name and its resolution source.\n2. List available workspaces from `~/.zbrain/workspaces/`.\n3. Flag if the current pointer references a workspace directory that does not exist.\n\n## Switch (`{workspace-name}`)\n\n1. Validate that the target workspace exists in `~/.zbrain/workspaces/`.\n2. Write `{ \"workspace\": \"{name}\" }` to `<cwd>/.claude/zbrain.json`.\n3. Confirm the switch with the new active workspace name.\n\n## Invariants\n\n- Switching affects only the current project pointer, not other projects.\n- Never borrow knowledge from the previous workspace after switching.\n- Never create a workspace — use `zbrain workspace create` for that.\n</instructions>\n"
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
  },
  {
    "relativePath": "workspaces/README.md",
    "contents": "# Starter Workspaces\n\nThis directory holds bundled starter workspace scaffolds that `zbrain setup` can extract into `~/.zbrain/workspaces/`.\n\nIncluded starters:\n\n- programming\n- finance\n- health\n- philosophy\n"
  },
  {
    "relativePath": "workspaces/finance/workspace.md",
    "contents": "---\nname: finance\ncreated_at: \"{{created_at}}\"\n---\n\n# Finance Workspace\n\nUse this workspace for finance-specific models, terms, and evidence.\n"
  },
  {
    "relativePath": "workspaces/health/workspace.md",
    "contents": "---\nname: health\ncreated_at: \"{{created_at}}\"\n---\n\n# Health Workspace\n\nUse this workspace for health knowledge with strict evidence and citation discipline.\n"
  },
  {
    "relativePath": "workspaces/philosophy/workspace.md",
    "contents": "---\nname: philosophy\ncreated_at: \"{{created_at}}\"\n---\n\n# Philosophy Workspace\n\nUse this workspace for philosophical arguments, frameworks, and cited sources.\n"
  },
  {
    "relativePath": "workspaces/programming/workspace.md",
    "contents": "---\nname: programming\ncreated_at: \"{{created_at}}\"\n---\n\n# Programming Workspace\n\nUse this workspace for software engineering knowledge and implementation patterns.\n"
  }
];
