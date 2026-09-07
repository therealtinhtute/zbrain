---
id: 01KZ5C29XDKSTXGEG8DQ1XNYCF
type: plan
intake_id: 01KZ5C2PM78GSWC7J8AJJA8AVT
lane: high-risk
status: completed
created: 2026-08-04
updated: 2026-08-05
---

# Plan: Trusted Memory Integrity and Lifecycle Closure

## Outcome
- result: zbrain preserves the existing four-state trusted-memory model while failing closed on stale or partially valid indexes, recovering interrupted lifecycle transitions, invalidating claims whose support is no longer trustworthy, enforcing workspace boundaries, and documenting the shipped Go runtime accurately.
- success_signals:
  - `zbrain ask` rejects any workspace whose canonical trust inputs changed after the last clean rebuild or whose most recent rebuild rejected any claim.
  - Interrupted supersession cannot become queryable as two approved claims; recovery converges to one consistent canonical state before trust is restored.
  - Approval, supersession, and revocation leave structured, inspectable transition history without erasing the prior approval attestation or canonical evidence.
  - Derived and evidence-based claims are excluded when any required support is revoked, superseded, missing, tampered, cyclic, or otherwise invalid.
  - Every explicit and lower-level workspace read/write rejects traversal names and nonexistent workspaces without creating paths outside an existing workspace.
  - Current operational documentation matches `zbrain --help`, the implemented runtime layout, and the actual completed/active plan state.
  - Focused regression tests, `go test ./...`, `make build`, `make smoke`, and the existing 100k-claim p95-under-two-seconds gate pass.

## Authority and Requirements
- authority:
  - `trusted-memory-spec.md` is the current product and trust-contract authority.
  - `docs/plans/completed/trusted-agent-memory.md` is the closed vertical-slice record and must not be reopened or duplicated.
  - `internal/runtime/claim.go`, `internal/runtime/claim_store.go`, `internal/runtime/evidence.go`, `internal/runtime/index.go`, `internal/runtime/query.go`, and `internal/cli/cli.go` are implementation evidence for the bounded gaps.
  - Owner direction from 2026-08-04 approves the minimal model `evidence -> draft -> approved -> trusted context`, with only `superseded` and `revoked` as approved-claim terminal states, and requests a full executable plan.
  - Repository `CLAUDE.md` requires a minimal Go-native implementation, focused tests, isolated smoke proof, and no Bun/Node/TypeScript reintroduction.
- requirements:
  - R1 [accepted]: A successful rebuild records a deterministic manifest of canonical trust inputs, and `zbrain ask` fails closed if any covered claim Markdown or required evidence input is added, removed, or changed afterward. | source: `trusted-memory-spec.md` §§7-9
  - R2 [accepted]: If any claim is rejected during the most recent rebuild, the published workspace state remains explicitly untrusted and every subsequent `zbrain ask` fails closed, including queries that match unrelated valid claims, until a fully clean rebuild succeeds. | source: `trusted-memory-spec.md` §8
  - R3 [accepted]: Every canonical mutation, including `zbrain migrate okf`, marks the workspace index dirty before the first write; a dirty-marking failure leaves canonical files unchanged and the previous index cannot be served after a successful mutation. | source: `trusted-memory-spec.md` §§8,10 and current CLI mutation invariant
  - R4 [accepted]: Supersession is recoverable as one logical operation across all affected claim files: interruption at any write point keeps trust blocked, and recovery converges to a state where exactly one of the old or replacement claims is approved and the other has the lifecycle status required by that outcome. | source: `trusted-memory-spec.md` §§5,10 and completed plan R3
  - R5 [accepted]: Runtime transitions enforce exactly `draft -> approved -> superseded|revoked`; approval is draft-only, supersession targets approved claims through a replacement draft, and revocation is approved-only. No fifth lifecycle status is introduced. | source: `trusted-memory-spec.md` §§3,5
  - R6 [accepted]: Approval, supersession, and revocation create structured transition records containing transition kind, timestamp, actor, reason where required, related claim IDs, and the prior verification digest where applicable; historical approval metadata and the original claim body remain inspectable rather than being cleared or rewritten as free-form audit text. | source: completed plan R3 and `trusted-memory-spec.md` §§5,10
  - R7 [accepted]: Rebuild revalidates each approved claim's complete trust dependency closure, including supporting-claim status and verification digest, cycle detection, evidence existence, metadata, byte length, and SHA-256; an invalid dependency rejects every affected dependent claim with a deterministic path/reason while preserving canonical files. | source: `trusted-memory-spec.md` §§6-8
  - R8 [accepted]: Dependency invalidation is logical and fail-closed: revoking or superseding a support claim, or tampering with/removing required evidence, cannot leave a dependent claim available to `zbrain ask`; no dependent claim or evidence is physically deleted. | source: `trusted-memory-spec.md` §§5,7,9
  - R9 [accepted]: Every CLI and runtime workspace read/write boundary validates a safe workspace name and existing workspace ownership before path construction or mutation; traversal and nonexistent-workspace inputs fail without creating directories or crossing workspace roots. | source: `trusted-memory-spec.md` §§4,10
  - R10 [accepted]: Current operational docs are reconciled with the shipped runtime by correcting the index layout and plan/gap references in `trusted-memory-spec.md`, and by removing or clearly retiring absent Bun/qmd and `init|learn|ingest|update` workflows from active acceptance/release documentation. | source: `trusted-memory-spec.md` §§2,4,8 and repository `CLAUDE.md`
  - R11 [accepted]: Each behavior above has focused happy-path, failure-injection, recovery, boundary, and regression tests; release proof includes `go test ./...`, `make build`, `make smoke`, and the existing 100k-claim p95-under-two-seconds benchmark. | source: `trusted-memory-spec.md` §§10-11

## Non-goals
- NG1: Do not reopen, rewrite, or re-plan `docs/plans/completed/trusted-agent-memory.md`, and do not reimplement the already-shipped digest-verification vertical slice.
- NG2: Do not add lifecycle statuses beyond `draft`, `approved`, `superseded`, and `revoked`; do not assign trust semantics to `stale_after` in this initiative.
- NG3: Do not add LLM/provider calls, MCP, vectors or semantic retrieval, hosted sync, team authentication, background services, GUI, session transcripts, or a second runtime/language.
- NG4: Do not physically erase claims or evidence. Privacy erasure and retention policy remain separate future decisions.
- NG5: Do not build authenticated multi-actor identity. Existing local actor labels may be recorded honestly but are not presented as cryptographic identity.
- NG6: Do not expand trusted-query JSON into a full evidence/citation graph or redesign retrieval ranking.
- NG7: Do not add a new end-user lifecycle or recovery command; recovery is an internal trust-boundary behavior unless implementation evidence proves the existing command surface cannot satisfy R4.
- NG8: Do not add fsync-level power-loss durability, Git-backed history, backup, or synchronization; operation-level consistency and deterministic recovery are the bounded target.
- NG9: Do not rewrite historical analyses, changelog entries, or unrelated active plans; documentation work is limited to current operational and authority documents.

## Approach and Risks
- approach:
  - Establish one runtime-owned workspace boundary before deriving any claim, evidence, index, query, or migration path. Existing-workspace operations reject unsafe names, nonexistent roots, root symlinks, and resolved child paths outside the workspace. Canonical mutation methods own the dirty-before-write barrier so direct runtime calls cannot bypass the CLI guard.
  - Store an exact-byte trust-input manifest inside each disposable workspace SQLite index. The manifest has deterministically sorted `path`, `kind`, `byte_length`, and SHA-256 entries for all `wiki/**/*.md` claim files and every `evidence/sources/*/{source.yaml,raw}` snapshot. A single `rebuild_state` row records `clean|rejected`, invalid count, aggregate manifest digest, and rebuild time. These are index states, not claim lifecycle statuses.
  - Make rebuild publication explicit: mark dirty before scanning, build a temporary database, write indexed rows plus manifest and rebuild state, atomically publish it, and clear dirty only after publication. A completed rebuild with any rejected claim publishes `rejected`; scan/build/publication failure leaves dirty. `zbrain ask` checks dirty, database existence, pending transition state, rebuild state, and a recomputed exact manifest before reading trusted rows.
  - Add optional append-only `zbrain.transitions` entries to canonical claims with `kind`, `at`, `by`, optional `reason`, related claim IDs, and optional prior verification digest. Existing claims without transition entries remain readable and are never backfilled automatically. New approvals include their transition entry before computing the verification digest.
  - Make multi-file supersession recoverable with one operational journal at `workspaces/<workspace>/.zbrain/pending-transition.json`. The journal is written atomically before the first claim rename and stores the operation ID plus every target path, expected preimage hash, target hash, and exact target bytes. Recovery only rolls forward when each current file matches its preimage or target hash; any mismatch preserves the journal, keeps the workspace untrusted, and refuses to overwrite. Mutations and `reindex` recover pending work; `ask` remains read-only and fails closed while a journal exists.
  - Validate approved trust dependencies once at approval and rebuild boundaries with a deterministic, memoized depth-first traversal. Supporting claims must exist, remain approved, match their verification digest, and recursively validate; evidence metadata and raw bytes must exist and match recorded size and SHA-256. Cycles invalidate the cycle and all dependents. Invalid claims are reported and preserved, never repaired, deleted, cascaded to another status, or validated repeatedly during query.
  - Finish by reconciling only current authority and operational documentation with the implemented Go runtime, then run focused tests, full tests, build, isolated smoke, and the existing 100k-claim p95 gate.
- constraints:
  - Preserve exactly `draft`, `approved`, `superseded`, and `revoked`; `clean|rejected` rebuild state and pending-operation journal state are operational metadata, not additional claim statuses.
  - Markdown claims and immutable evidence snapshots remain canonical. SQLite, manifests, rebuild state, dirty markers, and pending-operation journals are derived or operational and contain no unique trusted content.
  - Keep indexes at `$ZBRAIN_HOME/indexes/<workspace>.sqlite`; correct the authority spec instead of moving the implemented index layout.
  - Keep the current command surface and versioned JSON contracts. No recovery command, interactive prompt, daemon, external service, or second runtime is added.
  - Existing approved claims without transition history must keep parsing and digest validity. Empty transition metadata must not alter their canonical rendering.
  - Preserve the current single-writer CLI assumption. Competing or unexpected writes are detected through exclusive pending-operation creation and preimage mismatch, then fail closed rather than being merged or overwritten.
  - Use the Go standard library and existing YAML/SQLite dependencies; no new third-party runtime dependency is authorized.
