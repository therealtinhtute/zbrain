---
id: 01KYS3EP5V6WSGWDES5RZ8C10F
type: plan
intake_id: 01KYS3FVP9YPX7GJVFE7SGHVSF
lane: high-risk
status: completed
created: 2026-07-30
updated: 2026-07-31
---

# Plan: Trusted Agent Memory Vertical Slice

## Outcome
- result: A solo technical builder can store technical and general personal knowledge as local atomic claims, then call `zbrain ask` to receive a versioned JSON context containing only approved, provenance-backed claims; drafts remain separate promotion candidates, and unresolved gaps or conflicts fail closed.
- success_signals:
  - Approved claims are returned with citations and resolved scopes; drafts never appear in trusted context.
  - External factual claims cannot be approved without immutable local evidence, while owner-confirmed preferences and decisions follow their type-specific proof rule.
  - Conflicting approved claims block trusted context until explicitly resolved; superseded or revoked claims remain auditable but are excluded from retrieval.
  - Existing Go workspaces and legacy Markdown are preserved without being silently trusted or indexed.
  - The derived index can be deleted and deterministically rebuilt from canonical Markdown and evidence metadata.
  - On the owner's recorded development machine, a representative 100,000-claim corpus completes `zbrain ask` at p95 no slower than 2 seconds; p95 below 500ms remains a later optimization target.

## Authority and Requirements
- authority:
  - Owner-approved trusted hybrid memory design from the 2026-07-30 `/think` and interview session.
  - `CLAUDE.md` Go-native direction, runtime layout, embedded-assets authority, and required Go tests/smoke proof.
  - Current executable behavior in `internal/cli/` and `internal/runtime/`, which overrides stale Bun/TypeScript documentation.
  - Product trust thesis in `docs/planning/STRATEGY.md` and relevant historical decisions in `docs/planning/V2-ARCHITECTURE.md`, treated as decision history rather than current implementation truth.
  - SQLite FTS5 official behavior for BM25 ranking, integrity checks, and rebuildable indexes.
- requirements:
  - R1 [accepted]: The implementation remains Go-native, local-first, and distributable as the existing standalone CLI; no Node, Bun, TypeScript, network service, or second runtime is introduced. | source: `CLAUDE.md`; owner-approved design
  - R2 [accepted]: Each durable memory claim is one Markdown file under exactly one existing wiki tier: `axioms`, `mental-models`, `projects`, or `decisions`; the tier describes semantic role and is not a fixed ranking priority. | source: owner-approved design
  - R3 [accepted]: Claim lifecycle is `draft -> approved -> superseded|revoked`; approved content is immutable, and updates create a new claim linked by `supersedes` while preserving audit history. | source: owner-approved design
  - R4 [accepted]: External factual claims require immutable local evidence snapshots with origin, capture time, and content hash; personal preferences and decisions may be owner-confirmed; mental models and principles must link supporting claims or evidence. | source: owner-approved design
  - R5 [accepted]: Every query resolves one primary workspace from `workspace current` or `--workspace`; secondary workspaces are read-only and included only through explicit `--include` arguments, with all resolved scopes echoed in JSON. | source: owner-approved design
  - R6 [accepted]: The first agent integration is CLI plus a versioned JSON contract; zbrain returns trusted context, citations, conflicts, gaps, and separate promotion candidates but does not invoke an LLM or generate the final natural-language answer. | source: owner-approved design
  - R7 [accepted]: Markdown and immutable evidence files are canonical; trusted lifecycle mutations occur through CLI commands, while a local SQLite FTS5 database is a disposable, rebuildable derived cache. | source: owner-approved design
  - R8 [accepted]: Retrieval hard-filters scope and approved/current lifecycle state before BM25 plus metadata ranking; current tier-first or substring scoring is not preserved. | source: owner-approved design
  - R9 [accepted]: Existing Go workspace content is preserved; legacy Markdown lacking the claim schema is reported and quarantined from trust/retrieval until explicitly imported as a draft and approved. No migration from deleted Bun/TypeScript V2 runtime data is required. | source: owner-approved design
  - R10 [accepted]: `zbrain ask` may break its current `0.1.0-go` JSON shape and must fail closed on insufficient approved knowledge or unresolved same-scope conflicts; related drafts may appear only as separate promotion candidates. | source: owner-approved design
  - R11 [accepted]: The release gate prioritizes trust and data correctness; a representative 100,000-claim benchmark must achieve p95 at or below 2 seconds on the recorded development machine, while p95 below 500ms is tracked but does not block the first release. | source: owner-approved design
  - R12 [accepted]: Embedded runtime assets, Go CI, README/spec surfaces, and user-facing help touched by this slice must describe only behavior the Go binary actually provides; setup must not install instructions for deleted commands. | source: current repo audit; `CLAUDE.md`
  - R13 [accepted]: Verification includes focused unit/integration tests, isolated `ZBRAIN_HOME` smoke checks, deterministic delete-and-rebuild equivalence, migration preservation proof, and the 100,000-claim benchmark. | source: `CLAUDE.md`; owner-approved design

