# zbrain — Deep Technical Audit

> **Status: historical.** This audit covers the pre-reset Bun/TypeScript codebase
> (`src/core/*.ts`), which was removed in the Go-native rewrite. Keep for reference;
> it does not describe the current implementation.

> Scope: full repository at `master` (commit `fa873e1`). MVP-1.
> Vision audited against: *"A personal LLM wiki that AI agents can use safely."*
> Optimization axes: Simplicity · Correctness · Agent usability · Maintainability · OSS friendliness · Local-first.

---

# Executive Summary

## Overall assessment

zbrain is a **well-structured, cleanly-layered MVP** with an unusually disciplined separation between pure core logic and I/O. The code is readable, the dependency surface is tiny (4 runtime deps), and the local-first posture is real (markdown + SQLite under `~/.zbrain`). The author clearly thought about isolation, immutability, and atomicity — there are good comments explaining *why* (e.g. [evidence-ingest.ts:64](src/core/evidence-ingest.ts:65), [current-task.ts:115](src/core/current-task.ts:115)).

But as an **AI memory platform**, the system has three load-bearing gaps that undercut its own stated guarantees:

1. **The review→apply gate is bypassable by retrieval.** `qmd` indexes the *entire* workspace directory, including un-reviewed `evidence/sources/*/raw.md`. So raw, ungated source material is retrievable by `zbrain ask` alongside trusted wiki facts. The pipeline's central safety promise — "nothing reaches the agent until a human verifies it" — does not actually hold. ([qmd-adapter.ts:121](src/core/qmd-adapter.ts:121))
2. **There is no forgetting.** Memory only grows. `archived` exists in the type system but no command implements it, there is no update/supersede/delete path, and apply silently overwrites with last-writer-wins. ([evidence-list.ts:13](src/core/evidence-list.ts:13), [evidence-apply.ts:123](src/core/evidence-apply.ts:123))
3. **There are zero automated tests**, despite "Correctness" being an explicit goal. The test suite was deleted (commits `c057fe0`, `fa873e1`) and `make test` now references a script that no longer exists.

## Biggest strengths

- Clean **core/CLI/asset** layering; pure functions take `RuntimePaths` so everything is redirectable for tests. ([CLAUDE.md](CLAUDE.md), [runtime-paths.ts](src/core/runtime-paths.ts))
- **Local-first done right**: human-readable wiki markdown, single SQLite file, `ZBRAIN_HOME` override, atomic context-file writes.
- **Real isolation primitives**: `workspaceCollectionName` rejects traversal ([qmd-adapter.ts:73](src/core/qmd-adapter.ts:73)); `assertWorkspaceTarget`/`pathInside` sandbox apply writes ([evidence-store.ts:189](src/core/evidence-store.ts:189), [fs.ts:27](src/core/fs.ts:27)).
- **Tier-first retrieval** is a genuinely good simple-first idea — structure-as-priority instead of embeddings.

## Biggest weaknesses

- Retrieval index scope defeats the evidence gate (poisoning vector).
- No memory lifecycle past `applied` — no forgetting, invalidation, conflict handling, or update.
- No tests; "Correctness" goal is unverified.
- **Dual source of truth**: SQLite is authoritative for the CLI, but every agent rule tells the agent to read `~/.zbrain/projects.json`, which is a hand-mirrored copy that drifts. ([init via helpers.ts:217](src/commands/helpers.ts:217))
- **Periphery over-engineered, core under-engineered**: multi-workspace slot-allocation math ([retrieval.ts:84](src/core/retrieval.ts:84)) and `sessions`/`queries` tables ([db.ts:58](src/core/db.ts:58)) exist but are out-of-scope/unused, while forgetting and index-scoping are absent.

## Recommended priorities (do these first)

1. **Scope the qmd index to the four wiki tiers** (exclude `evidence/`). This is the single highest-leverage correctness+security fix.
2. **Re-add a test harness** and pin the invariants (state machine, gate, isolation, dedup) before adding features.
3. **Collapse to one source of truth** for project resolution (read SQLite everywhere, or generate JSON read-only with a clear "do not edit").
4. **Design the forgetting path** (archive/supersede/delete) even if minimal.

