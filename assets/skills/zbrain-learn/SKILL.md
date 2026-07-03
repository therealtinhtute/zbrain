---
name: zbrain:learn
description: Receive new learning material and record it as an immutable evidence source in the active workspace.
version: "4.0.0"
---

<role>
Act as the receiving entry point for zbrain. Capture raw source material and create an evidence source in the active workspace. Do not analyze, QA, apply, or answer from it.
</role>

<security>
- Read only from the active workspace; never cross workspace boundaries
- Do not apply knowledge directly; only create an evidence source
- Do not fabricate sources; only record material actually provided or fetched
- Do not record login pages, paywall gates, or empty pages as evidence
</security>

<instructions>
## Input

Takes source material as an argument, file, or prompt input:

```
zbrain:learn "raw note text"
zbrain:learn --file ./notes.md
zbrain:learn --type web --origin https://example.com --label "BM25 notes"
```

## Flow

1. Resolve active workspace by running `zbrain workspace current` (JSON output), or fallback to `~/.zbrain/config.yml`.
2. Normalize source metadata: `source_type`, `origin`, and `label`.
3. Create one evidence item under `~/.zbrain/workspaces/{workspace}/evidence/sources/{id}/`.
4. Update `evidence/_index.md` with state `ingested`.
5. Report the evidence ID and next command: `zbrain:ingest {id}`.

## Invariants

- Never analyze, QA, apply, or answer inside `zbrain:learn`.
- Never modify `raw.md` or `source.yaml` after creation.
- Never mix evidence across workspaces.
</instructions>
