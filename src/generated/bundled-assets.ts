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
    "relativePath": "commands/ask.md",
    "contents": "---\nname: zbrain:ask\ndescription: Retrieve ranked workspace context for one question.\nargument-hint: \"[question]\"\n---\n\n# zbrain:ask\n\n## Purpose\n\nRetrieve ranked context for one question from the active workspace only.\n\n## Inputs\n\n- `question`: free-form question text\n- active workspace from `<cwd>/.claude/zbrain.json` or `~/.zbrain/config.yml`\n\n## Flow\n\n1. Parse the question into workspace-scoped search keywords.\n2. Query qmd BM25 against the active workspace collection only.\n3. Re-rank results by tier:\n   - P0 `axioms/`\n   - P1 `mental-models/`\n   - P2 `projects/`\n   - P3 `decisions/`\n4. Write the ranked context into `current-task.md`.\n5. Stop if the result set has a knowledge gap instead of guessing.\n\n## Invariants\n\n- Never query another workspace collection.\n- Never answer without retrieved context.\n- Always preserve citation paths in the generated context file.\n"
  },
  {
    "relativePath": "commands/learn.md",
    "contents": "---\nname: zbrain:learn\ndescription: Run the four-stage evidence pipeline for the active workspace.\ndisable-model-invocation: true\n---\n\n# zbrain:learn\n\n## Purpose\n\nDrive the four-stage evidence pipeline for the active workspace.\n\n## Stages\n\n1. `ingest` creates immutable source files under `evidence/sources/{id}/`\n2. `analyze` generates structured notes and questions\n3. `qa` resolves questions and produces `verified-facts.md`\n4. `apply` updates workspace knowledge and triggers reindex\n\n## Invariants\n\n- `raw.md` and `source.yaml` are immutable after ingest.\n- `workspace_at_ingest` must match the active workspace at every stage.\n- Apply stops if any P0 or P1 question is `awaiting_external` or `deferred`.\n- Every verified fact must cite `question_id` and target wiki path.\n"
  },
  {
    "relativePath": "commands/reflect.md",
    "contents": "---\nname: zbrain:reflect\ndescription: Capture follow-up learning after execution or investigation.\n---\n\n# zbrain:reflect\n\n## Purpose\n\nCapture follow-up learning after reading, debugging, or task execution.\n\n## Expected Outcome\n\n- a new evidence item ready for `zbrain:learn`\n- or a note that the current workspace knowledge is still sufficient\n"
  },
  {
    "relativePath": "commands/reindex.md",
    "contents": "---\nname: zbrain:reindex\ndescription: Rebuild the qmd BM25 index for the active workspace.\ndisable-model-invocation: true\n---\n\n# zbrain:reindex\n\n## Purpose\n\nRebuild the qmd BM25 index for the active workspace collection.\n\n## Rules\n\n- Only index the active workspace.\n- Ignore evidence raw-source storage and QA/apply working files.\n"
  },
  {
    "relativePath": "commands/workspace.md",
    "contents": "---\nname: zbrain:workspace\ndescription: Inspect or switch the active workspace pointer.\ndisable-model-invocation: true\n---\n\n# zbrain:workspace\n\n## Purpose\n\nInspect or switch the active workspace pointer for the current project.\n\n## Sources\n\n- project pointer: `<cwd>/.claude/zbrain.json`\n- fallback default: `~/.zbrain/config.yml`\n\n## Rules\n\n- Switching only changes the pointer for the current project.\n- Do not borrow knowledge across workspaces.\n"
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
