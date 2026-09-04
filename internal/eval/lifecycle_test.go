package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	zcli "github.com/therealtinhtute/zbrain/internal/cli"
	zmcp "github.com/therealtinhtute/zbrain/internal/mcp"
	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

// TestEvalLifecycle is the eval-suite lifecycle correctness suite. It runs
// table-driven end-to-end chains — approve → supersede → revoke with digest
// validation after every step, plus batch-ceremony edges (skip, partial
// grant, replay-after-consume, expiry, digest mismatch) — through BOTH the
// CLI paths (zbrain claim/approval commands with scripted stdin) and the MCP
// claim_lifecycle tool. Results land in docs/proofs/eval-lifecycle.json.
func TestEvalLifecycle(t *testing.T) {
	type scenario struct {
		name string
		// run executes one full chain and returns the observed step records.
		run func(t *testing.T, e evalEnv) []map[string]any
	}
	scenarios := []scenario{
		{name: "approve_supersede_revoke_cli", run: runLifecycleChainCLI},
		{name: "approve_supersede_revoke_mcp", run: runLifecycleChainMCP},
		{name: "batch_ceremony_edges_cli", run: runBatchCeremonyEdgesCLI},
		{name: "batch_ceremony_edges_stores", run: runBatchCeremonyEdgesStores},
	}

	all := make([]map[string]any, 0, len(scenarios))
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			e := newEvalEnv(t)
			steps := sc.run(t, e)
			all = append(all, map[string]any{"scenario": sc.name, "steps": steps})
		})
	}

	writeEvalProof(t, "eval-lifecycle.json", map[string]any{
		"schema":    "zbrain.eval.lifecycle/v1",
		"scenarios": all,
	})
}

func evalStep(name string, detail string) map[string]any {
	return map[string]any{"step": name, "observed": detail}
}