- dependencies:
  - Existing `modernc.org/sqlite` through `database/sql` for FTS5, manifest rows, and rebuild state in the disposable index.
  - Existing YAML parsing/rendering and SHA-256 evidence/digest primitives.
  - Existing `ZBRAIN_HOME` isolation, deterministic 100k corpus support, and `make test|build|smoke` gates.
- rejected_alternatives:
  - Reopen or amend `docs/plans/completed/trusted-agent-memory.md`: rejected because that vertical slice is closed; this plan owns only proven follow-up deltas.
  - Keep only the dirty marker or compare aggregate mtime/size: rejected because out-of-band exact-content changes and completed rejected rebuilds remain indistinguishable from trusted state.
  - Store rebuild state in a separate JSON file: rejected because it can drift independently from the exact SQLite publication it describes.
  - Make SQLite canonical or use a SQLite transaction for supersession: rejected because canonical truth spans Markdown files and a database transaction cannot make those file renames operation-atomic.
  - Add a fifth `pending` or `superseding` claim status: rejected because operational recovery is orthogonal to the locked four-state lifecycle.
  - Clear verification metadata or append revocation prose to the claim body: rejected because it destroys or mixes audit history with claim content.
  - Validate dependency closures during every query or auto-revoke dependent claims: rejected because trust belongs at approval/index boundaries and canonical lifecycle changes require explicit owner action.
  - Add a public repair/recovery command: rejected because idempotent recovery can run at existing mutation and `reindex` boundaries while `ask` fails closed.
- risks:
  - risk: Exact manifest recomputation over 100,000 claims may push trusted-query p95 above two seconds.
    mitigation: Use deterministic directory traversal, streaming SHA-256, bounded worker concurrency, reused buffers, and one aggregate comparison without reopening parsed claims or SQLite rows.
    recovery: Profile only the manifest scan/hash path and optimize within exact-byte semantics. Keep the workspace blocked and stop the phase rather than weakening freshness to mtime-only checks or shipping above the gate.
  - risk: A crash or outside edit can leave a pending supersession journal whose preimages no longer match canonical files.
    mitigation: Persist all preimage and target hashes before the first rename, create only one pending operation per workspace, and make recovery idempotent without heuristic merges.
    recovery: Preserve the journal and canonical files, keep dirty/rejected trust state, report the exact conflicting target, and stop for operator reconciliation; never overwrite the mismatch or silently remove the journal.
  - risk: Transition metadata changes canonical digest inputs and can invalidate already-approved claims if empty fields are rendered or old files are normalized.
    mitigation: Omit empty transition collections, accept legacy claims without them, append the approval transition before digest calculation for new approvals, and add old-fixture round-trip/digest regression tests.
    recovery: Revert the renderer/parser change while retaining the journal design; do not backfill or rewrite existing approved files to make tests pass.
  - risk: Recursive support graphs can contain deep chains, duplicate edges, missing nodes, or cycles that produce nondeterministic rejection reports.
    mitigation: Sort IDs and paths, use `visiting` plus memoized terminal results, cache evidence verification by ID, and return one stable dependency path/reason per affected root.
    recovery: Keep canonical files unchanged and the index rejected; fix the deterministic validator before allowing a clean rebuild.
  - risk: Fail-closed rejected rebuilds can block unrelated valid queries until every approved invalid claim is explicitly repaired through the lifecycle.
    mitigation: Return precise relative paths, claim/evidence IDs, dependency paths, and reasons from `reindex`, while preserving drafts and all canonical evidence for repair.
    recovery: Use normal supersession/revocation or evidence restoration, then run a clean `zbrain reindex`; do not downgrade the rejection to a query gap.

