package runtime

import (
	"os"
	"path/filepath"
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

func TestReindexIndexesApprovedAndDraftButNotLegacyOrEvidence(t *testing.T) {
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
	if summary.Approved != 1 || summary.Draft != 1 || summary.Legacy != 2 || summary.Invalid != 0 {
		t.Fatalf("summary = %#v", summary)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() error = %v", err)
	}

	approvedResults, err := idx.Search("research", SearchOptions{Query: "description recall", Statuses: []ClaimStatus{ClaimStatusApproved}, Limit: 10})
	if err != nil {
		t.Fatalf("Search(approved) error = %v", err)
	}
	if len(approvedResults) != 1 || approvedResults[0].ID != approved.ID || approvedResults[0].Type != OKFClaimType || approvedResults[0].Description != approved.Description {
		t.Fatalf("approved results = %#v", approvedResults)
	}
	draftResults, err := idx.Search("research", SearchOptions{Query: "draft-only", Statuses: []ClaimStatus{ClaimStatusDraft}, Limit: 10})
	if err != nil {
		t.Fatalf("Search(draft) error = %v", err)
	}
	if len(draftResults) != 1 || draftResults[0].ID != draft.ID {
		t.Fatalf("draft results = %#v", draftResults)
	}
	poisonResults, err := idx.Search("research", SearchOptions{Query: "poison", Statuses: []ClaimStatus{ClaimStatusApproved, ClaimStatusDraft}, Limit: 10})
	if err != nil {
		t.Fatalf("Search(poison) error = %v", err)
	}
	if len(poisonResults) != 0 {
		t.Fatalf("poison results = %#v", poisonResults)
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
	if err := os.Remove(idx.DatabasePath("research")); err != nil {
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
