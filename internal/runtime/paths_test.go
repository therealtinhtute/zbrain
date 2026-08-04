package runtime

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsUsesExplicitRuntime(t *testing.T) {
	paths, err := ResolvePaths(Options{
		CWD:        "/tmp/project",
		HomeDir:    "/tmp/home",
		RuntimeDir: "/tmp/custom-zbrain",
	})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}

	if paths.RuntimeDir != filepath.Clean("/tmp/custom-zbrain") {
		t.Fatalf("RuntimeDir = %q", paths.RuntimeDir)
	}
	if paths.ConfigFile != filepath.Join(paths.RuntimeDir, "config.yml") {
		t.Fatalf("ConfigFile = %q", paths.ConfigFile)
	}
	if paths.WorkspacesDir != filepath.Join(paths.RuntimeDir, "workspaces") {
		t.Fatalf("WorkspacesDir = %q", paths.WorkspacesDir)
	}
}

func TestIsSafeWorkspaceName(t *testing.T) {
	valid := []string{"research", "team-1", "a1"}
	for _, name := range valid {
		if !IsSafeWorkspaceName(name) {
			t.Fatalf("expected %q to be safe", name)
		}
	}

	invalid := []string{"", " Research", "research ", "Research", "../x", "a_b", "a/b", "x y"}
	for _, name := range invalid {
		if IsSafeWorkspaceName(name) {
			t.Fatalf("expected %q to be unsafe", name)
		}
	}
}
