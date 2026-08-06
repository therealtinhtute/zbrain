package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DefaultWorkspace string
}

func ReadConfig(path string) (Config, error) {
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}

	config := Config{}
	for _, line := range strings.Split(string(contents), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "default_workspace" {
			continue
		}
		config.DefaultWorkspace = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return config, nil
}

func WriteConfig(path string, config Config) error {
	if err := ensureDirectoryMode(filepath.Dir(path), runtimeDirectoryMode); err != nil {
		return err
	}
	contents := "runtime_version: go-v1\n"
	if config.DefaultWorkspace != "" {
		contents += "default_workspace: " + config.DefaultWorkspace + "\n"
	}
	if err := os.WriteFile(path, []byte(contents), runtimeMetadataMode); err != nil {
		return err
	}
	return ensureFileMode(path, runtimeMetadataMode)
}

func EnsureConfig(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		if err := ensureDirectoryMode(filepath.Dir(path), runtimeDirectoryMode); err != nil {
			return false, err
		}
		return false, ensureFileMode(path, runtimeMetadataMode)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return true, WriteConfig(path, Config{})
}