---

# Architecture Audit

## Strengths

- **Three-layer model is correct and documented**: binary → runtime (`~/.zbrain`) → project (`.claude/`). Asset source-of-truth in `assets/`, bundled via codegen into [bundled-assets.ts](src/generated/bundled-assets.ts). This is a clean way to ship a self-extracting CLI.
- **I/O discipline**: core functions are pure or take `paths: RuntimePaths`. `QmdAdapter` and `QmdRunner` are injectable ([qmd-adapter.ts:18](src/core/qmd-adapter.ts:18)), `RetrievalAdapter` is injectable ([retrieval.ts:17](src/core/retrieval.ts:17)). This is exactly the seam you want for testing — which makes the *absence* of tests more frustrating, not less.
- **Cohesive modules**: evidence store / state / ingest / review / apply / list are small and single-purpose.

## Weaknesses

- **Two stores, one truth, no sync guarantee.** `db-projects.ts` is authoritative; `initProject` *also* writes `projects.json` because skills read it ([helpers.ts:217-224](src/commands/helpers.ts:217)). Nothing keeps them in sync after init (e.g. a later `workspace create` that sets a default, or a manual edit). The CLI (`ask`) resolves from DB ([ask.ts:35](src/commands/ask.ts:35)); the agent resolves from JSON ([claude-rules.md:8](assets/engine/claude-rules.md)). They *will* diverge.
- **`source.yaml` is dead on the write path.** `evidenceLocations` defines `sourceFile` ([evidence-store.ts:78](src/core/evidence-store.ts:78)) and `assertImmutableSourceSnapshot` checks it ([evidence-state.ts:75](src/core/evidence-state.ts:75)), but `ingestEvidence` never writes it — it only writes `raw.md` and a DB row ([evidence-ingest.ts:66-71](src/core/evidence-ingest.ts:66)). So evidence metadata now lives *only* in SQLite. This is dead code **and** a regression against the "human-readable data" goal.
- **Speculative schema.** `sessions` and `queries` tables are created but never inserted into anywhere in `src/` (verified: zero `INSERT INTO sessions/queries`). Premature; remove until needed. ([db.ts:58-74](src/core/db.ts:58))
- **Out-of-scope feature shipped.** README lists "Cross-workspace retrieval" as *out of scope for MVP-1*, yet `retrieveMultiWorkspaceContext` with keyword/`@tag` routing and slot math is fully built ([retrieval.ts:52](src/core/retrieval.ts:52)). It's clever but it's complexity the MVP said it didn't want.

## Risks

- **Schema evolution has no migration framework** — only `CREATE TABLE IF NOT EXISTS` + a `user_version` ceiling check ([db.ts:21-82](src/core/db.ts:21)). Any v2 column change needs hand-written ALTERs that don't exist yet. Fine today, a cliff later.
- **`appendIntegrationRules` does regex surgery on the user's `CLAUDE.md`** ([helpers.ts:139](src/commands/helpers.ts:139)) including `zwiki→zbrain` legacy rewriting. Editing a user's hand-maintained file with regex is fragile and a future support burden.

## Recommendations

- Pick **one** project store. Simplest: keep SQLite authoritative, and have the agent read project context via a tiny `zbrain resolve --json` command instead of a mirrored file. Removes drift entirely.
- Delete `sessions`/`queries` tables and `source.yaml`/`assertImmutableSourceSnapshot` dead paths, **or** wire them in. Don't leave half-built.
- Gate `retrieveMultiWorkspaceContext` behind an explicit flag and mark it experimental, or pull it until post-MVP.

---

# AI Memory System Audit

## Current Design Assessment

The memory model is **evidence-sourced wiki**: external material (`learn`) → human-verified facts (`ingest review`) → written into tiered wiki files (`ingest apply`) → retrieved lexically (`ask`). Conceptually this is a strong, auditable design: every fact should trace to an immutable source with a citation. The tier system (axioms/mental-models/projects/decisions) is a clean importance prior without ML.

