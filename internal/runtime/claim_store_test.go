package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClaimStoreDraftApproveSupersedeRevoke(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}

	draft := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	created, err := store.WriteDraft("research", draft)
	if err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if created.Status != ClaimStatusDraft {
		t.Fatalf("draft status = %q", created.Status)
	}
	if created.Path != "projects/clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.md" {
		t.Fatalf("draft path = %q", created.Path)
	}

	approved, err := store.Approve("research", draft.ID)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved.Status != ClaimStatusApproved {
		t.Fatalf("approved status = %q", approved.Status)
	}
	if approved.VerifiedAt == "" || approved.VerifiedBy != "owner" || !strings.HasPrefix(approved.VerifiedDigest, "sha256:") {
		t.Fatalf("approved verification metadata missing: %#v", approved)
	}

	replacement := validStoreClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ClaimBasisOwner)
	replacement.Body = "Replacement body\n"
	superseding, err := store.WriteSupersedingDraft("research", approved.ID, replacement)
	if err != nil {
		t.Fatalf("WriteSupersedingDraft() error = %v", err)
	}
	if strings.Join(superseding.Supersedes, ",") != approved.ID {
		t.Fatalf("Supersedes = %v", superseding.Supersedes)
	}

	approvedReplacement, err := store.Approve("research", replacement.ID)
	if err != nil {
		t.Fatalf("Approve(replacement) error = %v", err)
	}
	old, err := store.Read("research", approved.ID)
	if err != nil {
		t.Fatalf("Read(old) error = %v", err)
	}
	if old.Status != ClaimStatusSuperseded {
		t.Fatalf("old status = %q, want superseded", old.Status)
	}
	if approvedReplacement.Status != ClaimStatusApproved {
		t.Fatalf("replacement status = %q", approvedReplacement.Status)
	}

	priorDigest := approvedReplacement.VerifiedDigest
	revoked, err := store.Revoke("research", approvedReplacement.ID, "wrong scope")
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.Status != ClaimStatusRevoked {
		t.Fatalf("revoked status = %q", revoked.Status)
	}
	if revoked.Body != replacement.Body {
		t.Fatalf("revoked body changed: got %q, want %q", revoked.Body, replacement.Body)
	}
	if revoked.VerifiedDigest != priorDigest || len(revoked.Transitions) != 2 || revoked.Transitions[1].Kind != ClaimTransitionRevoke || revoked.Transitions[1].Reason != "wrong scope" {
		t.Fatalf("revocation history not preserved: %#v", revoked)
	}
}

func TestApproveTransitionGraph(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	approved, err := store.Approve("research", claim.ID)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if len(approved.Transitions) != 1 || approved.Transitions[0].Kind != ClaimTransitionApprove {
		t.Fatalf("approval history = %#v", approved.Transitions)
	}
	if _, err := store.Approve("research", claim.ID); err == nil {
		t.Fatalf("Approve(approved) error = nil")
	}
}

func TestSupersedeTransitionGraph(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	oldClaim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", oldClaim); err != nil {
		t.Fatalf("WriteDraft(old) error = %v", err)
	}
	oldApproved, err := store.Approve("research", oldClaim.ID)
	if err != nil {
		t.Fatalf("Approve(old) error = %v", err)
	}

	replacement := validStoreClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ClaimBasisOwner)
	replacement.Body = "Replacement body\n"
	draft, err := store.WriteSupersedingDraft("research", oldApproved.ID, replacement)
	if err != nil {
		t.Fatalf("WriteSupersedingDraft() error = %v", err)
	}
	approvedReplacement, err := store.Approve("research", draft.ID)
	if err != nil {
		t.Fatalf("Approve(replacement) error = %v", err)
	}
	if len(approvedReplacement.Transitions) != 1 || approvedReplacement.Transitions[0].Kind != ClaimTransitionSupersede || len(approvedReplacement.Transitions[0].RelatedClaimIDs) != 1 || approvedReplacement.Transitions[0].RelatedClaimIDs[0] != oldApproved.ID {
		t.Fatalf("replacement history = %#v", approvedReplacement.Transitions)
	}

	old, err := store.Read("research", oldApproved.ID)
	if err != nil {
		t.Fatalf("Read(old) error = %v", err)
	}
	if old.Status != ClaimStatusSuperseded || old.Body != oldApproved.Body || old.VerifiedAt != oldApproved.VerifiedAt || old.VerifiedBy != oldApproved.VerifiedBy || old.VerifiedDigest != oldApproved.VerifiedDigest {
		t.Fatalf("old approval history was not preserved: %#v", old)
	}
	if len(old.Transitions) != 2 || old.Transitions[1].Kind != ClaimTransitionSupersede || old.Transitions[1].PriorVerificationDigest != oldApproved.VerifiedDigest || old.Transitions[1].RelatedClaimIDs[0] != approvedReplacement.ID {
		t.Fatalf("old supersession history = %#v", old.Transitions)
	}
	if _, err := store.Revoke("research", old.ID, "obsolete"); err == nil {
		t.Fatalf("Revoke(superseded) error = nil")
	}
}

func TestRevokeTransitionGraph(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Revoke("research", claim.ID, "not ready"); err == nil {
		t.Fatalf("Revoke(draft) error = nil")
	}
	approved, err := store.Approve("research", claim.ID)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	revoked, err := store.Revoke("research", approved.ID, "not ready")
	if err != nil {
		t.Fatalf("Revoke(approved) error = %v", err)
	}
	if revoked.Status != ClaimStatusRevoked || revoked.Body != approved.Body || revoked.VerifiedDigest != approved.VerifiedDigest || len(revoked.Transitions) != 2 || revoked.Transitions[1].Kind != ClaimTransitionRevoke || revoked.Transitions[1].Reason != "not ready" {
		t.Fatalf("revocation history = %#v", revoked)
	}
	if _, err := store.Revoke("research", approved.ID, "again"); err == nil {
		t.Fatalf("Revoke(revoked) error = nil")
	}
}

func TestLifecycleHistoryPreserved(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	claim.Body = "Original body\n"
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	approved, err := store.Approve("research", claim.ID)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	beforeBody, beforeAt, beforeBy, beforeDigest := approved.Body, approved.VerifiedAt, approved.VerifiedBy, approved.VerifiedDigest
	if _, err := store.Revoke("research", approved.ID, "owner withdrew claim"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	revoked, err := store.Read("research", approved.ID)
	if err != nil {
		t.Fatalf("Read(revoked) error = %v", err)
	}
	if revoked.Body != beforeBody || revoked.VerifiedAt != beforeAt || revoked.VerifiedBy != beforeBy || revoked.VerifiedDigest != beforeDigest {
		t.Fatalf("original representation changed: %#v", revoked)
	}
	if len(revoked.Transitions) != 2 || revoked.Transitions[1].PriorVerificationDigest != beforeDigest {
		t.Fatalf("prior attestation missing: %#v", revoked.Transitions)
	}
}

func TestClaimStoreApproveEvidenceClaimVerifiesEvidenceAndWritesSources(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	evidence := addStoreEvidence(t, paths, "source bytes")
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisEvidence)
	claim.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	approved, err := store.Approve("research", claim.ID)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	metadataPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "source.yaml")
	metadata, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("ReadFile(metadata) error = %v", err)
	}
	wantDigest := evidenceSnapshotDigest(metadata, evidence)
	if len(approved.Sources) != 1 || approved.Sources[0].ID != evidence.ID || approved.Sources[0].Digest != wantDigest {
		t.Fatalf("Sources = %#v, want digest %q", approved.Sources, wantDigest)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(claim) error = %v", err)
	}
	if !strings.Contains(string(contents), "sources:") || !strings.Contains(string(contents), "verified:") {
		t.Fatalf("approved OKF claim missing sources/verified:\n%s", contents)
	}
}

