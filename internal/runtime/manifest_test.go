package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestManifestEvidenceInputs(t *testing.T) {
	paths := manifestTestPaths(t)
	root := filepath.Join(paths.WorkspacesDir, "research")

	files := []struct {
		relative string
		contents []byte
		kind     string
	}{
		{relative: "wiki/axioms/axiom.md", contents: []byte("axiom\n"), kind: TrustInputKindClaim},
		{relative: "wiki/mental-models/model.md", contents: []byte("model\n"), kind: TrustInputKindClaim},
		{relative: "wiki/projects/nested/project.md", contents: []byte("project\n"), kind: TrustInputKindClaim},
		{relative: "wiki/decisions/decision.md", contents: []byte("decision\n"), kind: TrustInputKindClaim},
		{relative: "evidence/sources/evd_test/source.yaml", contents: []byte("id: evd_test\n"), kind: TrustInputKindEvidenceMetadata},
		{relative: "evidence/sources/evd_test/raw", contents: []byte{0x00, 0x01, 0x02}, kind: TrustInputKindEvidenceRaw},
	}
	for _, file := range files {
		writeManifestFile(t, root, file.relative, file.contents)
	}
	for _, relative := range []string{
		"wiki/projects/derived.json",
		"wiki/projects/readme.txt",
		"workspace.md",
		"evidence/_index.md",
		"evidence/analysis/derived.md",
		"evidence/qa/report.md",
		"evidence/applied/applied.md",
		"evidence/archive/archived.md",
		"evidence/sources/evd_test/notes.md",
	} {
		writeManifestFile(t, root, relative, []byte("excluded\n"))
	}

	manifest, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest() error = %v", err)
	}
	if manifest.Digest == "" {
		t.Fatal("BuildTrustInputManifest() returned empty digest")
	}
	if len(manifest.Entries) != len(files) {
		t.Fatalf("manifest entries = %d, want %d: %#v", len(manifest.Entries), len(files), manifest.Entries)
	}

	want := make([]TrustInput, 0, len(files))
	for _, file := range files {
		want = append(want, manifestTestInput(t, root, file.relative, file.contents, file.kind))
	}
	sort.Slice(want, func(i, j int) bool {
		return want[i].Path < want[j].Path
	})
	if !reflect.DeepEqual(manifest.Entries, want) {
		t.Fatalf("manifest entries = %#v, want %#v", manifest.Entries, want)
	}

	repeated, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest(repeated) error = %v", err)
	}
	if !reflect.DeepEqual(manifest, repeated) {
		t.Fatalf("manifest is not deterministic: first=%#v repeated=%#v", manifest, repeated)
	}
}

func TestManifestDetectsChangeAndAddRemove(t *testing.T) {
	paths := manifestTestPaths(t)
	root := filepath.Join(paths.WorkspacesDir, "research")
	claimPath := filepath.Join(root, "wiki", "projects", "claim.md")
	original := []byte("one\n")
	replacement := []byte("two\n")
	if len(original) != len(replacement) {
		t.Fatalf("test setup replacement lengths differ")
	}
	if err := os.WriteFile(claimPath, original, 0o644); err != nil {
		t.Fatalf("WriteFile(original) error = %v", err)
	}

	before, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest(before) error = %v", err)
	}
	if err := os.WriteFile(claimPath, replacement, 0o644); err != nil {
		t.Fatalf("WriteFile(replacement) error = %v", err)
	}
	afterReplacement, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest(after replacement) error = %v", err)
	}
	beforeEntry := manifestEntry(t, before, "wiki/projects/claim.md")
	replacementEntry := manifestEntry(t, afterReplacement, "wiki/projects/claim.md")
	if beforeEntry.ByteLength != replacementEntry.ByteLength {
		t.Fatalf("replacement byte lengths = %d and %d, want equal", beforeEntry.ByteLength, replacementEntry.ByteLength)
	}
	if beforeEntry.SHA256 == replacementEntry.SHA256 {
		t.Fatal("same-size replacement did not change file digest")
	}
	if before.Digest == afterReplacement.Digest {
		t.Fatal("same-size replacement did not change manifest digest")
	}

	addedPath := filepath.Join(root, "wiki", "projects", "added.md")
	if err := os.WriteFile(addedPath, []byte("added\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(added) error = %v", err)
	}
	afterAdd, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest(after add) error = %v", err)
	}
	if len(afterAdd.Entries) != len(afterReplacement.Entries)+1 {
		t.Fatalf("entries after add = %d, want %d", len(afterAdd.Entries), len(afterReplacement.Entries)+1)
	}
	if afterAdd.Digest == afterReplacement.Digest {
		t.Fatal("add did not change manifest digest")
	}
	if err := os.Remove(addedPath); err != nil {
		t.Fatalf("Remove(added) error = %v", err)
	}
	afterRemove, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest(after remove) error = %v", err)
	}
	if !reflect.DeepEqual(afterRemove, afterReplacement) {
		t.Fatalf("remove did not restore manifest: afterRemove=%#v afterReplacement=%#v", afterRemove, afterReplacement)
	}
}

