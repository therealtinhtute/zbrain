package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const (
	workspaceControlDirectoryName = ".zbrain"
	coordinationLockFileName      = "coordination.lock"
	generationFileName            = "generation.json"
)

type WorkspaceGeneration struct {
	Current   uint64 `json:"current"`
	Published uint64 `json:"published"`
}

type workspaceLock struct {
	file *os.File
	fd   int
}

func (lock *workspaceLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unix.Flock(lock.fd, unix.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func acquireWorkspaceLock(paths Paths, workspace string, exclusive bool) (*workspaceLock, error) {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return nil, err
	}
	controlDirectory := filepath.Join(root, workspaceControlDirectoryName)
	if err := ensureWorkspaceControlDirectory(root, controlDirectory); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(controlDirectory, coordinationLockFileName)
	if err := validateWorkspaceControlFile(lockPath); err != nil {
		return nil, fmt.Errorf("validate coordination lock: %w", err)
	}

	fd, err := unix.Open(lockPath, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open coordination lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), lockPath)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open coordination lock: invalid file descriptor")
	}
	closeWithError := func(operation string, cause error) (*workspaceLock, error) {
		_ = file.Close()
		return nil, fmt.Errorf("%s: %w", operation, cause)
	}
	info, err := file.Stat()
	if err != nil {
		return closeWithError("stat coordination lock", err)
	}
	if !info.Mode().IsRegular() {
		return closeWithError("validate coordination lock", fmt.Errorf("%q is not a regular file", lockPath))
	}
	if err := file.Chmod(0o600); err != nil {
		return closeWithError("set coordination lock permissions", err)
	}
	lockMode := unix.LOCK_SH
	if exclusive {
		lockMode = unix.LOCK_EX
	}
	if err := unix.Flock(fd, lockMode); err != nil {
		return closeWithError("acquire coordination lock", err)
	}
	return &workspaceLock{file: file, fd: fd}, nil
}

func workspaceControlPath(paths Paths, workspace string, name string) (string, error) {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return "", err
	}
	controlDirectory := filepath.Join(root, workspaceControlDirectoryName)
	if err := validateWorkspaceControlDirectory(root, controlDirectory); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	path := filepath.Join(controlDirectory, name)
	if err := validateWorkspaceControlFile(path); err != nil {
		return "", err
	}
	return path, nil
}

func CoordinationLockPath(paths Paths, workspace string) (string, error) {
	return workspaceControlPath(paths, workspace, coordinationLockFileName)
}

func GenerationPath(paths Paths, workspace string) (string, error) {
	return workspaceControlPath(paths, workspace, generationFileName)
}

func (store IndexStore) CoordinationLockPath(workspace string) (string, error) {
	return CoordinationLockPath(store.Paths, workspace)
}

func (store IndexStore) GenerationPath(workspace string) (string, error) {
	return GenerationPath(store.Paths, workspace)
}

func ensureWorkspaceControlDirectory(root string, directory string) error {
	if err := validateWorkspaceControlDirectory(root, directory); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(directory, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create workspace control directory: %w", err)
	}
	if err := validateWorkspaceControlDirectory(root, directory); err != nil {
		return fmt.Errorf("validate workspace control directory: %w", err)
	}
	return nil
}

func validateWorkspaceControlDirectory(root string, directory string) error {
	if !pathWithin(root, directory) {
		return fmt.Errorf("workspace control directory resolves outside workspace")
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("workspace control directory must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace control directory is not a directory")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if resolved != absolute || !pathWithin(root, resolved) {
		return fmt.Errorf("workspace control directory resolves outside workspace")
	}
	return nil
}

func validateWorkspaceControlFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q must not be a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	if resolved != absolute {
		return fmt.Errorf("%q resolves through a symlink", path)
	}
	return nil
}

