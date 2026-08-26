---
id: 01KZ1HB7RGY7WNJGJ9YV6580VA
type: plan
intake_id: 01KZ1HB7S2399Y4KKACC0YYGRK
lane: high-risk
status: abandoned
created: 2026-08-02
updated: 2026-08-26
---

# Plan: Trusted single runtime

## Outcome
- result: The Go build is the only zbrain on the machine, and `zbrain ask` never hands an agent an approved claim whose file content no longer matches the digest recorded when it was approved.
- success_signals:
  - Editing an approved claim's body by hand and running `zbrain ask` no longer returns that claim with `status: ready`; the tampered claim is excluded and the response fails closed.
  - `zbrain reindex` classifies a digest-mismatched approved claim under `invalid` and names its claim ID and path, not under `approved`.
  - Creating or editing any wiki markdown file outside the CLI makes the next `zbrain ask` fail closed with the existing dirty-index error until `zbrain reindex` runs.
  - `command -v zbrain` resolves to the Go build; no Bun-compiled zbrain binary remains on PATH.
  - No MCP server entry named `zbrain` remains in the user's Claude configuration.
  - `go test ./...` passes with regression tests that reproduce the tamper and outside-edit scenarios recorded in the 2026-08-02 review.

## Authority and Requirements
- authority:
  - Owner instruction (2026-08-02): lock scope to review items A and F; drop MCP in favour of CLI-only access; discard the legacy runtime data rather than migrating it.
  - `trusted-memory-spec.md` §6 — approved claims are tied to the current canonical verification digest; §8 — dirty, missing, stale, or rejected indexes fail closed; §9 — trusted queries expose gaps instead of silently promoting untrusted content.
  - `README.md` "OKF trusted memory model" — only approved claim concepts are trusted by `zbrain ask`.
  - `CLAUDE.md` — keep the implementation Go-native and minimal; do not reintroduce Bun, Node, or TypeScript.
  - `internal/runtime/claim.go:304` `ClaimVerificationDigest` has exactly one caller, `internal/runtime/claim_store.go:93` (approve); no read path recomputes it.
  - `internal/runtime/index.go:77` `CheckFresh` consults only the `.dirty` marker and database existence, so edits made outside the CLI never invalidate the index.
  - Verified live state (2026-08-02): `~/.local/bin/zbrain` is a 64 MB Bun build dated 7 Jul; `~/.zbrain/zbrain.db` reports `user_version=2` and holds `notes`/`links`/`leases` tables the Go code never reads.
- requirements:
  - R1 [accepted]: `zbrain reindex` recomputes the verification digest of every approved claim it scans and treats a mismatch as invalid, so the claim never enters the FTS index. | source: `trusted-memory-spec.md` §6, `claim.go:304`
  - R2 [accepted]: `zbrain ask` returns no claim whose on-disk content disagrees with its recorded `verified.digest`, for every edit detectable under R1 or R3; the mtime-preserving case is carved out by NG8. | source: owner instruction, `README.md` trusted memory model
  - R3 [accepted]: index freshness accounts for wiki files changed outside the CLI; `zbrain ask` fails closed when any wiki markdown file is newer than the index it would query. | source: `trusted-memory-spec.md` §8, `index.go:77`
  - R4 [accepted]: a digest mismatch is reported with the offending claim ID and path in `reindex` output rather than silently dropped. | source: `trusted-memory-spec.md` §8
  - R5 [accepted]: the Bun-compiled binary at `~/.local/bin/zbrain` is removed from PATH using `trash`, and the Go build is installed in its place. | source: owner instruction, `CLAUDE.md`
  - R6 [accepted]: no MCP server configuration referencing the retired binary remains in the user's Claude configuration. | source: owner instruction
  - R7 [accepted]: the legacy `~/.zbrain` runtime is inventoried in writing, moved to trash, and replaced by a clean `zbrain setup`. | source: owner instruction
  - R8 [accepted]: every `assets/skills/zbrain-*/SKILL.md` describes only commands the Go CLI actually ships; no skill references an MCP tool. | source: owner instruction, `README.md` command surface
  - R9 [accepted]: the command surface listed in `CLAUDE.md` and `README.md` matches `zbrain --help` exactly. | source: `CLAUDE.md`
  - R10 [accepted]: `go test ./...` passes, including new regression tests for the tamper path (R1/R2) and the outside-edit path (R3). | source: `CLAUDE.md` implementation rules

## Non-goals
- NG1: returning claim bodies or snippets from `zbrain ask`, or adding `claim show` / `claim list` (review item B) — deferred to a later initiative.
- NG2: slug-based filenames or generated per-tier index pages (review item C) — deferred.
- NG3: draft revision, delete, or archive commands (review item D) — deferred.
- NG4: author attribution; `generated.by` and `verified.by` stay hardcoded to `owner` (review item E) — deferred.
- NG5: porting an MCP server to Go. Agents reach zbrain only through skills that shell out to the CLI.
- NG6: migrating legacy v1 notes or evidence into the OKF model; the owner chose discard over migration.
- NG7: git-backed versioning, sync, or backup of `~/.zbrain` (review item 9).
- NG8: defending against an edit that preserves file mtime and is never reindexed; accepted residual risk for a local-first single-user vault.
- NG9: enforcing `stale_after` at query time (review item 12).
- NG10: deleting the unreferenced `internal/runtime/search.go` search path; separate cleanup.
- NG11: normalizing the `null` versus `[]` inconsistency in `zbrain ask` JSON output (review item 3).

