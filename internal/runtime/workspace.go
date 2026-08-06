package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var WikiTiers = []string{"axioms", "mental-models", "projects", "decisions"}

type WorkspaceCurrent struct {
	ProjectRoot         string   `json:"project_root"`
	Workspace           string   `json:"workspace"`
	SecondaryWorkspaces []string `json:"secondary_workspaces"`
}

func CreateWorkspace(paths Paths, name string, now time.Time) error {
	if !IsSafeWorkspaceName(name) {
		return fmt.Errorf("workspace name must use lowercase letters, numbers, or hyphens only")
	}

	root := filepath.Join(paths.WorkspacesDir, name)
	if _, err := os.Stat(root); err == nil {
		return fmt.Errorf("workspace %q already exists", name)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := ensureDirectoryMode(paths.WorkspacesDir, runtimeDirectoryMode); err != nil {
		return err
	}
	if err := ensureDirectoryMode(root, runtimeDirectoryMode); err != nil {
		return err
	}
	for _, tier := range WikiTiers {
		if err := ensureDirectoryMode(filepath.Join(root, "wiki", tier), runtimeDirectoryMode); err != nil {
			return err
		}
	}
	for _, dir := range []string{
		"agents",
		"evidence/sources",
		"evidence/analysis",
		"evidence/qa",
		"evidence/applied",
		"evidence/archive",
	} {
		if err := ensureDirectoryMode(filepath.Join(root, dir), runtimeDirectoryMode); err != nil {
			return err
		}
	}

	workspaceReadme := fmt.Sprintf("# %s\n\nCreated: %s\n", name, now.UTC().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(root, "workspace.md"), []byte(workspaceReadme), runtimeMetadataMode); err != nil {
		return err
	}
	if err := ensureFileMode(filepath.Join(root, "workspace.md"), runtimeMetadataMode); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "evidence", "_index.md"), []byte("# Evidence Index\n"), runtimeMetadataMode); err != nil {
		return err
	}
	if err := ensureFileMode(filepath.Join(root, "evidence", "_index.md"), runtimeMetadataMode); err != nil {
		return err
	}

	config, err := ReadConfig(paths.ConfigFile)
	if err != nil {
		return err
	}
	if config.DefaultWorkspace == "" {
		config.DefaultWorkspace = name
		if err := WriteConfig(paths.ConfigFile, config); err != nil {
			return err
		}
	}
	return nil
}

func ResolveCurrentWorkspace(paths Paths) (WorkspaceCurrent, error) {
	config, err := ReadConfig(paths.ConfigFile)
	if err != nil {
		return WorkspaceCurrent{}, err
	}
	workspace := config.DefaultWorkspace
	if workspace == "" {
		return WorkspaceCurrent{}, errors.New("no default workspace configured; run `zbrain workspace create <name>` first")
	}
	if _, err := ValidateWorkspace(paths, workspace); err != nil {
		return WorkspaceCurrent{}, err
	}
	return WorkspaceCurrent{
		ProjectRoot:         paths.CWD,
		Workspace:           workspace,
		SecondaryWorkspaces: []string{},
	}, nil
}

func MarshalCurrent(current WorkspaceCurrent) ([]byte, error) {
	return json.MarshalIndent(current, "", "  ")
}
