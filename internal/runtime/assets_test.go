package runtime

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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

var documentedEmbeddedAssetPaths = []string{
	"README.md",
	"agents/wiki-planner.md",
	"agents/wiki-qmd-selector.md",
	"engine/claude-rules.md",
	"engine/codex-rules.md",
	"engine/constraints.md",
	"engine/evidence-rules.md",
	"engine/retrieval-rules.md",
	"engine/system-prompt.md",
	"skills/zbrain-ask/SKILL.md",
	"skills/zbrain-ingest/SKILL.md",
	"skills/zbrain-ingest/references/pipeline.md",
	"skills/zbrain-learn/SKILL.md",
	"skills/zbrain-research/SKILL.md",
	"templates/axiom.md",
	"templates/evidence-index.md",
	"templates/evidence-manifest.yaml",
	"templates/evidence-source.yaml",
	"templates/mental-model.md",
	"templates/project.md",
	"templates/workspace.md",
	"workspaces/.gitkeep",
}

func TestEmbeddedAssetLayout(t *testing.T) {
	tmp := t.TempDir()
	paths, err := ResolvePaths(Options{CWD: tmp, HomeDir: tmp, RuntimeDir: filepath.Join(tmp, ".zbrain")})
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}

	result, err := ExtractBundledAssets(paths)
	if err != nil {
		t.Fatalf("ExtractBundledAssets() error = %v", err)
	}

	expectedCopied := []string{}
	expectedSkipped := []string{}
	for _, path := range documentedEmbeddedAssetPaths {
		if strings.HasPrefix(path, "workspaces/") {
			expectedSkipped = append(expectedSkipped, path)
			continue
		}
		expectedCopied = append(expectedCopied, path)
	}
	assertAssetPathSet(t, "copied", result.Copied, expectedCopied)
	assertAssetPathSet(t, "skipped", result.Skipped, expectedSkipped)

	for _, root := range []string{"README.md", "agents", "engine", "skills", "templates"} {
		if _, err := os.Stat(filepath.Join(paths.RuntimeDir, filepath.FromSlash(root))); err != nil {
			t.Fatalf("missing extracted runtime path %q: %v", root, err)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.RuntimeDir, "assets")); !os.IsNotExist(err) {
		t.Fatalf("setup must not create a nested assets directory, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.RuntimeDir, "workspaces")); !os.IsNotExist(err) {
		t.Fatalf("workspace seed assets must not create active workspaces, stat error = %v", err)
	}
}

func TestEmbeddedAssetParity(t *testing.T) {
	actual := bundledAssetPaths(t)
	expected := append([]string(nil), documentedEmbeddedAssetPaths...)
	assertUniqueAssetPaths(t, "documented", expected)
	assertUniqueAssetPaths(t, "embedded", actual)
	assertAssetPathSet(t, "embedded", actual, expected)
}

func assertAssetPathSet(t *testing.T, label string, actual, expected []string) {
	t.Helper()
	actual = append([]string(nil), actual...)
	expected = append([]string(nil), expected...)
	sort.Strings(actual)
	sort.Strings(expected)
	if len(actual) != len(expected) {
		t.Fatalf("%s asset paths = %v, want %v", label, actual, expected)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf("%s asset paths = %v, want %v", label, actual, expected)
		}
	}
}

func assertUniqueAssetPaths(t *testing.T, label string, paths []string) {
	t.Helper()
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			t.Fatalf("duplicate %s asset path %q", label, path)
		}
		seen[path] = struct{}{}
	}
}

func bundledAssetPaths(t *testing.T) []string {
	t.Helper()
	paths := []string{}
	err := fs.WalkDir(bundled.FS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || strings.HasSuffix(path, ".go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk bundled assets: %v", err)
	}
	return paths
}

func TestSkillWorkspaceScope(t *testing.T) {
	contents := make(map[string]string)
	if err := fsWalkBundled(func(path string, body string) {
		contents[path] = body
	}); err != nil {
		t.Fatalf("walk bundled assets: %v", err)
	}

	for path, body := range contents {
		if !strings.Contains(body, "--workspace") && !strings.Contains(body, "--include") {
			continue
		}
		if !strings.Contains(body, "zbrain workspace current") {
			t.Errorf("%s uses workspace flags without primary-workspace resolution", path)
		}
		if !strings.Contains(body, "explicit") {
			t.Errorf("%s uses workspace flags without explicit scope consent", path)
		}
		if strings.Contains(body, "--include") && !strings.Contains(body, "secondary") {
			t.Errorf("%s uses --include without naming secondary workspace scope", path)
		}
	}

	required := map[string][]string{
		"agents/wiki-planner.md": {
			"active workspace only",
			"secondary workspace",
		},
		"engine/system-prompt.md": {
			"active workspace only",
		},
	}
	for path, tokens := range required {
		body, ok := contents[path]
		if !ok {
			t.Fatalf("missing bundled asset %q", path)
		}
		for _, token := range tokens {
			if !strings.Contains(body, token) {
				t.Errorf("%s is missing workspace scope guidance %q", path, token)
			}
		}
	}
}

func TestSkillShellSafety(t *testing.T) {
	forbidden := []string{
		`"{question}"`,
		`'<claim body>\n'`,
		`--workspace <`,
		`--include <`,
		`--file <`,
		`--origin <`,
		`--tier <`,
		`--title <`,
		`--basis <`,
		`--evidence <`,
		`--support <`,
		`--conflicts-with <`,
		`--reason <`,
		` <query>`,
	}
	contents := make(map[string]string)
	if err := fsWalkBundled(func(path string, body string) {
		contents[path] = body
	}); err != nil {
		t.Fatalf("walk bundled assets: %v", err)
	}
	for path, body := range contents {
		for _, token := range forbidden {
			if strings.Contains(body, token) {
				t.Errorf("%s contains unsafe shell placeholder %q", path, token)
			}
		}
	}

	required := map[string][]string{
		"skills/zbrain-ask/SKILL.md": {
			`args+=(--workspace "$workspace")`,
			`args+=(--include "$include")`,
			`args+=("$question")`,
			`"${args[@]}"`,
		},
		"skills/zbrain-learn/SKILL.md": {
			`printf '%s\n' "$body" | "${draft_args[@]}"`,
		},
		"engine/claude-rules.md": {
			`Never concatenate user text into a shell command string.`,
		},
	}
	for path, tokens := range required {
		body, ok := contents[path]
		if !ok {
			t.Fatalf("missing bundled asset %q", path)
		}
		for _, token := range tokens {
			if !strings.Contains(body, token) {
				t.Errorf("%s is missing safe argv guidance %q", path, token)
			}
		}
	}
}

func TestActiveAssetScope(t *testing.T) {
	forbidden := []string{
		"curl",
		"wget",
		"urllib.request.urlopen",
		"defuddle.md",
		"r.jina.ai",
		"--use-proxy",
		"fetch_local.py",
		"fetch.sh",
		"evidence-apply",
		"evidence-qa",
		"pending-external",
		"zbrain analysis",
		"zbrain qa",
		"zbrain apply",
		"zbrain fetch",
	}
	if err := fsWalkBundled(func(path string, body string) {
		for _, token := range forbidden {
			if strings.Contains(body, token) {
				t.Errorf("%s contains out-of-scope active asset token %q", path, token)
			}
		}
	}); err != nil {
		t.Fatalf("walk bundled assets: %v", err)
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