The problem is the gap between the *designed* pipeline and the *implemented* one.

## Memory Lifecycle Assessment

| Phase | Status | Evidence |
|---|---|---|
| Creation | ✅ Implemented | `learn` → `ingestEvidence` ([evidence-ingest.ts](src/core/evidence-ingest.ts)) |
| Review/verify | ⚠️ Thin | CLI auto-fills `q-1`/`P0`/`answered`, one fact ([ingest.ts:81](src/commands/ingest.ts:81)); the rich review in the skill is not enforced by code |
| Apply | ✅ Implemented, with checkpointing | [evidence-apply.ts](src/core/evidence-apply.ts) |
| **Update / supersede** | ❌ Missing | Apply overwrites with `overwrite:true` ([evidence-apply.ts:123](src/core/evidence-apply.ts:123)); no merge, no diff, no version |
| **Invalidation / staleness** | ❌ Missing | No `valid_until`, no review date, no "this fact was superseded by X" |
| **Archival** | ❌ Not implemented | `archived` is a legal transition ([evidence-state.ts:35](src/core/evidence-state.ts:35)) but `nextCommandByState.applied/archived → null` ([evidence-list.ts:16](src/core/evidence-list.ts:16)) and **no `ingest archive` command exists** |
| **Forgetting / deletion** | ❌ Missing | No delete path for wiki docs or evidence |

**This is the platform's most serious design gap.** A memory layer that cannot forget, update, or invalidate will accumulate stale and contradictory facts indefinitely. For a "long-term memory for AI agents," forgetting is not optional — it's the half of the system that keeps the other half trustworthy.

## Memory Quality Assessment

- **Duplication**: handled *only* for byte-identical content, per workspace, via `raw_sha256` ([evidence-ingest.ts:38](src/core/evidence-ingest.ts:38)). Near-duplicates (same fact, reworded; re-fetched article with a changed timestamp) create new evidence and can apply conflicting wiki edits. No semantic or fuzzy dedup (acceptable for MVP, but note it).
- **Conflicts**: undetected. Two evidence items applying to the same wiki path → silent overwrite, last writer wins. No "this contradicts existing fact" surface.
- **Stale memory**: no TTL, no last-verified timestamp on facts, no re-review prompt.
- **Poisoning**: see Retrieval + Security. Raw, unverified, possibly web-fetched content is retrievable. This is the live risk.
- **Drift**: because apply just appends/overwrites text into markdown, the wiki can diverge from its evidence over successive edits with no integrity check on the *wiki* (only on `raw.md`).

## Retrieval Assessment

- **BM25 via qmd** is a reasonable simple-first choice and matches the "no embeddings" constraint.
- **Tier-first ranking** ([retrieval-ranking.ts:34](src/core/retrieval-ranking.ts:34)) is good, but "higher tier always wins regardless of score" means a weakly-relevant axiom outranks a strongly-relevant decision. For some queries this *hurts* relevance. Consider a tier *weight* (score boost) rather than a hard lexicographic sort.
- **`classifyByTier` is naive substring matching** on `/axioms/` etc.; anything outside the four dirs defaults to **P2** ([retrieval-ranking.ts:31](src/core/retrieval-ranking.ts:31)). So `evidence/sources/*/raw.md` is retrieved and ranked **P2 — above decisions**.
- **No lazy indexing.** `ask` never reindexes; the index is only rebuilt at `apply` time ([ingest.ts:144](src/commands/ingest.ts:144)). A freshly created workspace, or one with only learned-but-not-applied evidence, has no index → `qmd search` exits non-zero → `searchWorkspace` throws ([qmd-adapter.ts:110](src/core/qmd-adapter.ts:110)). First-run UX is a hard error, not an empty result.

## Multi-Agent Assessment

The vision explicitly wants a **shared memory layer for multiple agents**. Current concurrency posture:

- **DB**: SQLite WAL ([db.ts:11](src/core/db.ts:11)) — safe for concurrent readers + serialized writers. Good.
- **Context file**: `current-task.md` is written atomically via temp+rename ([current-task.ts:115](src/core/current-task.ts:115)) — good against torn writes, **but it's one shared file per project** keyed by project root ([current-task.ts:123](src/core/current-task.ts:123)). Two agents asking different questions in the same project **clobber each other's context** — agent A may read agent B's retrieval. There is no per-session/per-agent context file.
- **Wiki writes**: apply writes wiki markdown with **no file lock**. Two concurrent applies to the same path race; checkpoint files are per-evidence so they don't protect a shared target.
- **FS+DB atomicity gap (known)**: the row is inserted in a txn, then `raw.md` is written *outside* it ([evidence-ingest.ts:66-71](src/core/evidence-ingest.ts:66), comment cites ISSUE-007). A crash/permission error between commit and write leaves a DB row whose `raw.md` is absent → every later `review`/`apply` throws on `readTextFile`. Orphan with no recovery command.

**Verdict**: safe-ish for a single agent; **not yet a safe shared layer**. The shared-context-file clobber is the concrete multi-agent bug to fix first.

## Future Evolution Assessment

Readiness for later capabilities, *without* building them now:

- **Semantic search / embeddings**: ✅ The `RetrievalAdapter` seam ([retrieval.ts:17](src/core/retrieval.ts:17)) makes swapping/augmenting the search backend clean. A future `HybridAdapter` is plausible without touching callers.
- **Vector index**: ✅ Could live beside `zbrain.db`; schema has room.
- **Knowledge graph**: ⚠️ Facts are free-text appended to markdown with no stable IDs or typed links, so there's no structure to lift into a graph later. If a graph is on the roadmap, give facts stable IDs *now* (cheap) — retrofitting is expensive.
- **Memory scoring / importance ranking**: ⚠️ Tier is the only signal. No access counts, recency, or feedback (the unused `queries` table *hints* at this intent — wire it if you want usage-based scoring later).
- **Forgetting**: ❌ must be designed before scale; see lifecycle.

---

# Local-First Audit

## Strengths

- Everything lives under `~/.zbrain`, overridable via `ZBRAIN_HOME` ([runtime-paths.ts:25](src/core/runtime-paths.ts:25)) — clean isolation for tests/portability.
- Wiki content is plain markdown — readable, greppable, git-friendly, survives the tool.
- Single SQLite file; no daemon, no network dependency for core ops.
- Atomic context writes; WAL journaling.

## Risks

- **"Human-readable data" partially broken.** Evidence metadata (state, hashes, origin, timestamps) now lives only in `zbrain.db`, not `source.yaml` ([evidence-ingest.ts](src/core/evidence-ingest.ts) writes no YAML). If the DB is lost or corrupted, the markdown alone can't reconstruct the pipeline state. A backup is now "markdown **and** an opaque binary," not "just my notes."
- **No backup/restore/export command.** Local-first products live or die on "can I move this to a new machine." Today: manually copy `~/.zbrain`. No `zbrain export`, no integrity self-check, no `doctor`.
- **Recovery story is absent.** The FS/DB orphan case (above) has no repair tool.

## Future Bottlenecks

- **Is SQLite the right choice?** Yes — for this scale and goals it's ideal. Don't second-guess it. The bottleneck won't be SQLite; it'll be **full-workspace qmd reindex on every apply** ([ingest.ts:144](src/commands/ingest.ts:144)), which is O(workspace) per write and will get slow as a workspace grows to thousands of docs. Incremental indexing is the future need, not a DB swap.
- **Migration difficulty later**: low for content (markdown), medium for metadata (need a real migration framework before schema v2).

---

# Product Audit

Target users: AI power users, Claude Code / OpenAI Agent users, local-first enthusiasts.

## What Users Will Love

- **"My agent stops hallucinating my own decisions."** The `ask`-before-answer rule + citations is a genuinely compelling pitch for Claude Code users.
- **Local, private, plain-text.** No cloud, no lock-in, inspectable. This crowd values that highly.
- **Tiny, fast, single binary.** Bun-compiled, 4 deps.
- **Workspace isolation** maps naturally to "work / finance / health / side-project" mental buckets.