## Phases and Verification
<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. Each planned phase records phase_slug, story_id, status, goal, depends_on, waves, tasks, and checks. -->
- planning_status: planned
- phases:
  - phase_slug: workspace-boundary-mutation-barrier
    story_id: 01KZ5CNHBWQ7XWCRNGC6G9D5AY
    status: done
    goal: Reject unsafe or nonexistent workspace paths and mark indexes dirty before every canonical mutation.
    depends_on: none
    allowed_surfaces:
      - Add `internal/runtime/workspace_boundary.go` and focused tests.
      - Modify `internal/runtime/paths.go`, workspace/config resolution, claim/evidence stores, index/query entry points, and their tests only where boundary enforcement requires it.
      - Modify `internal/cli/cli.go` and CLI tests for explicit workspace validation and migration dirty-before-write ordering.
    avoided_surfaces:
      - Claim lifecycle schema, transition history, and supersession semantics.
      - SQLite schema, trust-input manifest, rebuild-state semantics, ranking, or query JSON redesign.
      - Runtime assets, authority docs, completed plans, and user data outside isolated test homes.
    waves:
      - wave: 1-boundary-contract
        goal: Define one safe existing-workspace and child-path contract before changing callers.
        depends_on: none
        tasks:
          - task: 1.1 Add the central workspace boundary resolver.
            requirements: [R9]
            depends_on: none
            touched_surfaces:
              - add `internal/runtime/workspace_boundary.go`
              - add `internal/runtime/workspace_boundary_test.go`
              - minimally modify `internal/runtime/paths.go` and `internal/runtime/paths_test.go`
            avoided_surfaces:
              - workspace creation semantics for a new safe name
              - canonical claim/evidence writes
              - index database contents
            expected_outputs:
              - `ValidateWorkspace` rejects empty, uppercase/unsafe, absolute, traversal, nonexistent, and root-symlink workspaces before any derived path is built.
              - A child resolver accepts only relative paths whose resolved existing parent and target remain under the validated workspace root; absolute paths, `..`, and symlink escapes fail without cleaning attacker input.
              - Index paths remain root-level but are derived only after the owning workspace validates.
            checks:
              - `go test ./internal/runtime -run 'TestValidateWorkspace|TestResolveWorkspacePath|TestSafeWorkspaceName|TestPaths' -count=1`
              - Prove every rejected case leaves the isolated `ZBRAIN_HOME` tree byte-for-byte/path-for-path unchanged.
            stop_conditions:
              - Any traversal, absolute path, root symlink, or child symlink resolves outside `workspaces/<workspace>`.
              - A nonexistent workspace is created by validation or a read path.
      - wave: 2-call-site-barriers
        goal: Apply the stable boundary and dirty-before-write invariant across independent runtime and CLI owners.
        depends_on: wave 1-boundary-contract
        tasks:
          - task: 2.1 Harden ClaimStore and EvidenceStore reads and writes.
            requirements: [R3, R9]
            depends_on: task 1.1
            touched_surfaces:
              - modify `internal/runtime/claim_store.go`
              - modify `internal/runtime/claim_store_test.go`
              - modify `internal/runtime/evidence.go`
              - modify `internal/runtime/evidence_test.go`
            avoided_surfaces:
              - lifecycle transition rules beyond existing behavior
              - evidence network capture or deletion
              - SQLite schema
            expected_outputs:
              - Every claim/evidence lookup, scan, approval reference read, and mutation validates an existing workspace and safe child path.
              - Runtime-owned canonical mutation paths mark the workspace dirty before directory creation, temp-file creation, rename, chmod, or metadata/raw writes.
              - Dirty-mark failure changes no canonical bytes and creates no claim/evidence path.
            checks:
              - `go test ./internal/runtime -run 'TestClaimStore.*Workspace|TestClaimStore.*Dirty|TestEvidence.*Workspace|TestEvidence.*Dirty|TestEvidenceWorkspaceIsolation' -count=1`
              - Hash isolated claim/evidence trees before injected dirty-mark failures and compare them afterward.
            stop_conditions:
              - A direct store call bypasses validation or mutates before the dirty marker succeeds.
              - Evidence or claim lookup can cross a workspace through an ID/path combination.
          - task: 2.2 Harden index, query, current-workspace, and include boundaries.
            requirements: [R9]
            depends_on: task 1.1
            touched_surfaces:
              - modify `internal/runtime/index.go`
              - modify `internal/runtime/index_test.go`
              - modify `internal/runtime/query.go`
              - modify `internal/runtime/query_test.go`
              - minimally modify workspace/config resolution and focused tests
            avoided_surfaces:
              - manifest or rebuild-state tables
              - query ranking, status vocabulary, or response fields
              - implicit cross-workspace search
            expected_outputs:
              - Index create/read/check/search/rebuild validates workspace ownership before deriving `$ZBRAIN_HOME/indexes/<workspace>.*`.
              - Current, explicit, and included query scopes reject unsafe or nonexistent workspaces and remain isolated.
              - Invalid config scope fails closed instead of deriving an index path or searching another workspace.
            checks:
              - `go test ./internal/runtime -run 'TestIndex.*Workspace|TestResolveScopes|TestTrustedQuery.*Workspace|TestWorkspaceIsolation|TestCurrentWorkspace' -count=1`
              - Prove an omitted or invalid secondary workspace never affects IDs, ranking, gaps, or promotion candidates.
            stop_conditions:
              - Any runtime index/query API derives a path from an unvalidated workspace.
              - An invalid include/current scope falls back to another workspace.
          - task: 2.3 Move migration and explicit CLI workspace guards ahead of mutation.
            requirements: [R3, R9]
            depends_on: task 1.1
            touched_surfaces:
              - modify `internal/cli/cli.go`
              - modify `internal/cli/cli_test.go`
            avoided_surfaces:
              - command grammar or new flags
              - migration content/schema behavior beyond ordering and ownership
              - lifecycle transition history
            expected_outputs:
              - `migrate okf` validates the explicit/current workspace and marks dirty before rewriting the first legacy claim.
              - Injected dirty-mark failure leaves all migration inputs unchanged and the old index cannot appear fresh after any successful mutation.
              - `evidence add` and every claim mutation reject traversal and nonexistent explicit workspaces without creating directories or external dirty markers.
            checks:
              - `go test ./internal/cli -run 'TestRunMigrate.*Dirty|TestRunEvidence.*Workspace|TestRunClaim.*Workspace|Test.*Traversal|Test.*NonexistentWorkspace' -count=1`
              - Reproduce `--workspace ../../outside/pwn` under an isolated home and prove no path appears outside that home.
            stop_conditions:
              - Migration writes a claim before dirty marking succeeds.
              - Any CLI mutation creates a missing workspace or a dirty marker outside the validated index root.
      - wave: 3-boundary-gate
        goal: Prove the boundary and mutation barrier as one independently mergeable security unit.
        depends_on: wave 2-call-site-barriers
        tasks:
          - task: 3.1 Run focused and full boundary regression proof.
            requirements: [R3, R9, R11]
            depends_on: [task 2.1, task 2.2, task 2.3]
            touched_surfaces:
              - focused runtime and CLI test fixtures only if a missing boundary case is exposed
            avoided_surfaces:
              - lifecycle, manifest, dependency, retrieval, docs, and assets
            expected_outputs:
              - All direct-runtime and CLI unsafe/missing/symlink cases fail before path creation or canonical mutation.
              - Existing safe setup, workspace create/current, claim, evidence, reindex, and ask tests remain green.
            checks:
              - `go test ./internal/runtime -run 'Test.*Workspace|Test.*Boundary|Test.*Isolation|Test.*Path|Test.*Dirty' -count=1`
              - `go test ./internal/cli -run 'TestRun.*(Workspace|Evidence|Claim|Migrate)|Test.*Dirty|Test.*Traversal' -count=1`
              - `go test ./...`
              - `git diff --check`
            stop_conditions:
              - Any known unsafe reproduction succeeds or any safe existing command regresses.
    phase_stop_conditions:
      - A workspace boundary remains enforced only at the CLI while a runtime store can bypass it.
      - Dirty-before-write cannot be proven for every canonical mutation path.
    escalation_and_recovery:
      - Leave canonical files and dirty markers exactly as produced by the failing test, isolate the owning boundary call site, and fix it before starting the index-trust phase. Do not add path sanitization, implicit workspace creation, or a compatibility bypass.

  - phase_slug: manifest-rejected-index-state
    story_id: 01KZ5CNNB3P760X8DYTDC48P4V
    status: done
    goal: Make trusted queries fail closed on changed trust inputs and rejected or incomplete rebuilds.
    depends_on: workspace-boundary-mutation-barrier
    allowed_surfaces:
      - Add `internal/runtime/manifest.go`, `internal/runtime/manifest_test.go`, and a small index-state helper/test file if separation keeps `index.go` minimal.
      - Modify `internal/runtime/index.go`, `internal/runtime/query.go`, `internal/cli/cli.go`, focused tests, and existing benchmark support.
      - Extend the disposable SQLite schema only with manifest and rebuild-state metadata.
    avoided_surfaces:
      - Claim statuses, transition schema/journal, dependency traversal, ranking, and evidence indexing.
      - New commands, canonical-file rewrites during rebuild, docs, assets, and completed plans.
    waves:
      - wave: 1-manifest-state-primitives
        goal: Build exact deterministic manifest and SQLite state primitives behind focused tests.
        depends_on: none
        tasks:
          - task: 1.1 Implement deterministic trust-input manifest scanning.
            requirements: [R1]
            depends_on: none
            touched_surfaces:
              - add `internal/runtime/manifest.go`
              - add `internal/runtime/manifest_test.go`
            avoided_surfaces:
              - claim parsing and lifecycle validation
              - SQLite publication
              - query search/ranking
            expected_outputs:
              - Sorted entries for every claim Markdown file under the four wiki tiers and every evidence `source.yaml`/`raw` snapshot under `evidence/sources/`.
              - Each entry records slash-normalized workspace-relative path, kind, byte length, and SHA-256; an aggregate digest covers the deterministic encoded entries.
              - Add/remove/content-change, same-size content replacement, and evidence metadata/raw mutation all change the aggregate digest; indexes, dirty markers, journals, and evidence analysis/QA/applied/archive files are excluded.
            checks:
              - `go test ./internal/runtime -run 'TestTrustInputManifest|TestManifestDetects(Add|Remove|Change)|TestManifestEvidenceInputs|TestManifestDeterministic' -count=1`
              - Build the same manifest twice with shuffled directory creation order and compare exact entries/digest.
            stop_conditions:
              - A covered same-size byte change preserves the digest.
              - Directory enumeration order changes manifest output.
              - Derived/operational files enter the trust manifest.
          - task: 1.2 Add disposable rebuild-state and manifest tables.
            requirements: [R1, R2]
            depends_on: none
            touched_surfaces:
              - add `internal/runtime/index_state.go`
              - add `internal/runtime/index_state_test.go`
              - minimally extend index schema creation in `internal/runtime/index.go`
            avoided_surfaces:
              - canonical Markdown/evidence files
              - FTS ranking columns and result model
              - CLI grammar
            expected_outputs:
              - SQLite `trust_inputs(path, kind, byte_length, sha256)` rows and one `rebuild_state(status, invalid_count, manifest_digest, rebuilt_at)` row are created transactionally in the temporary index.
              - Only `clean` and `rejected` rebuild values are accepted; missing/unknown state is untrusted.
              - Reading state from a missing, corrupt, or old-schema index returns an explicit fail-closed error.
            checks:
              - `go test ./internal/runtime -run 'TestIndexRebuildState|TestIndexTrustInputs|TestIndexStateMissing|TestIndexStateRejected' -count=1`
              - Open a temporary index and inspect that state and manifest rows are committed with the same publication transaction.
            stop_conditions:
              - Rebuild state can be updated independently after index publication.
              - Missing or unknown state is interpreted as clean.
      - wave: 2-rebuild-publication
        goal: Publish rows, exact manifest, and clean/rejected outcome as one disposable index generation.
        depends_on: wave 1-manifest-state-primitives
        tasks:
          - task: 2.1 Integrate manifest and rejection state into IndexStore.Rebuild.
            requirements: [R1, R2, R3]
            depends_on: [task 1.1, task 1.2]
            touched_surfaces:
              - modify `internal/runtime/index.go`
              - modify `internal/runtime/index_test.go`
              - minimally extend reindex result types consumed by CLI
            avoided_surfaces:
              - canonical repair/backfill
              - dependency-closure rules not yet introduced
              - query response ranking and lifecycle status vocabulary
            expected_outputs:
              - Rebuild marks dirty before scan, computes one trust-input manifest, builds a temporary DB, writes valid index rows plus manifest/state, atomically renames the DB, and clears dirty only after publication.
              - Zero rejected claims publishes `clean`; any parse/digest/legacy rejection publishes `rejected` with the exact invalid count and existing path/reason details.
              - Scan, SQLite, integrity, or rename failure leaves dirty and keeps the prior DB unusable; rebuild never mutates canonical files.
            checks:
              - `go test ./internal/runtime -run 'TestRebuildManifest|TestRebuildCleanState|TestRebuildRejectedState|TestRebuildFailureLeavesDirty|TestRebuildDoesNotMutateCanonical' -count=1`
              - Inject failure before and after temporary-DB publication and prove no state can be observed as both new and clean prematurely.
            stop_conditions:
              - A rejected rebuild clears all evidence that rejection occurred.
              - A failed rebuild clears dirty or modifies canonical inputs.
              - Manifest rows describe a different scan generation from indexed rows.
      - wave: 3-query-trust-gate
        goal: Refuse every stale, rejected, dirty, missing, or malformed index before trusted search.
        depends_on: wave 2-rebuild-publication
        tasks:
          - task: 3.1 Enforce exact freshness and rebuild state in CheckFresh and TrustedQuery.
            requirements: [R1, R2]
            depends_on: task 2.1
            touched_surfaces:
              - modify `internal/runtime/index.go`
              - modify `internal/runtime/query.go`
              - modify `internal/runtime/index_test.go`
              - modify `internal/runtime/query_test.go`
            avoided_surfaces:
              - query ranking, scope merge, promotion-candidate behavior, or JSON schema redesign
              - query-time claim parsing or dependency traversal
              - canonical writes
            expected_outputs:
              - Freshness validation orders checks as validated workspace, dirty marker, DB existence/open/state, current exact manifest, then trusted search.
              - Any added, removed, or changed covered claim/evidence input returns an explicit stale error before FTS results are read.
              - A `rejected` rebuild blocks every query, including one matching an unrelated valid claim; missing/unknown state never degrades to `gap`.
            checks:
              - `go test ./internal/runtime -run 'TestCheckFresh|TestTrustedQuery.*(Stale|Rejected|Dirty|Missing)|TestOutsideEdit|TestUnrelatedValidClaimRejected' -count=1`
              - Rebuild clean, edit one covered file without CLI, query unrelated valid content, and prove no trusted claim is returned.
            stop_conditions:
              - Any untrusted index state reaches FTS search.
              - A rejected/stale workspace returns `ready` or `gap` instead of an explicit error.
          - task: 3.2 Expose rebuild trust outcome and prove the 100k gate.
            requirements: [R1, R2, R11]
            depends_on: task 3.1
            touched_surfaces:
              - modify `internal/cli/cli.go`
              - modify `internal/cli/cli_test.go`
              - modify existing benchmark helpers only if exact manifest scanning requires bounded optimization
            avoided_surfaces:
              - new commands or response schema version
              - weaker mtime-only freshness shortcuts
              - retrieval ranking changes unrelated to freshness cost
            expected_outputs:
              - Existing `reindex` JSON reports clean/rejected outcome, invalid count, and manifest digest without hiding existing path/reason diagnostics.
              - Existing `ask` error output distinguishes dirty, missing, stale, rejected, and pending operational state without returning trusted rows.
              - Exact freshness plus indexed query remains below two seconds p95 at 100,000 claims.
            checks:
              - `go test ./internal/cli -run 'TestRunReindex.*(Clean|Rejected|Manifest)|TestRunAsk.*(Stale|Rejected|Dirty|Missing)' -count=1`
              - `ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v`
              - `go test ./...`
              - `git diff --check`
            stop_conditions:
              - JSON reports success while the workspace is rejected or stale.
              - p95 exceeds two seconds after bounded profiling of manifest traversal/hash only.
    phase_stop_conditions:
      - Exact outside-edit detection or rejected-rebuild blocking is incomplete for claims or evidence.
      - Performance can pass only by weakening exact-byte trust semantics.
    escalation_and_recovery:
      - Preserve canonical inputs, leave dirty/rejected state visible, and rerun `zbrain reindex` only after the owning invalid input or implementation fault is corrected. If scale fails, profile and optimize manifest I/O without changing the trust contract.

  - phase_slug: lifecycle-transition-recovery
    story_id: 01KZ5CNS37J8DN068V7Y1JWJPZ
    status: done
    goal: Preserve structured lifecycle history and recover interrupted supersession without adding statuses.
    depends_on: manifest-rejected-index-state
    allowed_surfaces:
      - Modify claim parse/render/digest and ClaimStore lifecycle behavior with focused tests.
      - Add `internal/runtime/transition.go` and tests for journal persistence, target application, and recovery.
      - Minimally modify index/query/CLI entry points to recover or block pending transitions through existing commands.
    avoided_surfaces:
      - New claim statuses, public commands, physical deletion, dependency graph validation, ranking, docs, and assets.
      - Automatic backfill of existing approved claims or free-form audit prose in claim bodies.
    waves:
      - wave: 1-transition-contract
        goal: Add backward-compatible structured lifecycle history before changing mutation behavior.
        depends_on: none
        tasks:
          - task: 1.1 Add typed transition parse/render and digest rules.
            requirements: [R5, R6]
            depends_on: none
            touched_surfaces:
              - modify `internal/runtime/claim.go`
              - modify `internal/runtime/claim_test.go`
            avoided_surfaces:
              - filesystem mutation and journal behavior
              - dependency validation
              - CLI grammar
            expected_outputs:
              - Optional ordered `zbrain.transitions` entries support `approve`, `supersede`, and `revoke` with RFC3339 `at`, non-empty `by`, optional reason, stable related claim IDs, and optional prior verification digest.
              - Empty transitions are omitted so existing approved fixtures render and verify unchanged.
              - New approval transitions are part of canonical digest input; `verified.at`, `verified.by`, and `verified.digest` retain their current digest-exclusion rules.
            checks:
              - `go test ./internal/runtime -run 'TestClaimTransition|TestClaimRoundTrip|TestClaimVerificationDigest|TestLegacyApprovedClaimWithoutTransitions' -count=1`
              - Parse/render existing approved fixtures and compare their digest and body before/after the schema addition.
            stop_conditions:
              - Empty transition support changes existing approved canonical output or digest.
              - A transition can carry an unknown kind, invalid time, unsafe related ID, or empty actor.
      - wave: 2-recovery-primitives
        goal: Build journal recovery and lifecycle-graph proof independently behind the stable schema.
        depends_on: wave 1-transition-contract
        tasks:
          - task: 2.1 Implement the pending-transition journal and idempotent target recovery.
            requirements: [R4, R6]
            depends_on: task 1.1
            touched_surfaces:
              - add `internal/runtime/transition.go`
              - add `internal/runtime/transition_test.go`
              - minimally extend runtime path helpers for `workspaces/<workspace>/.zbrain/pending-transition.json`
            avoided_surfaces:
              - claim-store lifecycle orchestration
              - public commands
              - SQLite as canonical state
            expected_outputs:
              - An atomically written journal contains operation ID/kind/workspace and sorted target records with safe relative path, expected preimage SHA-256, target SHA-256, and exact target bytes.
              - Recovery skips exact targets, applies exact targets only over matching preimages through temp+rename, rejects unexpected hashes, and removes the journal only after every target matches.
              - Existing pending journal creation is exclusive; malformed, cross-workspace, duplicate-target, or path-escaping journals fail closed without writes.
            checks:
              - `go test ./internal/runtime -run 'TestTransitionJournal|TestTransitionRecovery|TestTransitionPreimageMismatch|TestTransitionJournalPath|TestTransitionRecoveryIdempotent' -count=1`
              - Inject interruption before and after each target rename and compare the recovered final hashes with the journal target hashes.
            stop_conditions:
              - Recovery overwrites an unexpected preimage, removes an incomplete journal, or accepts an unsafe target path.
              - Two pending operations can occupy one workspace.
          - task: 2.2 Lock the four-state transition graph and historical-preservation cases.
            requirements: [R5, R6]
            depends_on: task 1.1
            touched_surfaces:
              - extend `internal/runtime/claim_store_test.go`
              - extend `internal/cli/cli_test.go` only for existing lifecycle commands
            avoided_surfaces:
              - journal implementation details
              - dependency closure and index freshness
              - new cancellation/recovery command semantics
            expected_outputs:
              - Tests require draft-only approval, approved-only supersede target, approved-only revocation, and rejection of draft/superseded/revoked invalid transitions.
              - Tests preserve original body plus prior verification actor/time/digest on supersede/revoke and require structured reason/related IDs instead of appended body text.
              - No test or fixture introduces a fifth status.
            checks:
              - `go test ./internal/runtime -run 'TestApproveTransitionGraph|TestSupersedeTransitionGraph|TestRevokeTransitionGraph|TestLifecycleHistoryPreserved' -count=1`
              - `go test ./internal/cli -run 'TestRunClaim.*(Approve|Supersede|Revoke|InvalidTransition)' -count=1`
            stop_conditions:
              - `draft -> revoked`, `superseded -> revoked`, or superseding a non-approved claim succeeds.
              - Body or prior verification fields are cleared/rewritten to record history.
      - wave: 3-lifecycle-integration
        goal: Apply auditable single-file transitions and recoverable multi-file supersession through existing trust boundaries.
        depends_on: wave 2-recovery-primitives
        tasks:
          - task: 3.1 Integrate structured approval, supersession, and revocation in ClaimStore.
            requirements: [R4, R5, R6]
            depends_on: [task 2.1, task 2.2]
            touched_surfaces:
              - modify `internal/runtime/claim_store.go`
              - modify `internal/runtime/claim_store_test.go`
              - minimally modify `internal/cli/cli.go` and `internal/cli/cli_test.go`
            avoided_surfaces:
              - command grammar and JSON schema version
              - dependency cascade, physical deletion, and query ranking
              - automatic legacy transition backfill
            expected_outputs:
              - Ordinary approval appends `approve` before computing the digest. Revocation appends `revoke` with reason and prior digest while retaining body and verification metadata.
              - Superseding approval validates the replacement and every unique approved old ID before mutation, renders all final bytes, marks dirty, writes the journal, and applies all targets idempotently.
              - The replacement ends approved with related old IDs; every old approved target ends superseded with the replacement ID and prior digest; duplicate transitions are never appended during recovery.
            checks:
              - `go test ./internal/runtime -run 'TestClaimStore.*(Approve|Supersede|Revoke|Transition)|TestSupersessionFailureInjection|TestMultipleSupersededClaims' -count=1`
              - Inspect rendered replacement/old/revoked YAML and prove original bodies plus prior approval attestations remain inspectable.
            stop_conditions:
              - Any canonical target changes before dirty and journal persistence succeed.
              - A crash point can recover to two approved claims, zero approved claims, or duplicate transition entries.
          - task: 3.2 Integrate recovery and blocking at existing command boundaries.
            requirements: [R4]
            depends_on: task 3.1
            touched_surfaces:
              - modify lifecycle mutation entry points in `internal/runtime/claim_store.go`
              - modify `internal/runtime/index.go`
              - modify `internal/runtime/query.go`
              - extend focused runtime and CLI recovery tests
            avoided_surfaces:
              - a new recovery command
              - `ask` mutating canonical files
              - clearing dirty without a clean reindex
            expected_outputs:
              - Claim mutations and `reindex` attempt idempotent recovery before new work; a conflicting journal blocks them with the exact target/reason.
              - `ask` detects a pending journal and returns an explicit error without applying writes or reading trusted rows.
              - Successful recovery leaves the index dirty; only the existing clean `reindex` path restores queryability.
            checks:
              - `go test ./internal/runtime -run 'TestRebuildRecoversPendingTransition|TestTrustedQueryBlocksPendingTransition|TestMutationRecoversPendingTransition|TestRecoveryLeavesDirty' -count=1`
              - `go test ./internal/cli -run 'TestRunReindex.*Recovery|TestRunAsk.*PendingTransition|TestRunClaim.*Recovery' -count=1`
              - `go test ./...`
              - `git diff --check`
            stop_conditions:
              - `ask` applies recovery writes or searches while a journal exists.
              - Recovery clears dirty or trusts a workspace before a clean rebuilt manifest is published.
    phase_stop_conditions:
      - Structured history loses the previously approved representation or multi-file failure cannot deterministically recover.
      - Correctness requires a fifth claim status or user-facing repair command.
    escalation_and_recovery:
      - Preserve the pending journal, dirty marker, and canonical files; report the exact mismatch and stop. Resume only through idempotent internal recovery at mutation/reindex, followed by a clean reindex. Never guess, erase history, or edit the completed plan.

  - phase_slug: recursive-trust-dependency-validation
    story_id: 01KZ5CNXY7W0ZVD4ZB62T8TKFZ
    status: done
    goal: Reject approved claims whose support closure or evidence is invalid while preserving canonical history.
    depends_on: lifecycle-transition-recovery
    allowed_surfaces:
      - Add one runtime trust-dependency validator and focused graph/evidence tests.
      - Modify ClaimStore approval, evidence verification, index rebuild classification, and focused query/CLI tests.
      - Reuse manifest/rebuild-state behavior from the prior phase for fail-closed publication.
    avoided_surfaces:
      - Query-time graph traversal, semantic inference, auto-repair, auto-revoke, physical deletion, ranking, docs, and new commands.
      - `stale_after` trust semantics or authenticated reviewer identity.
    waves:
      - wave: 1-validator-primitives
        goal: Define deterministic support-graph and evidence validity primitives before lifecycle/index integration.
        depends_on: none
        tasks:
          - task: 1.1 Implement deterministic recursive supporting-claim validation.
            requirements: [R7, R8]
            depends_on: none
            touched_surfaces:
              - add `internal/runtime/trust_validation.go`
              - add `internal/runtime/trust_validation_test.go`
            avoided_surfaces:
              - evidence byte verification implementation
              - ClaimStore writes and SQLite rebuild
              - semantic/body conflict heuristics
            expected_outputs:
              - A sorted, memoized DFS validates every supporting ID through `unvisited`, `visiting`, and terminal states.
              - Each support must exist, parse, remain approved, match its verification digest, and recursively validate.
              - Missing nodes, duplicate/unsafe IDs, invalid digest, non-approved status, and cycles produce deterministic root-to-failure ID/path reasons; cycle members and dependents are invalid.
            checks:
              - `go test ./internal/runtime -run 'TestTrustValidation.*(Chain|Missing|Status|Digest|Cycle|Deterministic|Memoized)' -count=1`
              - Run the same cyclic/deep graph with shuffled file creation order and compare exact rejection paths/reasons.
            stop_conditions:
              - Traversal order changes results, a cycle recurses indefinitely, or a deep invalid support is accepted.
              - The validator mutates claim files or lifecycle status.
          - task: 1.2 Harden complete evidence metadata/raw verification with per-run caching.
            requirements: [R7, R8]
            depends_on: none
            touched_surfaces:
              - modify `internal/runtime/evidence.go`
              - modify `internal/runtime/evidence_test.go`
            avoided_surfaces:
              - evidence capture/network behavior
              - deletion or retention policy
              - query result provenance expansion
            expected_outputs:
              - Verification requires safe evidence ID/path, parseable metadata, valid capture time/media fields, existing raw bytes, exact recorded byte length, and exact SHA-256.
              - One validation run caches a stable success/failure per evidence ID without caching across rebuilds.
              - Missing metadata/raw, malformed metadata, size mismatch, and hash mismatch return deterministic ID/path/reason errors.
            checks:
              - `go test ./internal/runtime -run 'TestEvidenceVerify.*(Metadata|Missing|Size|Hash|Workspace|Cache)' -count=1`
              - Tamper metadata and raw bytes separately after approval and prove each verification fails without rewriting or chmod repair.
            stop_conditions:
              - Evidence verification trusts metadata without raw bytes or accepts size/hash mismatch.
              - Failure triggers evidence repair, deletion, or cross-workspace lookup.
      - wave: 2-approval-rebuild-integration
        goal: Apply the complete trust closure before approval writes and before index publication.
        depends_on: wave 1-validator-primitives
        tasks:
          - task: 2.1 Validate prospective approved claims before any approval mutation.
            requirements: [R7, R8]
            depends_on: [task 1.1, task 1.2]
            touched_surfaces:
              - modify `internal/runtime/claim_store.go`
              - modify `internal/runtime/claim_store_test.go`
            avoided_surfaces:
              - draft creation/promotion-candidate availability
              - automatic support rewriting or status cascade
              - index/search code
            expected_outputs:
              - Owner claims keep current owner confirmation rules; evidence/derived candidates validate every referenced evidence item and the complete approved support closure before rendering approved bytes.
              - Approval failure writes no transition, verification metadata, journal, or canonical target and leaves the index dirty only if the mutation barrier had already engaged.
              - A prospective cycle or invalid support is rejected with the full deterministic dependency path.
            checks:
              - `go test ./internal/runtime -run 'TestClaimStoreApprove.*(DeepSupport|InvalidDigest|Revoked|Superseded|MissingEvidence|TamperedEvidence|Cycle|NoWrite)' -count=1`
              - Hash the draft before failed approval and prove it remains unchanged.
            stop_conditions:
              - Approval writes trusted state before complete dependency validation succeeds.
              - A derived claim can approve against a draft, revoked, superseded, tampered, missing, or cyclic support.
          - task: 2.2 Validate every approved dependency closure during rebuild.
            requirements: [R2, R7, R8]
            depends_on: [task 1.1, task 1.2]
            touched_surfaces:
              - modify `internal/runtime/index.go`
              - modify `internal/runtime/index_test.go`
              - minimally extend rebuild rejection details consumed by CLI
            avoided_surfaces:
              - query-time validation
              - canonical mutation or dependent status changes
              - draft exclusion solely for incomplete approval proof
            expected_outputs:
              - Rebuild parses one workspace view, validates each approved root with memoized support/evidence results, excludes every invalid approved root/dependent, and publishes `rejected` through the existing state gate.
              - Structurally valid drafts remain searchable only as promotion candidates; approved invalid claims never enter FTS trusted rows.
              - Rejection output contains stable root path/ID, dependency path, and terminal reason while all canonical hashes remain unchanged.
            checks:
              - `go test ./internal/runtime -run 'TestRebuild.*(Dependency|Derived|Evidence|Cycle|RejectedState|CanonicalUnchanged)' -count=1`
              - Revoke/supersede a deep support, reindex, and inspect that every dependent is rejected with deterministic paths and no file rewrite.
            stop_conditions:
              - An invalid dependent is indexed, or rebuild mutates/deletes claims/evidence to resolve it.
              - Rejected dependency state can be served as a partial trusted index.
      - wave: 3-dependency-gate
        goal: Prove post-approval invalidation, repair, and query blocking end to end.
        depends_on: wave 2-approval-rebuild-integration
        tasks:
          - task: 3.1 Run lifecycle/evidence invalidation and clean-rebuild recovery scenarios.
            requirements: [R7, R8, R11]
            depends_on: [task 2.1, task 2.2]
            touched_surfaces:
              - extend `internal/runtime/query_test.go`
              - extend `internal/cli/cli_test.go`
              - focused fixtures only
            avoided_surfaces:
              - new CLI repair flow
              - physical erasure, semantic search, and query JSON redesign
              - docs and assets
            expected_outputs:
              - Revoked/superseded/tampered/missing support and tampered/missing evidence make `reindex` rejected and make every `ask` fail closed, including unrelated matches.
              - Normal explicit supersession/revocation/evidence correction followed by a fully clean reindex restores queryability without hidden cascade mutation.
              - No rejected scenario changes canonical hashes except the explicit lifecycle operation under test.
            checks:
              - `go test ./internal/runtime -run 'TestTrustedQuery.*Dependency|TestDependencyInvalidation|TestDependencyRepair|TestEvidenceInvalidation' -count=1`
              - `go test ./internal/cli -run 'TestRunReindex.*Dependency|TestRunAsk.*Dependency|TestRunClaim.*Dependent' -count=1`
              - `go test ./...`
              - `git diff --check`
            stop_conditions:
              - Any dependent remains queryable after its required trust input becomes invalid.
              - Restoration requires deleting history, auto-changing dependent status, or bypassing rejected rebuild state.
    phase_stop_conditions:
      - The complete dependency closure is not mechanically reproducible at approval and rebuild boundaries.
      - Any invalid dependency is hidden as a normal query gap or repaired by mutation.
    escalation_and_recovery:
      - Keep the index rejected and canonical history untouched. Report the deterministic dependency path; repair only through explicit claim lifecycle/evidence correction, then rebuild. Do not add query-time fallback or trust scoring.

  - phase_slug: operational-coherence-release-proof
    story_id: 01KZ5CP3DVVPXD2Z02FZEWYG14
    status: done
    goal: Align current operational docs with the hardened runtime and prove every trusted-memory release gate.
    depends_on: recursive-trust-dependency-validation
    allowed_surfaces:
      - Modify `trusted-memory-spec.md`, repository `CLAUDE.md`, `docs/acceptance-walkthrough.md`, and `docs/release.md`.
      - Run existing embedded-asset stale-instruction tests and inspect the intentional qmd deprecation placeholder without changing it unless the test proves active false instructions remain.
      - Run existing Go test/build/smoke/benchmark gates and focused help parity checks.
    avoided_surfaces:
      - `docs/plans/completed/trusted-agent-memory.md`, historical analyses/changelog, and the unrelated active research plan.
      - Product behavior, CLI grammar/JSON schema, assets already accurate, CI redesign, deleted Bun/TypeScript sources, and user runtime data.
    waves:
      - wave: 1-authority-operational-docs
        goal: Correct current authority and operator workflows against the finished runtime without rewriting history.
        depends_on: none
        tasks:
          - task: 1.1 Reconcile trusted-memory authority and project instructions.
            requirements: [R10]
            depends_on: none
            touched_surfaces:
              - modify `trusted-memory-spec.md`
              - modify repository `CLAUDE.md`
            avoided_surfaces:
              - future `references/memory-engine-spec.md` design content
              - completed and historical plans
              - new product promises beyond implemented phases
            expected_outputs:
              - Runtime layout shows root-level `$ZBRAIN_HOME/indexes/<workspace>.sqlite`/`.dirty`, not nested workspace indexes.
              - The stale nonexistent active-plan/digest-gap paragraph is removed and replaced with implemented manifest/rejected-state, journal/history, dependency, and boundary contracts.
              - Project instructions list the exact `zbrain --help` command surface and current test/build/smoke commands.
            checks:
              - `go run ./cmd/zbrain --help`
              - Compare every command and runtime path in both files with help output and `internal/runtime/paths.go`.
              - `git diff --check -- trusted-memory-spec.md CLAUDE.md`
            stop_conditions:
              - Authority docs point to a nonexistent active plan, claim an already-closed digest gap, or describe an unimplemented command/path.
          - task: 1.2 Replace stale Bun/qmd acceptance and release workflows with Go proof.
            requirements: [R10, R11]
            depends_on: none
            touched_surfaces:
              - modify `docs/acceptance-walkthrough.md`
              - modify `docs/release.md`
            avoided_surfaces:
              - historical source analyses and changelog
              - package publishing automation or platform promises not already supported
              - new commands, integrations, or runtime assets
            expected_outputs:
              - Acceptance walks through isolated `setup`, workspace create/current, local evidence, draft/approve, reindex, and `ask` trust behavior using only shipped commands.
              - Release instructions use `go test ./...`, `make build`, `make smoke`, and the 100k benchmark; absent `init`, `learn`, `ingest`, `update`, Bun, and qmd workflows are removed.
              - Examples use isolated `ZBRAIN_HOME` and never touch the operator's real runtime.
            checks:
              - `! grep -RInE 'bun (install|run|test)|bunx|zbrain (init|learn|ingest|update)([[:space:]`]|$)|qmd remains an external prerequisite' docs/acceptance-walkthrough.md docs/release.md`
              - Manually execute or map each documented command to an existing CLI help entry or Make target.
              - `git diff --check -- docs/acceptance-walkthrough.md docs/release.md`
            stop_conditions:
              - A documented acceptance/release step names an absent command, external prerequisite, or non-isolated runtime path.
      - wave: 2-release-gate
        goal: Prove command/docs/assets parity and all trusted-memory release invariants in one isolated final gate.
        depends_on: wave 1-authority-operational-docs
        tasks:
          - task: 2.1 Run parity, regression, build, smoke, and scale proof.
            requirements: [R10, R11]
            depends_on: [task 1.1, task 1.2]
            touched_surfaces:
              - no product surfaces; only the owning scoped file may change if a gate exposes a defect
            avoided_surfaces:
              - weakening tests/trust checks, editing completed plans, broad doc cleanup, and unrelated refactors
            expected_outputs:
              - `zbrain --help`, authority docs, acceptance/release docs, and active embedded instructions agree on the current Go command surface.
              - The only qmd asset hit remains `assets/agents/wiki-qmd-selector.md`, explicitly marked deprecated and directing callers to `zbrain ask`/`reindex`.
              - Full tests, build, isolated smoke, exact freshness/dependency/recovery gates, and 100k p95 all pass; the completed vertical-slice plan remains unchanged.
            checks:
              - `go run ./cmd/zbrain --help`
              - `go test ./internal/runtime -run 'TestExtractBundledAssets|TestBundledAssetsDoNotContainStaleRuntimeInstructions' -count=1`
              - `grep -RIn 'qmd' assets` returns only the reviewed deprecated placeholder.
              - `go test ./...`
              - `make build`
              - `make smoke`
              - `ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v`
              - `git diff --check`
              - `git diff --quiet -- docs/plans/completed/trusted-agent-memory.md`
              - `git status --short`
            stop_conditions:
              - Any help/doc/asset mismatch, failed trust/boundary/recovery/dependency test, non-isolated smoke, modified completed plan, or p95 at/above two seconds.
    phase_stop_conditions:
      - Current operational documentation still advertises deleted behavior or any release gate lacks executable proof.
    escalation_and_recovery:
      - Treat every mismatch or failed command as a release blocker. Fix only its owning phase/file, rerun the focused check and full gate, and never weaken trust assertions, alter historical plans, or publish outward-facing artifacts from this phase.

