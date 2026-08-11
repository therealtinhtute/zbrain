package runtime

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIndexFTS5Capability(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if err := idx.AssertFTS5(); err != nil {
		t.Fatalf("AssertFTS5() error = %v", err)
	}
}

func TestReindexPublishesRejectedStateForLegacyAndEvidence(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	approved := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Approved Local Memory", ClaimBasisOwner)
	approved.Description = "description recall token"
	if _, err := store.WriteDraft("research", approved); err != nil {
		t.Fatalf("WriteDraft(approved) error = %v", err)
	}
	if _, err := store.Approve("research", approved.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	draft := indexClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "Draft Candidate", ClaimBasisOwner)
	draft.Body = "draft-only recall token\n"
	if _, err := store.WriteDraft("research", draft); err != nil {
		t.Fatalf("WriteDraft(draft) error = %v", err)
	}
	legacyPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "legacy.md")
	if err := os.WriteFile(legacyPath, []byte("legacy local memory should not index"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}
	nonZbrainOKF := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "okf-note.md")
	if err := os.WriteFile(nonZbrainOKF, []byte("---\ntype: note\ntitle: Poison Note\n---\n\npoison local memory\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(non-zbrain OKF) error = %v", err)
	}
	evidencePath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", "raw.md")
	if err := os.WriteFile(evidencePath, []byte("poison local memory evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(evidence) error = %v", err)
	}

	idx := IndexStore{Paths: paths}
	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.Approved != 1 || summary.Draft != 1 || summary.Legacy != 2 || summary.Invalid != 0 || summary.InvalidCount != 2 || summary.RebuildState != RebuildStatusRejected {
		t.Fatalf("summary = %#v", summary)
	}
	manifest, state := readPublishedIndexState(t, idx, "research")
	if state.Status != RebuildStatusRejected || state.InvalidCount != 2 || state.ManifestDigest != manifest.Digest {
		t.Fatalf("published state = %#v, manifest = %#v", state, manifest)
	}
}

func TestRebuildRejectsDuplicateCanonicalClaimIDs(t *testing.T) {
	paths := indexTestPaths(t)
	id := "clm_77777777777777777777777777777777"
	flat := finalizeApprovedStoreClaim(t, indexClaim(id, "Flat duplicate index marker", ClaimBasisOwner))
	writeCanonicalStoreClaim(t, paths, flat)

	nestedPath := "projects/topics/security/" + id + ".md"
	nested := indexClaim(id, "Nested duplicate index marker", ClaimBasisOwner)
	nested.Body = "nested duplicate index marker\n"
	nested.Path = nestedPath
	nested = finalizeApprovedStoreClaim(t, nested)
	nestedAbsolutePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(nestedPath))
	if err := writeClaimAtomic(nestedAbsolutePath, nested); err != nil {
		t.Fatalf("writeClaimAtomic(nested duplicate) error = %v", err)
	}

	flatPath := "projects/" + id + ".md"
	flatAbsolutePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(flatPath))
	beforeFlat := sha256Hex(t, flatAbsolutePath)
	beforeNested := sha256Hex(t, nestedAbsolutePath)
	idx := IndexStore{Paths: paths}
	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.Approved != 0 || summary.Invalid != 2 || summary.InvalidCount != 2 || summary.RebuildState != RebuildStatusRejected {
		t.Fatalf("summary = %#v, want rejected duplicate state", summary)
	}
	if len(summary.InvalidClaims) != 2 || summary.InvalidClaims[0].Path != flatPath || summary.InvalidClaims[1].Path != nestedPath {
		t.Fatalf("InvalidClaims = %#v, want deterministic duplicate paths", summary.InvalidClaims)
	}
	for _, invalid := range summary.InvalidClaims {
		if !strings.Contains(invalid.Error, id) || !strings.Contains(invalid.Error, flatPath) || !strings.Contains(invalid.Error, nestedPath) || !strings.Contains(invalid.Error, "duplicate canonical claim ID") {
			t.Fatalf("invalid claim = %#v, want ID/path repair reason", invalid)
		}
	}
	manifest, state := readPublishedIndexState(t, idx, "research")
	if state.Status != RebuildStatusRejected || state.InvalidCount != 2 || state.ManifestDigest != manifest.Digest {
		t.Fatalf("published state = %#v, manifest = %#v", state, manifest)
	}
	if statuses := indexedClaimStatuses(t, paths, "research"); len(statuses) != 0 {
		t.Fatalf("indexed duplicate claims = %#v, want none", statuses)
	}
	if _, err := os.Stat(indexDirtyPath(t, idx, "research")); !os.IsNotExist(err) {
		t.Fatalf("dirty marker stat error = %v, want missing after rejected publication", err)
	}
	if after := sha256Hex(t, flatAbsolutePath); after != beforeFlat {
		t.Fatalf("flat duplicate changed: before %s after %s", beforeFlat, after)
	}
	if after := sha256Hex(t, nestedAbsolutePath); after != beforeNested {
		t.Fatalf("nested duplicate changed: before %s after %s", beforeNested, after)
	}
}

func TestRebuildRecoversPendingTransition(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	draft := indexClaim("clm_99999999999999999999999999999999", "Recovered Claim", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", draft); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", draft.ID+".md")
	preimage, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(preimage) error = %v", err)
	}
	approved := draft
	approved.Status = ClaimStatusApproved
	approved.VerifiedAt = fixedIndexNow().Format(time.RFC3339)
	approved.VerifiedBy = "owner"
	approved.Transitions = []ClaimTransition{{Kind: ClaimTransitionApprove, At: approved.VerifiedAt, By: approved.VerifiedBy}}
	approved.VerifiedDigest, err = ClaimVerificationDigest(approved)
	if err != nil {
		t.Fatalf("ClaimVerificationDigest() error = %v", err)
	}
	target, err := RenderClaimMarkdown(approved)
	if err != nil {
		t.Fatalf("RenderClaimMarkdown() error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_rebuild",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets: []PendingTransitionTarget{{
			Path:           "wiki/projects/" + draft.ID + ".md",
			PreimageSHA256: transitionSHA256(preimage),
			TargetSHA256:   transitionSHA256(target),
			TargetBytes:    target,
		}},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}

	idx := IndexStore{Paths: paths}
	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.Approved != 1 || summary.RebuildState != RebuildStatusClean {
		t.Fatalf("summary = %#v", summary)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() error = %v", err)
	}
	if _, err := ReadPendingTransition(paths, "research"); !os.IsNotExist(err) {
		t.Fatalf("ReadPendingTransition() error = %v, want missing journal", err)
	}
}

