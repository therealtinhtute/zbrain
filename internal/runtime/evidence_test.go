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

func sha256String(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])
}