func TestClaimStoreApproveRejectsTamperedEvidence(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	evidence := addStoreEvidence(t, paths, "trusted evidence")
	raw := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(raw, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(raw, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("WriteFile(raw) error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisEvidence)
	claim.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err == nil {
		t.Fatalf("Approve() error = nil, want tampered evidence error")
	}
}

func TestClaimStoreApproveRejectsDraftSupportingClaim(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	support := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", support); err != nil {
		t.Fatalf("WriteDraft(support) error = %v", err)
	}
	derived := validStoreClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ClaimBasisDerived)
	derived.SupportingClaimIDs = []string{support.ID}
	if _, err := store.WriteDraft("research", derived); err != nil {
		t.Fatalf("WriteDraft(derived) error = %v", err)
	}
	if _, err := store.Approve("research", derived.ID); err == nil {
		t.Fatalf("Approve(derived with draft support) error = nil")
	}
}

func TestClaimStoreRejectsInvalidApprovalBasis(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisEvidence)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err == nil {
		t.Fatalf("Approve() error = nil, want missing evidence error")
	}
}

func TestClaimStoreApproveDeepSupport(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}

	leaf := validStoreClaim(approvalTestClaimID(1), ClaimBasisOwner)
	leaf = finalizeApprovedStoreClaim(t, leaf)
	writeCanonicalStoreClaim(t, paths, leaf)
	current := leaf
	for number := 2; number <= 96; number++ {
		next := validStoreClaim(approvalTestClaimID(number), ClaimBasisDerived)
		next.SupportingClaimIDs = []string{current.ID}
		next = finalizeApprovedStoreClaim(t, next)
		writeCanonicalStoreClaim(t, paths, next)
		current = next
	}

	root := validStoreClaim(approvalTestClaimID(1000), ClaimBasisDerived)
	root.SupportingClaimIDs = []string{current.ID}
	if _, err := store.WriteDraft("research", root); err != nil {
		t.Fatalf("WriteDraft(root) error = %v", err)
	}
	approved, err := store.Approve("research", root.ID)
	if err != nil {
		t.Fatalf("Approve(deep support) error = %v", err)
	}
	if approved.Status != ClaimStatusApproved {
		t.Fatalf("approved status = %q, want approved", approved.Status)
	}
}

func TestClaimStoreApproveInvalidDigest(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	support := validStoreClaim(approvalTestClaimID(1), ClaimBasisOwner)
	support = finalizeApprovedStoreClaim(t, support)
	writeCanonicalStoreClaim(t, paths, support)
	supportPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", support.ID+".md")
	contents, err := os.ReadFile(supportPath)
	if err != nil {
		t.Fatalf("ReadFile(support) error = %v", err)
	}
	contents = []byte(strings.Replace(string(contents), "Store body", "Tampered body", 1))
	if err := os.WriteFile(supportPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(tampered support) error = %v", err)
	}

	root := validStoreClaim(approvalTestClaimID(2), ClaimBasisDerived)
	root.SupportingClaimIDs = []string{support.ID}
	if _, err := store.WriteDraft("research", root); err != nil {
		t.Fatalf("WriteDraft(root) error = %v", err)
	}
	rootPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", root.ID+".md")
	before := sha256Hex(t, rootPath)
	if _, err := store.Approve("research", root.ID); err == nil || !strings.Contains(err.Error(), "verification digest mismatch") {
		t.Fatalf("Approve(invalid digest) error = %v, want verification digest mismatch", err)
	}
	if after := sha256Hex(t, rootPath); after != before {
		t.Fatalf("draft changed after invalid approval: before %s after %s", before, after)
	}
	unchanged, err := store.Read("research", root.ID)
	if err != nil {
		t.Fatalf("Read(root) error = %v", err)
	}
	if unchanged.Status != ClaimStatusDraft || unchanged.VerifiedAt != "" || unchanged.VerifiedBy != "" || unchanged.VerifiedDigest != "" || len(unchanged.Transitions) != 0 {
		t.Fatalf("draft approval fields changed: %#v", unchanged)
	}
	journalPath, err := PendingTransitionPath(paths, "research")
	if err != nil {
		t.Fatalf("PendingTransitionPath() error = %v", err)
	}
	if _, err := os.Stat(journalPath); !os.IsNotExist(err) {
		t.Fatalf("pending journal after invalid approval: %v", err)
	}
}

func TestClaimStoreApproveRevoked(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	support := validStoreClaim(approvalTestClaimID(1), ClaimBasisOwner)
	if _, err := store.WriteDraft("research", support); err != nil {
		t.Fatalf("WriteDraft(support) error = %v", err)
	}
	if _, err := store.Approve("research", support.ID); err != nil {
		t.Fatalf("Approve(support) error = %v", err)
	}
	if _, err := store.Revoke("research", support.ID, "obsolete"); err != nil {
		t.Fatalf("Revoke(support) error = %v", err)
	}
	root := validStoreClaim(approvalTestClaimID(2), ClaimBasisDerived)
	root.SupportingClaimIDs = []string{support.ID}
	if _, err := store.WriteDraft("research", root); err != nil {
		t.Fatalf("WriteDraft(root) error = %v", err)
	}
	if _, err := store.Approve("research", root.ID); err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("Approve(revoked support) error = %v, want revoked", err)
	}
}

func TestClaimStoreApproveSuperseded(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	old := validStoreClaim(approvalTestClaimID(1), ClaimBasisOwner)
	if _, err := store.WriteDraft("research", old); err != nil {
		t.Fatalf("WriteDraft(old) error = %v", err)
	}
	if _, err := store.Approve("research", old.ID); err != nil {
		t.Fatalf("Approve(old) error = %v", err)
	}
	replacement := validStoreClaim(approvalTestClaimID(2), ClaimBasisOwner)
	if _, err := store.WriteSupersedingDraft("research", old.ID, replacement); err != nil {
		t.Fatalf("WriteSupersedingDraft() error = %v", err)
	}
	if _, err := store.Approve("research", replacement.ID); err != nil {
		t.Fatalf("Approve(replacement) error = %v", err)
	}
	root := validStoreClaim(approvalTestClaimID(3), ClaimBasisDerived)
	root.SupportingClaimIDs = []string{old.ID}
	if _, err := store.WriteDraft("research", root); err != nil {
		t.Fatalf("WriteDraft(root) error = %v", err)
	}
	if _, err := store.Approve("research", root.ID); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("Approve(superseded support) error = %v, want superseded", err)
	}
}

