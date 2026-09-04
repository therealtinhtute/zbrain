package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestEvidenceSnapshotStoresImmutableLocalCopy(t *testing.T) {
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("original evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}

	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	evidence, err := store.AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	if !evidenceIDPattern.MatchString(evidence.ID) {
		t.Fatalf("evidence ID = %q", evidence.ID)
	}
	if evidence.SHA256 != sha256String("original evidence") {
		t.Fatalf("SHA256 = %q", evidence.SHA256)
	}

	if err := os.WriteFile(source, []byte("mutated source"), 0o644); err != nil {
		t.Fatalf("WriteFile(mutated source) error = %v", err)
	}
	loaded, err := store.Read("research", evidence.ID)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if loaded.SHA256 != evidence.SHA256 || loaded.ByteLength != evidence.ByteLength {
		t.Fatalf("loaded metadata changed: %#v", loaded)
	}
	raw, err := os.ReadFile(filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw"))
	if err != nil {
		t.Fatalf("ReadFile(raw) error = %v", err)
	}
	if string(raw) != "original evidence" {
		t.Fatalf("raw snapshot = %q", raw)
	}
}

func TestEvidenceHashVerificationDetectsTamper(t *testing.T) {
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("trusted evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	evidence, err := store.AddFile("research", source, source, "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	raw := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(raw, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(raw, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("WriteFile(raw tamper) error = %v", err)
	}
	if err := store.Verify("research", evidence.ID); err == nil {
		t.Fatalf("Verify() error = nil, want tamper error")
	}
}

func TestEvidenceWorkspaceIsolation(t *testing.T) {
	paths := evidenceTestPaths(t)
	if err := CreateWorkspace(paths, "personal", fixedEvidenceNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("workspace scoped"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	evidence, err := store.AddFile("research", source, source, "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	if _, err := store.Read("personal", evidence.ID); err == nil {
		t.Fatalf("Read(cross workspace) error = nil")
	}
}

func evidenceTestPaths(t *testing.T) Paths {
	t.Helper()
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", fixedEvidenceNow()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return paths
}

func fixedEvidenceNow() time.Time {
	return time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
}

func TestEvidenceWorkspaceBoundaryRejectsSymlinkEscape(t *testing.T) {
	paths := evidenceTestPaths(t)
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	id := "evd_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "source.yaml")
	if err := os.WriteFile(outsidePath, []byte("id: "+id+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}
	root := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", id)
	if err := os.Symlink(outside, root); err != nil {
		t.Fatalf("Symlink(escape) error = %v", err)
	}
	if _, err := store.Read("research", id); err == nil {
		t.Fatalf("Read(symlink escape) error = nil")
	}
}

func TestEvidenceDirtyBarrierLeavesCanonicalTreeUnchanged(t *testing.T) {
	paths := evidenceTestPaths(t)
	dirtyPaths := paths
	dirtyPaths.IndexesDir = filepath.Join(t.TempDir(), "indexes-blocker")
	if err := os.WriteFile(dirtyPaths.IndexesDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile(indexes blocker) error = %v", err)
	}
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	sourcesDir := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources")
	before, err := os.ReadDir(sourcesDir)
	if err != nil {
		t.Fatalf("ReadDir(before) error = %v", err)
	}
	if _, err := (EvidenceStore{Paths: dirtyPaths, Now: fixedEvidenceNow}).AddFile("research", source, "file://source.txt", "text/plain"); err == nil {
		t.Fatalf("AddFile(dirty failure) error = nil")
	}
	after, err := os.ReadDir(sourcesDir)
	if err != nil {
		t.Fatalf("ReadDir(after) error = %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("evidence tree changed after dirty failure: before=%v after=%v", before, after)
	}
	if _, err := os.Stat(filepath.Join(dirtyPaths.IndexesDir, "research.dirty")); err == nil {
		t.Fatalf("dirty marker exists after dirty failure")
	}
}

func TestEvidenceAddRejectsUnsafeAndMissingWorkspaceWithoutMutation(t *testing.T) {
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	for _, workspace := range []string{"../outside", "missing"} {
		if _, err := store.AddFile(workspace, source, "file://source.txt", "text/plain"); err == nil {
			t.Fatalf("AddFile(%q) error = nil", workspace)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.WorkspacesDir, "outside")); !os.IsNotExist(err) {
		t.Fatalf("outside workspace error = %v, want absent", err)
	}
}

func TestEvidenceAddFileSkipsDuplicateSHA256(t *testing.T) {
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("original evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	first, err := store.AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile(first) error = %v", err)
	}
	if first.Deduped {
		t.Fatalf("first AddFile Deduped = true")
	}
	metadata, err := os.ReadFile(filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", first.ID, "source.yaml"))
	if err != nil {
		t.Fatalf("ReadFile(source.yaml) error = %v", err)
	}
	if strings.Contains(string(metadata), "deduped") {
		t.Fatalf("source.yaml persisted deduped:\n%s", metadata)
	}
	before, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(before skip) error = %v", err)
	}


	second, err := store.AddFile("research", source, "file://other-origin", "application/json")
	if err != nil {
		t.Fatalf("AddFile(duplicate) error = %v", err)
	}
	if second.ID != first.ID || !second.Deduped {
		t.Fatalf("duplicate AddFile = %#v, want id %q Deduped true", second, first.ID)
	}
	if second.Origin != first.Origin || second.MediaType != first.MediaType {
		t.Fatalf("skip merged metadata: %#v", second)
	}
	if ids := evidenceSourceIDs(t, paths, "research"); len(ids) != 1 || ids[0] != first.ID {
		t.Fatalf("evidence dirs = %v, want [%s]", ids, first.ID)
	}
	raw, err := os.ReadFile(filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", first.ID, "raw"))
	if err != nil {
		t.Fatalf("ReadFile(raw) error = %v", err)
	}
	if string(raw) != "original evidence" {
		t.Fatalf("raw snapshot = %q", raw)
	}
	after, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(after skip) error = %v", err)
	}
	if after.Current != before.Current {
		t.Fatalf("generation current bumped on skip: before=%#v after=%#v", before, after)
	}


	other := filepath.Join(t.TempDir(), "other.txt")
	if err := os.WriteFile(other, []byte("other evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(other) error = %v", err)
	}
	third, err := store.AddFile("research", other, "file://other.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile(distinct) error = %v", err)
	}
	if third.ID == first.ID || third.Deduped {
		t.Fatalf("distinct AddFile = %#v, want new id Deduped false", third)
	}
	if ids := evidenceSourceIDs(t, paths, "research"); len(ids) != 2 {
		t.Fatalf("evidence dirs = %v, want 2", ids)
	}
}

func TestEvidenceAddFileSkipDoesNotDirtyGeneration(t *testing.T) {
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("duplicate evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	first, err := store.AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile(first) error = %v", err)
	}
	if first.Deduped {
		t.Fatalf("first AddFile Deduped = true")
	}
	dirtyPath := filepath.Join(paths.IndexesDir, "research.dirty")
	if _, err := os.Stat(dirtyPath); err != nil {
		t.Fatalf("Stat(dirty after first add) error = %v, want dirty marker", err)
	}
	idx := IndexStore{Paths: paths}
	if _, err := idx.Rebuild("research"); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() after reindex error = %v", err)
	}
	before, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(before) error = %v", err)
	}
	if _, err := os.Stat(dirtyPath); err == nil {
		t.Fatalf("dirty marker exists after reindex")
	}

	second, err := store.AddFile("research", source, "file://source.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile(duplicate) error = %v", err)
	}
	if second.ID != first.ID || !second.Deduped {
		t.Fatalf("duplicate AddFile = %#v, want id %q Deduped true", second, first.ID)
	}
	after, err := readWorkspaceGeneration(paths, "research")
	if err != nil {
		t.Fatalf("readWorkspaceGeneration(after) error = %v", err)
	}
	if after.Current != before.Current {
		t.Fatalf("generation current changed on skip: before=%#v after=%#v", before, after)
	}
	if err := idx.CheckFresh("research"); err != nil {
		t.Fatalf("CheckFresh() after skip error = %v", err)
	}
	if _, err := os.Stat(dirtyPath); err == nil {
		t.Fatalf("dirty marker created by skip")
	}
	if ids := evidenceSourceIDs(t, paths, "research"); len(ids) != 1 || ids[0] != first.ID {
		t.Fatalf("evidence dirs = %v, want [%s]", ids, first.ID)
	}
}

func evidenceSourceIDs(t *testing.T, paths Paths, workspace string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(paths.WorkspacesDir, workspace, "evidence", "sources"))
	if err != nil {
		t.Fatalf("ReadDir(evidence/sources) error = %v", err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && evidenceIDPattern.MatchString(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	return ids
}

func TestEvidenceCheckClassifiesOriginDrift(t *testing.T) {
	paths := evidenceTestPaths(t)
	originDir := t.TempDir()
	writeOrigin := func(name string, contents string) string {
		t.Helper()
		path := filepath.Join(originDir, name)
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
		return path
	}
	unchangedPath := writeOrigin("unchanged.txt", "unchanged origin")
	changedPath := writeOrigin("changed.txt", "changed origin")
	missingPath := writeOrigin("missing.txt", "missing origin")
	remotePath := writeOrigin("remote.txt", "remote origin")

	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	unchanged, err := store.AddFile("research", unchangedPath, unchangedPath, "text/plain")
	if err != nil {
		t.Fatalf("AddFile(unchanged) error = %v", err)
	}
	scheme, err := store.AddFile("research", unchangedPath, "file://"+unchangedPath, "text/plain")
	if err != nil {
		t.Fatalf("AddFile(scheme) error = %v", err)
	}
	changed, err := store.AddFile("research", changedPath, changedPath, "text/plain")
	if err != nil {
		t.Fatalf("AddFile(changed) error = %v", err)
	}
	missing, err := store.AddFile("research", missingPath, missingPath, "text/plain")
	if err != nil {
		t.Fatalf("AddFile(missing) error = %v", err)
	}
	remote, err := store.AddFile("research", remotePath, "https://example.com/remote.txt", "text/plain")
	if err != nil {
		t.Fatalf("AddFile(remote) error = %v", err)
	}

	claimID, err := NewClaimID()
	if err != nil {
		t.Fatalf("NewClaimID() error = %v", err)
	}
	_, err = (ClaimStore{Paths: paths, Now: fixedEvidenceNow}).WriteDraft("research", Claim{
		Type:        OKFClaimType,
		ID:          claimID,
		Tier:        "projects",
		Status:      ClaimStatusDraft,
		Title:       "changed evidence backs this claim",
		Basis:       ClaimBasisEvidence,
		CreatedAt:   fixedEvidenceNow().UTC().Format(time.RFC3339),
		CreatedBy:   "owner",
		EvidenceIDs: []string{changed.ID, unchanged.ID},
		Body:        "The claim body.\n",
	})
	if err != nil {
		t.Fatalf("WriteDraft() error = %v", err)
	}

	if err := os.WriteFile(changedPath, []byte("mutated origin"), 0o644); err != nil {
		t.Fatalf("WriteFile(mutated origin) error = %v", err)
	}
	if err := os.Remove(missingPath); err != nil {
		t.Fatalf("Remove(missing origin) error = %v", err)
	}

	before := evidenceSnapshotStates(t, paths, []string{unchanged.ID, scheme.ID, changed.ID, missing.ID, remote.ID})
	report, err := store.CheckDrift("research")
	if err != nil {
		t.Fatalf("CheckDrift() error = %v", err)
	}
	after := evidenceSnapshotStates(t, paths, []string{unchanged.ID, scheme.ID, changed.ID, missing.ID, remote.ID})
	if len(before) != len(after) {
		t.Fatalf("snapshot state count changed: before=%d after=%d", len(before), len(after))
	}
	for key, state := range before {
		other, ok := after[key]
		if !ok {
			t.Fatalf("snapshot state %q missing after CheckDrift", key)
		}
		if state != other {
			t.Fatalf("snapshot %q mutated by CheckDrift: before=%#v after=%#v", key, state, other)
		}
	}

	statuses := make(map[string]EvidenceDriftFinding, len(report.Findings))
	for _, finding := range report.Findings {
		statuses[finding.ID] = finding
	}
	if len(report.Findings) != 5 {
		t.Fatalf("findings = %d, want 5 (%v)", len(report.Findings), report.Findings)
	}
	if got := statuses[unchanged.ID].Status; got != EvidenceDriftUnchanged {
		t.Fatalf("unchanged status = %q", got)
	}
	if got := statuses[scheme.ID].Status; got != EvidenceDriftUnchanged {
		t.Fatalf("file scheme status = %q", got)
	}
	if got := statuses[remote.ID].Status; got != EvidenceDriftUncheckable {
		t.Fatalf("remote status = %q", got)
	}
	changedFinding := statuses[changed.ID]
	if changedFinding.Status != EvidenceDriftChanged {
		t.Fatalf("changed status = %q", changedFinding.Status)
	}
	if changedFinding.RecordedSHA256 != changed.SHA256 {
		t.Fatalf("changed recorded sha256 = %q", changedFinding.RecordedSHA256)
	}
	if changedFinding.RecomputedSHA256 == changed.SHA256 {
		t.Fatalf("changed recomputed sha256 matches recorded digest")
	}
	if got := changedFinding.RecoveryAction; !strings.Contains(got, "supersede") || !strings.Contains(got, "re-approve") {
		t.Fatalf("changed recovery action = %q", got)
	}
	if got := statuses[missing.ID].Status; got != EvidenceDriftMissing {
		t.Fatalf("missing status = %q", got)
	}
	if got := statuses[missing.ID].RecoveryAction; !strings.Contains(got, "supersede") {
		t.Fatalf("missing recovery action = %q", got)
	}
	changedClaims := changedFinding.AffectedClaimIDs
	if len(changedClaims) != 1 || changedClaims[0] != claimID {
		t.Fatalf("changed affected claims = %v, want [%s]", changedClaims, claimID)
	}
	unchangedClaims := statuses[unchanged.ID].AffectedClaimIDs
	if len(unchangedClaims) != 1 || unchangedClaims[0] != claimID {
		t.Fatalf("unchanged affected claims = %v, want [%s]", unchangedClaims, claimID)
	}
	if got := statuses[remote.ID].AffectedClaimIDs; len(got) != 0 {
		t.Fatalf("remote affected claims = %v, want empty", got)
	}
	if got := statuses[scheme.ID].AffectedClaimIDs; len(got) != 0 {
		t.Fatalf("scheme affected claims = %v, want empty", got)
	}
	if statuses[unchanged.ID].RecoveryAction != "no action required" {
		t.Fatalf("unchanged recovery action = %q", statuses[unchanged.ID].RecoveryAction)
	}
}

func evidenceSnapshotStates(t *testing.T, paths Paths, ids []string) map[string]string {
	t.Helper()
	states := make(map[string]string)
	for _, id := range ids {
		for _, name := range []string{"raw", "source.yaml"} {
			path := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", id, name)
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", path, err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("Stat(%s) error = %v", path, err)
			}
			sum := sha256.Sum256(contents)
			states[id+"/"+name] = hex.EncodeToString(sum[:]) + ":" + info.ModTime().String()
		}
	}
	return states
}

func sha256String(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}

func TestEvidenceVerifyMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Evidence)
		want   string
	}{
		{
			name: "id mismatch",
			mutate: func(evidence *Evidence) {
				evidence.ID = "evd_cccccccccccccccccccccccccccccccc"
			},
			want: "metadata id",
		},
		{
			name: "missing origin",
			mutate: func(evidence *Evidence) {
				evidence.Origin = ""
			},
			want: "metadata origin",
		},
		{
			name: "invalid capture time",
			mutate: func(evidence *Evidence) {
				evidence.CapturedAt = "not-a-time"
			},
			want: "captured_at",
		},
		{
			name: "invalid media type",
			mutate: func(evidence *Evidence) {
				evidence.MediaType = "not media"
			},
			want: "media_type",
		},
		{
			name: "negative byte length",
			mutate: func(evidence *Evidence) {
				evidence.ByteLength = -1
			},
			want: "byte_length",
		},
		{
			name: "invalid sha256",
			mutate: func(evidence *Evidence) {
				evidence.SHA256 = "not-a-sha256"
			},
			want: "sha256",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, store, evidence := evidenceVerificationFixture(t)
			pathID := evidence.ID
			tt.mutate(&evidence)
			rewriteEvidenceMetadata(t, store, "research", pathID, evidence)
			err := store.Verify("research", pathID)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Verify() error = %v, want %q", err, tt.want)
			}
		})
	}

	t.Run("malformed yaml", func(t *testing.T) {
		_, store, evidence := evidenceVerificationFixture(t)
		metadataPath := filepath.Join(store.Paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "source.yaml")
		if err := os.Chmod(metadataPath, 0o644); err != nil {
			t.Fatalf("Chmod(metadata) error = %v", err)
		}
		if err := os.WriteFile(metadataPath, []byte("id: [\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(metadata) error = %v", err)
		}
		if err := store.Verify("research", evidence.ID); err == nil || !strings.Contains(err.Error(), "parse metadata") {
			t.Fatalf("Verify() error = %v, want parse metadata", err)
		}
	})
}