func readWorkspaceGeneration(paths Paths, workspace string) (WorkspaceGeneration, error) {
	path, err := GenerationPath(paths, workspace)
	if err != nil {
		return WorkspaceGeneration{}, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceGeneration{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(contents, &fields); err != nil {
		return WorkspaceGeneration{}, fmt.Errorf("decode generation state: %w", err)
	}
	if len(fields) != 2 {
		return WorkspaceGeneration{}, fmt.Errorf("generation state must contain only current and published")
	}
	current, ok := fields["current"]
	if !ok || bytes.Equal(bytes.TrimSpace(current), []byte("null")) {
		return WorkspaceGeneration{}, fmt.Errorf("generation state current is required")
	}
	published, ok := fields["published"]
	if !ok || bytes.Equal(bytes.TrimSpace(published), []byte("null")) {
		return WorkspaceGeneration{}, fmt.Errorf("generation state published is required")
	}
	state := WorkspaceGeneration{}
	if err := json.Unmarshal(current, &state.Current); err != nil {
		return WorkspaceGeneration{}, fmt.Errorf("decode generation current: %w", err)
	}
	if err := json.Unmarshal(published, &state.Published); err != nil {
		return WorkspaceGeneration{}, fmt.Errorf("decode generation published: %w", err)
	}
	if state.Published > state.Current {
		return WorkspaceGeneration{}, fmt.Errorf("generation published %d is newer than current %d", state.Published, state.Current)
	}
	return state, nil
}

func writeWorkspaceGeneration(paths Paths, workspace string, state WorkspaceGeneration) error {
	root, err := ValidateWorkspace(paths, workspace)
	if err != nil {
		return err
	}
	controlDirectory := filepath.Join(root, workspaceControlDirectoryName)
	if err := ensureWorkspaceControlDirectory(root, controlDirectory); err != nil {
		return err
	}
	path := filepath.Join(controlDirectory, generationFileName)
	return writeWorkspaceGenerationFile(path, state)
}

func writeWorkspaceGenerationFile(path string, state WorkspaceGeneration) error {
	if state.Published > state.Current {
		return fmt.Errorf("generation published %d is newer than current %d", state.Published, state.Current)
	}
	if err := validateWorkspaceControlFile(path); err != nil {
		return err
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal generation state: %w", err)
	}
	encoded = append(encoded, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+generationFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("create generation state temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set generation state temporary permissions: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write generation state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync generation state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close generation state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish generation state: %w", err)
	}
	return nil
}

func ensureWorkspaceGenerationUnlocked(paths Paths, workspace string) (WorkspaceGeneration, error) {
	state, err := readWorkspaceGeneration(paths, workspace)
	if err == nil {
		return state, nil
	}
	if !os.IsNotExist(err) {
		return WorkspaceGeneration{}, err
	}
	state = WorkspaceGeneration{}
	if err := writeWorkspaceGeneration(paths, workspace, state); err != nil {
		return WorkspaceGeneration{}, err
	}
	return state, nil
}

func beginCanonicalMutationUnlocked(paths Paths, workspace string) (WorkspaceGeneration, error) {
	index := IndexStore{Paths: paths}
	if _, _, err := index.validatedIndexPaths(workspace); err != nil {
		return WorkspaceGeneration{}, err
	}
	state, err := readWorkspaceGeneration(paths, workspace)
	if os.IsNotExist(err) {
		state = WorkspaceGeneration{}
	} else if err != nil {
		return WorkspaceGeneration{}, fmt.Errorf("read workspace generation: %w", err)
	}
	if state.Current == ^uint64(0) {
		return WorkspaceGeneration{}, fmt.Errorf("workspace %q generation exhausted", workspace)
	}
	if err := index.markDirtyUnlocked(workspace); err != nil {
		return WorkspaceGeneration{}, err
	}
	state.Current++
	if err := writeWorkspaceGeneration(paths, workspace, state); err != nil {
		return WorkspaceGeneration{}, fmt.Errorf("advance workspace generation: %w", err)
	}
	return state, nil
}

const (
	workspaceGenerationHookBeforeCanonicalWrite     = "before-canonical-write"
	workspaceGenerationHookRebuildAfterScan         = "rebuild-after-scan"
	workspaceGenerationHookRebuildBeforePublication = "rebuild-before-publication"
	workspaceGenerationHookTrustedQueryAfterLocking = "trusted-query-after-locking"
)

var workspaceGenerationHooks struct {
	sync.RWMutex
	values map[string]func()
}

func setWorkspaceGenerationTestHook(boundary string, hook func()) func() {
	workspaceGenerationHooks.Lock()
	if workspaceGenerationHooks.values == nil {
		workspaceGenerationHooks.values = make(map[string]func())
	}
	previous := workspaceGenerationHooks.values[boundary]
	if hook == nil {
		delete(workspaceGenerationHooks.values, boundary)
	} else {
		workspaceGenerationHooks.values[boundary] = hook
	}
	workspaceGenerationHooks.Unlock()
	return func() {
		workspaceGenerationHooks.Lock()
		if previous == nil {
			delete(workspaceGenerationHooks.values, boundary)
		} else {
			workspaceGenerationHooks.values[boundary] = previous
		}
		workspaceGenerationHooks.Unlock()
	}
}

func runWorkspaceGenerationTestHook(boundary string) {
	workspaceGenerationHooks.RLock()
	hook := workspaceGenerationHooks.values[boundary]
	workspaceGenerationHooks.RUnlock()
	if hook != nil {
		hook()
	}
}
