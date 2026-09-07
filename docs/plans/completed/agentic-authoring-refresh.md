---
id: plan-agentic-authoring-refresh-20260904T1215Z
type: plan
intake_id: intake-agentic-authoring-refresh-20260904T1215Z
lane: normal
status: completed
created: 2026-09-04
updated: 2026-09-07
---

# Plan: Agentic authoring, drift refresh, and eval suite

## Outcome
- result: Coding agents author claim *drafts* at scale through a resumable MCP
  authoring campaign (the host agent's own model writes prose; zbrain core
  stays LLM-free), the owner approves them in a single batch ceremony,
  evidence-origin drift is detected and drives a re-approval refresh cycle
  (the zbrain analog of OpenWiki's self-updating wiki), and a six-track eval
  suite (3 high, 3 medium) validates trust integrity, draft quality,
  lifecycle correctness, retrieval, drift detection, and performance.
- success_signals:
  - `zbrain mcp serve` exposes campaign tools (`campaign_begin`,
    `campaign_next`, `campaign_submit_draft`); campaign output is always
    `status: draft` claims visible only as `promotion_candidates`.
  - One interactive TTY ceremony approves multiple drafts with per-draft
    canonical digest confirmation; no draft reaches `approved` without it.
  - `zbrain evidence check` re-hashes local origins and reports
    `changed` / `missing` / `uncheckable` without mutating anything; `doctor`
    reports drift findings (exit 2) naming affected claim IDs.
  - Eval suite runs via documented commands and writes machine-readable
    results under `docs/proofs/`; H-tracks fail the initiative on violation.

## Authority and Requirements
- authority:
  - `trusted-memory-spec.md` — trust contract, design rules (§10), release
    gate (§11).
  - `docs/trusted-agent-gateway-spec.md` — MCP surface, challenge/token
    ceremony, fail-closed mapping.
  - `docs/plans/completed/conflict-aware-drafts.md` and
    `docs/trusted-agent-gateway-spec.md` — existing draft/lifecycle semantics
    the campaign must reuse, not fork.
  - Owner request 2026-09-04 (brainstorm): adapt OpenWiki-style agentic
    authoring + self-update into zbrain, keeping the no-LLM-in-core boundary
    (host-agent via MCP chosen; sidecar and in-core options rejected).
- requirements:
  - R1 [accepted]: No LLM, model-provider SDK, API key, or network call is
    added to zbrain core; authoring prose is produced exclusively by the MCP
    client (host agent) using its own model. Verifiable by dependency scan
    and absence of new outbound-call paths in `internal/runtime`.
  - R2 [accepted]: Campaign state is a resumable JSON run file at
    `workspaces/<workspace>/campaigns/<run-id>.json` with a versioned schema
    (runId, phase, ordered draft list with status); a malformed run file is a
    hard error that refuses to discard resumable work — never a silent reset.
  - R3 [accepted]: Every campaign-submitted draft is `status: draft` and is
    excluded from trusted results; no code path promotes a campaign draft to
    `approved` without the owner ceremony (test must prove the negative).
  - R4 [accepted]: Batch approval extends the existing challenge contract:
    one TTY session binds N draft digests, the owner confirms each digest
    suffix (or explicitly skips items), and consumed/expired/mismatched
    items fail closed exactly like the single-claim ceremony; partial
    completion records which items were granted.
  - R5 [accepted]: `zbrain evidence check` recomputes SHA-256 over origin
    content for local-scheme origins (`file://`, local paths), reporting
    `changed`, `missing`, or `uncheckable` per evidence item; it is
    read-only and never rewrites snapshots or canonical files.
  - R6 [accepted]: A drift finding lists affected claim IDs and the required
    recovery action (supersede + re-approve against a fresh snapshot);
    `doctor` exits 2 with the finding; nothing auto-heals, relocates, or
    silently re-binds claims to new evidence.
  - R7 [accepted]: The eval suite contains exactly six tracks with
    documented commands and machine-readable results in `docs/proofs/`:
    - H1 trust-integrity: adversarial attempts to surface drafts, invalid,
      revoked, superseded, or conflicting content through `ask` and
      `memory_ask` must fail closed in every case.
    - H2 draft precision: LLM-judge scoring that campaign drafts materially
      trace to their bound evidence (judged outside zbrain core; results
      imported as data).
    - H3 lifecycle correctness: supersede/revoke/digest validation including
      batch-ceremony edge cases (skip, partial, replay, expiry).
    - M1 retrieval quality: recall@k and MRR for lexical vs hybrid on the
      `docs/eval/queries.json` corpus.
    - M2 drift detection: recall/precision of `evidence check` findings on
      mutated/missing/unchanged fixtures.
    - M3 performance: p95 < 2s at 100k claims (`ZBRAIN_BENCH_100K=1`) and
      `-race` clean across runtime/cli/mcp/view.
  - R8 [accepted]: The full release gate passes and `docs/proofs/surface.txt`
    (surface test) is updated for every new command, flag, MCP tool, and
    resource; help output remains authoritative.

## Non-goals
- NG1: No LLM/provider integration in zbrain core and no network connectors
  (Notion/Slack/web-search analogs); ingestion stays local-file only.
- NG2: No auto-approval, trust-on-first-use, or author-pinned auto-trust for
  agent-written drafts.
- NG3: No automatic span/evidence relocation or auto-healing; drift produces
  hints and findings only.
- NG4: No scheduled CI/cron campaign runner; campaigns run when a host agent
  invokes them.
- NG5: No binary self-update mechanism (tool upgrade is out of scope; the
  "self-update" here means content refresh via drift detection).
- NG6: No new MCP transport (stdio only), no viewer mutation API, no CSP or
  bind-scope relaxation.
- NG7: Claim graph visualization in `view` is deferred to a later initiative.

## Approach and Risks
- approach: Expose authoring and drift as additive runtime services behind the
  existing trust boundary. (1) `evidence check` is a pure read-only scanner
  reusing the existing evidence digest + origin metadata; (2) batch approval
  generalizes the single-claim challenge/token ceremony (N bound digests, one
  TTY grant session); (3) campaign MCP tools orchestrate draft production with
  a resumable JSON run file, writing only through the existing claim-draft
  path; (4) the eval suite is a Go-native test/bench layer importing LLM-judge
  results as data (judge runs outside zbrain core). No transport, protocol, or
  trust-profile changes.
- constraints:
  - Drafts produced anywhere in this plan are `promotion_candidates` only;
    R3 is proven by a negative test (no code path campaign→approved without
    ceremony).
  - Challenge/token invariants (single consume, 15m/5m expiries, digest
    binding) are reused, not forked; batch adds binding of N digests.
  - `evidence check` never mutates snapshots, canonical files, or indexes;
    origins with non-local schemes are `uncheckable`, never fetched.
  - New MCP tools go through the existing generated-schema fail-closed
    mapping (isError / -32602 / -32603).
- rejected_alternatives:
  - Auto-approve or author-pinned trust for campaign drafts — violates trust
    model (NG2).
  - Sidecar LLM binary or in-core LLM calls — rejected at brainstorm (R1).
  - Auto-relocation of moved evidence spans — hint-only diagnostics (R6, NG3).
- risks:
  - Canonical digest churn: batch ceremony binds N drafts; partial grant must
    leave non-granted drafts untouched (untouched files, no dirty side
    effects beyond the existing mutation rule).
  - Campaign run file corruption mid-run: mitigation is R2 hard-error +
    recovery read path; run file is runtime metadata (0600), never a trust
    input.
  - Doctor exit-2 drift findings could break existing CI consumers:
    findings only appear when drift exists; `status` output stays
    backward-compatible (new fields additive only).
  - FTS5 index interaction: `evidence check` is read-only and does not
    require reindex; drift findings are runtime metadata, not trust inputs.
- recovery: Any phase can stop cleanly — each is independently releasable
  behind the existing surface test; failed phases leave canonical files
  untouched and the plan records the blocker in Progress.

## Phases and Verification
- planning_status: planned
- phases:
  - phase_slug: evidence-drift-check
    story_id: story-evidence-drift-20260904T1315Z
    status: checked
    goal: Read-only origin drift detection with doctor findings (R5, R6)
    depends_on: none
    requirements: [R5, R6, R8]
    allowed_surfaces: [internal/runtime/evidence.go, internal/runtime/paths.go,
      internal/runtime/trust_validation.go, internal/cli/, assets/,
      docs/proofs/surface.txt]
    avoided_surfaces: [internal/runtime/claim_store.go, internal/mcp/, internal/view/]
    waves:
      - wave: W1
        goal: Evidence check service + CLI command
        tasks:
          - id: W1.T1
            task: Implement origin re-hash service in internal/runtime/evidence.go
              — classify each evidence item as `changed` / `missing` /
              `uncheckable` (remote scheme) / `unchanged`; resolve affected
              claim IDs via existing claim store (read-only).
            depends_on: none
            expected_output: Pure function returning structured findings;
              no writes anywhere.
          - id: W1.T2
            task: Add `zbrain evidence check [--workspace <name>]` with JSON
              and human output; findings list evidence IDs, digests, affected
              claim IDs, and recovery action (supersede + re-approve).
            depends_on: W1.T1
            expected_output: Command visible in `zbrain evidence --help` and
              surface test.
          - id: W1.T3
            task: Wire drift findings into `doctor` (exit 2, additive finding
              kind) and document in help text.
            depends_on: W1.T1
            expected_output: `doctor --probe-embedder` unchanged; new finding
              kind appears only with real drift.
      checks:
        - command: go test ./internal/runtime -run 'TestEvidenceCheck' -count=1 -v
          expects: changed/missing/uncheckable/unchanged classified correctly
            on fixture workspaces; no mutation (mtimes + digests asserted).
        - command: go test ./internal/cli -run 'TestEvidenceCheck|TestDoctor' -count=1 -v
          expects: CLI JSON shape, exit codes 0/2 correct.
        - command: go test ./...
          expects: Full suite green.

  - phase_slug: batch-approval-ceremony
    story_id: story-batch-ceremony-20260904T1315Z
    status: checked
    goal: One-session owner approval binding N draft digests (R4)
    depends_on: none
    requirements: [R4, R8]
    allowed_surfaces: [internal/runtime/challenge.go, internal/runtime/transition.go,
      internal/runtime/claim.go, internal/cli/, docs/proofs/surface.txt]
    avoided_surfaces: [internal/mcp/, internal/view/, assets/]
    waves:
      - wave: W1
        goal: Multi-digest challenge contract
        tasks:
          - id: W1.T1
            task: Extend challenge model to bind an ordered list of
              (claim ID, canonical draft digest) pairs plus per-item op
              (approve); keep 15m challenge expiry and digest computation
              identical to the single-claim contract.
            depends_on: none
            expected_output: prepare creates one challenge for N items;
              schema versioned, backward compatible with single-item.
          - id: W1.T2
            task: `zbrain approval grant <challenge-id>` walks items in a
              single TTY session; owner confirms each digest suffix or
              explicitly skips; granted items recorded without consuming a
              token until grant completes.
            depends_on: W1.T1
            expected_output: Grant produces per-item approval record +
              one token (or skipped-items report when none granted).
          - id: W1.T3
            task: Apply path consumes token atomically, applies only granted
              items under workspace lock; skipped/expired/mismatched items
              fail closed with a per-item result, never a partial silent
              apply.
            depends_on: W1.T2
            expected_output: Replay/expiry/wrong-digest/skip-mismatch tests
              all fail closed.
      checks:
        - command: go test ./internal/runtime -run 'TestBatchChallenge|TestBatchGrant|TestBatchApply' -count=1 -v
          expects: N-item grant, skip, partial, replay-after-consume, expiry
            capped by challenge, digest mismatch — all per contract.
        - command: go test ./internal/cli -run 'TestApproval' -count=1 -v
          expects: TTY flow (scripted stdin) matches surface contract.
        - command: go test -race ./internal/runtime ./internal/cli
          expects: Race clean.

  - phase_slug: authoring-campaign-mcp
    story_id: story-campaign-mcp-20260904T1315Z
    status: checked
    goal: Resumable MCP authoring campaign producing drafts only (R1, R2, R3)
    depends_on: none
    requirements: [R1, R2, R3, R8]
    allowed_surfaces: [internal/runtime/coordination.go, internal/runtime/claim.go,
      internal/runtime/paths.go, internal/mcp/, assets/, docs/proofs/surface.txt]
    avoided_surfaces: [internal/view/]
    waves:
      - wave: W1
        goal: Campaign run file + runtime service
        tasks:
          - id: W1.T1
            task: Versioned campaign run file at
              `workspaces/<w>/campaigns/<run-id>.json` (runId, phase, ordered
              draft entries with status pending|submitted|superseded-by-owner)
              written 0600 atomically (temp+rename); malformed file is a hard
              error refusing to discard resumable work.
            depends_on: none
            expected_output: Schema tests + corrupt-file hard-error test.
          - id: W1.T2
            task: Runtime campaign service — begin (bind workspace + draft
              spec list), next (return next pending item context),
              submit_draft (write through the existing claim-draft path only;
              result is `status: draft`), resume, finish.
            depends_on: W1.T1
            expected_output: Every draft lands via existing draft path with
              existing validation; no new write path to approved state.
      - wave: W2
        goal: MCP tool surface
        tasks:
          - id: W2.T1
            task: Add `campaign_begin` / `campaign_next` / `campaign_submit_draft`
              (+ status read) to `zbrain mcp serve` with generated schemas,
              fail-closed input mapping, workspace binding identical to
              `claim_draft`.
            depends_on: W1.T2
            expected_output: Tools listed via tools/list; surface test updated.
          - id: W2.T2
            task: Negative-path proof — adversarial test that no sequence of
              campaign tool calls yields an approved claim (R3) and no
              campaign call triggers reindex or network.
            depends_on: W2.T1
            expected_output: Test proving the negative; R1 dependency scan
              clean (no new outbound calls in internal/runtime).
      checks:
        - command: go test ./internal/runtime -run 'TestCampaign' -count=1 -v
          expects: begin/next/submit/resume/finish + malformed-run hard error.
        - command: go test ./internal/mcp -run 'TestCampaign' -count=1 -v
          expects: Tool schemas, fail-closed input mapping, workspace binding.
        - command: go test ./internal/runtime -run 'TestCampaignCannotApprove' -count=1 -v
          expects: R3 negative proof passes.
        - command: go test -race ./internal/runtime ./internal/mcp
          expects: Race clean.

  - phase_slug: eval-suite
    story_id: story-eval-suite-20260904T1315Z
    status: checked
    goal: Six-track eval suite with machine-readable proofs (R7)
    depends_on: evidence-drift-check, batch-approval-ceremony, authoring-campaign-mcp
    requirements: [R7, R8]
    allowed_surfaces: [docs/eval/, internal/runtime/*_test.go, internal/cli/*_test.go,
      internal/mcp/*_test.go, docs/proofs/, Makefile]
    avoided_surfaces: [internal/runtime/evidence.go non-test, internal/mcp/tools.go non-test]
    waves:
      - wave: W1
        goal: High tracks
        tasks:
          - id: H1.T1
            task: Trust-integrity fuzz corpus — adversarial fixtures (drafts,
              revoked, superseded, conflicting, digest-tampered, legacy) driven
              through `ask` and `memory_ask`; every case must fail closed.
            depends_on: none
            expected_output: docs/eval/trust-integrity corpus + runner test;
              zero leaks.
          - id: H2.T1
            task: Draft-precision judge protocol — fixture workspace with
              bound evidence, campaign produces drafts (deterministic seed),
              judge rubric + import format for external LLM-judge results;
              zbrain core runs no model calls (R1).
            depends_on: authoring-campaign-mcp
            expected_output: docs/eval/draft-precision.md + results schema;
              golden run with mock judge verdicts.
          - id: H3.T1
            task: Lifecycle correctness suite — supersede/revoke/digest chains
              including batch edges (skip, partial, replay, expiry) end-to-end
              via CLI and MCP.
            depends_on: batch-approval-ceremony
            expected_output: Table-driven lifecycle matrix test.
      - wave: W2
        goal: Medium tracks
        tasks:
          - id: M1.T1
            task: Retrieval metrics — extend docs/eval/queries.json with
              expected hits; compute recall@k and MRR lexical vs hybrid.
            depends_on: none
            expected_output: Metrics emitted to docs/proofs/ JSON.
          - id: M2.T1
            task: Drift metrics — recall/precision of `evidence check` on
              mutated/missing/unchanged fixtures.
            depends_on: evidence-drift-check
            expected_output: Metrics emitted to docs/proofs/ JSON.
          - id: M3.T1
            task: Perf + race soak — assert p95 < 2s at 100k
              (ZBRAIN_BENCH_100K=1) and -race clean across the four packages;
              record baseline in docs/proofs/.
            depends_on: none
            expected_output: Machine-readable perf proof.
      checks:
        - command: go test ./internal/... -run 'TestEval' -count=1
          expects: All six tracks green on CI path.
        - command: ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$' -count=1 -v
          expects: p95 < 2s; result recorded.

  - phase_slug: surface-release-gate
    story_id: story-surface-release-20260904T1315Z
    status: checked
    goal: Surface snapshot, docs, and full release gate (R8)
    depends_on: evidence-drift-check, batch-approval-ceremony, authoring-campaign-mcp, eval-suite
    requirements: [R8]
    allowed_surfaces: [docs/proofs/surface.txt, README.md, trusted-memory-spec.md,
      docs/trusted-agent-gateway-spec.md, internal/cli/, Makefile]
    avoided_surfaces: []
    waves:
      - wave: W1
        goal: Contract surfaces + gate
        tasks:
          - id: W1.T1
            task: Regenerate docs/proofs/surface.txt; update README command
              list, gateway spec MCP tool list, help texts for evidence check /
              approval batch / campaign tools.
            depends_on: none
            expected_output: Surface test passes; docs match --help exactly.
          - id: W1.T2
            task: Full release gate run and record in docs/proofs/.
            depends_on: W1.T1
            expected_output: All gates green with proof artifacts.
      checks:
        - command: go test ./...
          expects: Full suite green.
        - command: go vet ./... && go test -race ./internal/runtime ./internal/cli ./internal/view ./internal/mcp
          expects: Vet clean, race clean.
        - command: make build && make smoke
          expects: Build + lifecycle smoke pass.
        - command: CGO_ENABLED=0 go build ./cmd/zbrain && git diff --check
          expects: CGO-free build ok, whitespace clean.

## Progress
- 2026-09-04T17:25Z | phase: surface-release-gate | wave: W1 | task: W1.T1 | task_status: done | run_id: none | verification: TestSurface pass without regen (phases 1-3 kept snapshot current); README command list gained `evidence check`; gateway spec tool list 7->10 with campaign semantics + batch-approval paragraph; grant --help already documents batch walk | surfaces: README.md, docs/trusted-agent-gateway-spec.md, docs/proofs/surface.txt
- 2026-09-04T17:25Z | phase: surface-release-gate | wave: W1 | task: W1.T2 | task_status: done | run_id: none | verification: full release gate green — go test ./... rc=0, go vet rc=0, -race 4 packages rc=0, make build rc=0, make smoke rc=0 (0 fail/error/panic), CGO_ENABLED=0 build ok, git diff --check clean | surfaces: (verification only)
- 2026-09-04T17:25Z | phase: surface-release-gate | wave: W1 | wave-summary: W1 complete — surfaces match --help exactly, full gate green; executed directly in-session (no subagent)
- 2026-09-04T16:30Z | phase: eval-suite | wave: W1 | task: H1.T1 | task_status: done | run_id: none | verification: `go test ./internal/eval -run 'TestEvalTrustIntegrity' -count=1 -v` pass (10 subtests incl. draft/revoked/superseded/conflict/tampered/stale/dirty/missing/legacy) driven through BOTH zbrain ask and memory_ask; zero leaks | surfaces: internal/eval/trust_integrity_test.go, docs/eval/trust-integrity/README.md, docs/proofs/eval-trust-integrity.json
- 2026-09-04T16:30Z | phase: eval-suite | wave: W1 | task: H2.T1 | task_status: done | run_id: none | verification: `go test ./internal/eval -run 'TestEvalDraftPrecisionGolden' -count=1 -v` pass — rubric + zbrain.eval.draft-precision/v1 import validation (fail closed on gaps/threshold/metrics mismatch); golden run with mock judge; zero model calls in core | surfaces: internal/eval/draft_precision_test.go, docs/eval/draft-precision.md, docs/proofs/eval-draft-precision-golden.json
- 2026-09-04T16:30Z | phase: eval-suite | wave: W1 | task: H3.T1 | task_status: done | run_id: none | verification: `go test ./internal/eval -run 'TestEvalLifecycle' -count=1 -v` pass (4 scenarios: CLI chain, MCP chain, batch edges CLI, batch edges stores) | surfaces: internal/eval/lifecycle_test.go, docs/proofs/eval-lifecycle.json
- 2026-09-04T16:30Z | phase: eval-suite | wave: W2 | task: M1.T1 | task_status: done | run_id: none | verification: `go test ./internal/runtime -run 'TestEvalRetrievalMetrics' -count=1 -v` pass — deterministic 90-claim corpus, 20 queries, recall@10 0.667 + MRR 1.0 lexical and hybrid; queries.json extended additively (relevant_total, texts unchanged, baseline stays comparable) | surfaces: internal/runtime/eval_retrieval_metrics_test.go, docs/eval/queries.json, docs/proofs/eval-retrieval-metrics.json
- 2026-09-04T16:30Z | phase: eval-suite | wave: W2 | task: M2.T1 | task_status: done | run_id: none | verification: `go test ./internal/runtime -run 'TestEvalDriftMetrics' -count=1 -v` pass — precision 1.0 / recall 1.0 on mutated/missing/unchanged fixtures + read-only proof (bytes+mtime unchanged over 9 files) | surfaces: internal/runtime/eval_drift_metrics_test.go, docs/proofs/eval-drift-metrics.json
- 2026-09-04T16:30Z | phase: eval-suite | wave: W2 | task: M3.T1 | task_status: PENDING-BENCH
- 2026-09-04T17:10Z | phase: eval-suite | wave: W2 | task: M3.T1 | task_status: DONE_WITH_CONCERNS | run_id: none | verification: owner waived the 100k bench run (pure-Go SQLite corpus too slow on this box; two attempts, one killed after 45min CPU with no result); `go test -race` clean on all four packages; existing docs/proofs/bench-baseline.json stands; perf-proof writer committed in index_benchmark_test.go for future runs | concern: p95@100k not re-measured this session
- 2026-09-04T15:45Z | phase: authoring-campaign-mcp | wave: W1 | task: W1.T1 | task_status: done | run_id: none | verification: `go test ./internal/runtime -run 'TestCampaign' -count=1 -v` pass (9 tests) — zbrain.campaign/v1 at workspaces/<w>/campaigns/<run-id>.json (0600, atomic temp+rename); run ID cmp_<32hex>; malformed run = hard error naming file, explicit operator recovery, never auto-reset | surfaces: internal/runtime/campaign.go, internal/runtime/campaign_test.go
- 2026-09-04T15:45Z | phase: authoring-campaign-mcp | wave: W1 | task: W1.T2 | task_status: done | run_id: none | verification: TestCampaign suite pass — Begin/Next/Submit/Resume/Finish; submit reuses claim-draft validation via ValidateClaim probe then writes through writeDraftUnlocked + recoverPendingTransitionForMutationUnlocked; already-submitted/unknown-index/finished fail closed; claim-creation failure leaves run file untouched | surfaces: internal/runtime/campaign.go
- 2026-09-04T15:45Z | phase: authoring-campaign-mcp | wave: W2 | task: W2.T1 | task_status: done | run_id: none | verification: `go test ./internal/mcp -run 'TestCampaign' -count=1 -v` pass (4 tests) — campaign_begin / campaign_next (doubles as status read) / campaign_submit_draft registered, workspace binding identical to claim_draft, fail-closed input mapping; tools/list 7->10 | surfaces: internal/mcp/tools.go, internal/mcp/tools_test.go, internal/mcp/campaign_test.go
- 2026-09-04T15:45Z | phase: authoring-campaign-mcp | wave: W2 | task: W2.T2 | task_status: done | run_id: none | verification: TestCampaignCannotApprove pass — no campaign path yields approved (zero transitions, verified material untouched, existing approved claim untouched); source scan rejects net/http + Approve/Revoke/Challenge/Transition/Rebuild symbols in campaign.go; published generation unchanged while dirty | surfaces: internal/runtime/campaign_test.go
- 2026-09-04T15:45Z | phase: authoring-campaign-mcp | wave: W2 | wave-summary: W2 complete — MCP surface + R3 negative proof verified; race rc=0, go test ./... rc=0, vet rc=0; executed by spawned subagent, gated in-session
- 2026-09-04T15:05Z | phase: batch-approval-ceremony | wave: W1 | task: W1.T1 | task_status: done | run_id: none | verification: `go test ./internal/runtime -run 'TestBatchChallenge|TestBatchGrant|TestBatchApply' -count=1 -v` pass (5 subtests) — schema stays zbrain.challenge/v3 with optional Items; batch action digest domain-separated (count + ordered items); single-item challenges byte-identical; existing v3 records validate | surfaces: internal/runtime/challenge.go
- 2026-09-04T15:05Z | phase: batch-approval-ceremony | wave: W1 | task: W1.T2 | task_status: done | run_id: none | verification: `go test ./internal/cli -run 'TestApproval' -count=1 -v` pass — single-session walk, per-item confirm-or-skip, mismatch aborts whole walk pre-persist, >=1 granted -> one token, skip-all -> no token persisted | surfaces: internal/cli/cli.go, internal/cli/cli_test.go
- 2026-09-04T15:05Z | phase: batch-approval-ceremony | wave: W1 | task: W1.T3 | task_status: done | run_id: none | verification: TestBatchApply + race pass — whole-batch token/expiry fail-closed first, per-item canonical revalidation under workspace lock, per-item result applied/skipped/failed, token consumed exactly once | surfaces: internal/runtime/lifecycle.go
- 2026-09-04T15:05Z | phase: batch-approval-ceremony | wave: W1 | wave-summary: W1 complete — batch ceremony verified (`go test -race` rc=0, `go test ./...` rc=0, `go vet ./...` rc=0); executed by spawned subagent, gated in-session; no existing tests modified
- 2026-09-04T14:30Z | phase: evidence-drift-check | wave: W1 | task: W1.T1 | task_status: done | run_id: none | verification: `go test ./internal/runtime -run 'TestEvidenceCheck' -count=1 -v` pass — `TestEvidenceCheckClassifiesOriginDrift` covers unchanged/changed/missing/uncheckable + affected-claim resolution + no-mutation (digest+mtime before/after asserted) | surfaces: internal/runtime/evidence.go, internal/runtime/evidence_test.go
- 2026-09-04T14:30Z | phase: evidence-drift-check | wave: W1 | task: W1.T2 | task_status: done | run_id: none | verification: `go test ./internal/cli -run 'TestEvidenceCheck|TestDoctor' -count=1 -v` pass — `zbrain evidence check [--workspace <name>]` JSON output (schema_version/workspace/findings), listed in `evidence --help` and root help; surface test regenerated | surfaces: internal/cli/cli.go, internal/cli/cli_test.go, docs/proofs/surface.txt
- 2026-09-04T14:30Z | phase: evidence-drift-check | wave: W1 | task: W1.T3 | task_status: done | run_id: none | verification: `TestDoctorReportsEvidenceDriftFinding` pass — per-item `evidence <id> drift: <status>; <recovery>` findings; exit 2 only with real drift; `--probe-embedder` untouched | surfaces: internal/cli/cli.go
- 2026-09-04T14:30Z | phase: evidence-drift-check | wave: W1 | wave-summary: W1 complete — evidence check service + CLI + doctor wiring verified; `go test ./...` and `go vet ./...` fully green; implementation executed by spawned subagent, verified and gated in-session

## Decisions
- 2026-09-07 | phase: surface-release-gate | absorb: none | rationale: initiative merged in PR #28 (f67ff12) and absorbed into Rust 0.4.0 cutover; bench M3 re-measured in 100k proof
- 2026-09-04T17:10Z | phase: eval-suite | decision: owner waived ZBRAIN_BENCH_100K=1 run this session; gate proceeds as APPROVE_WITH_REQUESTS with re-measurement as the explicit request | rationale: bench cost exceeds session budget; race soak + existing baseline + committed proof writer cover the track until a faster box is available
- 2026-09-04T16:30Z | phase: eval-suite | decision: eval runners live in internal/eval/ extending the existing harness (internal/mcp/mcp.go gained only a 7-line NewServerForEval test helper) | rationale: cross-layer tests (CLI ask + MCP memory_ask) need one fixture set; mcp package itself untouched except the constructor passthrough
- 2026-09-04T16:30Z | phase: eval-suite | decision: queries.json extended additively with relevant_total per query; query texts and corpus generator unchanged | rationale: keeps docs/proofs/eval-baseline.json directly comparable; proof JSON records baseline_comparable:true
- 2026-09-04T16:30Z | phase: eval-suite | decision: fixed stale path in docs/eval/trust-integrity/README.md (runner lives in internal/eval/, not internal/mcp/) | rationale: one-line doc correction during gate review
- 2026-09-04T15:46Z | phase: authoring-campaign-mcp | decision: campaign service in NEW internal/runtime/campaign.go instead of extending coordination.go | rationale: coordination.go is lock/generation plumbing; durable domain behavior mirrors the challenge.go precedent; no avoided surface touched
- 2026-09-04T15:46Z | phase: authoring-campaign-mcp | decision: campaign_next doubles as the status read (phase, counts, next index/spec) | rationale: keeps tool count minimal (3) per plan intent; avoids a fourth schema surface
- 2026-09-04T15:46Z | phase: authoring-campaign-mcp | decision: FinishCampaign is runtime-layer only in this phase; no MCP tool; superseded-by-owner status is persisted-but-unset until owner-facing CLI wiring lands in the final surface phase | rationale: plan W2.T1 enumerates exactly begin/next/submit(+status); open item tracked in Current State
- 2026-09-04T15:06Z | phase: batch-approval-ceremony | decision: batch apply + prepare landed in internal/runtime/lifecycle.go rather than transition.go/claim.go named in allowed_surfaces | rationale: ApplyChallengeBatch extends the existing challenge apply flow which lives in lifecycle.go; no avoided surface (internal/mcp/, internal/view/, assets/) touched; recorded as planning-time surface-list imprecision
- 2026-09-04T15:06Z | phase: batch-approval-ceremony | decision: fully skipped walk is NOT persisted — challenge remains pending and expires (15m) with zero mutations | rationale: strongest fail-closed reading; skipped items ARE persisted for partial grants via the granted/skipped partition enforced by validateChallengeRecord
- 2026-09-04T15:06Z | phase: batch-approval-ceremony | decision: per-item TTY confirmation uses each item's canonical draft digest suffix (full digest printed by show/grant) | rationale: the batch action digest covers all ordered items and cannot confirm an individual item; confirmation rule strength is identical to the single-claim ceremony
- 2026-09-04T14:30Z | phase: evidence-drift-check | decision: doctor drift findings rendered as prefixed `[]string` entries (`evidence <id> drift: <status>; <recovery>`) instead of a new structured finding-kind field | rationale: doctor's existing findings schema is []string; additive prefix preserves backward compatibility with minimal diff (R6 satisfied: exit 2 + affected claims + recovery action present)
- 2026-09-04T14:30Z | phase: evidence-drift-check | decision: `CheckDrift` skips evidence entries whose `source.yaml` cannot be read/parsed rather than classifying them | rationale: snapshot-integrity rejection belongs to the existing `EvidenceValidator`/`reindex` path; duplicating it here would risk divergent classification. Corrupt snapshots remain caught by `reindex` fail-closed behavior. Recorded as a known limitation for the eval M2 track

## Validation
- 2026-09-04T17:25Z | phase: surface-release-gate | mode: gate | verdict: APPROVED | judge: same-session | judge_model: opencode-go/omen-alpha (Omen Alpha) | reviewing agent authored the doc edits directly and ran all proof commands | proof_gaps: none for this phase (initiative-level M3 bench request from eval-suite gate still open) | not_independently_verified: smoke-run internals beyond rc=0 + zero fail/error/panic grep
  - `go test ./internal/cli -run 'TestSurface' -count=1` -> pass (rc=0)
  - `go test ./...` -> pass (rc=0, all packages)
  - `go vet ./...` -> pass (rc=0)
  - `go test -race ./internal/runtime ./internal/cli ./internal/view ./internal/mcp` -> pass (rc=0)
  - `make build` -> pass (rc=0)
  - `make smoke` -> pass (rc=0)
  - `CGO_ENABLED=0 go build ./cmd/zbrain` -> pass
  - `git diff --check` -> clean
  - plan alignment: R8 satisfied — surface snapshot current, README + gateway spec + help texts match shipped behavior
  - receipt: context_sources: [plan, README.md, gateway-spec, command outputs] | policy: gate (work.md step 11, check.md steps 1-4+6-11) | judge: same-session | judge_model: opencode-go/omen-alpha | retries: 0 | rollback_point: git 7a7d10b | failure_ledger: absent | not_independently_verified: smoke internals
- 2026-09-04T17:10Z | phase: eval-suite | mode: gate | verdict: APPROVE_WITH_REQUESTS | judge: same-session | judge_model: opencode-go/omen-alpha (Omen Alpha) | reviewing agent re-ran proof commands independently (cancelled subagent authored; implementation verified from tree) | proof_gaps: 100k p95 re-measurement (M3 bench waived by owner; request: re-run on faster hardware before release) | not_independently_verified: mock-judge realism for H2 (golden uses fixture verdicts by design), external MCP client schema shape
  - `go test ./internal/... -run 'TestEval' -count=1 -v` -> pass (rc=0; H1 10 subtests, H2 golden, H3 4 scenarios, M1, M2)
  - `go test -race ./internal/runtime ./internal/cli ./internal/mcp ./internal/view` -> pass (rc=0)
  - `go test ./...` -> pass (rc=0, verified during gate)
  - `go vet ./...` -> pass (rc=0)
  - `ZBRAIN_BENCH_100K=1 go test ./internal/runtime -run '^TestAskP95At100K$'` -> NOT RUN (waived; two attempts killed: env-miss skip + 45min-CPU kill)
  - plan alignment: H1/H2/H3/M1/M2 complete with proofs in docs/proofs/eval-*.json; M1 queries.json additive-only (baseline comparable); R1 holds (zero model calls — grep net/http absent from eval paths, judge verdicts imported as data)
  - receipt: context_sources: [plan, trusted-memory-spec.md, git diff, eval proof JSONs] | policy: gate (work.md step 11, check.md steps 1-4+6-11) | judge: same-session | judge_model: opencode-go/omen-alpha | retries: 0 | rollback_point: git 7a7d10b | failure_ledger: absent | not_independently_verified: H2 mock-judge realism, external-client schema shape
- 2026-09-04T15:50Z | phase: authoring-campaign-mcp | mode: gate | verdict: APPROVED | judge: same-session | judge_model: opencode-go/omen-alpha (Omen Alpha) | reviewing agent re-ran all proof commands independently (subagent ses_f94a49b34ffejkPJbv9O1a4CES authored) | proof_gaps: none | not_independently_verified: fsync durability of temp+rename under crash (unit-tested atomics only, no crash-injection test), schema-generation output shape as seen by external MCP clients (in-memory client harness only)
  - `go test ./internal/runtime -run 'TestCampaign' -count=1 -v` -> pass (rc=0, 9 tests)
  - `go test ./internal/mcp -run 'TestCampaign' -count=1 -v` -> pass (rc=0, 4 tests)
  - `go test ./internal/runtime -run 'TestCampaignCannotApprove' -count=1` -> pass (rc=0)
  - `go test -race ./internal/runtime ./internal/mcp` -> pass (rc=0)
  - `go test ./...` -> pass (rc=0)
  - `go vet ./...` -> pass (rc=0); `git diff --check` clean; net/http grep in campaign.go -> absent
  - plan alignment: R1 (no LLM/network — imports audited), R2 (resumable run file, malformed = hard error), R3 (draft-only, negative proof + write path reuses writeDraftUnlocked), avoided surface internal/view/ untouched, assets/ untouched
  - receipt: context_sources: [plan, gateway-spec, git diff, subagent report, campaign.go source] | policy: gate (work.md step 11, check.md steps 1-4+6-11) | judge: same-session | judge_model: opencode-go/omen-alpha | retries: 0 | rollback_point: git 7a7d10b | failure_ledger: absent | not_independently_verified: crash-injection durability, external-client schema shape
- 2026-09-04T15:10Z | phase: batch-approval-ceremony | mode: gate | verdict: APPROVED | judge: same-session | judge_model: opencode-go/omen-alpha (Omen Alpha) | reviewing agent re-ran all proof commands independently (subagent ses_f94af0197ffeXuvLcExBFYyqN5 authored) | proof_gaps: none | not_independently_verified: TTY interaction realism beyond scripted-stdin tests (no real terminal session run), token entropy source (reuses existing newChallengeToken unchanged)
  - `go test ./internal/runtime -run 'TestBatchChallenge|TestBatchGrant|TestBatchApply' -count=1 -v` -> pass (rc=0, 5 subtests)
  - `go test ./internal/cli -run 'TestApproval' -count=1 -v` -> pass (rc=0)
  - `go test -race ./internal/runtime ./internal/cli` -> pass (rc=0)
  - `go test ./...` -> pass (rc=0)
  - `go vet ./...` -> pass (rc=0)
  - `git diff --check` -> clean (rc=0); gofmt clean on changed files (two pre-existing unformatted test files untouched)
  - plan alignment: invariants preserved (15m/5m expiries, single consume, prepare persists no token, digest computation style identical); skipped/expired/mismatched fail closed per item; no partial silent apply; avoided surfaces untouched
  - receipt: context_sources: [docs/plans/active/agentic-authoring-refresh.md, docs/trusted-agent-gateway-spec.md, git diff, subagent report] | policy: gate (work.md step 11, check.md steps 1-4+6-11) | judge: same-session | judge_model: opencode-go/omen-alpha | retries: 0 | rollback_point: git 7a7d10b | failure_ledger: absent | not_independently_verified: real-TTY flow, token entropy
- 2026-09-04T14:35Z | phase: evidence-drift-check | mode: gate | verdict: APPROVED | judge: same-session | judge_model: opencode-go/omen-alpha (Omen Alpha) | reviewing agent did not author the diff (spawned subagent ses_f94b65315ffebLYwEbNZMwLih9 authored; reviewing agent re-ran all proof commands independently) | proof_gaps: none | not_independently_verified: fixture-workspace construction inside tests (trusted subagent-authored helpers; assertions re-executed pass), surface.txt regeneration mechanics (byte-exact snapshot test passes, regen path itself not manually replayed)
  - `go test ./internal/runtime -run 'TestEvidenceCheck' -count=1 -v` -> pass (rc=0)
  - `go test ./internal/cli -run 'TestEvidenceCheck|TestDoctor' -count=1 -v` -> pass (rc=0)
  - `go test ./...` -> pass (rc=0)
  - `go vet ./...` -> pass (rc=0)
  - plan alignment: diff confined to allowed_surfaces (internal/runtime/evidence.go+test, internal/cli/cli.go+test, docs/proofs/surface.txt); no mutation of snapshots/canonical/index verified via before/after digest+mtime assertions; avoided surfaces (claim_store.go, internal/mcp/, internal/view/) untouched
  - receipt: context_sources: [docs/plans/active/agentic-authoring-refresh.md, AGENTS.md, git diff, subagent report] | policy: gate (work.md step 11, check.md steps 1-4+6-11, complete manual review deferred to initiative-final handoff) | judge: same-session | judge_model: opencode-go/omen-alpha | retries: 0 | rollback_point: git 7a7d10b (pre-phase) | failure_ledger: absent | not_independently_verified: test fixture internals, surface regen path

## Current State and Next Action
- active_phase: none
- lifecycle_status: done
- latest_run_id: none
- latest_trace_ids: []
- latest_check_id: none
- latest_handoff_id: handoff-20260907
- blockers: none
- open_items: none
- exact_next_action: none — initiative complete; landed in PR #28 and cut over to Rust 0.4.0
