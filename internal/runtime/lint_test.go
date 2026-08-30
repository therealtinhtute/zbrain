package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStructuralFindingsCleanWorkspace(t *testing.T) {
	paths := evidenceTestPaths(t)
	findings, err := StructuralFindings(paths, "research")
	if err != nil {
		t.Fatalf("StructuralFindings() error = %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v, want none", findings)
	}
}

func TestStructuralFindingsDanglingSupportAndConflicts(t *testing.T) {
	paths := evidenceTestPaths(t)
	missing := "clm_ffffffffffffffffffffffffffffffff"
	id, err := NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	claim := validOwnerClaim()
	claim.ID = id
	claim.Basis = ClaimBasisDerived
	claim.SupportingClaimIDs = []string{missing}
	claim.ConflictsWith = []string{missing}
	if _, err := (ClaimStore{Paths: paths, Now: fixedEvidenceNow}).WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	findings, err := StructuralFindings(paths, "research")
	if err != nil {
		t.Fatalf("StructuralFindings() error = %v", err)
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "supporting_claim_ids references missing "+missing) {
		t.Fatalf("missing support finding: %v", findings)
	}
	if !strings.Contains(joined, "conflicts_with references missing "+missing) {
		t.Fatalf("missing conflicts finding: %v", findings)
	}
}

func TestStructuralFindingsMissingAndOrphanEvidence(t *testing.T) {
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("orphan bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	evidence, err := (EvidenceStore{Paths: paths, Now: fixedEvidenceNow}).AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	missingEvidence := "evd_ffffffffffffffffffffffffffffffff"
	id, err := NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	claim := validOwnerClaim()
	claim.ID = id
	claim.Basis = ClaimBasisEvidence
	claim.EvidenceIDs = []string{missingEvidence}
	if _, err := (ClaimStore{Paths: paths, Now: fixedEvidenceNow}).WriteDraft("research", claim); err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	findings, err := StructuralFindings(paths, "research")
	if err != nil {
		t.Fatalf("StructuralFindings() error = %v", err)
	}
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, "evidence_ids references missing "+missingEvidence) {
		t.Fatalf("missing evidence finding: %v", findings)
	}
	if !strings.Contains(joined, "evidence "+evidence.ID+" is not cited by any claim") {
		t.Fatalf("orphan evidence finding: %v", findings)
	}
}

func TestStructuralFindingsDuplicateSHA256(t *testing.T) {
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("shared bytes"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	first, err := (EvidenceStore{Paths: paths, Now: fixedEvidenceNow}).AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	cloneID := "evd_cccccccccccccccccccccccccccccccc"
	srcDir := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", first.ID)
	dstDir := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", cloneID)
	if err := os.MkdirAll(dstDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(clone) error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(srcDir, "raw"))
	if err != nil {
		t.Fatalf("ReadFile(raw) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dstDir, "raw"), raw, 0o400); err != nil {
		t.Fatalf("WriteFile(clone raw) error = %v", err)
	}
	meta, err := os.ReadFile(filepath.Join(srcDir, "source.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(source.yaml) error = %v", err)
	}
	cloned := strings.Replace(string(meta), first.ID, cloneID, 1)
	if err := os.WriteFile(filepath.Join(dstDir, "source.yaml"), []byte(cloned), 0o400); err != nil {
		t.Fatalf("WriteFile(clone source.yaml) error = %v", err)
	}
	findings, err := StructuralFindings(paths, "research")
	if err != nil {
		t.Fatalf("StructuralFindings() error = %v", err)
	}
	want := "duplicate sha256 " + first.SHA256 + " on evidence "
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, want) || !strings.Contains(joined, first.ID) || !strings.Contains(joined, cloneID) {
		t.Fatalf("duplicate sha finding: %v", findings)
	}
}

func TestStructuralFindingsStaleAfter(t *testing.T) {
	paths := evidenceTestPaths(t)
	id, err := NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	claim := validOwnerClaim()
	claim.ID = id
	claim.StaleAfter = "2020-01-01T00:00:00Z"
	store := ClaimStore{Paths: paths, Now: fixedEvidenceNow}
	draft, err := store.WriteDraft("research", claim)
	if err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}
	if _, err := store.Approve("research", draft.ID); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	findings, err := structuralFindingsAt(paths, "research", time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("structuralFindingsAt() error = %v", err)
	}
	want := "claim " + draft.ID + " stale_after 2020-01-01T00:00:00Z is in the past"
	joined := strings.Join(findings, "\n")
	if !strings.Contains(joined, want) {
		t.Fatalf("stale finding: %v", findings)
	}
}