func TestRecoveryLeavesDirty(t *testing.T) {
	paths := indexTestPaths(t)
	path := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "recovery.md")
	before := []byte("before\n")
	target := []byte("after\n")
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_dirty",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/recovery.md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	if err := RecoverPendingTransitionForMutation(paths, "research"); err != nil {
		t.Fatalf("RecoverPendingTransitionForMutation() error = %v", err)
	}
	assertFileBytes(t, path, target)
	if _, err := os.Stat(indexDirtyPath(t, IndexStore{Paths: paths}, "research")); err != nil {
		t.Fatalf("dirty marker error = %v", err)
	}
	assertNoPendingTransition(t, paths, "research")
}

func TestReindexExcludesTamperedApprovedClaim(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Tampered Search", ClaimBasisOwner)
	claim.Body = "original indexed body\n"
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "original indexed body", "tampered indexed body", 1))
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(tampered claim) error = %v", err)
	}

	idx := IndexStore{Paths: paths}
	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.Approved != 0 || summary.Invalid != 1 || summary.InvalidCount != 1 || summary.RebuildState != RebuildStatusRejected {
		t.Fatalf("summary = %#v, want approved=0 invalid=1 rejected", summary)
	}
	manifest, state := readPublishedIndexState(t, idx, "research")
	if state.Status != RebuildStatusRejected || state.InvalidCount != 1 || state.ManifestDigest != manifest.Digest {
		t.Fatalf("published state = %#v, manifest = %#v", state, manifest)
	}
}

func TestRebuildDependencyCanonicalUnchanged(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	support := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Revoked Support", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", support); err != nil {
		t.Fatalf("WriteDraft(support) error = %v", err)
	}
	if _, err := store.Approve("research", support.ID); err != nil {
		t.Fatalf("Approve(support) error = %v", err)
	}
	dependent := indexClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "Dependent Claim", ClaimBasisDerived)
	dependent.SupportingClaimIDs = []string{support.ID}
	if _, err := store.WriteDraft("research", dependent); err != nil {
		t.Fatalf("WriteDraft(dependent) error = %v", err)
	}
	if _, err := store.Approve("research", dependent.ID); err != nil {
		t.Fatalf("Approve(dependent) error = %v", err)
	}
	unrelated := indexClaim("clm_cccccccccccccccccccccccccccccccc", "Unrelated Claim", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", unrelated); err != nil {
		t.Fatalf("WriteDraft(unrelated) error = %v", err)
	}
	if _, err := store.Approve("research", unrelated.ID); err != nil {
		t.Fatalf("Approve(unrelated) error = %v", err)
	}
	if _, err := store.Revoke("research", support.ID, "no longer trusted"); err != nil {
		t.Fatalf("Revoke(support) error = %v", err)
	}

	dependentPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", dependent.ID+".md")
	unrelatedPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", unrelated.ID+".md")
	dependentBefore := sha256Hex(t, dependentPath)
	unrelatedBefore := sha256Hex(t, unrelatedPath)
	summary, err := (IndexStore{Paths: paths}).Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 1 || summary.Invalid != 1 || summary.InvalidCount != 1 {
		t.Fatalf("summary = %#v, want one rejected dependent and one approved unrelated claim", summary)
	}
	if len(summary.InvalidClaims) != 1 || summary.InvalidClaims[0].Path != "projects/"+dependent.ID+".md" || !strings.Contains(summary.InvalidClaims[0].Error, dependent.ID) || !strings.Contains(summary.InvalidClaims[0].Error, support.ID) || !strings.Contains(summary.InvalidClaims[0].Error, "revoked") {
		t.Fatalf("InvalidClaims = %#v, want deterministic dependency path", summary.InvalidClaims)
	}
	indexed := indexedClaimStatuses(t, paths, "research")
	if _, ok := indexed[dependent.ID]; ok {
		t.Fatalf("invalid dependent was indexed: %#v", indexed)
	}
	if indexed[unrelated.ID] != ClaimStatusApproved {
		t.Fatalf("unrelated indexed status = %q, want approved", indexed[unrelated.ID])
	}
	if after := sha256Hex(t, dependentPath); after != dependentBefore {
		t.Fatalf("dependent canonical bytes changed: before %s after %s", dependentBefore, after)
	}
	if after := sha256Hex(t, unrelatedPath); after != unrelatedBefore {
		t.Fatalf("unrelated canonical bytes changed: before %s after %s", unrelatedBefore, after)
	}
}

func TestEvidenceClaimDigestBinding(t *testing.T) {
	paths := indexTestPaths(t)
	evidence := addStoreEvidence(t, paths, "original evidence")
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	support := indexClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "Evidence Support", ClaimBasisEvidence)
	support.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", support); err != nil {
		t.Fatalf("WriteDraft(support) error = %v", err)
	}
	if _, err := store.Approve("research", support.ID); err != nil {
		t.Fatalf("Approve(support) error = %v", err)
	}
	dependent := indexClaim("clm_ffffffffffffffffffffffffffffffff", "Evidence Dependent", ClaimBasisDerived)
	dependent.SupportingClaimIDs = []string{support.ID}
	if _, err := store.WriteDraft("research", dependent); err != nil {
		t.Fatalf("WriteDraft(dependent) error = %v", err)
	}
	if _, err := store.Approve("research", dependent.ID); err != nil {
		t.Fatalf("Approve(dependent) error = %v", err)
	}

	idx := IndexStore{Paths: paths}
	initial, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("initial Rebuild() error = %v", err)
	}
	if initial.RebuildState != RebuildStatusClean || initial.Approved != 2 {
		t.Fatalf("initial summary = %#v, want clean two-claim index", initial)
	}

	replacement := []byte("tampered evidence")
	rawPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(rawPath, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(rawPath, replacement, 0o644); err != nil {
		t.Fatalf("WriteFile(replacement) error = %v", err)
	}
	updatedEvidence := evidence
	updatedEvidence.ByteLength = int64(len(replacement))
	updatedEvidence.SHA256 = sha256String(string(replacement))
	rewriteEvidenceMetadata(t, EvidenceStore{Paths: paths}, "research", evidence.ID, updatedEvidence)

	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("invalidating Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 0 || summary.Invalid != 2 || summary.InvalidCount != 2 {
		t.Fatalf("invalidating summary = %#v, want two rejected claims", summary)
	}
	for _, invalid := range summary.InvalidClaims {
		if !strings.Contains(invalid.Error, evidence.ID) || !strings.Contains(invalid.Error, "digest mismatch") {
			t.Fatalf("invalid claim = %#v, want evidence digest mismatch", invalid)
		}
	}
	if indexed := indexedClaimStatuses(t, paths, "research"); len(indexed) != 0 {
		t.Fatalf("digest-invalid evidence closure was indexed: %#v", indexed)
	}
}

