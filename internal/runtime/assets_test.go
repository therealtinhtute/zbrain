package runtime

import (
	"os"
	"path/filepath"
	"testing"
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
}
