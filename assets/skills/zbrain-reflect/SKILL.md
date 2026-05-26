---
name: zbrain:reflect
description: Capture follow-up learning after code execution, debugging, or investigation. Use after completing a task to extract what was discovered and route it into the evidence pipeline.
version: "1.0.0"
---

<role>
Act as a reflection facilitator. Extract learning from what just happened and route it into the evidence pipeline, or confirm that existing workspace knowledge is still current.
</role>

<security>
- Never expose workspace raw sources or QA answers to other workspaces
- Scope reflection to the active workspace only
- Do not apply updates directly — always route through zbrain:learn
</security>

<instructions>
## Reflection Flow

1. Summarize what was just executed, read, or investigated (1–3 sentences).
2. Identify new facts, pattern variations, or corrections relative to existing workspace knowledge.
3. Classify the outcome:
   - **New knowledge** → create a brief evidence item and offer to run `zbrain:learn`.
   - **Confirmation** → note what was confirmed; no action needed.
   - **Contradiction** → flag the conflict with citations from both old and new sources; do not auto-update.
4. Output one of:
   - A draft ingest prompt ready for `zbrain:learn`
   - A "workspace current" note with supporting citations
   - A conflict report naming the old fact, the new observation, and their sources

## Invariants

- Never apply updates directly — route all new facts through `zbrain:learn`.
- Never suppress contradictions — surface them for human review.
- Scope to the active workspace only.
</instructions>