func TestClaimStoreApproveMissingEvidence(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim(approvalTestClaimID(1), ClaimBasisEvidence)
	claim.EvidenceIDs = []string{"evd_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	before := sha256Hex(t, claimPath)
	if _, err := store.Approve("research", claim.ID); err == nil || !strings.Contains(err.Error(), "source.yaml") {
		t.Fatalf("Approve(missing evidence) error = %v, want source.yaml", err)
	}
	if after := sha256Hex(t, claimPath); after != before {
		t.Fatalf("draft changed after missing evidence: before %s after %s", before, after)
	}
}

func TestClaimStoreApproveTamperedEvidenceNoWrite(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	evidence := addStoreEvidence(t, paths, "trusted evidence")
	rawPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(rawPath, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(rawPath, []byte(strings.Repeat("x", int(evidence.ByteLength))), 0o644); err != nil {
		t.Fatalf("WriteFile(raw) error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim(approvalTestClaimID(1), ClaimBasisEvidence)
	claim.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", claim.ID+".md")
	before := sha256Hex(t, claimPath)
	if _, err := store.Approve("research", claim.ID); err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("Approve(tampered evidence) error = %v, want sha256", err)
	}
	if after := sha256Hex(t, claimPath); after != before {
		t.Fatalf("draft changed after tampered evidence: before %s after %s", before, after)
	}
}

func TestClaimStoreApproveRejectsSupportingClaimEvidence(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	evidence := addStoreEvidence(t, paths, "support evidence")
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	support := validStoreClaim(approvalTestClaimID(1), ClaimBasisEvidence)
	support.EvidenceIDs = []string{evidence.ID}
	if _, err := store.WriteDraft("research", support); err != nil {
		t.Fatalf("WriteDraft(support) error = %v", err)
	}
	if _, err := store.Approve("research", support.ID); err != nil {
		t.Fatalf("Approve(support) error = %v", err)
	}
	rawPath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(rawPath, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(rawPath, []byte(strings.Repeat("x", int(evidence.ByteLength))), 0o644); err != nil {
		t.Fatalf("WriteFile(raw) error = %v", err)
	}
	dependent := validStoreClaim(approvalTestClaimID(2), ClaimBasisDerived)
	dependent.SupportingClaimIDs = []string{support.ID}
	if _, err := store.WriteDraft("research", dependent); err != nil {
		t.Fatalf("WriteDraft(dependent) error = %v", err)
	}
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", dependent.ID+".md")
	before := sha256Hex(t, claimPath)
	if _, err := store.Approve("research", dependent.ID); err == nil || !strings.Contains(err.Error(), evidence.ID) || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("Approve(dependent) error = %v, want supporting evidence hash failure", err)
	}
	if after := sha256Hex(t, claimPath); after != before {
		t.Fatalf("dependent draft changed after supporting evidence failure: before %s after %s", before, after)
	}
}

func TestClaimStoreApproveCycle(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	first := validStoreClaim(approvalTestClaimID(1), ClaimBasisDerived)
	second := validStoreClaim(approvalTestClaimID(2), ClaimBasisDerived)
	first.SupportingClaimIDs = []string{second.ID}
	second.SupportingClaimIDs = []string{first.ID}
	writeCanonicalStoreClaim(t, paths, finalizeApprovedStoreClaim(t, first))
	writeCanonicalStoreClaim(t, paths, finalizeApprovedStoreClaim(t, second))
	root := validStoreClaim(approvalTestClaimID(3), ClaimBasisDerived)
	root.SupportingClaimIDs = []string{first.ID}
	if _, err := store.WriteDraft("research", root); err != nil {
		t.Fatalf("WriteDraft(root) error = %v", err)
	}
	if _, err := store.Approve("research", root.ID); err == nil || !strings.Contains(err.Error(), "dependency cycle detected") {
		t.Fatalf("Approve(cycle) error = %v, want dependency cycle detected", err)
	}
}

func TestClaimStoreDoesNotMutateLegacyMarkdown(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	legacyPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "legacy.md")
	legacy := []byte("# Legacy note\n\nNo claim schema here.\n")
	if err := os.WriteFile(legacyPath, legacy, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}
	before := sha256Hex(t, legacyPath)

	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	scan, err := store.ScanWorkspace("research")
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	if len(scan.Claims) != 0 {
		t.Fatalf("len(Claims) = %d, want 0", len(scan.Claims))
	}
	if len(scan.LegacyUnindexed) != 1 || scan.LegacyUnindexed[0] != "projects/legacy.md" {
		t.Fatalf("LegacyUnindexed = %v", scan.LegacyUnindexed)
	}
	if after := sha256Hex(t, legacyPath); after != before {
		t.Fatalf("legacy hash changed: before %s after %s", before, after)
	}
}

func TestClaimStoreScanReportsInvalidClaimWithoutMutation(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "bad.md")
	contents := []byte("---\nschema: zbrain.claim/v1\nid: bad\nstatus: draft\ntitle: Bad\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n---\n\nBad\n")
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(bad) error = %v", err)
	}
	before := sha256Hex(t, claimPath)

	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	scan, err := store.ScanWorkspace("research")
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	if len(scan.Invalid) != 1 || scan.Invalid[0].Path != "projects/bad.md" {
		t.Fatalf("Invalid = %#v", scan.Invalid)
	}
	if after := sha256Hex(t, claimPath); after != before {
		t.Fatalf("invalid claim hash changed: before %s after %s", before, after)
	}
}

func TestClaimStoreScanRejectsTamperedApprovedClaim(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
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
	contents = []byte(strings.Replace(string(contents), "Store body", "Tampered body", 1))
	if err := os.WriteFile(claimPath, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(tampered claim) error = %v", err)
	}

	scan, err := store.ScanWorkspace("research")
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	if len(scan.Claims) != 0 {
		t.Fatalf("len(Claims) = %d, want 0", len(scan.Claims))
	}
	if len(scan.Invalid) != 1 {
		t.Fatalf("Invalid = %#v, want one digest mismatch", scan.Invalid)
	}
	if scan.Invalid[0].Path != "projects/"+claim.ID+".md" || !strings.Contains(scan.Invalid[0].Error, "verification digest mismatch") {
		t.Fatalf("Invalid = %#v, want tampered path and digest mismatch", scan.Invalid)
	}
}

func TestClaimStoreMigrateOKFConvertsLegacyClaim(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	id := "clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", id+".md")
	legacy := []byte("---\nschema: zbrain.claim/v1\nid: " + id + "\nstatus: draft\ntitle: Legacy Claim\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\ntags: [legacy]\n---\n\nLegacy body\n")
	if err := os.WriteFile(claimPath, legacy, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy claim) error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	summary, err := store.MigrateOKF("research")
	if err != nil {
		t.Fatalf("MigrateOKF() error = %v", err)
	}
	if summary.Migrated != 1 || summary.Invalid != 0 || summary.ReapprovalRequired != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	contents, err := os.ReadFile(claimPath)
	if err != nil {
		t.Fatalf("ReadFile(migrated) error = %v", err)
	}
	if strings.Contains(string(contents), "schema: zbrain.claim/v1") || !strings.Contains(string(contents), "type: zbrain.claim") || !strings.Contains(string(contents), "profile: zbrain.trusted-memory/v1") {
		t.Fatalf("legacy claim was not migrated to OKF:\n%s", contents)
	}
}