## Non-goals
- NG1: Calling an LLM or generating a final natural-language answer inside zbrain.
- NG2: MCP server support in the first vertical slice.
- NG3: Vector embeddings, semantic retrieval, or hybrid vector/BM25 ranking.
- NG4: Agent session transcripts, working-memory persistence, or automatic session summarization.
- NG5: Team sharing, sync, authentication, permissions, or multi-user conflict resolution.
- NG6: Automatic URL crawling, model-based claim extraction, or unattended promotion of drafts.
- NG7: Migration from the deleted Bun/TypeScript V2 SQLite runtime.
- NG8: Direct editing of approved claims as a supported mutation path.
- NG9: Backward compatibility with the current `zbrain ask` JSON schema.
- NG10: Treating stale planning, release, or asset documentation as implemented product behavior.

## Approach and Risks
- approach:
  - Keep canonical state in one Markdown file per claim and one immutable local snapshot directory per evidence item. Add only two runtime dependencies: a typed YAML frontmatter parser and a CGo-free SQLite driver.
  - Use schema `zbrain.claim/v1`. Claim filenames are stable `clm_` plus 32 lowercase hexadecimal characters; evidence directories are `evd_` plus 32 lowercase hexadecimal characters. Tier and workspace are derived from the canonical path, never trusted from caller-supplied metadata.
  - Required claim metadata is `schema`, `id`, `status`, `title`, `basis`, `created_at`, and `created_by`. Optional relation fields are `evidence_ids`, `supporting_claim_ids`, `supersedes`, `conflicts_with`, and `tags`. `basis` is one of `owner`, `evidence`, or `derived`; approval validation enforces the matching proof rule.
  - Keep approved claims immutable. `claim supersede` creates a draft replacement linked to the current claim; approving the replacement atomically moves the old claim to `superseded`. `claim revoke` preserves the file and reason while excluding it from trusted retrieval.
  - Store evidence at `workspaces/<workspace>/evidence/sources/<evidence-id>/source.yaml` plus an immutable `raw` snapshot. `source.yaml` records original path or URI, capture time, media type, byte length, and SHA-256 hash. The first slice accepts prepared local files and does not fetch network content.
  - Store one derived database per workspace at `$ZBRAIN_HOME/indexes/<workspace>.sqlite`. Index both approved claims and drafts, but query them through separate channels. Approved/current lifecycle state and explicit scopes are hard filters before BM25 ranking; drafts may only populate `promotion_candidates`.
  - Treat conflict detection as structural, not semantic: `conflicts_with` and `supersedes` relations are canonical metadata. `ask` blocks when retrieved current approved claims have an unresolved explicit conflict. A knowledge gap is deterministic: no approved match after hard filters, or a blocked conflict. The slice must not claim LLM-level semantic sufficiency detection.
  - Keep index consistency fail-closed. CLI mutations use same-directory temporary files and atomic rename for canonical writes, mark the workspace index dirty before mutation, update the index transactionally, and clear the marker only after success. `ask` rejects a dirty or failed-integrity index; `reindex` rebuilds a temporary database from canonical files and atomically replaces the cache.
  - Preserve legacy Markdown byte-for-byte. Files without a valid `zbrain.claim/v1` schema are reported as `legacy_unindexed` and excluded until explicitly imported through `claim draft` and approved.
  - Redesign the public CLI around `evidence add`, `claim draft`, `claim approve`, `claim supersede`, `claim revoke`, `reindex`, and `ask`. Agent-facing commands emit versioned JSON with `schema_version: 1`; `ask` returns resolved scopes, trusted claims with citations, conflicts, gaps, promotion candidates, and index metadata. zbrain never calls an LLM.
  - Ship the approved behavior as one stable phase with dependency-ordered waves. Partial internal checkpoints may be tested, but the user-facing vertical slice is not complete until lifecycle, evidence, index, trusted query, migration safety, runtime assets, CI, and final proof all agree.
- constraints:
  - Go 1.24 is required by `go.mod`; the current local Go 1.22.2 toolchain must be upgraded or made available before executable proof.
  - `assets/` remains the embedded runtime source of truth. Existing asset paths that currently advertise dead commands must be overwritten with accurate content or an inert deprecation/redirect notice so a repeated `setup` cannot leave active false instructions.
  - Primary and included workspaces remain isolated on disk and in separate indexes. Cross-workspace retrieval is read-only and only occurs for explicit `--include` values.
  - Existing `workspace current`, config parsing, `ZBRAIN_HOME`, workspace tiers, and setup behavior remain compatible unless a requirement explicitly changes them.
  - No API keys, remote accounts, MCP servers, background daemons, network fetchers, or migration of deleted Bun/TypeScript runtime state are introduced.