// runLifecycleChainCLI drives approve → supersede → revoke through the
// zbrain CLI with scripted stdin, validating the verification digest after
// every step.
func runLifecycleChainCLI(t *testing.T, e evalEnv) []map[string]any {
	t.Helper()
	newApp := func(stdin string) *zcli.App {
		return &zcli.App{
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
			Stdin:  strings.NewReader(stdin),
			Paths:  e.paths,
			Now:    e.now,
		}
	}
	run := func(t *testing.T, app *zcli.App, args []string) string {
		t.Helper()
		if err := app.Run(args); err != nil {
			t.Fatalf("Run(%v) error = %v", args, err)
		}
		return app.Stdout.(*bytes.Buffer).String()
	}
	var decoded struct {
		ID string `json:"id"`
	}
	store := zruntime.ClaimStore{Paths: e.paths, Now: e.now}
	steps := []map[string]any{}

	// 1. draft (stdin body) + approve.
	app := newApp("chain original body\n")
	out := run(t, app, []string{"claim", "draft", "--tier", "projects", "--title", "Lifecycle Chain", "--basis", "owner"})
	if err := json.Unmarshal([]byte(out), &decoded); err != nil || decoded.ID == "" {
		t.Fatalf("claim draft output %q: %v", out, err)
	}
	originalID := decoded.ID
	app = newApp("")
	run(t, app, []string{"claim", "approve", originalID})
	original, err := store.Read("research", originalID)
	if err != nil {
		t.Fatalf("Read(original) error = %v", err)
	}
	if original.Status != zruntime.ClaimStatusApproved {
		t.Fatalf("original status = %q, want approved", original.Status)
	}
	if err := zruntime.VerifyClaimDigest(original); err != nil {
		t.Fatalf("VerifyClaimDigest(original after approve) error = %v", err)
	}
	steps = append(steps, evalStep("approve", "approved;digest_valid:"+digestSuffix(original.VerifiedDigest)))

	// 2. supersede via a superseding draft + approve.
	app = newApp("chain replacement body\n")
	out = run(t, app, []string{"claim", "supersede", originalID, "--tier", "projects", "--title", "Lifecycle Chain v2", "--basis", "owner"})
	if err := json.Unmarshal([]byte(out), &decoded); err != nil || decoded.ID == "" {
		t.Fatalf("claim supersede output %q: %v", out, err)
	}
	replacementID := decoded.ID
	app = newApp("")
	run(t, app, []string{"claim", "approve", replacementID})
	superseded, err := store.Read("research", originalID)
	if err != nil {
		t.Fatalf("Read(superseded) error = %v", err)
	}
	replacement, err := store.Read("research", replacementID)
	if err != nil {
		t.Fatalf("Read(replacement) error = %v", err)
	}
	if superseded.Status != zruntime.ClaimStatusSuperseded {
		t.Fatalf("superseded status = %q, want superseded", superseded.Status)
	}
	if replacement.Status != zruntime.ClaimStatusApproved {
		t.Fatalf("replacement status = %q, want approved", replacement.Status)
	}
	if err := zruntime.VerifyClaimDigest(superseded); err != nil {
		t.Fatalf("VerifyClaimDigest(superseded) error = %v", err)
	}
	if err := zruntime.VerifyClaimDigest(replacement); err != nil {
		t.Fatalf("VerifyClaimDigest(replacement) error = %v", err)
	}
	foundSupersede := false
	for _, transition := range superseded.Transitions {
		if transition.Kind == zruntime.ClaimTransitionSupersede && transition.PriorVerificationDigest == original.VerifiedDigest {
			foundSupersede = true
		}
	}
	if !foundSupersede {
		t.Fatalf("superseded transitions = %#v, want supersede transition binding prior digest", superseded.Transitions)
	}
	steps = append(steps, evalStep("supersede", "old:superseded;new:approved;digest_valid:"+digestSuffix(replacement.VerifiedDigest)))

	// 3. revoke with reason.
	app = newApp("")
	run(t, app, []string{"claim", "revoke", replacementID, "--reason", "eval chain revoke"})
	revoked, err := store.Read("research", replacementID)
	if err != nil {
		t.Fatalf("Read(revoked) error = %v", err)
	}
	if revoked.Status != zruntime.ClaimStatusRevoked {
		t.Fatalf("revoked status = %q, want revoked", revoked.Status)
	}
	if err := zruntime.VerifyClaimDigest(revoked); err != nil {
		t.Fatalf("VerifyClaimDigest(revoked) error = %v", err)
	}
	if revoked.VerifiedDigest != replacement.VerifiedDigest {
		t.Fatalf("revoked digest %q changed from approved digest %q", revoked.VerifiedDigest, replacement.VerifiedDigest)
	}
	steps = append(steps, evalStep("revoke", "revoked;digest_stable:"+digestSuffix(revoked.VerifiedDigest)))
	return steps
}