func TestEvidenceClaimMetadataDigestBinding(t *testing.T) {
	paths := indexTestPaths(t)
	evidence := addStoreEvidence(t, paths, "original evidence")
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_11111111111111111111111111111111", "Metadata Evidence Claim", ClaimBasisEvidence)
	claim.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if summary, err := idx.Rebuild("research"); err != nil || summary.RebuildState != RebuildStatusClean {
		t.Fatalf("initial Rebuild() summary=%#v error=%v, want clean", summary, err)
	}
	updatedEvidence := evidence
	updatedEvidence.MediaType = "application/json"
	rewriteEvidenceMetadata(t, EvidenceStore{Paths: paths}, "research", evidence.ID, updatedEvidence)

	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("metadata replacement Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 0 || summary.Invalid != 1 || summary.InvalidCount != 1 {
		t.Fatalf("metadata replacement summary = %#v, want one rejected claim", summary)
	}
	if len(summary.InvalidClaims) != 1 || !strings.Contains(summary.InvalidClaims[0].Error, "digest mismatch") {
		t.Fatalf("InvalidClaims = %#v, want metadata digest mismatch", summary.InvalidClaims)
	}
	if indexed := indexedClaimStatuses(t, paths, "research"); len(indexed) != 0 {
		t.Fatalf("metadata-invalid claim was indexed: %#v", indexed)
	}
}