- dependencies:
  - `modernc.org/sqlite` through `database/sql`, with an executable assertion that `sqlite_compileoption_used('ENABLE_FTS5')` returns `1` in the target binary.
  - A maintained YAML v3 package for typed frontmatter and evidence metadata; it is used only for deterministic parse/render, not a second runtime.
  - Go 1.24 toolchain on development and CI environments.
- rejected_alternatives:
  - Keep the current linear substring search: rejected because it scans every Markdown file, has no lifecycle filter or integrity model, and is not credible at 100,000 claims.
  - Make SQLite canonical: rejected because it removes inspectable local files as durable truth and makes rollback/export recovery depend on database health.
  - Use Bleve or a custom inverted index: rejected for the first slice because SQLite FTS5 already provides BM25, weighted columns, integrity checks, and deterministic rebuild with less custom index code.
  - Add vector retrieval: rejected because it adds model/index/network or local-model burden before deterministic trust behavior is proven.
  - Split lifecycle, indexing, and ask into separately released product phases: rejected because each partial release would expose an incomplete trust guarantee or leave shipped assets inconsistent with executable behavior.
- risks:
  - risk: The CGo-free SQLite build may not expose FTS5 on every target.
    mitigation: Prove the compile option and create/query a temporary FTS5 table before writing canonical user data. Pause the phase if the assertion fails; do not silently fall back to linear search.
    recovery: Keep all canonical files untouched, remove only the disposable test/index database, and refine the dependency choice before resuming.
  - risk: Review friction may erase the value advantage over ordinary local search.
    mitigation: Keep drafts quarantined but return only relevant drafts as `promotion_candidates` on first useful retrieval; never require reviewing an unrelated global inbox before asking.
    recovery: Measure dogfood behavior after the slice; any move toward automatic promotion requires a new approved product decision, not an implementation shortcut.
  - risk: Semantic conflicts or answer sufficiency cannot be inferred reliably without an LLM.
    mitigation: Limit the contract to explicit conflict relations and deterministic no-match/conflict gaps. Name this limitation in help, assets, tests, and JSON status values.
    recovery: Do not add heuristic semantic conflict claims; defer richer detection to a separately approved scope.
  - risk: A failed index update could make trusted answers stale.
    mitigation: Dirty markers, transactional upserts, integrity checks, and temporary-database rebuild make stale state observable and block `ask`.
    recovery: Run `zbrain reindex --workspace <name>`; if rebuilding fails, preserve canonical files and report exact invalid/legacy paths.
  - risk: Migration could silently trust or overwrite arbitrary existing Markdown.
    mitigation: `workspace create` must refuse an existing workspace, scanners never rewrite legacy files, and migration tests compare content hashes before and after reindex/import attempts.
    recovery: Stop on any planned overwrite; the user resolves the specific legacy file before import.
  - risk: Runtime assets and repository docs may continue to advertise deleted Bun/qmd/learn/ingest behavior.
    mitigation: Audit every embedded asset, add forbidden-stale-command tests, replace old active instructions at the same extracted paths, and convert CI/help/README/spec surfaces to the Go command contract.
    recovery: Treat any stale active instruction as a release blocker; historical planning/audit documents may remain only when explicitly labeled historical.
  - risk: The 100,000-claim corpus may exceed the 2-second p95 release gate on the owner's machine.
    mitigation: Use per-workspace FTS5 indexes, batched rebuild transactions, prepared statements, hard filters, bounded result sets, and a committed deterministic corpus generator.
    recovery: Profile and optimize within the approved BM25/index design. Pause rather than weakening trust/data gates or shipping above 2 seconds.

