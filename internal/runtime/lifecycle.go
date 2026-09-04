package runtime

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// claimMutationPlan contains all canonical bytes and validation results needed
// for one lifecycle mutation. The plan is built before a challenge token is
// consumed and committed afterward while the same workspace lock is held.
type claimMutationPlan struct {
	Claim   Claim
	Pending *PendingTransition
}

func (store ClaimStore) prepareApproveUnlocked(workspace string, id string, options ClaimMutationOptions) (claimMutationPlan, error) {
	normalizedOptions, err := options.normalized()
	if err != nil {
		return claimMutationPlan{}, err
	}
	claim, err := store.Read(workspace, id)
	if err != nil {
		return claimMutationPlan{}, err
	}
	if claim.Status != ClaimStatusDraft {
		return claimMutationPlan{}, fmt.Errorf("claim %s is %s; only draft claims can be approved", id, claim.Status)
	}
	if err := ValidateClaimApproval(claim); err != nil {
		return claimMutationPlan{}, err
	}
	evidenceValidator, err := store.validateApprovalReferences(workspace, claim)
	if err != nil {
		return claimMutationPlan{}, err
	}
	sources, err := store.claimSources(workspace, claim.EvidenceIDs, evidenceValidator)
	if err != nil {
		return claimMutationPlan{}, err
	}

	seenOldIDs := make(map[string]struct{}, len(claim.Supersedes))
	oldClaims := make([]Claim, 0, len(claim.Supersedes))
	for _, oldID := range claim.Supersedes {
		if _, ok := seenOldIDs[oldID]; ok {
			return claimMutationPlan{}, fmt.Errorf("claim %s supersedes duplicate claim %s", id, oldID)
		}
		seenOldIDs[oldID] = struct{}{}
		old, err := store.Read(workspace, oldID)
		if err != nil {
			return claimMutationPlan{}, err
		}
		if old.Status != ClaimStatusApproved {
			return claimMutationPlan{}, fmt.Errorf("claim %s is %s; only approved claims can be superseded", oldID, old.Status)
		}
		if err := VerifyClaimDigest(old); err != nil {
			return claimMutationPlan{}, fmt.Errorf("verify superseded claim %s: %w", oldID, err)
		}
		oldClaims = append(oldClaims, old)
	}

	verifiedAt := store.now().UTC().Format(time.RFC3339)
	claim.Schema = ""
	claim.Type = OKFClaimType
	claim.Status = ClaimStatusApproved
	claim.Sources = sources
	claim.VerifiedAt = verifiedAt
	claim.VerifiedBy = normalizedOptions.VerifiedBy
	claim.VerifiedDigest = ""
	transitionKind := ClaimTransitionApprove
	if len(oldClaims) > 0 {
		transitionKind = ClaimTransitionSupersede
	}
	claim.Transitions = appendClaimTransition(claim, ClaimTransition{
		Kind:            transitionKind,
		At:              verifiedAt,
		By:              claim.VerifiedBy,
		RelatedClaimIDs: append([]string(nil), claim.Supersedes...),
		Authorization:   cloneClaimTransitionAuthorization(normalizedOptions.Authorization),
	})
	digest, err := ClaimVerificationDigest(claim)
	if err != nil {
		return claimMutationPlan{}, err
	}
	claim.VerifiedDigest = digest

	for i := range oldClaims {
		oldClaims[i].Status = ClaimStatusSuperseded
		oldClaims[i].Transitions = appendClaimTransition(oldClaims[i], ClaimTransition{
			Kind:                    ClaimTransitionSupersede,
			At:                      verifiedAt,
			By:                      claim.VerifiedBy,
			RelatedClaimIDs:         []string{claim.ID},
			PriorVerificationDigest: oldClaims[i].VerifiedDigest,
			Authorization:           cloneClaimTransitionAuthorization(normalizedOptions.Authorization),
		})
	}
	var pending *PendingTransition
	if len(oldClaims) > 0 {
		prepared, err := store.pendingSupersession(workspace, claim, oldClaims)
		if err != nil {
			return claimMutationPlan{}, err
		}
		pending = &prepared
	}
	return claimMutationPlan{Claim: claim, Pending: pending}, nil
}

