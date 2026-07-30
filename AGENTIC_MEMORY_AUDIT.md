# Agentic Memory Audit – zbrain

## 1. Overview

- Project name: **zbrain** — Bun-compiled CLI for a workspace-isolated personal LLM wiki that AI agents read/write through skills, CLI verbs, and an MCP server.
- Repository / path reviewed: `/Users/tinhtute/Lab/zbrain` (branch `master`, HEAD `d66f921`), plus the **live runtime** at `~/.zbrain/` and installed binary `~/.local/bin/zbrain` (64 MB, built 2026-07-03).
- Date of audit: 2026-07-06
- Auditor: Claude Fable 5 (agentic memory architecture mode)
- Context: local & Git-backed shared agentic memory for a single developer–researcher.

All observations below are tied to files/lines actually read. Where something does not exist, it is stated as "no mechanism found". Hypotheses are marked **[assumption]**.

---

## 2. System Architecture

### 2.1 Memory layers

**Instruction layer** — lives in `assets/engine/` and is bundled → extracted → injected:

- `assets/engine/system-prompt.md` — runtime identity ("reads from the active workspace only", "stops when retrieval or QA evidence is insufficient").
- `assets/engine/constraints.md` — 5 invariants (workspace isolation, traceable sources, immutable `raw.md`/`source.yaml`, P0/P1 apply gate).
- `assets/engine/retrieval-rules.md`, `evidence-rules.md`, `claude-rules.md`, `codex-rules.md` — pipeline and per-runtime rules.
- Project-level: repo `CLAUDE.md` carries the same invariants for Claude Code sessions.
- Delivery chain: `scripts/generate-bundled-assets.mjs` embeds `assets/` into `src/generated/bundled-assets.ts` → `zbrain setup` extracts to `~/.zbrain/` → `zbrain init` injects into `<project>/.claude/`.

*Issue:* the instruction layer has drifted from the code (see §3.3 — docs say "qmd BM25 + strict tier-first ranking", code defaults to FTS5 + tier-*weighted* ranking).

**Skills/tools layer**

- Skills: `assets/skills/zbrain-{ask,learn,ingest,research}/SKILL.md` — each has `<role>`, `<security>`, `<instructions>` blocks. They shell out to `zbrain workspace current` and the CLI; they do not touch the DB directly.
- Agents: `assets/agents/wiki-planner.md`, `wiki-qmd-selector.md`.
- CLI: `src/commands/` — `note`, `learn`, `ingest`, `ask`, `reindex`, `doctor`, `lease`, `sync`, `export/import`, `mcp`.
- MCP server: `src/mcp/server.ts` — 5 tools: `recall`, `remember`, `add_note`, `list_pending`, `get_note` (JSON-RPC 2.0 over stdio).

**Memory layer** — file-first with a derived SQLite cache:

- Files are truth: notes are markdown with YAML frontmatter under `~/.zbrain/workspaces/<ws>/wiki/<tier>/<slug>.md` (`src/core/note-service.ts:1-3`).
- DB is cache: `~/.zbrain/zbrain.db`, rebuildable from files (`src/core/indexer.ts:1-3`, `rebuildWorkspace`). Schema in `src/db/migrations/001-v2-initial.sql`: `projects`, `evidence_sources`, `sessions`, `notes`, `note_fts` (FTS5), `note_fts_map`, `links`, `leases`.
- Git is transport, not truth: "The DB is rebuilt from files after every pull, never synced directly" (`src/core/git-sync.ts:1-4`).

**Identity:** *No mechanism found* for agent or user identity on memory items. Note frontmatter (`note-service.ts:160-172`) has `id`, `tier`, `status`, timestamps, `content_sha`, `sources`, `supersedes`, `review_by` — but no `author`/`agent` field. The only actor trace is the git commit message `sync: <hostname> <ISO>` (`git-sync.ts:169-173`). Session identity exists (`ZBRAIN_SESSION_ID` env or random UUID, `src/core/session.ts:26-31`) but is not stamped onto notes or evidence.

### 2.2 Data & file layout

Local runtime (`~/.zbrain/`, verified on disk):