## Approach and Risks
- approach: Verify trust at the index boundary rather than per query. `ClaimStore.ScanWorkspace` already parses every claim, so recomputing `ClaimVerificationDigest` there costs one hash per claim and lets a mismatched approved claim fall into the existing `ClaimScan.Invalid` bucket, which `IndexStore.Rebuild` already counts and excludes from the FTS table. Freshness uses the index database's own modification time as the watermark: any wiki markdown newer than it means the index no longer describes the tree, so `CheckFresh` fails closed exactly as the existing dirty marker does. Together these close both tamper paths without adding per-query file I/O, new state files, or a new command. Runtime consolidation runs last and only after the Go build is proven, so the irreversible steps never remove the fallback before the replacement is verified.
- rejected_alternatives:
  - Recompute the digest inside `TrustedQuery` per returned claim: forces `ask` to reopen and reparse every hit on every query, duplicates parsing already done at index time, and buys nothing because any content edit also moves the file's mtime.
  - A dedicated `zbrain verify` or `zbrain doctor` command: manual invocation reproduces the failure the 2026-08-02 review found, where a degraded runtime sat unnoticed because nothing ran the check.
  - Port the MCP server to Go in this initiative: requires redesigning the tool-to-claim mapping and an agent write path, which is a larger product decision than this initiative's trust fix.
  - Rename and keep the Bun binary alongside the Go build: preserves the two-runtime drift that this initiative exists to remove.
  - Write `zbrain migrate legacy` for the two legacy artifacts in `~/.zbrain`: a full phase of conversion code and tests for two files the owner chose to discard.
- constraints:
  - Go-native only; no Bun, Node, or TypeScript reintroduced (`CLAUDE.md`).
  - Markdown stays canonical and SQLite stays a disposable derived cache (`trusted-memory-spec.md` §4); no trust state may live only in the database.
  - Deletions use `trash`, never `rm` (`CLAUDE.md` implementation rules).
  - Every behavior change ships with tests, proven by `go test ./...` plus an isolated `ZBRAIN_HOME` smoke (`CLAUDE.md`).
  - No new Go module dependencies; digest work reuses `crypto/sha256` and the existing `ClaimVerificationDigest`.
- risks:
  - Digest recomputation depends on `ParseClaimMarkdown` and `RenderClaimMarkdown` round-tripping identically. If they diverge, every previously approved claim is reported invalid at once. Mitigation: phase `digest-verification` asserts round-trip on an approved fixture before wiring the check into the scan. Detection: `reindex` reports a nonzero `invalid` count for claims that were never edited. Recovery: revert the scan wiring; the index falls back to today's behavior. Blast radius is currently nil because no Go-created claim exists in the live runtime.
  - The digest covers only frontmatter fields modeled by `claimFrontmatter`; unmodeled keys are dropped at render and are therefore invisible to it. Accepted: every field that reaches an agent (`title`, `description`, `tags`, `body`, `sources`) is modeled and covered.
  - Walking the wiki tree on every `CheckFresh` may regress the `trusted-memory-spec.md` §11 release gate of p95 under 2 seconds at 100k claims. Mitigation: `internal/runtime/index_benchmark_test.go` runs as a phase check. Stop condition: if the gate fails, stop and reduce the walk to a recorded maximum-mtime watermark before continuing.
  - `retire-legacy-runtime` is irreversible on the owner's machine and removes the only binary that can read the legacy data. Mitigation: the phase cannot start until the three prior phases are `checked`; it writes a full inventory before touching anything and leaves trashed items in Trash until the fresh runtime is verified. Stop condition: any inventory entry not accounted for by the 2026-08-02 review halts the phase for owner re-confirmation.
  - Removing `.mcpServers.zbrain` edits the owner's global `~/.claude.json`. Mitigation: edit only that one key, back the file up in the same phase inventory, and confirm with the owner before writing.
- recovery: Phases 1 through 3 are ordinary Go and documentation changes revertable with `git revert`. Phase 4 is recovered by restoring the trashed binary and `~/.zbrain` from Trash and re-adding the `.mcpServers.zbrain` key from the inventory backup.

