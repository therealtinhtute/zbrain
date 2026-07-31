package runtime

import (
	"crypto/sha256"
	"encoding/hex"
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

	revoked, err := store.Revoke("research", approvedReplacement.ID, "wrong scope")
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if revoked.Status != ClaimStatusRevoked {
		t.Fatalf("revoked status = %q", revoked.Status)
	}
	if !strings.Contains(revoked.Body, "Revoked: wrong scope") {
		t.Fatalf("revocation reason missing from body: %q", revoked.Body)
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
	if len(approved.Sources) != 1 || approved.Sources[0].ID != evidence.ID || approved.Sources[0].Digest != "sha256:"+evidence.SHA256 {
		t.Fatalf("Sources = %#v, evidence = %#v", approved.Sources, evidence)
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
	if summary.Migrated != 1 || summary.Invalid != 0 {
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

func sha256Hex(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
