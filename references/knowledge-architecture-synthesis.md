---
title: "Knowledge Architecture Comparative Synthesis"
description: "Provisional comparison of five frozen local-first knowledge architecture sources across structure, lifecycle, trust, retrieval, security, correction, portability, cost, and scale."
source_title: "Frozen five-source corpus"
source_url: "https://github.com/therealtinhtute/zbrain/tree/master/references"
source_kind: "comparative synthesis of source-grounded local analyses"
source_notes:
  - "references/okf-v0.2-analysis.md"
  - "references/karpathy-llm-wiki-analysis.md"
  - "references/entity-vault-analysis.md"
  - "references/uteke-analysis.md"
  - "references/atomicstrata-llm-wiki-compiler-analysis.md"
accessed_at: "2026-08-13"
fetch_method: "synthesis from frozen local notes; no new external source"
status: provisional
tags: [knowledge-architecture, comparative-analysis, provenance, trust, retrieval, zbrain]
---

# Knowledge Architecture Comparative Synthesis

## Phạm vi và authority

Đây là synthesis provisional từ đúng năm note đã freeze:

1. [OKF v0.2](okf-v0.2-analysis.md) — technical specification.
2. [Karpathy LLM Wiki](karpathy-llm-wiki-analysis.md) — pattern plus product workflow/implementation.
3. [Entity Vault](entity-vault-analysis.md) — product workflow and starter-pack implementation.
4. [Uteke](uteke-analysis.md) — open-source local memory implementation.
5. [AtomicStrata llmwiki](atomicstrata-llm-wiki-compiler-analysis.md) — open-source compiler, MCP, lifecycle and viewer implementation pinned at commit `3e17bcfe8b50f24c14c6bcda0cb9224d94fd8206`.

Source facts, implementation observations and independent critique remain distinct in the local notes. This document compares what those notes support; it is not an approved zbrain architecture decision.

## Comparison matrix

| Rubric | Convergence | Meaningful disagreement or limit | Implication to evaluate for zbrain |
|---|---|---|---|
| Knowledge structure | Markdown/files remain the inspectable canonical layer. LLM Wiki/llmwiki use source→compiled pages; Entity Vault uses entity dossiers; OKF uses bundles; Uteke uses typed rows. | File identity, row identity and entity identity have different portability and migration costs. | Keep canonical Markdown and stable claim IDs; derived SQLite remains disposable. |
| Lifecycle | Durable knowledge needs a path from raw/input to a more useful current representation. Review queues and compiled-vs-timeline separation make correction visible. | OKF is mostly a format contract; Uteke is write-first memory; llmwiki supports direct live compile and review-held compile. | Draft is not trusted context; every trust transition needs explicit policy. |
| Provenance/trust | Source IDs, hashes, citations, timelines and review metadata are repeatedly useful. | Self-declared `verified` fields, free-text confidence and environment trust grants are not owner proof. | Bind approval to exact canonical draft, evidence snapshot and action digest. |
| Retrieval | Lexical retrieval is a viable baseline; hybrid semantic retrieval can improve recall; context packaging should be separate from answer generation. | Uteke's separate vector file creates repair/desync cost; llmwiki has richer graph/context expansion than zbrain needs initially. | Optional embeddings must stay disposable, same-index, degradable and observable. |
| Validation/security | Path confinement, lock/revalidate/apply, source fencing, stale detection and fail-closed gates are valuable. | URL/local-file fetch, internal model providers, static secrets and broad mutation tools expand attack surface. | stdio-only, content-in tools, loopback viewer, no API keys in core. |
| Human correction | Human-readable Markdown, diffs, reject/archive and current-vs-history zones support correction. | Correction authority varies from convention to environment flag; none in corpus provides one-time local owner ceremony for all sensitive transitions. | Owner pin approve/supersede/revoke; no auto-approve or trusted-session bypass. |
| Portability | Local-first storage and export are common goals. | Node/Rust/provider/vector dependencies increase operational burden; path-as-identity and separate indexes complicate migration. | Go single binary, Linux/macOS first, canonical format independent of index/provider. |
| Operational cost | Health/status/lint/recovery actions make automation usable. | Uteke's concurrency repairs and llmwiki's compiler/profile breadth represent different complexity centers. | Shared health model, exact recovery action, no hidden background daemon. |
| Scale | FTS and bounded context packs are practical local primitives; embeddings help semantic recall. | Corpus benchmarks and multi-user guarantees are sparse or source-specific. | Preserve current lexical gate; add deterministic fake-embedder benchmark before provider-dependent claims. |