## Phases and Verification
<!-- Phase and task definitions are immutable after to-plan. Do not add task status fields. Append-only Progress is the sole task execution-status source. Only each phase lifecycle status changes to mirror DB transitions: to-plan=planned; work after run create=in-progress; clean durable check=checked; closing handoff=done. Each planned phase records phase_slug, story_id, status, goal, depends_on, waves, tasks, and checks. -->
- planning_status: planned
- phases:
  - phase_slug: digest-verification
    story_id: 01KZ1HGH2MQHG7BMHK6WE6BVA0
    status: done
    goal: Reject approved claims whose on-disk content no longer matches their recorded verification digest, and name them in reindex output.
    depends_on: none
    requirements: [R1, R2, R4, R10]
    allowed_surfaces: [internal/runtime/claim.go, internal/runtime/claim_store.go, internal/runtime/index.go, internal/cli/cli.go, internal/runtime/claim_test.go, internal/runtime/claim_store_test.go, internal/runtime/index_test.go, internal/cli/cli_test.go]
    avoided_surfaces: [assets/, internal/runtime/evidence.go, internal/runtime/query.go, internal/runtime/search.go, ~/.zbrain, ~/.local/bin]
    waves:
      - wave: W1
        goal: Prove the digest round-trips before anything depends on it.
        tasks:
          - id: W1.T1
            task: Add a table test in `internal/runtime/claim_test.go` that renders an approved claim, parses it back, recomputes `ClaimVerificationDigest`, and asserts it equals the stored digest.
            depends_on: none
            expected_output: A passing test proving parse/render round-trip stability for an approved claim.
      - wave: W2
        goal: Turn a digest mismatch into an invalid claim that never reaches the index.
        tasks:
          - id: W2.T1
            task: Add `VerifyClaimDigest(claim Claim) error` to `internal/runtime/claim.go`, returning an error when `claim.Status` is `approved` and either `VerifiedDigest` is empty or differs from the recomputed digest, and returning nil for every other status.
            depends_on: W1.T1
            expected_output: New exported function plus unit tests for tampered body, tampered title, missing digest on an approved claim, and skipped non-approved statuses.
          - id: W2.T2
            task: Call `VerifyClaimDigest` in `ClaimStore.ScanWorkspace` after `ParseClaimMarkdown` succeeds, appending failures to `ClaimScan.Invalid` instead of `ClaimScan.Claims`.
            depends_on: W2.T1
            expected_output: A tampered approved claim is counted in `IndexSummary.Invalid`, excluded from the FTS table, and therefore absent from `zbrain ask`.
      - wave: W3
        goal: Make the rejection legible instead of a bare count.
        tasks:
          - id: W3.T1
            task: Add JSON tags to `InvalidClaim` and surface the invalid claim list (path plus error) in the `zbrain reindex` response in `internal/cli/cli.go`.
            depends_on: W2.T2
            expected_output: `zbrain reindex` JSON names each rejected claim's path and reason alongside the existing counts.
    checks:
      - command: go test ./...
        expects: All packages pass, including the new claim and index tests.
      - command: 'ZBRAIN_HOME=$SMOKE ./dist/zbrain reindex'
        expects: 'After approving a claim and rewriting its body by hand, output reports `invalid` of 1, `approved` of 0, and names the tampered claim path.'
      - command: 'ZBRAIN_HOME=$SMOKE ./dist/zbrain ask "markdown canonical"'
        expects: '`status` is `gap` and the tampered claim is absent from `claims`, reproducing the 2026-08-02 finding as a now-failing attack.'
    stop_condition: Round-trip test in W1.T1 fails, meaning parse/render is not digest-stable; stop before wiring W2.T2 and escalate.
    escalation: Report to the owner with the failing fixture; do not weaken the digest to make the test pass.

  - phase_slug: index-staleness
    story_id: 01KZ1HGH2WH622QH6MQHDAQXVC
    status: done
    goal: Fail retrieval closed when wiki markdown changes outside the CLI.
    depends_on: digest-verification
    requirements: [R2, R3, R10]
    allowed_surfaces: [internal/runtime/index.go, internal/runtime/index_test.go, internal/runtime/index_benchmark_test.go, internal/cli/cli_test.go]
    avoided_surfaces: [internal/runtime/claim.go, internal/runtime/claim_store.go, assets/, ~/.zbrain, ~/.local/bin]
    waves:
      - wave: W1
        goal: Give the index a trustworthy watermark.
        tasks:
          - id: W1.T1
            task: At the end of `IndexStore.Rebuild`, after the rename that publishes the database, set the database file's modification time to the current time so the watermark is never older than the scan that produced it.
            depends_on: none
            expected_output: The published index file's mtime reliably postdates every file it indexed.
      - wave: W2
        goal: Fail closed on any outside edit.
        tasks:
          - id: W2.T1
            task: Extend `IndexStore.CheckFresh` to walk the workspace wiki directory and return an error when any `.md` file's modification time is newer than the index database's, keeping the existing dirty-marker and missing-database checks ahead of it.
            depends_on: W1.T1
            expected_output: `zbrain ask` refuses to answer from an index that no longer describes the tree.
          - id: W2.T2
            task: Make that error name the first offending file path and instruct the reader to run `zbrain reindex`, matching the wording style of the existing dirty-index error.
            depends_on: W2.T1
            expected_output: The failure tells the operator which file caused it and how to recover.
          - id: W2.T3
            task: Add tests covering an edited existing claim, a hand-authored new markdown file, and the clean case after `reindex`.
            depends_on: W2.T2
            expected_output: Regression coverage for the outside-edit path from the 2026-08-02 review.
    checks:
      - command: go test ./...
        expects: All packages pass, including the new staleness tests.
      - command: go test ./internal/runtime/ -bench . -benchtime 1x -run '^$'
        expects: The 100k-claim query benchmark still satisfies the `trusted-memory-spec.md` §11 gate of p95 under 2 seconds.
      - command: 'ZBRAIN_HOME=$SMOKE ./dist/zbrain ask "markdown canonical"'
        expects: 'After hand-writing a new markdown file into the wiki, the command exits nonzero naming that file; after `zbrain reindex` it returns `status` of `ready` again.'
    stop_condition: The benchmark check breaches the 2-second gate.
    escalation: Stop and replace the tree walk with a recorded maximum-mtime watermark before proceeding; do not ship a regression against the published release gate.

  - phase_slug: docs-and-skills
    story_id: 01KZ1HGH33F6V1CGFNM7XPY6JT
    status: done
    goal: Align shipped docs and skills with the real Go command surface and remove MCP references.
    depends_on: index-staleness
    requirements: [R8, R9]
    allowed_surfaces: [CLAUDE.md, README.md, trusted-memory-spec.md, assets/skills/, assets/engine/]
    avoided_surfaces: [internal/, cmd/, ~/.zbrain, ~/.local/bin]
    waves:
      - wave: W1
        goal: Make every shipped description match the binary.
        tasks:
          - id: W1.T1
            task: Replace the stale command list in the `CLAUDE.md` "Current direction" section with the full surface printed by `zbrain --help`, which today omits `evidence`, `claim`, `migrate`, and `reindex`.
            depends_on: none
            expected_output: `CLAUDE.md` no longer describes a four-command binary.
          - id: W1.T2
            task: Update `trusted-memory-spec.md` §8 and `README.md` to state that reindex rejects digest-mismatched claims and that retrieval fails closed on outside edits.
            depends_on: none
            expected_output: The spec describes the trust behavior the code now enforces.
          - id: W1.T3
            task: Read every `assets/skills/zbrain-*/SKILL.md` and `assets/engine/*.md`, and correct any instruction that names a command, tool, or file the Go build does not ship.
            depends_on: none
            expected_output: No shipped skill or engine rule instructs an agent toward a surface that does not exist.
    checks:
      - command: 'grep -rhoE ''\bzbrain [a-z-]+'' assets/ CLAUDE.md README.md trusted-memory-spec.md | sort -u'
        expects: Every command token in the output appears in `zbrain --help`.
      - command: grep -rniE 'mcp|qmd|bun|typescript' assets/ CLAUDE.md README.md trusted-memory-spec.md
        expects: No hit instructs an agent to use a retired surface; remaining hits are historical prose only.
      - command: go test ./...
        expects: Still passing; documentation changes must not touch behavior.
    stop_condition: A skill turns out to depend on a command the Go build genuinely lacks.
    escalation: Record the gap and route it to a new initiative rather than adding product behavior inside this one.

  - phase_slug: retire-legacy-runtime
    story_id: 01KZ1HGH394B5FB0RB5KS1DF91
    status: planned
    goal: Trash the Bun binary and legacy runtime, install the Go build, and re-setup a clean runtime.
    depends_on: docs-and-skills
    requirements: [R5, R6, R7]
    allowed_surfaces: ['~/.local/bin/zbrain', '~/.zbrain', '~/.claude.json (only the .mcpServers.zbrain key)', dist/]
    avoided_surfaces: [internal/, cmd/, assets/, docs/ except this plan's Progress section]
    waves:
      - wave: W1
        goal: Write down what is about to be destroyed, before destroying it.
        tasks:
          - id: W1.T1
            task: Write an inventory covering every file under `~/.zbrain`, the `~/.local/bin/zbrain` binary with its size and modification time, and the current `.mcpServers.zbrain` JSON value, into the run artifact for this phase.
            depends_on: none
            expected_output: A written record sufficient to reconstruct what existed, satisfying R7's inventory requirement.
          - id: W1.T2
            task: Compare the inventory against the artifacts the 2026-08-02 review accounted for, namely one legacy wiki note, one legacy evidence source directory, the V2 `zbrain.db`, and the extracted asset directories.
            depends_on: W1.T1
            expected_output: Either a clean match, or a halt for owner re-confirmation.
      - wave: W2
        goal: Put the Go build in place before removing the fallback's data.
        tasks:
          - id: W2.T1
            task: Build `dist/zbrain` from source, `trash` the Bun binary at `~/.local/bin/zbrain`, and install the Go build in its place.
            depends_on: W1.T2
            expected_output: The only zbrain on PATH is the Go build.
          - id: W2.T2
            task: Remove the `.mcpServers.zbrain` key from `~/.claude.json`, backing the original value up in the phase inventory first.
            depends_on: W2.T1
            expected_output: No MCP server points at the retired binary.
      - wave: W3
        goal: Replace the legacy runtime with a clean one.
        tasks:
          - id: W3.T1
            task: '`trash ~/.zbrain`, then run `zbrain setup` and `zbrain workspace create research` against the default runtime root.'
            depends_on: W2.T2
            expected_output: A clean runtime containing no `zbrain.db` and no legacy workspace content.
    checks:
      - command: command -v zbrain && zbrain version
        expects: Resolves to `~/.local/bin/zbrain` and prints `0.1.0-go`.
      - command: ls ~/.zbrain
        expects: No `zbrain.db`, `zbrain.db-wal`, or `zbrain.db-shm` present.
      - command: python3 -c "import json,os;d=json.load(open(os.path.expanduser('~/.claude.json')));print('zbrain' in d.get('mcpServers',{}))"
        expects: Prints `False`.
      - command: zbrain workspace current
        expects: Prints JSON naming the freshly created workspace.
    stop_condition: W1.T2 finds any file in `~/.zbrain` not accounted for by the 2026-08-02 review.
    escalation: Halt before any `trash` call and ask the owner to re-confirm the discard decision against the actual inventory.

## Progress
<!-- Append-only durable entries record timestamp, phase, wave, task, task_status, run_id, trace_id, exact verification/result, and changed surfaces or blocker. -->
- timestamp: 2026-08-03T14:43:17Z
  phase: digest-verification
  wave: W1
  task: phase-start
  task_status: in-progress
  run_id: 01KZ41871KN0CZ8NAT0ZVBK5JG
  trace_id: none
  verification: not-run
  changed_surfaces: []
  blocker: none
- timestamp: 2026-08-03T14:44:26Z
  phase: digest-verification
  wave: W1
  task: W1.T1
  task_status: DONE
  run_id: 01KZ41871KN0CZ8NAT0ZVBK5JG
  trace_id: 01KZ41B6XBA4W3PHG24DCH04X0
  exact_verification: go test ./... -> pass; runtime package passed with digest round-trip coverage
  changed_surfaces: [internal/runtime/claim_test.go]
  blocker: none
- timestamp: 2026-08-03T14:47:33Z
  phase: digest-verification
  wave: W2
  task: W2.T1
  task_status: DONE
  run_id: 01KZ41871KN0CZ8NAT0ZVBK5JG
  trace_id: 01KZ41H3W4QZ283AENEZFT55N4
  exact_verification: go test ./... -> pass; validator tests cover body/title tamper, missing digest, and non-approved statuses
  changed_surfaces: [internal/runtime/claim.go, internal/runtime/claim_test.go]
  blocker: none
- timestamp: 2026-08-03T14:47:33Z
  phase: digest-verification
  wave: W2
  task: W2.T2
  task_status: DONE
  run_id: 01KZ41871KN0CZ8NAT0ZVBK5JG
  trace_id: 01KZ41H3W4QZ283AENEZFT55N4
  exact_verification: go test ./... -> pass; workspace scan and index tests exclude tampered approved claims
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/claim_store_test.go, internal/runtime/index_test.go]
  blocker: none
