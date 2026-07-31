package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bundled "github.com/therealtinhtute/zbrain/assets"
)

func TestExtractBundledAssetsCopiesRuntimeAssets(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}

	result, err := ExtractBundledAssets(paths)
	if err != nil {
		t.Fatalf("ExtractBundledAssets() error = %v", err)
	}
	if len(result.Copied) == 0 {
		t.Fatalf("expected copied assets")
	}
	if _, err := os.Stat(filepath.Join(paths.RuntimeDir, "README.md")); err != nil {
		t.Fatalf("missing README asset: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RuntimeDir, "templates", "workspace.md")); err != nil {
		t.Fatalf("missing workspace template asset: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RuntimeDir, "workspaces", "research")); !os.IsNotExist(err) {
		t.Fatalf("workspace seed assets must not create active workspaces, stat error = %v", err)
	}
}

func TestBundledAssetsDoNotContainStaleRuntimeInstructions(t *testing.T) {
	forbidden := []string{
		"bun install",
		"bun run",
		"src/index.ts",
		"qmd search",
		"current-task.md",
		"context_file",
		"zbrain note",
		"zbrain mcp",
		"zbrain sync",
		"zbrain learn",
		"zbrain ingest",
	}

	entries, err := bundled.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected bundled assets")
	}

	err = fsWalkBundled(func(path string, contents string) {
		for _, token := range forbidden {
			if strings.Contains(contents, token) {
				t.Errorf("%s contains stale runtime instruction %q", path, token)
			}
		}
	})
	if err != nil {
		t.Fatalf("walk bundled assets: %v", err)
	}
}

func fsWalkBundled(visit func(path string, contents string)) error {
	return walkBundledDir(".", visit)
}

func walkBundledDir(dir string, visit func(path string, contents string)) error {
	entries, err := bundled.FS.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := entry.Name()
		if dir != "." {
			path = dir + "/" + entry.Name()
		}
		if entry.IsDir() {
			if err := walkBundledDir(path, visit); err != nil {
				return err
			}
			continue
		}
		if strings.HasSuffix(path, ".go") {
			continue
		}
		contents, err := bundled.FS.ReadFile(path)
		if err != nil {
			return err
		}
		visit(path, string(contents))
	}
	return nil
}