```
~/.zbrain/
├── config.yml                    # default_workspace: research
├── zbrain.db (+ -wal/-shm)      # registry + index cache
├── engine/ skills/ agents/ templates/   # extracted assets
├── projects/<sha256-16>/         # per-project context output
│   ├── current-task.md           # legacy shared context file
│   └── sessions/<sid>.md         # V2 per-session context (none present yet)
└── workspaces/research/
    ├── .zbrain-layout-version    # "2"
    ├── workspace.md
    ├── wiki/{axioms,mental-models,projects,decisions}/*.md   # 4 real notes
    ├── evidence/{sources,analysis,qa,applied,archive}/<id>/  # 8+ evidence items
    ├── evidence/_index.md
    └── axioms/ mental-models/ projects/ decisions/   # EMPTY v1 leftovers
```

Key files and roles:

| File | Role |
|---|---|
| `wiki/<tier>/<slug>.md` | one memory note; frontmatter = metadata, body = content |
| `evidence/sources/<id>/{raw.md,source.yaml}` | immutable ingested source (SHA-256 pinned) |
| `evidence/qa/<id>/verified-facts.md` | reviewed facts awaiting apply |
| `evidence/applied/<id>/{checkpoint.json,manifest.yaml}` | apply provenance + resume checkpoint |
| `.trash/<id>.md` + `<id>.md.bak` | forget tombstone + content backup |
| `zbrain.db` | derived index: notes, FTS5, links, leases, sessions, evidence, projects |

Git/shared structure: a workspace becomes shared by `zbrain sync init <ws> --remote <url>` — git repo rooted at the workspace directory, single branch (`main`/`master` candidates, `git-sync.ts:55`). No branch-per-agent or folder-permission conventions found.

---

## 3. Memory Pipelines

### 3.1 Write pipeline

Two write paths with deliberately different trust levels (`src/mcp/server.ts:3-5`):

**Trusted fast path** (`zbrain note add` → `src/commands/note.ts:55-98`; MCP `add_note` → `server.ts:225-255`):
1. Resolve workspace (`workspace-resolver.ts`: SQLite project binding → legacy pointer → `config.yml` → single-workspace auto → error).
2. Conflict check (`src/core/conflict.ts:19-50`) — refuses a write to a path an `active` note already occupies unless `supersedes` is declared.
3. `createNote` writes frontmatter (`id` UUID, ISO timestamps, `content_sha` = SHA-256 of body, `sources`, `supersedes`) then file (`note-service.ts:147-193`).
4. `upsertNote` mirrors into `notes` + FTS5 + `links` inside a DB transaction (`indexer.ts:26-78`).

**Untrusted path** (`zbrain learn` / MCP `remember` → evidence pipeline):
- Creates `sources/<id>/raw.md` + `source.yaml` with dual SHA-256 fingerprints (`evidence-store.ts:115+`), state `ingested`.
- MCP `remember` explicitly does **not** auto-apply: "Pending human review" (`server.ts:216-222`).
- State machine `ingested → reviewed → applied → archived` is enforced (`evidence-state.ts:32-43`), with workspace lock at every transition (`evidence-review.ts:41`, `evidence-apply.ts:100`) and the P0/P1 QA gate before apply (`validateQAGate`, `evidence-state.ts:53-65`).

**Selection criteria:** the skill layer defines what gets written — `zbrain-learn/SKILL.md` forbids recording "login pages, paywall gates, or empty pages" and fabricated sources. *No code-level quality filter found* beyond the conflict check; content quality is delegated to the LLM following the skill.

**Versioning & traceability:**
- Supersede-not-overwrite: `supersedeNote` creates a new note linking `supersedes: [old.id]` and flips the old to `superseded` with `superseded_by` (`note-service.ts:223-293`). Old content is never destroyed.
- Optimistic locking via `content_sha` / `ShaMismatchError` (`note-service.ts:46-51`, `229-231`).
- Git history for synced workspaces.

**Known gaps / risks:**
1. **File→DB write is not atomic.** `createNote` (file) and `upsertNote` (DB) are two separate steps in the callers; a crash between them leaves drift. Mitigated by design (`doctor` detects it, `reindex`/lazy-index heal it), but nothing re-runs those automatically except `ask`'s narrow staleness check (§3.3).
2. **MCP default-workspace bug (high impact).** `recall`, `remember`, `add_note`, `list_pending` all default to `firstWorkspace()` — the *alphabetically first directory* under `workspaces/` (`server.ts:296-302`) — not the project's registered workspace or `config.yml` default. With ≥2 workspaces, an MCP agent that omits `workspace` silently reads/writes the wrong one, violating the isolation invariant the whole system is built around. The CLI path uses the proper resolver (`workspace-resolver.ts:56-85`); the MCP path does not.
3. **`add_note` can write axioms (P0 core memory) with zero review.** The tool schema allows `tier: "axioms"` (`server.ts:80`), so any MCP-connected agent has direct write access to the highest-trust tier. See §6.
4. **No write attribution** (who/which agent/which session created a note) — see §2.1.

