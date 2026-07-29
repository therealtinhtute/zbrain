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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	contents := "runtime_version: go-v1\n"
	if config.DefaultWorkspace != "" {
		contents += "default_workspace: " + config.DefaultWorkspace + "\n"
	}
	return os.WriteFile(path, []byte(contents), 0o644)
}

func EnsureConfig(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	return true, WriteConfig(path, Config{})
}