- timestamp: 2026-08-03T14:50:03Z
  phase: digest-verification
  wave: W3
  task: W3.T1
  task_status: DONE
  run_id: 01KZ41871KN0CZ8NAT0ZVBK5JG
  trace_id: 01KZ41V85MEHF43KY851H7YH76
  exact_verification: go test ./... -> pass; isolated tamper smoke reports invalid_claims path/reason and ask status gap
  changed_surfaces: [internal/runtime/claim_store.go, internal/runtime/index.go, internal/cli/cli_test.go]
  blocker: none
- timestamp: 2026-08-04T12:44:58Z
  phase: index-staleness
  wave: W1
  task: phase-start
  task_status: in-progress
  run_id: 01KZ6CVT6RGFFYT9X8Q7W6JXE8
  trace_id: none
  verification: not-run
  changed_surfaces: []
  blocker: none
- timestamp: 2026-08-04T12:48:29Z
  phase: index-staleness
  wave: W1
  task: W1.T1
  task_status: DONE_WITH_CONCERNS
  run_id: 01KZ6CVT6RGFFYT9X8Q7W6JXE8
  trace_id: 01KZ6D49D8MVX34S99PYYTNCRY
  exact_verification: TMPDIR=<non-symlink temp dir> go test ./internal/runtime -> pass
  changed_surfaces: [internal/runtime/index.go]
  blocker: default macOS TMPDIR tests hit the repository's existing symlink boundary guard under /var/folders; the same package passes with a non-symlink TMPDIR