## Adoption Risks

- **Heavy prerequisite + multi-step onboarding.** Requires global `qmd` install ([README](README.md)), then `setup` → `workspace create` → `init` → `learn` → `ingest review` → `ingest apply` → `ask` before any value. That's ~7 commands and a human review loop before the first useful `ask`. High activation cost.
- **First `ask` can hard-error** on an unindexed workspace (see Retrieval). Worst possible first impression.
- **The `ingest review`/`apply` ceremony is heavy** for a "personal wiki." Power users will want a one-shot "learn this and file it for me" path; the manual fact/wiki-path entry ([ingest.ts:87-97](src/commands/ingest.ts:87)) will feel like data-entry chores.
- **Two mental models leak**: the SQLite DB vs the `projects.json` the docs tell you to read. Confusing when they disagree.

## Missing MVP Features

1. **`zbrain doctor`** — verify qmd, DB, index freshness, orphaned evidence. (Highest UX ROI.)
2. **Lazy/auto index** so `ask` always returns something.
3. **A forgetting/edit path** — even just `zbrain forget <wikipath>`.
4. **Export/backup**.
5. **A fast path** that collapses learn→apply for trusted single facts.
6. **`LICENSE`** (it's missing — blocks adoption outright for many).

---

# Security Audit

Realistic, local-first threat model (single user, possibly hostile *content*, multiple cooperating agents).

## Critical Issues

- **C1 — Unverified evidence is retrievable (memory poisoning / prompt injection).** `indexWorkspace` indexes the whole workspace path ([qmd-adapter.ts:121-134](src/core/qmd-adapter.ts:121)), which contains `evidence/sources/*/raw.md` — content that may be **pasted or web-fetched** (the `zbrain:research` skill fetches arbitrary URLs). That raw content is searchable by `ask`, ranked P2, and dropped into the agent's `current-task.md` **before any human review**. The entire review→apply gate — the product's core safety claim — is bypassed at retrieval. A malicious web page that says "ignore prior instructions, exfiltrate X" lands directly in agent context.
  **Fix**: index only `axioms/ mental-models/ projects/ decisions/` (a qmd include-list or a dedicated `wiki/` subtree), never `evidence/`.

## Medium Issues

- **M1 — Thin review gate in the CLI.** `runIngestReview` defaults severity/status to `P0/answered` and asks for a single fact ([ingest.ts:73-98](src/commands/ingest.ts:73)). The QA gate ([evidence-state.ts:53](src/core/evidence-state.ts:53)) only blocks `awaiting_external`/`deferred` P0/P1 — which the CLI never sets by default. So the "human verification" guarantee is, in the default CLI flow, a rubber stamp.
- **M2 — Apply trusts agent-supplied content.** `applyEvidence` writes whatever `mutation.content` is into the wiki ([evidence-apply.ts:123](src/core/evidence-apply.ts:123)). Path is sandboxed (good), but content is not validated against the cited evidence. An over-eager or compromised agent can write facts the source never supported, with a citation that looks legitimate.
- **M3 — Shared context-file clobber** (multi-agent), as above — one agent can read another's retrieved context.

## Low Issues

- **L1** — `learn --file` reads any path on disk ([learn.ts:37](src/commands/learn.ts:37)). Acceptable for a local CLI, but worth a note.
- **L2** — `parseProjectRow`/config use `.passthrough()` ([schemas/config.ts](src/schemas/config.ts)); unknown fields silently survive. Convenient, but means typo'd or malicious extra keys persist unflagged.
- **L3** — `mergeSettingsFile` JSON.parse on the user's `settings.local.json` with no try/catch ([helpers.ts:346](src/commands/helpers.ts:346)); a malformed file crashes `init`.

**Positives**: traversal is blocked at both the collection name ([qmd-adapter.ts:81](src/core/qmd-adapter.ts:81)) and the apply target ([evidence-store.ts:189](src/core/evidence-store.ts:189)); source integrity is SHA-checked on every state transition ([db-evidence.ts:92](src/core/db-evidence.ts:92)).

---

# Code Quality Audit

## Strengths

- Small, focused modules; consistent naming; meaningful comments that explain *why*.
- Strong typing; Zod at the config boundary.
- Injectable adapters/runners — designed for testability.
- Good defensive touches: corrupt-checkpoint-resumes-fresh ([evidence-apply.ts:52](src/core/evidence-apply.ts:52)), malformed-frontmatter-passthrough ([evidence-apply.ts:64](src/core/evidence-apply.ts:64)), atomic context write.

## Technical Debt

- **Zero tests** in `src/` (verified); `Makefile:test` calls `bun test --run` against a deleted suite — a broken, misleading target. For a "Correctness"-first project this is the #1 debt.
- **Dead code**: `source.yaml` write path, `assertImmutableSourceSnapshot` ([evidence-state.ts:75](src/core/evidence-state.ts:75)), `sessions`/`queries` tables, `legacyProjectPointerFile` handling.
- **Two `listEvidenceIds`** with different signatures — one FS-based ([evidence-store.ts:99](src/core/evidence-store.ts:99)), one DB-based ([db-evidence.ts:62](src/core/db-evidence.ts:62)). The FS one is now vestigial (DB is the truth). Confusing.
- **`PRD.md` (the audit prompt) is committed** — noise in the repo root.

## Refactoring Opportunities

- Delete the FS-based evidence enumeration; standardize on DB.
- Extract the duplicated `listWorkspaceNames` (exists in both [workspace-resolver.ts:17](src/core/workspace-resolver.ts:17) and [helpers.ts:54](src/commands/helpers.ts:54)).
- Replace hard tier-sort with tier-weighted scoring to protect relevance.
- Consolidate runtime-dir legacy `.zwiki` fallback ([runtime-paths.ts:27](src/core/runtime-paths.ts:27)) — it's intricate logic for a migration most new users will never hit.

---

# OSS Readiness Audit

## Documentation

- **Good**: `README.md` (clear MVP scope + command surface), `CLAUDE.md` (excellent architecture overview), `wiki-spec.md`, `AGENTS.md`, `docs/`.
- **Missing**: `LICENSE` (critical — no license = legally unusable by others), `CONTRIBUTING.md`, and a "how to run/verify" that actually works (the test target is broken).

## Architecture Clarity

- Among the best parts of the repo. The three-layer model and asset-regeneration flow are documented well enough that a contributor can orient fast. The `assets/ → generate → bundled-assets.ts` step is the one non-obvious gotcha and it *is* documented.

## Contributor Experience

- **Blocked by no tests**: a contributor cannot safely change `retrieval-ranking` or the state machine and prove they didn't break an invariant. The injectable seams are there; the harness is gone. Re-adding tests is the single biggest contributor-experience unlock.
- `make` targets are friendly, but `make test` lies. Fix or remove.

---

# Top 20 Improvements

| Priority | Improvement | Impact | Effort | Reason |
|---|---|---|---|---|
| P0 | Scope qmd index to wiki tiers; exclude `evidence/` | Critical | Low | Closes the poisoning/gate-bypass hole (C1) — the core safety promise |
| P0 | Re-introduce a test suite; pin invariants (state machine, gate, isolation, dedup, ranking) | Critical | Med | "Correctness" goal is currently unverified; unblocks all future work |
| P0 | Add `LICENSE` | Critical | Trivial | No license = not adoptable as OSS |
| P0 | Fix/remove broken `make test` target | High | Trivial | Misleading; breaks first contributor run |
| P1 | Single source of truth for project resolution (DB vs projects.json) | High | Med | Eliminates drift between CLI and agent |
| P1 | Lazy/auto-index in `ask` so first run never hard-errors | High | Low | Fixes worst-case first impression |
| P1 | Design the forgetting path (archive/supersede/delete) | High | Med | A memory layer must forget; currently grows forever |
| P1 | Per-session context file (stop multi-agent clobber) | High | Low | Required for the "shared memory" vision (M3) |
| P1 | `zbrain doctor` (verify qmd, DB, index, orphans) | High | Med | Biggest single UX/reliability lift |
| P1 | Close FS+DB atomicity gap (ISSUE-007) + orphan repair | High | Med | Prevents unrecoverable evidence rows |
| P2 | Conflict detection on apply (warn on overwrite of existing fact) | Med | Med | Prevents silent contradictory memory |
| P2 | Incremental indexing instead of full reindex per apply | Med | Med | Future write-throughput bottleneck |
| P2 | Validate apply content against cited evidence / stronger review gate | Med | Med | Hardens M1/M2 |
| P2 | Remove dead code (`source.yaml` path, `sessions`/`queries`, FS `listEvidenceIds`) | Med | Low | Reduces confusion/maintenance |
| P2 | Tier-weighted scoring instead of hard tier sort | Med | Low | Protects relevance quality |
| P2 | `zbrain export`/backup + integrity check | Med | Med | Local-first portability promise |
| P3 | Stable fact IDs in wiki files | Med | Low | Cheap now; enables graph/scoring later |
| P3 | Gate or remove multi-workspace retrieval (out-of-MVP-scope) | Low | Low | Removes premature complexity |
| P3 | Schema migration framework before v2 | Med | Med | Avoids a future migration cliff |
| P3 | Remove `PRD.md` from repo; add `CONTRIBUTING.md` | Low | Trivial | Repo hygiene / OSS polish |

---

# What I Would Do Next

## Next 30 days (make the core honest)

- Fix **C1** (index scope) and add the **test harness** back; lock the invariants.
- Add `LICENSE`, fix `make test`.
- Lazy-index in `ask`; ship `zbrain doctor`.
- Collapse to one project source of truth.
- Delete dead code.

*Goal: every promise in the README is actually true and verified.*

## Next 90 days (make memory trustworthy)

- Implement the **forgetting/supersede/archive** lifecycle + conflict detection on apply.
- Per-session context files; close the FS/DB atomicity gap with an orphan-repair path.
- `export`/backup + integrity self-check.
- Strengthen the review gate so "verified" means something.

*Goal: a memory layer that can be trusted long-term, not just bootstrapped.*

## Next 12 months (earn the "foundational layer" claim)

- Stable fact IDs → optional knowledge-graph view.
- Usage-based scoring (wire the `queries` table) and recency/staleness signals.
- Pluggable hybrid retrieval (`RetrievalAdapter` already supports it) — add embeddings *as an option*, keeping lexical default.
- Incremental indexing; a real schema-migration framework.
- Harden the multi-agent shared-write story (locking/CRDT-ish merge for wiki files).

---

# Final Verdict

| Dimension | Score | Rationale |
|---|---|---|
| **Architecture** | **7/10** | Clean layering, great testability seams, good local-first design. Docked for dual source-of-truth, dead/speculative code, and out-of-scope complexity shipped while core gaps remain. |
| **AI Memory Design** | **4/10** | Strong evidence-sourced concept and tier model, but the gate is bypassed by retrieval (C1), there's no forgetting/update/conflict handling, and the default review is a rubber stamp. As a *memory platform* the load-bearing half is missing. |
| **Product** | **5/10** | Compelling pitch and the right instincts, undercut by heavy onboarding, a first-run hard-error, ceremony-heavy ingest, and no forgetting/backup. Good bones, rough activation. |
| **Maintainability** | **5/10** | Readable, well-commented, small surface — but **zero tests** for a correctness-first project, plus dead code and a lying build target. Easy to read, risky to change. |
| **OSS Potential** | **6/10** | Excellent architecture docs and a clear niche; blocked by missing `LICENSE`, no working tests for contributors, and repo-hygiene noise. Fixable in a week. |

**Bottom line:** zbrain is a *thoughtfully engineered shell around a memory system whose safety and lifecycle guarantees aren't fully implemented yet.* The architecture earns the right to grow into the vision — but today it does not deliver "memory AI agents can use **safely**," because unverified content is retrievable (C1) and memory can never be corrected or forgotten. Fix the index scope, add tests, and design forgetting; those three moves convert this from a promising MVP into a credible foundational layer.
