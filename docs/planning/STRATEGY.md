# zbrain — Strategic Evaluation as a Category-Defining Product

> This is not an implementation review. It evaluates the **idea**, and challenges the core thesis.
> Stated thesis: *"A personal LLM wiki that AI agents can use safely."*
> My counter-thesis (argued throughout): **the "wiki" and "personal" framing undersell and mis-position what this actually is — a provenance-gated, locally-owned trust layer for agent memory. The wiki is an implementation detail.**

---

## 1. What category is this project actually creating?

Not note-taking. Not "agent memory" in the Mem0 sense. The category zbrain is reaching for is:

> **Audit-grade memory for agents — a human-owned commit log of what an agent is allowed to believe, and where each belief came from.**

Every other memory product optimizes for **recall** (remember everything, surface it invisibly). zbrain optimizes for **trust** (remember only what traces to a source a human approved). That's a different axis entirely. The closest mental model is **"git for agent memory"**: versioned, attributable, owned, reviewable — provenance as a first-class citizen, not a metadata afterthought.

The category isn't "a better wiki." It's **"the trust/provenance layer that sits between agents and their long-term knowledge."** If that category becomes real, the winner is whoever owns the *provenance format and the human-approval workflow*, not whoever has the best retrieval.

---

## 2. What is the strongest unique insight?

**Agent memory's real failure mode is not forgetting — it's confidently remembering something wrong.** Hallucinated, poisoned, or stale memory is *worse* than no memory, because the agent acts on it with authority and you can't tell where it came from.

zbrain's insight: **curated beats captured.** If memory is human-verified and source-cited, then:
- you don't need to remember everything (so the corpus stays small),
- which means you don't need semantic recall over a million fuzzy fragments (so no embeddings),
- which means it can be local, lexical, and human-readable (so you own it).

The no-embeddings choice is not a limitation dressed as a virtue — it's *downstream of the trust insight*. Small + curated + cited makes lexical sufficient. That's a genuinely coherent chain of reasoning, and almost nobody else in the space is making it. **Workspace isolation as a poisoning boundary** is the second-strongest insight — most memory products have no security model at all.

---

## 3. What is the biggest strategic mistake?

**The unit of adoption is backwards.** Category-defining infra products start as an *invisible layer inside a tool people already use*, then earn the right to be standalone. zbrain inverts this: it asks the user to adopt a new CLI, install `qmd`, learn a 7-command `setup→init→learn→ingest review→apply→ask` ritual, and act as a manual librarian — **before any value is delivered.**

The fatal coupling: **it makes the human do the ingestion labor that should be the agent's job.** The human should be the *editor/approver*, not the *data-entry clerk*. Right now the agent answers questions and the human files the memories. That's exactly inverted from where the leverage is.

Second mistake, downstream of the first: **positioning as a "personal wiki."** That word anchors it against Obsidian — a mature, beloved, free incumbent — and invites "why not just Obsidian + an MCP plugin?" The real value (governed agent memory) is buried under a competitor frame it can't win.

The thesis-level danger: **the human-review gate is simultaneously the moat and the thing that kills adoption.** Provenance requires approval; approval requires effort; effort doesn't scale with the volume agents generate. If you don't drive the cost of approval to near-zero, the very mechanism that creates trust prevents use.

---

## 4. What assumptions are likely wrong?

| Assumption | Why it's shaky |
|---|---|
| **Humans will curate memory** | Curation fatigue is real and fast. At any volume the review queue becomes a graveyard. The patient-librarian user is rare. |
| **No embeddings, ever** | Correct as *sequencing* (start lexical), wrong as *ideology*. BM25 dies on vocabulary mismatch — "what did I decide about auth?" won't match a note titled "OAuth token rotation policy." Conceptual recall needs more than lexical eventually. |
| **One workspace per query (hard isolation)** | Real thinking is cross-domain. Strict isolation fights how knowledge actually connects; users will hit walls between their own workspaces. |
| **Agents will obey "ask before answering"** | That rule is a *prompt*, not an enforced gate. Compliance is probabilistic and best-effort. The safety guarantee is softer than it reads. |
| **The wiki is the product** | The *provenance graph* (source → approval → belief → usage) is probably the product. The wiki is just where it's stored. |
| **Local-first is a decisive draw** | For the AI-power-user niche, yes. For anyone else, indifferent — and "but I want it on my phone / shared with my team" arrives fast. |