- timestamp: 2026-08-04T12:53:38Z
  phase: index-staleness
  wave: W2
  task: W2.T1
  task_status: DONE
  run_id: 01KZ6CVT6RGFFYT9X8Q7W6JXE8
  trace_id: 01KZ6DDEDVTYW1HHYE53BHTG3A
  exact_verification: TMPDIR=<non-symlink temp dir> go test ./internal/runtime ./internal/cli -> pass; CheckFresh walks wiki markdown and compares file mtime to the published index
  changed_surfaces: [internal/runtime/index.go]
  blocker: none
- timestamp: 2026-08-04T12:53:38Z
  phase: index-staleness
  wave: W2
  task: W2.T2
  task_status: DONE
  run_id: 01KZ6CVT6RGFFYT9X8Q7W6JXE8
  trace_id: 01KZ6DDEDVTYW1HHYE53BHTG3A
  exact_verification: TMPDIR=<non-symlink temp dir> go test ./internal/runtime ./internal/cli -> pass; stale errors name the first wiki path and say run zbrain reindex
  changed_surfaces: [internal/runtime/index.go, internal/runtime/index_test.go, internal/cli/cli_test.go]
  blocker: none
- timestamp: 2026-08-04T12:53:38Z
  phase: index-staleness
  wave: W2
  task: W2.T3
  task_status: DONE
  run_id: 01KZ6CVT6RGFFYT9X8Q7W6JXE8
  trace_id: 01KZ6DDEDVTYW1HHYE53BHTG3A
  exact_verification: TMPDIR=<non-symlink temp dir> go test ./internal/runtime ./internal/cli -> pass; tests cover edited claims, hand-authored wiki claims, and clean recovery after reindex
  changed_surfaces: [internal/runtime/index_test.go, internal/cli/cli_test.go]
  blocker: none
