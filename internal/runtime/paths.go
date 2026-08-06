package runtime

import (
	"os"
	"path/filepath"
)

const (
	runtimeDirectoryMode  os.FileMode = 0o700
	runtimeMetadataMode   os.FileMode = 0o600
	evidenceDirectoryMode os.FileMode = 0o700
	evidenceFileMode      os.FileMode = 0o400
	derivedIndexMode      os.FileMode = 0o600
)

type Paths struct {
	CWD           string
	HomeDir       string
	RuntimeDir    string
	ConfigFile    string
	WorkspacesDir string
	IndexesDir    string
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
		IndexesDir:    filepath.Join(runtimeDir, "indexes"),
	}, nil
}

func ensureDirectoryMode(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func ensureFileMode(path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func IsSafeWorkspaceName(name string) bool {
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