## Phases and Verification
<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. Each planned phase records phase_slug, story_id, status, goal, depends_on, waves, tasks, and checks. -->
- planning_status: planned
- phases:
  - phase_slug: trusted-memory-vertical-slice
    story_id: 01KYS43VJXHWVKNSYZNGCN2753
    status: done
    goal: Deliver the approved provenance-gated local memory lifecycle, rebuildable indexed retrieval, and trusted `ask` JSON contract end to end without reintroducing the deleted runtime.
    depends_on: none
    allowed_surfaces:
      - `internal/runtime/` claim, evidence, workspace safety, indexing, scope resolution, and trusted query behavior.
      - `internal/cli/` command parsing, help, and versioned JSON contracts.
      - `go.mod`, generated `go.sum`, `Makefile`, and `.github/workflows/test.yml` for Go dependencies and proof gates.
      - Embedded `assets/` plus directly conflicting current surfaces `README.md`, `wiki-spec.md`, `AGENTS.md`, and `CONTRIBUTING.md`.
      - Focused Go tests, deterministic fixtures/generators, and isolated smoke/benchmark support required by R13.
    avoided_surfaces:
      - `CLAUDE.md` project policy.
      - Historical `.kit/` artifacts, `AGENTIC_MEMORY_AUDIT.md`, old release history, and deleted Bun/TypeScript source.
      - MCP, LLM providers, vector databases, sync/team/auth, session storage, network crawling, and background services.
      - Existing user runtime data outside isolated test homes during implementation and verification.
    waves:
      - wave: 1-contract
        goal: Lock canonical schemas, identifiers, validation, and round-trip behavior before filesystem or CLI mutation work.
        depends_on: none
        tasks:
          - task: 1.1 Define the claim contract and parser/renderer.
            requirements: [R2, R3, R4, R9]
            depends_on: none
            touched_surfaces:
              - add `internal/runtime/claim.go`
              - add `internal/runtime/claim_test.go`
              - reuse or replace the current ad-hoc frontmatter parsing in `internal/runtime/search.go` only after claim round-trip tests exist
              - update `go.mod` and add `go.sum` for the YAML dependency
            avoided_surfaces:
              - CLI dispatch
              - SQLite/index code
              - runtime assets and docs
            expected_outputs:
              - Typed `zbrain.claim/v1` metadata, tier/status/basis enums, stable `clm_<32hex>` ID generation, relation validation, deterministic Markdown parse/render, and path-derived tier validation.
              - Approval guard functions for `owner`, `evidence`, and `derived` basis without performing filesystem writes.
              - Explicit structural semantics for `supersedes` and `conflicts_with`; no semantic-conflict heuristic.
            checks:
              - `go test ./internal/runtime -run 'TestClaim|TestClaimValidation|TestClaimRoundTrip|TestClaimID' -count=1`
              - Inspect a round-tripped fixture and prove body bytes plus normalized metadata remain deterministic.
            stop_conditions:
              - Parser round-trip changes claim body content.
              - A path tier can disagree with the parsed tier contract without an error.
              - An external or derived claim can pass approval validation without required evidence/support links.
      - wave: 2-canonical-stores
        goal: Implement data-safe claim and evidence persistence plus legacy quarantine on canonical files.
        depends_on: wave 1-contract
        tasks:
          - task: 2.1 Implement atomic claim lifecycle storage and legacy scanning.
            requirements: [R2, R3, R7, R9]
            depends_on: task 1.1
            touched_surfaces:
              - add `internal/runtime/claim_store.go`
              - add `internal/runtime/claim_store_test.go`
              - modify `internal/runtime/workspace.go`
              - modify `internal/runtime/workspace_test.go`
            avoided_surfaces:
              - evidence snapshot implementation
              - SQLite/index code
              - public CLI command parsing
            expected_outputs:
              - Atomic draft writes, explicit approval, replacement-draft creation, atomic supersede-on-approval, revoke-with-reason, immutable approved-file handling, and lookup by stable claim ID.
              - Existing workspace creation refuses overwrite instead of rewriting `workspace.md` or evidence index files.
              - Workspace scan classifies valid claims and reports schema-less/invalid Markdown as `legacy_unindexed` without modifying bytes.
            checks:
              - `go test ./internal/runtime -run 'TestClaimStore|TestApprove|TestSupersede|TestRevoke|TestLegacy|TestCreateWorkspace' -count=1`
              - Hash a legacy fixture before and after scan/reindex preparation and prove the hashes match.
            stop_conditions:
              - Any lifecycle transition edits approved claim content in place.
              - Existing workspace creation overwrites a file.
              - Legacy Markdown is automatically approved, indexed as trusted, moved, or rewritten.
          - task: 2.2 Implement immutable evidence snapshots and proof validation inputs.
            requirements: [R4, R7]
            depends_on: task 1.1
            touched_surfaces:
              - add `internal/runtime/evidence.go`
              - add `internal/runtime/evidence_test.go`
              - preserve the existing `evidence/sources/` workspace boundary
            avoided_surfaces:
              - network fetchers
              - evidence analysis/QA/apply pipelines
              - claim approval mutation beyond calling the contract validator
            expected_outputs:
              - Stable `evd_<32hex>` IDs, `source.yaml`, byte-for-byte `raw` snapshots, SHA-256 verification, origin/media/size/capture metadata, duplicate-safe creation, and read-only post-capture behavior.
              - Evidence lookup verifies workspace ownership and rejects cross-workspace references.
            checks:
              - `go test ./internal/runtime -run 'TestEvidence|TestSnapshot|TestEvidenceHash|TestEvidenceWorkspaceIsolation' -count=1`
              - Mutate a copied source fixture after capture and prove the stored raw bytes/hash do not change.
            stop_conditions:
              - Capturing evidence modifies or depends on the source file after the copy completes.
              - A claim in one workspace can reference evidence from another workspace.
              - Any network access is required.
      - wave: 3-index
        goal: Add the disposable per-workspace FTS5 cache, fail-closed freshness, deterministic rebuild, and indexed retrieval primitives.
        depends_on: wave 2-canonical-stores
        tasks:
          - task: 3.1 Add index paths, FTS5 capability proof, schema, upsert, integrity, and rebuild.
            requirements: [R7, R8, R9, R11, R13]
            depends_on: [task 2.1, task 2.2]
            touched_surfaces:
              - modify `internal/runtime/paths.go`
              - modify or extend `internal/runtime/paths_test.go`
              - add `internal/runtime/index.go`
              - add `internal/runtime/index_test.go`
              - replace the linear implementation in `internal/runtime/search.go` while preserving only reusable Unicode/text normalization behavior that remains valid
              - replace focused expectations in `internal/runtime/search_test.go`
              - update `go.mod` and `go.sum` for `modernc.org/sqlite`
            avoided_surfaces:
              - public CLI grammar
              - cross-workspace query merging
              - final `ask` response assembly
            expected_outputs:
              - `$ZBRAIN_HOME/indexes/<workspace>.sqlite`, a workspace dirty marker, FTS5 compile-option probe, relational claim/evidence/relation metadata, weighted title/tags/body FTS columns, transactional upsert, integrity check, and temporary-database rebuild/replace.
              - Only valid claim-schema files enter the index; approved and draft rows remain distinguishable; evidence raw content is never searchable.
              - A deterministic 100,000-claim corpus generator and opt-in p95 benchmark test are added in `internal/runtime/index_benchmark_test.go`.
            checks:
              - `go test ./internal/runtime -run 'TestIndex|TestFTS5|TestReindex|TestIndexIntegrity|TestSearch' -count=1`
              - `ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v`
              - Build an index, capture ordered results, trash only the isolated test index, rebuild, and compare ordered IDs/scores/status filters.
            stop_conditions:
              - `sqlite_compileoption_used('ENABLE_FTS5')` is not `1` in the built target.
              - Dirty or corrupt indexes can still serve trusted results.
              - Rebuild changes canonical files or includes legacy/evidence raw content.
              - The benchmark exceeds 2 seconds p95 after bounded profiling within the approved design.
      - wave: 4-trusted-query
        goal: Assemble deterministic trusted context across one primary and explicit read-only secondary workspaces.
        depends_on: wave 3-index
        tasks:
          - task: 4.1 Implement scope resolution and the trusted query response model.
            requirements: [R5, R6, R8, R10]
            depends_on: task 3.1
            touched_surfaces:
              - add `internal/runtime/query.go`
              - add `internal/runtime/query_test.go`
              - minimally extend `internal/runtime/workspace.go` only where reusable workspace validation is needed
            avoided_surfaces:
              - automatic project binding
              - implicit global search
              - LLM answer generation or semantic sufficiency judgment
            expected_outputs:
              - One primary scope from current workspace or explicit override, deduplicated explicit includes, per-workspace index access, deterministic merged ranking, and echoed resolved scopes.
              - `schema_version: 1` response types containing trusted claims with citation IDs/paths, structural conflicts, deterministic gaps, separate draft promotion candidates, and index freshness metadata.
              - Status is blocked for dirty indexes or unresolved retrieved conflicts, gap for no approved matches, and ready only for conflict-free approved context.
            checks:
              - `go test ./internal/runtime -run 'TestResolveScopes|TestTrustedQuery|TestPromotionCandidates|TestConflict|TestGap|TestWorkspaceIsolation' -count=1`
              - Prove an omitted secondary workspace never affects IDs, ranking, gaps, or promotion candidates.
            stop_conditions:
              - A draft appears in trusted claims.
              - A non-included workspace influences output.
              - An explicit unresolved conflict returns ready status.
              - JSON implies semantic completeness beyond deterministic match/conflict rules.
      - wave: 5-cli
        goal: Expose canonical mutations, rebuild, and trusted ask through the approved Go CLI and versioned JSON.
        depends_on: wave 4-trusted-query
        tasks:
          - task: 5.1 Add evidence and claim lifecycle commands with machine-readable output.
            requirements: [R3, R4, R6, R7, R9]
            depends_on: task 4.1
            touched_surfaces:
              - modify `internal/cli/cli.go`
              - modify `internal/cli/cli_test.go`
            avoided_surfaces:
              - compatibility shims for deleted `learn`, `ingest`, or `note` commands
              - interactive TUI/prompts
              - direct edits to canonical files outside runtime stores
            expected_outputs:
              - `zbrain evidence add --file <path> --origin <uri-or-path> [--workspace <name>]`.
              - `zbrain claim draft --tier <tier> --title <title> --basis <owner|evidence|derived> [--evidence <id>]... [--support <id>]... [--conflicts-with <id>]... [--workspace <name>]`, reading the claim body from stdin.
              - `zbrain claim approve <id> [--workspace <name>]`, `zbrain claim supersede <id> ...`, and `zbrain claim revoke <id> --reason <text> [--workspace <name>]`.
              - All lifecycle commands return JSON with schema version, affected IDs/status, workspace, canonical path, and index freshness; validation failures write no partial trusted state.
            checks:
              - `go test ./internal/cli -run 'TestRunEvidence|TestRunClaim|TestClaimValidation|TestLifecycleJSON' -count=1`
              - Decode every success output and representative validation error payload as JSON in tests.
            stop_conditions:
              - A command can approve an invalid proof basis.
              - A canonical mutation succeeds while an index failure is hidden or leaves `ask` enabled on stale state.
              - Agent-facing success output is not valid versioned JSON.
          - task: 5.2 Redesign `reindex`, `ask`, help, and the isolated CLI smoke path.
            requirements: [R5, R6, R8, R10, R11, R13]
            depends_on: task 5.1
            touched_surfaces:
              - modify `internal/cli/cli.go`
              - modify `internal/cli/cli_test.go`
              - modify `Makefile`
            avoided_surfaces:
              - old `ask` JSON compatibility
              - context-file materialization
              - automatic workspace discovery beyond current/explicit flags
            expected_outputs:
              - `zbrain reindex [--workspace <name>]` returns indexed, draft, approved, invalid, and legacy counts plus integrity/freshness metadata.
              - `zbrain ask [--workspace <name>] [--include <name>]... <query>` emits the versioned trusted query contract and never generates a natural-language answer.
              - Help text names exact trust limits. `make smoke` uses a temporary `ZBRAIN_HOME` and exercises setup, workspace creation, evidence capture, draft, approval, reindex, trusted ask, gap, promotion candidate, supersede/revoke, and legacy preservation.
            checks:
              - `go test ./internal/cli -run 'TestRunReindex|TestRunAsk|TestAskScopes|TestAskGap|TestAskConflict' -count=1`
              - `make build`
              - `make smoke`
            stop_conditions:
              - Smoke touches the real user runtime.
              - `ask` silently searches an include not named by the caller.
              - Help or output claims LLM-generated answers, semantic conflict detection, or automatic promotion.
      - wave: 6-coherence-and-proof
        goal: Remove active repository/runtime drift and execute the complete trust, data, migration, build, smoke, and scale gates.
        depends_on: wave 5-cli
        tasks:
          - task: 6.1 Align embedded assets, current docs, and CI with the executable Go contract.
            requirements: [R1, R6, R12, R13]
            depends_on: task 5.2
            touched_surfaces:
              - audit and update every active file under `assets/`
              - modify `internal/runtime/assets_test.go`
              - modify `README.md`
              - rewrite `wiki-spec.md` as the current trusted-memory product/runtime spec
              - modify `AGENTS.md` and `CONTRIBUTING.md` to describe the Go repository
              - replace `.github/workflows/test.yml` with Go 1.24 test/build gates
              - modify `Makefile` only for final test/smoke/benchmark targets not completed in task 5.2
            avoided_surfaces:
              - rewriting historical audits or planning records as if they were current implementation
              - adding runtime skills for MCP, vector search, session memory, sync, or network research
              - deleting user-owned files from an existing runtime
            expected_outputs:
              - `setup` overwrites existing extracted active instruction paths with behavior supported by the new binary; obsolete skill/agent paths receive accurate deprecation/redirect content rather than live instructions for dead commands.
              - Embedded ask/retrieval/evidence rules use explicit scopes, claim lifecycle, evidence snapshots, promotion candidates, citations, and fail-closed gaps/conflicts; no qmd, tier-first ranking, context-file writes, or Bun commands remain active.
              - CI runs Go 1.24 tests and build on Linux/macOS. Current repository docs and help no longer advertise deleted command families as planned current work.
              - Asset tests walk the embedded filesystem and reject an explicit stale-token/command list.
            checks:
              - `go test ./internal/runtime -run 'TestExtractBundledAssets|TestBundledAssetsMatchCurrentCLI' -count=1`
              - `grep -RInE 'qmd|bun (install|run|test)|zbrain:(learn|ingest|research)|zbrain (learn|ingest|note|mcp|sync)' assets README.md wiki-spec.md AGENTS.md CONTRIBUTING.md .github/workflows/test.yml` returns no active instruction matches; documented historical/deprecation mentions must be individually reviewed.
              - `git diff --check`
            stop_conditions:
              - Re-running setup leaves an active extracted instruction that invokes a nonexistent command.
              - CI still installs Bun or omits the Go test/build gate.
              - Current docs claim behavior outside the approved vertical slice.
          - task: 6.2 Run final acceptance and record exact proof without changing product scope.
            requirements: [R1, R3, R4, R5, R7, R8, R9, R10, R11, R12, R13]
            depends_on: task 6.1
            touched_surfaces:
              - no product files unless a failing approved acceptance check requires a scoped fix in its owning task surface
              - append-only plan Progress/Validation and zharness run/check records during `work` and `check`
            avoided_surfaces:
              - weakening trust/data assertions to make tests pass
              - increasing the 2-second p95 gate
              - adding unapproved retrieval or migration behavior
            expected_outputs:
              - Passing unit/integration suite, Go build, isolated full smoke, deterministic rebuild equivalence, legacy hash preservation, cross-workspace isolation, conflict/gap fail-closed behavior, and recorded 100,000-claim p95 result with GOOS/GOARCH/CPU count.
              - A clean `check` verdict or explicit proof gaps; no claim of completion while Go 1.24 or FTS5 proof is unavailable.
            checks:
              - `go test ./...`
              - `make build`
              - `make smoke`
              - `ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v`
              - `git diff --check`
              - `git status --short`
            stop_conditions:
              - Any trust/data/migration test fails.
              - FTS5 capability or index integrity is unproven.
              - The 100,000-claim p95 exceeds 2 seconds.
              - Final assets/help/docs disagree with executable behavior.