## Progress
<!-- Append-only durable entries record timestamp, phase, wave, task, task_status, run_id, trace_id, exact verification/result, and changed surfaces or blocker. -->
- timestamp: 2026-08-04T03:55:16Z
  phase: workspace-boundary-mutation-barrier
  wave: phase-start
  task: Start workspace-boundary-mutation-barrier phase
  task_status: in-progress
  run_id: 01KZ5EJD7GZ9RG7CNB1J22EA56
  trace_id: none
  changed_surfaces: []
  verification: "zharness run create --slug workspace-boundary-mutation-barrier --plan-id 01KZ5C29XDKSTXGEG8DQ1XNYCF --json -> 01KZ5EJD7GZ9RG7CNB1J22EA56"
  blocker: none
- timestamp: 2026-08-04T03:59:29Z
  phase: workspace-boundary-mutation-barrier
  wave: 1-boundary-contract
  task: 1.1 Add the central workspace boundary resolver
  task_status: DONE
  run_id: 01KZ5EJD7GZ9RG7CNB1J22EA56
  trace_id: 01KZ5ETYBNGJZ5P7QNRZMJZW48
  changed_surfaces: [internal/runtime/workspace_boundary.go, internal/runtime/workspace_boundary_test.go]
  verification: "gofmt -w internal/runtime/workspace_boundary.go internal/runtime/workspace_boundary_test.go && go test ./internal/runtime -run 'TestValidateWorkspace|TestResolveWorkspacePath|TestSafeWorkspaceName|TestPaths' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-04T04:11:06Z
  phase: workspace-boundary-mutation-barrier
  wave: 2-call-site-barriers
  task: 2.1 Harden ClaimStore and EvidenceStore reads and writes
  task_status: DONE
  run_id: 01KZ5EJD7GZ9RG7CNB1J22EA56
  trace_id: 01KZ5FGC411CG15NM8XPFEBKPN
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/claim_store_test.go, internal/runtime/evidence.go, internal/runtime/evidence_test.go]
  verification: "go test ./internal/runtime -run 'TestClaimStore.*Workspace|TestClaimStore.*Dirty|TestEvidence.*Workspace|TestEvidence.*Dirty|TestEvidenceWorkspaceIsolation' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-04T04:11:06Z
  phase: workspace-boundary-mutation-barrier
  wave: 2-call-site-barriers
  task: 2.2 Harden index, query, current-workspace, and include boundaries
  task_status: DONE
  run_id: 01KZ5EJD7GZ9RG7CNB1J22EA56
  trace_id: 01KZ5FGC411CG15NM8XPFEBKPN
  changed_surfaces: [internal/runtime/index.go, internal/runtime/index_test.go, internal/runtime/query.go, internal/runtime/query_test.go, internal/runtime/workspace.go, internal/runtime/workspace_test.go]
  verification: "go test ./internal/runtime -run 'TestIndex.*Workspace|TestResolveScopes|TestTrustedQuery.*Workspace|TestWorkspaceIsolation|TestCurrentWorkspace' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-04T04:11:06Z
  phase: workspace-boundary-mutation-barrier
  wave: 2-call-site-barriers
  task: 2.3 Move migration and explicit CLI workspace guards ahead of mutation
  task_status: DONE
  run_id: 01KZ5EJD7GZ9RG7CNB1J22EA56
  trace_id: 01KZ5FGC411CG15NM8XPFEBKPN
  changed_surfaces: [internal/cli/cli.go, internal/cli/cli_test.go]
  verification: "go test ./internal/cli -run 'TestRunMigrate.*Dirty|TestRunEvidence.*Workspace|TestRunClaim.*Workspace|Test.*Traversal|Test.*NonexistentWorkspace' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-04T04:12:42Z
  phase: workspace-boundary-mutation-barrier
  wave: 3-boundary-gate
  task: 3.1 Run focused and full boundary regression proof
  task_status: DONE
  run_id: 01KZ5EJD7GZ9RG7CNB1J22EA56
  trace_id: 01KZ5FK8TP23D3ATXJSCKMCSY4
  changed_surfaces: [internal/runtime, internal/cli]
  verification: "go test ./internal/runtime -run 'Test.*Workspace|Test.*Boundary|Test.*Isolation|Test.*Path|Test.*Dirty' -count=1 -> PASS; go test ./internal/cli -run 'TestRun.*(Workspace|Evidence|Claim|Migrate)|Test.*Dirty|Test.*Traversal' -count=1 -> PASS; go test ./... -> PASS; git diff --check -> PASS"
  blocker: none
