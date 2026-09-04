package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func batchClaimIDs() []string {
	return []string{approvalTestClaimID(1), approvalTestClaimID(2), approvalTestClaimID(3)}
}

func writeBatchDrafts(t *testing.T, store ClaimStore, ids []string) {
	t.Helper()
	for _, id := range ids {
		if _, err := store.WriteDraft("research", validStoreClaim(id, ClaimBasisOwner)); err != nil {
			t.Fatalf("WriteDraft(%s) error = %v", id, err)
		}
	}
}

func batchItems(ids []string) []ChallengeItem {
	items := make([]ChallengeItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, ChallengeItem{ClaimID: id})
	}
	return items
}

func batchItemClaimIDs(items []ChallengeItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ClaimID)
	}
	return ids
}

func TestBatchChallenge(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	challengeStore := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	ids := batchClaimIDs()
	writeBatchDrafts(t, store, ids)

	prepared, err := store.PrepareBatchChallenge("research", batchItems(ids))
	if err != nil {
		t.Fatalf("PrepareBatchChallenge() error = %v", err)
	}
	challenge := prepared.Challenge
	if challenge.Schema != ChallengeSchemaVersion {
		t.Fatalf("schema = %q, want %q", challenge.Schema, ChallengeSchemaVersion)
	}
	if challenge.Operation != ChallengeOperationApprove {
		t.Fatalf("operation = %q, want approve", challenge.Operation)
	}
	if got := batchItemClaimIDs(challenge.Items); !sameStrings(got, ids) {
		t.Fatalf("items = %v, want %v in prepare order", got, ids)
	}
	for _, item := range challenge.Items {
		want, err := store.CanonicalDraftDigest("research", item.ClaimID)
		if err != nil {
			t.Fatalf("CanonicalDraftDigest(%s) error = %v", item.ClaimID, err)
		}
		if item.CanonicalDraftDigest != want {
			t.Fatalf("item %s digest = %q, want canonical %q", item.ClaimID, item.CanonicalDraftDigest, want)
		}
	}
	wantExpiry := fixedClaimStoreNow().UTC().Add(challengeLifetime).Format(time.RFC3339)
	if challenge.ExpiresAt != wantExpiry {
		t.Fatalf("ExpiresAt = %q, want %q", challenge.ExpiresAt, wantExpiry)
	}
	if challenge.TokenSHA256 != "" || challenge.TokenExpiresAt != "" || challenge.Granted {
		t.Fatalf("PrepareBatchChallenge() persisted token material: %#v", challenge)
	}
	read, err := challengeStore.Read("research", challenge.ID)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if read.ActionDigest != challenge.ActionDigest {
		t.Fatalf("Read() digest = %q, want %q", read.ActionDigest, challenge.ActionDigest)
	}

	// The action digest covers all bound items in order.
	reordered := []string{ids[2], ids[0], ids[1]}
	other, err := store.PrepareBatchChallenge("research", batchItems(reordered))
	if err != nil {
		t.Fatalf("PrepareBatchChallenge(reordered) error = %v", err)
	}
	if other.Challenge.ActionDigest == challenge.ActionDigest {
		t.Fatal("reordered items produced the same action digest")
	}

	// Tampering with the bound items fails closed on every read.
	path := filepath.Join(paths.WorkspacesDir, "research", ".zbrain", "challenges", challenge.ID+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var tampered Challenge
	if err := json.Unmarshal(original, &tampered); err != nil {
		t.Fatalf("unmarshal challenge: %v", err)
	}
	tampered.Items[0], tampered.Items[1] = tampered.Items[1], tampered.Items[0]
	encoded, err := json.MarshalIndent(tampered, "", "  ")
	if err != nil {
		t.Fatalf("marshal challenge: %v", err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), runtimeMetadataMode); err != nil {
		t.Fatalf("write tampered challenge: %v", err)
	}
	if _, err := challengeStore.Read("research", challenge.ID); err == nil || !strings.Contains(err.Error(), "action digest mismatch") {
		t.Fatalf("Read(tampered items) error = %v, want action digest mismatch", err)
	}

	// A single-item challenge keeps working exactly as before.
	single, err := challengeStore.Prepare("research", ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              ids[0],
		CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("Prepare(single) error = %v", err)
	}
	if len(single.Challenge.Items) != 0 || single.Challenge.GrantedItems != nil {
		t.Fatalf("single challenge gained batch fields: %#v", single.Challenge)
	}
	readSingle, err := challengeStore.Read("research", single.Challenge.ID)
	if err != nil {
		t.Fatalf("Read(single) error = %v", err)
	}
	if readSingle.ActionDigest != single.Challenge.ActionDigest {
		t.Fatalf("Read(single) digest = %q, want %q", readSingle.ActionDigest, single.Challenge.ActionDigest)
	}

	if _, err := store.PrepareBatchChallenge("research", nil); err == nil {
		t.Fatal("PrepareBatchChallenge(empty) error = nil, want rejection")
	}
	duplicate := batchItems([]string{ids[0], ids[0]})
	if _, err := store.PrepareBatchChallenge("research", duplicate); err == nil {
		t.Fatal("PrepareBatchChallenge(duplicate) error = nil, want rejection")
	}
	batchOfOne, err := store.PrepareBatchChallenge("research", batchItems([]string{ids[1]}))
	if err != nil {
		t.Fatalf("PrepareBatchChallenge(single item) error = %v", err)
	}
	if len(batchOfOne.Challenge.Items) != 1 {
		t.Fatalf("batch of one items = %d, want 1", len(batchOfOne.Challenge.Items))
	}
}

func TestBatchGrant(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	challengeStore := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	ids := batchClaimIDs()
	writeBatchDrafts(t, store, ids)
	prepare := func(t *testing.T, order []string) PreparedChallenge {
		t.Helper()
		prepared, err := store.PrepareBatchChallenge("research", batchItems(order))
		if err != nil {
			t.Fatalf("PrepareBatchChallenge() error = %v", err)
		}
		return prepared
	}

	t.Run("grant all issues one token", func(t *testing.T) {
		prepared := prepare(t, ids)
		granted, err := challengeStore.GrantItems("research", prepared.Challenge.ID, ids, nil)
		if err != nil {
			t.Fatalf("GrantItems() error = %v", err)
		}
		if granted.Token == "" || !granted.Granted || granted.GrantedAt == "" {
			t.Fatalf("GrantItems() state = %#v", granted.Challenge)
		}
		if !sameStrings(granted.GrantedItems, ids) || len(granted.SkippedItems) != 0 {
			t.Fatalf("granted items = %#v", granted.Challenge)
		}
		if _, err := challengeStore.Verify("research", prepared.Challenge.ID, granted.Token); err != nil {
			t.Fatalf("Verify() error = %v", err)
		}
		if _, err := challengeStore.GrantItems("research", prepared.Challenge.ID, ids, nil); err == nil || !strings.Contains(err.Error(), "already been owner-granted") {
			t.Fatalf("GrantItems(repeated) error = %v, want one-shot rejection", err)
		}
	})

	t.Run("skip one of three", func(t *testing.T) {
		prepared := prepare(t, ids)
		granted, err := challengeStore.GrantItems("research", prepared.Challenge.ID, []string{ids[0], ids[2]}, []string{ids[1]})
		if err != nil {
			t.Fatalf("GrantItems() error = %v", err)
		}
		if granted.Token == "" {
			t.Fatal("GrantItems() token is empty")
		}
		if !sameStrings(granted.GrantedItems, []string{ids[0], ids[2]}) || !sameStrings(granted.SkippedItems, []string{ids[1]}) {
			t.Fatalf("decisions = %#v", granted.Challenge)
		}
		if _, err := challengeStore.Read("research", prepared.Challenge.ID); err != nil {
			t.Fatalf("Read(after skip) error = %v", err)
		}
	})

	t.Run("partial grant issues exactly one token", func(t *testing.T) {
		prepared := prepare(t, ids)
		granted, err := challengeStore.GrantItems("research", prepared.Challenge.ID, []string{ids[1]}, []string{ids[0], ids[2]})
		if err != nil {
			t.Fatalf("GrantItems() error = %v", err)
		}
		if len(granted.Token) != 64 {
			t.Fatalf("GrantItems() token length = %d, want a single 32-byte hex token", len(granted.Token))
		}
		if !sameStrings(granted.GrantedItems, []string{ids[1]}) || !sameStrings(granted.SkippedItems, []string{ids[0], ids[2]}) {
			t.Fatalf("decisions = %#v", granted.Challenge)
		}
	})

	t.Run("skip all grants nothing", func(t *testing.T) {
		prepared := prepare(t, ids)
		if _, err := challengeStore.GrantItems("research", prepared.Challenge.ID, nil, ids); err == nil || !strings.Contains(err.Error(), "granted no items") {
			t.Fatalf("GrantItems(skip all) error = %v, want no-granted-items rejection", err)
		}
		unchanged, err := challengeStore.Read("research", prepared.Challenge.ID)
		if err != nil {
			t.Fatalf("Read(after skip all) error = %v", err)
		}
		if unchanged.Granted || unchanged.TokenSHA256 != "" || len(unchanged.SkippedItems) != 0 {
			t.Fatalf("skip all mutated the challenge: %#v", unchanged)
		}
	})

	t.Run("decisions must partition the bound items", func(t *testing.T) {
		prepared := prepare(t, ids)
		if _, err := challengeStore.GrantItems("research", prepared.Challenge.ID, []string{ids[0]}, nil); err == nil {
			t.Fatal("GrantItems(partial coverage) error = nil")
		}
		if _, err := challengeStore.GrantItems("research", prepared.Challenge.ID, []string{"clm_ffffffffffffffffffffffffffffffff"}, []string{ids[1], ids[2]}); err == nil {
			t.Fatal("GrantItems(unknown item) error = nil")
		}
		unchanged, err := challengeStore.Read("research", prepared.Challenge.ID)
		if err != nil {
			t.Fatalf("Read(after invalid decisions) error = %v", err)
		}
		if unchanged.Granted || unchanged.TokenSHA256 != "" {
			t.Fatalf("invalid decisions mutated the challenge: %#v", unchanged)
		}
	})
}

func TestBatchApply(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	challengeStore := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	ids := batchClaimIDs()
	writeBatchDrafts(t, store, ids)

	prepared, err := store.PrepareBatchChallenge("research", batchItems(ids))
	if err != nil {
		t.Fatalf("PrepareBatchChallenge() error = %v", err)
	}
	granted, err := challengeStore.GrantItems("research", prepared.Challenge.ID, []string{ids[0], ids[2]}, []string{ids[1]})
	if err != nil {
		t.Fatalf("GrantItems() error = %v", err)
	}
	token := granted.Token

	result, err := store.ApplyChallengeBatch("research", prepared.Challenge.ID, token, ClaimMutationOptions{})
	if err != nil {
		t.Fatalf("ApplyChallengeBatch() error = %v", err)
	}
	if result.ChallengeID != prepared.Challenge.ID || len(result.Items) != 3 {
		t.Fatalf("ApplyChallengeBatch() result = %#v", result)
	}
	wantStatus := map[string]string{
		ids[0]: BatchApplyItemApplied,
		ids[1]: BatchApplyItemSkipped,
		ids[2]: BatchApplyItemApplied,
	}
	for _, item := range result.Items {
		if wantStatus[item.ClaimID] != item.Status {
			t.Fatalf("item %s status = %q, want %q (result %#v)", item.ClaimID, item.Status, wantStatus[item.ClaimID], item)
		}
		if item.Status == BatchApplyItemSkipped && item.Path != "" {
			t.Fatalf("skipped item %s reported a path: %#v", item.ClaimID, item)
		}
	}
	for _, id := range []string{ids[0], ids[2]} {
		claim, err := store.Read("research", id)
		if err != nil {
			t.Fatalf("Read(%s) error = %v", id, err)
		}
		if claim.Status != ClaimStatusApproved {
			t.Fatalf("claim %s status = %q, want approved", id, claim.Status)
		}
	}
	skippedClaim, err := store.Read("research", ids[1])
	if err != nil {
		t.Fatalf("Read(skipped) error = %v", err)
	}
	if skippedClaim.Status != ClaimStatusDraft || len(skippedClaim.Transitions) != 0 {
		t.Fatalf("skipped claim changed: %#v", skippedClaim)
	}
	consumed, err := challengeStore.Read("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Read(after apply) error = %v", err)
	}
	if !consumed.Consumed {
		t.Fatalf("challenge not consumed after apply: %#v", consumed)
	}

	// A replayed token fails closed for the whole batch.
	if _, err := store.ApplyChallengeBatch("research", prepared.Challenge.ID, token, ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("ApplyChallengeBatch(replay) error = %v, want already consumed", err)
	}

	// An ungranted batch challenge fails closed before any item is applied.
	ungranted := func(t *testing.T) PreparedChallenge {
		t.Helper()
		prepared, err := store.PrepareBatchChallenge("research", batchItems([]string{ids[1]}))
		if err != nil {
			t.Fatalf("PrepareBatchChallenge() error = %v", err)
		}
		return prepared
	}
	candidate := ungranted(t)
	if _, err := store.ApplyChallengeBatch("research", candidate.Challenge.ID, "not-issued", ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "has not been owner-granted") {
		t.Fatalf("ApplyChallengeBatch(ungranted) error = %v, want owner-grant rejection", err)
	}
	stillDraft, err := store.Read("research", ids[1])
	if err != nil {
		t.Fatalf("Read(after ungranted apply) error = %v", err)
	}
	if stillDraft.Status != ClaimStatusDraft {
		t.Fatalf("ungranted apply mutated a claim: %#v", stillDraft)
	}
}

