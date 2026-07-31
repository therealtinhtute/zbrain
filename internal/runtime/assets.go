package runtime

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	bundled "github.com/therealtinhtute/zbrain/assets"
)

type ExtractionResult struct {
	Copied  []string
	Skipped []string
}

func ExtractBundledAssets(paths Paths) (ExtractionResult, error) {
	result := ExtractionResult{}
	err := fs.WalkDir(bundled.FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasPrefix(path, "workspaces/") {
			result.Skipped = append(result.Skipped, path)
			return nil
		}

		destination := filepath.Join(paths.RuntimeDir, filepath.FromSlash(path))
		contents, err := bundled.FS.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(destination, contents, 0o644); err != nil {
			return err
		}
		result.Copied = append(result.Copied, path)
		return nil
	})
	return result, err
}