func TestTrustInputManifestRejectsUnsafeMissingAndCoveredBoundaryInputs(t *testing.T) {
	paths := manifestTestPaths(t)
	for _, workspace := range []string{"../outside", "missing"} {
		if _, err := BuildTrustInputManifest(paths, workspace); err == nil {
			t.Fatalf("BuildTrustInputManifest(%q) error = nil", workspace)
		}
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(paths.WorkspacesDir, "linked")); err != nil {
		t.Fatalf("Symlink(workspace) error = %v", err)
	}
	if _, err := BuildTrustInputManifest(paths, "linked"); err == nil {
		t.Fatal("BuildTrustInputManifest(symlink workspace) error = nil")
	}

	t.Run("symlinked-claim", func(t *testing.T) {
		paths := manifestTestPaths(t)
		root := filepath.Join(paths.WorkspacesDir, "research")
		outsideClaim := filepath.Join(t.TempDir(), "outside.md")
		if err := os.WriteFile(outsideClaim, []byte("outside\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(outside claim) error = %v", err)
		}
		claimPath := filepath.Join(root, "wiki", "projects", "linked.md")
		if err := os.Symlink(outsideClaim, claimPath); err != nil {
			t.Fatalf("Symlink(claim) error = %v", err)
		}
		if _, err := BuildTrustInputManifest(paths, "research"); err == nil {
			t.Fatal("BuildTrustInputManifest(symlink claim) error = nil")
		}
	})

	t.Run("non-regular-claim", func(t *testing.T) {
		paths := manifestTestPaths(t)
		claimPath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "directory.md")
		if err := os.Mkdir(claimPath, 0o755); err != nil {
			t.Fatalf("Mkdir(claim) error = %v", err)
		}
		if _, err := BuildTrustInputManifest(paths, "research"); err == nil {
			t.Fatal("BuildTrustInputManifest(directory claim) error = nil")
		}
	})

	t.Run("symlinked-evidence-raw", func(t *testing.T) {
		paths := manifestTestPaths(t)
		root := filepath.Join(paths.WorkspacesDir, "research")
		sourceRoot := filepath.Join(root, "evidence", "sources", "evd_test")
		if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
			t.Fatalf("MkdirAll(source) error = %v", err)
		}
		outsideRaw := filepath.Join(t.TempDir(), "outside.raw")
		if err := os.WriteFile(outsideRaw, []byte("outside\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(outside raw) error = %v", err)
		}
		if err := os.Symlink(outsideRaw, filepath.Join(sourceRoot, "raw")); err != nil {
			t.Fatalf("Symlink(raw) error = %v", err)
		}
		if _, err := BuildTrustInputManifest(paths, "research"); err == nil {
			t.Fatal("BuildTrustInputManifest(symlink raw) error = nil")
		}
	})

	t.Run("non-regular-evidence-metadata", func(t *testing.T) {
		paths := manifestTestPaths(t)
		sourceRoot := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", "evd_test")
		if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
			t.Fatalf("MkdirAll(source) error = %v", err)
		}
		if err := os.Mkdir(filepath.Join(sourceRoot, "source.yaml"), 0o755); err != nil {
			t.Fatalf("Mkdir(source.yaml) error = %v", err)
		}
		if _, err := BuildTrustInputManifest(paths, "research"); err == nil {
			t.Fatal("BuildTrustInputManifest(directory metadata) error = nil")
		}
	})
}

func TestManifestDeterministic(t *testing.T) {
	paths := manifestTestPaths(t)
	if err := CreateWorkspace(paths, "other", time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CreateWorkspace(other) error = %v", err)
	}
	files := []struct {
		relative string
		contents []byte
	}{
		{relative: "wiki/projects/z.md", contents: []byte("z\n")},
		{relative: "wiki/axioms/a.md", contents: []byte("a\n")},
		{relative: "evidence/sources/evd_test/source.yaml", contents: []byte("id: evd_test\n")},
		{relative: "evidence/sources/evd_test/raw", contents: []byte{0x01, 0x02}},
	}
	firstRoot := filepath.Join(paths.WorkspacesDir, "research")
	secondRoot := filepath.Join(paths.WorkspacesDir, "other")
	for _, file := range files {
		writeManifestFile(t, firstRoot, file.relative, file.contents)
	}
	for i := len(files) - 1; i >= 0; i-- {
		writeManifestFile(t, secondRoot, files[i].relative, files[i].contents)
	}

	first, err := BuildTrustInputManifest(paths, "research")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest(research) error = %v", err)
	}
	second, err := BuildTrustInputManifest(paths, "other")
	if err != nil {
		t.Fatalf("BuildTrustInputManifest(other) error = %v", err)
	}
	if !reflect.DeepEqual(first.Entries, second.Entries) || first.Digest != second.Digest {
		t.Fatalf("manifest depends on creation order: first=%#v second=%#v", first, second)
	}
}

func manifestTestPaths(t *testing.T) Paths {
	t.Helper()
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{
		CWD:        filepath.Join(tmp, "project"),
		HomeDir:    tmp,
		RuntimeDir: filepath.Join(tmp, ".zbrain"),
	})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	return paths
}

func writeManifestFile(t *testing.T, root string, relative string, contents []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", relative, err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", relative, err)
	}
}

func manifestTestInput(t *testing.T, root string, relative string, contents []byte, kind string) TrustInput {
	t.Helper()
	sum := sha256.Sum256(contents)
	return TrustInput{
		Path:       filepath.ToSlash(relative),
		Kind:       kind,
		ByteLength: int64(len(contents)),
		SHA256:     hex.EncodeToString(sum[:]),
	}
}

func manifestEntry(t *testing.T, manifest TrustInputManifest, path string) TrustInput {
	t.Helper()
	for _, entry := range manifest.Entries {
		if entry.Path == path {
			return entry
		}
	}
	t.Fatalf("manifest entry %q not found: %#v", path, manifest.Entries)
	return TrustInput{}
}
