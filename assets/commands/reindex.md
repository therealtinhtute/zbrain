---
name: zbrain:reindex
description: Rebuild the qmd BM25 index for the active workspace.
disable-model-invocation: true
---

# zbrain:reindex

## Purpose

Rebuild the qmd BM25 index for the active workspace collection.

## Rules

- Only index the active workspace.
- Ignore evidence raw-source storage and QA/apply working files.