// runLifecycleChainMCP drives approve → supersede → revoke through the MCP
// claim_lifecycle tool with the owner grant ceremony in between.
func runLifecycleChainMCP(t *testing.T, e evalEnv) []map[string]any {
	t.Helper()
	server, err := zmcp.NewServerForEval(zmcp.Options{Paths: e.paths, Now: e.now, Version: "eval", Stderr: io.Discard})
	if err != nil {
		t.Fatalf("NewServerForEval() error = %v", err)
	}
	cs := connectEvalClient(t, server)
	store := zruntime.ClaimStore{Paths: e.paths, Now: e.now}
	steps := []map[string]any{}

	prepareApply := func(t *testing.T, action string, claimID string, extra map[string]any) (prepareOut, applyOut map[string]any) {
		t.Helper()
		args := map[string]any{"operation": "prepare", "action": action, "workspace": "research", "claim_id": claimID}
		for key, value := range extra {
			args[key] = value
		}
		res, err := callEvalTool(t, cs, "claim_lifecycle", args)
		if err != nil || res == nil || res.IsError {
			t.Fatalf("claim_lifecycle prepare %s error = %v text = %s", action, err, resultTextOf(res))
		}
		if err := json.Unmarshal([]byte(evalResultText(res)), &prepareOut); err != nil {
			t.Fatalf("decode prepare: %v", err)
		}
		challengeID, _ := prepareOut["challenge_id"].(string)
		if challengeID == "" {
			t.Fatalf("prepare returned no challenge_id: %#v", prepareOut)
		}
		if token, ok := prepareOut["token"]; ok && token != "" {
			t.Fatalf("prepare exposed plaintext token: %v", token)
		}
		granted, workspace, err := (zruntime.ChallengeStore{Paths: e.paths, Now: e.now}).FindChallenge(challengeID)
		if err != nil {
			t.Fatalf("FindChallenge(%s) error = %v", challengeID, err)
		}
		token := (func() string {
			grantedChallenge, err := (zruntime.ChallengeStore{Paths: e.paths, Now: e.now}).Grant(workspace, granted.ID)
			if err != nil {
				t.Fatalf("Grant(%s) error = %v", granted.ID, err)
			}
			return grantedChallenge.Token
		})()
		applyArgs := map[string]any{"operation": "apply", "action": action, "challenge_id": challengeID, "token": token}
		for key, value := range extra {
			applyArgs[key] = value
		}
		res, err = callEvalTool(t, cs, "claim_lifecycle", applyArgs)
		if err != nil || res == nil || res.IsError {
			t.Fatalf("claim_lifecycle apply %s error = %v text = %s", action, err, resultTextOf(res))
		}
		if err := json.Unmarshal([]byte(evalResultText(res)), &applyOut); err != nil {
			t.Fatalf("decode apply: %v", err)
		}
		return prepareOut, applyOut
	}

	// 1. approve.
	draft := evalClaim(newEvalClaimID(t), "MCP Chain", "mcp chain original body\n", "")
	markDirtyForMutation(t, e.paths)
	created, err := store.WriteDraft("research", draft)
	if err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	_, applyOut := prepareApply(t, "approve", created.ID, nil)
	if applyOut["status"] != string(zruntime.ClaimStatusApproved) {
		t.Fatalf("apply approve status = %#v, want approved", applyOut["status"])
	}
	approved, err := store.Read("research", created.ID)
	if err != nil {
		t.Fatalf("Read(approved) error = %v", err)
	}
	if err := zruntime.VerifyClaimDigest(approved); err != nil {
		t.Fatalf("VerifyClaimDigest(approved) error = %v", err)
	}
	if approved.VerifiedBy != "owner:mcp" {
		t.Fatalf("approved verified_by = %q, want owner:mcp", approved.VerifiedBy)
	}
	steps = append(steps, evalStep("approve", "approved;digest_valid:"+digestSuffix(approved.VerifiedDigest)))

	// 2. supersede.
	replacement := evalClaim(newEvalClaimID(t), "MCP Chain v2", "mcp chain replacement body\n", "")
	markDirtyForMutation(t, e.paths)
	superseding, err := store.WriteSupersedingDraft("research", created.ID, replacement)
	if err != nil {
		t.Fatalf("WriteSupersedingDraft() error = %v", err)
	}
	_, applyOut = prepareApply(t, "supersede", superseding.ID, nil)
	if applyOut["status"] != string(zruntime.ClaimStatusApproved) {
		t.Fatalf("apply supersede status = %#v, want approved", applyOut["status"])
	}
	old, err := store.Read("research", created.ID)
	if err != nil {
		t.Fatalf("Read(old) error = %v", err)
	}
	fresh, err := store.Read("research", superseding.ID)
	if err != nil {
		t.Fatalf("Read(fresh) error = %v", err)
	}
	if old.Status != zruntime.ClaimStatusSuperseded || fresh.Status != zruntime.ClaimStatusApproved {
		t.Fatalf("post-supersede statuses = %q/%q, want superseded/approved", old.Status, fresh.Status)
	}
	if err := zruntime.VerifyClaimDigest(old); err != nil {
		t.Fatalf("VerifyClaimDigest(superseded) error = %v", err)
	}
	if err := zruntime.VerifyClaimDigest(fresh); err != nil {
		t.Fatalf("VerifyClaimDigest(replacement) error = %v", err)
	}
	steps = append(steps, evalStep("supersede", "old:superseded;new:approved;digest_valid:"+digestSuffix(fresh.VerifiedDigest)))

	// 3. revoke.
	_, applyOut = prepareApply(t, "revoke", superseding.ID, map[string]any{"revoke_reason": "eval mcp chain revoke"})
	if applyOut["status"] != string(zruntime.ClaimStatusRevoked) {
		t.Fatalf("apply revoke status = %#v, want revoked", applyOut["status"])
	}
	revoked, err := store.Read("research", superseding.ID)
	if err != nil {
		t.Fatalf("Read(revoked) error = %v", err)
	}
	if revoked.Status != zruntime.ClaimStatusRevoked {
		t.Fatalf("revoked status = %q, want revoked", revoked.Status)
	}
	if err := zruntime.VerifyClaimDigest(revoked); err != nil {
		t.Fatalf("VerifyClaimDigest(revoked) error = %v", err)
	}
	steps = append(steps, evalStep("revoke", "revoked;digest_stable:"+digestSuffix(revoked.VerifiedDigest)))

	// 4. replay-after-consume must fail closed.
	res, err := callEvalTool(t, cs, "claim_lifecycle", map[string]any{
		"operation": "apply", "challenge_id": "chg_00000000000000000000000000000000", "token": "deadbeef",
	})
	if err != nil {
		t.Fatalf("replay call error = %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("replay apply unexpectedly succeeded: %s", evalResultText(res))
	}
	steps = append(steps, evalStep("replay_after_consume", "error"))
	return steps
}

// runBatchCeremonyEdgesCLI covers batch-ceremony edges through the CLI
// approval grant walk with scripted stdin plus the store apply path: skip,
// partial grant, and replay-after-consume.
func runBatchCeremonyEdgesCLI(t *testing.T, e evalEnv) []map[string]any {
	t.Helper()
	store := zruntime.ClaimStore{Paths: e.paths, Now: e.now}
	steps := []map[string]any{}

	newApp := func(stdin string) *zcli.App {
		return &zcli.App{
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
			Stdin:  strings.NewReader(stdin),
			Paths:  e.paths,
			Now:    e.now,
		}
	}
	makeDraft := func(t *testing.T, title string, body string) string {
		t.Helper()
		app := newApp(body)
		if err := app.Run([]string{"claim", "draft", "--tier", "projects", "--title", title, "--basis", "owner"}); err != nil {
			t.Fatalf("Run(claim draft %s) error = %v", title, err)
		}
		var decoded struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(app.Stdout.(*bytes.Buffer).String()), &decoded); err != nil || decoded.ID == "" {
			t.Fatalf("decode draft output: %v", err)
		}
		return decoded.ID
	}
	suffixOf := func(t *testing.T, claimID string) string {
		t.Helper()
		digest, err := store.CanonicalDraftDigest("research", claimID)
		if err != nil {
			t.Fatalf("CanonicalDraftDigest(%s) error = %v", claimID, err)
		}
		return digestSuffix(digest)
	}

	// Partial grant: confirm item 1, skip item 2, confirm item 3.
	ids := []string{
		makeDraft(t, "Batch Alpha", "batch alpha body\n"),
		makeDraft(t, "Batch Beta", "batch beta body\n"),
		makeDraft(t, "Batch Gamma", "batch gamma body\n"),
	}
	items := make([]zruntime.ChallengeItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, zruntime.ChallengeItem{ClaimID: id})
	}
	prepared, err := store.PrepareBatchChallenge("research", items)
	if err != nil {
		t.Fatalf("PrepareBatchChallenge() error = %v", err)
	}
	app := newApp(strings.Join([]string{suffixOf(t, ids[0]), "skip", suffixOf(t, ids[2])}, "\n") + "\n")
	if err := app.Run([]string{"approval", "grant", prepared.Challenge.ID}); err != nil {
		t.Fatalf("Run(approval grant partial) error = %v", err)
	}
	var granted struct {
		Token        string   `json:"token"`
		GrantedItems []string `json:"granted_items"`
		SkippedItems []string `json:"skipped_items"`
	}
	if err := json.Unmarshal([]byte(app.Stdout.(*bytes.Buffer).String()), &granted); err != nil {
		t.Fatalf("decode grant output: %v", err)
	}
	if granted.Token == "" || len(granted.GrantedItems) != 2 || len(granted.SkippedItems) != 1 {
		t.Fatalf("partial grant = %#v", granted)
	}
	result, err := store.ApplyChallengeBatch("research", prepared.Challenge.ID, granted.Token, zruntime.ClaimMutationOptions{})
	if err != nil {
		t.Fatalf("ApplyChallengeBatch() error = %v", err)
	}
	statuses := ""
	for i, item := range result.Items {
		want := zruntime.BatchApplyItemApplied
		if i == 1 {
			want = zruntime.BatchApplyItemSkipped
		}
		if item.Status != want {
			t.Fatalf("item %d status = %q, want %q", i, item.Status, want)
		}
		statuses += item.Status + ","
	}
	steps = append(steps, evalStep("partial_grant", "granted=2;skipped=1;statuses="+statuses))

	// Replay-after-consume: the same token must be rejected.
	if _, err := store.ApplyChallengeBatch("research", prepared.Challenge.ID, granted.Token, zruntime.ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("replay apply error = %v, want already consumed", err)
	}
	steps = append(steps, evalStep("replay_after_consume", "error:already consumed"))

	// A fully skipped walk issues no token and leaves the challenge ungranted.
	skipIDs := []string{makeDraft(t, "Batch Delta", "batch delta body\n"), makeDraft(t, "Batch Epsilon", "batch epsilon body\n")}
	skipItems := []zruntime.ChallengeItem{{ClaimID: skipIDs[0]}, {ClaimID: skipIDs[1]}}
	skipPrepared, err := store.PrepareBatchChallenge("research", skipItems)
	if err != nil {
		t.Fatalf("PrepareBatchChallenge(skip) error = %v", err)
	}
	app = newApp("skip\nskip\n")
	if err := app.Run([]string{"approval", "grant", skipPrepared.Challenge.ID}); err != nil {
		t.Fatalf("Run(approval grant skip-all) error = %v", err)
	}
	var skipAll struct {
		Token        string   `json:"token"`
		GrantedItems []string `json:"granted_items"`
		SkippedItems []string `json:"skipped_items"`
	}
	if err := json.Unmarshal([]byte(app.Stdout.(*bytes.Buffer).String()), &skipAll); err != nil {
		t.Fatalf("decode skip-all output: %v", err)
	}
	if skipAll.Token != "" || len(skipAll.GrantedItems) != 0 || len(skipAll.SkippedItems) != 2 {
		t.Fatalf("skip-all output = %#v, want no token and both items skipped", skipAll)
	}
	ungranted, err := (zruntime.ChallengeStore{Paths: e.paths, Now: e.now}).Read("research", skipPrepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Read(after skip-all) error = %v", err)
	}
	if ungranted.Granted || ungranted.TokenSHA256 != "" {
		t.Fatalf("skip-all granted the challenge: %#v", ungranted)
	}
	steps = append(steps, evalStep("skip_all", "no_token;challenge ungranted"))
	return steps
}

