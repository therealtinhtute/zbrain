# Phase Plan: P5 — Evidence Pipeline

Inputs: P2 (state machine), P4 (slash command files)
Depends on: P2, P4

---

## Wave 1: Evidence Templates

### Task 1.1: Finalize evidence templates
- Review and finalize assets/templates/evidence-index.md:
  - Table format: | ID | Source | State | Ingested | Updated |
  - Instructions for agent: "This file is the authoritative state tracker. Update state column on every transition."
- Review assets/templates/evidence-source.yaml:
  - Fields: id, type (text|book|article|experience|code), title, workspace_at_ingest, ingested_at, state, tags
  - Include comment: "# IMMUTABLE — do not modify after ingest"
- **Verification**: Templates parse correctly with js-yaml / markdown parser
- **Touched**: assets/templates/evidence-index.md, assets/templates/evidence-source.yaml

---

## Wave 2: Analysis Prompts

### Task 2.1: Define 4 CORE analysis prompts in learn.md
- Update the --analyze section of assets/commands/learn.md with detailed prompts:
  - **01-summary**: "Summarize the key claims, concepts, and frameworks from this source. List 5-10 bullet points."
  - **02-contradiction**: "Compare this source against existing wiki entries in {workspace}. List any contradictions, noting the conflicting wiki entry path."
  - **04-questions**: "Generate questions to verify understanding. Categorize as P0 (core fact verification), P1 (framework application), P2 (project-specific), P3 (decision context). At least 2 questions per priority level."
  - **08-gaps**: "What knowledge is missing? What does this source assume but not explain? What follow-up sources would fill the gaps?"
- **Verification**: Each prompt is specific, references workspace context, produces structured output
- **Touched**: assets/commands/learn.md (--analyze section)

---

## Wave 3: QA + Apply Refinement

### Task 3.1: QA batching rules in learn.md
- Update --qa section with detailed batching:
  - Present P0 questions first (critical — block apply if unanswered)
  - Then P1 (important — block apply if unanswered)
  - Then P2, P3 (can be deferred without blocking)
  - Each answer recorded in batch-N-answers.md with format: `### Q{id}\n**Question**: ...\n**Answer**: ...\n**Confidence**: high|medium|low`
  - verified-facts.md format: `### F{n}\n**Fact**: ...\n**Source**: Q{id} from evidence {evidence_id}\n**Applies to**: {wiki_path}\n**Action**: create|update|validate`
- **Verification**: Format examples in learn.md are copy-pasteable
- **Touched**: assets/commands/learn.md (--qa section)

### Task 3.2: Apply + checkpoint logic in learn.md
- Update --apply section with:
  - Read verified-facts.md, process each fact
  - For each "create" action: use appropriate template (axiom.md, mental-model.md, project.md)
  - For each "update" action: read existing file, merge fact, preserve existing citations
  - Write checkpoint.json after each file: { processed: ["F1", "F2"], remaining: ["F3"], last_file: "axioms/new-fact.md" }
  - Write manifest.yaml: { evidence_id, applied_at, files_created: [], files_updated: [], facts_applied: N }
  - After all facts: trigger `qmd --config-name zwiki index --collection {workspace}`
  - Resume logic: if checkpoint.json exists, skip already-processed facts
- **Verification**: Checkpoint format documented, resume scenario described
- **Touched**: assets/commands/learn.md (--apply section)

---

## Wave 4: Integration Test

### Task 4.1: End-to-end evidence test script
- Create tests/evidence-e2e.md — manual test script:
  1. `zwiki workspace create test-evidence`
  2. Create a sample text file (3 paragraphs about a programming concept)
  3. `/learn <sample.txt>` → verify sources/{id}/ created
  4. `/learn --analyze {id}` → verify 4 analysis files
  5. `/learn --qa {id}` → answer questions → verify verified-facts.md
  6. `/learn --apply {id}` → verify wiki entry created + qmd reindex
  7. Attempt re-ingest of same source → should work (new ID)
  8. Attempt analyze in different workspace → should fail (I-2)
- **Verification**: Test script covers all 4 stages + invariant violations
- **Touched**: tests/evidence-e2e.md

---

## Stop Conditions
- Analysis prompts produce low-quality output → iterate prompt wording
- QA batching unclear → simplify to sequential (no batching)

## Escalation
- If agent doesn't follow invariant rules reliably → add explicit validation in TypeScript (move some logic from slash command to CLI)
