package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func sha256String(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