func TestRebuildRejectsLegacyEvidenceDigestWithRecoveryGuidance(t *testing.T) {
	paths := indexTestPaths(t)
	evidence := addStoreEvidence(t, paths, "legacy digest evidence")
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_33333333333333333333333333333333", "Legacy Evidence Claim", ClaimBasisEvidence)
	claim.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	approved, err := store.Approve("research", claim.ID)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	approved.Sources[0].Digest = "sha256:" + evidence.SHA256
	approved.VerifiedDigest, err = ClaimVerificationDigest(approved)
	if err != nil {
		t.Fatalf("ClaimVerificationDigest() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	if err := writeClaimAtomic(claimPath, approved); err != nil {
		t.Fatalf("writeClaimAtomic() error = %v", err)
	}

	summary, err := (IndexStore{Paths: paths}).Rebuild("research")
	if err != nil {
		t.Fatalf("legacy digest Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 0 || summary.Invalid != 1 || summary.InvalidCount != 1 {
		t.Fatalf("legacy digest summary = %#v, want one rejected claim", summary)
	}
	if len(summary.InvalidClaims) != 1 || !strings.Contains(summary.InvalidClaims[0].Error, "legacy raw digest") || !strings.Contains(summary.InvalidClaims[0].Error, "supersede and reapprove") {
		t.Fatalf("InvalidClaims = %#v, want explicit legacy digest recovery guidance", summary.InvalidClaims)
	}
}

func TestRebuildRejectsEvidenceSourceClosureMismatch(t *testing.T) {
	paths := indexTestPaths(t)
	evidence := addStoreEvidence(t, paths, "closure evidence")
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_22222222222222222222222222222222", "Closure Evidence Claim", ClaimBasisEvidence)
	claim.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	approved, err := store.Approve("research", claim.ID)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	approved.EvidenceIDs = []string{evidence.ID, evidence.ID}
	approved.VerifiedDigest, err = ClaimVerificationDigest(approved)
	if err != nil {
		t.Fatalf("ClaimVerificationDigest() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	if err := writeClaimAtomic(claimPath, approved); err != nil {
		t.Fatalf("writeClaimAtomic() error = %v", err)
	}

	summary, err := (IndexStore{Paths: paths}).Rebuild("research")
	if err != nil {
		t.Fatalf("closure mismatch Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 0 || summary.Invalid != 1 || summary.InvalidCount != 1 {
		t.Fatalf("closure mismatch summary = %#v, want one rejected claim", summary)
	}
	if len(summary.InvalidClaims) != 1 || !strings.Contains(summary.InvalidClaims[0].Error, "duplicate evidence id") {
		t.Fatalf("InvalidClaims = %#v, want duplicate evidence id", summary.InvalidClaims)
	}
}

func TestRebuildEvidenceRejectedState(t *testing.T) {
	paths := indexTestPaths(t)
	evidence := addStoreEvidence(t, paths, "evidence bytes")
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_dddddddddddddddddddddddddddddddd", "Tampered Evidence Claim", ClaimBasisEvidence)
	claim.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	claimBefore := sha256Hex(t, claimPath)
	rawPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(rawPath, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(rawPath, []byte(strings.Repeat("x", int(evidence.ByteLength))), 0o644); err != nil {
		t.Fatalf("WriteFile(raw) error = %v", err)
	}

	summary, err := (IndexStore{Paths: paths}).Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 0 || summary.Invalid != 1 || summary.InvalidCount != 1 {
		t.Fatalf("summary = %#v, want rejected evidence claim", summary)
	}
	if len(summary.InvalidClaims) != 1 || !strings.Contains(summary.InvalidClaims[0].Error, evidence.ID) || !strings.Contains(summary.InvalidClaims[0].Error, "raw") {
		t.Fatalf("InvalidClaims = %#v, want evidence path/reason", summary.InvalidClaims)
	}
	if after := sha256Hex(t, claimPath); after != claimBefore {
		t.Fatalf("claim canonical bytes changed: before %s after %s", claimBefore, after)
	}
	if indexed := indexedClaimStatuses(t, paths, "research"); len(indexed) != 0 {
		t.Fatalf("invalid evidence claim was indexed: %#v", indexed)
	}
}

func TestRebuildRejectsDependentWhenSupportingEvidenceInvalid(t *testing.T) {
	paths := indexTestPaths(t)
	evidence := addStoreEvidence(t, paths, "support evidence bytes")
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	support := indexClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "Evidence Support", ClaimBasisEvidence)
	support.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", support); err != nil {
		t.Fatalf("WriteDraft(support) error = %v", err)
	}
	if _, err := store.Approve("research", support.ID); err != nil {
		t.Fatalf("Approve(support) error = %v", err)
	}
	dependent := indexClaim("clm_ffffffffffffffffffffffffffffffff", "Evidence Dependent", ClaimBasisDerived)
	dependent.SupportingClaimIDs = []string{support.ID}
	if _, err := store.WriteDraft("research", dependent); err != nil {
		t.Fatalf("WriteDraft(dependent) error = %v", err)
	}
	if _, err := store.Approve("research", dependent.ID); err != nil {
		t.Fatalf("Approve(dependent) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if summary, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("initial Rebuild() error = %v", err)
	} else if summary.RebuildState != RebuildStatusClean {
		t.Fatalf("initial rebuild summary = %#v, want clean", summary)
	}

	supportPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", support.ID+".md")
	dependentPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", dependent.ID+".md")
	supportBefore := sha256Hex(t, supportPath)
	dependentBefore := sha256Hex(t, dependentPath)
	rawPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(rawPath, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(rawPath, []byte(strings.Repeat("x", int(evidence.ByteLength))), 0o644); err != nil {
		t.Fatalf("WriteFile(raw) error = %v", err)
	}

	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("invalidating Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 0 || summary.Invalid != 2 || summary.InvalidCount != 2 {
		t.Fatalf("summary = %#v, want support and dependent rejected", summary)
	}
	if len(summary.InvalidClaims) != 2 || summary.InvalidClaims[0].Path != "projects/"+support.ID+".md" || summary.InvalidClaims[1].Path != "projects/"+dependent.ID+".md" {
		t.Fatalf("InvalidClaims = %#v, want deterministic support/dependent paths", summary.InvalidClaims)
	}
	for _, invalid := range summary.InvalidClaims {
		if !strings.Contains(invalid.Error, evidence.ID) || !strings.Contains(invalid.Error, "raw") {
			t.Fatalf("invalid claim = %#v, want supporting evidence path/reason", invalid)
		}
	}
	if indexed := indexedClaimStatuses(t, paths, "research"); len(indexed) != 0 {
		t.Fatalf("invalid support closure was indexed: %#v", indexed)
	}
	if after := sha256Hex(t, supportPath); after != supportBefore {
		t.Fatalf("support canonical bytes changed: before %s after %s", supportBefore, after)
	}
	if after := sha256Hex(t, dependentPath); after != dependentBefore {
		t.Fatalf("dependent canonical bytes changed: before %s after %s", dependentBefore, after)
	}
}

func TestRebuildCycleRejectedState(t *testing.T) {
	paths := indexTestPaths(t)
	first := indexClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "Cycle First", ClaimBasisDerived)
	second := indexClaim("clm_ffffffffffffffffffffffffffffffff", "Cycle Second", ClaimBasisDerived)
	first.SupportingClaimIDs = []string{second.ID}
	second.SupportingClaimIDs = []string{first.ID}
	first = finalizeApprovedStoreClaim(t, first)
	second = finalizeApprovedStoreClaim(t, second)
	writeCanonicalStoreClaim(t, paths, first)
	writeCanonicalStoreClaim(t, paths, second)
	firstPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", first.ID+".md")
	secondPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", second.ID+".md")
	firstBefore := sha256Hex(t, firstPath)
	secondBefore := sha256Hex(t, secondPath)

	summary, err := (IndexStore{Paths: paths}).Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if summary.RebuildState != RebuildStatusRejected || summary.Approved != 0 || summary.Invalid != 2 || summary.InvalidCount != 2 {
		t.Fatalf("summary = %#v, want two cyclic roots rejected", summary)
	}
	if len(summary.InvalidClaims) != 2 || !strings.Contains(summary.InvalidClaims[0].Error, "dependency cycle detected") || !strings.Contains(summary.InvalidClaims[1].Error, "dependency cycle detected") {
		t.Fatalf("InvalidClaims = %#v, want cycle reasons", summary.InvalidClaims)
	}
	if indexed := indexedClaimStatuses(t, paths, "research"); len(indexed) != 0 {
		t.Fatalf("cyclic claims were indexed: %#v", indexed)
	}
	if after := sha256Hex(t, firstPath); after != firstBefore {
		t.Fatalf("first cycle claim changed: before %s after %s", firstBefore, after)
	}
	if after := sha256Hex(t, secondPath); after != secondBefore {
		t.Fatalf("second cycle claim changed: before %s after %s", secondBefore, after)
	}
}

func TestRebuildManifest(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_cccccccccccccccccccccccccccccccc", "Manifest Claim", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	idx := IndexStore{Paths: paths}
	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	manifest, state := readPublishedIndexState(t, idx, "research")
	expected, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest() error = %v", err)
	}
	if !sameTrustInputManifest(manifest, expected) {
		t.Fatalf("published manifest = %#v, want %#v", manifest, expected)
	}
	if state.Status != RebuildStatusClean || summary.RebuildState != RebuildStatusClean || summary.ManifestDigest != manifest.Digest {
		t.Fatalf("summary/state = %#v/%#v, manifest = %#v", summary, state, manifest)
	}
}

func TestRebuildCleanState(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	_, state := readPublishedIndexState(t, idx, "research")
	if summary.RebuildState != RebuildStatusClean || summary.InvalidCount != 0 || state.Status != RebuildStatusClean || state.InvalidCount != 0 {
		t.Fatalf("summary/state = %#v/%#v", summary, state)
	}
	if _, err := os.Stat(indexDirtyPath(t, idx, "research")); !os.IsNotExist(err) {
		t.Fatalf("dirty marker stat error = %v, want missing", err)
	}
}

func TestCheckFreshAcceptsCleanIndex(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() error = %v", err)
	}
}

func TestCheckFreshReportsStaleAfterCoveredInputEdit(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Covered Input", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "Covered Input", "Changed Covered Input", 1))
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(changed claim) error = %v", err)
	}
	freshErr := idx.CheckFresh("research")
	if freshErr == nil || !strings.Contains(freshErr.Error(), "stale") || !strings.Contains(freshErr.Error(), claimPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
		t.Fatalf("CheckFresh() error = %v, want stale error naming %q and reindex recovery", freshErr, claimPath)
	}
}

func TestCheckFreshReportsStaleAfterNewWikiClaimAndReindex(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("initial Rebuild() error = %v", err)
	}
	databasePath := indexDatabasePath(t, idx, "research")
	databaseInfo, err := os.Stat(databasePath)
	if err != nil {
		t.Fatalf("Stat(database) error = %v", err)
	}
	claim := indexClaim("clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "Hand Authored Claim", ClaimBasisOwner)
	contents, err := RenderClaimMarkdown(claim)
	if err != nil {
		t.Fatalf("RenderClaimMarkdown() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(claim) error = %v", err)
	}
	newer := databaseInfo.ModTime().Add(time.Second)
	if err := os.Chtimes(claimPath, newer, newer); err != nil {
		t.Fatalf("Chtimes(claim) error = %v", err)
	}

	freshErr := idx.CheckFresh("research")
	if freshErr == nil || !strings.Contains(freshErr.Error(), claimPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
		t.Fatalf("CheckFresh() error = %v, want stale error naming %q and reindex recovery", freshErr, claimPath)
	}

	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("recovery Rebuild() error = %v", err)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() error = %v after reindex", err)
	}
}