func (store ClaimStore) commitApproveUnlocked(workspace string, plan claimMutationPlan) (Claim, error) {
	if _, err := beginCanonicalMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	if plan.Pending != nil {
		if err := writePendingTransitionUnlocked(store.Paths, workspace, *plan.Pending); err != nil {
			return Claim{}, err
		}
		runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
		if err := recoverPendingTransitionUnlocked(store.Paths, workspace); err != nil {
			return Claim{}, err
		}
		return plan.Claim, nil
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
	if err := store.writeExisting(workspace, plan.Claim); err != nil {
		return Claim{}, err
	}
	return plan.Claim, nil
}

func (store ClaimStore) prepareRevokeUnlocked(workspace string, id string, reason string, options ClaimMutationOptions) (claimMutationPlan, error) {
	normalizedOptions, err := options.normalized()
	if err != nil {
		return claimMutationPlan{}, err
	}
	claim, err := store.Read(workspace, id)
	if err != nil {
		return claimMutationPlan{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return claimMutationPlan{}, fmt.Errorf("revoke reason is required")
	}
	if claim.Status != ClaimStatusApproved {
		return claimMutationPlan{}, fmt.Errorf("claim %s is %s; only approved claims can be revoked", id, claim.Status)
	}
	if err := VerifyClaimDigest(claim); err != nil {
		return claimMutationPlan{}, fmt.Errorf("verify claim %s before revoke: %w", id, err)
	}
	claim.Status = ClaimStatusRevoked
	claim.Transitions = appendClaimTransition(claim, ClaimTransition{
		Kind:                    ClaimTransitionRevoke,
		At:                      store.now().UTC().Format(time.RFC3339),
		By:                      normalizedOptions.VerifiedBy,
		Reason:                  strings.TrimSpace(reason),
		RelatedClaimIDs:         []string{id},
		PriorVerificationDigest: claim.VerifiedDigest,
		Authorization:           cloneClaimTransitionAuthorization(normalizedOptions.Authorization),
	})
	return claimMutationPlan{Claim: claim}, nil
}

func (store ClaimStore) commitRevokeUnlocked(workspace string, plan claimMutationPlan) (Claim, error) {
	if _, err := beginCanonicalMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}
	runWorkspaceGenerationTestHook(workspaceGenerationHookBeforeCanonicalWrite)
	if err := store.writeExisting(workspace, plan.Claim); err != nil {
		return Claim{}, err
	}
	return plan.Claim, nil
}

// CanonicalDigest returns the digest of the current canonical claim after
// resolving it under the workspace read lock.
func (store ClaimStore) CanonicalDigest(workspace string, id string) (Claim, string, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, false)
	if err != nil {
		return Claim{}, "", err
	}
	defer func() { _ = lock.Close() }()
	return store.canonicalDigestUnlocked(workspace, id)
}

// CanonicalDraftDigest returns the current canonical claim digest. The claim
// must still satisfy the caller's lifecycle-specific status checks before it
// is used in a challenge.
func (store ClaimStore) CanonicalDraftDigest(workspace string, id string) (string, error) {
	_, digest, err := store.CanonicalDigest(workspace, id)
	return digest, err
}

func (store ClaimStore) canonicalDigestUnlocked(workspace string, id string) (Claim, string, error) {
	claim, err := store.Read(workspace, id)
	if err != nil {
		return Claim{}, "", err
	}
	digest, err := ClaimCanonicalDigest(claim)
	if err != nil {
		return Claim{}, "", fmt.Errorf("compute canonical claim digest: %w", err)
	}
	return claim, digest, nil
}