## Progress
<!-- Append-only durable entries record timestamp, phase, wave, task, task_status, run_id, trace_id, exact verification/result, and changed surfaces or blocker. -->
- timestamp: 2026-07-30T09:56:53Z
  phase: trusted-memory-vertical-slice
  wave: phase-start
  task: Start trusted memory vertical slice
  task_status: in-progress
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: none
  changed_surfaces: [docs/plans/active/trusted-agent-memory.md]
  verification: "zharness run create --slug trusted-memory-vertical-slice --plan-id 01KYS3EP5V6WSGWDES5RZ8C10F --json -> 01KYS78XDZYD8ANSW6P7J8BFPV"
  blocker: none
- timestamp: 2026-07-30T09:59:59Z
  phase: trusted-memory-vertical-slice
  wave: 1-contract
  task: "1.1 Define the claim contract and parser/renderer"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: 01KYS7EVH3VBE5T0JGPSCB9E58
  changed_surfaces: [go.mod, go.sum, internal/runtime/claim.go, internal/runtime/claim_test.go]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH gofmt -w internal/runtime/claim.go internal/runtime/claim_test.go && PATH=$HOME/.local/share/go1.24.0/bin:$PATH go test ./internal/runtime -run 'TestClaim|TestClaimValidation|TestClaimRoundTrip|TestClaimID' -count=1 -> ok github.com/therealtinhtute/zbrain/internal/runtime 0.002s"
  blocker: none