### 3.2 Management pipeline

**Lifecycle / decay:**
- Note lifecycle state machine (`src/core/lifecycle.ts:19-24`): `active ↔ superseded/archived/forgotten` with restore paths. `forget` moves content to `.trash/<id>.md.bak` plus a tombstone recording reason/original path (`note-service.ts:312-341`).
- Soft decay signal: `review_by` date on notes; `doctor` flags notes past their review date (`doctor.ts:100-114`). This is the only decay mechanism.
- *No mechanism found* for automatic pruning, compression, summarization, or consolidation of notes. Idle-session GC exists (`fixIdleSessions`, 30-day threshold, `doctor.ts:164-175`) but applies to session context files only.

**Promotion:**
- *No mechanism found* for tier promotion (decision → mental-model → axiom) or private → shared promotion. Sharing is all-or-nothing at workspace granularity (`sync init`). Moving a note between workspaces or tiers means manually re-creating the file. README (`Team setup`) confirms: keep team knowledge in a dedicated workspace; `personal` is never synced.

**Conflict resolution:**
- Same-machine: `detectConflict` refuses overlapping writes without a declared supersede (`conflict.ts`); advisory leases with TTL (default 60 s) via `zbrain lease` (`concurrency.ts:22-43`) — advisory only, `createNote`/`supersedeNote` do **not** check leases before writing.
- Cross-machine: git. `sync` commits → `pull --rebase` → push; on conflict it aborts the rebase and demands manual resolution — "Never auto-resolve a rebase conflict" (`git-sync.ts:192-197`).
- Semantic contradictions (two active notes asserting opposite facts in *different* paths): *no mechanism found* — no arbiter agent, no contradiction detection at note level. (The evidence pipeline produces `02-contradictions.md` analysis files per source — seen in `~/.zbrain/workspaces/research/evidence/analysis/` — but nothing consumes them programmatically.)

### 3.3 Read pipeline

**Retrieval mechanism** (`zbrain ask` → `src/commands/ask.ts`):
1. Workspace resolution (registry → legacy pointer → global config → single-ws auto, `workspace-resolver.ts:56-85`).
2. Lazy reindex if wiki files exist but DB rows = 0 (`ask.ts:27-44`, AC-P1-8).
3. Default adapter is **FTS5**, not qmd (`createRetrievalAdapter(..., engine = "fts5")`, `src/adapters/retrieval/index.ts`). Query is tokenized into prefix terms `term*` with AND semantics, special chars stripped (`fts5-adapter.ts:89-97`). Only `status='active'` notes are searchable by default (`fts5-adapter.ts:34`) — superseded/archived memory is correctly out of retrieval.
4. Ranking: `weightedScore = BM25 × tier_weight` (axioms 1.5, mental-models 1.3, projects 1.1, decisions 1.0 — `fts5-adapter.ts:99-104`, `retrieval-ranking.ts:45-69`). Deliberately *not* strict tier-first: "a strongly-relevant decision can outrank a weakly-relevant axiom" (`retrieval-ranking.ts:42-44`).
5. Multi-workspace: secondaries triggered by config keywords or `@workspace` tags; primary fills first, secondaries share remaining slots with a floor of 1 for explicit tags; results deduped by `workspace:path` (`retrieval.ts:67-143`).

**Context injection:** ranked results are rendered to markdown (`current-task.ts:34-104` — tier-grouped tables, full bodies, explicit "Knowledge Gaps" section) and written atomically (tmp+rename) to a **per-session** file `projects/<hash>/sessions/<sid>.md` (`session.ts:42-54`), fixing V1's shared `current-task.md` clobbering between parallel agents. The agent runtime then reads that file.

**Recall vs static knowledge:** memory = wiki notes + evidence only. Session context files are write-only retrieval outputs; *no mechanism found* that reads past sessions back as episodic memory (`readSessionContext` exists in `session.ts:56-60` but no caller replays prior sessions into new contexts).