func TestLegacyMigrationRequiresExplicitReapproval(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	id := "clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", id+".md")
	legacy := []byte("---\nschema: zbrain.claim/v1\nid: " + id + "\nstatus: approved\ntitle: Legacy Approved\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n---\n\nLegacy approved body\n")
	if err := os.WriteFile(claimPath, legacy, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy claim) error = %v", err)
	}

	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	summary, err := store.MigrateOKF("research")
	if err != nil {
		t.Fatalf("MigrateOKF() error = %v", err)
	}
	if summary.Migrated != 1 || summary.Invalid != 0 || summary.ReapprovalRequired != 1 || len(summary.ReapprovalCandidates) != 1 || summary.ReapprovalCandidates[0] != id {
		t.Fatalf("summary = %#v", summary)
	}
	migrated, err := store.Read("research", id)
	if err != nil {
		t.Fatalf("Read(migrated) error = %v", err)
	}
	if migrated.Status != ClaimStatusDraft || migrated.ID != id || migrated.Body != "\nLegacy approved body\n" || migrated.VerifiedAt != "" || migrated.VerifiedBy != "" || migrated.VerifiedDigest != "" {
		t.Fatalf("migrated claim = %#v, want explicit draft reapproval candidate", migrated)
	}
	if err := VerifyClaimDigest(migrated); err != nil {
		t.Fatalf("VerifyClaimDigest(migrated draft) error = %v", err)
	}

	approved, err := store.Approve("research", id)
	if err != nil {
		t.Fatalf("Approve(migrated) error = %v", err)
	}
	if approved.Status != ClaimStatusApproved || approved.VerifiedDigest == "" {
		t.Fatalf("approved migrated claim = %#v", approved)
	}
	if err := VerifyClaimDigest(approved); err != nil {
		t.Fatalf("VerifyClaimDigest(approved migrated claim) error = %v", err)
	}
}

func TestLegacyMigrationPreservesApprovedClaimWithValidOKFDigest(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	id := "clm_cccccccccccccccccccccccccccccccc"
	verifiedAt := fixedClaimStoreNow().Format(time.RFC3339)
	claim := validStoreClaim(id, ClaimBasisOwner)
	claim.Body = "\nStore body\n"
	claim.Status = ClaimStatusApproved
	claim.VerifiedAt = verifiedAt
	claim.VerifiedBy = "owner"
	digest, err := ClaimVerificationDigest(claim)
	if err != nil {
		t.Fatalf("ClaimVerificationDigest() error = %v", err)
	}
	legacy := []byte("---\nschema: zbrain.claim/v1\nid: " + id + "\nstatus: approved\ntitle: Store claim\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\nverified:\n  at: " + verifiedAt + "\n  by: owner\n  digest: " + digest + "\n---\n\nStore body\n")
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", id+".md")
	if err := os.WriteFile(claimPath, legacy, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy claim) error = %v", err)
	}

	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	summary, err := store.MigrateOKF("research")
	if err != nil {
		t.Fatalf("MigrateOKF() error = %v", err)
	}
	if summary.Migrated != 1 || summary.Invalid != 0 || summary.ReapprovalRequired != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	migrated, err := store.Read("research", id)
	if err != nil {
		t.Fatalf("Read(migrated) error = %v", err)
	}
	if migrated.Status != ClaimStatusApproved || migrated.VerifiedDigest != digest {
		t.Fatalf("migrated approved claim = %#v, want preserved valid OKF attestation", migrated)
	}
	if err := VerifyClaimDigest(migrated); err != nil {
		t.Fatalf("VerifyClaimDigest(migrated) error = %v", err)
	}
}

func TestClaimStoreRejectsApprovedInPlaceOverwrite(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	claim.Body = "Mutated body\n"
	if _, err := store.WriteDraft("research", claim); err == nil {
		t.Fatalf("WriteDraft(existing approved) error = nil")
	}
}

func TestClaimMutationAuthorizationMetadata(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	authorization := func(id string) *ClaimTransitionAuthorization {
		return &ClaimTransitionAuthorization{
			ChallengeID: id,
			Method:      "claim_lifecycle.apply",
			MCPClient:   "mcp-client/1.0",
		}
	}

	first := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", first); err != nil {
		t.Fatalf("WriteDraft(first) error = %v", err)
	}
	approved, err := store.ApproveWithOptions("research", first.ID, ClaimMutationOptions{
		VerifiedBy:    "owner:mcp",
		Authorization: authorization("chg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	})
	if err != nil {
		t.Fatalf("ApproveWithOptions() error = %v", err)
	}
	if approved.VerifiedBy != "owner:mcp" || approved.Transitions[0].Authorization == nil {
		t.Fatalf("approval provenance = %#v", approved)
	}
	if got := approved.Transitions[0].Authorization; got.ChallengeID != "chg_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || got.Method != "claim_lifecycle.apply" || got.MCPClient != "mcp-client/1.0" {
		t.Fatalf("approval authorization = %#v", got)
	}

	replacement := validStoreClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ClaimBasisOwner)
	replacement.Body = "Replacement body\n"
	if _, err := store.WriteSupersedingDraft("research", approved.ID, replacement); err != nil {
		t.Fatalf("WriteSupersedingDraft() error = %v", err)
	}
	superseded, err := store.ApproveWithOptions("research", replacement.ID, ClaimMutationOptions{
		VerifiedBy:    "owner:mcp",
		Authorization: authorization("chg_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
	})
	if err != nil {
		t.Fatalf("ApproveWithOptions(supersede) error = %v", err)
	}
	if got := superseded.Transitions[0].Authorization; got == nil || got.ChallengeID != "chg_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("replacement authorization = %#v", got)
	}
	old, err := store.Read("research", approved.ID)
	if err != nil {
		t.Fatalf("Read(old) error = %v", err)
	}
	if got := old.Transitions[1].Authorization; got == nil || got.ChallengeID != "chg_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" || got.Method != "claim_lifecycle.apply" || got.MCPClient != "mcp-client/1.0" {
		t.Fatalf("supersession authorization = %#v", got)
	}

	revoked, err := store.RevokeWithOptions("research", superseded.ID, "wrong scope", ClaimMutationOptions{
		VerifiedBy:    "owner:mcp",
		Authorization: authorization("chg_cccccccccccccccccccccccccccccccc"),
	})
	if err != nil {
		t.Fatalf("RevokeWithOptions() error = %v", err)
	}
	if revoked.VerifiedBy != "owner:mcp" {
		t.Fatalf("revocation verified.by = %q", revoked.VerifiedBy)
	}
	if got := revoked.Transitions[len(revoked.Transitions)-1].Authorization; got == nil || got.ChallengeID != "chg_cccccccccccccccccccccccccccccccc" {
		t.Fatalf("revocation authorization = %#v", got)
	}
	if err := VerifyClaimDigest(revoked); err != nil {
		t.Fatalf("VerifyClaimDigest(revoked) error = %v", err)
	}
}