// PrepareChallenge atomically validates the current canonical claim and
// persists a challenge under the workspace lock. The general ChallengeStore
// Prepare method remains available for the local owner ceremony, which does
// not require a canonical claim digest.
func (store ClaimStore) PrepareChallenge(workspace string, prepare ChallengePrepare) (PreparedChallenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return PreparedChallenge{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return PreparedChallenge{}, err
	}
	claim, digest, err := store.canonicalDigestUnlocked(workspace, prepare.ClaimID)
	if err != nil {
		return PreparedChallenge{}, err
	}
	if strings.TrimSpace(prepare.CanonicalDraftDigest) == "" {
		prepare.CanonicalDraftDigest = digest
	}
	challenge := Challenge{
		Workspace:               workspace,
		Operation:               prepare.Operation,
		ClaimID:                 prepare.ClaimID,
		CanonicalDraftDigest:    prepare.CanonicalDraftDigest,
		SupersededIDs:           append([]string(nil), prepare.SupersededIDs...),
		PriorVerificationDigest: prepare.PriorVerificationDigest,
		RevokeReason:            prepare.RevokeReason,
	}
	if err := store.validateChallengeAgainstClaim(challenge, claim, digest); err != nil {
		return PreparedChallenge{}, err
	}
	return (ChallengeStore(store)).prepareUnlocked(workspace, prepare)
}

// PrepareBatchChallenge atomically validates every bound draft claim against
// its current canonical state and persists one batch challenge under the
// workspace lock. An item digest left empty is bound from the current
// canonical digest; a non-empty digest must match the canonical state.
func (store ClaimStore) PrepareBatchChallenge(workspace string, items []ChallengeItem) (PreparedChallenge, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return PreparedChallenge{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return PreparedChallenge{}, err
	}
	bound := make([]ChallengeItem, len(items))
	copy(bound, items)
	for i, item := range bound {
		claim, digest, err := store.canonicalDigestUnlocked(workspace, item.ClaimID)
		if err != nil {
			return PreparedChallenge{}, err
		}
		if strings.TrimSpace(item.CanonicalDraftDigest) == "" {
			bound[i].CanonicalDraftDigest = digest
		}
		if err := store.validateChallengeAgainstClaim(Challenge{
			ID:                   "batch",
			Workspace:            workspace,
			Operation:            ChallengeOperationApprove,
			ClaimID:              claim.ID,
			CanonicalDraftDigest: bound[i].CanonicalDraftDigest,
		}, claim, digest); err != nil {
			return PreparedChallenge{}, err
		}
	}
	return (ChallengeStore(store)).prepareBatchUnlocked(workspace, bound)
}

// ApplyChallenge validates and consumes a challenge token, then commits the
// corresponding claim transition while retaining the same exclusive workspace
// lock for the entire operation. Semantic validation completes before token
// consumption; commit failures still leave the token consumed and therefore
// cannot be retried as an unauthorized mutation.
func (store ClaimStore) ApplyChallenge(workspace string, challengeID string, token string, options ClaimMutationOptions) (Claim, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return Claim{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return Claim{}, err
	}

	challengeStore := ChallengeStore(store)
	challenge, err := challengeStore.readChallengeUnlocked(workspace, challengeID)
	if err != nil {
		return Claim{}, err
	}
	if err := challengeStore.validateToken(challenge, token); err != nil {
		return Claim{}, err
	}
	if !challenge.Granted {
		return Claim{}, fmt.Errorf("challenge %s has not been owner-granted", challenge.ID)
	}
	claim, digest, err := store.canonicalDigestUnlocked(workspace, challenge.ClaimID)
	if err != nil {
		return Claim{}, err
	}
	if err := store.validateChallengeAgainstClaim(challenge, claim, digest); err != nil {
		return Claim{}, err
	}
	if options.Authorization != nil && options.Authorization.ChallengeID != challenge.ID {
		return Claim{}, fmt.Errorf("claim transition authorization challenge id does not match challenge %s", challenge.ID)
	}

	var plan claimMutationPlan
	switch challenge.Operation {
	case ChallengeOperationApprove, ChallengeOperationSupersede:
		plan, err = store.prepareApproveUnlocked(workspace, challenge.ClaimID, options)
	case ChallengeOperationRevoke:
		plan, err = store.prepareRevokeUnlocked(workspace, challenge.ClaimID, challenge.RevokeReason, options)
	default:
		return Claim{}, fmt.Errorf("challenge %s operation %q is not supported", challenge.ID, challenge.Operation)
	}
	if err != nil {
		return Claim{}, err
	}
	if _, err := challengeStore.consumeUnlocked(workspace, challenge.ID, token); err != nil {
		return Claim{}, err
	}
	if challenge.Operation == ChallengeOperationRevoke {
		return store.commitRevokeUnlocked(workspace, plan)
	}
	return store.commitApproveUnlocked(workspace, plan)
}