func TestBatchApplyIsolatesFailedItems(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	challengeStore := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	ids := batchClaimIDs()
	writeBatchDrafts(t, store, ids)

	prepared, err := store.PrepareBatchChallenge("research", batchItems(ids))
	if err != nil {
		t.Fatalf("PrepareBatchChallenge() error = %v", err)
	}
	granted, err := challengeStore.GrantItems("research", prepared.Challenge.ID, ids, nil)
	if err != nil {
		t.Fatalf("GrantItems() error = %v", err)
	}
	// The canonical draft of one bound claim changes after prepare.
	changed := validStoreClaim(ids[1], ClaimBasisOwner)
	changed.Body = "Changed after challenge\n"
	if _, err := store.WriteDraft("research", changed); err != nil {
		t.Fatalf("WriteDraft(changed) error = %v", err)
	}

	result, err := store.ApplyChallengeBatch("research", prepared.Challenge.ID, granted.Token, ClaimMutationOptions{})
	if err != nil {
		t.Fatalf("ApplyChallengeBatch() error = %v", err)
	}
	statuses := map[string]string{}
	for _, item := range result.Items {
		statuses[item.ClaimID] = item.Status
		if item.Status == BatchApplyItemFailed && !strings.Contains(item.Error, "canonical draft digest is stale") {
			t.Fatalf("failed item %s error = %q, want stale digest", item.ClaimID, item.Error)
		}
	}
	if statuses[ids[1]] != BatchApplyItemFailed || statuses[ids[0]] != BatchApplyItemApplied || statuses[ids[2]] != BatchApplyItemApplied {
		t.Fatalf("per-item statuses = %#v", result.Items)
	}
	failedClaim, err := store.Read("research", ids[1])
	if err != nil {
		t.Fatalf("Read(failed) error = %v", err)
	}
	if failedClaim.Status != ClaimStatusDraft {
		t.Fatalf("failed claim was mutated: %#v", failedClaim)
	}
	for _, id := range []string{ids[0], ids[2]} {
		claim, err := store.Read("research", id)
		if err != nil {
			t.Fatalf("Read(%s) error = %v", id, err)
		}
		if claim.Status != ClaimStatusApproved {
			t.Fatalf("claim %s status = %q, want approved", id, claim.Status)
		}
	}
}

