---
id: 01KZ8PVTFHBYTJJXQZWJG07RHB
type: plan
intake_id: 01KZ96279PXDAXYZ5XC9QR9QS9
lane: high-risk
status: active
created: 2026-08-05
updated: 2026-08-05
---

# Plan: Trusted Memory Hardening and Claude Code Skill Coherence

## Outcome
- result: zbrain keeps canonical trusted memory authoritative under index tampering, concurrent mutation, freshness edge cases, and evidence replacement while its Claude Code skills and embedded assets accurately describe and safely invoke the Go-native CLI.
- success_signals:
  - Trusted queries reject index rows that do not match approved canonical claims, current evidence, and the published trust generation.
  - Concurrent mutation, rebuild, and query tests cannot publish or serve a stale clean index.
  - Content changes remain detectable even when file mtimes are restored, without weakening the existing fail-closed contract.
  - Legacy approved claims have a deterministic migrate-to-draft and explicit reapproval path, and migration freshness reporting reflects the actual index state.
  - Embedded skills, agents, engine rules, templates, runtime layout documentation, and `zbrain --help` agree on the shipped command surface and scope rules.
  - Active skills use shell-safe argument handling, do not expose executable network-fetch workflows, and preserve explicit workspace boundaries.
  - `go test ./...`, vet, race, build, isolated smoke, and the 100k-claim p95-under-two-seconds gate remain green.

## Authority and Requirements
- authority:
  - `trusted-memory-spec.md` — canonical trusted-memory contract, lifecycle, evidence, freshness, and release gates.
  - repository `CLAUDE.md` — Go-native implementation constraints, runtime layout, and proof commands.
  - `internal/cli/cli.go` and `internal/runtime/` — shipped command grammar and actual runtime behavior.
  - `internal/runtime/assets.go`, `assets/embed.go`, `assets/skills/`, `assets/agents/`, and `assets/engine/` — embedded Claude Code-facing runtime content.
  - `internal/runtime/*_test.go` and `internal/cli/*_test.go` — existing security, trust, boundary, lifecycle, and CLI regression contracts.
  - `docs/acceptance-walkthrough.md` and `docs/release.md` — operator-facing acceptance and release proof.
  - owner decision in this request: plan the full runtime-trust plus Claude Code skills/assets hardening scope and commit/push the plan for continuation on another machine.