func TestCheckFreshReportsStaleAfterTrustInputDeletion(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_ffffffffffffffffffffffffffffffff", "Deleted Claim", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	if err := os.Remove(claimPath); err != nil {
		t.Fatalf("Remove(claim) error = %v", err)
	}
	freshErr := idx.CheckFresh("research")
	if freshErr == nil || !strings.Contains(freshErr.Error(), claimPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
		t.Fatalf("CheckFresh() error = %v, want stale deletion error naming %q", freshErr, claimPath)
	}
}

type tokenTestFileInfo struct {
	sys any
}

func (info tokenTestFileInfo) Name() string       { return "token-test" }
func (info tokenTestFileInfo) Size() int64        { return 0 }
func (info tokenTestFileInfo) Mode() os.FileMode  { return 0 }
func (info tokenTestFileInfo) ModTime() time.Time { return time.Time{} }
func (info tokenTestFileInfo) IsDir() bool        { return false }
func (info tokenTestFileInfo) Sys() any           { return info.sys }

type tokenTestTimespec struct {
	Sec  int64
	Nsec int64
}

type tokenTestCtim struct {
	Ctim tokenTestTimespec
}

type tokenTestCtimespec struct {
	Ctimespec tokenTestTimespec
}

type tokenTestScalar struct {
	Ctime     int64
	CtimeNsec int64
}

type tokenTestMalformed struct {
	ChangeTime string
}

func TestFileChangeToken(t *testing.T) {
	want := int64(42)*int64(time.Second) + 123
	cases := []struct {
		name string
		sys  any
		want int64
	}{
		{name: "linux ctime shape", sys: tokenTestCtim{Ctim: tokenTestTimespec{Sec: 42, Nsec: 123}}, want: want},
		{name: "darwin ctime shape", sys: tokenTestCtimespec{Ctimespec: tokenTestTimespec{Sec: 42, Nsec: 123}}, want: want},
		{name: "scalar ctime shape", sys: tokenTestScalar{Ctime: 42, CtimeNsec: 123}, want: want},
		{name: "pointer metadata", sys: &tokenTestCtim{Ctim: tokenTestTimespec{Sec: 42, Nsec: 123}}, want: want},
		{name: "unsupported metadata", sys: tokenTestMalformed{}, want: unavailableFileChangeToken},
		{name: "nil metadata", sys: nil, want: unavailableFileChangeToken},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := fileChangeToken(tokenTestFileInfo{sys: test.sys}); got != test.want {
				t.Fatalf("fileChangeToken() = %d, want %d", got, test.want)
			}
		})
	}
	if got := fileChangeToken(nil); got != unavailableFileChangeToken {
		t.Fatalf("fileChangeToken(nil) = %d, want %d", got, unavailableFileChangeToken)
	}
}

func TestCombineChangeStampRejectsInvalidRange(t *testing.T) {
	cases := []struct {
		name        string
		seconds     int64
		nanoseconds int64
	}{
		{name: "negative nanoseconds", seconds: 0, nanoseconds: -1},
		{name: "nanoseconds too large", seconds: 0, nanoseconds: int64(time.Second)},
		{name: "positive overflow", seconds: int64(1<<63 - 1), nanoseconds: 0},
		{name: "negative overflow", seconds: -1 << 63, nanoseconds: 0},
		{name: "unavailable collision", seconds: -1, nanoseconds: int64(time.Second) - 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := combineChangeStamp(test.seconds, test.nanoseconds); got != unavailableFileChangeToken {
				t.Fatalf("combineChangeStamp() = %d, want %d", got, unavailableFileChangeToken)
			}
		})
	}
}

func TestContentDigestFreshness(t *testing.T) {
	t.Run("claim edit with restored mtime", func(t *testing.T) {
		paths := indexTestPaths(t)
		store := ClaimStore{Paths: paths, Now: fixedIndexNow}
		claim := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Content Digest Claim", ClaimBasisOwner)
		if _, err := store.WriteDraft("research", claim); err != nil {
			t.Fatalf("WriteDraft() error = %v", err)
		}
		if _, err := store.Approve("research", claim.ID); err != nil {
			t.Fatalf("Approve() error = %v", err)
		}
		idx := IndexStore{Paths: paths}
		if _, err := idx.Rebuild("research"); err != nil {
			t.Fatalf("Rebuild() error = %v", err)
		}
		claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
		info, err := os.Stat(claimPath)
		if err != nil {
			t.Fatalf("Stat(claim) error = %v", err)
		}
		contents, err := os.ReadFile(claimPath)
		if err != nil {
			t.Fatalf("ReadFile(claim) error = %v", err)
		}
		contents = []byte(strings.Replace(string(contents), "Content Digest Claim", "Content Digest Delta", 1))
		if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
			t.Fatalf("WriteFile(claim) error = %v", err)
		}
		if err := os.Chtimes(claimPath, info.ModTime(), info.ModTime()); err != nil {
			t.Fatalf("Chtimes(claim) error = %v", err)
		}
		freshErr := idx.CheckFresh("research")
		if freshErr == nil || !strings.Contains(freshErr.Error(), claimPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
			t.Fatalf("CheckFresh() error = %v, want content-digest stale error naming %q", freshErr, claimPath)
		}
	})

	t.Run("evidence edit with restored mtime", func(t *testing.T) {
		paths := indexTestPaths(t)
		evidence := addStoreEvidence(t, paths, "original evidence")
		idx := IndexStore{Paths: paths}
		if _, err := idx.Rebuild("research"); err != nil {
			t.Fatalf("Rebuild() error = %v", err)
		}
		rawPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
		info, err := os.Stat(rawPath)
		if err != nil {
			t.Fatalf("Stat(raw) error = %v", err)
		}
		if err := os.Chmod(rawPath, 0o644); err != nil {
			t.Fatalf("Chmod(raw) error = %v", err)
		}
		if err := os.WriteFile(rawPath, []byte("tampered evidence"), 0o644); err != nil {
			t.Fatalf("WriteFile(raw) error = %v", err)
		}
		if err := os.Chtimes(rawPath, info.ModTime(), info.ModTime()); err != nil {
			t.Fatalf("Chtimes(raw) error = %v", err)
		}
		freshErr := idx.CheckFresh("research")
		if freshErr == nil || !strings.Contains(freshErr.Error(), rawPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
			t.Fatalf("CheckFresh() error = %v, want content-digest stale error naming %q", freshErr, rawPath)
		}
	})
}