func TestBatchApplyFailsClosedOnExpiryAndWrongToken(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	challengeStore := ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}
	ids := batchClaimIDs()
	writeBatchDrafts(t, store, ids)

	prepared, err := store.PrepareBatchChallenge("research", batchItems(ids))
	if err != nil {
		t.Fatalf("PrepareBatchChallenge() error = %v", err)
	}
	granted, err := challengeStore.GrantItems("research", prepared.Challenge.ID, ids, nil)
	if err != nil {
		t.Fatalf("GrantItems() error = %v", err)
	}

	wrongTokenStore := store
	if _, err := wrongTokenStore.ApplyChallengeBatch("research", prepared.Challenge.ID, "not-the-token", ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "token mismatch") {
		t.Fatalf("ApplyChallengeBatch(wrong token) error = %v, want token mismatch", err)
	}
	for _, id := range ids {
		claim, err := store.Read("research", id)
		if err != nil {
			t.Fatalf("Read(%s) error = %v", id, err)
		}
		if claim.Status != ClaimStatusDraft {
			t.Fatalf("wrong-token apply mutated claim %s: %#v", id, claim)
		}
	}
	if _, err := challengeStore.Verify("research", prepared.Challenge.ID, granted.Token); err != nil {
		t.Fatalf("Verify(after wrong-token apply) error = %v", err)
	}

	expiredStore := ClaimStore{Paths: paths, Now: func() time.Time {
		return fixedClaimStoreNow().Add(challengeLifetime).Add(time.Microsecond)
	}}
	if _, err := expiredStore.ApplyChallengeBatch("research", prepared.Challenge.ID, granted.Token, ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("ApplyChallengeBatch(expired) error = %v, want expired", err)
	}
	for _, id := range ids {
		claim, err := store.Read("research", id)
		if err != nil {
			t.Fatalf("Read(%s) error = %v", id, err)
		}
		if claim.Status != ClaimStatusDraft {
			t.Fatalf("expired apply mutated claim %s: %#v", id, claim)
		}
	}
}