- timestamp: 2026-07-30T10:03:00Z
  phase: trusted-memory-vertical-slice
  wave: 2-canonical-stores
  task: "2.1 Implement atomic claim lifecycle storage and legacy scanning"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: none
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/claim_store_test.go, internal/runtime/workspace.go, internal/runtime/workspace_test.go]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH gofmt -w internal/runtime/claim_store.go internal/runtime/claim_store_test.go internal/runtime/workspace.go internal/runtime/workspace_test.go && PATH=$HOME/.local/share/go1.24.0/bin:$PATH go test ./internal/runtime -run 'TestClaimStore|TestApprove|TestSupersede|TestRevoke|TestLegacy|TestCreateWorkspace' -count=1 -> ok github.com/therealtinhtute/zbrain/internal/runtime 0.006s"
  blocker: none
- timestamp: 2026-07-30T10:05:22Z
  phase: trusted-memory-vertical-slice
  wave: 2-canonical-stores
  task: "2.2 Implement immutable evidence snapshots and proof validation inputs"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: 01KYS7RQBB5NRZQ7DSCZV0V55C
  changed_surfaces: [internal/runtime/evidence.go, internal/runtime/evidence_test.go]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH gofmt -w internal/runtime/evidence_test.go && PATH=$HOME/.local/share/go1.24.0/bin:$PATH go test ./internal/runtime -run 'TestEvidence|TestSnapshot|TestEvidenceHash|TestEvidenceWorkspaceIsolation' -count=1 -> ok github.com/therealtinhtute/zbrain/internal/runtime 0.005s; first attempt failed because the tamper fixture attempted to overwrite a read-only raw snapshot, then passed after chmoding only the temp fixture"
  blocker: none