**Gaps / risks:**
1. **Instruction↔code drift.** `assets/skills/zbrain-ask/SKILL.md` and repo `CLAUDE.md` instruct agents to run `qmd search` with strict tier-first ranking; the shipped CLI uses FTS5 with tier-weighted ranking. An agent following the skill literally bypasses the FTS5 index, the status filter, and lazy-index healing. `assets/engine/retrieval-rules.md` also still says "Materialize ranked context into `current-task.md`" (the V1 shared file).
2. **`context_file` split-brain.** The `projects` table still stores the legacy shared `current-task.md` path as `context_file` (verified in live DB; acknowledged as "AC-P1-9 partial; not yet wired to per-session directory" in `current-task.ts:106-109`). Skills tell agents to read `context_file`, but V2 retrieval writes to `sessions/<sid>.md` — an agent can read a **stale** context (the live `current-task.md` is from 2026-06-16 and contains an empty-result retrieval).
3. Lazy-index staleness check only fires when DB rows = 0 (`ask.ts:41-43`); a *partially* stale index (files edited/added after last index) is not detected at ask time — only `doctor`/`reindex` catch it.

---

## 4. Shared vs Private Memory

- **Private** = any workspace never given a `.git` (the live `research` workspace is in this state — no `.git` present, verified). Stays entirely on-machine.
- **Shared** = workspace turned into a git repo via `sync init` (`git-sync.ts:63-149`), with a well-designed join guard: `SAFE_TO_DISCARD_SCAFFOLD` (`git-sync.ts:61`) enumerates exactly the scaffold files that may be cleaned when adopting a remote branch; anything else untracked aborts the join and even rolls back the `git init` (`git-sync.ts:110-120`) so real notes are never silently discarded.
- **Permissions:** *no mechanism found* beyond the git remote's own ACL. No file-level or row-level read/write permissions, no per-agent write scopes, no signed commits. Within a shared workspace, every clone has full write to every tier including axioms.
- **Sensitive content:**
  - Good: the DB (which could hold cross-workspace data) is never synced; it is rebuilt from files after every pull (`git-sync.ts:212`).
  - Concern: `.trash/` **is** synced by explicit design — ".trash/ syncs so forget propagates" (`git-sync.ts:143-148`). Since `forget` keeps full content in `.trash/<id>.md.bak` (`note-service.ts:334-336`), "forgetting" a sensitive note in a shared workspace *publishes its content to the remote forever* (git history + trash backup). There is no redaction-grade delete.
  - *No mechanism found* for classifying notes as sensitive or restricting which agents may read which tier.

---

## 5. Audit Trail & Observability

**What exists:**
- Git history on synced workspaces: one batch commit per sync, message `sync: <hostname> <ISO>` (`git-sync.ts:169-173`). For git-backed workspaces you *can* reconstruct "what the system believed at time T" (`git checkout <sha>` + `reindex`).
- Evidence provenance: `ingested_at`, `state_updated_at`, dual SHA-256, `workspace_at_ingest` per source (`001-v2-initial.sql:14-28`); apply checkpoints (`evidence/applied/<id>/checkpoint.json`) and citation manifests.
- Note timestamps + supersede chains: `created_at`/`updated_at` in frontmatter, `supersedes`/`superseded_by` links form an inspectable version chain (also in `links` table).
- Session telemetry: `sessions` table upserted on each ask/recall (`session.ts:90-97`).
- Health: `zbrain doctor` runs 8 checks — schema version, DB↔file consistency, orphaned evidence, stale `review_by`, broken supersede links, lease pressure, idle sessions, FTS5↔notes count drift (`doctor.ts:200-219`), with `--fix` for session GC.

**What is missing:**
- *No operation log found* for note mutations (add/supersede/archive/forget/restore) or for queries — the `queries` table was deliberately dropped (`001-v2-initial.sql:87`). On a **local-only** workspace (the current live state), history is unrecoverable beyond the supersede chain: an in-place file edit or a `forget`+trash-purge leaves no trace.
- No monitoring/alerting; `doctor` is manual-invocation only. *No mechanism found* wiring `doctor` or `sync` into session hooks (README suggests a `SessionStart`/`SessionEnd` hook but nothing ships it).

**Live-runtime drift found during this audit (concrete, verified):**