// Per-item outcome statuses reported by ApplyChallengeBatch.
const (
	BatchApplyItemApplied string = "applied"
	BatchApplyItemSkipped string = "skipped"
	BatchApplyItemFailed  string = "failed"
)

// BatchApplyItemResult reports the outcome of one bound item.
type BatchApplyItemResult struct {
	ClaimID string `json:"claim_id"`
	Status  string `json:"status"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

// BatchApplyResult reports the per-item outcome of applying a batch challenge.
type BatchApplyResult struct {
	ChallengeID string                 `json:"challenge_id"`
	Items       []BatchApplyItemResult `json:"items"`
}

// ApplyChallengeBatch validates and consumes a batch challenge token exactly
// once, then applies only the owner-granted items while retaining the same
// exclusive workspace lock. Every granted item is independently revalidated
// against its current canonical state; a failed item never aborts the batch
// silently and is reported as failed without being applied. Skipped items are
// reported without any mutation. Invalid tokens, expired challenges, or a
// mismatched workspace fail closed before any item is applied.
func (store ClaimStore) ApplyChallengeBatch(workspace string, challengeID string, token string, options ClaimMutationOptions) (BatchApplyResult, error) {
	lock, err := acquireWorkspaceLock(store.Paths, workspace, true)
	if err != nil {
		return BatchApplyResult{}, err
	}
	defer func() { _ = lock.Close() }()
	if err := recoverPendingTransitionForMutationUnlocked(store.Paths, workspace); err != nil {
		return BatchApplyResult{}, err
	}
	challengeStore := ChallengeStore(store)
	challenge, err := challengeStore.readChallengeUnlocked(workspace, challengeID)
	if err != nil {
		return BatchApplyResult{}, err
	}
	if len(challenge.Items) == 0 {
		return BatchApplyResult{}, fmt.Errorf("challenge %s is not a batch challenge", challenge.ID)
	}
	if err := challengeStore.validateToken(challenge, token); err != nil {
		return BatchApplyResult{}, err
	}
	if options.Authorization != nil && options.Authorization.ChallengeID != challenge.ID {
		return BatchApplyResult{}, fmt.Errorf("claim transition authorization challenge id does not match challenge %s", challenge.ID)
	}
	skipped := make(map[string]struct{}, len(challenge.SkippedItems))
	for _, id := range challenge.SkippedItems {
		skipped[id] = struct{}{}
	}
	results := make([]BatchApplyItemResult, len(challenge.Items))
	type validatedItem struct {
		index int
		item  ChallengeItem
	}
	validated := make([]validatedItem, 0, len(challenge.Items))
	for i, item := range challenge.Items {
		results[i] = BatchApplyItemResult{ClaimID: item.ClaimID}
		if _, ok := skipped[item.ClaimID]; ok {
			results[i].Status = BatchApplyItemSkipped
			continue
		}
		claim, digest, err := store.canonicalDigestUnlocked(workspace, item.ClaimID)
		if err == nil {
			err = store.validateChallengeAgainstClaim(Challenge{
				ID:                   challenge.ID,
				Workspace:            workspace,
				Operation:            ChallengeOperationApprove,
				ClaimID:              item.ClaimID,
				CanonicalDraftDigest: item.CanonicalDraftDigest,
			}, claim, digest)
		}
		if err != nil {
			results[i].Status = BatchApplyItemFailed
			results[i].Error = err.Error()
			continue
		}
		validated = append(validated, validatedItem{index: i, item: item})
	}
	if _, err := challengeStore.consumeUnlocked(workspace, challenge.ID, token); err != nil {
		return BatchApplyResult{}, err
	}
	for _, entry := range validated {
		plan, err := store.prepareApproveUnlocked(workspace, entry.item.ClaimID, options)
		if err != nil {
			results[entry.index].Status = BatchApplyItemFailed
			results[entry.index].Error = err.Error()
			continue
		}
		claim, err := store.commitApproveUnlocked(workspace, plan)
		if err != nil {
			results[entry.index].Status = BatchApplyItemFailed
			results[entry.index].Error = err.Error()
			continue
		}
		results[entry.index].Status = BatchApplyItemApplied
		results[entry.index].Path = claim.Path
	}
	return BatchApplyResult{ChallengeID: challenge.ID, Items: results}, nil
}

func (store ClaimStore) validateChallengeAgainstClaim(challenge Challenge, claim Claim, digest string) error {
	if challenge.ClaimID != claim.ID {
		return fmt.Errorf("challenge %s claim %q does not match canonical claim %q", challenge.ID, challenge.ClaimID, claim.ID)
	}
	if strings.TrimSpace(challenge.CanonicalDraftDigest) == "" {
		return fmt.Errorf("challenge %s canonical draft digest is required", challenge.ID)
	}
	if challenge.CanonicalDraftDigest != digest {
		return fmt.Errorf("challenge %s canonical draft digest is stale", challenge.ID)
	}
	expectedSuperseded := append([]string(nil), claim.Supersedes...)
	sort.Strings(expectedSuperseded)
	actualSuperseded := append([]string(nil), challenge.SupersededIDs...)
	sort.Strings(actualSuperseded)
	if !sameStrings(actualSuperseded, expectedSuperseded) {
		return fmt.Errorf("challenge %s superseded IDs do not match the canonical claim", challenge.ID)
	}
	switch challenge.Operation {
	case ChallengeOperationApprove:
		if claim.Status != ClaimStatusDraft {
			return fmt.Errorf("claim %s is %s; only draft claims can be approved", claim.ID, claim.Status)
		}
		if len(expectedSuperseded) != 0 {
			return fmt.Errorf("challenge %s approve action has superseded claims; use supersede", challenge.ID)
		}
		if challenge.PriorVerificationDigest != "" {
			return fmt.Errorf("challenge %s prior verification digest is not valid for approve", challenge.ID)
		}
		if challenge.RevokeReason != "" {
			return fmt.Errorf("challenge %s revoke reason is not valid for approve", challenge.ID)
		}
	case ChallengeOperationSupersede:
		if claim.Status != ClaimStatusDraft {
			return fmt.Errorf("claim %s is %s; only draft claims can be superseded", claim.ID, claim.Status)
		}
		if len(expectedSuperseded) == 0 {
			return fmt.Errorf("challenge %s supersede action requires a superseded claim", challenge.ID)
		}
		prior, err := store.firstSupersededVerificationDigest(challenge.Workspace, expectedSuperseded)
		if err != nil {
			return err
		}
		if challenge.PriorVerificationDigest != prior {
			return fmt.Errorf("challenge %s prior verification digest is stale", challenge.ID)
		}
		if challenge.RevokeReason != "" {
			return fmt.Errorf("challenge %s revoke reason is not valid for supersede", challenge.ID)
		}
	case ChallengeOperationRevoke:
		if claim.Status != ClaimStatusApproved {
			return fmt.Errorf("claim %s is %s; only approved claims can be revoked", claim.ID, claim.Status)
		}
		if err := VerifyClaimDigest(claim); err != nil {
			return fmt.Errorf("verify claim %s before revoke: %w", claim.ID, err)
		}
		if challenge.PriorVerificationDigest != claim.VerifiedDigest {
			return fmt.Errorf("challenge %s prior verification digest is stale", challenge.ID)
		}
		if strings.TrimSpace(challenge.RevokeReason) == "" {
			return fmt.Errorf("challenge %s revoke reason is required", challenge.ID)
		}
	default:
		return fmt.Errorf("challenge %s operation %q is not supported", challenge.ID, challenge.Operation)
	}
	return nil
}

func (store ClaimStore) firstSupersededVerificationDigest(workspace string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", nil
	}
	claim, err := store.Read(workspace, ids[0])
	if err != nil {
		return "", err
	}
	if claim.Status != ClaimStatusApproved {
		return "", fmt.Errorf("claim %s is %s; only approved claims can be superseded", claim.ID, claim.Status)
	}
	if err := VerifyClaimDigest(claim); err != nil {
		return "", fmt.Errorf("verify superseded claim %s: %w", claim.ID, err)
	}
	return claim.VerifiedDigest, nil
}

func sameStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