- timestamp: 2026-08-04T13:27:55Z
  phase: index-staleness
  wave: W1
  task: phase-start
  task_status: in-progress
  run_id: 01KZ6FAZGVZQVCA2FMRVCV6F90
  trace_id: none
  verification: not-run
  changed_surfaces: []
  blocker: previous check REQUEST_CHANGES because the 100k-claim p95 was 5.483018125s, above the 2-second gate
- timestamp: 2026-08-04T13:47:10Z
  phase: index-staleness
  wave: W2
  task: performance-remediation
  task_status: DONE_WITH_CONCERNS
  run_id: 01KZ6FAZGVZQVCA2FMRVCV6F90
  trace_id: 01KZ6GT85QP9MFYKKDHYDF2RFP
  exact_verification: TMPDIR=<non-symlink temp dir> go test ./internal/runtime ./internal/cli -> pass; TMPDIR=<non-symlink temp dir> go test ./... -> pass; go vet ./... -> pass; TMPDIR=<non-symlink temp dir> go test -race ./... -> pass; make build -> pass; ZBRAIN_BENCH_100K=1 TMPDIR=<non-symlink temp dir> go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -> pass, assertion enforces p95 <=2s; safe-temp outside-edit smoke -> pass; make smoke -> concern, hardcoded /tmp is rejected by the existing macOS symlink boundary guard
  changed_surfaces: [internal/runtime/index.go, internal/runtime/index_test.go]
  blocker: default make smoke remains environment-blocked by its hardcoded /tmp path; safe equivalent passes

- timestamp: 2026-08-04T14:05:00Z
  phase: index-staleness
  wave: W2
  task: phase-start
  task_status: in-progress
  run_id: 01KZ6J24SPDZ9952Z0VTKFR0VA
  trace_id: none
  verification: not-run
  changed_surfaces: []
  blocker: prior full check REQUEST_CHANGES because make smoke used a hardcoded /tmp path rejected by the existing macOS symlink boundary guard
- timestamp: 2026-08-04T14:05:00Z
  phase: index-staleness
  wave: W2
  task: smoke-proof-remediation
  task_status: in-progress
  run_id: 01KZ6J24SPDZ9952Z0VTKFR0VA
  trace_id: none
  verification: not-run
  changed_surfaces: []
  blocker: none
- timestamp: 2026-08-04T14:16:25Z
  phase: index-staleness
  wave: W2
  task: smoke-proof-remediation
  task_status: DONE
  run_id: 01KZ6J24SPDZ9952Z0VTKFR0VA
  trace_id: 01KZ6JGE3CZGZV5F8BMAMKC5JD
  exact_verification: make smoke -> pass; canonicalized TMPDIR with pwd -P and completed setup, workspace creation, evidence capture, approval, reindex, and trusted ask
  changed_surfaces: [Makefile]
  blocker: none
- timestamp: 2026-08-04T14:49:38Z
  phase: docs-and-skills
  wave: W1
  task: phase-start
  task_status: in-progress
  run_id: 01KZ6M0E06BF4E507DMXVTRX1E
  trace_id: none
  verification: not-run
  changed_surfaces: []
  blocker: none

- timestamp: 2026-08-04T14:56:14Z
  phase: docs-and-skills
  wave: W1
  task: W1.T1
  task_status: DONE
  run_id: 01KZ6M0E06BF4E507DMXVTRX1E
  trace_id: 01KZ6ME03ZB39AE7QXM9ZT5Y4P
  exact_verification: make build && ./dist/zbrain --help -> pass; command-list comparison confirms CLAUDE.md and README.md match the help surface exactly
  changed_surfaces: [CLAUDE.md, README.md]
  blocker: none