func TestPrepareAndApplyChallengeAtomicallyAuthorizesApproval(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	prepared, err := store.PrepareChallenge("research", ChallengePrepare{
		Workspace: "research",
		Operation: ChallengeOperationApprove,
		ClaimID:   claim.ID,
	})
	if err != nil {
		t.Fatalf("PrepareChallenge() error = %v", err)
	}
	if _, err := store.ApplyChallenge("research", prepared.Challenge.ID, "not-issued", ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "has not been owner-granted") {
		t.Fatalf("ApplyChallenge(before grant) error = %v, want owner-grant rejection", err)
	}
	unchanged, err := store.Read("research", claim.ID)
	if err != nil {
		t.Fatalf("Read(after ungranted apply) error = %v", err)
	}
	if unchanged.Status != ClaimStatusDraft || unchanged.VerifiedDigest != "" || len(unchanged.Transitions) != 0 {
		t.Fatalf("claim changed after ungranted apply: %#v", unchanged)
	}
	unconsumed, err := (ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}).Read("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Read(challenge after ungranted apply) error = %v", err)
	}
	if unconsumed.Granted || unconsumed.Consumed {
		t.Fatalf("challenge changed after ungranted apply: %#v", unconsumed)
	}
	granted, err := (ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}).Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	token := granted.Token
	approved, err := store.ApplyChallenge("research", prepared.Challenge.ID, token, ClaimMutationOptions{
		VerifiedBy: "owner:mcp",
		Authorization: &ClaimTransitionAuthorization{
			ChallengeID: prepared.Challenge.ID,
			Method:      "claim_lifecycle.apply",
			MCPClient:   "mcp-client/1.0",
		},
	})
	if err != nil {
		t.Fatalf("ApplyChallenge() error = %v", err)
	}
	if approved.Status != ClaimStatusApproved || approved.VerifiedBy != "owner:mcp" {
		t.Fatalf("ApplyChallenge() claim = %#v", approved)
	}
	if got := approved.Transitions[0].Authorization; got == nil || got.ChallengeID != prepared.Challenge.ID {
		t.Fatalf("ApplyChallenge() authorization = %#v", got)
	}
	if _, err := store.ApplyChallenge("research", prepared.Challenge.ID, token, ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "already consumed") {
		t.Fatalf("ApplyChallenge(replay) error = %v, want consumed", err)
	}
}

func TestApplyChallengeAuthorizesSupersedeAndRevoke(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	old := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", old); err != nil {
		t.Fatalf("WriteDraft(old) error = %v", err)
	}
	oldApproved, err := store.Approve("research", old.ID)
	if err != nil {
		t.Fatalf("Approve(old) error = %v", err)
	}
	replacement := validStoreClaim("clm_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ClaimBasisOwner)
	replacement, err = store.WriteSupersedingDraft("research", oldApproved.ID, replacement)
	if err != nil {
		t.Fatalf("WriteSupersedingDraft() error = %v", err)
	}
	supersedingDigest, err := store.CanonicalDraftDigest("research", replacement.ID)
	if err != nil {
		t.Fatalf("CanonicalDraftDigest(replacement) error = %v", err)
	}
	supersedeChallenge, err := store.PrepareChallenge("research", ChallengePrepare{
		Workspace:               "research",
		Operation:               ChallengeOperationSupersede,
		ClaimID:                 replacement.ID,
		CanonicalDraftDigest:    supersedingDigest,
		SupersededIDs:           replacement.Supersedes,
		PriorVerificationDigest: oldApproved.VerifiedDigest,
	})
	if err != nil {
		t.Fatalf("PrepareChallenge(supersede) error = %v", err)
	}
	grantedSupersede, err := (ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}).Grant("research", supersedeChallenge.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant(supersede) error = %v", err)
	}
	supersedeToken := grantedSupersede.Token
	superseded, err := store.ApplyChallenge("research", supersedeChallenge.Challenge.ID, supersedeToken, ClaimMutationOptions{
		VerifiedBy: "owner:mcp",
		Authorization: &ClaimTransitionAuthorization{
			ChallengeID: supersedeChallenge.Challenge.ID,
			Method:      "claim_lifecycle.apply",
			MCPClient:   "mcp-client/1.0",
		},
	})
	if err != nil {
		t.Fatalf("ApplyChallenge(supersede) error = %v", err)
	}
	if superseded.Status != ClaimStatusApproved || superseded.Transitions[0].Kind != ClaimTransitionSupersede {
		t.Fatalf("superseded replacement = %#v", superseded)
	}
	oldAfter, err := store.Read("research", old.ID)
	if err != nil {
		t.Fatalf("Read(old after supersede) error = %v", err)
	}
	if oldAfter.Status != ClaimStatusSuperseded || oldAfter.Transitions[1].Authorization == nil || oldAfter.Transitions[1].Authorization.ChallengeID != supersedeChallenge.Challenge.ID {
		t.Fatalf("old supersession metadata = %#v", oldAfter)
	}

	revokeDigest, err := store.CanonicalDraftDigest("research", replacement.ID)
	if err != nil {
		t.Fatalf("CanonicalDraftDigest(revoke) error = %v", err)
	}
	revokeChallenge, err := store.PrepareChallenge("research", ChallengePrepare{
		Workspace:               "research",
		Operation:               ChallengeOperationRevoke,
		ClaimID:                 replacement.ID,
		CanonicalDraftDigest:    revokeDigest,
		SupersededIDs:           replacement.Supersedes,
		PriorVerificationDigest: superseded.VerifiedDigest,
		RevokeReason:            "wrong scope",
	})
	if err != nil {
		t.Fatalf("PrepareChallenge(revoke) error = %v", err)
	}
	grantedRevoke, err := (ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}).Grant("research", revokeChallenge.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant(revoke) error = %v", err)
	}
	revokeToken := grantedRevoke.Token
	revoked, err := store.ApplyChallenge("research", revokeChallenge.Challenge.ID, revokeToken, ClaimMutationOptions{
		VerifiedBy: "owner:mcp",
		Authorization: &ClaimTransitionAuthorization{
			ChallengeID: revokeChallenge.Challenge.ID,
			Method:      "claim_lifecycle.apply",
			MCPClient:   "mcp-client/1.0",
		},
	})
	if err != nil {
		t.Fatalf("ApplyChallenge(revoke) error = %v", err)
	}
	if revoked.Status != ClaimStatusRevoked || revoked.Transitions[len(revoked.Transitions)-1].Authorization == nil || revoked.Transitions[len(revoked.Transitions)-1].Authorization.ChallengeID != revokeChallenge.Challenge.ID {
		t.Fatalf("revoked metadata = %#v", revoked)
	}
}

func TestApplyChallengeRejectsStaleCanonicalDigestBeforeConsumption(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	digest, err := store.CanonicalDraftDigest("research", claim.ID)
	if err != nil {
		t.Fatalf("CanonicalDraftDigest() error = %v", err)
	}
	prepared, err := store.PrepareChallenge("research", ChallengePrepare{
		Workspace:            "research",
		Operation:            ChallengeOperationApprove,
		ClaimID:              claim.ID,
		CanonicalDraftDigest: digest,
	})
	if err != nil {
		t.Fatalf("PrepareChallenge() error = %v", err)
	}
	grantedStale, err := (ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}).Grant("research", prepared.Challenge.ID)
	if err != nil {
		t.Fatalf("Grant() error = %v", err)
	}
	token := grantedStale.Token
	claim.Body = "Changed after challenge\n"
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft(changed) error = %v", err)
	}
	if _, err := store.ApplyChallenge("research", prepared.Challenge.ID, token, ClaimMutationOptions{}); err == nil || !strings.Contains(err.Error(), "canonical draft digest is stale") {
		t.Fatalf("ApplyChallenge(stale) error = %v, want stale digest", err)
	}
	if _, err := (ChallengeStore{Paths: paths, Now: fixedClaimStoreNow}).Verify("research", prepared.Challenge.ID, token); err != nil {
		t.Fatalf("Verify(after stale apply) error = %v, want token unconsumed", err)
	}
}