- timestamp: 2026-08-04T04:45:00Z
  phase: manifest-rejected-index-state
  wave: phase-start
  task: Start manifest-rejected-index-state phase
  task_status: in-progress
  run_id: 01KZ5HKXBS3B2E925HV86SATZ5
  trace_id: none
  changed_surfaces: []
  verification: "zharness run create --slug manifest-rejected-index-state --plan-id 01KZ5C29XDKSTXGEG8DQ1XNYCF --json -> 01KZ5HKXBS3B2E925HV86SATZ5"
  blocker: none

- timestamp: 2026-08-04T08:16:50Z
  phase: manifest-rejected-index-state
  wave: 1-manifest-state-primitives
  task: 1.1 Implement deterministic trust-input manifest scanning
  task_status: DONE
  run_id: 01KZ5HKXBS3B2E925HV86SATZ5
  trace_id: 01KZ5XHB8J9Z39YKFV86RQV3FG
  changed_surfaces: [internal/runtime/manifest.go, internal/runtime/manifest_test.go]
  verification: "go test ./internal/runtime -run 'TestTrustInputManifest|TestManifestDetects(Add|Remove|Change)|TestManifestEvidenceInputs|TestManifestDeterministic' -count=1 -> PASS; go test ./internal/runtime -> PASS; go test ./... -> PASS"
  blocker: none
