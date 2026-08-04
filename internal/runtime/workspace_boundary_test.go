package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestValidateWorkspaceReturnsExistingCanonicalRoot(t *testing.T) {
	paths := newBoundaryTestPaths(t)
	createBoundaryWorkspace(t, paths, "research")

	root, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(paths.WorkspacesDir, "research"))
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if root != want {
		t.Fatalf("ValidateWorkspace() = %q, want %q", root, want)
	}
}

func TestValidateWorkspaceRejectsUnsafeMissingAndSymlinkRoots(t *testing.T) {
	paths := newBoundaryTestPaths(t)
	createBoundaryWorkspace(t, paths, "research")

	before := snapshotTree(t, paths.RuntimeDir)
	for _, name := range []string{"", "Research", "../outside", "missing"} {
		if _, err := ValidateWorkspace(paths, name); err == nil {
			t.Fatalf("ValidateWorkspace(%q) error = nil", name)
		}
	}
	if got := snapshotTree(t, paths.RuntimeDir); !reflect.DeepEqual(got, before) {
		t.Fatalf("validation changed runtime tree:\nbefore=%v\nafter=%v", before, got)
	}

	outside := t.TempDir()
	link := filepath.Join(paths.WorkspacesDir, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := ValidateWorkspace(paths, "linked"); err == nil {
		t.Fatalf("ValidateWorkspace(symlink root) error = nil")
	}
}

func TestResolveWorkspacePathRejectsTraversalAndSymlinkEscape(t *testing.T) {
	paths := newBoundaryTestPaths(t)
	createBoundaryWorkspace(t, paths, "research")

	for _, relative := range []string{
		"../outside",
		"wiki/../../outside",
		"/tmp/outside",
		"./wiki/projects/claim.md",
		"wiki/projects/../axioms/claim.md",
	} {
		if _, err := ResolveWorkspacePath(paths, "research", relative); err == nil {
			t.Fatalf("ResolveWorkspacePath(%q) error = nil", relative)
		}
	}

	outside := t.TempDir()
	escape := filepath.Join(paths.WorkspacesDir, "research", "wiki", "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if _, err := ResolveWorkspacePath(paths, "research", "wiki/escape/new.md"); err == nil {
		t.Fatalf("ResolveWorkspacePath(symlink escape) error = nil")
	}

	outsideFile := filepath.Join(outside, "existing.md")
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	existingEscape := filepath.Join(paths.WorkspacesDir, "research", "wiki", "existing.md")
	if err := os.Symlink(outsideFile, existingEscape); err != nil {
		t.Fatalf("Symlink(existing) error = %v", err)
	}
	if _, err := ResolveWorkspacePath(paths, "research", "wiki/existing.md"); err == nil {
		t.Fatalf("ResolveWorkspacePath(existing symlink escape) error = nil")
	}
}

func TestResolveWorkspacePathAllowsSafeNewPathAndInRootSymlink(t *testing.T) {
	paths := newBoundaryTestPaths(t)
	createBoundaryWorkspace(t, paths, "research")

	newPath, err := ResolveWorkspacePath(paths, "research", "wiki/projects/new.md")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath(new path) error = %v", err)
	}
	root, err := ValidateWorkspace(paths, "research")
	if err != nil {
		t.Fatalf("ValidateWorkspace() error = %v", err)
	}
	want := filepath.Join(root, "wiki", "projects", "new.md")
	if newPath != want {
		t.Fatalf("ResolveWorkspacePath() = %q, want %q", newPath, want)
	}

	link := filepath.Join(root, "wiki", "project-link")
	if err := os.Symlink(filepath.Join(root, "wiki", "projects"), link); err != nil {
		t.Fatalf("Symlink(in-root) error = %v", err)
	}
	resolved, err := ResolveWorkspacePath(paths, "research", "wiki/project-link/new.md")
	if err != nil {
		t.Fatalf("ResolveWorkspacePath(in-root symlink) error = %v", err)
	}
	if resolved != filepath.Join(root, "wiki", "project-link", "new.md") {
		t.Fatalf("ResolveWorkspacePath(in-root symlink) = %q", resolved)
	}
}

func newBoundaryTestPaths(t *testing.T) Paths {
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
	if err := os.MkdirAll(paths.WorkspacesDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workspaces) error = %v", err)
	}
	return paths
}

func createBoundaryWorkspace(t *testing.T, paths Paths, name string) {
	t.Helper()
	if err := CreateWorkspace(paths, name, time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
}

type fileSnapshot struct {
	mode    os.FileMode
	size    int64
	modTime int64
}

func snapshotTree(t *testing.T, root string) map[string]fileSnapshot {
	t.Helper()
	snapshot := map[string]fileSnapshot{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		snapshot[rel] = fileSnapshot{
			mode:    info.Mode(),
			size:    info.Size(),
			modTime: info.ModTime().UnixNano(),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotTree() error = %v", err)
	}
	return snapshot
}