func claimStoreTestPaths(t *testing.T) (Paths, string) {
	t.Helper()
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", fixedClaimStoreNow()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return paths, tmp
}

func validStoreClaim(id string, basis ClaimBasis) Claim {
	return Claim{
		Type:      OKFClaimType,
		ID:        id,
		Tier:      "projects",
		Status:    ClaimStatusDraft,
		Title:     "Store claim",
		Basis:     basis,
		CreatedAt: fixedClaimStoreNow().Format(time.RFC3339),
		CreatedBy: "owner",
		Body:      "Store body\n",
	}
}

func approvalTestClaimID(number int) string {
	return fmt.Sprintf("clm_%032x", number)
}

func finalizeApprovedStoreClaim(t *testing.T, claim Claim) Claim {
	t.Helper()
	claim.Status = ClaimStatusApproved
	claim.VerifiedAt = fixedClaimStoreNow().Format(time.RFC3339)
	claim.VerifiedBy = "owner"
	claim.VerifiedDigest = ""
	digest, err := ClaimVerificationDigest(claim)
	if err != nil {
		t.Fatalf("ClaimVerificationDigest() error = %v", err)
	}
	claim.VerifiedDigest = digest
	return claim
}

func writeCanonicalStoreClaim(t *testing.T, paths Paths, claim Claim) {
	t.Helper()
	path := filepath.Join(paths.WorkspacesDir, "research", "wiki", claim.Tier, claim.ID+".md")
	if err := writeClaimAtomic(path, claim); err != nil {
		t.Fatalf("writeClaimAtomic(%s) error = %v", claim.ID, err)
	}
}

func fixedClaimStoreNow() time.Time {
	return time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
}

func addStoreEvidence(t *testing.T, paths Paths, body string) Evidence {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	evidence, err := (EvidenceStore{Paths: paths, Now: fixedClaimStoreNow}).AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	return evidence
}

func TestMutationRecoversPendingTransition(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	recoveryPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "recovery.md")
	before := []byte("before mutation\n")
	target := []byte("recovered mutation\n")
	if err := os.WriteFile(recoveryPath, before, 0o644); err != nil {
		t.Fatalf("WriteFile(before) error = %v", err)
	}
	if err := WritePendingTransition(paths, "research", PendingTransition{
		OperationID: "txn_mutation",
		Kind:        ClaimTransitionSupersede,
		Workspace:   "research",
		Targets:     []PendingTransitionTarget{pendingTransitionTarget("wiki/projects/recovery.md", before, target)},
	}); err != nil {
		t.Fatalf("WritePendingTransition() error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_cccccccccccccccccccccccccccccccc", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	assertFileBytes(t, recoveryPath, target)
	assertNoPendingTransition(t, paths, "research")
	if _, err := store.Read("research", claim.ID); err != nil {
		t.Fatalf("Read(new claim) error = %v", err)
	}
}

func TestClaimStoreWorkspaceIsolationAndSymlinkBoundary(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	if err := CreateWorkspace(paths, "personal", fixedClaimStoreNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_cccccccccccccccccccccccccccccccc", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Read("personal", claim.ID); err == nil {
		t.Fatalf("Read(cross workspace) error = nil")
	}

	outside := t.TempDir()
	outsidePath := filepath.Join(outside, claim.ID+".md")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	escapePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "axioms", claim.ID+".md")
	if err := os.Symlink(outsidePath, escapePath); err != nil {
		t.Fatalf("Symlink(escape) error = %v", err)
	}
	if _, err := store.Read("research", claim.ID); err == nil {
		t.Fatalf("Read(symlink escape) error = nil")
	}
}

func TestClaimStoreDirtyBarrierLeavesCanonicalTreeUnchanged(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	dirtyPaths := paths
	dirtyPaths.IndexesDir = filepath.Join(t.TempDir(), "indexes-blocker")
	if err := os.WriteFile(dirtyPaths.IndexesDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(indexes blocker) error = %v", err)
	}
	store := ClaimStore{Paths: dirtyPaths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_dddddddddddddddddddddddddddddddd", ClaimBasisOwner)
	projectsDir := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects")
	before, err := os.ReadDir(projectsDir)
	if err != nil {
		t.Fatalf("ReadDir(before) error = %v", err)
	}
	if _, err := store.WriteDraft("research", claim); err == nil {
		t.Fatalf("WriteDraft(dirty failure) error = nil")
	}
	after, err := os.ReadDir(projectsDir)
	if err != nil {
		t.Fatalf("ReadDir(after) error = %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("claim tree changed after dirty failure: before=%v after=%v", before, after)
	}
	if _, err := os.Stat(filepath.Join(dirtyPaths.IndexesDir, "research.dirty")); err == nil {
		t.Fatalf("dirty marker exists after dirty failure")
	}
}

func TestClaimStoreMigrateDirtyBarrierLeavesCanonicalBytesUnchanged(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	id := "clm_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", id+".md")
	legacy := []byte("---\nschema: zbrain.claim/v1\nid: " + id + "\nstatus: draft\ntitle: Legacy Claim\nbasis: owner\ncreated_at: 2026-07-30T09:00:00Z\ncreated_by: owner\n---\n\nLegacy body\n")
	if err := os.WriteFile(claimPath, legacy, 0o644); err != nil {
		t.Fatalf("WriteFile(legacy) error = %v", err)
	}
	dirtyPaths := paths
	dirtyPaths.IndexesDir = filepath.Join(t.TempDir(), "indexes-blocker")
	if err := os.WriteFile(dirtyPaths.IndexesDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(indexes blocker) error = %v", err)
	}
	before := sha256Hex(t, claimPath)
	if _, err := (ClaimStore{Paths: dirtyPaths, Now: fixedClaimStoreNow}).MigrateOKF("research"); err == nil {
		t.Fatalf("MigrateOKF(dirty failure) error = nil")
	}
	if after := sha256Hex(t, claimPath); after != before {
		t.Fatalf("legacy claim changed after dirty failure: before %s after %s", before, after)
	}
}

func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func TestClaimStoreScanRejectsDuplicateCanonicalClaimIDs(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	id := "clm_44444444444444444444444444444444"
	flat := validStoreClaim(id, ClaimBasisOwner)
	flat.Title = "Flat duplicate marker"
	flat.Body = "flat duplicate marker\n"
	writeCanonicalStoreClaim(t, paths, flat)

	nestedPath := "projects/topics/security/" + id + ".md"
	nested := flat
	nested.Title = "Nested duplicate marker"
	nested.Body = "nested duplicate marker\n"
	nested.Path = nestedPath
	writePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(nestedPath))
	if err := writeClaimAtomic(writePath, nested); err != nil {
		t.Fatalf("writeClaimAtomic(nested duplicate) error = %v", err)
	}

	flatPath := "projects/" + id + ".md"
	beforeFlat := sha256Hex(t, filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(flatPath)))
	beforeNested := sha256Hex(t, writePath)
	scan, err := store.ScanWorkspace("research")
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	if len(scan.Claims) != 0 {
		t.Fatalf("Claims = %#v, want no ambiguous claims", scan.Claims)
	}
	if len(scan.Invalid) != 2 {
		t.Fatalf("Invalid = %#v, want one entry per duplicate path", scan.Invalid)
	}
	wantPaths := []string{flatPath, nestedPath}
	for index, invalid := range scan.Invalid {
		if invalid.Path != wantPaths[index] {
			t.Fatalf("Invalid[%d].Path = %q, want %q", index, invalid.Path, wantPaths[index])
		}
		for _, path := range wantPaths {
			if !strings.Contains(invalid.Error, path) {
				t.Fatalf("Invalid[%d].Error = %q, want path %q", index, invalid.Error, path)
			}
		}
		if !strings.Contains(invalid.Error, id) || !strings.Contains(invalid.Error, "duplicate canonical claim ID") {
			t.Fatalf("Invalid[%d].Error = %q, want deterministic duplicate-ID reason", index, invalid.Error)
		}
	}
	if after := sha256Hex(t, filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(flatPath))); after != beforeFlat {
		t.Fatalf("flat duplicate changed: before %s after %s", beforeFlat, after)
	}
	if after := sha256Hex(t, writePath); after != beforeNested {
		t.Fatalf("nested duplicate changed: before %s after %s", beforeNested, after)
	}

	trustScan, err := store.ScanWorkspaceForTrust("research")
	if err != nil {
		t.Fatalf("ScanWorkspaceForTrust() error = %v", err)
	}
	if len(trustScan.Claims) != 0 || len(trustScan.Invalid) != 2 {
		t.Fatalf("trust scan = %#v, want no claims and two invalid duplicate paths", trustScan)
	}
}

