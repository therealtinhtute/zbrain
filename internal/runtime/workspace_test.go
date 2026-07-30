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