- timestamp: 2026-08-04T08:16:50Z
  phase: manifest-rejected-index-state
  wave: 1-manifest-state-primitives
  task: 1.2 Add disposable rebuild-state and manifest tables
  task_status: DONE
  run_id: 01KZ5HKXBS3B2E925HV86SATZ5
  trace_id: 01KZ5XHB8J9Z39YKFV86RQV3FG
  changed_surfaces: [internal/runtime/index_state.go, internal/runtime/index_state_test.go, internal/runtime/index.go]
  verification: "go test ./internal/runtime -run 'TestIndexRebuildState|TestIndexTrustInputs|TestIndexStateMissing|TestIndexStateRejected' -count=1 -> PASS; go test ./internal/runtime -run 'TestIndex(RebuildState|TrustInputs)|TestIndexState' -count=1 -> PASS"
  blocker: none

- timestamp: 2026-08-04T08:26:26Z
  phase: manifest-rejected-index-state
  wave: 2-rebuild-publication
  task: 2.1 Integrate manifest and rejection state into IndexStore.Rebuild
  task_status: DONE
  run_id: 01KZ5HKXBS3B2E925HV86SATZ5
  trace_id: 01KZ5Y35H2MFHK5TA0FRW6Z2R6
  changed_surfaces: [internal/runtime/index.go, internal/runtime/index_test.go]
  verification: "go test ./internal/runtime -run 'TestRebuildManifest|TestRebuildCleanState|TestRebuildRejectedState|TestRebuildFailureLeavesDirty|TestRebuildDoesNotMutateCanonical' -count=1 -> PASS; go test ./internal/runtime -> PASS; go test ./... -> PASS"
  blocker: none
- timestamp: 2026-08-04T08:26:26Z
  phase: manifest-rejected-index-state
  wave: 2-rebuild-publication
  task: 2.2 Test rebuild publication and failure ordering
  task_status: DONE
  run_id: 01KZ5HKXBS3B2E925HV86SATZ5
  trace_id: 01KZ5Y35H2MFHK5TA0FRW6Z2R6
  changed_surfaces: [internal/runtime/index_test.go]
  verification: "go test ./internal/runtime -run 'TestRebuildManifest|TestRebuildCleanState|TestRebuildRejectedState|TestRebuildFailureLeavesDirty|TestRebuildDoesNotMutateCanonical' -count=1 -> PASS; go vet ./internal/runtime -> PASS; git diff --check -> PASS"
  blocker: none

- timestamp: 2026-08-04T09:14:24Z
  phase: manifest-rejected-index-state
  wave: 3-query-trust-gate
  task: 3.1 Enforce exact freshness and rebuild state in CheckFresh and TrustedQuery; 3.2 expose rebuild trust outcome and prove the 100k gate
  task_status: DONE
  run_id: 01KZ5HKXBS3B2E925HV86SATZ5
  trace_id: 01KZ60WYPWNXRPGFZQ3K650F4K
  changed_surfaces: [internal/runtime/index.go, internal/runtime/query.go, internal/runtime/manifest.go, internal/runtime/index_state.go, internal/cli/cli.go, internal/runtime/index_test.go, internal/runtime/query_test.go, internal/cli/cli_test.go]
  verification: "focused freshness/state tests -> PASS; actual CLI reindex/ask trust tests -> PASS; go test ./... -> PASS; go test -race ./internal/runtime -> PASS; go vet ./... -> PASS; git diff --check -> PASS; make build && make smoke -> PASS; ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v -> PASS, p95=1.92081284s samples=40"
  blocker: none
- timestamp: 2026-08-05T02:45:50Z
  phase: lifecycle-transition-recovery
  wave: phase-start
  task: Start lifecycle-transition-recovery phase
  task_status: in-progress
  run_id: 01KZ7WZTJBNDKF6AZ7CHNTB727
  trace_id: none
  changed_surfaces: []
  verification: "zharness run create --slug lifecycle-transition-recovery --plan-id 01KZ5C29XDKSTXGEG8DQ1XNYCF --json -> 01KZ7WZTJBNDKF6AZ7CHNTB727"
  blocker: none