- requirements:
  - R1 [accepted]: Trusted query results must be bound to the current canonical approved claim identity, verification digest, path, and status; a SQLite-only edit must never promote or alter trusted context. | source: trusted-memory-spec.md sections 1, 5, 8, and 9
  - R2 [accepted]: Workspace mutation, rebuild, publication, and query freshness checks must coordinate through a per-workspace generation or lock protocol that cannot publish an older canonical view as clean after a concurrent mutation. | source: trusted-memory-spec.md sections 4 and 8
  - R3 [accepted]: Freshness validation must detect canonical claim and evidence content changes even when an attacker restores file mtimes; the implementation must retain fail-closed behavior and the p95 release threshold. | source: trusted-memory-spec.md sections 8 and 11
  - R4 [accepted]: Every evidence-backed approved claim must bind its approved source reference to the current immutable evidence snapshot digest, and replacing metadata/raw bytes must invalidate the claim. | source: trusted-memory-spec.md section 7
  - R5 [accepted]: Legacy approved claims must have a supported migration path that produces an explicit reapproval candidate rather than silently preserving trust without an OKF verification digest. | source: trusted-memory-spec.md section 6
  - R6 [accepted]: `migrate okf` must report index freshness only from an actual freshness check or an explicitly false/unknown result; it must not claim freshness merely because no document was migrated. | source: trusted-memory-spec.md section 8
  - R7 [accepted]: Runtime layout documentation, extractor behavior, README, tests, and active skills must agree on the actual paths emitted below `ZBRAIN_HOME`, including embedded skills, agents, templates, indexes, and workspaces. | source: CLAUDE.md, internal/runtime/assets.go, assets/embed.go
  - R8 [accepted]: CLI help and skill guidance must expose the supported command arguments and workspace scopes, and each command must reject unknown flags rather than silently discarding or reinterpreting them. | source: internal/cli/cli.go, trusted-memory-spec.md section 2
  - R9 [accepted]: Claude Code skill examples must pass user-controlled values through shell-safe argv or quoted variables, preserve explicit workspace/include consent, and never instruct agents to interpolate untrusted text into shell source. | source: assets/skills/zbrain-ask/SKILL.md, assets/skills/zbrain-learn/SKILL.md, assets/engine/claude-rules.md
  - R10 [accepted]: Active embedded runtime assets must not expose executable network-fetch, legacy QA/apply, or deleted-command workflows; deprecated material must be removed from runtime assets or clearly isolated and tested as legacy. | source: trusted-memory-spec.md out-of-scope section and assets/skills/
  - R11 [accepted]: Canonical claim paths, lifecycle lookup, conflict detection, runtime permissions, and `workspace current` output must have one documented contract with focused regression coverage where current behavior is intentionally retained or changed. | source: trusted-memory-spec.md sections 4, 5, and 9; internal/runtime/*_test.go

## Non-goals
- NG1: Reintroducing Bun, Node, TypeScript, Commander, clack, qmd, LLM calls, MCP, vector search, hosted sync, or network research into the product runtime.
- NG2: Adding claim lifecycle states, auto-revocation, physical deletion, query-time repair, or mutation of canonical claims/evidence to hide invalid dependencies.
- NG3: Weakening the existing verification digest, evidence hash, workspace boundary, rejected-index, transition recovery, or p95-under-two-seconds assertions to make tests pass.
- NG4: Redesigning CI, packaging automation, platform support, or unrelated research plans.
- NG5: Rewriting historical completed plans, changing user runtime data, or broad-cleaning assets that are outside the trust/skills scope.
- NG6: Introducing a new skill framework; improvements must use the existing embedded asset and Go CLI model.

## Approach and Risks
- approach:
  - Preserve Markdown and immutable evidence as the only trust authority; harden each derived-state boundary instead of moving trust into SQLite or skill text.
  - Implement in dependency order: canonical index binding, workspace generation coordination, content/evidence digest freshness, legacy migration/reporting, CLI/assets parity, shell-safe skill guidance, runtime boundary compatibility, then release proof.
  - For every behavior change, add a focused regression first, make rejection fail closed, and prove canonical claims/evidence are not rewritten, deleted, or auto-revoked.
  - Keep the Go command surface and four-state lifecycle unchanged; use existing runtime paths and embedded assets rather than introducing a new framework.
  - Run independent skills/assets work in parallel with the trust chain where possible, then require all phase gates before release proof.
- rejected_alternatives:
  - Query-time rehashing of every returned claim: rejected because it duplicates rebuild-boundary validation and threatens the 100k-claim p95 gate.
  - Mtime-only freshness or a SQLite-only authority record: rejected because restored mtimes and database tampering must remain detectable from canonical content.
  - Automatic repair, deletion, or revocation of invalid canonical inputs: rejected by the trusted-memory contract and the four-state lifecycle.
  - A broad rewrite of the skill system or a new skill framework: rejected because the existing embedded Go asset model is sufficient and must remain reviewable.
- constraints:
  - Go-native only; no Bun, Node, TypeScript, Commander, clack, qmd, LLM, MCP, vector search, network research, hosted sync, or new command family.
  - Do not modify completed plans, unrelated active plans, or the operator's real runtime data; all manual validation uses isolated `ZBRAIN_HOME`.
  - Preserve fail-closed behavior, workspace isolation, transition recovery, rejected-index semantics, and the p95-under-two-seconds benchmark gate.
  - Phase definitions and task definitions become immutable after this planning pass; only lifecycle status may change later.
- risks:
  - risk: stronger canonical binding and content verification may increase rebuild/query cost.
    mitigation: keep verification at rebuild/publication boundaries, measure the existing 100k benchmark, and stop on a p95 regression above two seconds.
  - risk: concurrent mutation/rebuild recovery could leave an index incorrectly marked clean or strand a lock.
    mitigation: use atomic generation/dirty transitions, deterministic crash-window tests, and fail closed whenever ownership or generation is ambiguous.
  - risk: legacy migration could accidentally preserve trust without a verifiable digest.
    mitigation: make migrated documents explicit reapproval candidates and test that missing verification metadata never enters trusted results.
  - risk: shell-safe skill changes could silently break valid workspace/include workflows.
    mitigation: test documented command examples against `--help`, use adversarial values containing spaces, quotes, and shell metacharacters, and retain explicit scope controls.
  - risk: embedded assets and documentation may drift again after the fix.
    mitigation: add extraction/layout and command-parity tests that inspect shipped assets and help output from the same repository revision.
  - risk: tightening permissions or path handling could break existing isolated runtimes.
    mitigation: define the minimum required permission contract, cover fresh and existing workspaces, and stop if the change crosses the documented compatibility boundary.
- recovery:
  - Stop the active phase when a requirement cannot be proved, a canonical input would be mutated, or a non-goal is implicated; record the blocker in `## Progress` and route back to `brainstorm refine` before changing scope.
  - If a test exposes an unsafe partial publication, leave the workspace fail closed, preserve the journal/dirty evidence, and recover only through the existing mutation recovery path.

## Phases and Verification
<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. Each planned phase records phase_slug, story_id, status, goal, depends_on, waves, tasks, and checks. -->
- planning_status: planned
- phases:
  - phase_slug: canonical-index-binding
    story_id: 01KZ962RJ9MK11SXBKD14AQ0Y5
    status: checked
    goal: Bind derived index rows and trusted query results to canonical approved claim identity, digest, status, and path.
    depends_on: none
    allowed_surfaces: [internal/runtime/index.go, internal/runtime/query.go, internal/runtime/claim.go, internal/runtime/*_test.go, trusted-memory-spec.md]
    avoided_surfaces: [claim lifecycle states, canonical auto-repair, unrelated CLI commands]
    waves:
      - wave: 1
        objective: Define and implement the smallest canonical binding carried by derived rows and checked at publication/query boundaries.
        tasks:
          - task_id: cib-1
            description: Trace the current rebuild and ask paths, choose the canonical binding representation, and implement fail-closed validation for indexed approved claims.
            depends_on: none
            touched_surfaces: [internal/runtime/index.go, internal/runtime/query.go]
            expected_output: Every trusted row has a verifiable canonical identity/path/status/digest binding; SQLite-only edits cannot change trusted context.
            verify: "go test ./internal/runtime -run '^TestCanonicalIndexBinding$' -count=1 -v"
            stop_if: The design makes SQLite authoritative or requires query-time repair/rewrites.
            escalation: Stop and return to brainstorm refine with the failing contract.
          - task_id: cib-2
            description: Add regression fixtures for SQLite-only body, status, path, and digest tampering plus missing canonical rows.
            depends_on: cib-1
            touched_surfaces: [internal/runtime/*_test.go]
            expected_output: Each tamper case is rejected while canonical Markdown remains unchanged.
            verify: "go test ./internal/runtime -run '^TestCanonicalIndexBinding' -count=1 -v"
            stop_if: A tampered database still yields ready trusted context.
            escalation: Preserve the failing fixture and route to runtime trust review.
      - wave: 2
        objective: Make diagnostics and documentation explain the binding failure without changing the trusted query contract.
        tasks:
          - task_id: cib-3
            description: Add deterministic failure reasons and update the trusted-memory contract for the derived-row binding boundary.
            depends_on: cib-2
            touched_surfaces: [internal/runtime/query.go, trusted-memory-spec.md, docs/acceptance-walkthrough.md]
            expected_output: Operators receive an actionable fail-closed error and the docs state the same authority rule.
            verify: "go test ./internal/runtime ./internal/cli && git diff --check"
            stop_if: Diagnostics disclose no offending workspace-local path or weaken fail-closed behavior.
            escalation: Record the gap in Progress and defer release proof.
    checks:
      - command: "go test ./internal/runtime -run '^TestCanonicalIndexBinding' -count=1 -v"
        proves: Canonical binding, SQLite tamper rejection, and canonical preservation.
      - command: "go test ./internal/runtime ./internal/cli"
        proves: Existing trust/query and CLI regressions remain green.

  - phase_slug: workspace-generation-coordination
    story_id: 01KZ962RJHNRTBPZCJVJGGBMEY
    status: planned
    goal: Coordinate mutation, rebuild, publication, and query freshness so concurrent work cannot publish or serve a stale clean index.
    depends_on: canonical-index-binding
    allowed_surfaces: [internal/runtime/index.go, internal/runtime/transition.go, internal/runtime/claim_store.go, internal/runtime/query.go, internal/runtime/*_test.go]
    avoided_surfaces: [distributed locking, background services, hosted synchronization]
    waves:
      - wave: 1
        objective: Establish a workspace-local generation/lock protocol around canonical mutation and index publication.
        tasks:
          - task_id: wgc-1
            description: Model the workspace generation and ownership transitions, including dirty-before-write, rebuild start, publish, and recovery boundaries.
            depends_on: canonical-index-binding
            touched_surfaces: [internal/runtime/index.go, internal/runtime/transition.go, internal/runtime/claim_store.go]
            expected_output: A single workspace cannot publish clean state for a generation older than the canonical inputs it observed.
            verify: "go test ./internal/runtime -run '^TestWorkspaceGeneration' -count=1 -v"
            stop_if: The protocol permits a clean marker after an unresolved writer or ambiguous generation.
            escalation: Leave the workspace fail closed and route the race window to trust review.
          - task_id: wgc-2
            description: Add deterministic interleaving tests for mutation during scan, publication, freshness check, and pending-transition recovery.
            depends_on: wgc-1
            touched_surfaces: [internal/runtime/*_test.go]
            expected_output: Stale publication is rejected or marked dirty in every controlled interleaving.
            verify: "go test -race ./internal/runtime -run '^TestWorkspaceGeneration' -count=1 -v"
            stop_if: A race test is flaky, data-racy, or reports clean after a stale publish.
            escalation: Keep the reproducer and do not advance the release phase.
      - wave: 2
        objective: Verify crash-window recovery and preserve existing transition journal semantics.
        tasks:
          - task_id: wgc-3
            description: Extend failure-injection coverage for generation ownership, interrupted publication, and journal recovery.
            depends_on: wgc-2
            touched_surfaces: [internal/runtime/transition_test.go, internal/runtime/index_test.go]
            expected_output: Recovery either completes the exact intended operation or leaves trusted querying blocked.
            verify: "go test ./internal/runtime -run '^(TestPendingTransition|TestWorkspaceGeneration)' -count=1 -v"
            stop_if: Recovery silently discards canonical or journal state.
            escalation: Stop mutation work and preserve the journal fixture for diagnosis.
    checks:
      - command: "go test -race ./internal/runtime -run '^TestWorkspaceGeneration' -count=1 -v"
        proves: Concurrent mutation/publication behavior and race safety.
      - command: "go test ./internal/runtime -run '^(TestPendingTransition|TestWorkspaceGeneration)' -count=1 -v"
        proves: Crash recovery remains fail closed.

  - phase_slug: content-digest-evidence
    story_id: 01KZ962RJRXFSGC7M4SV3AMGHW
    status: planned
    goal: Replace mtime-only freshness assumptions with content-digest verification and bind approved claims to current evidence snapshots.
    depends_on: workspace-generation-coordination
    allowed_surfaces: [internal/runtime/index.go, internal/runtime/evidence.go, internal/runtime/claim_store.go, internal/runtime/query.go, internal/runtime/*_test.go, trusted-memory-spec.md]
    avoided_surfaces: [mutable evidence, query-time full rehash, evidence deletion or repair]
    waves:
      - wave: 1
        objective: Make freshness compare canonical content, not only filesystem metadata, while keeping verification at the rebuild boundary.
        tasks:
          - task_id: cde-1
            description: Implement or extend a workspace-local content manifest/generation digest for canonical claims and evidence metadata/raw inputs.
            depends_on: workspace-generation-coordination
            touched_surfaces: [internal/runtime/index.go, internal/runtime/evidence.go]
            expected_output: Outside content edits are detected even when file mtimes are restored to their previous values.
            verify: "go test ./internal/runtime -run '^TestContentDigestFreshness' -count=1 -v"
            stop_if: Freshness depends on an attacker-controlled mtime or reads outside the selected workspace.
            escalation: Keep the mtime-restoration reproducer and return to the manifest design.
          - task_id: cde-2
            description: Store and verify the evidence digest associated with each approved evidence-backed claim and its supporting closure.
            depends_on: cde-1
            touched_surfaces: [internal/runtime/claim.go, internal/runtime/claim_store.go, internal/runtime/evidence.go, internal/runtime/index.go]
            expected_output: Same-size metadata/raw replacement invalidates the claim and excludes it from trusted results.
            verify: "go test ./internal/runtime -run '^TestEvidenceClaimDigestBinding' -count=1 -v"
            stop_if: An approved claim remains trusted after current evidence bytes or metadata no longer match its approved digest.
            escalation: Preserve canonical evidence and block migration/release work.
      - wave: 2
        objective: Measure the stronger freshness boundary and prevent an integrity regression from hiding in the happy path.
        tasks:
          - task_id: cde-3
            description: Add focused restoration/tamper tests and update the performance benchmark or caching boundary without moving trust into query-time repair.
            depends_on: cde-2
            touched_surfaces: [internal/runtime/*_test.go, docs/acceptance-walkthrough.md, docs/release.md]
            expected_output: Freshness and evidence regressions are reproducible and the 100k query p95 remains at or below two seconds.
            verify: "ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v"
            stop_if: p95 exceeds two seconds or the optimization weakens a trust assertion.
            escalation: Record the measured regression and defer release proof.
    checks:
      - command: "go test ./internal/runtime -run '^(TestContentDigestFreshness|TestEvidenceClaimDigestBinding)' -count=1 -v"
        proves: Mtime restoration, content tamper, and evidence replacement fail closed.
      - command: "ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v"
        proves: The scale gate remains at or below two seconds.

  - phase_slug: legacy-migration-reporting
    story_id: 01KZ962RJY3381D5X3MFYRPGTS
    status: planned
    goal: Provide explicit legacy claim migration and reapproval behavior and make migrate okf freshness reporting truthful.
    depends_on: content-digest-evidence
    allowed_surfaces: [internal/runtime/migrate.go, internal/runtime/claim_store.go, internal/cli/cli.go, internal/runtime/*_test.go, trusted-memory-spec.md]
    avoided_surfaces: [implicit trust preservation, new lifecycle states, automatic approval]
    waves:
      - wave: 1
        objective: Trace legacy document shapes and make the migration/reapproval boundary explicit.
        tasks:
          - task_id: lmr-1
            description: Add fixtures for legacy approved documents with and without OKF verification metadata and define their post-migration status/output.
            depends_on: content-digest-evidence
            touched_surfaces: [internal/runtime/*_test.go, internal/runtime/migrate.go]
            expected_output: Legacy documents without a valid digest remain outside trusted results until normal approval completes.
            verify: "go test ./internal/runtime -run '^TestLegacyMigration' -count=1 -v"
            stop_if: Migration silently creates an approved trusted claim without explicit verification.
            escalation: Stop and preserve the legacy fixture as a release blocker.
          - task_id: lmr-2
            description: Implement the smallest supported migration/reapproval path using existing claim commands and preserve canonical history.
            depends_on: lmr-1
            touched_surfaces: [internal/runtime/migrate.go, internal/runtime/claim_store.go, internal/cli/cli.go]
            expected_output: Operators can migrate, inspect, and explicitly approve a legacy claim without new lifecycle states or canonical deletion.
            verify: "go test ./internal/runtime ./internal/cli -run '^TestLegacyMigration' -count=1 -v"
            stop_if: The path requires an undocumented command or mutates unrelated workspace content.
            escalation: Return to scope lock before adding a command.
      - wave: 2
        objective: Make migration output reflect actual index freshness and rejected state.
        tasks:
          - task_id: lmr-3
            description: Fix migrate okf reporting to perform or surface a real freshness check and add CLI JSON regression coverage.
            depends_on: lmr-2
            touched_surfaces: [internal/cli/cli.go, internal/runtime/index.go, internal/runtime/*_test.go]
            expected_output: No-op migration cannot report index_fresh true without a verified current index.
            verify: "go test ./internal/cli ./internal/runtime -run '^TestMigrateOKFFreshness' -count=1 -v"
            stop_if: Output claims freshness from migration counts alone.
            escalation: Mark the migration phase blocked and retain the failing JSON fixture.
    checks:
      - command: "go test ./internal/runtime ./internal/cli -run '^(TestLegacyMigration|TestMigrateOKFFreshness)' -count=1 -v"
        proves: Legacy trust transition and truthful freshness reporting.
      - command: "go run ./cmd/zbrain migrate okf --help"
        proves: The documented migration surface remains discoverable without adding an unapproved command family.

  - phase_slug: cli-assets-parity
    story_id: 01KZ962RK5ZCWFRS5YZSWDAC34
    status: planned
    goal: Align runtime asset extraction, documentation, embedded guidance, CLI help, supported flags, and unknown-flag handling.
    depends_on: none
    allowed_surfaces: [internal/runtime/assets.go, assets/embed.go, assets/*, internal/cli/cli.go, internal/cli/*_test.go, CLAUDE.md, trusted-memory-spec.md, docs/acceptance-walkthrough.md, docs/release.md]
    avoided_surfaces: [new commands, alternative asset frameworks, unrelated active plans]
    waves:
      - wave: 1
        objective: Make extracted runtime layout and operator documentation describe the same paths.
        tasks:
          - task_id: cap-1
            description: Compare embedded paths, extraction destinations, setup output, README, and runtime layout docs; choose one root-level contract and update only contradictory references.
            depends_on: none
            touched_surfaces: [internal/runtime/assets.go, assets/embed.go, CLAUDE.md, trusted-memory-spec.md, docs/acceptance-walkthrough.md, docs/release.md]
            expected_output: A fresh isolated setup produces exactly the documented assets, indexes, and workspace paths.
            verify: "go test ./internal/runtime -run '^TestEmbeddedAssetLayout' -count=1 -v && make smoke"
            stop_if: Docs and extractor still disagree or setup touches the operator's real home.
            escalation: Keep the isolated extraction fixture and stop asset work.
          - task_id: cap-2
            description: Add an asset parity test that enumerates embedded active skills/agents/templates and rejects missing, duplicate, or stale documented entries.
            depends_on: cap-1
            touched_surfaces: [internal/runtime/*_test.go, assets/*]
            expected_output: Runtime asset drift fails in repository tests before release.
            verify: "go test ./internal/runtime -run '^TestEmbeddedAssetParity' -count=1 -v"
            stop_if: The test requires network access or reads user runtime data.
            escalation: Reduce the test to embedded source assets and document the gap.
      - wave: 2
        objective: Make help and parser behavior authoritative for every shipped command.
        tasks:
          - task_id: cap-3
            description: Inventory supported flags, add missing help entries, and reject unknown flags without changing valid command semantics.
            depends_on: cap-1
            touched_surfaces: [internal/cli/cli.go, internal/cli/*_test.go, trusted-memory-spec.md]
            expected_output: `zbrain --help` and subcommand help cover supported scope/format flags, and unknown flags return an error.
            verify: "go test ./internal/cli -run '^(TestCLIHelpParity|TestUnknownFlag)' -count=1 -v && go run ./cmd/zbrain --help"
            stop_if: A valid documented flag is rejected or an unknown flag is silently accepted.
            escalation: Preserve the parser reproducer and stop skill updates.
    checks:
      - command: "go test ./internal/runtime ./internal/cli -run '^(TestEmbeddedAsset(Layout|Parity)|TestCLIHelpParity|TestUnknownFlag)' -count=1 -v"
        proves: Asset extraction/parity and CLI help/parser agreement.
      - command: "make smoke"
        proves: The documented setup/runtime layout works in isolation.

  - phase_slug: shell-safe-scope-guidance
    story_id: 01KZ962RKBH5ZBEDGYPP4AG0G2
    status: planned
    goal: Make active Claude Code skills shell-safe, workspace-explicit, and free of executable network or stale legacy workflows.
    depends_on: cli-assets-parity
    allowed_surfaces: [assets/skills/, assets/agents/, assets/engine/, assets/templates/, internal/runtime/*_test.go, CLAUDE.md, trusted-memory-spec.md]
    avoided_surfaces: [network research, executable fetch scripts, deleted commands, hidden workspace expansion]
    waves:
      - wave: 1
        objective: Remove unsafe interpolation patterns and make user-controlled values use safe argv/quoting guidance.
        tasks:
          - task_id: ssg-1
            description: Audit active skill/agent command examples for shell interpolation, unsafe redirects, and implicit workspace/include expansion; rewrite to safe forms.
            depends_on: cli-assets-parity
            touched_surfaces: [assets/skills/, assets/agents/, assets/engine/]
            expected_output: Values containing spaces, quotes, dollar signs, backticks, and newlines cannot become shell source in active guidance.
            verify: "go test ./internal/runtime -run '^TestSkillShellSafety' -count=1 -v"
            stop_if: An active example still interpolates untrusted text into a shell command.
            escalation: Remove the example from active assets until it can be expressed safely.
          - task_id: ssg-2
            description: Make workspace-only defaults and explicit `--workspace`/`--include` behavior consistent across skills, agents, and command docs.
            depends_on: ssg-1
            touched_surfaces: [assets/skills/, assets/agents/, assets/engine/, CLAUDE.md]
            expected_output: Guidance never broadens the read scope without explicit operator consent.
            verify: "go test ./internal/runtime ./internal/cli -run '^TestSkillWorkspaceScope' -count=1 -v"
            stop_if: A skill can read another workspace by default or names an unsupported scope flag.
            escalation: Preserve the conflicting asset and route to scope review.
      - wave: 2
        objective: Keep only supported active workflows in the embedded runtime content.
        tasks:
          - task_id: ssg-3
            description: Remove executable network-fetch guidance and isolate or delete stale QA/apply/deleted-command assets according to the non-goals contract.
            depends_on: ssg-2
            touched_surfaces: [assets/skills/, assets/agents/, assets/engine/, assets/templates/]
            expected_output: Active embedded assets contain no network research or obsolete command workflow, with any retained legacy material clearly labeled and non-active.
            verify: "go test ./internal/runtime -run '^TestActiveAssetScope' -count=1 -v"
            stop_if: Runtime setup still installs an executable out-of-scope workflow as active guidance.
            escalation: Stop and ask for a new scope decision rather than hiding the asset.
    checks:
      - command: "go test ./internal/runtime ./internal/cli -run '^(TestSkillShellSafety|TestSkillWorkspaceScope|TestActiveAssetScope)' -count=1 -v"
        proves: Shell safety, explicit workspace scope, and active asset boundaries.
      - command: "git diff --check"
        proves: Skill/documentation edits are cleanly reviewable.

  - phase_slug: runtime-boundary-compatibility
    story_id: 01KZ962RKHJ5XEDRZXHC44KDM8
    status: planned
    goal: Unify nested claim lookup, workspace boundaries, runtime permissions, and workspace current compatibility behavior with focused tests.
    depends_on: content-digest-evidence
    allowed_surfaces: [internal/runtime/claim_store.go, internal/runtime/paths.go, internal/runtime/evidence.go, internal/runtime/config.go, internal/cli/cli.go, internal/runtime/*_test.go, CLAUDE.md, trusted-memory-spec.md]
    avoided_surfaces: [cross-workspace reads, user runtime migrations without authorization, compatibility shims outside the current command contract]
    waves:
      - wave: 1
        objective: Make canonical path resolution and lifecycle lookup agree for nested workspace content.
        tasks:
          - task_id: rbc-1
            description: Add nested-path fixtures and verify claim identity lookup, lifecycle mutation, conflict detection, and index exclusion all resolve the same workspace-local canonical document.
            depends_on: content-digest-evidence
            touched_surfaces: [internal/runtime/claim_store.go, internal/runtime/paths.go, internal/runtime/*_test.go]
            expected_output: Nested claims cannot be indexed but missed by approve/supersede/revoke or resolved outside the workspace.
            verify: "go test ./internal/runtime -run '^TestNestedClaimBoundary' -count=1 -v"
            stop_if: Any lifecycle operation follows a path outside the selected workspace or disagrees with query/index identity.
            escalation: Preserve the nested fixture and block boundary changes.
          - task_id: rbc-2
            description: Document and test the minimum owner-only permissions for runtime metadata, canonical evidence, and derived indexes without changing canonical content semantics.
            depends_on: rbc-1
            touched_surfaces: [internal/runtime/*, internal/runtime/*_test.go, CLAUDE.md, trusted-memory-spec.md]
            expected_output: Fresh setup and mutation outputs have the documented permission contract and remain usable by the owning operator.
            verify: "go test ./internal/runtime -run '^TestRuntimePermissions' -count=1 -v"
            stop_if: Permissions expose secrets broadly or make isolated normal use impossible without an approved compatibility decision.
            escalation: Record the platform-specific gap and defer tightening rather than guessing.
      - wave: 2
        objective: Resolve retained workspace-current compatibility behavior explicitly.
        tasks:
          - task_id: rbc-3
            description: Test `workspace current` JSON against the documented retained/removed context-file behavior and remove only unsupported stale concepts.
            depends_on: rbc-2
            touched_surfaces: [internal/cli/cli.go, internal/runtime/config.go, internal/cli/*_test.go, CLAUDE.md]
            expected_output: Current-workspace output has one stable documented shape and does not imply unsupported session transcript storage.
            verify: "go test ./internal/cli -run '^TestWorkspaceCurrentContract' -count=1 -v"
            stop_if: The output change breaks the shipped command contract without a migration or scope decision.
            escalation: Keep the existing behavior and document it as an intentional compatibility surface.
    checks:
      - command: "go test ./internal/runtime ./internal/cli -run '^(TestNestedClaimBoundary|TestRuntimePermissions|TestWorkspaceCurrentContract)' -count=1 -v"
        proves: Nested path, permission, and workspace-current contracts.
      - command: "go test -race ./internal/runtime ./internal/cli"
        proves: Boundary changes remain race-safe with existing runtime behavior.

  - phase_slug: release-proof
    story_id: 01KZ962RKRS5N6B1C48FB0HHVV
    status: planned
    goal: Run the complete regression, race, build, isolated smoke, performance, documentation, and release-proof gate for the initiative.
    depends_on: runtime-boundary-compatibility
    gate_prerequisites: [legacy-migration-reporting, shell-safe-scope-guidance]
    allowed_surfaces: [go tests, docs/acceptance-walkthrough.md, docs/release.md, CLAUDE.md]
    avoided_surfaces: [new implementation scope, force push, real user runtime data]
    waves:
      - wave: 1
        objective: Reconcile operator-facing docs and run repository-wide static checks after all implementation phases are checked.
        tasks:
          - task_id: rlp-1
            description: Update acceptance/release documentation only where the final command, asset, trust, permission, or freshness behavior differs from the locked contract.
            depends_on: [legacy-migration-reporting, shell-safe-scope-guidance, runtime-boundary-compatibility]
            touched_surfaces: [CLAUDE.md, trusted-memory-spec.md, docs/acceptance-walkthrough.md, docs/release.md]
            expected_output: Docs name only shipped commands and reproduce the isolated proof path.
            verify: "go run ./cmd/zbrain --help && git diff --check"
            stop_if: Documentation requires an unimplemented command or external service.
            escalation: Return the documentation change to the owning phase.
      - wave: 2
        objective: Execute all release gates without touching the operator's normal runtime.
        tasks:
          - task_id: rlp-2
            description: Run the complete Go, vet, race, build, smoke, and benchmark suite and capture exact outputs in the validation record.
            depends_on: rlp-1
            touched_surfaces: [dist/zbrain, isolated ZBRAIN_HOME only]
            expected_output: All required checks pass, smoke reports ready trusted querying, and benchmark p95 is at or below two seconds.
            verify: "go test ./... && go vet ./... && go test -race ./internal/runtime ./internal/cli && make build && make smoke && ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v"
            stop_if: Any trust, boundary, race, build, smoke, or performance gate fails.
            escalation: Do not publish; record the exact failure and route to its owning phase.
          - task_id: rlp-3
            description: Confirm no unrelated active plan or user runtime data changed and prepare the durable handoff for the next machine.
            depends_on: rlp-2
            touched_surfaces: [docs/plans/active/trusted-memory-hardening.md, git status only]
            expected_output: The plan is complete, the next action is the first work phase, and only authorized repository files are changed.
            verify: "git status --short && git diff --check && zharness query phases --json"
            stop_if: The phase DB state, plan state, or working tree contains an unexplained change.
            escalation: Stop before commit and inspect the target before any cleanup.
    checks:
      - command: "go test ./... && go vet ./... && go test -race ./internal/runtime ./internal/cli"
        proves: Full correctness, static analysis, and race gates.
      - command: "make build && make smoke"
        proves: Build and isolated Go-native runtime acceptance.
      - command: "ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v"
        proves: The 100k-claim query p95 release gate.
      - command: "git diff --check && zharness query phases --json"
        proves: Clean durable planning state and phase/database agreement.

## Progress
<!-- Append-only durable entries record timestamp, phase, wave, task, task_status, run_id, trace_id, exact verification/result, and changed surfaces or blocker. -->
- timestamp: 2026-08-05T14:45:20Z
  phase: canonical-index-binding
  wave: phase-start
  task: phase-start
  task_status: in-progress
  run_id: 01KZ9659369MJDN0GWPB1QMH2Z
  trace_id: none
  verification: not-run
  changed_surfaces: []
  blocker: none
- timestamp: 2026-08-05T15:05:23Z
  phase: canonical-index-binding
  wave: 1
  task: cib-1
  task_status: DONE
  run_id: 01KZ9659369MJDN0GWPB1QMH2Z
  trace_id: none
  exact_verification: "TMPDIR=/private/tmp/zbrain-safe go test ./internal/runtime -run '^TestCanonicalIndexBinding$' -count=1 -v -> pass; indexed rows carry body, tags, and verification digest, and TrustedQuery validates canonical bindings before mapping results"
  changed_surfaces: [internal/runtime/index.go, internal/runtime/query.go]
  blocker: none
- timestamp: 2026-08-05T15:05:23Z
  phase: canonical-index-binding
  wave: 1
  task: cib-2
  task_status: DONE
  run_id: 01KZ9659369MJDN0GWPB1QMH2Z
  trace_id: 01KZ97BER63HT2S0GJ4FB7EYWZ
  exact_verification: "TMPDIR=/private/tmp/zbrain-safe go test ./internal/runtime -run '^TestCanonicalIndexBinding' -count=1 -v -> pass; body, status, path, digest, and missing-target SQLite mutations fail closed while canonical bytes remain unchanged"
  changed_surfaces: [internal/runtime/query_test.go]
  blocker: none

## Decisions
<!-- Append-only durable entries record timestamp, phase/task, decision, and rationale. -->
- none

## Validation
<!-- Append-only durable entries record timestamp, phase, exact command/result/output, run_id, check_id, verdict, and proof_gaps. -->
- timestamp: 2026-08-05T15:09:25Z
  phase: canonical-index-binding
  commands:
    - TMPDIR=/private/tmp/zbrain-safe go test ./internal/runtime -run '^TestCanonicalIndexBinding$' -count=1 -v -> pass; red run before implementation failed body/status/path cases and digest column was absent
    - TMPDIR=/private/tmp/zbrain-safe go test ./internal/runtime -run '^TestCanonicalIndexBinding' -count=1 -v -> pass; five SQLite-only tamper cases fail closed and canonical bytes remain unchanged
    - TMPDIR=/private/tmp/zbrain-safe go test ./... -> pass
    - TMPDIR=/private/tmp/zbrain-safe go vet ./... -> pass
    - TMPDIR=/private/tmp/zbrain-safe go test -race ./... -> pass
    - make build -> pass
    - make smoke -> pass; isolated setup, workspace creation, claim approval, reindex, and trusted ask completed
    - git diff --check -> pass
    - zharness audit --json -> pointer drift before new check; resolved by this check record
  run_id: 01KZ9659369MJDN0GWPB1QMH2Z
  check_id: 01KZ97H6KYTC027KZ470B8XT3V
  verdict: APPROVED
  proof_gaps: same-session review; default macOS TMPDIR test invocation remains blocked by the existing /var/folders symlink boundary guard, while the canonical safe-temp invocation passes

## Current State and Next Action
- active_phase: canonical-index-binding
- lifecycle_status: checked
- latest_run_id: 01KZ9659369MJDN0GWPB1QMH2Z
- latest_trace_ids: [01KZ97BER63HT2S0GJ4FB7EYWZ]
- latest_check_id: 01KZ97H6KYTC027KZ470B8XT3V
- latest_handoff_id: 01KZ8QK3J8Q2T5PF0VAFRDH1QM
- blockers: none
- open_items: [execute the planned trust and skills phases; preserve all 8 story IDs; release proof remains gated on every prerequisite phase]
- exact_next_action: handoff
