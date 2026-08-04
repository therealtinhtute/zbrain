package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateWorkspace returns the canonical root of an existing workspace.
// Workspace names are never cleaned or normalized before validation.
func ValidateWorkspace(paths Paths, name string) (string, error) {
	if !IsSafeWorkspaceName(name) {
		return "", fmt.Errorf("workspace name must use lowercase letters, numbers, or hyphens only")
	}

	workspacesRoot, err := canonicalExistingDirectory(paths.WorkspacesDir)
	if err != nil {
		return "", fmt.Errorf("validate workspaces root: %w", err)
	}

	root := filepath.Join(workspacesRoot, name)
	info, err := os.Lstat(root)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("workspace %q does not exist", name)
	}
	if err != nil {
		return "", fmt.Errorf("stat workspace %q: %w", name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("workspace %q must not be a symlink", name)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace %q is not a directory", name)
	}

	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q: %w", name, err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q absolute path: %w", name, err)
	}
	if !pathWithin(workspacesRoot, resolvedRoot) {
		return "", fmt.Errorf("workspace %q resolves outside the workspaces root", name)
	}
	return resolvedRoot, nil
}

// ResolveWorkspacePath returns a canonical path inside an existing workspace.
// Existing path components are resolved so symlink escapes are rejected while
// non-existent final components remain valid targets for callers that write.
func ResolveWorkspacePath(paths Paths, workspace string, relative string) (string, error) {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return "", err
	}

	clean, err := safeRelativePath(relative)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, clean)
	if !pathWithin(root, target) {
		return "", fmt.Errorf("workspace path %q is outside workspace %q", relative, workspace)
	}
	if err := validateExistingPath(root, target); err != nil {
		return "", fmt.Errorf("workspace path %q: %w", relative, err)
	}
	return target, nil
}

func canonicalExistingDirectory(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%q must not be a symlink", path)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func safeRelativePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("workspace path must not be empty")
	}
	native := filepath.FromSlash(path)
	clean := filepath.Clean(native)
	if filepath.IsAbs(native) || filepath.VolumeName(native) != "" || clean != native || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("workspace path %q is not safe", path)
	}
	return clean, nil
}

func validateExistingPath(root string, target string) error {
	candidate := target
	for {
		_, err := os.Lstat(candidate)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(candidate)
			if err != nil {
				return fmt.Errorf("resolve existing path: %w", err)
			}
			resolved, err = filepath.Abs(resolved)
			if err != nil {
				return fmt.Errorf("resolve existing path absolute path: %w", err)
			}
			if !pathWithin(root, resolved) {
				return fmt.Errorf("resolved path escapes workspace")
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return fmt.Errorf("path has no existing workspace ancestor")
		}
		candidate = parent
	}
}

func pathWithin(root string, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