func TestComputeBatchActionDigestCoversItems(t *testing.T) {
	items := []ChallengeItem{
		{ClaimID: "clm_0123456789abcdef0123456789abcdef", CanonicalDraftDigest: "sha256:" + strings.Repeat("a", 64)},
		{ClaimID: "clm_11111111111111111111111111111111", CanonicalDraftDigest: "sha256:" + strings.Repeat("b", 64)},
	}
	base := ChallengePrepare{Workspace: "research", Operation: ChallengeOperationApprove, Items: items}
	baseline := ComputeChallengeActionDigest(base)
	if !strings.HasPrefix(baseline, "sha256:challenge-v1:") {
		t.Fatalf("batch digest = %q, want sha256:challenge-v1: prefix", baseline)
	}
	mutations := map[string]func(c ChallengePrepare) ChallengePrepare{
		"item_digest": func(c ChallengePrepare) ChallengePrepare {
			c.Items[1].CanonicalDraftDigest = "sha256:" + strings.Repeat("c", 64)
			return c
		},
		"item_claim": func(c ChallengePrepare) ChallengePrepare {
			c.Items[1].ClaimID = "clm_22222222222222222222222222222222"
			return c
		},
		"item_order": func(c ChallengePrepare) ChallengePrepare {
			c.Items = []ChallengeItem{items[1], items[0]}
			return c
		},
		"item_count": func(c ChallengePrepare) ChallengePrepare {
			c.Items = items[:1]
			return c
		},
		"workspace": func(c ChallengePrepare) ChallengePrepare {
			c.Workspace = "other"
			return c
		},
	}
	for name, mutate := range mutations {
		if got := ComputeChallengeActionDigest(mutate(base)); got == baseline {
			t.Fatalf("changing %s did not change the batch action digest", name)
		}
	}
	// The batch digest is domain-separated from the single-claim digest style.
	single := ComputeChallengeActionDigest(ChallengePrepare{
		Workspace:            base.Workspace,
		Operation:            base.Operation,
		ClaimID:              items[0].ClaimID,
		CanonicalDraftDigest: items[0].CanonicalDraftDigest,
	})
	if single == baseline {
		t.Fatal("batch digest collides with the single-claim digest shape")
	}
}
