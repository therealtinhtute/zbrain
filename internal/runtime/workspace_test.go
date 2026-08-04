package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateWorkspaceCreatesLayoutAndDefault(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}

	if err := CreateWorkspace(paths, "research", time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	for _, tier := range WikiTiers {
		if _, err := os.Stat(filepath.Join(paths.WorkspacesDir, "research", "wiki", tier)); err != nil {
			t.Fatalf("missing tier %s: %v", tier, err)
		}
	}

	config, err := ReadConfig(paths.ConfigFile)
	if err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}
	if config.DefaultWorkspace != "research" {
		t.Fatalf("DefaultWorkspace = %q", config.DefaultWorkspace)
	}
}

func TestCreateWorkspaceRejectsExistingWorkspace(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if _, err := EnsureConfig(paths.ConfigFile); err != nil {
		t.Fatalf("EnsureConfig() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	workspaceReadme := filepath.Join(paths.WorkspacesDir, "research", "workspace.md")
	custom := []byte("# Custom workspace\n")
	if err := os.WriteFile(workspaceReadme, custom, 0o644); err != nil {
		t.Fatalf("WriteFile(workspace.md) error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatalf("CreateWorkspace(existing) error = nil")
	}
	contents, err := os.ReadFile(workspaceReadme)
	if err != nil {
		t.Fatalf("ReadFile(workspace.md) error = %v", err)
	}
	if string(contents) != string(custom) {
		t.Fatalf("workspace.md was overwritten: %q", contents)
	}
}

func TestResolveCurrentWorkspaceRejectsUnsafeMissingAndSymlinkWorkspace(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: filepath.Join(tmp, "project"), HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Now()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	for _, workspace := range []string{"../outside", "missing"} {
		if err := WriteConfig(paths.ConfigFile, Config{DefaultWorkspace: workspace}); err != nil {
			t.Fatalf("WriteConfig(%q) error = %v", workspace, err)
		}
		if _, err := ResolveCurrentWorkspace(paths); err == nil {
			t.Fatalf("ResolveCurrentWorkspace(%q) error = nil", workspace)
		}
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(paths.WorkspacesDir, "linked")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	if err := WriteConfig(paths.ConfigFile, Config{DefaultWorkspace: "linked"}); err != nil {
		t.Fatalf("WriteConfig(linked) error = %v", err)
	}
	if _, err := ResolveCurrentWorkspace(paths); err == nil {
		t.Fatalf("ResolveCurrentWorkspace(symlink) error = nil")
	}
}

func TestResolveCurrentWorkspaceReturnsJSONShape(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: filepath.Join(tmp, "project"), HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Now()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	current, err := ResolveCurrentWorkspace(paths)
	if err != nil {
		t.Fatalf("ResolveCurrentWorkspace() error = %v", err)
	}
	encoded, err := MarshalCurrent(current)
	if err != nil {
		t.Fatalf("MarshalCurrent() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("current JSON is invalid: %v", err)
	}
	if decoded["workspace"] != "research" {
		t.Fatalf("workspace = %v", decoded["workspace"])
	}
	if decoded["project_root"] != paths.CWD {
		t.Fatalf("project_root = %v", decoded["project_root"])
	}
	if decoded["context_file"] == "" {
		t.Fatalf("context_file missing")
	}
}
