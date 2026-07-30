package runtime

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSearchWorkspaceParsesMarkdownFields(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Now()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	notePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "mental-models", "retrieval.md")
	contents := "---\ntitle: Markdown Retrieval\ntags: [go, search]\n---\n\n# Query Pipeline\n\nUse [SQLite FTS5](https://sqlite.org) for local-first memory search.\n\n```go\n// noisy code should not become body text\nsecretSearchIdentifier\n```\n"
	if err := os.WriteFile(notePath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	results, err := SearchWorkspace(paths, "research", "local memory", 10)
	if err != nil {
		t.Fatalf("SearchWorkspace() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Title != "Markdown Retrieval" {
		t.Fatalf("Title = %q", results[0].Title)
	}
	if results[0].Tier != "mental-models" {
		t.Fatalf("Tier = %q", results[0].Tier)
	}
	if results[0].Path != "mental-models/retrieval.md" {
		t.Fatalf("Path = %q", results[0].Path)
	}
	if results[0].Snippet == "" {
		t.Fatalf("Snippet missing")
	}
}

func TestSearchWorkspaceSupportsUnicodeQueryTokens(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Now()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	notePath := filepath.Join(paths.WorkspacesDir, "research", "wiki", "projects", "tieng-viet.md")
	if err := os.WriteFile(notePath, []byte("# Ghi nhớ tiếng Việt\n\nAgent cần tìm kiếm tri thức nội bộ."), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	results, err := SearchWorkspace(paths, "research", "tìm kiếm", 10)
	if err != nil {
		t.Fatalf("SearchWorkspace() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
}

func TestSearchWorkspaceSearchesOnlyWiki(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if err := CreateWorkspace(paths, "research", time.Now()); err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}

	evidencePath := filepath.Join(paths.WorkspacesDir, "research", "evidence", "sources", "raw.md")
	if err := os.WriteFile(evidencePath, []byte("poison-token should never be retrieved"), 0o644); err != nil {
		t.Fatalf("WriteFile(evidence) error = %v", err)
	}

	results, err := SearchWorkspace(paths, "research", "poison-token", 10)
	if err != nil {
		t.Fatalf("SearchWorkspace() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}