func TestContentDigestFreshnessFallbackWithoutChangeTokens(t *testing.T) {
	previous := trustFileChangeToken
	trustFileChangeToken = func(os.FileInfo) int64 { return -1 }
	t.Cleanup(func() { trustFileChangeToken = previous })

	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_12121212121212121212121212121212", "Fallback Digest Claim", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	info, err := os.Stat(claimPath)
	if err != nil {
		t.Fatalf("Stat(claim) error = %v", err)
	}
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "Fallback Digest Claim", "Fallback Digest Delta", 1))
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(claim) error = %v", err)
	}
	if err := os.Chtimes(claimPath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("Chtimes(claim) error = %v", err)
	}
	freshErr := idx.CheckFresh("research")
	if freshErr == nil || !strings.Contains(freshErr.Error(), claimPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
		t.Fatalf("CheckFresh() error = %v, want manifest-fallback stale error naming %q", freshErr, claimPath)
	}
}

func TestContentDigestFreshnessNewInputWithRestoredDirectoryMtime(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	projectsDir := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects")
	info, err := os.Stat(projectsDir)
	if err != nil {
		t.Fatalf("Stat(projects directory) error = %v", err)
	}
	addedPath := filepath.Join(projectsDir, "added.md")
	if err := os.WriteFile(addedPath, []byte("added trust input\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(added) error = %v", err)
	}
	if err := os.Chtimes(projectsDir, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("Chtimes(projects directory) error = %v", err)
	}
	freshErr := idx.CheckFresh("research")
	if freshErr == nil || !strings.Contains(freshErr.Error(), addedPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
		t.Fatalf("CheckFresh() error = %v, want added-input stale error naming %q", freshErr, addedPath)
	}
}

func TestCheckFreshReportsStaleAfterEvidenceEdit(t *testing.T) {
	paths := indexTestPaths(t)
	sourceRoot := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", "evd_test")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(source) error = %v", err)
	}
	metadataPath := filepath.Join(sourceRoot, "source.yaml")
	rawPath := filepath.Join(sourceRoot, "raw")
	if err := os.WriteFile(metadataPath, []byte("id: evd_test\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(metadata) error = %v", err)
	}
	if err := os.WriteFile(rawPath, []byte("original evidence\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(raw) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if err := os.WriteFile(rawPath, []byte("changed evidence\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(changed raw) error = %v", err)
	}
	freshErr := idx.CheckFresh("research")
	if freshErr == nil || !strings.Contains(freshErr.Error(), rawPath) || !strings.Contains(freshErr.Error(), "run zbrain reindex") {
		t.Fatalf("CheckFresh() error = %v, want stale evidence error naming %q", freshErr, rawPath)
	}
}

func TestCheckFreshRejectsNewWikiSymlink(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	outsidePath := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	linkPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "linked.md")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Fatalf("Symlink(linked claim) error = %v", err)
	}
	freshErr := idx.CheckFresh("research")
	if freshErr == nil || !strings.Contains(freshErr.Error(), "symlink") || !strings.Contains(freshErr.Error(), linkPath) {
		t.Fatalf("CheckFresh() error = %v, want symlink error naming %q", freshErr, linkPath)
	}
}

func TestOutsideEditDoesNotStaleIndex(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	outsideInput := filepath.Join(paths.WorkspacesDir, "research", "workspace.md")
	if err := os.WriteFile(outsideInput, []byte("# edited workspace metadata\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside input) error = %v", err)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() error = %v after outside edit", err)
	}
}

func TestCheckFreshRejectsDirtyMissingRejectedAndMalformedState(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		paths := indexTestPaths(t)
		idx := IndexStore{Paths: paths}
		err := idx.CheckFresh("research")
		if err == nil || !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("CheckFresh() error = %v, want missing error", err)
		}
	})

	t.Run("dirty", func(t *testing.T) {
		paths := indexTestPaths(t)
		idx := IndexStore{Paths: paths}
		if _, err := idx.Rebuild("research"); err != nil {
			t.Fatalf("Rebuild() error = %v", err)
		}
		if err := idx.MarkDirty("research"); err != nil {
			t.Fatalf("MarkDirty() error = %v", err)
		}
		err := idx.CheckFresh("research")
		if err == nil || !strings.Contains(err.Error(), "dirty") {
			t.Fatalf("CheckFresh() error = %v, want dirty error", err)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		paths := indexTestPaths(t)
		legacyPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "legacy.md")
		if err := os.WriteFile(legacyPath, []byte("legacy input\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(legacy) error = %v", err)
		}
		idx := IndexStore{Paths: paths}
		if _, err := idx.Rebuild("research"); err != nil {
			t.Fatalf("Rebuild() error = %v", err)
		}
		err := idx.CheckFresh("research")
		if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("CheckFresh() error = %v, want rejected error", err)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		paths := indexTestPaths(t)
		idx := IndexStore{Paths: paths}
		if _, err := idx.Rebuild("research"); err != nil {
			t.Fatalf("Rebuild() error = %v", err)
		}
		databasePath := indexDatabasePath(t, idx, "research")
		db, err := sql.Open("sqlite", databasePath)
		if err != nil {
			t.Fatalf("sql.Open() error = %v", err)
		}
		if _, err := db.Exec("delete from rebuild_state"); err != nil {
			db.Close()
			t.Fatalf("delete rebuild state error = %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close() error = %v", err)
		}
		err = idx.CheckFresh("research")
		if err == nil || !strings.Contains(err.Error(), "malformed") {
			t.Fatalf("CheckFresh() error = %v, want malformed error", err)
		}
	})
}