---

## 5. What becomes painful at 10x scale?

*(~thousands of notes, a few agents, daily use)*

- **The review gate is the bottleneck.** A backlog of learned-but-not-applied evidence accumulates; the agent's memory lags reality by however far behind the human is.
- **Lexical misses start to hurt.** Vocabulary mismatch means users *know* a fact is in there but can't retrieve it — the most corrosive failure for a memory tool (erodes trust in the whole system).
- **Manual conflict & duplication management** becomes real work (V2's supersede helps, but someone still decides).
- **Multi-agent write contention** (V1 has none; V2's leases/optimistic locking get exercised here).

## 6. What becomes painful at 100x scale?

*(~tens of thousands of notes, many agents, team-shared)*

- **Lexical retrieval hits its ceiling.** This is where "no embeddings" stops being a clean simplification and becomes a real lid on recall quality. Some local-embedding recall booster becomes near-mandatory.
- **Local-first vs. multi-machine multi-agent** collide. Concurrent writes across devices want sync/CRDT/a server — which fractures the local-first purity. You'll be forced to choose, and the choice is existential.
- **The human gate is fully untenable.** You need *automated trust scoring* (source reputation, corroboration, recency) — which is a meaningfully different product than "human approves each fact."
- **Workspace isolation fragments the corpus** you most want connected; demand for a cross-workspace knowledge graph becomes loud.
- **Provenance becomes a compliance asset.** "Who approved this belief, when, from what source" is exactly what regulated/team environments will pay for — the scaling pain at 100x is also the **monetization opportunity**.

---

## 7. What should never be built?

- **A hosted multi-tenant SaaS.** It kills the local-first thesis and turns zbrain into a worse Mem0.
- **A mandatory vector DB / always-on embeddings.** Violates the core insight and bolts on ops weight. (A *local, optional* embedding booster is fine — see §10/migration.)
- **A full Obsidian-class GUI editor.** Unwinnable war. Integrate with editors; don't become one.
- **Auto-capture-everything memory.** The moment you record everything silently, you're Rewind/Mem0 and you've abandoned the curation insight that makes you distinct.
- **A custom query DSL.** Solo-dev-killing complexity for marginal gain.
- **Enterprise RBAC / real-time collab at this stage.** Premature; build the provenance substrate first — RBAC is a layer *on top* later.

---

## 8. What should be built immediately?

1. **An MCP server, not (only) a CLI.** The form factor must be an *invisible layer inside Claude Code / Cursor*: `remember(fact, source)` and `recall(query)` with provenance, callable by the agent. This is the single highest-leverage move — it fixes the backwards adoption unit (§3).
2. **Agent-drafted, human-one-click review.** The agent proposes the fact, tier, target path, and citation; the human approves/edits with a keystroke. Drive approval cost toward zero — this is what keeps the trust gate viable (§3, §4).
3. **Provenance as the headline.** The demo is: *"Claude remembered my architecture decision across sessions — and showed me the exact source I approved it from."* Lead with trust + ownership, not "a wiki."
4. **The V2 foundation** (files-as-truth, FTS5, lifecycle, tests). Already specced in `V2-ARCHITECTURE.md`.
5. **A 60-second aha demo** for the AI-power-user niche. Adoption dies in the 7-command ritual; the demo must show value before setup cost is felt.

---

## 9. If this project succeeds, what does V5 look like in 3 years?

**zbrain becomes the provenance/trust layer that sits *under* the agent tools, not beside them.** Concretely:

- **A standardized memory-provenance format** — "the git commit log of agent beliefs" — that other tools (Claude Code, Cursor, custom agents) read and write. The win condition is owning the *format and approval protocol*, not the app.
- **Local-first core, optional encrypted sync** for teams — never a default cloud. Ownership stays the brand.
- **Lexical + optional local embeddings** as a recall *booster* — but provenance, not retrieval, remains the moat. Trust is the product; recall is a feature.
- **Automated trust scoring** layered over (not replacing) human approval — corroboration, source reputation, staleness — so the gate scales.
- **The "SQLite of agent memory":** boring, embeddable, everywhere, the obvious default. Possibly a *spec others implement* rather than only software you run.

The honest alternative ending: it stays a beloved tool for a few thousand local-first power users and never crosses the chasm — which happens if it never escapes the CLI/wiki framing and the gate stays high-friction.

---

## 10. Architecture vs. the field

Axes that matter: **provenance** (can you trace a belief to a source?), **curation vs capture**, **local/owned**, **human-readable**, **agent-native form factor**, **retrieval ceiling**.

| Product | What it is | Where zbrain wins | Where it loses / threat |
|---|---|---|---|
| **Obsidian** | Local markdown PKM, human-first, plugin ecosystem | Agent-native + provenance gate + lifecycle; memory designed for *agents to consume safely* | Obsidian is mature, beloved, free; **Obsidian + an MCP plugin can eat the "wiki" angle**. Don't fight it on PKM — differentiate on agent trust. |
| **Mem0** | Agent memory API: auto-extracted, embeddings/vector, cloud-leaning | Trust, ownership, human-readability, no cloud dependency, security isolation | Mem0 is **frictionless** — agents just remember. zbrain's gate is heavier. Mem0 wins convenience; zbrain wins accountability. The market may reward convenience. |
| **OpenMemory (MCP)** | Local-ish memory MCP server, auto-capture, vector-based | Provenance + human gate + workspace isolation (it has none of these) | It already has **the right form factor (MCP)** that zbrain lacks. This is the clearest "adopt their shape, keep my substance" lesson. |
| **Claude Projects** | Curated per-project context, dead simple, cloud, Anthropic-owned | Local ownership, lifecycle, multi-agent, cross-tool, provenance | "**Good enough for most**" incumbent. Zero setup. If Anthropic adds persistence + citations, it covers 80% of casual demand. zbrain must be where Projects *can't* go: local, multi-tool, audited. |
| **NotebookLM** | Source-grounded Q&A that cites the documents you give it | Persistent evolving memory, agent write-back, local, multi-tool | **Closest philosophical cousin** — grounded + cited is the *same instinct*. But it's cloud, Google, doc-centric, read-only memory. It validates "grounded + cited" works at scale; zbrain extends it to *persistent, agent-mutated* memory. |
| **MCP Memory Servers** (generic) | Key-value / simple graph memory over MCP, no provenance, no gate, no security | Substance: provenance, human gate, lifecycle, isolation, human-readable store | They have form (MCP) without substance. **zbrain's substance + their form factor is the actual play.** |

**Synthesis of the comparison:** NotebookLM proves the *grounded-and-cited* thesis. MCP servers + OpenMemory prove the *form factor*. Mem0 proves the *demand for agent memory*. **Nobody combines provenance + human ownership + local-first + agent-native form factor.** That intersection is empty — and it's exactly where zbrain's idea lives. The idea is sound; the *packaging* (CLI + wiki + manual labor) is what's standing between it and the category.

---

## Verdict on the core thesis

The thesis is **directionally right but mis-stated.** "Personal LLM wiki" is a Trojan horse hiding the real product: **provenance-gated, locally-owned agent memory — the trust layer agents don't have.**

Three things have to be true for it to define the category:
1. **The agent becomes the librarian, the human becomes the approver** — and approval costs ~one keystroke. (Today it's backwards and expensive.)
2. **It ships as an invisible layer (MCP), not a CLI ritual.** Form factor is destiny for infra.
3. **Provenance stays the moat; retrieval is allowed to evolve** (lexical now, optional local embeddings later) without becoming the identity.

Get those right and zbrain is creating a category nobody else occupies. Get them wrong and it's a thoughtful local wiki that a Claude Projects feature or an Obsidian plugin quietly absorbs. The idea earns the swing — the current packaging does not yet.