- timestamp: 2026-07-30T10:15:39Z
  phase: trusted-memory-vertical-slice
  wave: 3-index
  task: "3.1 Add index paths, FTS5 capability proof, schema, upsert, integrity, and rebuild"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: 01KYS8BJ4BHEZJD1FYTZF2F5NG
  changed_surfaces: [go.mod, go.sum, internal/runtime/paths.go, internal/runtime/index.go, internal/runtime/index_test.go, internal/runtime/index_benchmark_test.go]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local go test ./internal/runtime -run 'TestIndex|TestFTS5|TestReindex|TestIndexIntegrity|TestSearch|TestAskP95At100K' -count=1 -v -> PASS with TestAskP95At100K skipped unless opted in; PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v -> PASS, 100k claim search p95=1.073710454s samples=40"
  blocker: none
- timestamp: 2026-07-30T10:18:28Z
  phase: trusted-memory-vertical-slice
  wave: 4-trusted-query
  task: "4.1 Implement scope resolution and the trusted query response model"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: 01KYS8GPGGYWMXQ6P11ZXFK5AR
  changed_surfaces: [internal/runtime/query.go, internal/runtime/query_test.go]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local gofmt -w internal/runtime/query.go internal/runtime/query_test.go && PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local go test ./internal/runtime -run 'TestResolveScopes|TestTrustedQuery|TestPromotionCandidates|TestConflict|TestGap|TestWorkspaceIsolation' -count=1 -> ok github.com/therealtinhtute/zbrain/internal/runtime 0.109s"
  blocker: none
