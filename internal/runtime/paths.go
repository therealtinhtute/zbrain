package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	CWD           string
	HomeDir       string
	RuntimeDir    string
	ConfigFile    string
	WorkspacesDir string
	ProjectsDir   string
}

type Options struct {
	CWD        string
	HomeDir    string
	RuntimeDir string
}

func ResolvePaths(options Options) (Paths, error) {
	cwd := options.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return Paths{}, err
		}
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return Paths{}, err
	}

	home := options.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return Paths{}, err
	}

	runtimeDir := options.RuntimeDir
	if runtimeDir == "" {
		runtimeDir = os.Getenv("ZBRAIN_HOME")
	}
	if runtimeDir == "" {
		runtimeDir = filepath.Join(home, ".zbrain")
	}
	runtimeDir, err = filepath.Abs(runtimeDir)
	if err != nil {
		return Paths{}, err
	}

	return Paths{
		CWD:           cwd,
		HomeDir:       home,
		RuntimeDir:    runtimeDir,
		ConfigFile:    filepath.Join(runtimeDir, "config.yml"),
		WorkspacesDir: filepath.Join(runtimeDir, "workspaces"),
		ProjectsDir:   filepath.Join(runtimeDir, "projects"),
	}, nil
}

func CurrentTaskFilePath(paths Paths) string {
	sum := sha256.Sum256([]byte(paths.CWD))
	projectID := hex.EncodeToString(sum[:])[:16]
	return filepath.Join(paths.ProjectsDir, projectID, "current-task.md")
}

func IsSafeWorkspaceName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name != filepath.Base(name) {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return false
	}
	return true
}
