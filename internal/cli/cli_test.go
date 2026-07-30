package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

func testApp(t *testing.T) (App, string) {
	t.Helper()
	tmp := t.TempDir()
	paths, err := zruntime.ResolvePaths(zruntime.Options{CWD: filepath.Join(tmp, "project"), HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	return App{
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
		Paths:  paths,
		Now:    func() time.Time { return time.Date(2026, 7, 29, 0, 0, 0, 0, time.UTC) },
	}, tmp
}

func stdout(app App) string {
	return app.Stdout.(*bytes.Buffer).String()
}

func TestRunSetup(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	out := stdout(app)
	if !strings.Contains(out, "zbrain setup complete") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunWorkspaceCreateCurrent(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"workspace", "current"}); err != nil {
		t.Fatalf("Run(workspace current) error = %v", err)
	}
	out := stdout(app)
	if !strings.Contains(out, `"workspace": "research"`) {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRunAskSearchesActiveWorkspace(t *testing.T) {
	app, _ := testApp(t)
	if err := app.Run([]string{"setup"}); err != nil {
		t.Fatalf("Run(setup) error = %v", err)
	}
	if err := app.Run([]string{"workspace", "create", "research"}); err != nil {
		t.Fatalf("Run(workspace create) error = %v", err)
	}
	notePath := filepath.Join(app.Paths.WorkspacesDir, "research", "wiki", "projects", "roadmap.md")
	if err := os.WriteFile(notePath, []byte("---\ntitle: Go Roadmap\n---\n\n# Retrieval\n\nMarkdown search should stay local first."), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app.Stdout = &bytes.Buffer{}
	if err := app.Run([]string{"ask", "local", "first"}); err != nil {
		t.Fatalf("Run(ask) error = %v", err)
	}
	out := stdout(app)
	if !strings.Contains(out, `"title": "Go Roadmap"`) {
		t.Fatalf("unexpected output: %s", out)
	}
	if !strings.Contains(out, `"path": "projects/roadmap.md"`) {
		t.Fatalf("unexpected output: %s", out)
	}
}
