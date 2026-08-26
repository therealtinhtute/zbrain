# Planning source material

These documents are the planning source material that produced SPEC.md and
the v2.0.0 release. They are kept for traceability — every architectural
decision in the v2 release traces back to one of these files.

| Document | Purpose | Authored |
|---|---|---|
| [AUDIT.md](./AUDIT.md) | Pre-v2 audit of the v1 codebase. Identifies the dual-source-of-truth bug (A1), the C1 indexing-scope bug, and other issues that v2 closes. | 2026-06-20 |
| [STRATEGY.md](./STRATEGY.md) | Strategic positioning of zbrain v2: workspace-isolated knowledge retrieval, the MCP-server-as-distribution move (§8), and the human-review gate as a moat. | 2026-06-20 |
| [V2-ARCHITECTURE.md](./V2-ARCHITECTURE.md) | V2 architecture: files-as-truth storage, FTS5 retrieval, tier-weighted scoring, lifecycle state machine, multi-agent safety. | 2026-06-20 |
| [MCP-V2-BRAINSTORM.md](./MCP-V2-BRAINSTORM.md) | Brainstorm of ten MCP server ideas against the stateless `2026-07-28` spec revision, scoring, and a mini-PRD for the selected prototype (`crashlens`). | 2026-08-25 |

These are **not** user-facing docs. The authoritative spec is [`SPEC.md`](../../SPEC.md);
the implementation is the code under `src/`. Read these only when investigating
why a design decision was made.

If you change behavior in `src/` that contradicts one of these documents,
update the relevant doc (and the SPEC entry it maps to) in the same PR.
