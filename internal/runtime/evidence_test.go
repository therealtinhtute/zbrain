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
