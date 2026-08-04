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