- timestamp: 2026-08-05T02:50:46Z
  phase: lifecycle-transition-recovery
  wave: 1-transition-contract
  task: 1.1 Add typed transition parse/render and digest rules
  task_status: DONE
  run_id: 01KZ7WZTJBNDKF6AZ7CHNTB727
  trace_id: 01KZ7X9WYERRT1Y537CBTVEE22
  changed_surfaces: [internal/runtime/claim.go, internal/runtime/claim_test.go]
  verification: "gofmt -w internal/runtime/claim.go internal/runtime/claim_test.go && go test ./internal/runtime -run 'TestClaimTransition|TestClaimRoundTrip|TestClaimVerificationDigest|TestLegacyApprovedClaimWithoutTransitions' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-05T02:56:08Z
  phase: lifecycle-transition-recovery
  wave: 2-recovery-primitives
  task: 2.1 Implement the pending-transition journal and idempotent target recovery
  task_status: DONE
  run_id: 01KZ7WZTJBNDKF6AZ7CHNTB727
  trace_id: none
  changed_surfaces: [internal/runtime/transition.go, internal/runtime/transition_test.go]
  verification: "gofmt -w internal/runtime/transition.go internal/runtime/transition_test.go && go test ./internal/runtime -run 'TestTransitionJournal|TestTransitionRecovery|TestTransitionPreimageMismatch|TestTransitionJournalPath|TestTransitionRecoveryIdempotent' -count=1 -> PASS; go test ./internal/runtime -run 'TestTransition' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-05T03:00:22Z
  phase: lifecycle-transition-recovery
  wave: 2-recovery-primitives
  task: 2.2 Lock the four-state transition graph and historical-preservation cases
  task_status: DONE
  run_id: 01KZ7WZTJBNDKF6AZ7CHNTB727
  trace_id: 01KZ7XW1EV0K10JAP6VQRMGAPJ
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/claim_store_test.go, internal/cli/cli_test.go]
  verification: "gofmt -w internal/runtime/claim_store.go internal/runtime/claim_store_test.go && go test ./internal/runtime -run 'TestApproveTransitionGraph|TestSupersedeTransitionGraph|TestRevokeTransitionGraph|TestLifecycleHistoryPreserved' -count=1 -> PASS; gofmt -w internal/cli/cli_test.go && go test ./internal/cli -run 'TestRunClaim.*(Approve|Supersede|Revoke|InvalidTransition)' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-05T03:14:10Z
  phase: lifecycle-transition-recovery
  wave: 3-lifecycle-integration
  task: 3.1 Integrate structured approval, supersession, and revocation in ClaimStore
  task_status: DONE
  run_id: 01KZ7WZTJBNDKF6AZ7CHNTB727
  trace_id: 01KZ7YKB0Q68PP0NEHK52XGAXM
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/claim_store_test.go, internal/cli/cli_test.go]
  verification: "go test ./internal/runtime -run 'TestClaimStore.*(Approve|Supersede|Revoke|Transition)|TestSupersessionFailureInjection|TestMultipleSupersededClaims' -count=1 -> PASS; go test ./internal/cli -run 'TestRunClaim.*(Approve|Supersede|Revoke|InvalidTransition)' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-05T03:14:10Z
  phase: lifecycle-transition-recovery
  wave: 3-lifecycle-integration
  task: 3.2 Integrate recovery and blocking at existing command boundaries
  task_status: DONE
  run_id: 01KZ7WZTJBNDKF6AZ7CHNTB727
  trace_id: 01KZ7YKB0Q68PP0NEHK52XGAXM
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/index.go, internal/runtime/query.go, internal/runtime/index_test.go, internal/runtime/query_test.go, internal/runtime/claim_store_test.go, internal/cli/cli_test.go]
  verification: "go test ./internal/runtime -run 'TestRebuildRecoversPendingTransition|TestTrustedQueryBlocksPendingTransition|TestMutationRecoversPendingTransition|TestRecoveryLeavesDirty' -count=1 -> PASS; go test ./internal/cli -run 'TestRunReindex.*Recovery|TestRunAsk.*PendingTransition|TestRunClaim.*Recovery' -count=1 -> PASS; go test ./... -> PASS; git diff --check -> PASS; make build -> PASS; make smoke -> PASS"
  blocker: none
- timestamp: 2026-08-05T03:39:20Z
  phase: recursive-trust-dependency-validation
  wave: phase-start
  task: Start recursive-trust-dependency-validation phase
  task_status: in-progress
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  trace_id: none
  changed_surfaces: []
  verification: "zharness run create --slug recursive-trust-dependency-validation --plan-id 01KZ5C29XDKSTXGEG8DQ1XNYCF --json -> 01KZ80207KNEBFSVBVM02KMRF5"
  blocker: none
- timestamp: 2026-08-05T03:47:53Z
  phase: recursive-trust-dependency-validation
  wave: 1-validator-primitives
  task: 1.1 Implement deterministic recursive supporting-claim validation
  task_status: DONE
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  trace_id: none
  changed_surfaces: [internal/runtime/trust_validation.go, internal/runtime/trust_validation_test.go]
  verification: "gofmt -w internal/runtime/trust_validation.go internal/runtime/trust_validation_test.go && go test ./internal/runtime -run 'TestTrustValidation.*(Chain|Missing|Status|Digest|Cycle|Deterministic|Memoized)' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-05T03:52:56Z
  phase: recursive-trust-dependency-validation
  wave: 1-validator-primitives
  task: 1.2 Harden complete evidence metadata/raw verification with per-run caching
  task_status: DONE
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  trace_id: none
  changed_surfaces: [internal/runtime/evidence.go, internal/runtime/evidence_test.go]
  verification: "gofmt -w internal/runtime/evidence.go internal/runtime/evidence_test.go && go test ./internal/runtime -run 'TestEvidenceVerify.*(Metadata|Missing|Size|Hash|Workspace|Cache)' -count=1 -> PASS; go test ./internal/runtime -run '^TestEvidence' -count=1 -> PASS; go test ./internal/runtime -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-05T03:57:42Z
  phase: recursive-trust-dependency-validation
  wave: 2-approval-rebuild-integration
  task: 2.1 Validate prospective approved claims before any approval mutation
  task_status: DONE
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  trace_id: none
  changed_surfaces: [internal/runtime/trust_validation.go, internal/runtime/evidence.go, internal/runtime/claim_store.go, internal/runtime/claim_store_test.go]
  verification: "gofmt -w internal/runtime/trust_validation.go internal/runtime/claim_store.go internal/runtime/claim_store_test.go && go test ./internal/runtime -run 'TestClaimStoreApprove.*(DeepSupport|InvalidDigest|Revoked|Superseded|MissingEvidence|TamperedEvidence|Cycle|NoWrite)' -count=1 -> PASS; go test ./internal/runtime -run '^TestClaimStore' -count=1 -> PASS"
  blocker: none
- timestamp: 2026-08-05T04:05:00Z
  phase: recursive-trust-dependency-validation
  wave: 2-approval-rebuild-integration
  task: 2.2 Validate every approved dependency closure during rebuild
  task_status: DONE
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  trace_id: none
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/index.go, internal/runtime/index_test.go]
  verification: "gofmt -w internal/runtime/index_test.go && go test ./internal/runtime -run 'TestRebuild.*(Dependency|Derived|Evidence|Cycle|RejectedState|CanonicalUnchanged)' -count=1 -> PASS; go test ./... -> PASS; git diff --check -> PASS"
  blocker: none
- timestamp: 2026-08-05T04:12:14Z
  phase: recursive-trust-dependency-validation
  wave: 3-dependency-gate
  task: 3.1 Run lifecycle/evidence invalidation and clean-rebuild recovery scenarios
  task_status: DONE
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  trace_id: none
  changed_surfaces: [internal/runtime/query_test.go, internal/cli/cli_test.go]
  verification: "gofmt -w internal/runtime/query_test.go internal/cli/cli_test.go && go test ./internal/runtime -run 'TestTrustedQuery.*Dependency|TestDependencyInvalidation|TestDependencyRepair|TestEvidenceInvalidation' -count=1 -> PASS; go test ./internal/cli -run 'TestRunReindex.*Dependency|TestRunAsk.*Dependency|TestRunClaim.*Dependent' -count=1 -> PASS; go test ./... -> PASS; git diff --check -> PASS"
  blocker: none
- timestamp: 2026-08-05T04:24:09Z
  phase: recursive-trust-dependency-validation
  wave: 2-approval-rebuild-integration
  task: Validate evidence attached to every supporting claim in the recursive closure
  task_status: DONE
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  trace_id: none
  changed_surfaces: [internal/runtime/evidence.go, internal/runtime/trust_validation.go, internal/runtime/claim_store.go, internal/runtime/index.go, internal/runtime/claim_store_test.go, internal/runtime/index_test.go]
  verification: "go test ./internal/runtime -run 'TestClaimStoreApproveRejectsSupportingClaimEvidence|TestRebuildRejectsDependentWhenSupportingEvidenceInvalid|TestRebuild.*(Dependency|Derived|Evidence|Cycle|RejectedState|CanonicalUnchanged)' -count=1 -> PASS; go test ./... -> PASS; go vet ./... -> PASS; go test -race ./internal/runtime ./internal/cli -> PASS; make build && make smoke -> PASS; ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v -> PASS, p95=1.832549913s; git diff --check -> PASS"
  blocker: none
- timestamp: 2026-08-05T04:32:12Z
  phase: operational-coherence-release-proof
  wave: phase-start
  task: Start operational-coherence-release-proof phase
  task_status: in-progress
  run_id: 01KZ832HGZ7C6NXDNCXH16GWMV
  trace_id: none
  changed_surfaces: []
  verification: "zharness run create --slug operational-coherence-release-proof --plan-id 01KZ5C29XDKSTXGEG8DQ1XNYCF --json -> 01KZ832HGZ7C6NXDNCXH16GWMV"
  blocker: none
- timestamp: 2026-08-05T04:43:20Z
  phase: operational-coherence-release-proof
  wave: 1-authority-operational-docs
  task: 1.1 Reconcile authority project docs and 1.2 replace stale acceptance/release workflows
  task_status: DONE
  run_id: 01KZ832HGZ7C6NXDNCXH16GWMV
  trace_id: none
  changed_surfaces: [trusted-memory-spec.md, CLAUDE.md, docs/acceptance-walkthrough.md, docs/release.md]
  verification: "go run ./cmd/zbrain --help -> PASS; make help -> PASS; no stale Bun/qmd/deleted-command instructions in active acceptance/release docs; git diff --check -- trusted-memory-spec.md CLAUDE.md docs/acceptance-walkthrough.md docs/release.md -> PASS"
  blocker: none