func TestClaimStoreDuplicateCanonicalIDBlocksLifecycle(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	id := "clm_55555555555555555555555555555555"
	flat := validStoreClaim(id, ClaimBasisOwner)
	flat.Title = "Flat lifecycle duplicate"
	flat.Body = "flat lifecycle duplicate\n"
	writeCanonicalStoreClaim(t, paths, flat)

	nestedPath := "projects/topics/security/" + id + ".md"
	nested := flat
	nested.Title = "Nested lifecycle duplicate"
	nested.Body = "nested lifecycle duplicate\n"
	nested.Path = nestedPath
	nestedAbsolutePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(nestedPath))
	if err := writeClaimAtomic(nestedAbsolutePath, nested); err != nil {
		t.Fatalf("writeClaimAtomic(nested duplicate) error = %v", err)
	}

	flatPath := "projects/" + id + ".md"
	flatAbsolutePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(flatPath))
	beforeFlat := sha256Hex(t, flatAbsolutePath)
	beforeNested := sha256Hex(t, nestedAbsolutePath)
	expectDuplicate := func(operation string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s error = nil, want duplicate-ID rejection", operation)
		}
		if !strings.Contains(err.Error(), "duplicate canonical claim ID") || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), flatPath) || !strings.Contains(err.Error(), nestedPath) {
			t.Fatalf("%s error = %v, want deterministic duplicate-ID paths", operation, err)
		}
	}

	if _, err := store.Read("research", id); err != nil {
		expectDuplicate("Read", err)
	} else {
		t.Fatalf("Read() error = nil, want duplicate-ID rejection")
	}

	flat.Body = "mutated flat duplicate\n"
	if _, err := store.WriteDraft("research", flat); err != nil {
		expectDuplicate("WriteDraft(flat)", err)
	} else {
		t.Fatalf("WriteDraft(flat) error = nil, want duplicate-ID rejection")
	}
	nested.Body = "mutated nested duplicate\n"
	if _, err := store.WriteDraft("research", nested); err != nil {
		expectDuplicate("WriteDraft(nested)", err)
	} else {
		t.Fatalf("WriteDraft(nested) error = nil, want duplicate-ID rejection")
	}
	if _, err := store.Approve("research", id); err != nil {
		expectDuplicate("Approve", err)
	} else {
		t.Fatalf("Approve() error = nil, want duplicate-ID rejection")
	}
	if _, err := store.Revoke("research", id, "ambiguous"); err != nil {
		expectDuplicate("Revoke", err)
	} else {
		t.Fatalf("Revoke() error = nil, want duplicate-ID rejection")
	}
	replacementID := "clm_66666666666666666666666666666666"
	if _, err := store.WriteSupersedingDraft("research", id, validStoreClaim(replacementID, ClaimBasisOwner)); err != nil {
		expectDuplicate("WriteSupersedingDraft", err)
	} else {
		t.Fatalf("WriteSupersedingDraft() error = nil, want duplicate-ID rejection")
	}

	if after := sha256Hex(t, flatAbsolutePath); after != beforeFlat {
		t.Fatalf("flat duplicate changed: before %s after %s", beforeFlat, after)
	}
	if after := sha256Hex(t, nestedAbsolutePath); after != beforeNested {
		t.Fatalf("nested duplicate changed: before %s after %s", beforeNested, after)
	}
	replacementPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", replacementID+".md")
	if _, err := os.Stat(replacementPath); !os.IsNotExist(err) {
		t.Fatalf("replacement claim was created, stat error = %v", err)
	}
}

func TestClaimStoreAliasFilenameDuplicateCanonicalID(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	id := "clm_aaaaaaaabbbbbbbbccccccccdddddddd"
	flat := validStoreClaim(id, ClaimBasisOwner)
	writeCanonicalStoreClaim(t, paths, flat)

	aliasPath := "projects/topics/security/alias.md"
	alias := flat
	alias.Path = aliasPath
	alias.Title = "Alias duplicate"
	alias.Body = "alias duplicate\n"
	aliasAbsolutePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(aliasPath))
	if err := writeClaimAtomic(aliasAbsolutePath, alias); err != nil {
		t.Fatalf("writeClaimAtomic(alias duplicate) error = %v", err)
	}

	flatPath := "projects/" + id + ".md"
	flatAbsolutePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(flatPath))
	beforeFlat := sha256Hex(t, flatAbsolutePath)
	beforeAlias := sha256Hex(t, aliasAbsolutePath)
	expectDuplicate := func(operation string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s error = nil, want duplicate-ID rejection", operation)
		}
		if !strings.Contains(err.Error(), "duplicate canonical claim ID") || !strings.Contains(err.Error(), id) || !strings.Contains(err.Error(), flatPath) || !strings.Contains(err.Error(), aliasPath) {
			t.Fatalf("%s error = %v, want duplicate-ID paths", operation, err)
		}
	}

	if _, err := store.Read("research", id); err != nil {
		expectDuplicate("Read", err)
	} else {
		t.Fatalf("Read() error = nil, want duplicate-ID rejection")
	}
	alias.Body = "mutated alias duplicate\n"
	if _, err := store.WriteDraft("research", alias); err != nil {
		expectDuplicate("WriteDraft(alias)", err)
	} else {
		t.Fatalf("WriteDraft(alias) error = nil, want duplicate-ID rejection")
	}

	scan, err := store.ScanWorkspace("research")
	if err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	if len(scan.Claims) != 0 || len(scan.Invalid) != 2 {
		t.Fatalf("scan = %#v, want no claims and two invalid duplicate paths", scan)
	}
	for _, invalid := range scan.Invalid {
		if !strings.Contains(invalid.Error, id) || !strings.Contains(invalid.Error, flatPath) || !strings.Contains(invalid.Error, aliasPath) {
			t.Fatalf("invalid claim = %#v, want both duplicate paths", invalid)
		}
	}
	if after := sha256Hex(t, flatAbsolutePath); after != beforeFlat {
		t.Fatalf("flat duplicate changed: before %s after %s", beforeFlat, after)
	}
	if after := sha256Hex(t, aliasAbsolutePath); after != beforeAlias {
		t.Fatalf("alias duplicate changed: before %s after %s", beforeAlias, after)
	}
}

