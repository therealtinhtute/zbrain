// fixture-gen drives the Go (oracle) runtime through the m0 surface and
// emits a normalized manifest of the resulting runtime tree. The Rust
// zbrain-parity binary emits the same schema; scripts/parity.sh diffs them.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	zruntime "github.com/therealtinhtute/zbrain/internal/runtime"
)

type treeEntry struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	Mode string `json:"mode"`
}

type manifest struct {
	Config        string      `json:"config"`
	WorkspaceMD   string      `json:"workspace_md"`
	EvidenceIndex string      `json:"evidence_index"`
	Tree          []treeEntry `json:"tree"`
	DefaultRead   string      `json:"default_read"`
	GenerationRel string      `json:"generation_rel"`
}

func main() {
	home := flag.String("home", "", "runtime home directory (required)")
	workspace := flag.String("workspace", "research", "workspace name to create")
	flag.Parse()
	if *home == "" {
		fail("home is required")
	}
	if err := run(*home, *workspace); err != nil {
		fail(err.Error())
	}
}

func run(home, workspace string) error {
	paths, err := zruntime.ResolvePaths(zruntime.Options{
		CWD:        home,
		HomeDir:    home,
		RuntimeDir: filepath.Join(home, "runtime"),
	})
	if err != nil {
		return fmt.Errorf("resolve paths: %w", err)
	}
	if _, err := zruntime.EnsureConfig(paths.ConfigFile); err != nil {
		return fmt.Errorf("ensure config: %w", err)
	}
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	if err := zruntime.CreateWorkspace(paths, workspace, now); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	current, err := zruntime.ResolveCurrentWorkspace(paths)
	if err != nil {
		return fmt.Errorf("resolve current: %w", err)
	}

	root := filepath.Join(paths.WorkspacesDir, workspace)
	config, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	workspaceMD, err := os.ReadFile(filepath.Join(root, "workspace.md"))
	if err != nil {
		return fmt.Errorf("read workspace.md: %w", err)
	}
	evidenceIndex, err := os.ReadFile(filepath.Join(root, "evidence", "_index.md"))
	if err != nil {
		return fmt.Errorf("read evidence/_index.md: %w", err)
	}

	tree, err := walkTree(paths.RuntimeDir)
	if err != nil {
		return err
	}
	resolvedRoot, err := zruntime.ValidateWorkspace(paths, workspace)
	if err != nil {
		return fmt.Errorf("validate workspace: %w", err)
	}
	genPath, err := zruntime.IndexStore{Paths: paths}.GenerationPath(workspace)
	if err != nil {
		return fmt.Errorf("generation path: %w", err)
	}
	controlRel, err := filepath.Rel(resolvedRoot, genPath)
	if err != nil {
		return fmt.Errorf("rel generation path: %w", err)
	}

	out, err := json.MarshalIndent(manifest{
		Config:        string(config),
		WorkspaceMD:   string(workspaceMD),
		EvidenceIndex: string(evidenceIndex),
		Tree:          tree,
		DefaultRead:   current.Workspace,
		GenerationRel: controlRel,
	}, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	return nil
}

func walkTree(runtimeDir string) ([]treeEntry, error) {
	var tree []treeEntry
	err := filepath.WalkDir(runtimeDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(runtimeDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		}
		tree = append(tree, treeEntry{Path: rel, Kind: kind, Mode: fmt.Sprintf("%04o", info.Mode().Perm())})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk tree: %w", err)
	}
	sort.Slice(tree, func(i, j int) bool { return tree[i].Path < tree[j].Path })
	return tree, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "fixture-gen: "+message)
	os.Exit(1)
}