- timestamp: 2026-08-04T14:56:14Z
  phase: docs-and-skills
  wave: W1
  task: W1.T2
  task_status: DONE
  run_id: 01KZ6M0E06BF4E507DMXVTRX1E
  trace_id: 01KZ6ME03ZB39AE7QXM9ZT5Y4P
  exact_verification: git diff --check -> pass; README.md and trusted-memory-spec.md state digest rejection, path reporting, and fail-closed retrieval after outside edits
  changed_surfaces: [README.md, trusted-memory-spec.md]
  blocker: none
- timestamp: 2026-08-04T14:56:14Z
  phase: docs-and-skills
  wave: W1
  task: W1.T3
  task_status: DONE
  run_id: 01KZ6M0E06BF4E507DMXVTRX1E
  trace_id: 01KZ6ME03ZB39AE7QXM9ZT5Y4P
  exact_verification: read all assets/skills and assets/engine files; retired-reference and unsupported-command scans -> clean; no asset edits required
  changed_surfaces: []
  blocker: none
  review_note: deprecated qmd prose remains only in assets/agents/wiki-qmd-selector.md, outside the approved surfaces and explicitly marked unused

## Decisions
<!-- Append-only durable entries record timestamp, phase/task, decision, and rationale. -->
- timestamp: 2026-08-03T14:50:03Z
  phase: digest-verification
  task: W3.T1
  decision: Do not change internal/cli/cli.go to expose invalid claim details.
  rationale: runReindex already embeds IndexSummary in its JSON response; adding InvalidClaims to IndexSummary surfaces the path and error without duplicate response fields and keeps the CLI handler thin.
- timestamp: 2026-08-04T12:56:13Z
  phase: index-staleness
  task: phase-checks
  decision: Run repository tests and isolated smoke with a non-symlink TMPDIR.
  rationale: macOS default /var/folders resolves through a symlink and existing boundary validation rejects it; a safe temp root proves the implementation without weakening path security.
- timestamp: 2026-08-04T12:56:13Z
  phase: index-staleness
  task: phase-checks
  decision: Treat the performance benchmark as an explicit proof gap because no Benchmark function or index_benchmark_test.go exists.
  rationale: The benchmark command exits successfully without executing a benchmark, so the under-2-second gate is not measured.
- timestamp: 2026-08-04T13:11:30Z
  phase: index-staleness
  task: phase-checks
  decision: Correct the prior benchmark note: `internal/runtime/index_benchmark_test.go` exists as `TestAskP95At100K`, but the phase command's `-bench` invocation does not execute it.
  rationale: Run the named test explicitly with `ZBRAIN_BENCH_100K=1` so the release gate measures the intended 100k-claim p95.
- timestamp: 2026-08-04T13:47:10Z
  phase: index-staleness
  task: performance-remediation
  decision: Replace per-query content hashing and full wiki traversal with recorded mtimes for every indexed trust input and trust directory; stat known paths on the hot path and walk only a directory whose recorded mtime changed.
  rationale: The existing manifest hash already breached the 2-second p95 before the wiki walk was added; metadata comparison preserves edit, add, delete, evidence, and symlink fail-closed checks while avoiding repeated file reads on clean queries.

- timestamp: 2026-08-04T14:16:25Z
  phase: index-staleness
  task: smoke-proof-remediation
  decision: Canonicalize the smoke temporary root with shell `pwd -P` before creating the isolated home and source file.
  rationale: The user explicitly authorized closing the repository smoke proof blocker; resolving `/tmp` to its real path preserves the existing workspace boundary guard and makes the gate portable across symlinked macOS temp aliases.
- `2026-08-26T15:21:23Z` — plan abandoned. rationale: superseded by MCP v2 protocol upgrade (0.3.0) and site delivery; remaining retire-legacy-runtime inventory does not match 2026-08-02 review - requires re-inventory before destructive trash.

## Validation
<!-- Append-only durable entries record timestamp, phase, exact command/result/output, run_id, check_id, verdict, and proof_gaps. -->
- timestamp: 2026-08-03T14:58:08Z
  phase: digest-verification
  commands:
    - go test ./... -> pass
    - make build -> pass
    - make smoke -> pass
    - go vet ./... -> pass
    - go test -race ./... -> pass
    - isolated tamper smoke -> pass; reindex reported approved=0, invalid=1 with path and verification digest mismatch; ask returned status=gap with no claims
  run_id: 01KZ41871KN0CZ8NAT0ZVBK5JG
  check_id: 01KZ424PPRS3P09Z1AYH5KRFCN
  verdict: APPROVED
  proof_gaps: independent full-diff review was unavailable; judge was same-session
- timestamp: 2026-08-04T13:54:18Z
  phase: index-staleness
  commands:
    - TMPDIR=<non-symlink temp dir> go test ./... -> pass
    - go vet ./... -> pass
    - TMPDIR=<non-symlink temp dir> go test -race ./... -> pass
    - make build -> pass
    - make smoke -> did not complete the reindex/ask flow; hardcoded /tmp path was rejected by the existing macOS symlink boundary guard and subsequent JSON parsing received no reindex output
    - safe-temp outside-edit smoke -> pass; ask named the exact wiki path and said run zbrain reindex
    - ZBRAIN_BENCH_100K=1 TMPDIR=<non-symlink temp dir> go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -> fail; p95=5.483018125s, want <=2s
    - clean HEAD baseline benchmark -> fail; p95=5.046111625s, want <=2s
  run_id: 01KZ6CVT6RGFFYT9X8Q7W6JXE8
  check_id: 01KZ6EDJ0YH2EN2590GS49KF84
  verdict: REQUEST_CHANGES
  proof_gaps: independent full-diff review was unavailable; judge was same-session; benchmark command in the phase definition does not execute TestAskP95At100K