// runBatchCeremonyEdgesStores covers batch-ceremony edges that the CLI does
// not expose as commands: challenge expiry and per-item digest mismatch at
// apply time.
func runBatchCeremonyEdgesStores(t *testing.T, e evalEnv) []map[string]any {
	t.Helper()
	store := zruntime.ClaimStore{Paths: e.paths, Now: e.now}
	steps := []map[string]any{}

	makeDraft := func(t *testing.T, title string, body string) zruntime.Claim {
		t.Helper()
		claim := evalClaim(newEvalClaimID(t), title, body, "")
		markDirtyForMutation(t, e.paths)
		created, err := store.WriteDraft("research", claim)
		if err != nil {
			t.Fatalf("WriteDraft(%s) error = %v", title, err)
		}
		return created
	}

	// Expiry: a grant after challenge expiry must fail closed.
	expired := []zruntime.Claim{makeDraft(t, "Expiry One", "expiry one body\n"), makeDraft(t, "Expiry Two", "expiry two body\n")}
	expiryItems := []zruntime.ChallengeItem{{ClaimID: expired[0].ID}, {ClaimID: expired[1].ID}}
	expiryPrepared, err := store.PrepareBatchChallenge("research", expiryItems)
	if err != nil {
		t.Fatalf("PrepareBatchChallenge(expiry) error = %v", err)
	}
	lateStore := zruntime.ChallengeStore{Paths: e.paths, Now: func() time.Time {
		return evalFixedNow().Add(16 * time.Minute)
	}}
	if _, err := lateStore.GrantItems("research", expiryPrepared.Challenge.ID, []string{expired[0].ID}, []string{expired[1].ID}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("late GrantItems error = %v, want expired", err)
	}
	steps = append(steps, evalStep("challenge_expiry", "error:expired"))

	// Token expiry: a valid grant applied after the 5-minute token TTL (and
	// before the challenge TTL) must fail closed.
	tokenDrafts := []zruntime.Claim{makeDraft(t, "Token One", "token one body\n"), makeDraft(t, "Token Two", "token two body\n")}
	tokenItems := []zruntime.ChallengeItem{{ClaimID: tokenDrafts[0].ID}, {ClaimID: tokenDrafts[1].ID}}
	tokenPrepared, err := store.PrepareBatchChallenge("research", tokenItems)
	if err != nil {
		t.Fatalf("PrepareBatchChallenge(token expiry) error = %v", err)
	}
	granted, err := (zruntime.ChallengeStore{Paths: e.paths, Now: e.now}).GrantItems("research", tokenPrepared.Challenge.ID, []string{tokenDrafts[0].ID, tokenDrafts[1].ID}, nil)
	if err != nil {
		t.Fatalf("GrantItems() error = %v", err)
	}
	lateApply := zruntime.ClaimStore{Paths: e.paths, Now: func() time.Time {
		return evalFixedNow().Add(6 * time.Minute)
	}}
	if _, err := lateApply.ApplyChallengeBatch("research", tokenPrepared.Challenge.ID, granted.Token, zruntime.ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("late ApplyChallengeBatch error = %v, want token expired", err)
	}
	steps = append(steps, evalStep("token_expiry", "error:token expired"))

	// Digest mismatch: mutating a bound draft between prepare and apply must
	// fail that item only; the untouched item still applies.
	mutated := makeDraft(t, "Mismatch One", "mismatch one original body\n")
	untouched := makeDraft(t, "Mismatch Two", "mismatch two body\n")
	mismatchItems := []zruntime.ChallengeItem{{ClaimID: mutated.ID}, {ClaimID: untouched.ID}}
	mismatchPrepared, err := store.PrepareBatchChallenge("research", mismatchItems)
	if err != nil {
		t.Fatalf("PrepareBatchChallenge(mismatch) error = %v", err)
	}
	mutatedPath := filepath.Join(e.paths.WorkspacesDir, "research", "wiki", "projects", mutated.ID+".md")
	contents, err := os.ReadFile(mutatedPath)
	if err != nil {
		t.Fatalf("ReadFile(draft) error = %v", err)
	}
	if err := os.WriteFile(mutatedPath, []byte(strings.Replace(string(contents), "mismatch one original body", "mismatch one tampered body", 1)), 0o600); err != nil {
		t.Fatalf("WriteFile(mutated draft) error = %v", err)
	}
	mismatchGranted, err := (zruntime.ChallengeStore{Paths: e.paths, Now: e.now}).GrantItems("research", mismatchPrepared.Challenge.ID, []string{mutated.ID, untouched.ID}, nil)
	if err != nil {
		t.Fatalf("GrantItems(mismatch) error = %v", err)
	}
	result, err := store.ApplyChallengeBatch("research", mismatchPrepared.Challenge.ID, mismatchGranted.Token, zruntime.ClaimMutationOptions{})
	if err != nil {
		t.Fatalf("ApplyChallengeBatch(mismatch) error = %v", err)
	}
	if result.Items[0].Status != zruntime.BatchApplyItemFailed || !strings.Contains(result.Items[0].Error, "stale") {
		t.Fatalf("mutated item = %#v, want failed with stale digest", result.Items[0])
	}
	if result.Items[1].Status != zruntime.BatchApplyItemApplied {
		t.Fatalf("untouched item = %#v, want applied", result.Items[1])
	}
	steps = append(steps, evalStep("digest_mismatch", "item0:failed(stale);item1:applied"))
	return steps
}

func digestSuffix(digest string) string {
	if len(digest) <= 16 {
		return digest
	}
	return digest[len(digest)-16:]
}

func newEvalClaimID(t *testing.T) string {
	t.Helper()
	id, err := zruntime.NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	return id
}

func resultTextOf(res *mcp.CallToolResult) string {
	if res == nil {
		return "<nil>"
	}
	return evalResultText(res)
}