func TestRebuildRejectedState(t *testing.T) {
	paths := indexTestPaths(t)
	legacyPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "legacy.md")
	if err := os.WriteFile(legacyPath, []byte("legacy input\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	summary, err := idx.Rebuild("research")
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	_, state := readPublishedIndexState(t, idx, "research")
	if summary.RebuildState != RebuildStatusRejected || summary.InvalidCount != 1 || state.Status != RebuildStatusRejected || state.InvalidCount != 1 {
		t.Fatalf("summary/state = %#v/%#v", summary, state)
	}
	if _, err := os.Stat(indexDirtyPath(t, idx, "research")); !os.IsNotExist(err) {
		t.Fatalf("dirty marker stat error = %v, want missing", err)
	}
}

func TestRebuildFailureLeavesDirty(t *testing.T) {
	paths := indexTestPaths(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	linked := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "linked.md")
	if err := os.Symlink(outside, linked); err != nil {
		t.Fatalf("Symlink(linked) error = %v", err)
	}

	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err == nil {
		t.Fatal("Rebuild() error = nil, want manifest failure")
	}
	if _, err := os.Stat(indexDirtyPath(t, idx, "research")); err != nil {
		t.Fatalf("dirty marker stat error = %v, want present", err)
	}
	if _, err := os.Stat(indexDatabasePath(t, idx, "research")); !os.IsNotExist(err) {
		t.Fatalf("database stat error = %v, want missing", err)
	}
}

func TestRebuildDoesNotMutateCanonical(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_dddddddddddddddddddddddddddddddd", "Canonical Claim", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "Canonical Claim", "Tampered Claim", 1))
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(tampered) error = %v", err)
	}
	before, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(snapshot) error = %v", err)
	}

	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	after, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("canonical claim changed during rebuild: before=%q after=%q", before, after)
	}
}

func TestRebuildRecoversUnsupportedIndexSchema(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("initial Rebuild() error = %v", err)
	}
	databasePath, err := idx.DatabasePath("research")
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec("pragma user_version = 1"); err != nil {
		_ = db.Close()
		t.Fatalf("set old schema version error = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := idx.CheckFresh("research"); err == nil || !strings.Contains(err.Error(), "unsupported index schema version") {
		t.Fatalf("CheckFresh() error = %v, want unsupported schema error", err)
	}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("recovery Rebuild() error = %v", err)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() after recovery error = %v", err)
	}
}

func TestIndexDirtyMarkerBlocksSearch(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if err := idx.MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	if _, err := idx.Search("research", SearchOptions{Query: "anything", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10}); err == nil {
		t.Fatalf("Search() error = nil, want dirty index error")
	}
}

func TestIndexOperationsRejectUnsafeOrMissingWorkspaceBeforePathCreation(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}

	for _, workspace := range []string{"../outside", "missing"} {
		t.Run(workspace, func(t *testing.T) {
			if err := idx.MarkDirty(workspace); err == nil {
				t.Fatalf("MarkDirty(%q) error = nil", workspace)
			}
			if err := idx.CheckFresh(workspace); err == nil {
				t.Fatalf("CheckFresh(%q) error = nil", workspace)
			}
			if _, err := idx.Search(workspace, SearchOptions{
				Query:    "anything",
				Statuses: []ClaimStatus{ClaimStatusApproved},
				Limit:    10,
			}); err == nil {
				t.Fatalf("Search(%q) error = nil", workspace)
			}
			if _, err := idx.Rebuild(workspace); err == nil {
				t.Fatalf("Rebuild(%q) error = nil", workspace)
			}
		})
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(paths.WorkspacesDir, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	for _, operation := range []string{"MarkDirty", "CheckFresh", "Search", "Rebuild"} {
		t.Run("symlink-"+operation, func(t *testing.T) {
			switch operation {
			case "MarkDirty":
				if err := idx.MarkDirty("linked"); err == nil {
					t.Fatalf("MarkDirty(symlink) error = nil")
				}
			case "CheckFresh":
				if err := idx.CheckFresh("linked"); err == nil {
					t.Fatalf("CheckFresh(symlink) error = nil")
				}
			case "Search":
				if _, err := idx.Search("linked", SearchOptions{
					Query:    "anything",
					Statuses: []ClaimStatus{ClaimStatusApproved},
					Limit:    10,
				}); err == nil {
					t.Fatalf("Search(symlink) error = nil")
				}
			case "Rebuild":
				if _, err := idx.Rebuild("linked"); err == nil {
					t.Fatalf("Rebuild(symlink) error = nil")
				}
			}
		})
	}
	if _, err := os.Stat(paths.IndexesDir); !os.IsNotExist(err) {
		t.Fatalf("invalid index operations created indexes directory: stat error = %v", err)
	}
}

func TestIndexOperationsRejectSymlinkedIndexDirectory(t *testing.T) {
	paths := indexTestPaths(t)
	outside := t.TempDir()
	linkedIndexes := filepath.Join(t.TempDir(), "indexes")
	if err := os.Symlink(outside, linkedIndexes); err != nil {
		t.Fatalf("Symlink(indexes) error = %v", err)
	}
	paths.IndexesDir = linkedIndexes
	idx := IndexStore{Paths: paths}

	for _, operation := range []string{"MarkDirty", "CheckFresh", "Search", "Rebuild"} {
		t.Run(operation, func(t *testing.T) {
			switch operation {
			case "MarkDirty":
				if err := idx.MarkDirty("research"); err == nil {
					t.Fatalf("MarkDirty() error = nil")
				}
			case "CheckFresh":
				if err := idx.CheckFresh("research"); err == nil {
					t.Fatalf("CheckFresh() error = nil")
				}
			case "Search":
				if _, err := idx.Search("research", SearchOptions{
					Query:    "anything",
					Statuses: []ClaimStatus{ClaimStatusApproved},
					Limit:    10,
				}); err == nil {
					t.Fatalf("Search() error = nil")
				}
			case "Rebuild":
				if _, err := idx.Rebuild("research"); err == nil {
					t.Fatalf("Rebuild() error = nil")
				}
			}
		})
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("ReadDir(outside) error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("symlinked index directory was mutated: %v", entries)
	}
}

func TestIndexOperationsAllowSymlinkedAncestorPath(t *testing.T) {
	tmp := t.TempDir()
	realRoot := filepath.Join(tmp, "real")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(realRoot) error = %v", err)
	}
	linkedRoot := filepath.Join(tmp, "linked")
	if err := os.Symlink(realRoot, linkedRoot); err != nil {
		t.Fatalf("Symlink(linkedRoot) error = %v", err)
	}
	paths, err := ResolvePaths(Options{
		CWD:        linkedRoot,
		HomeDir:    linkedRoot,
		RuntimeDir: filepath.Join(linkedRoot, ".zbrain"),
	})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", fixedIndexNow()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if err := idx.MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	if _, err := os.Stat(indexDirtyPath(t, idx, "research")); err != nil {
		t.Fatalf("Stat(dirty) error = %v", err)
	}
}