- timestamp: 2026-07-30T10:23:18Z
  phase: trusted-memory-vertical-slice
  wave: 5-cli
  task: "5.1 Add evidence and claim lifecycle commands with machine-readable output"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: 01KYS8SJ986GNRVF3E8E3CE21H
  changed_surfaces: [internal/cli/cli.go, internal/cli/cli_test.go, internal/runtime/assets.go, internal/runtime/assets_test.go]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local gofmt -w internal/runtime/assets.go internal/runtime/assets_test.go internal/cli/cli.go internal/cli/cli_test.go && PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local go test ./internal/cli -run 'TestRunEvidence|TestRunClaim|TestClaimValidation|TestLifecycleJSON' -count=1 -> ok github.com/therealtinhtute/zbrain/internal/cli 0.005s"
  blocker: none
- timestamp: 2026-07-30T10:25:24Z
  phase: trusted-memory-vertical-slice
  wave: 5-cli
  task: "5.2 Redesign reindex, ask, help, and the isolated CLI smoke path"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: 01KYS8XCNR0J3D1EDH32VJ1M6A
  changed_surfaces: [internal/cli/cli.go, internal/cli/cli_test.go, Makefile]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local go test ./internal/cli -run 'TestRunReindex|TestRunAsk|TestAskScopes|TestAskGap|TestAskConflict' -count=1 -> ok github.com/therealtinhtute/zbrain/internal/cli 0.097s; PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local make build && PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local make smoke -> trusted CLI smoke returned ready context JSON"
  blocker: none
- timestamp: 2026-07-31T03:55:05Z
  phase: trusted-memory-vertical-slice
  wave: 6-coherence-and-proof
  task: "6.1 Align embedded assets, current docs, and CI with the executable Go contract"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: none
  changed_surfaces: [assets, internal/runtime/assets_test.go, README.md, wiki-spec.md, AGENTS.md, CONTRIBUTING.md, .github/workflows/test.yml]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local go test ./internal/runtime -run 'TestExtractBundledAssets|TestBundledAssetsDoNotContainStaleRuntimeInstructions' -count=1 -> ok github.com/therealtinhtute/zbrain/internal/runtime 0.004s; active docs/assets stale-token scan -> no stale active tokens; git diff --check -> no output"
  blocker: none
- timestamp: 2026-07-31T03:55:05Z
  phase: trusted-memory-vertical-slice
  wave: 6-coherence-and-proof
  task: "6.2 Run final acceptance and record exact proof without changing product scope"
  task_status: DONE
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  trace_id: none
  changed_surfaces: [internal/cli/cli.go, internal/cli/cli_test.go, docs/plans/active/trusted-agent-memory.md]
  verification: "PATH=$HOME/.local/share/go1.24.0/bin:$PATH GOTOOLCHAIN=local go test ./... -> pass; make build -> pass; make smoke -> ready trusted context JSON with isolated ZBRAIN_HOME; ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v -> PASS, 100k claim search p95=1.108644747s samples=40, GOOS=linux GOARCH=amd64 CPU=20; git diff --check -> no output; git status --short -> working tree contains expected trusted-memory slice changes"
  blocker: none

## Decisions
<!-- Append-only durable entries record timestamp, phase/task, decision, and rationale. -->
- none

## Validation
<!-- Append-only durable entries record timestamp, phase, exact command/result/output, run_id, check_id, verdict, and proof_gaps. -->
- timestamp: 2026-07-31T03:56:21Z
  phase: trusted-memory-vertical-slice
  command: "check full gate: go test ./...; make build; make smoke; ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v; git diff --check"
  result: "APPROVED; full tests passed; build passed; isolated smoke returned ready trusted context JSON; 100k p95=1.108644747s samples=40; git diff --check clean; same-session full review found and fixed dirty-marker mutation issue before recording"
  run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
  check_id: 01KYV51F923TZG9QVPZNRKGE5M
  verdict: APPROVED
  proof_gaps: "same-session review; not independently verified by a separate reviewer"

## Current State and Next Action
- active_phase: none
- lifecycle_status: done
- latest_run_id: 01KYS78XDZYD8ANSW6P7J8BFPV
- latest_trace_ids: [01KYS7EVH3VBE5T0JGPSCB9E58, 01KYS7RQBB5NRZQ7DSCZV0V55C, 01KYS8BJ4BHEZJD1FYTZF2F5NG, 01KYS8GPGGYWMXQ6P11ZXFK5AR, 01KYS8SJ986GNRVF3E8E3CE21H, 01KYS8XCNR0J3D1EDH32VJ1M6A]
- latest_check_id: 01KYV51F923TZG9QVPZNRKGE5M
- latest_handoff_id: 01KYV5VCWHHPG2R7N4PS2K6N9J
- blockers: none
- open_items: none
- exact_next_action: initiative closed; optional push or PR from branch feat/trusted-memory-vertical-slice
