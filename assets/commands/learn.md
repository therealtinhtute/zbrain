---
name: zbrain:learn
description: Run the four-stage evidence pipeline for the active workspace.
disable-model-invocation: true
---

# zbrain:learn

## Purpose

Drive the four-stage evidence pipeline for the active workspace.

## Stages

1. `ingest` creates immutable source files under `evidence/sources/{id}/`
2. `analyze` generates structured notes and questions
3. `qa` resolves questions and produces `verified-facts.md`
4. `apply` updates workspace knowledge and triggers reindex

## Invariants

- `raw.md` and `source.yaml` are immutable after ingest.
- `workspace_at_ingest` must match the active workspace at every stage.
- Apply stops if any P0 or P1 question is `awaiting_external` or `deferred`.
- Every verified fact must cite `question_id` and target wiki path.