The live `~/.zbrain/zbrain.db` is **V1-shaped**: `PRAGMA user_version = 1`; tables present = `projects`, `evidence_sources`, `sessions`, `queries`; tables **missing** = `notes`, `note_fts`, `note_fts_map`, `links`, `leases`. Meanwhile the workspace layout marker says `2` and 4 real wiki notes exist on disk (`wiki/axioms/agent-prompting-fundamentals.md`, `wiki/mental-models/*.md`, dated 2026-07-04). Consequences right now:

- FTS5 `recall`/`ask` against this DB would fail or return nothing until `initDb` runs (any `setup`/`ask`/`note` invocation with the V2 binary self-heals via `bootstrapV2Schema` + lazy index — `db.ts:25-35`, `ask.ts:64-66` — but nothing has done so yet).
- Whatever wrote those wiki notes on 2026-07-04 did not go through the V2 CLI write path (which would have created the tables). **[assumption]** They were written directly as markdown by an agent/skill, bypassing conflict check and indexing — exactly the drift class `doctor` exists to catch, except `doctor` itself can't run its note checks against a DB with no `notes` table.
- The registered project's `context_file` (`projects` row → `.../21d7ad061d6ecbc1/current-task.md`) holds a stale, empty-result retrieval from 2026-06-16; no `sessions/` files exist.

This is the clearest single takeaway of the audit: **the architecture's self-healing exists but is passive — the live system has been sitting in a degraded state for ~2 days with no signal.**

---

## 6. Safety & Integrity

**Protections that exist (and are good):**
- Immutability: dual SHA-256 on `raw.md`/`source.yaml`, `assertImmutableSourceSnapshot` (`evidence-state.ts:75-85`).
- Workspace lock on every evidence transition (`assertWorkspaceLock`).
- QA gate: apply blocked while any P0/P1 question is `awaiting_external`/`deferred` (`validateQAGate`).
- Citation coverage assertion (`evidence-state.ts:67-73`).
- Path-traversal rejection on workspace names so a workspace can never escape `workspacesDir` or address a foreign qmd collection (`qmd-adapter.ts:78-86`).
- Optimistic locking (`content_sha`), advisory leases, per-session context files, atomic tmp+rename context writes, transactional DB upserts, never-auto-resolve rebase conflicts, join-guard against clobbering local content.
- Lifecycle transition enforcement (`InvalidTransitionError`).

**Guardrails missing:**
1. **Core-memory write access is flat.** MCP `add_note` writes directly to any tier including `axioms` (P0). The evidence pipeline is described as "the moat" for unverified material (`server.ts:4-5`), but the moat has a bridge: nothing distinguishes a trusted agent from an untrusted one, and nothing restricts `add_note` to lower tiers. Combined with the `firstWorkspace()` default (§3.1), an agent can pollute the top-ranked tier of the wrong workspace in one call.
2. **Leases are not enforced** at write time — `createNote`/`supersedeNote` never consult the `leases` table; protection depends on agents voluntarily calling `zbrain lease acquire`.
3. **No content sanitation for retrieval injection.** Note bodies (which may originate from ingested web content) are injected verbatim into context files (`current-task.ts:76-85`). The evidence pipeline gates *facts*, but a prompt-injection payload surviving into an applied note flows straight into future agent contexts. *No mechanism found* for flagging or stripping instruction-like content.
4. **No size/quality limits** on note bodies or evidence at the code level.

**Testing:** solid for the layer's size — 19 test files, ~109 tests (`tests/`), covering lifecycle transitions, concurrency/leases, sync (7 tests incl. join guard), doctor checks, MCP protocol (10), indexer roundtrip, per-session retrieval, registry & workspace migrations, FTS query tokenization, end-to-end smoke. CI runs them (`.github/workflows/test.yml`). *No test found* for the MCP default-workspace behavior with multiple workspaces (which is where the §3.1 bug hides).

---

## 7. Strengths