func TestEvidenceVerifyMissing(t *testing.T) {
	t.Run("metadata", func(t *testing.T) {
		_, store, evidence := evidenceVerificationFixture(t)
		metadataPath := filepath.Join(store.Paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "source.yaml")
		if err := os.Rename(metadataPath, metadataPath+".missing"); err != nil {
			t.Fatalf("Rename(metadata) error = %v", err)
		}
		if err := store.Verify("research", evidence.ID); err == nil || !strings.Contains(err.Error(), "source.yaml") {
			t.Fatalf("Verify() error = %v, want missing metadata", err)
		}
	})

	t.Run("raw", func(t *testing.T) {
		_, store, evidence := evidenceVerificationFixture(t)
		rawPath := filepath.Join(store.Paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
		if err := os.Rename(rawPath, rawPath+".missing"); err != nil {
			t.Fatalf("Rename(raw) error = %v", err)
		}
		if err := store.Verify("research", evidence.ID); err == nil || !strings.Contains(err.Error(), "raw") {
			t.Fatalf("Verify() error = %v, want missing raw", err)
		}
	})
}

func TestEvidenceVerifySize(t *testing.T) {
	_, store, evidence := evidenceVerificationFixture(t)
	evidence.ByteLength++
	rewriteEvidenceMetadata(t, store, "research", evidence.ID, evidence)
	if err := store.Verify("research", evidence.ID); err == nil || !strings.Contains(err.Error(), "byte length") {
		t.Fatalf("Verify() error = %v, want byte length mismatch", err)
	}
}

func TestEvidenceVerifyHash(t *testing.T) {
	t.Run("raw bytes", func(t *testing.T) {
		_, store, evidence := evidenceVerificationFixture(t)
		rawPath := filepath.Join(store.Paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
		if err := os.Chmod(rawPath, 0o644); err != nil {
			t.Fatalf("Chmod(raw) error = %v", err)
		}
		if err := os.WriteFile(rawPath, []byte(strings.Repeat("x", int(evidence.ByteLength))), 0o644); err != nil {
			t.Fatalf("WriteFile(raw) error = %v", err)
		}
		if err := store.Verify("research", evidence.ID); err == nil || !strings.Contains(err.Error(), "sha256") {
			t.Fatalf("Verify() error = %v, want sha256 mismatch", err)
		}
	})

	t.Run("metadata hash", func(t *testing.T) {
		_, store, evidence := evidenceVerificationFixture(t)
		evidence.SHA256 = strings.Repeat("0", 64)
		rewriteEvidenceMetadata(t, store, "research", evidence.ID, evidence)
		if err := store.Verify("research", evidence.ID); err == nil || !strings.Contains(err.Error(), "sha256") {
			t.Fatalf("Verify() error = %v, want sha256 mismatch", err)
		}
	})
}

func TestEvidenceVerifyWorkspace(t *testing.T) {
	paths := evidenceTestPaths(t)
	if err := CreateWorkspace(paths, "personal", fixedEvidenceNow()); err != nil {
		t.Fatalf("CreateWorkspace(personal) error = %v", err)
	}
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("workspace scoped"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	evidence, err := store.AddFile("research", source, source, "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	validator, err := NewEvidenceValidator(store, "personal")
	if err != nil {
		t.Fatalf("NewEvidenceValidator() error = %v", err)
	}
	if err := validator.Verify(evidence.ID); err == nil || !strings.Contains(err.Error(), "source.yaml") {
		t.Fatalf("Verify(cross workspace) error = %v, want missing metadata", err)
	}
	if _, err := NewEvidenceValidator(store, "../outside"); err == nil {
		t.Fatalf("NewEvidenceValidator(unsafe workspace) error = nil")
	}
}

func TestEvidenceVerifyCache(t *testing.T) {
	_, store, evidence := evidenceVerificationFixture(t)
	validator, err := NewEvidenceValidator(store, "research")
	if err != nil {
		t.Fatalf("NewEvidenceValidator() error = %v", err)
	}
	if err := validator.Verify(evidence.ID); err != nil {
		t.Fatalf("first Verify() error = %v", err)
	}
	rawPath := filepath.Join(store.Paths.WorkspacesDir, "research", "evidence", "sources", evidence.ID, "raw")
	if err := os.Chmod(rawPath, 0o644); err != nil {
		t.Fatalf("Chmod(raw) error = %v", err)
	}
	if err := os.WriteFile(rawPath, []byte(strings.Repeat("x", int(evidence.ByteLength))), 0o644); err != nil {
		t.Fatalf("WriteFile(raw) error = %v", err)
	}
	if err := validator.Verify(evidence.ID); err != nil {
		t.Fatalf("cached Verify() error = %v, want cached success", err)
	}
	if got := validator.verifyCount[evidence.ID]; got != 1 {
		t.Fatalf("verify count = %d, want 1", got)
	}

	freshValidator, err := NewEvidenceValidator(store, "research")
	if err != nil {
		t.Fatalf("NewEvidenceValidator(fresh) error = %v", err)
	}
	if err := freshValidator.Verify(evidence.ID); err == nil {
		t.Fatalf("fresh Verify() error = nil, want tamper error")
	}
}

func evidenceVerificationFixture(t *testing.T) (Paths, EvidenceStore, Evidence) {
	t.Helper()
	paths := evidenceTestPaths(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("trusted evidence"), 0o644); err != nil {
		t.Fatalf("WriteFile(source) error = %v", err)
	}
	store := EvidenceStore{Paths: paths, Now: fixedEvidenceNow}
	evidence, err := store.AddFile("research", source, source, "text/plain")
	if err != nil {
		t.Fatalf("AddFile() error = %v", err)
	}
	return paths, store, evidence
}

func rewriteEvidenceMetadata(t *testing.T, store EvidenceStore, workspace string, pathID string, evidence Evidence) {
	t.Helper()
	path := filepath.Join(store.Paths.WorkspacesDir, workspace, "evidence", "sources", pathID, "source.yaml")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("Chmod(metadata) error = %v", err)
	}
	contents, err := yaml.Marshal(evidence)
	if err != nil {
		t.Fatalf("yaml.Marshal(evidence) error = %v", err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(metadata) error = %v", err)
	}
}