## Convergent patterns worth retaining

### 1. Separate canonical content from derived state

OKF, Karpathy/llmwiki and Entity Vault make files readable and diffable; Uteke demonstrates the cost when retrieval state is a separate mutable topology. The strongest common pattern is canonical source plus rebuildable derived state, not “database as truth.”

### 2. Make provenance a retrieval feature

Citations, line ranges, source windows, freshness badges, evidence timelines and context-pack metadata all make a result inspectable. Provenance is not only an audit log; it changes whether an agent can safely use a result.

### 3. Treat correction as a normal lifecycle

Review queues, append-only timelines, stale repair, rejection archives and human-editable Markdown all assume generated output will need correction. Systems that hide correction behind prompts or trust scores are weaker.

### 4. Degrade explicitly

Lexical fallback, unknown freshness, pending review and structured warnings are safer than fabricated healthy/semantic states. A gateway should return what it knows and an exact recovery action.

## Contradictions and authority caveats

- **Specification vs implementation:** OKF's format and attestation ideas are design authority for its own contract, not proof that identity, verification or attestation are cryptographically enforced. The implementation notes are stronger evidence for observed runtime behavior but remain source-level unless executed.
- **Compiler trust vs gateway trust:** llmwiki's default live compile and provider integrations optimize throughput and knowledge accumulation. zbrain's target explicitly rejects internal completion and treats every generated claim as a promotion candidate.
- **Entity memory vs source-grounded memory:** Entity Vault's compiled truth/timeline split is useful for mutable dossiers, but its free-text confidence and convention-based append-only timeline do not replace evidence digest validation.
- **Server/daemon shape:** Uteke's cross-process hardening points toward a daemon or single-writer design, while zbrain's current CLI/process-per-command contract already has workspace locks and SQLite derived indexes. No corpus evidence justifies adding a daemon to this gateway milestone.
- **Graph richness vs boundary simplicity:** llmwiki and Uteke model graph edges, but graph expansion is not required for a trustworthy first retrieval contract. Adding graph semantics before lifecycle proof is a scope risk.

## Security and trust gaps exposed by the corpus

1. Verification metadata can be self-asserted or stale unless it binds a content digest and generation ordering.
2. Raw transcripts, web pages and connector content are persistent prompt-injection surfaces.
3. URL fetch, arbitrary file reads and provider credentials in an agent gateway create SSRF, local-read and secret-exfiltration paths.
4. Separate vector files can desynchronize from canonical content and status.
5. Broad profiles and workflow engines increase the number of write paths that must enforce the same gate.

## Open questions

- Is local owner presence sufficient for the intended threat model, or is signed identity needed later?
- Should evidence spans support multiple disjoint ranges or only one contiguous range per entry?
- What exact viewer fields are safe to expose when raw evidence can contain secrets or instructions?
- Which local OpenAI-compatible embedder behavior is stable enough for deterministic operator diagnostics?
- Does a future daemon materially improve concurrent mutation safety enough to justify lifecycle cost?

## Synthesis verdict

The corpus supports a narrow conclusion: zbrain should become a **trust gateway around an existing canonical Markdown/evidence runtime**, with typed agent access, owner-pinned promotion, cryptographic span binding, explicit health and read-only viewing. It does not support importing a complete compiler, profile framework, graph database, remote server or internal LLM provider into the core.