func TestNestedClaimBoundary(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	id := "clm_11111111111111111111111111111111"
	nestedPath := "projects/topics/security/" + id + ".md"
	claim := validStoreClaim(id, ClaimBasisOwner)
	claim.Title = "Nested boundary marker"
	claim.Body = "nested boundary marker\n"
	claim.Path = nestedPath
	canonicalPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(nestedPath))
	if err := writeClaimAtomic(canonicalPath, claim); err != nil {
		t.Fatalf("writeClaimAtomic(nested) error = %v", err)
	}

	loaded, err := store.Read("research", id)
	if err != nil {
		t.Fatalf("Read(nested) error = %v", err)
	}
	if loaded.Path != nestedPath {
		t.Fatalf("Read(nested).Path = %q, want %q", loaded.Path, nestedPath)
	}

	claim.Body = "updated nested boundary marker\n"
	written, err := store.WriteDraft("research", claim)
	if err != nil {
		t.Fatalf("WriteDraft(nested) error = %v", err)
	}
	if written.Path != nestedPath {
		t.Fatalf("WriteDraft(nested).Path = %q, want %q", written.Path, nestedPath)
	}
	rootPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", id+".md")
	if _, err := os.Stat(rootPath); !os.IsNotExist(err) {
		t.Fatalf("nested draft created root duplicate, stat error = %v", err)
	}

	approved, err := store.Approve("research", id)
	if err != nil {
		t.Fatalf("Approve(nested) error = %v", err)
	}
	if approved.Path != nestedPath || approved.Status != ClaimStatusApproved {
		t.Fatalf("approved nested claim = %#v", approved)
	}

	conflictID := "clm_22222222222222222222222222222222"
	conflictPath := "projects/topics/security/conflict/" + conflictID + ".md"
	conflict := validStoreClaim(conflictID, ClaimBasisOwner)
	conflict.Title = "Nested conflict marker"
	conflict.Body = "nested conflict marker\n"
	conflict.ConflictsWith = []string{id}
	conflict.Path = conflictPath
	if err := writeClaimAtomic(filepath.Join(paths.WorkspacesDir, "research", "wiki", filepath.FromSlash(conflictPath)), conflict); err != nil {
		t.Fatalf("writeClaimAtomic(conflict) error = %v", err)
	}
	if _, err := store.WriteDraft("research", conflict); err != nil {
		t.Fatalf("WriteDraft(conflict) error = %v", err)
	}
	conflictApproved, err := store.Approve("research", conflictID)
	if err != nil {
		t.Fatalf("Approve(conflict) error = %v", err)
	}
	if conflictApproved.Path != conflictPath {
		t.Fatalf("approved conflict path = %q, want %q", conflictApproved.Path, conflictPath)
	}

	if err := CreateWorkspace(paths, "personal", fixedClaimStoreNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}
	if _, err := store.Read("personal", id); err == nil {
		t.Fatalf("Read(cross workspace) error = nil")
	}

	index := IndexStore{Paths: paths}
	if _, err := index.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild(nested) error = %v", err)
	}
	results, err := index.Search("research", SearchOptions{Query: "nested", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10})
	if err != nil {
		t.Fatalf("Search(nested) error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("nested search results = %#v, want two claims", results)
	}
	indexedPaths := make(map[string]string, len(results))
	queryClaims := make([]QueryClaim, 0, len(results))
	for _, result := range results {
		indexedPaths[result.ID] = result.Path
		queryClaims = append(queryClaims, QueryClaim{Workspace: "research", ID: result.ID, Path: result.Path})
	}
	if indexedPaths[id] != nestedPath || indexedPaths[conflictID] != conflictPath {
		t.Fatalf("indexed nested paths = %#v", indexedPaths)
	}
	conflicts, err := findQueryConflicts(paths, queryClaims)
	if err != nil {
		t.Fatalf("findQueryConflicts(nested) error = %v", err)
	}
	if len(conflicts) != 1 || len(conflicts[0].ClaimIDs) != 2 {
		t.Fatalf("nested conflicts = %#v", conflicts)
	}
}

func TestRuntimePermissions(t *testing.T) {
	paths, _ := claimStoreTestPaths(t)
	assertPermissionMode(t, paths.RuntimeDir, runtimeDirectoryMode)
	assertPermissionMode(t, paths.ConfigFile, runtimeMetadataMode)
	assertPermissionMode(t, paths.WorkspacesDir, runtimeDirectoryMode)
	workspaceRoot := filepath.Join(paths.WorkspacesDir, "research")
	assertPermissionMode(t, workspaceRoot, runtimeDirectoryMode)
	assertPermissionMode(t, filepath.Join(workspaceRoot, "wiki", "projects"), runtimeDirectoryMode)
	assertPermissionMode(t, filepath.Join(workspaceRoot, "workspace.md"), runtimeMetadataMode)
	assertPermissionMode(t, filepath.Join(workspaceRoot, "evidence"), evidenceDirectoryMode)

	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("immutable evidence"), 0o600); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	evidence, err := (EvidenceStore{Paths: paths, Now: fixedClaimStoreNow}).AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	evidenceRoot := filepath.Join(workspaceRoot, "evidence", "sources", evidence.ID)
	assertPermissionMode(t, filepath.Join(workspaceRoot, "evidence", "sources"), evidenceDirectoryMode)
	assertPermissionMode(t, evidenceRoot, evidenceDirectoryMode)
	assertPermissionMode(t, filepath.Join(evidenceRoot, "raw"), evidenceFileMode)
	assertPermissionMode(t, filepath.Join(evidenceRoot, "source.yaml"), evidenceFileMode)

	store := ClaimStore{Paths: paths, Now: fixedClaimStoreNow}
	claim := validStoreClaim("clm_33333333333333333333333333333333", ClaimBasisOwner)
	if _, err := store.WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	claimPath := filepath.Join(workspaceRoot, "wiki", "projects", claim.ID+".md")
	assertPermissionMode(t, claimPath, runtimeMetadataMode)
	if _, err := store.Approve("research", claim.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	assertPermissionMode(t, claimPath, runtimeMetadataMode)

	index := IndexStore{Paths: paths}
	if _, err := index.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	assertPermissionMode(t, paths.IndexesDir, runtimeDirectoryMode)
	databasePath, err := index.DatabasePath("research")
	if err != nil {
		t.Fatalf("DatabasePath() error = %v", err)
	}
	assertPermissionMode(t, databasePath, derivedIndexMode)
	if err := index.MarkDirty("research"); err != nil {
		t.Fatalf("MarkDirty() error = %v", err)
	}
	dirtyPath, err := index.DirtyPath("research")
	if err != nil {
		t.Fatalf("DirtyPath() error = %v", err)
	}
	assertPermissionMode(t, dirtyPath, derivedIndexMode)
}

func assertPermissionMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want.Perm() {
		t.Fatalf("mode(%s) = %04o, want %04o", path, got.Perm(), want.Perm())
	}
}