1. **File-first, DB-as-cache is the right call** and is executed consistently: files are truth (`indexer.ts:2-3`), DB is rebuildable, git never has to merge a binary DB (`git-sync.ts:1-4`). This makes the whole memory layer greppable, diffable, and recoverable.
2. **Supersede-not-overwrite lifecycle** gives cheap versioning, honest conflict semantics across machines (markdown git conflicts instead of silent loss), and a restorable trash.
3. **Two-tier trust model in principle**: untrusted material must cross `learn → review → QA gate → apply` with immutable, hash-pinned sources and citation coverage — a genuinely well-designed evidence moat.
4. **Workspace isolation is enforced in code**, not just prose: workspace-scoped SQL everywhere, `workspace_at_ingest` locks, collection-name traversal guard.
5. **Multi-agent hygiene**: per-session context files (fixing a real V1 clobbering bug), TTL leases, atomic writes, session GC.
6. **`doctor` as reconciliation** acknowledges the file↔DB drift failure mode instead of pretending it can't happen — 8 targeted checks matching the real invariants.
7. **Careful git-join semantics**: `SAFE_TO_DISCARD_SCAFFOLD` + rollback of `git init` on refusal is the kind of edge-case care most projects skip.
8. Good planning provenance: `.kit/planning/SPEC.md`, phase plans, and `AC-AUDIT.md` map acceptance criteria to code, which made this audit's evidence-tracing easy.

---

## 8. Issues & Risks

Ranked by severity.

**Architecture**

- **A1 — MCP workspace default violates the core invariant.** `firstWorkspace()` = alphabetical directory order (`server.ts:296-302`), ignoring the project registry and `config.yml`. Wrong-workspace reads *and writes* with ≥2 workspaces. The CLI resolver already exists and should be used.
- **A2 — Flat write authority over core memory.** `add_note` → `axioms` with no review, no tier restriction, no agent identity (§6.1). The evidence moat only protects the paths that choose to use it.
- **A3 — Instruction layer drift.** Skills/engine rules describe qmd + strict tier-first + `current-task.md`; code ships FTS5 + tier-weighted + per-session files (§3.3.1). Agents following instructions literally bypass the real pipeline.
- **A4 — `context_file` split-brain** (legacy shared path in registry vs per-session writes) means agents can consume stale context (§3.3.2; live example on disk).

**Operational**

- **O1 — Live runtime is in degraded drift right now**: V1 DB schema + unindexed wiki notes + stale context file (§5). Self-healing is passive; nothing surfaced this.
- **O2 — No automated health cadence**: `doctor`/`sync`/`reindex` are all manual; README's session-hook suggestion is unshipped.
- **O3 — No note-operation journal**: local-only workspaces (the current reality — `research` is not git-backed) have no reconstructable history and no offsite copy. A disk failure loses all memory.
- **O4 — Empty v1 tier directories** at the workspace root alongside `wiki/` invite agents to write notes to the wrong (unindexed) location. **[assumption]** This may be how the 2026-07-04 notes were nearly misplaced.

**Security / privacy**

- **S1 — `forget` publishes, not redacts, on shared workspaces**: `.trash/*.md.bak` full content syncs to the remote by design (§4). Fine for team knowledge, dangerous for sensitive notes; the semantics deserve an explicit choice.
- **S2 — No sensitivity classification or read restrictions** anywhere in the model.
- **S3 — Prompt-injection surface**: ingested web content → applied notes → verbatim context injection with no flagging (§6.3).

**Retrieval / relevance**

- **R1 — Prefix-AND FTS query** (`term1* AND term2*`, `fts5-adapter.ts:89-97`): natural-language questions with ≥4 meaningful tokens will often match nothing (all terms required); no stemming, no synonym/semantic fallback. The empty live `current-task.md` retrieval ("No documents matched") is consistent with this failure mode.
- **R2 — Partial index staleness invisible at ask time** (only the rows==0 case triggers lazy reindex, `ask.ts:41-43`).
- **R3 — No feedback loop**: queries aren't logged (table dropped), so there's no data on retrieval misses to tune weights or spot knowledge gaps beyond the per-request "Knowledge Gaps" section.

---

## 9. Recommendations

**Short-term (high-impact, low-effort)**

1. **Heal the live runtime** (addresses O1): run `zbrain setup` (or `zbrain reindex --workspace research`) with the V2 binary to migrate the DB and index the 4 wiki notes; delete the empty v1 tier dirs (O4); confirm with `zbrain doctor`.
2. **Fix A1**: make MCP tools resolve the workspace via `resolveActiveWorkspace(paths)` (the resolver already exists in `workspace-resolver.ts`) instead of `firstWorkspace()`; add a multi-workspace MCP test.
3. **Restrict A2 minimally**: reject `tier: "axioms"` (and optionally `mental-models`) in `add_note`, routing those through `remember` — one guard clause in `toolAddNote`, consistent with the moat the code already claims.
4. **Re-align the instruction layer (A3/A4)**: update `zbrain-ask/SKILL.md`, `engine/retrieval-rules.md`, and `CLAUDE.md` to say "run `zbrain ask`" (letting the CLI own adapter/ranking) rather than prescribing qmd mechanics; either write the per-session path back into the registry `context_file` or have `ask` print the session file path for the agent to read (it already outputs `session_id`).
5. **Decide S1 explicitly**: either `.gitignore` `.trash/` in shared workspaces (forget = local-only) or document loudly that forget-on-shared is permanent publication.

