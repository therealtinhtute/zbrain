---
title: "zbrain Trusted Agent Gateway — Architecture Brainstorm"
description: "Provisional owner-led mapping from the frozen knowledge architecture synthesis to a future zbrain trust gateway initiative."
source_title: "zbrain architecture brainstorm"
source_url: "https://github.com/therealtinhtute/zbrain/tree/master/references"
source_kind: "provisional product brainstorm"
source_notes:
  - "references/knowledge-architecture-synthesis.md"
  - "trusted-memory-spec.md"
accessed_at: "2026-08-13"
fetch_method: "owner roadmap plus frozen local comparative synthesis"
status: provisional
approval_state: owner-roadmap-approved-for-planning; implementation-not-yet-approved
tags: [zbrain, trusted-memory, agent-gateway, mcp, approvals, brainstorm]
---

# zbrain Trusted Agent Gateway — Architecture Brainstorm

## Boundary

Roadmap này records a planning direction, not completed product behavior. Any implementation must start a separate high-risk initiative and amend the authoritative trusted-memory spec first. Existing CLI, canonical Markdown, evidence snapshots and fail-closed query behavior remain mandatory invariants.

## Candidate adaptations

| Idea | State | Why |
|---|---|---|
| Typed MCP stdio gateway over shared runtime services | candidate for gateway initiative | Copies llmwiki's shared CLI/MCP service shape while keeping transport local and narrow. |
| Evidence capture → draft → owner-pinned lifecycle | candidate for gateway initiative | Combines review queue revalidation with zbrain's stronger trust boundary. |
| One-time challenge/token for approve, supersede and revoke | candidate for gateway initiative | Closes the corpus gap around reusable environment grants and binds action context. |
| Cryptographic evidence spans | candidate for gateway initiative | Makes citation coordinates verifiable against immutable raw snapshots. |
| Shared status/doctor health model | candidate for gateway initiative | Copies explicit degraded/unknown/recovery behavior from llmwiki operations. |
| Optional hybrid retrieval in one disposable SQLite index | candidate for later slice | Adapts lexical fallback and avoids Uteke's separate vector-file desync. |
| Loopback read-only viewer | candidate for later slice | Keeps local inspectability without adding mutation API or remote auth. |

## Explicitly rejected imports

- Internal knowledge compiler or completion/chat LLM in zbrain core.
- API-key storage, provider adapters or hidden model calls.
- Auto-approve, auto-reindex after mutation, or saved query answers as trusted claims.
- Broad CLP/profile framework, graph editor, hosted sync, team authorization or remote MCP in gateway v1.
- URL crawling or arbitrary local-file fetch through MCP.
- Separate vector database/file and stale cached text fallback.
- Windows portability in the first gateway initiative.

## Decisions that are planning-approved, not implementation-approved

1. The next implementation initiative may be named `trusted-agent-gateway` and classified high-risk.
2. MCP transport starts with stdio only; stdout protocol purity and stderr diagnostics are required.
3. Lifecycle mutations require owner presence through a short-lived, one-time challenge grant; MCP never grants itself approval.
4. Canonical Markdown remains the source of truth; index, embeddings, challenges and viewer state are derived/runtime artifacts.
5. Hybrid retrieval and viewer are optional milestones that must not weaken lexical/trust behavior.

## Unresolved owner questions

- Is `owner:mcp` sufficient attribution for the local owner ceremony?
- Should challenge summaries include claim body excerpts or only title/IDs/digests to minimize leakage?
- What is the acceptable evidence span/body exposure policy for the viewer?
- Should optional embeddings be enabled by config only or require an explicit CLI flag per rebuild?
- Is 1 MiB the right capture cap for all agent clients?

## Separate lifecycle boundary

Implementation of any candidate above requires a new brainstorm/to-plan lifecycle. This artifact must not be read as permission to edit runtime code, add dependencies, or publish a release.