- timestamp: 2026-08-04T13:58:03Z
  phase: index-staleness
  commands:
    - TMPDIR=/private/tmp/zbrain-safe go test ./... -> pass
    - go vet ./... -> pass
    - TMPDIR=/private/tmp/zbrain-safe go test -race ./... -> pass
    - make build -> pass
    - ZBRAIN_BENCH_100K=1 TMPDIR=/private/tmp/zbrain-safe go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -> pass; TestAskP95At100K assertion enforces p95 <=2s
    - make smoke -> REQUEST_CHANGES evidence; hardcoded /tmp root is rejected by the existing macOS symlink boundary guard and the reindex/ask flow does not complete
    - safe-temp outside-edit smoke -> pass; ask names the exact path and says run zbrain reindex
    - git diff --check -> pass
  run_id: 01KZ6FAZGVZQVCA2FMRVCV6F90
  check_id: 01KZ6H3S75BKY300J0DYMMF9FW
  verdict: REQUEST_CHANGES
  proof_gaps: independent full-diff review was unavailable; judge was same-session; repository make smoke remains unproven in this macOS path environment

- timestamp: 2026-08-04T14:22:24Z
  phase: index-staleness
  commands:
    - TMPDIR=/private/tmp/zbrain-safe go test ./... -> pass
    - go vet ./... -> pass
    - TMPDIR=/private/tmp/zbrain-safe go test -race ./... -> pass
    - make build -> pass
    - make smoke -> pass; canonicalized temp root resolved to /private/var and setup, workspace, evidence, approval, reindex, and ask all completed
    - ZBRAIN_BENCH_100K=1 TMPDIR=/private/tmp/zbrain-safe go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -> pass in 177.036s; assertion enforces p95 <=2s
    - git diff --check -> pass
  run_id: 01KZ6J24SPDZ9952Z0VTKFR0VA
  check_id: 01KZ6JEM9HHE96H6XM05XAT6Z0
  verdict: APPROVED
  proof_gaps: independent full-diff review was unavailable; same-session manual review covered security, performance, architecture, code quality, and sibling temp-root handling

- timestamp: 2026-08-04T14:59:15Z
  phase: docs-and-skills
  commands:
    - make build -> pass
    - make smoke -> pass; isolated runtime completed setup, workspace creation, evidence capture, claim approval, reindex, and trusted ask
    - ./dist/zbrain --help + command-list comparison -> pass; CLAUDE.md and README.md match all 12 command specs
    - active assets retired-reference scan -> pass; assets/skills and assets/engine contain no MCP, qmd, Bun, TypeScript, Commander, or clack guidance
    - active assets unsupported-command scan -> pass
    - go test ./... -> environment failure; existing path-security guard rejects symlinked /var/folders temp root
    - TMPDIR=/private/tmp/zbrain-safe go test ./... -> pass
    - git diff --check -> pass
  run_id: 01KZ6M0E06BF4E507DMXVTRX1E
  check_id: 01KZ6MJ8DZVZ3M4QF7GC2M1J1H
  verdict: APPROVED
  proof_gaps: independent full-diff review was unavailable; same-session review covered docs, active assets, command surface, trust wording, and carried-forward checked runtime changes; default macOS TMPDIR test invocation remains environment-blocked

## Current State and Next Action
- active_phase: retire-legacy-runtime
- lifecycle_status: planned
- latest_run_id: 01KZ6M0E06BF4E507DMXVTRX1E
- latest_trace_ids: [01KZ6D49D8MVX34S99PYYTNCRY, 01KZ6DDEDVTYW1HHYE53BHTG3A, 01KZ6GT85QP9MFYKKDHYDF2RFP, 01KZ6JGE3CZGZV5F8BMAMKC5JD, 01KZ6ME03ZB39AE7QXM9ZT5Y4P]
- latest_check_id: 01KZ6MJ8DZVZ3M4QF7GC2M1J1H
- latest_handoff_id: 01KZ6QSS400ASYG17WNDDA629T
- completed_work: digest verification, invalid-claim reporting, regression tests, build, smoke, race, static checks, index mtime watermark, recorded trust-input/directory mtimes, targeted changed-directory freshness scans, wiki markdown freshness rejection, canonicalized make smoke temp root, docs/skills command-surface synchronization, and clean handoff closure of index-staleness and docs-and-skills
- blockers: none
- open_items: [inventory ~/.zbrain, ~/.local/bin/zbrain, and the .mcpServers.zbrain key before any irreversible cleanup; retire-legacy-runtime has not started]
- exact_next_action: work full phase retire-legacy-runtime