func TestIndexOperationsRejectSymlinkedIndexFiles(t *testing.T) {
	paths := indexTestPaths(t)
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	t.Run("database", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.sqlite")
		before := []byte("outside database bytes")
		if err := os.WriteFile(outside, before, 0o644); err != nil {
			t.Fatalf("WriteFile(outside database) error = %v", err)
		}
		if err := os.Remove(indexDatabasePath(t, idx, "research")); err != nil {
			t.Fatalf("Remove(database) error = %v", err)
		}
		if err := os.Symlink(outside, indexDatabasePath(t, idx, "research")); err != nil {
			t.Fatalf("Symlink(database) error = %v", err)
		}
		for _, operation := range []string{"MarkDirty", "CheckFresh", "Search", "Rebuild"} {
			t.Run(operation, func(t *testing.T) {
				switch operation {
				case "MarkDirty":
					if err := idx.MarkDirty("research"); err == nil {
						t.Fatalf("MarkDirty() error = nil")
					}
				case "CheckFresh":
					if err := idx.CheckFresh("research"); err == nil {
						t.Fatalf("CheckFresh() error = nil")
					}
				case "Search":
					if _, err := idx.Search("research", SearchOptions{Query: "anything", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10}); err == nil {
						t.Fatalf("Search() error = nil")
					}
				case "Rebuild":
					if _, err := idx.Rebuild("research"); err == nil {
						t.Fatalf("Rebuild() error = nil")
					}
				}
			})
		}
		after, err := os.ReadFile(outside)
		if err != nil {
			t.Fatalf("ReadFile(outside database) error = %v", err)
		}
		if string(after) != string(before) {
			t.Fatalf("outside database changed: before=%q after=%q", before, after)
		}
	})

	t.Run("dirty-marker", func(t *testing.T) {
		if err := os.Remove(indexDatabasePath(t, idx, "research")); err != nil {
			t.Fatalf("Remove(database symlink) error = %v", err)
		}
		outside := filepath.Join(t.TempDir(), "outside.dirty")
		before := []byte("outside dirty bytes")
		if err := os.WriteFile(outside, before, 0o644); err != nil {
			t.Fatalf("WriteFile(outside dirty) error = %v", err)
		}
		if err := os.Symlink(outside, indexDirtyPath(t, idx, "research")); err != nil {
			t.Fatalf("Symlink(dirty marker) error = %v", err)
		}
		for _, operation := range []string{"MarkDirty", "CheckFresh", "Search", "Rebuild"} {
			t.Run(operation, func(t *testing.T) {
				switch operation {
				case "MarkDirty":
					if err := idx.MarkDirty("research"); err == nil {
						t.Fatalf("MarkDirty() error = nil")
					}
				case "CheckFresh":
					if err := idx.CheckFresh("research"); err == nil {
						t.Fatalf("CheckFresh() error = nil")
					}
				case "Search":
					if _, err := idx.Search("research", SearchOptions{Query: "anything", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10}); err == nil {
						t.Fatalf("Search() error = nil")
					}
				case "Rebuild":
					if _, err := idx.Rebuild("research"); err == nil {
						t.Fatalf("Rebuild() error = nil")
					}
				}
			})
		}
		after, err := os.ReadFile(outside)
		if err != nil {
			t.Fatalf("ReadFile(outside dirty) error = %v", err)
		}
		if string(after) != string(before) {
			t.Fatalf("outside dirty marker changed: before=%q after=%q", before, after)
		}
	})
}

func TestReindexIsDeterministicAfterDeletingIndex(t *testing.T) {
	paths := indexTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedIndexNow}
	claim := indexClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "Deterministic Search", ClaimBasisOwner)
	claim.Body = "alpha beta deterministic body\n"
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	first, err := idx.Search("research", SearchOptions{Query: "deterministic", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10})
	if err != nil {
		t.Fatalf("Search(first) error = %v", err)
	}
	if err := os.Remove(indexDatabasePath(t, idx, "research")); err != nil {
		t.Fatalf("Remove(index) error = %v", err)
	}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild(second) error = %v", err)
	}
	second, err := idx.Search("research", SearchOptions{Query: "deterministic", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10})
	if err != nil {
		t.Fatalf("Search(second) error = %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID || first[0].Path != second[0].Path {
		t.Fatalf("results changed: first=%#v second=%#v", first, second)
	}
}

func indexTestPaths(t *testing.T) Paths {
	t.Helper()
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", fixedIndexNow()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return paths
}

func indexClaim(id string, title string, basis ClaimBasis) Claim {
	return Claim{
		Type:      OKFClaimType,
		ID:        id,
		Tier:      "projects",
		Status:    ClaimStatusDraft,
		Title:     title,
		Basis:     basis,
		CreatedAt: fixedIndexNow().Format(time.RFC3339),
		CreatedBy: "owner",
		Tags:      []string{"memory"},
		Body:      "Local-first memory retrieval body\n",
	}
}

func fixedIndexNow() time.Time {
	return time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
}

func indexedClaimStatuses(t *testing.T, paths Paths, workspace string) map[string]ClaimStatus {
	t.Helper()
	dbPath, err := (IndexStore{Paths: paths}).DatabasePath(workspace)
	if err != nil {
		t.Fatalf("DatabasePath(%q) error = %v", workspace, err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	rows, err := db.Query("select id, status from claims order by id")
	if err != nil {
		t.Fatalf("Query(claims) error = %v", err)
	}
	defer rows.Close()
	statuses := make(map[string]ClaimStatus)
	for rows.Next() {
		var id string
		var status ClaimStatus
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatalf("Scan(claim) error = %v", err)
		}
		statuses[id] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err() = %v", err)
	}
	return statuses
}

func readPublishedIndexState(t *testing.T, idx IndexStore, workspace string) (TrustInputManifest, RebuildState) {
	t.Helper()
	db, err := sql.Open("sqlite", indexDatabasePath(t, idx, workspace))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	manifest, state, err := ReadIndexState(db)
	if err != nil {
		t.Fatalf("ReadIndexState() error = %v", err)
	}
	return manifest, state
}

func indexDatabasePath(t *testing.T, idx IndexStore, workspace string) string {
	t.Helper()
	path, err := idx.DatabasePath(workspace)
	if err != nil {
		t.Fatalf("DatabasePath(%q) error = %v", workspace, err)
	}
	return path
}

func indexDirtyPath(t *testing.T, idx IndexStore, workspace string) string {
	t.Helper()
	path, err := idx.DirtyPath(workspace)
	if err != nil {
		t.Fatalf("DirtyPath(%q) error = %v", workspace, err)
	}
	return path
}