- timestamp: 2026-08-05T04:43:20Z
  phase: operational-coherence-release-proof
  wave: 2-release-gate
  task: 2.1 Run parity, regression, build, smoke, and scale proof
  task_status: DONE
  run_id: 01KZ832HGZ7C6NXDNCXH16GWMV
  trace_id: none
  changed_surfaces: [internal/cli, internal/runtime, trusted-memory-spec.md, CLAUDE.md, docs/acceptance-walkthrough.md, docs/release.md]
  verification: "go test ./... -> PASS; go vet ./... -> PASS; go test -race ./internal/runtime ./internal/cli -> PASS; make smoke -> PASS; ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v -> PASS, p95=1.805479637s samples=40; git diff --check -> PASS"
  blocker: none

## Decisions
<!-- Append-only durable entries record timestamp, phase/task, decision, and rationale. -->
- timestamp: 2026-08-05T03:00:22Z
  phase: lifecycle-transition-recovery
  task: 2.2 Lock the four-state transition graph and historical-preservation cases
  decision: Implement the four-state graph and historical preservation in ClaimStore while locking the contract tests, rather than deferring the behavior to wave 3.
  rationale: Existing ClaimStore allowed revoking non-approved claims, cleared approval attestations, appended free-form revocation text to claim bodies, and silently ignored non-approved supersession targets; the task checks cannot pass honestly without closing those contract gaps. The phase allows ClaimStore lifecycle behavior, while recoverable multi-file orchestration remains in wave 3.

## Validation
<!-- Append-only durable entries record timestamp, phase, exact command/result/output, run_id, check_id, verdict, and proof_gaps. -->
- timestamp: 2026-08-04T04:41:01Z
  phase: workspace-boundary-mutation-barrier
  exact_commands:
    - command: "go test ./internal/runtime -run 'TestIndexOperationsRejectSymlinkedIndex|TestIndexOperationsRejectUnsafeOrMissingWorkspaceBeforePathCreation|TestIndexDirtyMarkerBlocksSearch|TestReindexIsDeterministicAfterDeletingIndex' -count=1"
      result: PASS
      output: "index boundary and API regression tests passed"
    - command: "go test ./..."
      result: PASS
      output: "all packages passed"
    - command: "go vet ./..."
      result: PASS
      output: "no diagnostics"
    - command: "git diff --check"
      result: PASS
      output: "no whitespace errors"
    - command: "make build"
      result: PASS
      output: "go build -o dist/zbrain ./cmd/zbrain"
    - command: "make smoke"
      result: PASS
      output: "isolated setup, workspace create/current, claim approve, reindex, and trusted ask passed"
    - command: "ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v"
      result: PASS
      output: "100k claim search p95=1.091891564s samples=40"
  run_id: 01KZ5EJD7GZ9RG7CNB1J22EA56
  check_id: 01KZ5H5RPZ03FEF8HS97Z2A5NP
  verdict: APPROVED
  judge: independent
  judge_model: gpt-5.6-luna
  proof_gaps: none

- timestamp: 2026-08-04T09:14:24Z
  phase: manifest-rejected-index-state
  exact_commands:
    - command: "go test ./internal/runtime -run 'TestTrustInputManifest|TestManifestDetects(Add|Remove|Change)|TestManifestEvidenceInputs|TestManifestDeterministic|TestIndexRebuildState|TestIndexTrustInputs|TestIndexStateMissing|TestIndexStateRejected|TestCheckFresh' -count=1"
      result: PASS
      output: "focused manifest, state, and freshness tests passed"
    - command: "go test ./internal/cli -run 'TestRun(ReindexAndAskTrustedContext|ReindexReportsTamperedApprovedClaim)|TestRunAskReportsFreshnessErrors' -count=1"
      result: PASS
      output: "actual CLI reindex/ask trust tests passed"
    - command: "go test ./..."
      result: PASS
      output: "all packages passed"
    - command: "go test -race ./internal/runtime"
      result: PASS
      output: "runtime race detector passed"
    - command: "go vet ./..."
      result: PASS
      output: "no diagnostics"
    - command: "git diff --check"
      result: PASS
      output: "no whitespace errors"
    - command: "make build && make smoke"
      result: PASS
      output: "isolated build, setup, workspace, reindex, and trusted ask passed"
    - command: "ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run TestAskP95At100K -count=1 -v"
      result: PASS
      output: "100k claim search p95=1.92081284s samples=40"
    - command: "git diff --quiet -- docs/plans/completed/trusted-agent-memory.md"
      result: PASS
      output: "completed plan unchanged"
  run_id: 01KZ5HKXBS3B2E925HV86SATZ5
  check_id: 01KZ60VY0MQWR73Y88VDBR2FD0
  verdict: APPROVED
  judge: same-session
  judge_model: gpt-5.6-luna
  proof_gaps: "No separate external reviewer; manual Security, Performance, Architecture, and Code Quality review was performed same-session."

- timestamp: 2026-08-05T03:21:03Z
  phase: lifecycle-transition-recovery
  exact_commands:
    - command: "gofmt -w internal/runtime/transition.go internal/runtime/transition_test.go && go test ./internal/runtime -run 'TestTransition|TestRebuildRecoversPendingTransition|TestTrustedQueryBlocksPendingTransition|TestMutationRecoversPendingTransition|TestRecoveryLeavesDirty' -count=1"
      result: PASS
      output: "journal publication and all pre/post-rename recovery-state tests passed"
    - command: "go test ./..."
      result: PASS
      output: "all packages passed"
    - command: "go vet ./..."
      result: PASS
      output: "no diagnostics"
    - command: "go test -race ./internal/runtime ./internal/cli"
      result: PASS
      output: "runtime and CLI race detectors passed"
    - command: "git diff --check"
      result: PASS
      output: "no whitespace errors"
    - command: "make build"
      result: PASS
      output: "go build -o dist/zbrain ./cmd/zbrain"
    - command: "make smoke"
      result: PASS
      output: "isolated setup, workspace, claim lifecycle, reindex, and trusted ask passed"
  run_id: 01KZ7WZTJBNDKF6AZ7CHNTB727
  check_id: 01KZ7YW9YWN44GEF997BTNJPK1
  verdict: APPROVED
  judge: same-session
  judge_model: gpt-5.6-luna
  proof_gaps: "No separate external reviewer; manual Security, Performance, Architecture, and Code Quality review was same-session. Deterministic pre/post-rename states are covered; no live process-kill fault-injection harness was run."

- timestamp: 2026-08-05T04:25:06Z
  phase: recursive-trust-dependency-validation
  exact_commands:
    - command: "go test ./internal/runtime -run 'TestTrustedQuery.*Dependency|TestDependencyInvalidation|TestDependencyRepair|TestEvidenceInvalidation' -count=1"
      result: PASS
      output: "recursive dependency invalidation, rejected-query, evidence tamper/missing, and clean-rebuild repair tests passed"
    - command: "go test ./internal/cli -run 'TestRunReindex.*Dependency|TestRunAsk.*Dependency|TestRunClaim.*Dependent' -count=1"
      result: PASS
      output: "CLI dependency reindex rejection and ask fail-closed tests passed"
    - command: "go test ./..."
      result: PASS
      output: "all packages passed after supporting-claim evidence closure fix"
    - command: "go vet ./..."
      result: PASS
      output: "no diagnostics"
    - command: "go test -race ./internal/runtime ./internal/cli"
      result: PASS
      output: "runtime and CLI race detectors passed"
    - command: "make build && make smoke"
      result: PASS
      output: "Go build, help parity, isolated setup/workspace/claim/reindex/ask smoke passed"
    - command: "ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v"
      result: PASS
      output: "100k claim search p95=1.832549913s samples=40"
    - command: "git diff --check && git diff --quiet -- docs/plans/completed/trusted-agent-memory.md"
      result: PASS
      output: "no whitespace errors; completed plan unchanged"
  run_id: 01KZ80207KNEBFSVBVM02KMRF5
  check_id: 01KZ82PDYY125WPMAE4HYZG5KB
  verdict: APPROVED
  judge: same-session
  judge_model: gpt-5.6-luna
  proof_gaps: "No independent external reviewer; Security, Performance, Architecture, and Code Quality review was same-session. A supporting-claim evidence closure edge case was found and fixed; no live process-kill fault-injection harness was run."
- timestamp: 2026-08-05T04:44:59Z
  phase: operational-coherence-release-proof
  exact_commands:
    - command: "go run ./cmd/zbrain --help"
      result: PASS
      output: "current Go command surface matched authority and operational docs"
    - command: "make help"
      result: PASS
      output: "build, test, smoke, install-local, and clean targets listed"
    - command: "no stale Bun/qmd/deleted-command instructions in active acceptance/release docs"
      result: PASS
      output: "no stale acceptance/release instructions"
    - command: "go test ./..."
      result: PASS
      output: "all packages passed"
    - command: "go vet ./..."
      result: PASS
      output: "no diagnostics"
    - command: "go test -race ./internal/runtime ./internal/cli"
      result: PASS
      output: "runtime and CLI race detectors passed"
    - command: "make smoke"
      result: PASS
      output: "build, help, isolated setup/workspace/evidence/claim/reindex/ask smoke passed"
    - command: "ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v"
      result: PASS
      output: "100k claim search p95=1.805479637s samples=40"
    - command: "git diff --check"
      result: PASS
      output: "no whitespace errors"
    - command: "zharness audit --json"
      result: PASS
      output: "no contract violations or unlinked proofs after check recording"
  run_id: 01KZ832HGZ7C6NXDNCXH16GWMV
  check_id: 01KZ83SXFVMKK77SEQ4WD9T31P
  verdict: APPROVED
  judge: same-session
  judge_model: gpt-5.6-luna
  proof_gaps: "No independent external reviewer; this same-session review did not run a live process-kill fault-injection harness or cross-platform packaging builds."

## Current State and Next Action
- active_phase: none
- lifecycle_status: done
- completed_work: workspace-boundary-mutation-barrier, manifest-rejected-index-state, lifecycle-transition-recovery, recursive-trust-dependency-validation, and operational-coherence-release-proof are done; lifecycle transitions, atomic pending-transition recovery, four-state enforcement, recursive trust closure validation, trust-boundary blocking, current operational docs, and full release proof are recorded
- latest_run_id: 01KZ832HGZ7C6NXDNCXH16GWMV
- latest_trace_ids: []
- latest_check_id: 01KZ83SXFVMKK77SEQ4WD9T31P
- latest_handoff_id: 01KZ83WFJJZ6MP2P361DAG9JWT
- blockers: none
- open_items: none
- exact_next_action: initiative closed; start a new scoped phase with /to-plan