**Medium-term (architecture / tooling)**

6. **Attribution**: add `author`/`agent`/`session_id` frontmatter fields stamped at `createNote`/`supersedeNote`, and pass `--author` through to sync commits. Links every memory to its writer; prerequisite for any future trust tiering (A2 done properly).
7. **Operation journal**: an append-only `journal.ndjson` per workspace (op, note id, actor, sha, timestamp) written in the same code paths as note mutations. Restores reconstructability for local-only workspaces (O3) and gives `doctor` a richer signal.
8. **Automate health cadence (O2)**: ship the `SessionStart`/`SessionEnd` hook README already describes — `sync` (if git-backed) + `doctor` summary at session start; make `ask`'s staleness check compare file mtimes/`content_sha` against DB rows (R2).
9. **Promotion tooling**: `zbrain note promote <id> --to-tier <t>` / `--to-workspace <ws>` implemented as supersede-in-place + create-in-target, preserving the chain — turning the currently manual private→shared and decision→axiom flows into audited operations.
10. **Retrieval robustness (R1)**: OR-fallback when AND yields zero hits; strip stopwords before tokenizing; re-add a lightweight query log to measure miss rate (R3).

**Long-term**

11. Hybrid retrieval (embeddings alongside FTS5 behind the existing `RetrievalAdapter` registry — the seam in `adapters/retrieval/index.ts` is already built for this).
12. A consolidation/review agent: periodically reads a tier, proposes merges/supersedes/contradiction resolutions as *evidence items* (so they pass the existing QA gate) — an arbiter that reuses the moat instead of new machinery, and finally consumes those `02-contradictions.md` analyses.
13. Per-agent capability model for MCP (read-only vs evidence-only vs wiki-write), once attribution (rec 6) exists to hang it on.
14. Episodic memory: distill session context files into candidate `decisions/` notes at session end (via the evidence pipeline), instead of GC-ing them unread.

---

## 10. Next Experiments

1. **Retrieval miss-rate baseline** (validates R1/rec 10): replay ~30 realistic questions against the `research` workspace after reindexing; record hit-rate and top-1 tier with the current AND-prefix query vs an OR-fallback build. Signal: % of queries returning 0 results; target < 10%.
2. **Drift-detection latency** (validates rec 8): intentionally hand-edit a wiki note (simulating the 2026-07-04 event), measure how many sessions pass before the system notices, with and without the SessionStart doctor hook. Signal: time-to-detection → should drop to 0 sessions.
3. **Two-workspace MCP probe** (validates rec 2): create a second workspace alphabetically before `research`, issue `recall`/`add_note` without `workspace` from an MCP client, assert the resolved workspace matches the project binding. Currently expected to fail; becomes a regression test.
4. **Tier-weight sensitivity**: with a populated wiki (≥50 notes), grid-search `TIER_WEIGHTS` (e.g. axioms 1.25–2.0) against a hand-labeled relevance set of ~20 queries. Metric: nDCG@8 per weight vector — grounds the currently hand-picked 1.5/1.3/1.1/1.0.
5. **Shared-workspace fire drill**: `sync init` a scratch workspace to a private remote, run the two-teammate flow from README on two checkouts (or two `ZBRAIN_HOME`s), force a same-file edit collision, and verify the conflict surfaces as a readable markdown conflict + post-resolution `reindex` converges. Signal: zero silent data loss; document the resolution playbook that falls out.
6. **Forget-semantics check** (validates rec 5): forget a note in the scratch shared workspace, sync, and inspect the remote — confirm (and then decide about) content presence in `.trash/` and git history.

Iterate: re-run this audit's §5 live-state checks (DB schema version, notes-table row count vs wiki file count, context-file freshness) after each change — they are three one-liners and together form a cheap "memory health" smoke test worth wiring into `doctor`.
